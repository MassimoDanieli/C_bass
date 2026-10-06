package app

import (
	"fmt"
	"github.com/MassimoDanieli/c_bass/internal/audio"
	"math"
	"sort"

	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/player"
	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
)

// song is a recording open for playing: its part written out, and the player.
type song struct {
	id      string
	project *project.Project
	player  *player.Player
	bass    *audio.Buffer // the two tracks, kept to read the notes again
	backing *audio.Buffer
	tuning  fretboard.Tuning
	pulse   *rhythm.Rhythm
	score   *rhythm.Score
	perBar  int

	loopOn       bool
	loopA, loopB int // bars, both included

	chosen string // the note chosen for correcting, by its name; "" for none
	chord  int    // the chord chosen, by its place; -1 for none

	history []state // what the part was before each change made by hand
	made    int     // notes added by hand so far, to name them
}

func newSong(id string, result *project.Result) *song {
	s := &song{id: id, project: result.Project, player: player.New(result.Bass, result.Backing), chord: -1, bass: result.Bass, backing: result.Backing}
	s.pulse = result.Project.Rhythm
	if s.pulse == nil || len(s.pulse.Beats) < 2 || s.pulse.PerBar < 1 {
		s.pulse = rhythm.Steady(120, result.Project.Duration, 4)
	}
	s.perBar = s.pulse.PerBar
	for i := range s.project.Events { // every note needs a name to be told from the others
		if s.project.Events[i].ID == "" {
			s.project.Events[i].ID = fmt.Sprintf("n-%d-%d", i, int(s.project.Events[i].Start*1000))
		}
	}
	s.write()
	s.tellBeats()
	return s
}

// write lays the notes out on the page.
func (s *song) write() {
	s.tuning = fretboard.TuningFor(s.project.Tuning)
	notes := make([]rhythm.Note, len(s.project.Events))
	for i, event := range s.project.Events {
		notes[i] = rhythm.Note{Start: event.Start, End: event.End}
	}
	s.score = s.pulse.Notate(notes)
}

// retune chooses the fingering again for another instrument.
func (s *song) retune(key string) {
	frets := s.project.Frets
	if frets <= 0 {
		frets = 12
	}
	s.project.Refinger(key, frets)
	s.write()
}

// page is where a moment of the recording falls on the page, in beats from the first bar line.
func (s *song) page(seconds float64) float64 { return s.pulse.Position(seconds) - s.score.Shift }

// moment is the inverse of page.
func (s *song) moment(beats float64) float64 {
	return math.Max(0, math.Min(s.player.Duration(), s.pulse.Time(beats+s.score.Shift)))
}

// bar is the bar a moment falls in, counted from 0.
func (s *song) bar(seconds float64) int {
	beats := s.page(seconds) + 1e-6
	if beats < 0 && beats > -0.5 { // the page begins a hair after the recording does: still bar one
		beats = 0
	}
	return s.pulse.BarAt(beats)
}

func (s *song) barStart(bar int) float64 { return s.moment(float64(s.pulse.BarStart(bar))) }

// lastBar is the bar in which the recording ends.
func (s *song) lastBar() int { return s.bar(s.player.Duration()) }

// sounding is the note being played at a moment, or -1.
func (s *song) sounding(seconds float64) int {
	events := s.project.Events
	i := sort.Search(len(events), func(i int) bool { return events[i].Start > seconds }) - 1
	if i >= 0 && seconds < events[i].End {
		return i
	}
	return -1
}

// coming is the next note to start after a moment, or -1.
func (s *song) coming(seconds float64) int {
	events := s.project.Events
	if i := sort.Search(len(events), func(i int) bool { return events[i].Start > seconds }); i < len(events) {
		return i
	}
	return -1
}

// shiftBars moves every bar line by a beat, for when the "one" was heard in the wrong place.
func (s *song) shiftBars(by int) {
	s.remember()
	s.pulse.Downbeat = ((s.pulse.Downbeat+by)%s.perBar + s.perBar) % s.perBar
	s.changed()
}

func (s *song) applyLoop() {
	s.player.SetLoop(s.barStart(s.loopA), s.barStart(s.loopB+1), s.loopOn)
}

// loopFrom starts the repeat at a bar, keeping the end if it still comes after.
func (s *song) loopFrom(bar int) {
	if !s.loopOn || s.loopB < bar {
		s.loopB = bar
	}
	s.loopA, s.loopOn = bar, true
	s.applyLoop()
}

// loopTo ends the repeat at a bar, keeping the start if it still comes before.
func (s *song) loopTo(bar int) {
	if !s.loopOn || s.loopA > bar {
		s.loopA = bar
	}
	s.loopB, s.loopOn = bar, true
	s.applyLoop()
}

func (s *song) loopOff() {
	s.loopOn = false
	s.applyLoop()
}

var latin = [...]string{"Do", "Do#", "Re", "Re#", "Mi", "Fa", "Fa#", "Sol", "Sol#", "La", "La#", "Si"}

// noteName is the name of a note in the language of the window.
func (g *Game) noteName(midi int) string {
	if g.settings.English {
		return fretboard.PitchName(midi)
	}
	return latin[((midi%12)+12)%12]
}
