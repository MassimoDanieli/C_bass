// Package demucs separates a recording into drums, bass, other and vocals with the Hybrid
// Transformer Demucs model (Meta), run through ONNX Runtime.
//
// The model itself only covers the neural network. The spectrogram that goes in and the one
// that comes back out are computed here, following demucs-js by Kevin Gibbons
// (https://github.com/bakkot/demucs-js, MIT), itself after sevagh/demucs.onnx.
package demucs

import "math"

const (
	fftSize = 4096
	hop     = 1024
	bins    = fftSize / 2 // the model does not see the last frequency bin
)

// transform is a radix-2 FFT of a fixed size with its tables worked out once.
type transform struct {
	size     int
	reversed []int
	cos, sin []float64
}

func newTransform(size int) *transform {
	t := &transform{size: size, reversed: make([]int, size), cos: make([]float64, size/2), sin: make([]float64, size/2)}
	bitsN := 0
	for 1<<bitsN < size {
		bitsN++
	}
	for i := range t.reversed {
		r := 0
		for b := 0; b < bitsN; b++ {
			if i&(1<<b) != 0 {
				r |= 1 << (bitsN - 1 - b)
			}
		}
		t.reversed[i] = r
	}
	for i := range t.cos {
		angle := -2 * math.Pi * float64(i) / float64(size)
		t.cos[i], t.sin[i] = math.Cos(angle), math.Sin(angle)
	}
	return t
}

// forward transforms real and imag in place.
func (t *transform) forward(real, imag []float64) {
	n := t.size
	for i, r := range t.reversed {
		if i < r {
			real[i], real[r] = real[r], real[i]
			imag[i], imag[r] = imag[r], imag[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		halfLength := length >> 1
		step := n / length
		for start := 0; start < n; start += length {
			for j := 0; j < halfLength; j++ {
				wr, wi := t.cos[j*step], t.sin[j*step]
				a, b := start+j, start+j+halfLength
				tr := wr*real[b] - wi*imag[b]
				ti := wr*imag[b] + wi*real[b]
				real[b], imag[b] = real[a]-tr, imag[a]-ti
				real[a] += tr
				imag[a] += ti
			}
		}
	}
}

// reflectIndex mirrors an index into [0, n) the way torch's reflect padding does.
func reflectIndex(i, n int) int {
	for i < 0 || i >= n {
		if i < 0 {
			i = -i
		}
		if i >= n {
			i = 2*(n-1) - i
		}
	}
	return i
}

// spectrogram computes what the model expects for one channel of `length` samples: `frames`
// columns of `bins` complex values, written into real and imag as [bin*frames+frame].
//
// It is torch.stft (Hann window, centred, normalised) of the signal reflect-padded by 3/2 hop
// on the left, with the first two frames and the last frequency bin dropped.
func (s *separator) spectrogram(signal []float32, real, imag []float32, frames int) {
	n := len(signal)
	pad := hop / 2 * 3
	norm := 1 / math.Sqrt(fftSize)
	re, im := s.re, s.im
	// Sample at position p of the twice-padded signal: first padding (pad, right up to frames*hop+pad),
	// then the centring padding of fftSize/2 on both sides.
	first := func(p int) float64 { // index into the once-padded signal
		return float64(signal[reflectIndex(p-pad, n)])
	}
	onceLength := pad + frames*hop + pad
	at := func(p int) float64 {
		return first(reflectIndex(p-fftSize/2, onceLength))
	}
	for frame := 0; frame < frames; frame++ {
		start := (frame + 2) * hop
		for i := 0; i < fftSize; i++ {
			re[i] = at(start+i) * s.window[i] * norm
			im[i] = 0
		}
		s.fft.forward(re, im)
		for bin := 0; bin < bins; bin++ {
			real[bin*frames+frame] = float32(re[bin])
			imag[bin*frames+frame] = float32(im[bin])
		}
	}
}

// inverse turns one channel of the model's spectrogram output back into `length` samples,
// added into out. It mirrors spectrogram: the dropped bin and the two frames at each end come
// back as zeros, then torch.istft, then the padding is cut away.
func (s *separator) inverse(real, imag []float32, frames, length int, out []float32) {
	pad := hop / 2 * 3
	full := hop*((length+hop-1)/hop) + 2*pad // samples the inverse transform produces
	sum := s.overlap[:full]
	for i := range sum {
		sum[i] = 0
	}
	weight := s.windowSum(frames, full)
	norm := math.Sqrt(fftSize)
	re, im := s.re, s.im
	for frame := 0; frame < frames; frame++ {
		for bin := 0; bin < bins; bin++ {
			re[bin] = float64(real[bin*frames+frame])
			im[bin] = -float64(imag[bin*frames+frame]) // conjugate: the inverse is a forward transform of it
		}
		re[bins], im[bins] = 0, 0
		for bin := bins + 1; bin < fftSize; bin++ {
			re[bin] = re[fftSize-bin]
			im[bin] = -im[fftSize-bin]
		}
		s.fft.forward(re, im)
		start := (frame+2)*hop - fftSize/2
		for i := 0; i < fftSize; i++ {
			if p := start + i; p >= 0 && p < full {
				sum[p] += re[i] / fftSize * s.window[i] * norm
			}
		}
	}
	for i := 0; i < length; i++ {
		p := pad + i
		v := sum[p]
		if weight[p] > 1e-8 {
			v /= weight[p]
		}
		out[i] += float32(v)
	}
}

// windowSum is the overlap of the squared windows of every frame, the silent ones at the two
// ends included, which is what the inverse transform divides by. It only depends on the sizes.
func (s *separator) windowSum(frames, full int) []float64 {
	if len(s.weight) == full {
		return s.weight
	}
	s.weight = make([]float64, full)
	for frame := 0; frame < frames+4; frame++ {
		start := frame*hop - fftSize/2
		for i := 0; i < fftSize; i++ {
			if p := start + i; p >= 0 && p < full {
				s.weight[p] += s.window[i] * s.window[i]
			}
		}
	}
	return s.weight
}
