// Package chords reads the harmony of a recording: which chord is sounding on every beat,
// from what is left of the recording once the bass is taken out, with the bass line itself
// saying which note is at the bottom.
package chords

import (
	"math"
	"math/cmplx"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// Chord is one chord of the piece, from a moment to another, in seconds.
type Chord struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	// Root is the pitch class of its root: 0 is C, 11 is B.
	Root int `json:"root"`
	// Quality is "" for major, "m", "7", "m7" or "maj7".
	Quality string `json:"quality"`
	// Edited marks a chord set by hand.
	Edited bool `json:"edited,omitempty"`
}

// Qualities are the kinds of chord told apart, in the order they are offered when one is
// changed by hand.
var Qualities = []string{"", "m", "7", "m7", "maj7"}

var shapes = map[string][]int{"": {0, 4, 7}, "m": {0, 3, 7}, "7": {0, 4, 7, 10}, "m7": {0, 3, 7, 10}, "maj7": {0, 4, 7, 11}}

var names = [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
var latin = [...]string{"Do", "Do#", "Re", "Re#", "Mi", "Fa", "Fa#", "Sol", "Sol#", "La", "La#", "Si"}

// Name writes the chord out: A7, Dm7; or in the Latin way: La7, Rem7.
func (c Chord) Name(inLatin bool) string {
	root := names[((c.Root%12)+12)%12]
	if inLatin {
		root = latin[((c.Root%12)+12)%12]
	}
	return root + c.Quality
}

const (
	rate   = 11025 // enough for the notes that make a chord
	size   = 4096  // 0.37 s: long enough to tell semitones apart down to the low register
	hop    = 1024
	lowest = 80.0 // Hz: under this is the bass and the bass drum
	top    = 2000.0
)

// fft transforms in place; the length must be a power of two.
func fft(a []complex128) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(length)))
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for k := 0; k < length/2; k++ {
				u, v := a[i+k], a[i+k+length/2]*w
				a[i+k], a[i+k+length/2] = u+v, u-v
				w *= step
			}
		}
	}
}

// chroma measures, for every stretch of a tenth of a second, how much of each of the twelve
// pitch classes is sounding.
func chroma(recording *audio.Buffer) [][12]float64 {
	step := float64(recording.SampleRate) / rate
	n := int(float64(recording.Len()) / step)
	mono := make([]float64, n)
	for i := range mono {
		from, to := int(float64(i)*step), int(float64(i+1)*step)
		var sum float64
		for j := from; j < to && j < recording.Len(); j++ {
			for _, channel := range recording.Channels {
				sum += float64(channel[j])
			}
		}
		mono[i] = sum / float64(max(1, to-from)*len(recording.Channels))
	}
	window := make([]float64, size)
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/size)
	}
	// which pitch class each bin of the spectrum belongs to, and how squarely it sits on it
	type place struct {
		class  int
		weight float64
	}
	places := make([]place, size/2)
	for bin := range places {
		hz := float64(bin) * rate / size
		if hz < lowest || hz > top {
			continue
		}
		pitch := 69 + 12*math.Log2(hz/440)
		off := pitch - math.Round(pitch)
		places[bin] = place{((int(math.Round(pitch)) % 12) + 12) % 12, math.Exp(-off * off / (2 * 0.2 * 0.2))}
	}
	frames := max(0, (n-size)/hop+1)
	out := make([][12]float64, frames)
	buffer := make([]complex128, size)
	for f := range out {
		for i := range buffer {
			buffer[i] = complex(mono[f*hop+i]*window[i], 0)
		}
		fft(buffer)
		for bin, p := range places {
			if p.weight > 0 {
				// the loud and the soft count less unequally than they measure: a chord is
				// its notes, not its loudest note
				out[f][p.class] += p.weight * math.Log1p(200*cmplx.Abs(buffer[bin])/size)
			}
		}
	}
	return out
}

type kind struct {
	root    int
	quality string
}

