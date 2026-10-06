package app

import (
	"fmt"
	"math"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
)

// pixelsPerBeat is how wide a beat is written.
const pixelsPerBeat = 104

// playScreen is the recording being played: the tablature scrolling under a fixed line, the
// neck with the note to play, and the controls.
func (g *Game) playScreen(c *canvas) {
	s := g.song
	if s == nil {
		return
	}
	now := s.player.Position()
	if c.in != nil {
		g.playKeys(s, now)
		if g.song == nil {
			return
		}
	}
	const pad = 20
	header := rect{0, 0, g.w, 64}
	controls := rect{pad, g.h - 150, g.w - 2*pad, 130}
	middle := rect{pad, header.h, g.w - 2*pad, controls.y - header.h - 14}
	tabHeight := float32(math.Round(float64(middle.h * 0.53)))
	tab := rect{middle.x, middle.y, middle.w, tabHeight}
	neck := rect{middle.x, middle.y + tabHeight + 14, middle.w, middle.h - tabHeight - 14}

	g.playHeader(c, header, s, now)
	if g.song == nil {
		return
	}
	g.drawTab(c, tab, s, now)
	g.drawNeck(c, neck, s, now)
	g.playControls(c, controls, s, now)
}

func (g *Game) playKeys(s *song, now float64) {
	p := s.player
	switch {
	case g.pressed(ebiten.KeySpace):
		p.SetPlaying(!p.Playing())
	case g.pressed(ebiten.KeyEscape):
		g.goHome()
	case g.pressed(ebiten.KeyArrowLeft):
		bar := s.bar(now)
		if now-s.barStart(bar) < 0.35 {
			bar--
		}
		p.Seek(s.barStart(bar))
	case g.pressed(ebiten.KeyArrowRight):
		p.Seek(s.barStart(s.bar(now) + 1))
	case g.pressed(ebiten.KeyHome):
		p.Seek(0)
	case g.pressed(ebiten.KeyArrowDown), g.pressed(ebiten.KeyMinus):
		g.nudgeSpeed(s, -0.05)
	case g.pressed(ebiten.KeyArrowUp), g.pressed(ebiten.KeyEqual):
		g.nudgeSpeed(s, 0.05)
	case g.pressed(ebiten.KeyA):
		s.loopFrom(s.bar(now))
	case g.pressed(ebiten.KeyB):
		s.loopTo(s.bar(now))
	case g.pressed(ebiten.KeyL):
		if s.loopOn {
			s.loopOff()
		} else {
			s.loopFrom(s.bar(now))
		}
	case g.pressed(ebiten.KeyM):
		if g.settings.Bass > 0 {
			g.settings.Bass = 0
		} else {
			g.settings.Bass = 1
		}
		p.SetGains(g.settings.Bass, g.settings.Rest)
		g.saveSettings()
	}
}

func (g *Game) nudgeSpeed(s *song, by float64) {
	speed := math.Round((s.player.Speed()+by)*20) / 20
	s.player.SetSpeed(math.Max(0.4, math.Min(1.2, speed)))
}

func (g *Game) playHeader(c *canvas, r rect, s *song, now float64) {
	if g.button(c, "back", rect{20, 15, 92, 34}, g.t("‹  Brani", "‹  Library"), plain) {
		g.goHome()
		return
	}
	names := ""
	for _, open := range s.tuning.Open {
		names += " " + g.noteName(open)
	}
	tuningText := fmt.Sprintf("%d %s  ·", len(s.tuning.Open), g.t("corde", "strings")) + names
	tuning := rect{r.w - 20 - 230, 15, 230, 34}
	bars := fmt.Sprintf("%s %d", g.t("battuta", "bar"), s.bar(now)+1)
	c.label(bars, tuning.x-18, 32, 14, regular, colDim, right)
	room := tuning.x - 18 - c.width(bars, 14, regular) - 24 - 130
	title := c.fit(s.project.Title, 19, bold, room*0.62)
	c.label(title, 130, 32, 19, bold, colText, left)
	about := fmt.Sprintf("%.0f BPM  ·  %d/4  ·  %d %s", s.pulse.Tempo(), s.perBar, len(s.project.Events), g.t("note", "notes"))
	c.label(c.fit(about, 14, regular, room-c.width(title, 19, bold)-18), 130+c.width(title, 19, bold)+18, 33, 14, regular, colDim, left)
	if g.button(c, "tuning", tuning, tuningText, plain) {
		next := fretboard.Tunings[0]
		for i, t := range fretboard.Tunings {
			if t.Key == s.tuning.Key {
				next = fretboard.Tunings[(i+1)%len(fretboard.Tunings)]
			}
		}
		s.retune(next.Key)
		g.settings.Tuning = next.Key
		g.saveSettings()
		if err := g.lib.SaveProject(s.id, s.project); err != nil {
			g.say(err.Error())
		}
	}
}

