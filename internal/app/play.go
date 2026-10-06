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

// doubt is the confidence under which a note is shown as uncertain: its pitch was hard to
// make out, as with a muted note or a very short one.
const doubt = 0.65

// playScreen is the recording being played: the tablature scrolling under a fixed line, the
// neck with the note to play, and the controls.
func (g *Game) playScreen(c *canvas) {
	s := g.song
	if s == nil {
		return
	}
	now := s.player.Position()
	const pad = 20
	header := rect{0, 0, g.w, 64}
	controls := rect{pad, g.h - 184, g.w - 2*pad, 164}
	middle := rect{pad, header.h, g.w - 2*pad, controls.y - header.h - 14}
	tabHeight := float32(math.Round(float64(middle.h * 0.58)))
	tab := rect{middle.x, middle.y, middle.w, tabHeight}
	neck := rect{middle.x, middle.y + tabHeight + 14, middle.w, middle.h - tabHeight - 14}

	// the menu, when open, is in front of everything: it takes the click first
	export := rect{g.w - 20 - 44 - 10 - 96, 15, 96, 34}
	if c.in != nil && g.menu {
		g.exportMenu(c, export, s)
		if c.in.pressed || c.in.released {
			if c.in.released {
				g.menu = false
			}
			swallowed := *c.in
			swallowed.pressed, swallowed.released, swallowed.down = false, false, false
			c = &canvas{in: &swallowed, mx: c.mx, my: c.my, scale: c.scale, w: c.w, h: c.h}
		}
	}
	if c.in != nil {
		g.playKeys(s, now)
		if g.song == nil {
			return
		}
	}
	g.playHeader(c, header, export, s, now)
	if g.song == nil {
		return
	}
	g.drawTab(c, tab, s, now)
	g.drawNeck(c, neck, s, now)
	g.playControls(c, controls, s, now)
	if c.painting() && g.menu {
		g.exportMenu(c, export, s)
	}
}

// togglePlay starts or stops. Starting counts a bar in first, if that is wanted.
func (g *Game) togglePlay(s *song, now float64) {
	p := s.player
	if p.Playing() {
		p.SetPlaying(false)
		return
	}
	if !g.settings.CountIn || now >= p.Duration()-0.1 {
		p.SetPlaying(true)
		return
	}
	// as many clicks as the bar has beats, as far apart as the beats are here and at this speed
	page := s.page(now)
	apart := (s.moment(page+1) - s.moment(page)) / p.Speed()
	if apart <= 0 {
		apart = 0.5
	}
	p.PlayCounted(s.pulse.BeatsIn(s.bar(now)), apart)
}

// saved writes the part to the library after a change made by hand.
func (g *Game) saved(s *song) {
	if err := g.lib.SaveProject(s.id, s.project); err != nil {
		g.say(err.Error())
	}
}

func (g *Game) playKeys(s *song, now float64) {
	p := s.player
	command := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	if command && g.pressed(ebiten.KeyZ) {
		if s.undo() {
			g.saved(s)
		}
		return
	}
	// with a note chosen, the keys work on it
	if i := s.find(s.chosen); i >= 0 {
		switch {
		case g.pressed(ebiten.KeyEscape):
			s.chosen = ""
		case g.pressed(ebiten.KeyArrowUp) && shift:
			s.restring(i, 1)
		case g.pressed(ebiten.KeyArrowDown) && shift:
			s.restring(i, -1)
		case g.pressed(ebiten.KeyArrowUp):
			s.transpose(i, 1)
		case g.pressed(ebiten.KeyArrowDown):
			s.transpose(i, -1)
		case g.pressed(ebiten.KeyArrowLeft) && shift:
			s.move(i, -1)
		case g.pressed(ebiten.KeyArrowRight) && shift:
			s.move(i, 1)
		case g.pressed(ebiten.KeyArrowLeft) && i > 0:
			s.chosen = s.project.Events[i-1].ID
			p.Seek(s.project.Events[i-1].Start)
			return
		case g.pressed(ebiten.KeyArrowRight) && i+1 < len(s.project.Events):
			s.chosen = s.project.Events[i+1].ID
			p.Seek(s.project.Events[i+1].Start)
			return
		case g.pressed(ebiten.KeyMinus):
			s.resize(i, -1)
		case g.pressed(ebiten.KeyEqual):
			s.resize(i, 1)
		case g.pressed(ebiten.KeyDelete), g.pressed(ebiten.KeyBackspace):
			s.remove(i)
			s.chosen = ""
		case g.pressed(ebiten.KeySpace):
			g.togglePlay(s, now)
			return
		default:
			return
		}
		g.saved(s)
		return
	}
	switch {
	case g.pressed(ebiten.KeySpace):
		g.togglePlay(s, now)
	case g.pressed(ebiten.KeyEscape):
		if s.chord >= 0 {
			s.chord = -1
		} else {
			g.goHome()
		}
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
	case g.pressed(ebiten.KeyK):
		g.settings.Metronome = !g.settings.Metronome
		p.SetMetronome(g.settings.Metronome)
		g.saveSettings()
	}
}