// Find reads the chords of a recording. The backing is the recording without its bass, the
// pulse its beats and bars, the bass the line as read: a chord changes on a beat, more
// willingly on the first of a bar, and its root is more likely the note the bass plays.
func Find(backing *audio.Buffer, pulse *rhythm.Rhythm, bass []transcribe.Event) []Chord {
	if backing == nil || pulse == nil || len(pulse.Beats) < 2 {
		return []Chord{}
	}
	frames := chroma(backing)
	beats := pulse.Beats
	count := len(beats) - 1
	// what sounds on each beat: the chroma of its frames, and the notes of the bass in it
	heard := make([][12]float64, count)
	low := make([][12]float64, count)
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
			heard[b][c] -= least // what every pitch class has alike is drums and noise
			sum += heard[b][c] * heard[b][c]
		}
		loud[b] = math.Sqrt(sum)
		if sum > 0 {
			for c := 0; c < 12; c++ {
				heard[b][c] /= math.Sqrt(sum)
			}
		}
	}
	first := func(b int) bool { // is this beat the first of its bar?
		page := float64(b - pulse.Downbeat)
		return pulse.BarStart(pulse.BarAt(page)) == int(page)
	}
	for _, note := range bass {
		for b := 0; b < count; b++ {
			overlap := math.Min(note.End, beats[b+1]) - math.Max(note.Start, beats[b])
			if overlap <= 0 {
				continue
			}
			weight := overlap / (beats[b+1] - beats[b])
			if note.Start >= beats[b]-0.05 && note.Start < beats[b]+0.08 {
				weight += 0.5 // the note struck on the beat says more than one passing through
			}
			low[b][((note.Midi%12)+12)%12] += weight
		}
	}
	var loudest float64
	for _, v := range loud {
		loudest = math.Max(loudest, v)
	}
	// The bass is part of the chord: a pianist leaves the root out because the bass has it.
	// Its notes join what was heard above, the one on the first beat of the bar most of all.
	for b := 0; b < count; b++ {
		var bottom, sum float64
		for c := 0; c < 12; c++ {
			bottom += low[b][c]
		}
		if bottom == 0 || loud[b] == 0 {
			continue
		}
		weight := 0.25
		if first(b) {
			weight = 0.7
		}
		for c := 0; c < 12; c++ {
			heard[b][c] += weight * low[b][c] / bottom
			sum += heard[b][c] * heard[b][c]
		}
		for c := 0; c < 12; c++ {
			heard[b][c] /= math.Sqrt(sum)
		}
	}

	// for every beat, the first beat of its bar
	opens := make([]int, count)
	for b := range opens {
		page := b - pulse.Downbeat
		opens[b] = pulse.BarStart(pulse.BarAt(float64(page))) + pulse.Downbeat
		if opens[b] < 0 || opens[b] >= count {
			opens[b] = -1
		}
	}
	var kinds []kind
	for root := 0; root < 12; root++ {
		for _, quality := range Qualities {
			kinds = append(kinds, kind{root, quality})
		}
	}
	silent := len(kinds) // one more state: nothing sounding
	fit := func(b int, k kind) float64 {
		shape := shapes[k.quality]
		var sum float64
		for _, interval := range shape {
			sum += heard[b][(k.root+interval)%12]
		}
		score := sum / math.Sqrt(float64(len(shape)))
		if len(shape) > 3 {
			score -= 0.06 // a seventh has to be heard to be written
		}
		var bottom float64
		for c := 0; c < 12; c++ {
			bottom += low[b][c]
		}
		if bottom > 0 {
			score += 0.1 * low[b][k.root] / bottom
		}
		// The bass plays the root where the bar starts, then walks: what it played on the
		// first beat speaks for the whole bar. It is what tells C minor seventh from the
		// E flat a pianist's hand plays over it.
		if opening := opens[b]; opening >= 0 {
			var bottom float64
			for c := 0; c < 12; c++ {
				bottom += low[opening][c]
			}
			if bottom > 0 {
				score += 0.3 * low[opening][k.root] / bottom
			}
		}
		return score
	}
	// the best path through the beats: staying costs nothing, changing costs something
	const change = 0.8
	best := make([][]float64, count)
	from := make([][]int, count)
	for b := 0; b < count; b++ {
		best[b] = make([]float64, len(kinds)+1)
		from[b] = make([]int, len(kinds)+1)
		// chords change with the bars: readily on the first beat, sometimes halfway through,
		// seldom anywhere else
		cost := change
		page := b - pulse.Downbeat
		bar := pulse.BarAt(float64(page))
		switch within, length := page-pulse.BarStart(bar), pulse.BeatsIn(bar); {
		case within == 0:
			cost = change * 0.12
		case length%2 == 0 && within == length/2:
			cost = change * 0.45
		}
		for s := range best[b] {
			here := 0.25 // what silence is worth: more than a chord that is not there
			if s != silent {
				here = fit(b, kinds[s])
				if loudest > 0 && loud[b] < loudest*0.05 {
					here = 0
				}
			}
			if b == 0 {
				best[b][s] = here
				continue
			}
			best[b][s], from[b][s] = math.Inf(-1), s
			for p := range best[b-1] {
				value := best[b-1][p]
				if p != s {
					value -= cost
				}
				if value > best[b][s] {
					best[b][s], from[b][s] = value, p
				}
			}
			best[b][s] += here
		}
	}
	if count == 0 {
		return []Chord{}
	}
	state := 0
	for s, v := range best[count-1] {
		if v > best[count-1][state] {
			state = s
		}
	}
	path := make([]int, count)
	for b := count - 1; b >= 0; b-- {
		path[b] = state
		state = from[b][state]
	}
	out := []Chord{}
	for b := 0; b < count; b++ {
		if path[b] == silent {
			continue
		}
		k := kinds[path[b]]
		if n := len(out); n > 0 && out[n-1].Root == k.root && out[n-1].Quality == k.quality && math.Abs(out[n-1].End-beats[b]) < 1e-6 {
			out[n-1].End = beats[b+1]
			continue
		}
		out = append(out, Chord{Start: beats[b], End: beats[b+1], Root: k.root, Quality: k.quality})
	}
	return out
}

// At is the chord sounding at a moment, or nil.
func At(list []Chord, seconds float64) *Chord {
	for i := range list {
		if seconds >= list[i].Start && seconds < list[i].End {
			return &list[i]
		}
	}
	return nil
}
