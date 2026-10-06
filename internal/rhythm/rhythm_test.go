package rhythm

import (
	"math"
	"reflect"
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/audio"
)

func clickTrack(bpm float64) *audio.Buffer {
	const rate, seconds = 11025, 24
	data := make([]float32, rate*seconds)
	seed := 1
	noise := func() float64 { seed = seed * 16807 % 2147483647; return float64(seed)/2147483647 - 0.5 }
	for beat := 0; float64(beat)*60/bpm < seconds-0.1; beat++ {
		start := int(math.Round((0.2 + float64(beat)*60/bpm) * rate))
		level := 0.5
		if beat%4 == 0 {
			level = 1
		}
		for i := 0; i < 400; i++ {
			data[start+i] += float32(level * noise() * math.Exp(-float64(i)/80))
		}
	}
	return &audio.Buffer{SampleRate: rate, Channels: [][]float32{data}}
}

func TestTheBeatOfAClickTrack(t *testing.T) {
	pulse := Analyse(clickTrack(100), 4)
	if pulse == nil {
		t.Fatal("no beat found")
	}
	if tempo := pulse.Tempo(); math.Abs(tempo-100) > 1.5 {
		t.Fatalf("a 100 BPM click was read as %.1f BPM", tempo)
	}
	for i := 1; i < len(pulse.Beats); i++ {
		if math.Abs(pulse.Beats[i]-pulse.Beats[i-1]-0.6) > 0.04 {
			t.Fatalf("beats %d and %d are %.3fs apart", i-1, i, pulse.Beats[i]-pulse.Beats[i-1])
		}
	}
	phase := math.Mod((pulse.Beats[0]-0.2)/0.6+0.5, 1) - 0.5
	if math.Abs(phase) > 0.08 {
		t.Fatalf("beats are %.2f of a beat away from the clicks", phase)
	}
}

func TestPositions(t *testing.T) {
	even := Steady(120, 16, 4)
	if even.Tempo() != 120 || even.Position(1) != 2 {
		t.Fatalf("tempo %.1f, position of 1s %.2f", even.Tempo(), even.Position(1))
	}
	shifted := *even
	shifted.Downbeat = 1
	if shifted.Position(1) != 1 {
		t.Fatal("positions count from the first bar line")
	}
	if got := even.Time(even.Position(3.21)); math.Abs(got-3.21) > 1e-9 {
		t.Fatalf("there and back gives %.4f", got)
	}
	if got := even.Position(-1); math.Abs(got+2) > 1e-9 {
		t.Fatalf("the pulse should carry on before the first beat, got %.3f", got)
	}
	if even.Rescale(true).Tempo() != 240 || even.Rescale(false).Tempo() != 60 {
		t.Fatal("doubling and halving the tempo")
	}
}

func TestSplitValues(t *testing.T) {
	cases := []struct {
		start, length int
		rest          bool
		want          []int
		why           string
	}{
		{0, 16, false, []int{16}, "a whole bar"},
		{0, 12, false, []int{12}, "a dotted half"},
		{0, 6, false, []int{6}, "a dotted quarter"},
		{2, 4, false, []int{4}, "a quarter on the off-beat stays a quarter"},
		{1, 3, false, []int{3}, "a dotted eighth up to the beat"},
		{3, 6, false, []int{1, 4, 1}, "a value that straddles beats is split at them"},
		{4, 12, false, []int{4, 8}, "three beats from beat two: a quarter tied to a half"},
		{2, 6, false, []int{2, 4}, "an eighth up to the beat, then a quarter"},
		{4, 6, false, []int{6}, "a dotted quarter from beat two"},
		{0, 12, true, []int{8, 4}, "rests are never dotted"},
		{2, 6, true, []int{2, 4}, "rests are split at the beat"},
	}
	for _, c := range cases {
		var got []int
		for _, piece := range SplitValues(c.start, c.length, 16, c.rest) {
			got = append(got, piece.Value)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: from %d for %d gives %v, want %v", c.why, c.start, c.length, got, c.want)
		}
	}
}

