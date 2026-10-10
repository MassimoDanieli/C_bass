// Package app is the window: choose a recording, wait while it is worked out, then play it
// with the tablature scrolling by and the neck showing the note to play.
package app

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/demo"
	"github.com/MassimoDanieli/c_bass/internal/fetch"
	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/icon"
	"github.com/MassimoDanieli/c_bass/internal/library"
	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/selfupdate"
)

type screen int

const (
	home screen = iota
	working
	playing
)

// settings are what the program remembers from one time to the next.
type settings struct {
	// Language is "it" or "en"; empty until chosen, when the computer's own language decides.
	Language string `json:"language,omitempty"`
	English  bool   `json:"english"`
	// Seeded says the pieces that come with the program have been put in the library once.
	Seeded bool `json:"seeded"`
	// Sound is the version of the instruments those pieces were last played with.
	Sound  int     `json:"sound"`
	Tuning string  `json:"tuning"`
	Bass   float64 `json:"bass"` // volume of the bass, 0 to 1.5
	Rest   float64 `json:"rest"` // volume of everything else, 0 to 1
	// the helps for practising, and where on the neck the part is fingered
	CountIn   bool `json:"countIn"`
	Metronome bool `json:"metronome"`
	Quicken   bool `json:"quicken"`
	Low       bool `json:"lowPosition"`
	// NoUpdateCheck turns off the question asked of GitHub at start: is there a newer version?
	NoUpdateCheck bool `json:"noUpdateCheck"`
	// UpdatedFrom is the version the program updated itself from, until the new one has said so.
	UpdatedFrom string `json:"updatedFrom,omitempty"`
}

// Game is the whole program, as the window library wants it.
type Game struct {
	version  string
	lib      *library.Library
	entries  []library.Entry
	settings settings
	screen   screen

	job  *job
	song *song

	in          pointer
	keys        []ebiten.Key
	active      string // the control the mouse went down on
	confirm     string // the recording about to be removed
	scroll      float32
	notice      string
	noticed     time.Time
	good        bool   // the notice is good news, not a complaint
	stays       bool   // the notice waits for a click: whoever it is for may be away
	menu        bool   // the list of ways to save the part is open
	guide       string // "help" or "about" when one of those pages is open over the screen
	guideScroll float32
	latest      latest
	awake       int // frames still to paint: an idle window is left as it is
	frames      int
	seen        [4]float32
	plan        string         // the file this program can update itself from; "" if it cannot
	planOnce    sync.Once      // the plan is made once
	choose      chan []pending // the answer of the file dialog: nil if nothing was chosen
	queue       []pending      // recordings waiting to be worked out
	lot         batch          // what became of the ones given together

	scale float32
	w, h  float32

	shot       string // write a picture of the window here, then leave
	shotAt     float64
	shotFrames int
}

