package audio

import (
	"math"
	"testing"
)

// A tone made a fifth shorter is a fifth higher, as loud as it was, and nothing else.
func TestRepitch(t *testing.T) {
	const rate, hz = 44100, 220.0
	n := 2 * rate
	source := &Buffer{SampleRate: rate, Channels: [][]float32{make([]float32, n)}}
	for i := range source.Channels[0] {
		source.Channels[0][i] = float32(0.5 * math.Sin(2*math.Pi*hz*float64(i)/rate))
	}
	for _, factor := range []float64{1.5, math.Pow(2, -3.0/12)} {
		out := Repitch(source, factor)
		if want := int(math.Round(float64(n) / factor)); out.Len() != want || out.SampleRate != rate {
			t.Fatalf("factor %.3f: %d frames at %d, want %d", factor, out.Len(), out.SampleRate, want)
		}
		// against the tone it should be
		var worst float64
		signal := out.Channels[0]
		for i := 100; i < len(signal)-100; i++ {
			want := 0.5 * math.Sin(2*math.Pi*hz*factor*float64(i)/rate)
			worst = math.Max(worst, math.Abs(float64(signal[i])-want))
		}
		if worst > 0.01 {
			t.Errorf("factor %.3f: off the tone it should be by %.4f", factor, worst)
		}
	}
	if Repitch(source, 1) != source {
		t.Error("a factor of one made a copy")
	}
}