// drawTab writes the tablature around the present moment. The page moves, the line where
// to play stays still: a number reaches it when its note sounds.
func (g *Game) drawTab(c *canvas, r rect, s *song, now float64) {
	if !c.painting() {
		return
	}
	c.round(r, 14, colPanel)
	strings := len(s.tuning.Open)
	const gutter = 52
	spacing := max(16, min(42, (r.h-48-66)/float32(strings-1)))
	staffTop := r.y + 48 + ((r.h-48-66)-spacing*float32(strings-1))/2
	staffBottom := staffTop + spacing*float32(strings-1)
	lineOf := func(str int) float32 { return staffBottom - float32(str)*spacing }
	head := r.x + gutter + (r.w-gutter)*0.27
	page := s.page(now)
	xOf := func(beats float64) float32 { return head + float32((beats-page)*pixelsPerBeat) }
	firstBeat := page - float64((head-r.x-gutter)/pixelsPerBeat) - 1
	lastBeat := page + float64((r.x+r.w-head)/pixelsPerBeat) + 1
	in := c.clip(rect{r.x + gutter, r.y, r.w - gutter - 8, r.h})
	barSlots := s.score.BarSlots
	const lead = pixelsPerBeat / 8 // a bar line stands half a sixteenth before the first note of its bar

	// the stretch being repeated
	if s.loopOn {
		x0, x1 := xOf(float64(s.loopA*s.perBar))-lead, xOf(float64((s.loopB+1)*s.perBar))-lead
		in.fill(rect{x0, staffTop - 16, x1 - x0, staffBottom - staffTop + 32}, fade(colAccent, 0.07))
		in.fill(rect{x0, staffTop - 16, 2, staffBottom - staffTop + 32}, fade(colAccent, 0.6))
		in.fill(rect{x1 - 2, staffTop - 16, 2, staffBottom - staffTop + 32}, fade(colAccent, 0.6))
	}
	// the strings
	for str := 0; str < strings; str++ {
		in.fill(rect{r.x + gutter, lineOf(str) - 0.5, r.w - gutter, 1}, colLine)
	}
	// beats and bar lines
	for beat := int(math.Floor(firstBeat)); float64(beat) <= lastBeat; beat++ {
		x := xOf(float64(beat)) - lead
		if ((beat%s.perBar)+s.perBar)%s.perBar != 0 {
			in.fill(rect{x, staffBottom + 4, 1, 5}, colFaint)
			continue
		}
		in.fill(rect{x - 0.75, staffTop, 1.5, staffBottom - staffTop}, colDim)
		if bar := beat / s.perBar; beat >= 0 {
			in.label(fmt.Sprint(bar+1), x+5, staffTop-17, 12, medium, colDim, left)
		}
	}

	events := s.project.Events
	placed := s.score.Placed
	sounding := s.sounding(now)
	firstSlot, lastSlot := int(math.Floor(firstBeat*rhythm.Division))-barSlots, int(math.Ceil(lastBeat*rhythm.Division))
	from := sort.Search(len(placed), func(i int) bool { return placed[i].Slot+placed[i].Slots >= firstSlot })
	for i := from; i < len(placed) && placed[i].Slot <= lastSlot; i++ {
		event := events[placed[i].Index]
		beats := float64(placed[i].Slot) / rhythm.Division
		x := xOf(beats)
		ink := colText
		if event.End <= now {
			ink = colFaint
		}
		if event.String < 0 || event.String >= strings {
			in.label(g.noteName(event.Midi), x, staffTop-17, 12, medium, colDanger, centre)
			continue
		}
		y := lineOf(event.String)
		text := fmt.Sprint(event.Fret)
		width := in.width(text, 17, bold)
		// a held note is written once: a line runs for as long as it lasts
		if end := xOf(beats+float64(placed[i].Slots)/rhythm.Division) - 9; end-(x+width/2+5) > 5 {
			hold := fade(colText, 0.28)
			if placed[i].Index == sounding {
				hold = fade(colAccent, 0.8)
			} else if event.End <= now {
				hold = fade(colFaint, 0.5)
			}
			in.round(rect{x + width/2 + 5, y - 1.5, end - (x + width/2 + 5), 3}, 1.5, hold)
		}
		if placed[i].Index == sounding {
			in.round(rect{x - width/2 - 6, y - 12, width + 12, 24}, 7, colAccent)
			in.label(text, x, y, 17, bold, colOnLight, centre)
		} else {
			in.fill(rect{x - width/2 - 3, y - 9, width + 6, 18}, colPanel)
			in.label(text, x, y, 17, bold, ink, centre)
		}
	}
	g.drawValues(in, s, xOf, staffBottom, firstSlot, lastSlot, now, lineOf)

	// the line where to play
	in.fill(rect{head - 1, staffTop - 22, 2, staffBottom - staffTop + 44}, fade(colAccent, 0.9))
	in.polygon(colAccent, head-6, staffTop-28, head+6, staffTop-28, head, staffTop-18)

	// the names of the strings, which stay put
	for str := 0; str < strings; str++ {
		c.label(g.noteName(s.tuning.Open[str]), r.x+gutter-14, lineOf(str), 13, medium, colDim, right)
	}
	c.label("TAB", r.x+18, r.y+22, 11, bold, colFaint, left)
}