func (g *Game) nudgeSpeed(s *song, by float64) {
	speed := math.Round((s.player.Speed()+by)*20) / 20
	s.player.SetSpeed(math.Max(0.4, math.Min(1.2, speed)))
}

func (g *Game) playHeader(c *canvas, r, export rect, s *song, now float64) {
	if g.button(c, "back", rect{20, 15, 92, 34}, g.t("‹  Brani", "‹  Library"), plain) {
		g.goHome()
		return
	}
	if g.button(c, "language", rect{r.w - 20 - 44, 15, 44, 34}, g.t("EN", "IT"), quiet) {
		g.switchLanguage()
	}
	if g.button(c, "export", export, g.t("Esporta", "Export"), map[bool]look{true: chosen, false: plain}[g.menu]) {
		g.menu = !g.menu
	}
	names := ""
	for _, open := range s.tuning.Open {
		names += " " + g.noteName(open)
	}
	tuningText := fmt.Sprintf("%d %s  ·", len(s.tuning.Open), g.t("corde", "strings")) + names
	tuning := rect{export.x - 10 - 200, 15, 200, 34}
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
		g.saved(s)
	}
	// where on the neck: near the nut, or wherever the hand moves least
	place := rect{tuning.x - 10 - 132, 15, 132, 34}
	placeText := g.t("tutto il manico", "whole neck")
	if s.project.Low {
		placeText = g.t("primi tasti", "first frets")
	}
	if g.button(c, "place", place, placeText, map[bool]look{true: chosen, false: plain}[s.project.Low]) {
		s.project.Low = !s.project.Low
		g.settings.Low = s.project.Low
		g.saveSettings()
		s.retune(s.tuning.Key)
		g.saved(s)
	}
	bars := fmt.Sprintf("%s %d", g.t("battuta", "bar"), s.bar(now)+1)
	c.label(bars, place.x-18, 32, 14, regular, colDim, right)
	room := place.x - 18 - c.width(bars, 14, regular) - 24 - 130
	title := c.fit(g.titleOf(s.project.Title, s.project.Key), 19, bold, room*0.62)
	c.label(title, 130, 32, 19, bold, colText, left)
	about := fmt.Sprintf("%.0f BPM  ·  %d/4  ·  %d %s", s.pulse.Tempo(), s.perBar, len(s.project.Events), g.t("note", "notes"))
	c.label(c.fit(about, 14, regular, room-c.width(title, 19, bold)-18), 130+c.width(title, 19, bold)+18, 33, 14, regular, colDim, left)
}

// exportMenu is the short list under the Export button.
func (g *Game) exportMenu(c *canvas, under rect, s *song) {
	items := []struct{ kind, text string }{
		{"pdf", g.t("PDF da stampare", "PDF to print")},
		{"musicxml", g.t("MusicXML (Guitar Pro, MuseScore)", "MusicXML (Guitar Pro, MuseScore)")},
		{"text", g.t("Tablatura in testo", "Tablature as text")},
	}
	box := rect{under.x + under.w - 290, under.y + under.h + 6, 290, float32(len(items))*38 + 12}
	c.round(box, 10, colRaised)
	c.outline(box, 10, 1, colLine)
	for i, item := range items {
		row := rect{box.x + 6, box.y + 6 + float32(i)*38, box.w - 12, 36}
		if c.painting() && row.has(c.mx, c.my) {
			c.round(row, 7, colHover)
		}
		c.label(item.text, row.x+12, row.y+18, 14, regular, colText, left)
		if c.in != nil && c.in.released && row.has(c.in.x, c.in.y) {
			g.export(s, item.kind)
		}
	}
}

