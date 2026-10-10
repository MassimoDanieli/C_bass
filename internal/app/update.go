package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MassimoDanieli/c_bass/internal/selfupdate"
)

// Once at each start the program asks GitHub which versions of it have been published, to say
// so when there is a newer one. It is one request for a public page: nothing about the
// computer, the recordings or the person goes with it. It can be turned off.

const releases = "https://api.github.com/repos/MassimoDanieli/C_bass/releases?per_page=20"

// releasesAt is where the list is asked for: another address can be given, for trying an update out.
func releasesAt() string {
	if address := os.Getenv("CBASS_RELEASES"); address != "" {
		return address
	}
	return releases
}

// latest is what the check found.
type latest struct {
	sync.Mutex
	version, page string // the newest version published and where it is, if it is newer than this one
	asked, failed bool
	assets        []selfupdate.Asset // its files

	// the update made from inside the program
	updating    bool
	done, total int64
	restart     func() error // set when the new version is in place
	problem     error        // set when it did not work: then the page is opened instead
}

// progress says how the update is going.
func (l *latest) progress() (updating bool, done, total int64, restart func() error, problem error) {
	l.Lock()
	defer l.Unlock()
	return l.updating, l.done, l.total, l.restart, l.problem
}

// busy says whether an update is under way, for the window to keep painting its progress.
func (l *latest) busy() bool {
	l.Lock()
	defer l.Unlock()
	return l.updating || l.restart != nil
}

// selfFile is the file this program can update itself from, if it can.
func (g *Game) selfFile() (selfupdate.Asset, bool) {
	exe, err := os.Executable()
	if err != nil {
		return selfupdate.Asset{}, false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	plan := selfupdate.Plan(exe)
	g.latest.Lock()
	assets, problem := g.latest.assets, g.latest.problem
	g.latest.Unlock()
	if plan == "" || problem != nil {
		return selfupdate.Asset{}, false
	}
	asset, err := selfupdate.Find(assets, plan)
	return asset, err == nil
}

// update fetches the new version and puts it in place, away from the window; takeUpdate starts it.
func (g *Game) update(asset selfupdate.Asset) {
	g.latest.Lock()
	if g.latest.updating {
		g.latest.Unlock()
		return
	}
	g.latest.updating, g.latest.done, g.latest.total = true, 0, 0
	g.latest.Unlock()
	go func() {
		exe, err := os.Executable()
		if err == nil {
			if resolved, e := filepath.EvalSymlinks(exe); e == nil {
				exe = resolved
			}
		}
		var restart func() error
		if err == nil {
			restart, err = selfupdate.Apply(asset, exe, func(done, total int64) {
				g.latest.Lock()
				g.latest.done, g.latest.total = done, total
				g.latest.Unlock()
			})
		}
		g.latest.Lock()
		defer g.latest.Unlock()
		g.latest.updating = false
		if err != nil {
			note("updating to %s: %v", g.latest.version, err)
			g.latest.problem = err
			return
		}
		note("version %s is in place", g.latest.version)
		g.latest.restart = restart
	}()
}

// takeUpdate starts the new version once it is in place, and closes this one.
func (g *Game) takeUpdate() {
	if os.Getenv("CBASS_UPDATE_NOW") != "" && !g.latest.busy() { // for trying an update out without a person to press
		if asset, ok := g.selfFile(); ok {
			os.Unsetenv("CBASS_UPDATE_NOW")
			g.update(asset)
		}
	}
	_, _, _, restart, _ := g.latest.progress()
	if restart == nil {
		return
	}
	g.settings.UpdatedFrom = g.version
	g.saveSettings()
	if err := restart(); err != nil {
		note("starting the new version: %v", err)
		g.latest.Lock()
		g.latest.restart, g.latest.problem = nil, err
		g.latest.Unlock()
		g.say(g.t("L'aggiornamento è al suo posto: riapri il programma.", "The update is in place: open the program again."))
		return
	}
	g.closeSong()
	os.Exit(0)
}

// updateButton is the button that says a newer version is out: it updates the program when
// it can, or opens the page of the release when it cannot. Its text says which.
func (g *Game) updateButton(c *canvas, id string, r func(width float32) rect, look look) {
	found, page, _, _ := g.latest.get()
	if found == "" {
		return
	}
	updating, done, total, restart, problem := g.latest.progress()
	asset, can := g.selfFile()
	text := g.t("È uscita la versione ", "Version ") + found + g.t(": scarica", " is out: download")
	switch {
	case restart != nil:
		text = g.t("Riapro il programma…", "Opening the program again…")
	case updating && total > 0:
		text = fmt.Sprintf(g.t("Scarico la versione %s… %d%%", "Fetching version %s… %d%%"), found, done*100/total)
	case updating:
		text = fmt.Sprintf(g.t("Scarico la versione %s…", "Fetching version %s…"), found)
	case can:
		text = g.t("È uscita la versione ", "Version ") + found + g.t(": aggiorna", " is out: update")
	case problem != nil:
		text = g.t("Aggiornamento non riuscito: scarica la versione ", "The update did not work: download version ") + found
	}
	if g.button(c, id, r(c.width(text, 14, medium)+28), text, look) && !updating && restart == nil {
		if can {
			g.update(asset)
		} else {
			show(page)
		}
	}
}

func (l *latest) get() (version, page string, asked, failed bool) {
	l.Lock()
	defer l.Unlock()
	return l.version, l.page, l.asked, l.failed
}

// lookForUpdate asks, away from the window, and keeps the answer.
func (g *Game) lookForUpdate() {
	if g.settings.NoUpdateCheck || g.shot != "" || os.Getenv("CBASS_NO_UPDATE") != "" {
		return
	}
	if _, ok := parseVersion(g.version); !ok {
		return // a build with no version of its own cannot be older than anything
	}
	go func() {
		found, page, assets, err := newest(releasesAt())
		g.latest.Lock()
		defer g.latest.Unlock()
		g.latest.asked, g.latest.failed = true, err != nil
		if err != nil {
			note("looking for a newer version: %v", err)
			return
		}
		if newer(found, g.version) {
			g.latest.version, g.latest.page, g.latest.assets = found, page, assets
			note("version %s is out", found)
		}
	}()
}

// newest is the highest version among the releases listed at an address.
func newest(address string) (version, page string, assets []selfupdate.Asset, err error) {
	client := &http.Client{Timeout: 8 * time.Second}
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return "", "", nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "C_bass")
	response, err := client.Do(request)
	if err != nil {
		return "", "", nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", nil, &http.ProtocolError{ErrorString: response.Status}
	}
	var list []struct {
		Tag    string             `json:"tag_name"`
		Page   string             `json:"html_url"`
		Draft  bool               `json:"draft"`
		Assets []selfupdate.Asset `json:"assets"`
	}
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		return "", "", nil, err
	}
	for _, release := range list {
		tag := strings.TrimPrefix(release.Tag, "v")
		if _, ok := parseVersion(tag); !ok || release.Draft {
			continue
		}
		if version == "" || newer(tag, version) {
			version, page, assets = tag, release.Page, release.Assets
		}
	}
	return version, page, assets, nil
}

