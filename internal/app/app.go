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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/icon"
	"github.com/MassimoDanieli/c_bass/internal/library"
	"github.com/MassimoDanieli/c_bass/internal/project"
)

type screen int

const (
	home screen = iota
	working
	playing
)

// settings are what the program remembers from one time to the next.
type settings struct {
	English bool    `json:"english"`
	Tuning  string  `json:"tuning"`
	Bass    float64 `json:"bass"` // volume of the bass, 0 to 1.5
	Rest    float64 `json:"rest"` // volume of everything else, 0 to 1
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

	in      pointer
	keys    []ebiten.Key
	active  string // the control the mouse went down on
	confirm string // the recording about to be removed
	scroll  float32
	notice  string
	noticed time.Time
	choose  chan string // the answer of the file dialog

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
	g := &Game{version: version, lib: lib, settings: settings{Tuning: "4", Bass: 1, Rest: 1}, scale: 1}
	g.loadSettings()
	g.entries = lib.List()
	g.shot = os.Getenv("CBASS_SHOT")
	g.shotAt, _ = strconv.ParseFloat(os.Getenv("CBASS_SHOT_AT"), 64)
	if os.Getenv("CBASS_SHOT_DO") == "working" { // a picture of the work in progress, without doing any
		g.job = &job{title: "Giro di prova", started: time.Now(), stage: stageSeparating, done: 12, total: 31, downloaded: true}
		g.screen = working
	} else if open := os.Getenv("CBASS_OPEN"); open != "" {
		if lib.Has(open) {
			g.openEntry(open)
		} else {
			g.openPath(open)
		}
	}
	ebiten.SetWindowTitle("C_bass")
	ebiten.SetWindowSize(1180, 780)
	ebiten.SetWindowSizeLimits(900, 640, -1, -1)
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

// t picks the Italian or the English of a text.
func (g *Game) t(italian, english string) string {
	if g.settings.English {
		return english
	}
	return italian
}

func (g *Game) say(message string) {
	g.notice, g.noticed = message, time.Now()
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
	x, y := g.mouse()
	_, wheel := ebiten.Wheel()
	g.in = pointer{
		x: x, y: y, wheel: float32(wheel),
		pressed:  inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft),
		released: inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft),
		down:     ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),
	}
	g.keys = inpututil.AppendJustPressedKeys(g.keys[:0])

	g.takeDrop()
	g.takeChoice()
	g.takeJob()

	c := &canvas{in: &g.in, mx: x, my: y, scale: g.scale, w: g.w, h: g.h}
	g.frame(c)
	if g.in.released {
		g.active = ""
	}
	return nil
}

// Draw paints the window.
func (g *Game) Draw(target *ebiten.Image) {
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
	if g.notice == "" || time.Since(g.noticed) > 7*time.Second {
		return
	}
	width := min(c.width(g.notice, 14, regular)+40, g.w-40)
	box := rect{(g.w - width) / 2, g.h - 62, width, 40}
	c.round(box, 10, colRaised)
	c.outline(box, 10, 1, colDanger)
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
			}
		}
	}
	if os.Getenv("CBASS_SHOT_DO") == "english" {
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

func playable(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3", ".wav", ".wave":
		return true
	}
	return false
}

// takeDrop opens a recording dropped on the window.
func (g *Game) takeDrop() {
	dropped := ebiten.DroppedFiles()
	if dropped == nil {
		return
	}
	entries, err := fs.ReadDir(dropped, ".")
	if err != nil || len(entries) == 0 {
		return
	}
	if g.screen == working && g.job != nil && !g.job.failed() {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !playable(entry.Name()) {
			continue
		}
		data, err := fs.ReadFile(dropped, entry.Name())
		if err != nil {
			g.say(err.Error())
			return
		}
		g.open(entry.Name(), data)
		return
	}
	g.say(g.t("Servono file MP3 o WAV.", "Only MP3 and WAV files can be opened."))
}

func (g *Game) askForFile() {
	if g.choose != nil {
		return
	}
	g.choose = make(chan string, 1)
	answer := g.choose
	prompt := g.t("Scegli un brano", "Choose a recording")
	go func() { answer <- chooseFile(prompt) }()
}

func (g *Game) takeChoice() {
	if g.choose == nil {
		return
	}
	select {
	case path := <-g.choose:
		g.choose = nil
		if path != "" {
			g.openPath(path)
		}
	default:
	}
}

func (g *Game) openPath(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		g.say(err.Error())
		return
	}
	g.open(filepath.Base(path), data)
}

// open starts work on a recording: straight to playing if it is already in the library.
func (g *Game) open(name string, data []byte) {
	title := strings.TrimSuffix(name, filepath.Ext(name))
	j := &job{title: title, started: time.Now()}
	g.startJob(j)
	tuning := g.settings.Tuning
	go j.run(func() (*song, error) {
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
		result, err := project.Analyse(mix, title, project.Options{Version: g.version, Tuning: tuning}, func(step project.Step) {
			switch step.Stage {
			case project.Downloading:
				j.at(stageDownloading, step.Detail, step.Done, step.Total)
			case project.Separating:
				j.at(stageSeparating, "", step.Done, step.Total)
			case project.Separated, project.ReadingNotes:
				j.at(stageNotes, "", step.Done, step.Total)
			case project.NotesRead:
				j.at(stageBeat, "", 0, 0)
			}
		})
		if err != nil {
			return nil, err
		}
		j.at(stageSaving, "", 0, 0)
		if err := g.lib.Save(id, result); err != nil {
			return nil, err
		}
		return newSong(id, result), nil
	})
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
	s := g.job.result()
	if s == nil {
		return
	}
	g.job = nil
	g.song = s
	if s.project.Tuning != g.settings.Tuning {
		s.retune(g.settings.Tuning)
		g.lib.SaveProject(s.id, s.project)
	}
	s.player.SetGains(g.settings.Bass, g.settings.Rest)
	g.entries = g.lib.List()
	g.screen = playing
}

func (g *Game) goHome() {
	g.closeSong()
	g.job = nil
	g.entries = g.lib.List()
	g.screen = home
}

// ---- the work in progress ----

type stage int

const (
	stageReading stage = iota
	stageDownloading
	stageSeparating
	stageNotes
	stageBeat
	stageSaving
	stageLoading
)

// job is a recording being worked out, away from the window so that it stays alive.
type job struct {
	mu          sync.Mutex
	title       string
	started     time.Time
	stage       stage
	detail      string
	done, total int64
	downloaded  bool
	err         error
	song        *song
}

func (j *job) at(s stage, detail string, done, total int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
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
			}
		}()
		s, err = work()
	}()
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
			back, ink = fade(colAccent, 0.22), colAccent
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
			c.outline(r, 8, 1, fade(colAccent, 0.7))
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
