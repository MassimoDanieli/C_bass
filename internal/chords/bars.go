package chords

import (
	"fmt"
	"math"
	"os"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// FirstBeat says which of the beats the bars start on. The pulse comes with a guess made from
// the drums alone, which in four tells the strong beats from the weak but often takes the
// third for the first: the bass drum plays both alike. The harmony does not: chords change,
// and the bass moves to a new note, where a bar starts far more than halfway through it. So
// between the guess and the beat half a bar away, the one where more changes is the first.
func FirstBeat(backing *audio.Buffer, pulse *rhythm.Rhythm, bass []transcribe.Event) int {
	if backing == nil || pulse == nil || pulse.PerBar < 2 || pulse.PerBar%2 != 0 || len(pulse.Beats) < 4*pulse.PerBar {
		return pulse.Downbeat
	}
	beats := pulse.Beats
	heard, loud := onBeats(chroma(backing), beats)
	count := len(heard)
	// the note the bass strikes on each beat, if it strikes one
	struck := make([]int, count)
	for b := range struck {
		struck[b] = -1
	}
	for _, note := range bass {
		for b := 0; b < count; b++ {
			if note.Start >= beats[b]-0.06 && note.Start < beats[b]+0.09 {
				struck[b] = ((note.Midi % 12) + 12) % 12
			}
		}
	}
	half := pulse.PerBar / 2
	changes := make([]float64, pulse.PerBar)
	seen := make([]float64, pulse.PerBar)
	for b := half; b+half <= count; b++ {
		var before, after [12]float64
		for k := 0; k < half; k++ {
			for c := 0; c < 12; c++ {
				before[c] += heard[b-1-k][c] * loud[b-1-k]
				after[c] += heard[b+k][c] * loud[b+k]
			}
		}
		var dot, n1, n2 float64
		for c := 0; c < 12; c++ {
			dot += before[c] * after[c]
			n1 += before[c] * before[c]
			n2 += after[c] * after[c]
		}
		change := 0.0
		if n1 > 0 && n2 > 0 {
			change = 1 - dot/math.Sqrt(n1*n2)
		}
		// the bass on a new note, against the one it struck half a bar before
		if struck[b] >= 0 && struck[b-half] >= 0 && struck[b] != struck[b-half] {
			change += bassWeight
		}
		phase := b % pulse.PerBar
		changes[phase] += change
		seen[phase]++
	}
	for i := range changes {
		if seen[i] > 0 {
			changes[i] /= seen[i]
		}
	}
	guess := ((pulse.Downbeat % pulse.PerBar) + pulse.PerBar) % pulse.PerBar
	other := (guess + half) % pulse.PerBar
	if os.Getenv("CBASS_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "first beat: guess %d changes %.3f, other %d changes %.3f\n", guess, changes[guess], other, changes[other])
	}
	if changes[other] > changes[guess]*clearly {
		return other
	}
	return guess
}

const (
	bassWeight = 0.15
	clearly    = 1.15 // how much more must change on the other beat to move the bars there
)

// onBeats is what sounds on each beat: the chroma of its frames with what every pitch class
// has alike taken away (that is drums and noise), brought to one length; and how loud it was.
func onBeats(frames [][12]float64, beats []float64) ([][12]float64, []float64) {
	count := len(beats) - 1
	heard := make([][12]float64, count)
	loud := make([]float64, count)
	for b := 0; b < count; b++ {
		// the frames whose middle falls in the beat: a frame is longer than its step, and
		// taken by its start it would hear the next chord coming
		from := int(math.Ceil((beats[b]*rate - size/2) / hop))
		to := int(math.Ceil((beats[b+1]*rate - size/2) / hop))
		for f := max(0, from); f < to && f < len(frames); f++ {
			for c := 0; c < 12; c++ {
				heard[b][c] += frames[f][c]
			}
		}
		var least, sum float64 = math.Inf(1), 0
		for c := 0; c < 12; c++ {
			least = math.Min(least, heard[b][c])
		}
		for c := 0; c < 12; c++ {
			heard[b][c] -= least
			sum += heard[b][c] * heard[b][c]
		}
		loud[b] = math.Sqrt(sum)
		if sum > 0 {
			for c := 0; c < 12; c++ {
				heard[b][c] /= math.Sqrt(sum)
			}
		}
	}
	return heard, loud
}