// tabView is where things are in the tablature panel.
type tabView struct {
	r                       rect
	gutter                  float32
	staffTop, staffBottom   float32
	spacing, head           float32
	page                    float64
	strings                 int
	chordsY, editY, valuesY float32
}

// chordX is where the name of a chord is written: where the chord begins, or at the left
// edge while it lasts and its beginning has gone by, so that the chord being played is
// always named.
func (g *Game) chordX(c *canvas, s *song, v tabView, i int) float32 {
	chord := s.project.Chords[i]
	x := v.xOf(s.page(chord.Start))
	edge := v.r.x + v.gutter + 8
	if x >= edge {
		return x
	}
	last := v.xOf(s.page(chord.End)) - c.width(chord.Name(!g.settings.English), 16, bold) - 18
	return max(x, min(edge, last))
}

func (v tabView) xOf(beats float64) float32 { return v.head + float32((beats-v.page)*pixelsPerBeat) }
func (v tabView) beatsAt(x float32) float64 { return v.page + float64((x-v.head)/pixelsPerBeat) }
func (v tabView) lineOf(str int) float32    { return v.staffBottom - float32(str)*v.spacing }
func (v tabView) inside() rect              { return rect{v.r.x + v.gutter, v.r.y, v.r.w - v.gutter - 8, v.r.h} }

func (g *Game) tabView(r rect, s *song, now float64) tabView {
	v := tabView{r: r, gutter: 52, strings: len(s.tuning.Open), page: s.page(now)}
	const above, values, edit = 82, 44, 50
	room := r.h - above - values - edit
	v.spacing = max(13, min(40, room/float32(v.strings-1)))
	v.staffTop = r.y + above + max(0, (room-v.spacing*float32(v.strings-1))/2)
	v.staffBottom = v.staffTop + v.spacing*float32(v.strings-1)
	v.head = r.x + v.gutter + (r.w-v.gutter)*0.27
	v.chordsY = v.staffTop - 42
	v.editY = r.y + r.h - 44
	return v
}

// lead is how far before the first note of its bar a bar line stands: half a sixteenth.
const lead = pixelsPerBeat / 8