// drawValues writes under the staff how long each note lasts, the way a tablature does:
// a stem for a quarter, a shorter one for a half, flags or beams for eighths and sixteenths,
// a dot for a dotted value, a slur where a note runs on into the next sign.
func (g *Game) drawValues(c *canvas, s *song, xOf func(float64) float32, staffBottom float32, firstSlot, lastSlot int, now float64, lineOf func(int) float32) {
	symbols := s.score.Symbols
	barSlots := s.score.BarSlots
	slotOf := func(symbol rhythm.Symbol) int { return symbol.Bar*barSlots + symbol.Slot }
	top, bottom := staffBottom+16, staffBottom+40
	from := sort.Search(len(symbols), func(i int) bool { return slotOf(symbols[i]) >= firstSlot })
	short := func(symbol rhythm.Symbol) bool { return !symbol.Rest && symbol.Value < rhythm.Division }
	for i := from; i < len(symbols) && slotOf(symbols[i]) <= lastSlot; i++ {
		symbol := symbols[i]
		if symbol.Rest {
			continue
		}
		slot := slotOf(symbol)
		x := xOf(float64(slot) / rhythm.Division)
		ink := colDim
		if s.project.Events[symbol.Index].End <= now {
			ink = colFaint
		}
		if symbol.Tied {
			// the note carries on: a slur from the sign before, and at the start of a bar its fret again
			if i > 0 {
				x0 := xOf(float64(slotOf(symbols[i-1])) / rhythm.Division)
				y := bottom + 7
				c.line(x0+2, y, x0+(x-x0)*0.25, y+4, 1.2, ink)
				c.line(x0+(x-x0)*0.25, y+4, x0+(x-x0)*0.75, y+4, 1.2, ink)
				c.line(x0+(x-x0)*0.75, y+4, x-2, y, 1.2, ink)
			}
			if event := s.project.Events[symbol.Index]; symbol.Slot == 0 && event.String >= 0 && event.String < len(s.tuning.Open) {
				text := fmt.Sprintf("(%d)", event.Fret)
				width := c.width(text, 13, medium)
				y := lineOf(event.String)
				c.fill(rect{x - width/2 - 2, y - 8, width + 4, 16}, colPanel)
				c.label(text, x, y, 13, medium, colFaint, centre)
			}
		}
		switch {
		case symbol.Value >= 16: // a whole note has no stem
			continue
		case symbol.Value >= 8:
			c.fill(rect{x - 0.75, top + 12, 1.5, bottom - top - 12}, ink)
		default:
			c.fill(rect{x - 0.75, top, 1.5, bottom - top}, ink)
		}
		if symbol.Value%3 == 0 {
			c.disc(x+5.5, top+6, 1.8, ink)
		}
		if !short(symbol) {
			continue
		}
		// short notes in the same beat, one straight after the other, are joined by a beam
		joined := func(a, b rhythm.Symbol) bool {
			return short(a) && short(b) && a.Bar == b.Bar && a.Slot/rhythm.Division == b.Slot/rhythm.Division && a.Slot+a.Value == b.Slot
		}
		before := i > 0 && joined(symbols[i-1], symbol)
		after := i+1 < len(symbols) && joined(symbol, symbols[i+1])
		sixteenth := symbol.Value == 1
		switch {
		case after:
			x1 := xOf(float64(slotOf(symbols[i+1])) / rhythm.Division)
			c.fill(rect{x - 0.75, bottom - 3, x1 - x + 1.5, 3}, ink)
			if sixteenth && symbols[i+1].Value == 1 {
				c.fill(rect{x - 0.75, bottom - 9, x1 - x + 1.5, 3}, ink)
			} else if sixteenth && !before {
				c.fill(rect{x, bottom - 9, 8, 3}, ink)
			}
		case before:
			if sixteenth && symbols[i-1].Value != 1 {
				c.fill(rect{x - 8, bottom - 9, 8, 3}, ink)
			}
		default:
			c.line(x, bottom, x+7, bottom-8, 1.8, ink)
			if sixteenth {
				c.line(x, bottom-6, x+7, bottom-14, 1.8, ink)
			}
		}
	}
}