// Run opens the window and stays until it is closed.
func Run(version string) error {
	lib, err := library.Open()
	if err != nil {
		return err
	}
	openDiary(version)
	g := &Game{version: version, lib: lib, settings: settings{Tuning: "4", Bass: 1, Rest: 1}, scale: 1}
	g.loadSettings()
	if g.settings.Language == "" {
		g.settings.English = systemLanguage() != "it"
	}
	if !g.settings.Seeded && os.Getenv("CBASS_NO_SEED") == "" {
		if _, err := demo.Install(lib); err == nil {
			g.settings.Seeded, g.settings.Sound = true, demo.Sound
			g.saveSettings()
		}
	} else if g.settings.Seeded && g.settings.Sound < demo.Sound && os.Getenv("CBASS_NO_SEED") == "" {
		// the instruments have been made better since: the pieces are played again
		if err := demo.Refresh(lib); err == nil {
			g.settings.Sound = demo.Sound
			g.saveSettings()
		}
	}
	g.entries = lib.List()
	g.shot = os.Getenv("CBASS_SHOT")
	g.guide = os.Getenv("CBASS_GUIDE") // for a picture of those pages
	if from := g.settings.UpdatedFrom; from != "" {
		if exe, err := os.Executable(); err == nil {
			selfupdate.Cleanup(exe) // the version the update replaced
		}
		if from != version {
			g.tell(g.t("Aggiornato: dalla versione ", "Updated: from version ") + from + g.t(" alla ", " to ") + version)
		}
		g.settings.UpdatedFrom = ""
		g.saveSettings()
	}
	g.lookForUpdate()
	if pretend := os.Getenv("CBASS_LATEST"); pretend != "" { // for a picture of the notice
		g.latest.version, g.latest.page, g.latest.asked = pretend, homePage+"/releases", true
	}
	g.shotAt, _ = strconv.ParseFloat(os.Getenv("CBASS_SHOT_AT"), 64)
	if os.Getenv("CBASS_SHOT_DO") == "working" { // a picture of the work in progress, without doing any
		g.job = &job{title: "Giro di prova", started: time.Now(), stage: stageSeparating, done: 12, total: 31, downloaded: true}
		g.screen = working
	} else if open := os.Getenv("CBASS_OPEN"); open != "" {
		switch {
		case lib.Has(open):
			g.openEntry(open)
		case strings.HasPrefix(open, "http://") || strings.HasPrefix(open, "https://"):
			g.enqueue([]pending{{name: open, link: open}})
		default:
			g.openPath(open)
		}
	}
	ebiten.SetWindowTitle("C_bass")
	width, height := 1180, 820
	fmt.Sscanf(os.Getenv("CBASS_SIZE"), "%dx%d", &width, &height) // for trying other sizes
	ebiten.SetWindowSize(width, height)
	ebiten.SetScreenClearedEveryFrame(false)
	ebiten.SetWindowSizeLimits(1040, 740, -1, -1)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetWindowIcon([]image.Image{icon.Draw(64, 0), icon.Draw(128, 0), icon.Draw(256, 0)})
	return ebiten.RunGame(g)
}

func settingsPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "C_bass", "settings.json")
}

func (g *Game) loadSettings() {
	if data, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(data, &g.settings)
	}
	g.settings.Bass = math.Max(0, math.Min(1.5, g.settings.Bass))
	g.settings.Rest = math.Max(0, math.Min(1, g.settings.Rest))
}

func (g *Game) saveSettings() {
	path := settingsPath()
	if path == "" || g.shot != "" {
		return
	}
	if data, err := json.MarshalIndent(g.settings, "", " "); err == nil {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, data, 0o644)
	}
}

// switchLanguage goes from one language to the other, and remembers the choice.
func (g *Game) switchLanguage() {
	g.settings.English = !g.settings.English
	g.settings.Language = map[bool]string{true: "en", false: "it"}[g.settings.English]
	g.saveSettings()
}

// titleOf is how a recording is named: its title, and its key when it has one.
func (g *Game) titleOf(title, key string) string {
	if key == "" {
		return title
	}
	for midi := 0; midi < 12; midi++ {
		if fretboard.PitchName(midi) == key {
			return title + " " + g.t("in", "in") + " " + g.noteName(midi)
		}
	}
	return title
}

// t picks the Italian or the English of a text.
func (g *Game) t(italian, english string) string {
	if g.settings.English {
		return english
	}
	return italian
}

// say shows a complaint at the foot of the window for a few seconds.
func (g *Game) say(message string) {
	g.notice, g.noticed, g.good, g.stays = message, time.Now(), false, false
}

// tell shows good news the same way.
func (g *Game) tell(message string) {
	g.notice, g.noticed, g.good, g.stays = message, time.Now(), true, false
}

// showing says whether a notice is on the screen.
func (g *Game) showing() bool {
	return g.notice != "" && (g.stays || time.Since(g.noticed) <= noticeTime)
}

const noticeTime = 7 * time.Second

// stir says whether anything on the screen may have changed, so that it needs painting
// again: a window nobody is touching, with nothing playing, costs nothing.
func (g *Game) stir(x, y, wheel float32) bool {
	now := [4]float32{x, y, g.w, g.h}
	moved := now != g.seen
	g.seen = now
	switch {
	case moved, wheel != 0, len(g.keys) > 0, g.in.pressed, g.in.released, g.in.down:
		return true
	case g.shot != "", g.job != nil, g.choose != nil, g.latest.busy():
		return true
	case g.notice != "" && time.Since(g.noticed) <= noticeTime+time.Second:
		return true
	case g.song != nil && (g.song.player.Playing() || g.song.player.Counting()):
		return true
	}
	return false
}