// drawTab writes the tablature around the present moment. The page moves, the line where
// to play stays still: a number reaches it when its note sounds. A note or a chord can be
// chosen with a click, and put right with the buttons under the staff.
func (g *Game) drawTab(c *canvas, r rect, s *song, now float64) {
	v := g.tabView(r, s, now)
	if c.painting() {
		c.round(r, 14, colPanel)
		c.label("TAB", r.x+18, r.y+22, 11, bold, colFaint, left)
	}
	g.barControls(c, v, s, now)
	if len(s.project.Events) == 0 && len(s.project.Chords) == 0 && c.painting() {
		c.label(g.t("In questo brano non c'è un basso.", "There is no bass in this recording."), r.x+r.w/2, r.y+r.h/2-24, 20, medium, colText, centre)
		c.label(g.t("Non c'è niente da trascrivere: resta la base, da suonarci sopra.", "There is nothing to write out: what is left is the backing, to play along to."), r.x+r.w/2, r.y+r.h/2+6, 14, regular, colDim, centre)
	}
	firstBeat := v.beatsAt(r.x+v.gutter) - 1
	lastBeat := v.beatsAt(r.x+r.w) + 1
	events := s.project.Events
	placed := s.score.Placed
	firstSlot, lastSlot := int(math.Floor(firstBeat*rhythm.Division))-s.score.BarSlots, int(math.Ceil(lastBeat*rhythm.Division))
	from := sort.Search(len(placed), func(i int) bool { return placed[i].Slot+placed[i].Slots >= firstSlot })

	// a click on the page: on a chord, on a note, or on an empty place to go there
	if zone := (rect{r.x + v.gutter, v.chordsY - 12, r.w - v.gutter - 8, v.staffBottom + 14 - (v.chordsY - 12)}); c.in != nil && c.in.pressed && zone.has(c.in.x, c.in.y) {
		hit := false
		for i, chord := range s.project.Chords {
			x := g.chordX(c, s, v, i)
			if (rect{x - 6, v.chordsY - 12, c.width(chord.Name(!g.settings.English), 16, bold) + 12, 24}).has(c.in.x, c.in.y) {
				s.chord, s.chosen, hit = i, "", true
			}
		}
		for i := from; !hit && i < len(placed) && placed[i].Slot <= lastSlot; i++ {
			event := events[placed[i].Index]
			x := v.xOf(float64(placed[i].Slot) / rhythm.Division)
			y := v.staffTop - 17
			if event.String >= 0 && event.String < v.strings {
				y = v.lineOf(event.String)
			}
			if (rect{x - 14, y - 13, 28, 26}).has(c.in.x, c.in.y) {
				s.chosen, s.chord, hit = event.ID, -1, true
			}
		}
		if !hit {
			s.chosen, s.chord = "", -1
			if c.in.y >= v.staffTop-14 {
				s.player.Seek(s.moment(v.beatsAt(c.in.x)))
			}
		}
	}
	g.editBar(c, v, s, now)
	if !c.painting() {
		return
	}

	in := c.clip(v.inside())
	staffTop, staffBottom := v.staffTop, v.staffBottom
	// the stretch being repeated
	if s.loopOn {
		x0, x1 := v.xOf(float64(s.pulse.BarStart(s.loopA)))-lead, v.xOf(float64(s.pulse.BarStart(s.loopB+1)))-lead
		in.fill(rect{x0, staffTop - 16, x1 - x0, staffBottom - staffTop + 32}, fade(colAccent2, 0.09))
		in.fill(rect{x0, staffTop - 16, 2, staffBottom - staffTop + 32}, fade(colAccent2, 0.7))
		in.fill(rect{x1 - 2, staffTop - 16, 2, staffBottom - staffTop + 32}, fade(colAccent2, 0.7))
	}
	// the strings
	for str := 0; str < v.strings; str++ {
		in.fill(rect{r.x + v.gutter, v.lineOf(str) - 0.5, r.w - v.gutter, 1}, colLine)
	}
	// beats and bar lines
	for beat := int(math.Floor(firstBeat)); float64(beat) <= lastBeat; beat++ {
		x := v.xOf(float64(beat)) - lead
		bar := s.pulse.BarAt(float64(beat))
		if s.pulse.BarStart(bar) != beat {
			in.fill(rect{x, staffBottom + 4, 1, 5}, colFaint)
			continue
		}
		in.fill(rect{x - 0.75, staffTop, 1.5, staffBottom - staffTop}, colDim)
		if beat >= 0 {
			text := fmt.Sprint(bar + 1)
			ink := colDim
			if beats := s.pulse.BeatsIn(bar); beats != s.perBar { // a bar of its own length says so
				text += fmt.Sprintf("   %d/4", beats)
				ink = colAccent2
			}
			in.label(text, x+5, staffTop-17, 12, medium, ink, left)
		}
	}
	// the chords, above
	for i, chord := range s.project.Chords {
		if v.xOf(s.page(chord.Start)) > r.x+r.w || v.xOf(s.page(chord.End)) < r.x+v.gutter {
			continue
		}
		x := g.chordX(c, s, v, i)
		name := chord.Name(!g.settings.English)
		ink := colText
		switch {
		case now >= chord.Start && now < chord.End:
			ink = colAccent
		case chord.End <= now:
			ink = colFaint
		}
		if i == s.chord {
			box := rect{x - 6, v.chordsY - 12, in.width(name, 16, bold) + 12, 24}
			in.round(box, 7, fade(colAccent2, 0.18))
			in.outline(box, 7, 1.5, colAccent2)
		}
		in.label(name, x, v.chordsY, 16, bold, ink, left)
	}

	sounding := s.sounding(now)
	for i := from; i < len(placed) && placed[i].Slot <= lastSlot; i++ {
		event := events[placed[i].Index]
		beats := float64(placed[i].Slot) / rhythm.Division
		x := v.xOf(beats)
		ink := colText
		if event.End <= now {
			ink = colFaint
		}
		picked := event.ID == s.chosen
		if event.String < 0 || event.String >= v.strings {
			in.label(g.noteName(event.Midi), x, staffTop-17, 12, medium, colDanger, centre)
			if picked {
				in.outline(rect{x - 14, staffTop - 29, 28, 24}, 7, 1.5, colAccent2)
			}
			continue
		}
		y := v.lineOf(event.String)
		text := fmt.Sprint(event.Fret)
		width := in.width(text, 17, bold)
		// a held note is written once: a line runs for as long as it lasts
		if end := v.xOf(beats+float64(placed[i].Slots)/rhythm.Division) - 9; end-(x+width/2+5) > 5 {
			hold := fade(colText, 0.28)
			if placed[i].Index == sounding {
				hold = fade(colAccent, 0.8)
			} else if event.End <= now {
				hold = fade(colFaint, 0.5)
			}
			in.round(rect{x + width/2 + 5, y - 1.5, end - (x + width/2 + 5), 3}, 1.5, hold)
		}
		// a note the reader is not sure of says so
		doubtful := event.Confidence > 0 && event.Confidence < doubt && !event.Edited
		if placed[i].Index == sounding {
			in.round(rect{x - width/2 - 6, y - 12, width + 12, 24}, 7, colAccent)
			in.label(text, x, y, 17, bold, colOnLight, centre)
		} else {
			in.fill(rect{x - width/2 - 3, y - 9, width + 6, 18}, colPanel)
			if doubtful && event.End > now {
				ink = colDim
			}
			in.label(text, x, y, 17, bold, ink, centre)
		}
		if doubtful {
			in.label("?", x+width/2+5, y-9, 11, bold, colDim, centre)
		}
		if picked {
			in.outline(rect{x - width/2 - 8, y - 14, width + 16, 28}, 8, 1.5, colAccent2)
		}
	}
	g.drawValues(in, s, v.xOf, staffBottom, firstSlot, lastSlot, now, v.lineOf)

	// the line where to play
	in.fill(rect{v.head - 1, staffTop - 22, 2, staffBottom - staffTop + 44}, fade(colAccent, 0.9))
	in.polygon(colAccent, v.head-6, staffTop-28, v.head+6, staffTop-28, v.head, staffTop-18)

	// the names of the strings, which stay put
	for str := 0; str < v.strings; str++ {
		c.label(g.noteName(s.tuning.Open[str]), r.x+v.gutter-14, v.lineOf(str), 13, medium, colDim, right)
	}
}

