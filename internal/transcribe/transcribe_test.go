package transcribe

import (
	"math"
	"testing"
)

const rate = analysisRate

func tone(frequency, seconds float64, shape func(t float64) float64) []float32 {
	out := make([]float32, int(math.Round(seconds*rate)))
	for i := range out {
		t := float64(i) / rate
		out[i] = float32(shape(t) * (math.Sin(2*math.Pi*frequency*t) + 0.4*math.Sin(4*math.Pi*frequency*t)))
	}
	return out
}

func silence(seconds float64) []float32 { return make([]float32, int(math.Round(seconds*rate))) }

func join(parts ...[]float32) []float32 {
	var out []float32
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func notes(signal []float32) []Event {
	return isolatedNotes(signal, rate, 0.72, func(float64) {})
}

func TestAHeldNoteIsOneNote(t *testing.T) {
	swell := func(t float64) float64 { return 0.25 + 0.2*math.Min(1, t/2) }
	got := notes(join(silence(0.3), tone(55, 4, swell), silence(0.3)))
	if len(got) != 1 {
		t.Fatalf("a note held for four seconds was read as %d notes", len(got))
	}
	if got[0].Midi != 33 || got[0].End-got[0].Start < 3.7 {
		t.Fatalf("got midi %d lasting %.2fs", got[0].Midi, got[0].End-got[0].Start)
	}
}

func TestPluckedNotesAreSeparate(t *testing.T) {
	pluck := func(t float64) float64 { return 0.4 * math.Exp(-t*9) }
	parts := [][]float32{silence(0.3)}
	for i := 0; i < 8; i++ {
		parts = append(parts, tone(55, 0.25, pluck))
	}
	parts = append(parts, silence(0.3))
	got := notes(join(parts...))
	if len(got) != 8 {
		t.Fatalf("eight plucks were read as %d notes", len(got))
	}
	for _, event := range got {
		if event.Midi != 33 {
			t.Fatalf("a plucked A1 was read as midi %d", event.Midi)
		}
	}
}

func TestAChangeOfPitchStartsANote(t *testing.T) {
	flat := func(float64) float64 { return 0.3 }
	got := notes(join(silence(0.3), tone(55, 0.8, flat), tone(65.41, 0.8, flat), silence(0.3)))
	if len(got) != 2 || got[0].Midi != 33 || got[1].Midi != 36 {
		t.Fatalf("a slur from A1 to C2 was read as %+v", got)
	}
	if math.Abs(got[1].Start-1.1) > 0.06 {
		t.Fatalf("the second note starts at %.2fs, expected about 1.10", got[1].Start)
	}
}

func TestSilenceHasNoNotes(t *testing.T) {
	if got := notes(silence(2)); len(got) != 0 {
		t.Fatalf("silence was read as %d notes", len(got))
	}
}

func TestSelectVotesPrefersAgreement(t *testing.T) {
	got := selectVotes([]pitched{{33, 0.72}, {33, 0.68}, {45, 0.91}})
	if got.midi != 33 {
		t.Fatalf("two windows agreeing on 33 lost to one confident 45: got %d", got.midi)
	}
}

func TestAnalysisOffsetsStayInsideTheNote(t *testing.T) {
	offsets := analysisOffsets(1, 1.11)
	if len(offsets) < 2 {
		t.Fatalf("a short note gets %d windows", len(offsets))
	}
	for _, offset := range offsets {
		if 1+offset >= 1.11 {
			t.Fatalf("a window at +%.3f runs into the next note", offset)
		}
	}
}

func TestNormalizeAndOctaves(t *testing.T) {
	events := Normalize([]Event{
		{Start: 0.5, End: 2, Midi: 33, Confidence: 0.9},
		{Start: 0, End: 0.9, Midi: 33, Confidence: 0.9},
		{Start: 1, End: 1.2, Midi: 45, Confidence: 0.5},
	}, 3)
	if events[0].Start != 0 || events[0].End != 0.5 || events[1].End != 1 {
		t.Fatalf("notes are not in order or overlap: %+v", events)
	}
	if events[0].ID == "" || events[0].ID == events[1].ID {
		t.Fatal("notes need distinct identities")
	}
	fixed := StabilizeOctaves(events)
	if fixed[2].Midi != 33 || fixed[2].RawMidi != 45 {
		t.Fatalf("a quick repeat an octave up should be pulled back: midi %d raw %d", fixed[2].Midi, fixed[2].RawMidi)
	}
}
