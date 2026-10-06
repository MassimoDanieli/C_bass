package audio

import (
	"math"
	"sync"
)

// Repitch returns the buffer made shorter (factor above 1) or longer (below 1) by a factor,
// at the same sample rate: played back it is as many times higher, and as many times faster.
// Played back slowed down by the same factor it is higher at the speed it was, which is how
// a recording is moved to another key.
//
// It is the same windowed-sinc filtering as Resample with the filter worked out once, for
// 512 places between two samples, instead of at every sample: a recording of minutes is done
// in about a second.
func Repitch(b *Buffer, factor float64) *Buffer {
	if factor == 1 || b.Len() == 0 {
		return b
	}
	const (
		half   = 12 // samples of the recording on each side of the one being made
		places = 512
	)
	cutoff := math.Min(1, 1/factor) * 0.96 // nothing above half the rate it will be heard at
	table := make([][2 * half]float32, places)
	for p := range table {
		offset := float64(p) / places
		var sum float64
		var taps [2 * half]float64
		for j := range taps {
			x := float64(j-half+1) - offset
			taps[j] = cutoff * sinc(cutoff*x) * hann(x/half)
			sum += taps[j]
		}
		for j := range taps {
			table[p][j] = float32(taps[j] / sum)
		}
	}
	frames := int(math.Round(float64(b.Len()) / factor))
	step := float64(b.Len()) / float64(frames)
	out := &Buffer{SampleRate: b.SampleRate, Channels: make([][]float32, len(b.Channels))}
	var wait sync.WaitGroup
	for c, source := range b.Channels {
		target := make([]float32, frames)
		out.Channels[c] = target
		wait.Add(1)
		go func() {
			defer wait.Done()
			for i := range target {
				position := float64(i) * step
				centre := int(position)
				taps := &table[int((position-float64(centre))*places)]
				var sum float32
				if first := centre - half + 1; first >= 0 && centre+half < len(source) {
					for j, tap := range taps {
						sum += source[first+j] * tap
					}
				} else {
					for j, tap := range taps {
						if k := first + j; k >= 0 && k < len(source) {
							sum += source[k] * tap
						}
					}
				}
				target[i] = sum
			}
		}()
	}
	wait.Wait()
	return out
}