// drawNeck draws the fingerboard with the one note to play: full when it sounds, a ring
// while waiting for the next.
func (g *Game) drawNeck(c *canvas, r rect, s *song, now float64) {
	if !c.painting() {
		return
	}
	c.round(r, 14, colPanel)
	strings := len(s.tuning.Open)
	frets := s.project.Frets
	if frets <= 0 {
		frets = 12
	}
	board := rect{r.x + 96, r.y + 30, r.w - 96 - 28, r.h - 30 - 44}
	// frets get closer together up the neck, as on the instrument, but less so: the high
	// ones must stay wide enough to read
	fretX := func(fret int) float32 {
		even := float64(fret) / float64(frets)
		real := (1 - math.Pow(2, -float64(fret)/12)) / (1 - math.Pow(2, -float64(frets)/12))
		return board.x + board.w*float32(0.45*even+0.55*real)
	}
	margin := board.h / float32(strings) / 2
	lineOf := func(str int) float32 {
		return board.y + board.h - margin - float32(str)*(board.h-2*margin)/float32(strings-1)
	}
	c.round(board, 5, colWood)
	c.fill(rect{board.x, board.y, board.w, board.h * 0.5}, fade(colWoodLit, 0.35))
	for fret := 1; fret <= frets; fret++ {
		middle := (fretX(fret-1) + fretX(fret)) / 2
		switch {
		case fret%12 == 0:
			c.disc(middle, board.y+board.h*0.27, 6, colInlay)
			c.disc(middle, board.y+board.h*0.73, 6, colInlay)
		case fret%12 == 3 || fret%12 == 5 || fret%12 == 7 || fret%12 == 9:
			c.disc(middle, board.y+board.h/2, 6, colInlay)
		}
		ink, w := colFaint, regular
		if m := fret % 12; m == 0 || m == 3 || m == 5 || m == 7 || m == 9 {
			ink, w = colDim, medium
		}
		c.label(fmt.Sprint(fret), middle, board.y+board.h+20, 12, w, ink, centre)
		if fret < frets {
			c.fill(rect{fretX(fret) - 1.25, board.y, 2.5, board.h}, colFret)
		}
	}
	c.round(rect{board.x - 6, board.y - 1, 7, board.h + 2}, 2, colNut)
	for str := 0; str < strings; str++ {
		thickness := 1.3 + 2.2*float32(strings-1-str)/float32(strings-1)
		y := lineOf(str)
		c.fill(rect{board.x - 6, y - thickness/2, board.w + 6, thickness}, colString)
		c.fill(rect{board.x - 6, y + thickness/2, board.w + 6, 1}, fade(rgb(0, 0, 0), 0.35))
		c.label(g.noteName(s.tuning.Open[str]), r.x+30, y, 13, medium, colDim, centre)
	}
	place := func(str, fret int) (float32, float32) {
		if fret == 0 {
			return board.x - 34, lineOf(str)
		}
		return (fretX(fret-1) + fretX(fret)) / 2, lineOf(str)
	}
	radius := min(17, margin*0.92)
	if i := s.sounding(now); i >= 0 {
		event := s.project.Events[i]
		if event.String >= 0 && event.String < strings && event.Fret <= frets {
			x, y := place(event.String, event.Fret)
			c.disc(x, y, radius+7, fade(colAccent, 0.22))
			c.disc(x, y, radius, colAccent)
			c.label(g.noteName(event.Midi), x, y, radius*0.8, bold, colOnLight, centre)
		}
		g.neckCaption(c, r, s, i, true)
	} else if i := s.coming(now); i >= 0 {
		event := s.project.Events[i]
		if event.String >= 0 && event.String < strings && event.Fret <= frets {
			x, y := place(event.String, event.Fret)
			c.disc(x, y, radius, fade(colPanel, 0.75))
			c.ring(x, y, radius, 2, colAccent)
			c.label(g.noteName(event.Midi), x, y, radius*0.8, bold, colAccent, centre)
		}
		g.neckCaption(c, r, s, i, false)
	}
	c.label(g.t("MANICO", "NECK"), r.x+18, r.y+16, 11, bold, colFaint, left)
}