// Layout makes the screen as large as the window in real pixels, so that a Retina display
// draws sharp; everything is then laid out in the units of the window.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.scale = float32(ebiten.Monitor().DeviceScaleFactor())
	g.w, g.h = float32(outsideWidth), float32(outsideHeight)
	return int(math.Ceil(float64(g.w * g.scale))), int(math.Ceil(float64(g.h * g.scale)))
}

func (g *Game) mouse() (float32, float32) {
	x, y := ebiten.CursorPosition()
	return float32(x) / g.scale, float32(y) / g.scale
}

// Update takes in what the player did and acts on it.
func (g *Game) Update() error {
	defer crashed("update")
	x, y := g.mouse()
	_, wheel := ebiten.Wheel()
	g.in = pointer{
		x: x, y: y, wheel: float32(wheel),
		pressed:  inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft),
		released: inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft),
		down:     ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),
	}
	g.keys = inpututil.AppendJustPressedKeys(g.keys[:0])
	g.frames++
	if g.stir(x, y, float32(wheel)) || ebiten.DroppedFiles() != nil {
		g.awake = 20
	} else if g.frames%60 == 0 && g.awake == 0 {
		g.awake = 1 // once a second anyway, in case the system threw the picture away
	}

	g.takeDrop()
	g.takeChoice()
	g.takeJob()
	g.takeUpdate()

	c := &canvas{in: &g.in, mx: x, my: y, scale: g.scale, w: g.w, h: g.h}
	g.frame(c)
	if g.in.released {
		g.active = ""
	}
	return nil
}

// Draw paints the window.
func (g *Game) Draw(target *ebiten.Image) {
	defer crashed("draw")
	if g.awake <= 0 {
		return
	}
	g.awake--
	x, y := g.mouse()
	c := &canvas{dst: target, mx: x, my: y, scale: g.scale, w: g.w, h: g.h}
	c.fill(rect{0, 0, g.w, g.h}, colBack)
	g.frame(c)
	g.drawNotice(c)
	if g.shot != "" {
		g.takeShot(target)
	}
}

// frame lays out the current screen: to act on it when c takes the mouse, to paint it otherwise.
func (g *Game) frame(c *canvas) {
	if g.guide != "" {
		g.guideScreen(c)
		return
	}
	if c.in != nil && g.pressed(ebiten.KeyF1) {
		g.guide, g.guideScroll = "help", 0
		return
	}
	switch g.screen {
	case home:
		g.homeScreen(c)
	case working:
		g.workingScreen(c)
	case playing:
		g.playScreen(c)
	}
}

func (g *Game) pressed(key ebiten.Key) bool {
	for _, k := range g.keys {
		if k == key {
			return true
		}
	}
	return false
}

func (g *Game) drawNotice(c *canvas) {
	if g.stays && c.in != nil && c.in.pressed {
		g.stays, g.notice = false, ""
	}
	if !g.showing() {
		return
	}
	edge := colDanger
	if g.good {
		edge = colAccent2
	}
	width := min(c.width(g.notice, 14, regular)+40, g.w-40)
	box := rect{(g.w - width) / 2, g.h - 62, width, 40}
	c.round(box, 10, colRaised)
	c.outline(box, 10, 1, edge)
	c.label(c.fit(g.notice, 14, regular, width-30), g.w/2, box.y+20, 14, regular, colText, centre)
}

