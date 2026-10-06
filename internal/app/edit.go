package app

import (
	"fmt"
	"github.com/MassimoDanieli/c_bass/internal/project"
	"math"
	"sort"

	"github.com/MassimoDanieli/c_bass/internal/chords"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// No reader gets every note right, and what it gets wrong has to be put right by hand: a
// note's pitch, string, length and place; a chord's root and kind; a bar's length. Every
// change can be taken back.

// state is what a change can alter, kept so that the change can be undone.
type state struct {
	events      []transcribe.Event
	chords      []chords.Chord
	odd         []rhythm.OddBar
	downbeat    int
	beats       []float64
	sensitivity float64
	transpose   int
	key         string
	sections    []project.Section
}

// remember keeps the part as it is now, to come back to.
func (s *song) remember() {
	if len(s.history) >= 100 {
		s.history = s.history[1:]
	}
	s.history = append(s.history, state{
		append([]transcribe.Event(nil), s.project.Events...),
		append([]chords.Chord(nil), s.project.Chords...),
		append([]rhythm.OddBar(nil), s.pulse.Odd...),
		s.pulse.Downbeat,
		s.pulse.Beats, // never changed in place: a new pulse is made instead
		s.project.Sensitivity,
		s.project.Transpose,
		s.project.Key,
		append([]project.Section(nil), s.project.Sections...),
	})
}

// undo takes the last change back; it reports whether there was one.
func (s *song) undo() bool {
	if len(s.history) == 0 {
		return false
	}
	last := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	s.project.Events, s.project.Chords = last.events, last.chords
	s.pulse.Odd, s.pulse.Downbeat, s.pulse.Beats = last.odd, last.downbeat, last.beats
	s.project.Sensitivity = last.sensitivity
	s.project.Sections = last.sections
	if s.section >= len(s.project.Sections) {
		s.section = -1
	}
	if s.project.Transpose != last.transpose {
		s.project.Transpose, s.project.Key = last.transpose, last.key
		s.player.SetPitch(last.transpose)
	}
	s.changed()
	return true
}

// changed puts the part back in order after a change and lays it out again.
func (s *song) changed() {
	events := s.project.Events
	sort.SliceStable(events, func(a, b int) bool { return events[a].Start < events[b].Start })
	for i := range events {
		if i+1 < len(events) && events[i].End > events[i+1].Start {
			events[i].End = events[i+1].Start // a note stops where the next one starts
		}
	}
	sort.SliceStable(s.project.Chords, func(a, b int) bool { return s.project.Chords[a].Start < s.project.Chords[b].Start })
	s.project.Rhythm = s.pulse
	s.write()
	s.tellBeats()
	if s.loopOn {
		s.applyLoop()
	}
}

// find is the place of a note in the part, by its name; -1 if it is gone.
func (s *song) find(id string) int {
	for i, event := range s.project.Events {
		if event.ID == id {
			return i
		}
	}
	return -1
}

// slotOf is the sixteenth a note is written on, and how many it lasts.
func (s *song) slotOf(i int) (int, int) {
	placed := s.score.Placed[i]
	return placed.Slot, placed.Slots
}

// when is the moment of a sixteenth, counted from the first bar line.
func (s *song) when(slot int) float64 { return s.moment(float64(slot) / rhythm.Division) }

func (s *song) frets() int {
	if s.project.Frets > 0 {
		return s.project.Frets
	}
	return 12
}

// place finds where to play a pitch: on the string wanted if it is there, otherwise on the
// string where it falls nearest to the fret it was at.
func (s *song) place(midi, wanted, near int) (int, int) {
	open := s.tuning.Open
	if wanted >= 0 && wanted < len(open) {
		if fret := midi - open[wanted]; fret >= 0 && fret <= s.frets() {
			return wanted, fret
		}
	}
	best, bestFret := -1, -1
	for str, base := range open {
		fret := midi - base
		if fret < 0 || fret > s.frets() {
			continue
		}
		if best < 0 || math.Abs(float64(fret-near)) < math.Abs(float64(bestFret-near)) {
			best, bestFret = str, fret
		}
	}
	return best, bestFret
}

// transpose moves a note up or down by semitones, keeping it on its string where it can.
func (s *song) transpose(i, semitones int) {
	event := &s.project.Events[i]
	midi := event.Midi + semitones
	if midi < s.tuning.Open[0] || midi > s.tuning.Open[len(s.tuning.Open)-1]+s.frets() {
		return // off the instrument
	}
	s.remember()
	event.Midi, event.Edited = midi, true
	event.String, event.Fret = s.place(midi, event.String, event.Fret)
	event.Locked = event.String >= 0
	s.changed()
}

// restring moves a note to the next string up or down that has it.
func (s *song) restring(i, direction int) {
	event := &s.project.Events[i]
	for str := event.String + direction; str >= 0 && str < len(s.tuning.Open); str += direction {
		if fret := event.Midi - s.tuning.Open[str]; fret >= 0 && fret <= s.frets() {
			s.remember()
			event.String, event.Fret, event.Locked, event.Edited = str, fret, true, true
			s.changed()
			return
		}
	}
}

// resize makes a note longer or shorter by sixteenths, as it is written. It cannot run into
// the next note; and since a gap of a single sixteenth before the next note is not written as
// a rest (it is how a note is let go), shortening goes on until the note reads shorter.
func (s *song) resize(i, sixteenths int) {
	id := s.project.Events[i].ID
	slot, slots := s.slotOf(i)
	remembered := false
	for step := 1; step <= 3; step++ {
		length := max(1, slots+sixteenths*step)
		end := s.when(slot + length)
		if i+1 < len(s.project.Events) {
			end = math.Min(end, s.project.Events[i+1].Start)
		}
		event := &s.project.Events[i]
		if end <= event.Start+0.02 || math.Abs(end-event.End) < 1e-6 {
			return
		}
		if !remembered {
			s.remember()
			remembered = true
		}
		event.End, event.Edited = end, true
		s.changed()
		i = s.find(id)
		if _, now := s.slotOf(i); now != slots {
			return
		}
	}
}

// move puts a note earlier or later by sixteenths, between the notes on either side of it.
func (s *song) move(i, sixteenths int) {
	events := s.project.Events
	slot, slots := s.slotOf(i)
	target := slot + sixteenths
	if i > 0 {
		if previous, _ := s.slotOf(i - 1); target <= previous {
			return
		}
	}
	if i+1 < len(events) {
		if next, _ := s.slotOf(i + 1); target >= next {
			return
		}
	}
	start := s.when(target)
	if start < 0 || start >= s.player.Duration() {
		return
	}
	s.remember()
	event := &s.project.Events[i]
	event.Start, event.End, event.Edited = start, s.when(target+slots), true
	if i > 0 && events[i-1].End > start {
		events[i-1].End = start
	}
	s.changed()
}

// remove takes a note out.
func (s *song) remove(i int) {
	s.remember()
	s.project.Events = append(s.project.Events[:i:i], s.project.Events[i+1:]...)
	s.changed()
}

// add writes a new note at a moment, on the sixteenth it falls on: an eighth long, or as
// long as there is room for, at the pitch of the note before it. It returns the note's name,
// or "" if there is a note on that sixteenth already.
func (s *song) add(seconds float64) string {
	slot := int(math.Round(s.page(seconds) * rhythm.Division))
	start := s.when(slot)
	events := s.project.Events
	before := sort.Search(len(events), func(i int) bool { return events[i].Start > start+0.01 }) - 1
	if before >= 0 {
		if taken, _ := s.slotOf(before); taken == slot {
			return ""
		}
	}
	end := s.when(slot + 2)
	if before+1 < len(events) {
		end = math.Min(end, events[before+1].Start)
	}
	if end-start < 0.03 {
		return ""
	}
	s.remember()
	note := transcribe.Event{Start: start, End: end, Midi: s.tuning.Open[0] + 5, Confidence: 1, Edited: true, String: -1}
	if before >= 0 {
		note.Midi, note.String, note.Fret = events[before].Midi, events[before].String, events[before].Fret
	}
	note.String, note.Fret = s.place(note.Midi, note.String, note.Fret)
	note.Locked = note.String >= 0
	s.made++
	note.ID = fmt.Sprintf("h-%d-%d", s.made, int(start*1000))
	note.RawMidi = note.Midi
	s.project.Events = append(events, note)
	s.changed()
	return note.ID
}

// --- chords ---

// chordRoot moves the root of a chord by semitones.
func (s *song) chordRoot(i, semitones int) {
	s.remember()
	chord := &s.project.Chords[i]
	chord.Root, chord.Edited = ((chord.Root+semitones)%12+12)%12, true
}

// chordKind gives a chord the next kind: major, minor, seventh, minor seventh, major seventh.
func (s *song) chordKind(i int) {
	s.remember()
	chord := &s.project.Chords[i]
	for k, quality := range chords.Qualities {
		if quality == chord.Quality {
			chord.Quality, chord.Edited = chords.Qualities[(k+1)%len(chords.Qualities)], true
			return
		}
	}
	chord.Quality, chord.Edited = "", true
}

// chordRemove takes a chord out; the one before it, if it reached it, lasts on in its place.
func (s *song) chordRemove(i int) {
	s.remember()
	list := s.project.Chords
	if i > 0 && math.Abs(list[i-1].End-list[i].Start) < 0.02 {
		list[i-1].End = list[i].End
	}
	s.project.Chords = append(list[:i:i], list[i+1:]...)
}

// chordAdd starts a new chord on the beat of a moment, splitting the chord that is there or
// filling the gap up to the next. It returns its place in the list, or -1 if a chord starts
// on that beat already.
func (s *song) chordAdd(seconds float64) int {
	beat := math.Floor(s.page(seconds) + 1e-6)
	start := s.moment(beat)
	list := s.project.Chords
	for _, chord := range list {
		if math.Abs(chord.Start-start) < 0.02 {
			return -1
		}
	}
	s.remember()
	bar := s.pulse.BarAt(beat)
	chord := chords.Chord{Start: start, End: s.barStart(bar + 1), Edited: true}
	for i := range list {
		switch {
		case list[i].Start < start && list[i].End > start: // inside a chord: it splits
			chord.Root, chord.Quality, chord.End = list[i].Root, list[i].Quality, list[i].End
			list[i].End = start
		case list[i].Start > start && list[i].Start < chord.End:
			chord.End = list[i].Start
		}
	}
	s.project.Chords = append(list, chord)
	sort.SliceStable(s.project.Chords, func(a, b int) bool { return s.project.Chords[a].Start < s.project.Chords[b].Start })
	for i, c := range s.project.Chords {
		if c.Start == start {
			return i
		}
	}
	return -1
}

// --- bars ---

// setBeats gives a bar its own number of beats: the way to say that a bar of two sits in a
// piece in four, after which every bar line falls right again.
func (s *song) setBeats(bar, beats int) {
	if bar < 0 || beats < 1 || beats > 12 || beats == s.pulse.BeatsIn(bar) {
		return
	}
	s.remember()
	s.pulse.SetBeatsIn(bar, beats)
	s.changed()
}

// retempo counts the piece twice as fast or half as fast. Which of the two a piece is in is
// often a matter of opinion (is it 87, or 174?), and the program can settle on the one the
// player would not: this turns one into the other. The notes stay where they are; the bars
// and the note values are written again.
func (s *song) retempo(double bool) {
	if !double && len(s.pulse.Beats) < 16 {
		return
	}
	if double && s.pulse.Tempo() > 260 {
		return
	}
	s.remember()
	fresh := s.pulse.Rescale(double)
	s.pulse.Beats, s.pulse.Downbeat, s.pulse.Odd = fresh.Beats, fresh.Downbeat, nil
	s.loopOff()
	s.changed()
}

// repeats says which of the reader's settings for notes struck again the part was read with.
func (s *song) repeats() int {
	for i, value := range project.Sensitivities {
		if math.Abs(value-s.project.Sensitivity) < 0.005 {
			return i
		}
	}
	return 0
}

// reread reads the notes again from the bass, more or less ready to take a small rise in
// level for a note struck again. Notes corrected by hand are read over with the rest; like
// every other change, it can be taken back.
func (s *song) reread(setting int) {
	if s.bass == nil || s.backing == nil || setting < 0 || setting >= len(project.Sensitivities) {
		return
	}
	s.remember()
	s.project.Sensitivity = project.Sensitivities[setting]
	if setting == 0 {
		s.project.Sensitivity = 0
	}
	s.project.Events = project.ReadWith(s.bass, s.backing, s.project, s.project.Sensitivity)
	for i := range s.project.Events {
		s.made++
		s.project.Events[i].ID = fmt.Sprintf("r-%d-%d", s.made, i)
	}
	s.chosen = ""
	s.changed()
}

// --- sections ---

// sectionAdd starts a section at the bar a moment falls in, and returns which one it is; -1
// if one starts there already. It is given the name that most often comes next.
func (s *song) sectionAdd(seconds float64) int {
	start := s.barStart(max(0, s.bar(seconds)))
	for _, section := range s.project.Sections {
		if math.Abs(section.Start-start) < 0.05 {
			return -1
		}
	}
	kind := "verse"
	if len(s.project.Sections) == 0 && start < 0.2*s.player.Duration() {
		kind = "intro"
	}
	s.remember()
	list := append(append([]project.Section(nil), s.project.Sections...), project.Section{Start: start, Kind: kind})
	sort.SliceStable(list, func(a, b int) bool { return list[a].Start < list[b].Start })
	s.project.Sections = list
	s.changed()
	for i, section := range list {
		if section.Start == start {
			return i
		}
	}
	return -1
}

// sectionKind gives a section the next name on the list.
func (s *song) sectionKind(i int) {
	if i < 0 || i >= len(s.project.Sections) {
		return
	}
	s.remember()
	list := append([]project.Section(nil), s.project.Sections...)
	next := 0
	for k, kind := range project.SectionKinds {
		if kind == list[i].Kind {
			next = (k + 1) % len(project.SectionKinds)
		}
	}
	list[i].Kind = project.SectionKinds[next]
	s.project.Sections = list
	s.changed()
}

func (s *song) sectionRemove(i int) {
	if i < 0 || i >= len(s.project.Sections) {
		return
	}
	s.remember()
	list := append([]project.Section(nil), s.project.Sections[:i]...)
	s.project.Sections = append(list, s.project.Sections[i+1:]...)
	s.changed()
}

// sectionEnd is where a section stops: where the next one starts, or the recording ends.
func (s *song) sectionEnd(i int) float64 {
	if i+1 < len(s.project.Sections) {
		return s.project.Sections[i+1].Start
	}
	return s.player.Duration()
}

// sectionRepeat repeats a section, from its first bar to its last.
func (s *song) sectionRepeat(i int) {
	if i < 0 || i >= len(s.project.Sections) {
		return
	}
	first := s.bar(s.project.Sections[i].Start + 0.01)
	last := s.bar(s.sectionEnd(i) - 0.05)
	if i+1 < len(s.project.Sections) {
		last = s.bar(s.sectionEnd(i)+0.01) - 1
	}
	s.loopA, s.loopB, s.loopOn = first, max(first, last), true
	s.applyLoop()
	s.player.Seek(s.barStart(first))
}

// sectionAt is the section a moment falls in, or -1 before the first.
func (s *song) sectionAt(seconds float64) int {
	at := -1
	for i, section := range s.project.Sections {
		if section.Start <= seconds+0.01 {
			at = i
		}
	}
	return at
}

// keyRange is how far from the recording's own key a piece can be moved, either way.
const keyRange = 6

// moveKey moves the whole piece a semitone up or down: the recording is played higher or
// lower at the speed it had, and the part, with its chords, is written where it now sounds.
func (s *song) moveKey(semitones int) {
	to := s.project.Transpose + semitones
	if semitones == 0 || to < -keyRange || to > keyRange {
		return
	}
	s.remember()
	s.project.MoveKey(semitones)
	for i := range s.project.Events { // a name for any note that lost its own
		if s.project.Events[i].ID == "" {
			s.made++
			s.project.Events[i].ID = fmt.Sprintf("k-%d-%d", s.made, i)
		}
	}
	s.player.SetPitch(s.project.Transpose)
	s.changed()
}

// offbeat moves every beat half a beat later, for a piece whose beat was followed on the
// "ands". Twice is a whole beat.
func (s *song) offbeat() {
	if len(s.pulse.Beats) < 4 {
		return
	}
	s.remember()
	fresh := s.pulse.Halfway()
	s.pulse.Beats = fresh.Beats
	s.changed()
}

// tellBeats gives the player the beats to click on, the first of each bar marked.
func (s *song) tellBeats() {
	strong := make([]bool, len(s.pulse.Beats))
	for i := range strong {
		page := i - s.pulse.Downbeat
		strong[i] = s.pulse.BarStart(s.pulse.BarAt(float64(page))) == page
	}
	s.player.SetBeats(s.pulse.Beats, strong)
}