func (g *Game) neckCaption(c *canvas, r rect, s *song, index int, sounding bool) {
	event := s.project.Events[index]
	text := g.noteName(event.Midi)
	if event.String >= 0 && event.String < len(s.tuning.Open) {
		where := fmt.Sprintf(g.t("corda %s, tasto %d", "%s string, fret %d"), g.noteName(s.tuning.Open[event.String]), event.Fret)
		if event.Fret == 0 {
			where = fmt.Sprintf(g.t("corda %s a vuoto", "open %s string"), g.noteName(s.tuning.Open[event.String]))
		}
		text += "  ·  " + where
	} else {
		text += "  ·  " + g.t("fuori dal manico con questa accordatura", "off the neck in this tuning")
	}
	ink := colAccent
	if !sounding {
		text = g.t("poi  ", "next  ") + text
		ink = colDim
	}
	c.label(text, r.x+r.w-28, r.y+16, 13, medium, ink, right)
}

// playControls is the row at the bottom: where in the recording, play, speed, repeat, volumes.
func (g *Game) playControls(c *canvas, r rect, s *song, now float64) {
	p := s.player
	duration := p.Duration()
	c.round(r, 14, colPanel)

	// where in the recording
	track := rect{r.x + 70, r.y + 18, r.w - 140, 16}
	c.label(clock(now), r.x+22, track.y+8, 13, medium, colText, left)
	c.label(clock(duration), r.x+r.w-22, track.y+8, 13, regular, colDim, right)
	if s.loopOn && c.painting() && duration > 0 {
		a, b := float32(s.barStart(s.loopA)/duration), float32(s.barStart(s.loopB+1)/duration)
		c.round(rect{track.x + track.w*a, track.y - 2, max(3, track.w*(b-a)), 20}, 4, fade(colAccent, 0.2))
	}
	fraction := 0.0
	if duration > 0 {
		fraction = now / duration
	}
	if to, moved := g.slider(c, "where", track, fraction); moved {
		p.Seek(to * duration)
	}

	y := r.y + 62
	// play
	play := rect{r.x + 22, y, 52, 52}
	if g.hit(c, "play", play) {
		p.SetPlaying(!p.Playing())
	}
	if c.painting() {
		cx, cy := play.x+26, play.y+26
		back := colAccent
		if play.has(c.mx, c.my) {
			back = rgb(0xff, 0xbd, 0x55)
		}
		c.disc(cx, cy, 26, back)
		if p.Playing() {
			c.round(rect{cx - 9, cy - 10, 6, 20}, 1.5, colOnLight)
			c.round(rect{cx + 3, cy - 10, 6, 20}, 1.5, colOnLight)
		} else {
			c.polygon(colOnLight, cx-7, cy-11, cx-7, cy+11, cx+12, cy)
		}
	}
	if g.button(c, "start", rect{play.x + 64, y + 9, 40, 34}, "|‹", plain) {
		p.Seek(0)
	}
	if g.button(c, "bar-back", rect{play.x + 110, y + 9, 40, 34}, "‹", plain) {
		bar := s.bar(now)
		if now-s.barStart(bar) < 0.35 {
			bar--
		}
		p.Seek(s.barStart(bar))
	}
	if g.button(c, "bar-on", rect{play.x + 156, y + 9, 40, 34}, "›", plain) {
		p.Seek(s.barStart(s.bar(now) + 1))
	}

	// the three groups share what room is left
	x := play.x + 196 + 34
	room := r.x + r.w - 22 - x
	speedWidth, loopWidth := float32(150), float32(210)
	gap := max(18, min(44, (room-speedWidth-loopWidth-220)/2))
	volumeWidth := room - speedWidth - loopWidth - 2*gap

	// speed
	c.label(g.t("VELOCITÀ", "SPEED"), x, y-4, 11, bold, colFaint, left)
	if g.button(c, "slower", rect{x, y + 9, 36, 34}, "−", plain) {
		g.nudgeSpeed(s, -0.05)
	}
	speedLook := colText
	if math.Abs(p.Speed()-1) > 0.001 {
		speedLook = colAccent
	}
	c.label(fmt.Sprintf("%.0f%%", p.Speed()*100), x+75, y+26, 18, bold, speedLook, centre)
	if g.hit(c, "full-speed", rect{x + 40, y + 9, 70, 34}) {
		p.SetSpeed(1)
	}
	if g.button(c, "faster", rect{x + 114, y + 9, 36, 34}, "+", plain) {
		g.nudgeSpeed(s, 0.05)
	}
	x += speedWidth + gap

	// repeat
	c.label(g.t("RIPETI", "REPEAT"), x, y-4, 11, bold, colFaint, left)
	bar := s.bar(now)
	fromLook, toLook := plain, plain
	if s.loopOn {
		fromLook, toLook = chosen, chosen
	}
	fromText, toText := g.t("da qui", "from here"), g.t("a qui", "to here")
	if s.loopOn {
		fromText = fmt.Sprintf("%s %d", g.t("da", "from"), s.loopA+1)
		toText = fmt.Sprintf("%s %d", g.t("a", "to"), s.loopB+1)
	}
	if g.button(c, "loop-from", rect{x, y + 9, 82, 34}, fromText, fromLook) {
		s.loopFrom(bar)
	}
	if g.button(c, "loop-to", rect{x + 88, y + 9, 76, 34}, toText, toLook) {
		s.loopTo(bar)
	}
	if s.loopOn {
		if g.button(c, "loop-off", rect{x + 170, y + 9, 36, 34}, "×", plain) {
			s.loopOff()
		}
	}
	x += loopWidth + gap

	// volumes
	each := (volumeWidth - 24) / 2
	bassLabel := g.t("BASSO", "BASS")
	if g.settings.Bass == 0 {
		bassLabel += g.t("  ·  muto", "  ·  muted")
	}
	c.label(bassLabel, x, y-4, 11, bold, colFaint, left)
	if to, moved := g.slider(c, "bass", rect{x, y + 18, each, 16}, g.settings.Bass/1.5); moved {
		g.settings.Bass = math.Round(to*1.5*20) / 20
		p.SetGains(g.settings.Bass, g.settings.Rest)
		if c.in.released {
			g.saveSettings()
		}
	}
	if c.painting() { // a notch at the level of the recording
		c.fill(rect{x + each/1.5 - 0.5, y + 37, 1, 5}, colFaint)
	}
	x += each + 24
	c.label(g.t("IL RESTO", "THE REST"), x, y-4, 11, bold, colFaint, left)
	if to, moved := g.slider(c, "rest", rect{x, y + 18, each, 16}, g.settings.Rest); moved {
		g.settings.Rest = math.Round(to*20) / 20
		p.SetGains(g.settings.Bass, g.settings.Rest)
		if c.in.released {
			g.saveSettings()
		}
	}
	if p.Silent && c.painting() {
		c.label(g.t("Nessuna uscita audio: il brano scorre in silenzio.", "No sound output: the recording runs in silence."), r.x+r.w-22, r.y+r.h-14, 12, regular, colDanger, right)
	}
}