// takeShot saves a picture of the window once it has settled, for checking it without a
// person in front of it.
func (g *Game) takeShot(target *ebiten.Image) {
	if g.screen == working && g.job != nil && !g.job.failed() && os.Getenv("CBASS_SHOT_DO") != "working" {
		return
	}
	g.shotFrames++
	if g.shotFrames == 2 && g.song != nil {
		g.song.player.Seek(g.shotAt)
		for _, action := range strings.Split(os.Getenv("CBASS_SHOT_DO"), ",") {
			switch action {
			case "loop":
				g.song.loopFrom(g.song.bar(g.shotAt))
				g.song.loopTo(g.song.bar(g.shotAt) + 1)
			case "slow":
				g.song.player.SetSpeed(0.7)
			case "five":
				g.song.retune("5")
			case "six":
				g.song.retune("6")
			case "play":
				g.song.player.SetPlaying(true)
			case "note":
				if i := g.song.coming(g.shotAt); i >= 0 {
					g.song.chosen = g.song.project.Events[i].ID
				}
			case "chord":
				for i, chord := range g.song.project.Chords {
					if g.shotAt >= chord.Start && g.shotAt < chord.End {
						g.song.chord = i
					}
				}
			case "odd":
				g.song.setBeats(g.song.bar(g.shotAt)+1, 2)
			case "menu":
				g.menu = true
			case "key":
				g.song.moveKey(2)
			case "sections":
				g.song.sectionAdd(0)
				g.song.sectionKind(g.song.sectionAdd(g.shotAt))
				g.song.section = g.song.sectionAt(g.shotAt)
				g.song.sectionAdd(g.shotAt + 12)
			case "export":
				for _, kind := range []string{"text", "musicxml", "midi", "pdf"} {
					g.export(g.song, kind)
				}
			case "low":
				g.song.project.Low = true
				g.song.retune(g.song.project.Tuning)
			case "helps":
				g.settings.CountIn, g.settings.Metronome, g.settings.Quicken = true, true, true
			}
		}
	}
	if strings.Contains(os.Getenv("CBASS_SHOT_DO"), "english") {
		g.settings.English = true
	}
	if g.shotFrames < 20 {
		return
	}
	file, err := os.Create(g.shot)
	if err == nil {
		err = png.Encode(file, target)
		file.Close()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cbass:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// ---- opening a recording ----

// pending is a recording waiting for its turn: its name, and how to read it when the turn comes.
type pending struct {
	name string
	read func() ([]byte, error)
	link string // a recording to fetch from the web, instead
}

// batch counts what became of several recordings given at once.
type batch struct {
	total, ready, known int
	failed              []string // the names of the ones that did not work
}

// recordingsIn lists the audio files of a file system, folders within folders included, in
// the order of their names.
func recordingsIn(files fs.FS) []pending {
	var found []pending
	fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // a folder that cannot be read is passed over
		}
		if entry.IsDir() {
			if path != "." && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if audio.Readable(entry.Name()) && !strings.HasPrefix(entry.Name(), ".") {
			found = append(found, pending{name: entry.Name(), read: func() ([]byte, error) { return fs.ReadFile(files, path) }})
		}
		return nil
	})
	return found
}

// recordingsAt lists the audio files at some paths: the files themselves, and what is in the folders.
func recordingsAt(paths []string) []pending {
	var found []pending
	for _, path := range paths {
		info, err := os.Stat(path)
		switch {
		case err != nil:
			continue
		case info.IsDir():
			found = append(found, recordingsIn(os.DirFS(path))...)
		case audio.Readable(path):
			found = append(found, pending{name: filepath.Base(path), read: func() ([]byte, error) { return os.ReadFile(path) }})
		}
	}
	return found
}

// takeDrop takes the recordings dropped on the window: one, several, or folders of them.
func (g *Game) takeDrop() {
	dropped := ebiten.DroppedFiles()
	if dropped == nil {
		return
	}
	if entries, err := fs.ReadDir(dropped, "."); err != nil || len(entries) == 0 {
		return
	}
	g.enqueue(recordingsIn(dropped))
}

// enqueue starts work on some recordings, one after the other. Given while others are being
// worked out, they wait their turn after those.
func (g *Game) enqueue(found []pending) {
	if len(found) == 0 {
		g.say(g.t("Servono file audio: MP3, WAV, FLAC, M4A.", "An audio file is needed: MP3, WAV, FLAC, M4A."))
		return
	}
	if g.screen == working && g.job != nil && !g.job.failed() {
		if g.lot.total == 0 {
			return // a recording from the list is being opened: that is not a queue to join
		}
		g.queue = append(g.queue, found...)
		g.lot.total += len(found)
		return
	}
	g.queue, g.lot = found, batch{total: len(found)}
	g.next()
}

// several says whether more than one recording was given: then none is opened at the end,
// and the list says what became of each.
func (g *Game) several() bool { return g.lot.total > 1 }