// barControls are at the top right of the tablature: the length of the bar being played,
// and the place of all the bar lines.
func (g *Game) barControls(c *canvas, v tabView, s *song, now float64) {
	r := v.r
	x := r.x + r.w - 14
	y := r.y + 8
	// the bar lines can be moved by a beat, when the "one" was heard in the wrong place
	for i, by := range []int{1, -1} {
		x -= 34
		if g.button(c, []string{"bars-on", "bars-back"}[i], rect{x, y, 30, 26}, []string{"›", "‹"}[i], quiet) {
			s.shiftBars(by)
			g.saved(s)
		}
		x -= 4
	}
	x -= 6
	label := g.t("stanghette", "bar lines")
	c.label(label, x, y+13, 12, regular, colFaint, right)
	x -= c.width(label, 12, regular) + 26
	// and one bar can have its own number of beats
	bar := s.bar(now)
	if bar >= 0 {
		beats := s.pulse.BeatsIn(bar)
		for i, by := range []int{1, -1} {
			x -= 34
			if g.button(c, []string{"beats-more", "beats-fewer"}[i], rect{x, y, 30, 26}, []string{"+", "−"}[i], quiet) {
				s.setBeats(bar, beats+by)
				g.saved(s)
			}
			x -= 4
		}
		x -= 6
		ink := colFaint
		if beats != s.perBar {
			ink = colAccent2
		}
		text := fmt.Sprintf("%s %d: %d/4", g.t("battuta", "bar"), bar+1, beats)
		c.label(text, x, y+13, 12, regular, ink, right)
		x -= c.width(text, 12, regular) + 26
	}
	// and the whole piece can be counted twice as fast, or half
	for i, double := range []bool{true, false} {
		x -= 38
		if g.button(c, []string{"tempo-double", "tempo-half"}[i], rect{x, y, 34, 26}, []string{"×2", "÷2"}[i], quiet) {
			s.retempo(double)
			g.saved(s)
		}
		x -= 4
	}
	x -= 6
	c.label(fmt.Sprintf("%.0f BPM", s.pulse.Tempo()), x, y+13, 12, regular, colFaint, right)
}

