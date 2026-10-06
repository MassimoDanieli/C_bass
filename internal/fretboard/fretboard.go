// Package fretboard knows the instrument: its tunings, and where on the neck a line is best played.
package fretboard

import (
	"fmt"
	"math"
	"strings"

	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// Tuning is a set of open strings, lowest first, as MIDI notes.
type Tuning struct {
	Key   string
	Label string
	Open  []int
}

// Tunings are the instruments the program knows.
var Tunings = []Tuning{
	{"4", "E A D G", []int{28, 33, 38, 43}},
	{"5", "B E A D G", []int{23, 28, 33, 38, 43}},
	{"5c", "E A D G C", []int{28, 33, 38, 43, 48}},
	{"6", "B E A D G C", []int{23, 28, 33, 38, 43, 48}},
}

// TuningFor returns the tuning with that key, or the four-string bass.
func TuningFor(key string) Tuning {
	for _, tuning := range Tunings {
		if tuning.Key == key {
			return tuning
		}
	}
	return Tunings[0]
}

var noteNames = [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// NoteName is the name of a MIDI note with its octave: 28 is E1.
func NoteName(midi int) string {
	return fmt.Sprintf("%s%d", noteNames[((midi%12)+12)%12], int(math.Floor(float64(midi)/12))-1)
}

// PitchName is the name of a MIDI note without its octave.
func PitchName(midi int) string { return noteNames[((midi%12)+12)%12] }

// StringNames are the names of the open strings of a tuning.
func (t Tuning) StringNames() []string {
	names := make([]string, len(t.Open))
	for i, midi := range t.Open {
		names[i] = PitchName(midi)
	}
	return names
}

func (t Tuning) String() string { return strings.Join(t.StringNames(), " ") }

type position struct {
	str, fret int // str is -1 when the note cannot be played
	locked    bool
}

func candidates(event transcribe.Event, open []int, maxFret int) []position {
	if event.Locked && event.String >= 0 && event.String < len(open) && event.Fret >= 0 && event.Fret <= maxFret && open[event.String]+event.Fret == event.Midi {
		return []position{{event.String, event.Fret, true}}
	}
	var out []position
	for s, openMidi := range open {
		if fret := event.Midi - openMidi; fret >= 0 && fret <= maxFret {
			out = append(out, position{s, fret, false})
		}
	}
	if len(out) == 0 {
		return []position{{-1, -1, false}}
	}
	return out
}

func transitionCost(previous, current position, previousEvent, currentEvent transcribe.Event) float64 {
	if previous.str < 0 || current.str < 0 {
		return 18
	}
	fretMove := math.Abs(float64(current.fret - previous.fret))
	stringMove := math.Abs(float64(current.str - previous.str))
	pause := math.Max(0, currentEvent.Start-previousEvent.End)
	onsetGap := math.Max(0, currentEvent.Start-previousEvent.Start)
	cost := fretMove*1.55 + stringMove*2.3
	if fretMove > 5 {
		cost += (fretMove - 5) * 1.75
	}
	if stringMove > 2 {
		cost += (stringMove - 2) * 1.4
	}
	if currentEvent.Midi == previousEvent.Midi && current.str == previous.str && current.fret == previous.fret {
		if onsetGap <= 0.75 {
			cost -= 3.2
		} else {
			cost -= 1.4
		}
	}
	if pause > 0.4 {
		cost *= 0.62
	}
	return cost + float64(current.fret)*0.02
}

// Finger chooses a string and a fret for every note, as one path through the whole line that
// keeps the hand from jumping about. Positions the player has locked are kept.
func Finger(events []transcribe.Event, tuning Tuning, maxFret int) []transcribe.Event {
	if len(events) == 0 {
		return nil
	}
	layers := make([][]position, len(events))
	costs := make([][]float64, len(events))
	back := make([][]int, len(events))
	for i, event := range events {
		layers[i] = candidates(event, tuning.Open, maxFret)
		costs[i] = make([]float64, len(layers[i]))
		back[i] = make([]int, len(layers[i]))
		for c, pos := range layers[i] {
			costs[i][c] = math.Inf(1)
			back[i][c] = -1
			bonus := 0.0
			if pos.locked {
				bonus = 2
			}
			if i == 0 {
				if pos.str < 0 {
					costs[i][c] = 40
				} else {
					costs[i][c] = float64(pos.fret)*0.11 + float64(pos.str)*0.06 - bonus
				}
				continue
			}
			for p, previous := range layers[i-1] {
				if value := costs[i-1][p] + transitionCost(previous, pos, events[i-1], event) - bonus; value < costs[i][c] {
					costs[i][c] = value
					back[i][c] = p
				}
			}
		}
	}
	last := len(events) - 1
	cursor := 0
	for c, value := range costs[last] {
		if value < costs[last][cursor] {
			cursor = c
		}
	}
	out := append([]transcribe.Event(nil), events...)
	for i := last; i >= 0; i-- {
		out[i].String, out[i].Fret = layers[i][cursor].str, layers[i][cursor].fret
		cursor = back[i][cursor]
		if cursor < 0 && i > 0 {
			cursor = 0
		}
	}
	return out
}