// next starts on the first recording still waiting, or goes back to the list when none is.
func (g *Game) next() {
	for len(g.queue) > 0 {
		item := g.queue[0]
		g.queue = g.queue[1:]
		if item.link != "" {
			g.openLink(item.link)
			return
		}
		data, err := item.read()
		if err != nil {
			note("%q: cannot be read: %v", item.name, err)
			if !g.several() {
				g.say(err.Error())
				return
			}
			g.lot.failed = append(g.lot.failed, item.name)
			continue
		}
		if g.several() && g.lib.Has(library.ID(data)) {
			g.lot.known++ // worked out before: it is in the list already
			continue
		}
		g.open(item.name, data)
		return
	}
	if !g.several() {
		return
	}
	lot := g.lot
	g.goHome()
	g.scroll = 0
	// one or several, in the two languages
	count := func(n int, one, many, single, plural string) string {
		if n == 1 {
			return "1 " + g.t(one, single)
		}
		return fmt.Sprintf("%d %s", n, g.t(many, plural))
	}
	var parts []string
	if lot.ready > 0 {
		parts = append(parts, count(lot.ready, "brano pronto", "brani pronti", "recording ready", "recordings ready"))
	}
	if lot.known > 0 {
		parts = append(parts, count(lot.known, "già nella lista", "già nella lista", "already in the list", "already in the list"))
	}
	if len(lot.failed) > 0 {
		parts = append(parts, count(len(lot.failed), "non riuscito", "non riusciti", "did not work", "did not work")+": "+strings.Join(lot.failed, ", "))
	}
	if message := strings.Join(parts, "  ·  "); len(lot.failed) > 0 {
		g.say(message)
	} else {
		g.tell(message)
	}
	g.stays = true // a long queue is left to itself: what became of it waits to be read
}

func (g *Game) askForFile() { g.ask(false) }

// askForLink asks for the address of an audio file on the web.
func (g *Game) askForLink() {
	if g.choose != nil {
		return
	}
	g.choose = make(chan []pending, 1)
	answer := g.choose
	prompt := g.t("L'indirizzo di un file audio (MP3, WAV, FLAC, M4A):", "The address of an audio file (MP3, WAV, FLAC, M4A):")
	go func() {
		text := strings.TrimSpace(askText(prompt))
		if text == "" {
			answer <- nil
			return
		}
		answer <- []pending{{name: text, link: text}}
	}()
}

func (g *Game) askForFolder() { g.ask(true) }

// ask shows the system's panel for choosing recordings, or a folder of them.
func (g *Game) ask(folder bool) {
	if g.choose != nil {
		return
	}
	g.choose = make(chan []pending, 1)
	answer := g.choose
	if folder {
		prompt := g.t("Scegli una cartella di brani", "Choose a folder of recordings")
		go func() {
			if path := chooseFolder(prompt); path != "" {
				found := recordingsAt([]string{path})
				if found == nil {
					found = []pending{} // a folder was chosen, with nothing to read in it
				}
				answer <- found
				return
			}
			answer <- nil
		}()
		return
	}
	prompt := g.t("Scegli uno o più brani", "Choose one or more recordings")
	go func() { answer <- recordingsAt(chooseFiles(prompt)) }()
}

func (g *Game) takeChoice() {
	if g.choose == nil {
		return
	}
	select {
	case found := <-g.choose:
		g.choose = nil
		if found != nil {
			g.enqueue(found)
		}
	default:
	}
}

func (g *Game) openPath(path string) {
	g.enqueue(recordingsAt([]string{path}))
}

// open starts work on a recording: straight to playing if it is already in the library.
func (g *Game) open(name string, data []byte) {
	j := &job{title: strings.TrimSuffix(name, filepath.Ext(name)), started: time.Now()}
	g.startJob(j)
	go j.run(func() (*song, error) { return g.work(j, name, data) })
}

// openLink fetches the recording a link leads to, then works on it as on a file.
func (g *Game) openLink(link string) {
	title := link
	if parsed, err := url.Parse(link); err == nil && parsed.Path != "" {
		title = strings.TrimSuffix(path.Base(parsed.Path), path.Ext(parsed.Path))
	}
	j := &job{title: title, started: time.Now(), link: true}
	g.startJob(j)
	go j.run(func() (*song, error) {
		j.at(stageFetching, "", 0, 0)
		data, name, err := fetch.Get(link, func(done, total int64) { j.at(stageFetching, "", done, total) })
		if err != nil {
			return nil, err
		}
		j.mu.Lock()
		j.title = strings.TrimSuffix(name, filepath.Ext(name))
		j.mu.Unlock()
		return g.work(j, name, data)
	})
}