// editBar is under the staff: what can be done to the note or the chord chosen, and what
// can always be done: add one, take the last change back.
func (g *Game) editBar(c *canvas, v tabView, s *song, now float64) {
	x := v.r.x + 16
	y := v.editY
	const h = 30
	group := func(label string) {
		c.label(label, x, y+h/2, 12, regular, colFaint, left)
		x += c.width(label, 12, regular) + 8
	}
	press := func(id, text string, width float32, l look) bool {
		clicked := g.button(c, id, rect{x, y, width, h}, text, l)
		x += width + 4
		return clicked
	}
	gap := func() { x += 14 }
	changed := false
	if i := s.find(s.chosen); i >= 0 {
		event := s.project.Events[i]
		group(g.t("Nota", "Note") + "  " + g.noteName(event.Midi))
		if press("note-down", "−", 30, plain) {
			s.transpose(i, -1)
			changed = true
		}
		if press("note-up", "+", 30, plain) {
			s.transpose(s.find(s.chosen), 1)
			changed = true
		}
		gap()
		group(g.t("Corda", "String"))
		if press("string-down", "↓", 30, plain) {
			s.restring(s.find(s.chosen), -1)
			changed = true
		}
		if press("string-up", "↑", 30, plain) {
			s.restring(s.find(s.chosen), 1)
			changed = true
		}
		gap()
		group(g.t("Durata", "Length"))
		if press("shorter", "−", 30, plain) {
			s.resize(s.find(s.chosen), -1)
			changed = true
		}
		if press("longer", "+", 30, plain) {
			s.resize(s.find(s.chosen), 1)
			changed = true
		}
		gap()
		group(g.t("Posto", "Place"))
		if press("earlier", "‹", 30, plain) {
			s.move(s.find(s.chosen), -1)
			changed = true
		}
		if press("later", "›", 30, plain) {
			s.move(s.find(s.chosen), 1)
			changed = true
		}
		gap()
		if press("remove-note", g.t("Elimina", "Remove"), 76, plain) {
			s.remove(s.find(s.chosen))
			s.chosen = ""
			changed = true
		}
	} else if s.chord >= 0 && s.chord < len(s.project.Chords) {
		chord := s.project.Chords[s.chord]
		group(g.t("Accordo", "Chord") + "  " + chord.Name(!g.settings.English))
		if press("root-down", "−", 30, plain) {
			s.chordRoot(s.chord, -1)
			changed = true
		}
		if press("root-up", "+", 30, plain) {
			s.chordRoot(s.chord, 1)
			changed = true
		}
		gap()
		kind := map[string]string{"": g.t("maggiore", "major"), "m": g.t("minore", "minor"), "7": g.t("settima", "seventh"), "m7": g.t("minore settima", "minor seventh"), "maj7": g.t("settima maggiore", "major seventh")}[chord.Quality]
		if press("chord-kind", kind+"  ›", 150, plain) {
			s.chordKind(s.chord)
			changed = true
		}
		gap()
		if press("remove-chord", g.t("Elimina", "Remove"), 76, plain) {
			s.chordRemove(s.chord)
			s.chord = -1
			changed = true
		}
	} else if c.painting() {
		c.label(g.t("Clic su una nota o su un accordo per correggerli.", "Click a note or a chord to put it right."), x, y+h/2, 13, regular, colFaint, left)
	}
	// on the right, what is always there
	x = v.r.x + v.r.w - 16 - 96 - 4 - 104 - 4 - 92
	if press("add-note", g.t("+ Nota", "+ Note"), 92, plain) {
		if id := s.add(now); id != "" {
			s.chosen, s.chord = id, -1
			changed = true
		}
	}
	if press("add-chord", g.t("+ Accordo", "+ Chord"), 104, plain) {
		if i := s.chordAdd(now); i >= 0 {
			s.chord, s.chosen = i, ""
			changed = true
		}
	}
	undo := quiet
	if len(s.history) > 0 {
		undo = plain
	}
	if press("undo", g.t("Annulla", "Undo"), 96, undo) {
		changed = s.undo()
		if s.find(s.chosen) < 0 {
			s.chosen = ""
		}
		if s.chord >= len(s.project.Chords) {
			s.chord = -1
		}
	}
	if changed {
		g.saved(s)
	}
}

