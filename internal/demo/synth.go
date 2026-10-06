// Package demo holds the recordings that come with the program: short pieces in five styles,
// written for it, with the bass and the rest already apart. They let the window be tried
// before any recording of one's own is analysed, and give something to practise on.
package demo

import (
	"math"
	"math/rand/v2"

	"github.com/MassimoDanieli/c_bass/internal/audio"
)

// Rate of everything made here.
const Rate = 44100

// track is a stereo recording being built.
type track struct {
	left, right []float32
}

func newTrack(seconds float64) *track {
	n := int(seconds * Rate)
	return &track{make([]float32, n), make([]float32, n)}
}

func (t *track) buffer() *audio.Buffer {
	return &audio.Buffer{SampleRate: Rate, Channels: [][]float32{t.left, t.right}}
}

// add mixes a sound in at a time, panned from -1 (left) to 1 (right).
func (t *track) add(at float64, sound []float32, pan float32) {
	start := int(at * Rate)
	l, r := float32(math.Sqrt(float64(1-pan)/2)), float32(math.Sqrt(float64(1+pan)/2))
	for i, v := range sound {
		if j := start + i; j >= 0 && j < len(t.left) {
			t.left[j] += v * l
			t.right[j] += v * r
		}
	}
}

// Bass is a plucked string: a Karplus-Strong loop, as a bass guitar sounds in outline. Dead
// notes are the string struck with the hand on it.
func Bass(midi int, seconds, level float64, dead bool, seed uint64) []float32 {
	rng := rand.New(rand.NewPCG(seed, 7))
	frequency := 440 * math.Pow(2, float64(midi-69)/12)
	period := Rate / frequency
	n := int(period)
	frac := period - float64(n)
	ring := make([]float64, n+1)
	for i := range ring {
		ring[i] = rng.Float64()*2 - 1
	}
	// a softer pluck: the noise smoothed, so there is less fizz and more string
	for pass := 0; pass < 3; pass++ {
		prev := ring[len(ring)-1]
		for i := range ring {
			cur := ring[i]
			ring[i] = 0.5 * (cur + prev)
			prev = cur
		}
	}
	damp := 0.9965
	if dead {
		damp = 0.80
	}
	out := make([]float32, int(seconds*Rate))
	k := 0
	var last float64
	for i := range out {
		next := ring[(k+1)%len(ring)]
		// a fractional delay keeps the pitch true
		v := (1-frac)*ring[k] + frac*next
		ring[k] = damp * 0.5 * (ring[k] + next)
		// a little body: the string's own low-pass
		last += 0.6 * (v - last)
		out[i] = float32(last * level)
		k = (k + 1) % len(ring)
	}
	shape(out, 0.003, 0.015)
	return out
}

// shape gives a sound a quick rise and a clean end.
func shape(out []float32, attack, release float64) {
	a, r := int(attack*Rate), int(release*Rate)
	for i := range out {
		g := 1.0
		if i < a {
			g = float64(i) / float64(a)
		}
		if left := len(out) - i; left < r {
			g *= float64(left) / float64(r)
		}
		out[i] = float32(float64(out[i]) * g)
	}
}

func kick(level float64) []float32 {
	out := make([]float32, int(0.25*Rate))
	for i := range out {
		t := float64(i) / Rate
		f := 50 + 90*math.Exp(-t*35)
		out[i] = float32(level * math.Exp(-t*18) * math.Sin(2*math.Pi*f*t))
	}
	shape(out, 0.001, 0.02)
	return out
}

func snare(level float64, rng *rand.Rand) []float32 {
	out := make([]float32, int(0.18*Rate))
	for i := range out {
		t := float64(i) / Rate
		noise := rng.Float64()*2 - 1
		out[i] = float32(level * (0.7*math.Exp(-t*22)*noise + 0.5*math.Exp(-t*30)*math.Sin(2*math.Pi*190*t)))
	}
	shape(out, 0.001, 0.02)
	return out
}

func hat(level float64, open bool, rng *rand.Rand) []float32 {
	length := 0.05
	decay := 90.0
	if open {
		length, decay = 0.3, 14
	}
	out := make([]float32, int(length*Rate))
	var high, prev float64
	for i := range out {
		t := float64(i) / Rate
		noise := rng.Float64()*2 - 1
		high = 0.7 * (high + noise - prev) // only the top of the noise: a cymbal, not a snare
		prev = noise
		out[i] = float32(level * math.Exp(-t*decay) * high)
	}
	shape(out, 0.001, 0.01)
	return out
}

func rim(level float64) []float32 {
	out := make([]float32, int(0.06*Rate))
	for i := range out {
		t := float64(i) / Rate
		out[i] = float32(level * math.Exp(-t*60) * (math.Sin(2*math.Pi*820*t) + 0.5*math.Sin(2*math.Pi*1700*t)))
	}
	shape(out, 0.001, 0.01)
	return out
}

func ride(level float64, rng *rand.Rand) []float32 {
	out := make([]float32, int(0.5*Rate))
	for i := range out {
		t := float64(i) / Rate
		noise := rng.Float64()*2 - 1
		out[i] = float32(level * math.Exp(-t*7) * (0.3*noise + 0.5*math.Sin(2*math.Pi*2900*t) + 0.3*math.Sin(2*math.Pi*4300*t)))
	}
	shape(out, 0.001, 0.02)
	return out
}

// Pad is a soft chord held for a while: an electric piano in outline, so the harmony is heard.
func Pad(midis []int, seconds, level float64) []float32 {
	out := make([]float32, int(seconds*Rate))
	for _, midi := range midis {
		f := 440 * math.Pow(2, float64(midi-69)/12)
		for i := range out {
			t := float64(i) / Rate
			env := math.Exp(-t*1.2) * math.Min(1, t/0.01)
			out[i] += float32(level * env * (math.Sin(2*math.Pi*f*t) + 0.25*math.Sin(4*math.Pi*f*t)*math.Exp(-t*4) + 0.08*math.Sin(6*math.Pi*f*t)))
		}
	}
	shape(out, 0.005, 0.05)
	return out
}
