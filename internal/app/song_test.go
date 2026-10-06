package app

import (
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