// drawValues writes under the staff how long each note lasts, the way a tablature does:
// a stem for a quarter, a shorter one for a half, flags or beams for eighths and sixteenths,
// a dot for a dotted value, a slur where a note runs on into the next sign.
func (g *Game) drawValues(c *canvas, s *song, xOf func(float64) float32, staffBottom float32, firstSlot, lastSlot int, now float64, lineOf func(int) float32) {
	symbols := s.score.Symbols
	slotOf := func(symbol rhythm.Symbol) int { return symbol.At }
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
			c.ring(x, y, radius, 2, colAccent2)
			c.label(g.noteName(event.Midi), x, y, radius*0.8, bold, colAccent2, centre)
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
		ink = colAccent2
	}
	c.label(text, r.x+r.w-28, r.y+16, 13, medium, ink, right)
}

// playControls is the panel at the bottom: where in the recording, play, speed, repeat,
// volumes, and the helps for practising.
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
		c.round(rect{track.x + track.w*a, track.y - 2, max(3, track.w*(b-a)), 20}, 4, fade(colAccent2, 0.25))
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
		g.togglePlay(s, now)
	}
	if c.painting() {
		cx, cy := play.x+26, play.y+26
		back := colAccent
		if play.has(c.mx, c.my) {
			back = rgb(0xff, 0xc4, 0x62)
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
	chips := x

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

	// the helps for practising, each one on or off
	y = r.y + 124
	x = chips
	chip := func(id, text string, on *bool, apply func()) {
		width := c.width(text, 14, medium) + 28
		if g.button(c, id, rect{x, y, width, 30}, text, map[bool]look{true: chosen, false: plain}[*on]) {
			*on = !*on
			apply()
			g.saveSettings()
		}
		x += width + 8
	}
	counting := g.t("Conta una battuta", "Count a bar in")
	if p.Counting() {
		counting = g.t("Conto…", "Counting…")
	}
	chip("count-in", counting, &g.settings.CountIn, func() {})
	chip("metronome", g.t("Metronomo", "Metronome"), &g.settings.Metronome, func() { p.SetMetronome(g.settings.Metronome) })
	chip("quicken", g.t("Più veloce a ogni giro", "Faster every time round"), &g.settings.Quicken, func() { g.applyQuicken(s) })
	if g.settings.Quicken && !s.loopOn && c.painting() {
		c.label(g.t("vale quando ripeti un tratto", "works while a stretch repeats"), x+4, y+15, 12, regular, colFaint, left)
	}
	if p.Silent && c.painting() {
		c.label(g.t("Nessuna uscita audio: il brano scorre in silenzio.", "No sound output: the recording runs in silence."), r.x+r.w-22, r.y+r.h-16, 12, regular, colDanger, right)
	}
}

// applyQuicken tells the player whether a repeated stretch speeds up: five in a hundred
// every time round, up to full speed.
func (g *Game) applyQuicken(s *song) {
	if g.settings.Quicken {
		s.player.SetQuicken(0.05, 1)
	} else {
		s.player.SetQuicken(0, 0)
	}
}