// work works a recording out, away from the window, and keeps the result in the library.
func (g *Game) work(j *job, name string, data []byte) (*song, error) {
	title := strings.TrimSuffix(name, filepath.Ext(name))
	tuning, low := g.settings.Tuning, g.settings.Low
	id := library.ID(data)
	if g.lib.Has(id) {
		return g.load(j, id)
	}
	j.at(stageReading, "", 0, 0)
	recording, err := audio.DecodeBytes(data, name)
	if err != nil {
		return nil, err
	}
	mix := project.Prepare(recording)
	data = nil
	result, err := project.Analyse(mix, title, project.Options{Version: g.version, Tuning: tuning, Low: low, Stop: j.stop.Load}, func(step project.Step) {
		switch step.Stage {
		case project.Downloading:
			j.at(stageDownloading, step.Detail, step.Done, step.Total)
		case project.Separating:
			j.at(stageSeparating, "", step.Done, step.Total)
		case project.Separated, project.ReadingNotes:
			j.at(stageNotes, "", step.Done, step.Total)
		case project.NotesRead:
			j.at(stageBeat, "", 0, 0)
		case project.BeatFound:
			j.at(stageChords, "", 0, 0)
		}
	})
	if err != nil {
		return nil, err
	}
	if j.stop.Load() { // given up while the notes were being read: nothing is kept
		return nil, project.ErrStopped
	}
	j.at(stageSaving, "", 0, 0)
	if err := g.lib.Save(id, result); err != nil {
		return nil, err
	}
	return newSong(id, result), nil
}

func (g *Game) openEntry(id string) {
	title := ""
	for _, entry := range g.entries {
		if entry.ID == id {
			title = entry.Title
		}
	}
	j := &job{title: title, started: time.Now()}
	g.startJob(j)
	go j.run(func() (*song, error) { return g.load(j, id) })
}

func (g *Game) load(j *job, id string) (*song, error) {
	j.at(stageLoading, "", 0, 0)
	result, err := g.lib.Load(id)
	if err != nil {
		return nil, err
	}
	// read by an earlier version of the program: read again, from the bass already separated
	changed := false
	if result.Project.Reader < project.Reader {
		j.at(stageNotes, "", 0, 0)
		changed = project.Reread(result)
	}
	if result.Project.Chords == nil {
		j.at(stageChords, "", 0, 0)
		changed = project.Harmonise(result) || changed
	}
	if changed {
		if err := g.lib.SaveProject(id, result.Project); err != nil {
			return nil, err
		}
	}
	return newSong(id, result), nil
}

func (g *Game) startJob(j *job) {
	g.closeSong()
	g.job, g.screen, g.confirm = j, working, ""
}

func (g *Game) closeSong() {
	if g.song != nil {
		g.song.player.Close()
		g.song = nil
	}
}

// takeJob moves on to playing when the work is done.
func (g *Game) takeJob() {
	if g.job == nil || g.screen != working {
		return
	}
	if g.several() && g.job.failed() { // one of several: noted, and on to the next
		g.lot.failed = append(g.lot.failed, g.job.title)
		g.job = nil
		g.next()
		return
	}
	s := g.job.result()
	if s == nil {
		return
	}
	g.job = nil
	if g.several() {
		s.player.Close()
		g.lot.ready++
		g.next()
		return
	}
	g.song = s
	if s.project.Tuning != g.settings.Tuning {
		s.retune(g.settings.Tuning)
		g.lib.SaveProject(s.id, s.project)
	}
	s.player.SetGains(g.settings.Bass, g.settings.Rest)
	s.player.SetMetronome(g.settings.Metronome)
	g.applyQuicken(s)
	g.entries = g.lib.List()
	g.screen, g.menu = playing, false
}

func (g *Game) goHome() {
	g.closeSong()
	if g.job != nil {
		g.job.stop.Store(true) // whatever it was doing, nobody is waiting for it any more
	}
	g.job, g.menu = nil, false
	g.queue, g.lot = nil, batch{}
	g.entries = g.lib.List()
	g.screen = home
}

// ---- the work in progress ----

type stage int

const (
	stageFetching stage = iota
	stageReading
	stageDownloading
	stageSeparating
	stageNotes
	stageBeat
	stageChords
	stageSaving
	stageLoading
)

