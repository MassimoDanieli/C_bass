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
	return isolatedNotes(signal, rate, 0.72, 0, 26, func(float64) {})
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
	got := selectVotes([]pitched{{midi: 33, confidence: 0.72}, {midi: 33, confidence: 0.68}, {midi: 45, confidence: 0.91}})
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

// wave builds a sound from a pitch and a level for each harmonic, both free to change in time.
func wave(seconds float64, pitch func(t float64) float64, harmonics ...func(t float64) float64) []float32 {
	out := make([]float32, int(math.Round(seconds*rate)))
	var phase float64
	for i := range out {
		t := float64(i) / rate
		phase += 2 * math.Pi * 440 * math.Pow(2, (pitch(t)-69)/12) / rate
		var v float64
		for h, level := range harmonics {
			v += level(t) * math.Sin(float64(h+1)*phase)
		}
		out[i] = float32(v)
	}
	return out
}

func steady(midi float64) func(float64) float64 { return func(float64) float64 { return midi } }
func level(v float64) func(float64) float64     { return func(float64) float64 { return v } }

func midis(events []Event) []int {
	out := make([]int, len(events))
	for i, event := range events {
		out[i] = event.Midi
	}
	return out
}

func same(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A low string left ringing loses its fundamental: what is left is the spectrum of the note
// an octave above. It is still the note that was played, and one note.
func TestANoteLosingItsFundamentalStaysOneNote(t *testing.T) {
	fading := func(t float64) float64 { return 0.3 * math.Max(0, 1-t/0.9) }
	got := notes(join(silence(0.3), wave(3, steady(31), fading, level(0.25), fading, level(0.12)), silence(0.3)))
	if !same(midis(got), []int{31}) {
		t.Fatalf("a G1 held for three seconds was read as %v", midis(got))
	}
}

// Octaves played one after the other are the line, not a misreading to be smoothed away.
func TestOctavesArePlayedAsOctaves(t *testing.T) {
	pluck := func(t float64) float64 { return 0.4 * math.Exp(-t*5) }
	parts := [][]float32{silence(0.3)}
	var want []int
	for i := 0; i < 8; i++ {
		midi := 28 + 12*(i%2)
		parts = append(parts, wave(0.25, steady(float64(midi)), pluck, func(t float64) float64 { return 0.4 * pluck(t) }, func(t float64) float64 { return 0.2 * pluck(t) }))
		want = append(want, midi)
	}
	parts = append(parts, silence(0.3))
	buffer := join(parts...)
	got := StabilizeOctaves(Normalize(isolatedNotes(buffer, rate, 0.72, 0, 26, func(float64) {}), 3))
	if !same(midis(got), want) {
		t.Fatalf("E1 and E2 in turn were read as %v", midis(got))
	}
}

// A recording a little under concert pitch, or a fretless played a little flat: the notes
// sit between two names, and must not flicker from one to the other.
func TestNotesBetweenTwoPitchesKeepOneName(t *testing.T) {
	pluck := func(t float64) float64 { return 0.4 * math.Exp(-t*2) }
	wobble := func(midi float64) func(float64) float64 {
		return func(t float64) float64 { return midi - 0.45 + 0.08*math.Sin(2*math.Pi*5*t) }
	}
	parts := [][]float32{silence(0.3)}
	for _, midi := range []float64{36, 38, 40, 36, 43, 41, 38, 36} {
		parts = append(parts, wave(0.5, wobble(midi), pluck, func(t float64) float64 { return 0.3 * pluck(t) }))
	}
	parts = append(parts, silence(0.3))
	got := notes(join(parts...))
	if len(got) != 8 {
		t.Fatalf("eight notes 45 cents flat were read as %d: %v", len(got), midis(got))
	}
	// whichever way the whole is named, the line must keep its shape
	for i, step := range []int{2, 2, -4, 7, -2, -3, -2} {
		if got[i+1].Midi-got[i].Midi != step {
			t.Fatalf("the line lost its shape: %v", midis(got))
		}
	}
}

// A slide goes through every pitch on the way: only where it starts and where it lands are notes.
func TestASlideLeavesNoNotesOnTheWay(t *testing.T) {
	glide := func(t float64) float64 {
		switch {
		case t < 0.5:
			return 33
		case t < 0.7:
			return 33 + 5*(t-0.5)/0.2
		}
		return 38
	}
	got := notes(join(silence(0.3), wave(1.4, glide, level(0.3), level(0.12)), silence(0.3)))
	if !same(midis(got), []int{33, 38}) {
		t.Fatalf("a slide from A1 to D2 was read as %v", midis(got))
	}
}

// What a separation leaves behind where there is no bass is not silence, and measured only
// against itself it would be read as notes: it is measured against the recording.
func TestWhatIsLeftOfNoBassIsNotNotes(t *testing.T) {
	seed := uint32(7)
	faint := make([]float32, 6*rate)
	for i := range faint {
		seed = seed*1664525 + 1013904223
		// noise, with a wandering hum in it
		faint[i] = float32(0.0002*(float64(seed>>8)/float64(1<<23)-1) + 0.0002*math.Sin(2*math.Pi*(70+20*math.Sin(float64(i)/rate))*float64(i)/rate))
	}
	if got := isolatedNotes(faint, rate, 0.72, 0, 26, func(float64) {}); len(got) == 0 {
		t.Skip("nothing was read even without the floor")
	}
	if got := isolatedNotes(faint, rate, 0.72, 0.004, 26, func(float64) {}); len(got) != 0 {
		t.Fatalf("%d notes were read in what is left of no bass", len(got))
	}
}

// A four-string bass has no C1: a C2 whose waves come in unequal pairs is still a C2.
func TestNothingIsReadBelowTheInstrument(t *testing.T) {
	uneven := wave(2, steady(36), level(0.3), level(0.1))
	period := float64(rate) / (440 * math.Pow(2, (36.0-69)/12))
	for i := range uneven {
		if int(float64(i)/period)%2 == 1 {
			uneven[i] *= 0.8
		}
	}
	got := notes(join(silence(0.3), uneven, silence(0.3)))
	if !same(midis(got), []int{36}) {
		t.Fatalf("a C2 was read as %v", midis(got))
	}
}
