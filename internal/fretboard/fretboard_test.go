package fretboard

import (
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

func TestNames(t *testing.T) {
	if NoteName(28) != "E1" || NoteName(33) != "A1" || NoteName(23) != "B0" || PitchName(42) != "F#" {
		t.Fatal(NoteName(28), NoteName(33), NoteName(23), PitchName(42))
	}
	if TuningFor("5").String() != "B E A D G" || TuningFor("nonsense").Key != "4" {
		t.Fatal("tunings")
	}
}

func TestFingeringIsPlayable(t *testing.T) {
	var line []transcribe.Event
	for i, midi := range []int{31, 33, 35, 36, 38} {
		line = append(line, transcribe.Event{Start: float64(i) * 0.5, End: float64(i)*0.5 + 0.5, Midi: midi, String: -1})
	}
	for _, event := range Finger(line, TuningFor("4"), 12) {
		if event.String < 0 || TuningFor("4").Open[event.String]+event.Fret != event.Midi {
			t.Fatalf("midi %d placed at string %d fret %d", event.Midi, event.String, event.Fret)
		}
	}
}

func TestLockedAndUnplayable(t *testing.T) {
	line := []transcribe.Event{
		{Start: 0, End: 1, Midi: 33, String: 0, Fret: 5, Locked: true}, // A1 on the E string, as the player wants
		{Start: 1, End: 2, Midi: 20, String: -1},                       // below the instrument
	}
	got := Finger(line, TuningFor("4"), 12)
	if got[0].String != 0 || got[0].Fret != 5 {
		t.Fatalf("a locked position was moved to string %d fret %d", got[0].String, got[0].Fret)
	}
	if got[1].String != -1 {
		t.Fatal("a note below the open strings cannot be placed")
	}
}
