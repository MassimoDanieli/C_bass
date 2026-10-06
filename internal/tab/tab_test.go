package tab

import (
	"strings"
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

func TestText(t *testing.T) {
	tuning := fretboard.TuningFor("4")
	events := fretboard.Finger([]transcribe.Event{
		{Start: 0, End: 2.5, Midi: 33, String: -1},  // A1 held for a bar and a quarter
		{Start: 2.5, End: 3, Midi: 36, String: -1},  // C2
		{Start: 3, End: 3.25, Midi: 38, String: -1}, // D2
	}, tuning, 12)
	text := Text("Test", events, rhythm.Steady(120, 8, 4), tuning, 4)
	lines := strings.Split(text, "\n")
	if !strings.HasPrefix(lines[1], "E A D G · 120 BPM · 4/4") {
		t.Fatalf("heading: %q", lines[1])
	}
	var a string
	for _, line := range lines {
		if strings.HasPrefix(line, "A ") {
			a = line
		}
	}
	bars := strings.Split(a, "|")
	if len(bars) < 3 || !strings.HasPrefix(bars[1], "0-") || !strings.HasPrefix(bars[2], "(0)") {
		t.Fatalf("the held note should be written once, then in brackets in the next bar:\n%s", text)
	}
	if strings.Count(a, "0") != 2 || !strings.Contains(a, "3") || !strings.Contains(a, "5") {
		t.Fatalf("A string: %s", a)
	}
	if !strings.Contains(text, "~q") || !strings.Contains(text, "w ") {
		t.Fatalf("the values should show a whole note tied to a quarter:\n%s", text)
	}
}