// version is a version number taken apart: 0.7.0-beta.1 is 0, 7, 0 and "beta.1".
type version struct {
	numbers [3]int
	before  string // what follows a hyphen: a version before the one the numbers name
}

func parseVersion(text string) (version, bool) {
	var v version
	text = strings.TrimPrefix(strings.TrimSpace(text), "v")
	text, v.before, _ = strings.Cut(text, "-")
	parts := strings.Split(text, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return v, false
		}
		v.numbers[i] = n
	}
	return v, true
}

// newer says whether version a comes after version b.
func newer(a, b string) bool {
	x, ok1 := parseVersion(a)
	y, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range x.numbers {
		if x.numbers[i] != y.numbers[i] {
			return x.numbers[i] > y.numbers[i]
		}
	}
	switch {
	case x.before == y.before:
		return false
	case x.before == "": // 1.0.0 comes after 1.0.0-beta.3
		return true
	case y.before == "":
		return false
	}
	// beta.2 before beta.10: piece by piece, numbers as numbers
	p, q := strings.Split(x.before, "."), strings.Split(y.before, ".")
	for i := 0; i < len(p) && i < len(q); i++ {
		m, err1 := strconv.Atoi(p[i])
		n, err2 := strconv.Atoi(q[i])
		switch {
		case err1 == nil && err2 == nil:
			if m != n {
				return m > n
			}
		case p[i] != q[i]:
			return p[i] > q[i]
		}
	}
	return len(p) > len(q)
}