func TestALongNoteIsWrittenOnceAndTied(t *testing.T) {
	even := Steady(120, 16, 4)
	score := even.Notate([]Note{{0, 6}, {6, 6.25}, {6.25, 6.5}})
	type sign struct {
		value int
		tied  bool
	}
	var long []sign
	for _, symbol := range score.Symbols {
		if !symbol.Rest && symbol.Index == 0 {
			long = append(long, sign{symbol.Value, symbol.Tied})
		}
	}
	if !reflect.DeepEqual(long, []sign{{16, false}, {16, true}, {16, true}}) {
		t.Fatalf("a note of three bars is written as %v", long)
	}
	var last []string
	for _, symbol := range score.Symbols {
		if symbol.Bar == 3 {
			kind := "note"
			if symbol.Rest {
				kind = "rest"
			}
			last = append(last, kind+string(rune('0'+symbol.Value)))
		}
	}
	if !reflect.DeepEqual(last, []string{"note2", "note2", "rest4", "rest8"}) {
		t.Fatalf("the fourth bar is written as %v", last)
	}
}

func TestLateNotesAreWrittenOnTheirBeat(t *testing.T) {
	even := Steady(120, 16, 4)
	var late []Note
	for i := 0; i < 16; i++ {
		late = append(late, Note{float64(i)*0.25 + 0.04, float64(i)*0.25 + 0.27})
	}
	if shift := even.Calibrate(late); math.Abs(shift-0.08) > 0.01 {
		t.Fatalf("notes 40 ms late are measured %.3f beats late", shift)
	}
	for i, placed := range even.Quantize(late) {
		if placed.Slot != i*2 || placed.Slots != 2 {
			t.Fatalf("note %d is at slot %d for %d", i, placed.Slot, placed.Slots)
		}
	}
}

// A bar of two beats in a piece in four: every bar after it starts two beats earlier, and a
// note held across it is cut at its lines, not at where the lines would have been.
func TestABarOfItsOwnLength(t *testing.T) {
	r := Steady(120, 30, 4)
	r.SetBeatsIn(2, 2)
	for bar, want := range []int{0, 4, 8, 10, 14} {
		if got := r.BarStart(bar); got != want {
			t.Errorf("bar %d starts at beat %d, want %d", bar+1, got, want)
		}
	}
	if r.BarStart(-1) != -4 {
		t.Errorf("the pickup bar starts at beat %d", r.BarStart(-1))
	}
	for _, c := range []struct {
		beats float64
		bar   int
	}{{-0.5, -1}, {0, 0}, {7.9, 1}, {8, 2}, {9.9, 2}, {10, 3}, {13.9, 3}, {14, 4}} {
		if got := r.BarAt(c.beats); got != c.bar {
			t.Errorf("beat %.1f is in bar %d, want %d", c.beats, got+1, c.bar+1)
		}
	}
	// one note from beat 6 to beat 12: half of bar 2, all of the short bar 3, half of bar 4
	score := r.Notate([]Note{{Start: 3, End: 6}})
	var got []Symbol
	for _, symbol := range score.Symbols {
		if !symbol.Rest {
			got = append(got, symbol)
		}
	}
	want := []Symbol{{Bar: 1, Slot: 8, Value: 8, At: 24}, {Bar: 2, Slot: 0, Value: 8, Tied: true, At: 32}, {Bar: 3, Slot: 0, Value: 8, Tied: true, At: 40}}
	if len(got) != len(want) {
		t.Fatalf("written as %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sign %d is %+v, want %+v", i, got[i], want[i])
		}
	}
	r.SetBeatsIn(2, 4) // back to the usual
	if len(r.Odd) != 0 || r.BarStart(3) != 12 {
		t.Errorf("the bar did not go back to four beats: %+v", r.Odd)
	}
}
