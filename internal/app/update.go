package app

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Once at each start the program asks GitHub which versions of it have been published, to say
// so when there is a newer one. It is one request for a public page: nothing about the
// computer, the recordings or the person goes with it. It can be turned off.

const releases = "https://api.github.com/repos/MassimoDanieli/C_bass/releases?per_page=20"

// latest is what the check found.
type latest struct {
	sync.Mutex
	version, page string // the newest version published and where it is, if it is newer than this one
	asked, failed bool
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
		found, page, err := newest(releases)
		g.latest.Lock()
		defer g.latest.Unlock()
		g.latest.asked, g.latest.failed = true, err != nil
		if err != nil {
			note("looking for a newer version: %v", err)
			return
		}
		if newer(found, g.version) {
			g.latest.version, g.latest.page = found, page
			note("version %s is out", found)
		}
	}()
}

// newest is the highest version among the releases listed at an address.
func newest(address string) (version, page string, err error) {
	client := &http.Client{Timeout: 8 * time.Second}
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return "", "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "C_bass")
	response, err := client.Do(request)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", &http.ProtocolError{ErrorString: response.Status}
	}
	var list []struct {
		Tag   string `json:"tag_name"`
		Page  string `json:"html_url"`
		Draft bool   `json:"draft"`
	}
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		return "", "", err
	}
	for _, release := range list {
		tag := strings.TrimPrefix(release.Tag, "v")
		if _, ok := parseVersion(tag); !ok || release.Draft {
			continue
		}
		if version == "" || newer(tag, version) {
			version, page = tag, release.Page
		}
	}
	return version, page, nil
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
