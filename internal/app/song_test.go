package app

import (
	"github.com/MassimoDanieli/c_bass/internal/chords"
	"math"
	"os"
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// A made-up recording at 120 BPM: a quarter note on every beat of the first two bars, then
// one note held for a whole bar.
func testSong(t *testing.T) *song {
	t.Helper()
	os.Setenv("CBASS_NO_AUDIO", "1")
	frames := 12 * 44100
	track := func() *audio.Buffer {
		return &audio.Buffer{SampleRate: 44100, Channels: [][]float32{make([]float32, frames), make([]float32, frames)}}
	}
	var events []transcribe.Event
	for beat := 0; beat < 8; beat++ {
		events = append(events, transcribe.Event{Start: float64(beat) * 0.5, End: float64(beat)*0.5 + 0.45, Midi: 33, String: 1, Fret: 0})
	}
	events = append(events, transcribe.Event{Start: 4, End: 6, Midi: 40, String: 1, Fret: 7})
	s := newSong("test", &project.Result{
		Project: &project.Project{Title: "Prova", Duration: 12, Tuning: "4", Frets: 12, Rhythm: rhythm.Steady(120, 12, 4), Events: events},
		Bass:    track(), Backing: track(),
	})
	t.Cleanup(s.player.Close)
	return s
}

func TestWhereAMomentFalls(t *testing.T) {
	s := testSong(t)
	for _, c := range []struct {
		seconds float64
		bar     int
	}{{0, 0}, {1.9, 0}, {2, 1}, {3.99, 1}, {4, 2}, {11, 5}} {
		if got := s.bar(c.seconds); got != c.bar {
			t.Errorf("%.2f s is in bar %d, want %d", c.seconds, got, c.bar)
		}
	}
	if got := s.barStart(2); math.Abs(got-4) > 1e-6 {
		t.Errorf("bar 3 starts at %f", got)
	}
	if got := s.moment(s.page(3.3)); math.Abs(got-3.3) > 1e-6 {
		t.Errorf("there and back gives %f", got)
	}
}

func TestTheNoteToPlay(t *testing.T) {
	s := testSong(t)
	if got := s.sounding(0.2); got != 0 {
		t.Errorf("at 0.2 s note %d sounds", got)
	}
	if got := s.sounding(0.47); got != -1 { // between two notes
		t.Errorf("at 0.47 s note %d sounds", got)
	}
	if got := s.coming(0.47); got != 1 {
		t.Errorf("after 0.47 s comes note %d", got)
	}
	if got := s.sounding(5); got != 8 {
		t.Errorf("at 5 s note %d sounds", got)
	}
	if got := s.coming(7); got != -1 {
		t.Errorf("after the last note comes %d", got)
	}
}

// A note held for a bar is one sign on the page, four beats long.
func TestAHeldNoteIsWrittenOnce(t *testing.T) {
	s := testSong(t)
	last := s.score.Placed[len(s.score.Placed)-1]
	if last.Slot != 32 || last.Slots != 16 {
		t.Errorf("the held note is at sixteenth %d for %d", last.Slot, last.Slots)
	}
}

func TestRepeat(t *testing.T) {
	s := testSong(t)
	s.loopFrom(1)
	if !s.loopOn || s.loopA != 1 || s.loopB != 1 {
		t.Fatalf("from bar 2: %v %d-%d", s.loopOn, s.loopA, s.loopB)
	}
	s.loopTo(3)
	if s.loopA != 1 || s.loopB != 3 {
		t.Fatalf("to bar 4: %d-%d", s.loopA, s.loopB)
	}
	s.loopFrom(4) // past the end: the stretch becomes that bar alone
	if s.loopA != 4 || s.loopB != 4 {
		t.Fatalf("from bar 5: %d-%d", s.loopA, s.loopB)
	}
	s.loopTo(2) // before the start: likewise
	if s.loopA != 2 || s.loopB != 2 {
		t.Fatalf("to bar 3: %d-%d", s.loopA, s.loopB)
	}
	s.loopOff()
	if s.loopOn {
		t.Fatal("still repeating")
	}
}

func TestAnotherInstrument(t *testing.T) {
	s := testSong(t)
	s.retune("5")
	if len(s.tuning.Open) != 5 || s.project.Tuning != "5" {
		t.Fatalf("tuning %q with %d strings", s.project.Tuning, len(s.tuning.Open))
	}
	for i, event := range s.project.Events {
		if event.String < 0 || s.tuning.Open[event.String]+event.Fret != event.Midi {
			t.Fatalf("note %d: string %d fret %d does not give %d", i, event.String, event.Fret, event.Midi)
		}
	}
}

func TestMovingTheBarLines(t *testing.T) {
	s := testSong(t)
	if got := s.bar(2.2); got != 1 {
		t.Fatalf("2.2 s is in bar %d", got+1)
	}
	s.shiftBars(1) // the "one" is a beat later: 2.2 s is still in the bar before
	if got := s.bar(2.2); got != 0 {
		t.Fatalf("after moving the bar lines a beat on, 2.2 s is in bar %d", got+1)
	}
	if got := s.barStart(1); math.Abs(got-2.5) > 1e-6 {
		t.Fatalf("the second bar starts at %f", got)
	}
	for i := 0; i < 3; i++ {
		s.shiftBars(1)
	}
	if got := s.bar(2.2); got != 1 { // four beats on is where it started
		t.Fatalf("after a whole bar round, 2.2 s is in bar %d", got+1)
	}
	s.shiftBars(-1)
	if s.pulse.Downbeat != 3 {
		t.Fatalf("a beat back from the first is the last: got %d", s.pulse.Downbeat)
	}
}

func TestCorrectingANote(t *testing.T) {
	s := testSong(t)
	id := s.project.Events[1].ID // the A1 on the second beat, open A string
	s.transpose(s.find(id), 3)   // up to C2
	if e := s.project.Events[s.find(id)]; e.Midi != 36 || e.String != 1 || e.Fret != 3 || !e.Edited || !e.Locked {
		t.Fatalf("up three semitones: %+v", e)
	}
	s.restring(s.find(id), -1) // the same C2 on the E string
	if e := s.project.Events[s.find(id)]; e.String != 0 || e.Fret != 8 {
		t.Fatalf("on the string below: string %d fret %d", e.String, e.Fret)
	}
	s.restring(s.find(id), -1) // there is no string below the lowest
	if e := s.project.Events[s.find(id)]; e.String != 0 {
		t.Fatalf("moved off the neck: string %d", e.String)
	}
	s.transpose(s.find(id), -20) // below the instrument: refused
	if e := s.project.Events[s.find(id)]; e.Midi != 36 {
		t.Fatalf("transposed off the instrument: %d", e.Midi)
	}

	// a quarter shortened: a dotted eighth and a sixteenth of rest is not how it would be
	// written, so it becomes an eighth
	s.resize(s.find(id), -1)
	if _, slots := s.slotOf(s.find(id)); slots != 2 {
		t.Fatalf("shortened to %d sixteenths", slots)
	}
	s.resize(s.find(id), 9) // it cannot run into the next note
	if e, next := s.project.Events[s.find(id)], s.project.Events[s.find(id)+1]; e.End > next.Start+1e-9 {
		t.Fatalf("runs into the next note: ends %.3f, next starts %.3f", e.End, next.Start)
	}
	s.move(s.find(id), 1) // a sixteenth later
	if slot, _ := s.slotOf(s.find(id)); slot != 5 {
		t.Fatalf("moved to sixteenth %d", slot)
	}
	s.move(s.find(id), 5) // past the next note: refused
	if slot, _ := s.slotOf(s.find(id)); slot != 5 {
		t.Fatalf("jumped over the next note to sixteenth %d", slot)
	}

	count := len(s.project.Events)
	s.remove(s.find(id))
	if len(s.project.Events) != count-1 || s.find(id) != -1 {
		t.Fatal("the note is still there")
	}
	added := s.add(0.5) // back on the second beat
	if i := s.find(added); i != 1 || s.project.Events[i].Midi != 33 || len(s.project.Events) != count {
		t.Fatalf("added at %d: %+v", i, s.project.Events)
	}
	if s.add(0.5) != "" {
		t.Fatal("a second note was written on the same sixteenth")
	}
	// every change can be taken back, one by one, to the part as it was read
	for s.undo() {
	}
	if e := s.project.Events[1]; len(s.project.Events) != count || e.Midi != 33 || e.Edited || e.ID != id {
		t.Fatalf("after undoing everything: %+v", e)
	}
}

func TestCorrectingChordsAndBars(t *testing.T) {
	s := testSong(t)
	s.project.Chords = []chords.Chord{{Start: 0, End: 4, Root: 9, Quality: "m"}}
	i := s.chordAdd(2.2) // on the beat at 2 s, the first of bar 2: the chord splits there
	if i != 1 || len(s.project.Chords) != 2 || s.project.Chords[0].End != 2 || s.project.Chords[1].End != 4 || s.project.Chords[1].Root != 9 {
		t.Fatalf("split: %+v", s.project.Chords)
	}
	if s.chordAdd(2.3) != -1 {
		t.Fatal("two chords on one beat")
	}
	s.chordRoot(1, 5)
	s.chordKind(1) // minor -> seventh
	if c := s.project.Chords[1]; c.Root != 2 || c.Quality != "7" || !c.Edited {
		t.Fatalf("changed to %+v", c)
	}
	s.chordRemove(1)
	if len(s.project.Chords) != 1 || s.project.Chords[0].End != 4 {
		t.Fatalf("after removing: %+v", s.project.Chords)
	}

	s.setBeats(1, 2) // bar 2 has two beats: bar 3 starts at 3 s instead of 4
	if got := s.barStart(2); math.Abs(got-3) > 1e-6 {
		t.Fatalf("bar 3 starts at %.2f", got)
	}
	s.undo()
	if got := s.barStart(2); math.Abs(got-4) > 1e-6 {
		t.Fatalf("after undoing, bar 3 starts at %.2f", got)
	}
}

// Counting a piece twice as fast and then half as fast leaves it as it was; and the change
// can be taken back.
func TestCountingTwiceAsFast(t *testing.T) {
	s := testSong(t)
	tempo, bars, notes := s.pulse.Tempo(), s.lastBar(), len(s.project.Events)
	s.retempo(true)
	if got := s.pulse.Tempo(); got < tempo*1.9 || got > tempo*2.1 {
		t.Fatalf("twice %.0f is not %.0f", tempo, got)
	}
	if got := s.lastBar(); got < bars*2-1 || got > bars*2+2 {
		t.Errorf("%d bars became %d", bars, got)
	}
	s.retempo(false)
	if got := s.pulse.Tempo(); got < tempo*0.97 || got > tempo*1.03 {
		t.Errorf("back to %.0f, not %.0f", got, tempo)
	}
	s.undo()
	s.undo()
	if got := s.pulse.Tempo(); got < tempo*0.99 || got > tempo*1.01 || len(s.project.Events) != notes {
		t.Errorf("undone, the tempo is %.0f, not %.0f", got, tempo)
	}
}

// Moving the beat half a beat later, twice, is one whole beat; and it can be taken back.
func TestMovingTheBeatHalfway(t *testing.T) {
	s := testSong(t)
	first, second := s.pulse.Beats[0], s.pulse.Beats[1]
	s.offbeat()
	if got, want := s.pulse.Beats[0], (first+second)/2; got < want-0.002 || got > want+0.002 {
		t.Fatalf("the first beat is at %.3f, not halfway at %.3f", got, want)
	}
	s.offbeat()
	if got := s.pulse.Beats[0]; got < second-0.003 || got > second+0.003 {
		t.Errorf("twice halfway is %.3f, not the next beat at %.3f", got, second)
	}
	s.undo()
	s.undo()
	if s.pulse.Beats[0] != first {
		t.Errorf("undone, the first beat is at %.3f, not %.3f", s.pulse.Beats[0], first)
	}
}
