package demucs

import (
	"math"
	"testing"
)

func newWork() *separator {
	work := &separator{
		fft: newTransform(fftSize), window: make([]float64, fftSize),
		re: make([]float64, fftSize), im: make([]float64, fftSize),
		overlap: make([]float64, segment+8*hop),
	}
	for i := range work.window {
		work.window[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/fftSize))
	}
	return work
}

func TestTransformOfASine(t *testing.T) {
	const size = 64
	fft := newTransform(size)
	re, im := make([]float64, size), make([]float64, size)
	for i := range re {
		re[i] = math.Cos(2 * math.Pi * 5 * float64(i) / size)
	}
	fft.forward(re, im)
	for bin := range re {
		want := 0.0
		if bin == 5 || bin == size-5 {
			want = size / 2
		}
		if math.Abs(math.Hypot(re[bin], im[bin])-want) > 1e-9 {
			t.Fatalf("bin %d has magnitude %f, want %f", bin, math.Hypot(re[bin], im[bin]), want)
		}
	}
}

func TestReflect(t *testing.T) {
	for _, c := range [][3]int{{-1, 5, 1}, {-3, 5, 3}, {5, 5, 3}, {6, 5, 2}, {2, 5, 2}, {-9, 5, 1}} {
		if got := reflectIndex(c[0], c[1]); got != c[2] {
			t.Errorf("reflect(%d, %d) = %d, want %d", c[0], c[1], got, c[2])
		}
	}
}

// What goes into the network as a spectrogram must come back out as the same sound: the
// network only ever scales it.
func TestSpectrogramAndBack(t *testing.T) {
	work := newWork()
	frames := (segment + hop - 1) / hop
	signal := make([]float32, segment)
	for i := range signal {
		x := float64(i) / SampleRate
		signal[i] = float32(0.4*math.Sin(2*math.Pi*110*x) + 0.2*math.Sin(2*math.Pi*1870*x+1) + 0.1*math.Sin(2*math.Pi*41*x))
	}
	re := make([]float32, bins*frames)
	im := make([]float32, bins*frames)
	work.spectrogram(signal, re, im, frames)
	out := make([]float32, segment)
	work.inverse(re, im, frames, segment, out)
	var worst float64
	// The first and last hops and a half are attenuated by design: the network's waveform branch covers them.
	for i := 3 * hop; i < segment-3*hop; i++ {
		worst = math.Max(worst, math.Abs(float64(signal[i]-out[i])))
	}
	if worst > 1e-4 {
		t.Fatalf("the sound came back up to %g away from itself", worst)
	}
}