// job is a recording being worked out, away from the window so that it stays alive.
type job struct {
	mu          sync.Mutex
	title       string
	link        bool // fetched from the web first
	started     time.Time
	stage       stage
	detail      string
	done, total int64
	downloaded  bool
	err         error
	song        *song
	stop        atomic.Bool // set to give the work up
	noted       time.Time
}

func (j *job) at(s stage, detail string, done, total int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if s != j.stage || j.noted.IsZero() {
		note("%q: step %d after %.1f s", j.title, s, time.Since(j.started).Seconds())
		j.noted = time.Now()
	}
	j.stage, j.detail, j.done, j.total = s, detail, done, total
	if s == stageDownloading {
		j.downloaded = true
	}
}

func (j *job) run(work func() (*song, error)) {
	var s *song
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%v", r)
				note("%q: CRASH: %v\n%s", j.title, r, debug.Stack())
			}
		}()
		s, err = work()
	}()
	switch {
	case j.stop.Load():
		note("%q: given up after %.1f s", j.title, time.Since(j.started).Seconds())
	case err != nil:
		note("%q: failed after %.1f s: %v", j.title, time.Since(j.started).Seconds(), err)
	default:
		note("%q: ready after %.1f s", j.title, time.Since(j.started).Seconds())
	}
	if s != nil && j.stop.Load() { // finished after it was given up: nobody will play it
		s.player.Close()
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.song, j.err = s, err
}

func (j *job) result() *song {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.song
}

func (j *job) failed() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.err != nil
}

func (j *job) snapshot() (stage, string, int64, int64, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.stage, j.detail, j.done, j.total, j.downloaded, j.err
}

// ---- controls ----

type look int

const (
	plain look = iota
	primary
	quiet
	danger
	chosen
)

// hit says whether the control named id was clicked: pressed and released on it.
func (g *Game) hit(c *canvas, id string, r rect) bool {
	if c.in == nil {
		return false
	}
	if c.in.pressed && r.has(c.in.x, c.in.y) {
		g.active = id
	}
	return c.in.released && g.active == id && r.has(c.in.x, c.in.y)
}

// button draws a button and says whether it was clicked.
func (g *Game) button(c *canvas, id string, r rect, text string, l look) bool {
	clicked := g.hit(c, id, r)
	if c.painting() {
		hover := r.has(c.mx, c.my)
		back, ink := colRaised, colText
		switch l {
		case primary:
			back, ink = colAccent, colOnLight
		case chosen:
			back, ink = fade(colAccent2, 0.2), colAccent2
		case quiet:
			back, ink = colBack, colDim
		case danger:
			back, ink = colDanger, colOnLight
		}
		if hover && (l == plain || l == quiet) {
			back, ink = colHover, colText
		}
		c.round(r, 8, back)
		if l == chosen {
			c.outline(r, 8, 1, fade(colAccent2, 0.7))
		}
		if g.active == id {
			c.round(r, 8, fade(rgb(0, 0, 0), 0.18))
		}
		c.label(c.fit(text, 14, medium, r.w-12), r.x+r.w/2, r.y+r.h/2, 14, medium, ink, centre)
	}
	return clicked
}

// slider draws a horizontal slider for a value from 0 to 1 and returns where the mouse put it.
func (g *Game) slider(c *canvas, id string, r rect, value float64) (float64, bool) {
	changed := false
	grab := rect{r.x - 8, r.y - 10, r.w + 16, r.h + 20}
	if c.in != nil {
		if c.in.pressed && grab.has(c.in.x, c.in.y) {
			g.active = id
		}
		if g.active == id && (c.in.down || c.in.released) {
			value = math.Max(0, math.Min(1, float64((c.in.x-r.x)/r.w)))
			changed = true
		}
	}
	if c.painting() {
		middle := r.y + r.h/2
		c.round(rect{r.x, middle - 2.5, r.w, 5}, 2.5, colLine)
		c.round(rect{r.x, middle - 2.5, r.w * float32(value), 5}, 2.5, colAccent)
		radius := float32(7)
		if g.active == id || grab.has(c.mx, c.my) {
			radius = 8.5
		}
		c.disc(r.x+r.w*float32(value), middle, radius, colText)
	}
	return value, changed
}

func clock(seconds float64) string {
	total := int(math.Max(0, seconds))
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
