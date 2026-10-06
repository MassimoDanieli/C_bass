// Package transcribe reads a bass line out of a recording: which notes, when, and for how long.
//
// It is the Go version of the transcriber of Manico (https://github.com/MassimoDanieli/accordi_di_basso).
package transcribe

import (
	"fmt"
	"math"
	"sort"

	"github.com/MassimoDanieli/c_bass/internal/audio"
)

// analysisRate is the sample rate the analysis works at: enough for the bass register.
const analysisRate = 5512

// Event is one note of the line.
type Event struct {
	ID         string  `json:"id"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Midi       int     `json:"midi"`
	RawMidi    int     `json:"rawMidi"`
	Confidence float64 `json:"confidence"`
	// String and Fret say where to play it; String is -1 when the note is off the fretboard.
	String int  `json:"string"`
	Fret   int  `json:"fret"`
	Locked bool `json:"lockedPosition,omitempty"`
	// Sure marks a note whose octave was read from the whole of it, clearly: it is not to be
	// second-guessed from the notes around it. Octaves played in turn are a bass line, not a
	// misreading to smooth away.
	Sure   bool `json:"-"`
	Edited bool `json:"edited,omitempty"`
}

// Options for Transcribe.
type Options struct {
	// Isolated says the recording is a bass on its own, as separated by Demucs.
	Isolated bool
	// Sensitivity to new attacks, from 0.55 (fewer notes) to 0.90 (more); 0.72 if zero.
	Sensitivity float64
	// Lowest is the lowest open string of the instrument, as a MIDI note: 28 for a four-string
	// bass, which is taken if zero. Nothing more than a tone below it is read (a string tuned
	// down is common, a note the instrument does not have is a misreading of its octave).
	Lowest int
	// Floor is a level, as measured by Level, below which nothing is taken for a note. A
	// bass separated from a recording that has none is not silence but a faint noise, and
	// measured only against itself the noise would be read as notes.
	Floor float64
	// Progress, if set, is called with a value from 0 to 1.
	Progress func(float64)
}

// Level is how loud a recording is where it is loud, in the register of the bass: what the
// level of its separated bass can be held against.
func Level(buffer *audio.Buffer) float64 {
	signal, rate := Prepare(buffer)
	return loudLevel(smoothLevel(signal, rate), rate)
}

// loudLevel is the level reached by the loudest twentieth of a recording, sampled a hundred
// times a second.
func loudLevel(level []float64, rate int) float64 {
	hop := int(math.Round(float64(rate) * 0.01))
	env := make([]float64, 0, len(level)/hop+1)
	for i := 0; i < len(level); i += hop {
		env = append(env, level[i])
	}
	return percentile(env, 0.95)
}

// Transcribe returns the notes of a recording, in order, with octave slips corrected.
func Transcribe(buffer *audio.Buffer, options Options) []Event {
	if options.Sensitivity == 0 {
		options.Sensitivity = 0.72
	}
	if options.Progress == nil {
		options.Progress = func(float64) {}
	}
	signal, rate := Prepare(buffer)
	var events []Event
	if options.Isolated {
		lowest := options.Lowest
		if lowest == 0 {
			lowest = 28
		}
		events = isolatedNotes(signal, rate, options.Sensitivity, options.Floor, lowest-2, options.Progress)
	} else {
		events = mixNotes(signal, rate, options.Sensitivity, buffer.Duration(), options.Progress)
	}
	return StabilizeOctaves(Normalize(dedupe(events), buffer.Duration()))
}

// Prepare reduces a recording to the mono, band-limited signal the analysis works on:
// everything below 30 Hz and above 360 Hz is filtered out, at 5512 samples per second.
func Prepare(buffer *audio.Buffer) ([]float32, int) {
	ratio := float64(buffer.SampleRate) / analysisRate
	frames := buffer.Len()
	length := int(math.Max(1, math.Floor(float64(frames)/ratio)))
	out := make([]float32, length)
	dt := 1 / float64(buffer.SampleRate)
	highRC := 1 / (2 * math.Pi * 30)
	highAlpha := highRC / (highRC + dt)
	lowAlpha := 1 - math.Exp(-2*math.Pi*360/float64(buffer.SampleRate))
	var high, previous, low float64
	channels := float64(len(buffer.Channels))
	for i := 0; i < length; i++ {
		from := int(math.Floor(float64(i) * ratio))
		to := int(math.Max(float64(from+1), math.Floor(float64(i+1)*ratio)))
		var sum float64
		count := 0
		for j := from; j < to && j < frames; j++ {
			var sample float64
			for _, channel := range buffer.Channels {
				sample += float64(channel[j])
			}
			sample /= channels
			high = highAlpha * (high + sample - previous)
			previous = sample
			low += lowAlpha * (high - low)
			sum += low
			count++
		}
		if count > 0 {
			out[i] = float32(sum / float64(count))
		}
	}
	return out, analysisRate
}

func clamp(v, low, high float64) float64 { return math.Max(low, math.Min(high, v)) }

func percentile(values []float64, ratio float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Floor(float64(len(sorted)-1) * ratio))
	return sorted[max(0, min(len(sorted)-1, index))]
}

// smoothLevel is how loud the signal is from moment to moment: rectified and low-passed at
// 22 Hz in both directions, so that it neither ripples with the note nor lags behind it.
func smoothLevel(signal []float32, rate int) []float64 {
	k := 1 - math.Exp(-2*math.Pi*22/float64(rate))
	out := make([]float64, len(signal))
	var a, b float64
	for i, v := range signal {
		a += k * (math.Abs(float64(v)) - a)
		b += k * (a - b)
		out[i] = b
	}
	a, b = 0, 0
	for i := len(signal) - 1; i >= 0; i-- {
		a += k * (out[i] - a)
		b += k * (a - b)
		out[i] = b
	}
	return out
}

type pitched struct {
	midi       int
	confidence float64
	exact      float64 // the pitch between semitones, where it was measured that finely
}

// octaveMargin is how much worse a shorter period may fit and still be taken for the
// fundamental. Twice the true period always fits as well as the period itself, so the
// shortest good one is the note. But half the period fits nearly as well when the second
// harmonic is strong, as on the lowest strings, and then the note is read an octave up: the
// margin cannot be made safe against both, so a single frame is not trusted with the octave.
// The pitch of a note is read from all its frames together, where the margin is safe.
const octaveMargin = 0.08

// fundamental picks the period in a curve of how well each lag fits: the shortest lag that
// peaks within the margin of the best. It returns the lag refined between samples.
func fundamental(scores []float64, minimumLag, maximumLag int, margin float64) (float64, float64, bool) {
	bestLag, bestScore := -1, -1.0
	for lag := minimumLag; lag <= maximumLag; lag++ {
		if scores[lag] > bestScore {
			bestScore, bestLag = scores[lag], lag
		}
	}
	if bestScore < 0.5 {
		return 0, 0, false
	}
	chosen := bestLag
	strong := math.Max(0.6, bestScore-margin)
	for lag := minimumLag + 1; lag < bestLag; lag++ {
		if scores[lag] >= strong && scores[lag] >= scores[lag-1] && scores[lag] >= scores[lag+1] {
			chosen = lag
			break
		}
	}
	// The top of the peak lies between samples: at the high notes a whole sample is most of a semitone.
	exact := float64(chosen)
	if chosen > minimumLag && chosen < maximumLag {
		before, at, after := scores[chosen-1], scores[chosen], scores[chosen+1]
		if curve := before - 2*at + after; curve < 0 {
			exact += clamp(0.5*(before-after)/curve, -0.5, 0.5)
		}
	}
	return exact, scores[chosen], true
}

// pitchOfLag is the pitch of a period, in semitones on the MIDI scale and fractions of one.
func pitchOfLag(lag float64, rate int) float64 {
	return 69 + 12*math.Log2(float64(rate)/lag/440)
}

// framePitch finds the pitch of a short window by normalised autocorrelation. The curve of
// how well every lag fits is left in scores.
func framePitch(signal []float32, rate, start, size, minimumLag, maximumLag int, scores []float64) (pitched, bool) {
	if start < 0 || start+size+maximumLag > len(signal) {
		return pitched{}, false
	}
	var base float64
	for i := start; i < start+size; i++ {
		base += float64(signal[i]) * float64(signal[i])
	}
	if base <= 0 {
		return pitched{}, false
	}
	energy := base
	window := signal[start : start+size]
	for lag := 1; lag <= maximumLag; lag++ {
		in, gone := float64(signal[start+size+lag-1]), float64(signal[start+lag-1])
		energy += in*in - gone*gone
		if lag < minimumLag {
			continue
		}
		shifted := signal[start+lag : start+lag+size]
		var xy float64
		for i, v := range window {
			xy += float64(v) * float64(shifted[i])
		}
		scores[lag] = xy / math.Sqrt(base*energy+1e-20)
	}
	lag, score, ok := fundamental(scores, minimumLag, maximumLag, octaveMargin)
	if !ok {
		return pitched{}, false
	}
	exact := pitchOfLag(lag, rate)
	midi := int(math.Round(exact))
	if midi < 23 || midi > 76 {
		return pitched{}, false
	}
	return pitched{midi, score, exact}, true
}

// tuningOf says how far the recording sits from concert pitch, in semitones between -0.5 and
// 0.5: where its notes fall between the semitones, if they agree on it. A recording a quarter
// tone flat would otherwise have every note named by the toss of a coin.
func tuningOf(pitch, sure []float64) float64 {
	var x, y, weight float64
	for i, p := range pitch {
		if p == 0 || sure[i] < 0.9 {
			continue
		}
		angle := 2 * math.Pi * (p - math.Round(p))
		x += math.Cos(angle)
		y += math.Sin(angle)
		weight++
	}
	if weight < 50 || math.Hypot(x, y)/weight < 0.5 {
		return 0 // too little to go on, or no agreement: a fretless played freely
	}
	return math.Atan2(y, x) / (2 * math.Pi)
}

// isolatedNotes follows the note of a bass on its own instead of looking for bursts of energy.
// A held note keeps its level; a plucked one dips and comes back within a few hundredths of a
// second; a slurred one changes pitch without either. Those three are told apart here.
func isolatedNotes(signal []float32, rate int, sensitivity, floor float64, lowest int, report func(float64)) []Event {
	hop := int(math.Round(float64(rate) * 0.01))
	fps := float64(rate) / float64(hop)
	count := len(signal) / hop
	level := smoothLevel(signal, rate)
	env := make([]float64, count)
	for i := range env {
		env[i] = level[i*hop]
	}
	reference := percentile(env, 0.95)
	if reference == 0 {
		reference = 1e-9
	}
	gate := math.Max(reference*0.07, floor)
	active := func(i int) bool { return env[i] > gate }

	// How much the level must come back up, within five hundredths of a second, to count as a new attack.
	threshold := clamp(1.15+(0.72-sensitivity)*0.5, 1.04, 1.5)
	const reach = 5
	rise := make([]float64, count)
	for i := range rise {
		rise[i] = 1
	}
	for i := 0; i+reach < count; i++ {
		rise[i] = env[i+reach] / (env[i] + reference*0.02)
	}
	var attacks []int
	for i := 3; i < count-reach-3; i++ {
		if rise[i] < threshold || env[i+reach] <= gate {
			continue
		}
		top := true
		for j := i - 3; j <= i+3; j++ {
			if rise[j] > rise[i] {
				top = false
			}
		}
		if !top {
			continue
		}
		if last := len(attacks) - 1; last >= 0 && i-attacks[last] < 5 {
			if rise[i] > rise[attacks[last]] {
				attacks[last] = i
			}
		} else {
			attacks = append(attacks, i)
		}
	}
	bounds := make(map[int]bool, len(attacks))
	struck := make(map[int]bool, len(attacks))
	for _, i := range attacks {
		bounds[i+2] = true
		struck[i+2] = true
	}

	size := int(math.Round(float64(rate) * 0.085))
	minimumLag := max(2, rate/330)
	// the longest period looked for: half a semitone below the lowest note, and never under 31 Hz
	maximumLag := min(rate/31, int(float64(rate)/(440*math.Pow(2, (float64(lowest)-0.5-69)/12))))
	scores := make([]float64, maximumLag+2)
	raw := make([]float64, count) // the pitch of each frame, between semitones; 0 where there is none
	sure := make([]float64, count)
	curves := make([][]float32, count) // how well every lag fits, for the frames with a pitch
	for i := 0; i < count; i++ {
		if active(i) {
			if found, ok := framePitch(signal, rate, i*hop-size/2, size, minimumLag, maximumLag, scores); ok {
				raw[i], sure[i] = found.exact, found.confidence
				curve := make([]float32, maximumLag+1)
				for lag := minimumLag; lag <= maximumLag; lag++ {
					curve[lag] = float32(scores[lag])
				}
				curves[i] = curve
			}
		}
		if i%400 == 0 {
			report(float64(i) / float64(count))
		}
	}
	tuning := tuningOf(raw, sure)
	// The median of five frames steadies the pitch.
	pitch := make([]float64, count)
	near := make([]float64, 0, 5)
	for i := 0; i < count; i++ {
		near = near[:0]
		for j := max(0, i-2); j <= min(count-1, i+2); j++ {
			if raw[j] != 0 {
				near = append(near, raw[j])
			}
		}
		if len(near) >= 3 {
			sort.Float64s(near)
			if len(near)%2 == 1 {
				pitch[i] = near[len(near)/2]
			} else {
				pitch[i] = (near[len(near)/2-1] + near[len(near)/2]) / 2
			}
		}
	}
	// A new note starts where the pitch moves away from where the note has been sitting and
	// settles somewhere else for four hundredths (twice that for an octave, the usual
	// misreading). The pitch is followed between semitones: a note played a little flat, as on
	// a fretless or on a recording not tuned to 440, would otherwise flicker between two
	// names; and a slide passes through without leaving a note at every fret.
	const away, settled = 0.7, 0.4
	current, held, run, candidate := 0.0, 0, 0, 0.0
	var bottom, top float64
	for i := 0; i < count; i++ {
		p := pitch[i]
		if !active(i) {
			current, held, run = 0, 0, 0
			continue
		}
		if p == 0 {
			continue
		}
		if bounds[i] || current == 0 {
			current, held, run = p, 1, 0
			continue
		}
		if math.Abs(p-current) <= away {
			run = 0
			held = min(held+1, 30)
			current += (p - current) / float64(held)
			continue
		}
		// settled: the frames of the run all within a narrow band, which a pitch on its way
		// somewhere else never is
		if run > 0 && math.Max(top, p)-math.Min(bottom, p) <= settled {
			run++
			candidate += (p - candidate) / float64(run)
			bottom, top = math.Min(bottom, p), math.Max(top, p)
		} else {
			run, candidate, bottom, top = 1, p, p, p
		}
		need := 4
		if octaves := math.Abs(candidate-current) / 12; math.Abs(octaves-math.Round(octaves)) < 0.05 {
			need = 8
		}
		if run >= need {
			bounds[i-run+1] = true
			current, held, run = candidate, run, 0
		}
	}

	// The stretches between one boundary and the next.
	type piece struct {
		from, to   int
		midi       int
		confidence float64
		struck     bool // it starts with an attack, or after a silence
	}
	var notes []piece
	for i := 0; i < count; {
		if !active(i) {
			i++
			continue
		}
		j := i
		for j < count && active(j) {
			j++
		}
		from := i
		for k := i + 1; k <= j; k++ {
			if k == j || bounds[k] {
				notes = append(notes, piece{from: from, to: k, struck: from == i || struck[from]})
				from = k
			}
		}
		i = j
	}
	// The pitch of a stretch is read from the whole of it at once: the fit of every lag is
	// averaged over its frames before the period is chosen. A frame on its own can be fooled,
	// above all between an octave and the next; the average of a note cannot so easily.
	mean := make([]float64, maximumLag+2)
	var within []float64
	pitchOf := func(from, to int) (int, float64) {
		// A frame looks 40 ms each way: near the ends of the stretch it sees the neighbours
		// too. The ends are left out, as far as the stretch is long enough to spare them.
		edge := min(4, (to-from)/3)
		first, last := from+edge, to-edge
		// Nor do the frames on their way to another pitch count, in a slide or a bend: only
		// those that sit where most of the stretch sits, in whatever octave they were read.
		within = within[:0]
		for i := first; i < last; i++ {
			if raw[i] != 0 {
				within = append(within, raw[i])
			}
		}
		if len(within) == 0 {
			return 0, 0
		}
		sort.Float64s(within)
		middle := within[len(within)/2]
		for lag := range mean {
			mean[lag] = 0
		}
		frames := 0
		for i := first; i < last; i++ {
			if curves[i] == nil {
				continue
			}
			if off := math.Abs(raw[i] - middle); math.Abs(off-12*math.Round(off/12)) > away {
				continue
			}
			for lag := minimumLag; lag <= maximumLag; lag++ {
				mean[lag] += float64(curves[i][lag])
			}
			frames++
		}
		for lag := minimumLag; lag <= maximumLag; lag++ {
			mean[lag] /= float64(frames)
		}
		lag, score, ok := fundamental(mean, minimumLag, maximumLag, octaveMargin)
		if !ok {
			return 0, 0
		}
		midi := int(math.Round(pitchOfLag(lag, rate) - tuning))
		if midi < 23 || midi > 76 {
			return 0, 0
		}
		return midi, score
	}
	for k := range notes {
		notes[k].midi, notes[k].confidence = pitchOf(notes[k].from, notes[k].to)
	}

	// Boundaries that are not the start of a note are taken out, until none is left.
	const (
		fragment = 5  // hundredths: too short to be anything
		short    = 8  // too short to be a note unless it is clearly one
		blur     = 10 // how long the start of a note can take to settle on its pitch
		release  = 12 // how long its end can take to die away
		steady   = 30 // long enough for what was read to be what was played
	)
	length := func(p piece) int { return p.to - p.from }
	join := func(k int, midi int, confidence float64) { // k and k+1 become one
		notes[k] = piece{notes[k].from, notes[k+1].to, midi, confidence, notes[k].struck || (length(notes[k]) < blur && notes[k+1].struck)}
		notes = append(notes[:k+1], notes[k+2:]...)
	}
	for changed := true; changed; {
		changed = false
		for k := 0; k+1 < len(notes); k++ {
			a, b := notes[k], notes[k+1]
			if a.to != b.from {
				continue
			}
			lastOfRun := k+2 >= len(notes) || notes[k+2].from != b.to
			switch {
			case b.midi == 0:
				// no pitch: the note before rings on
				join(k, a.midi, a.confidence)
			case a.midi == 0:
				join(k, b.midi, b.confidence)
			case a.midi == b.midi && (!b.struck || length(a) < short):
				// the same note, and nothing struck in between: the two halves of one attack
				midi, confidence := pitchOf(a.from, b.to)
				if midi != a.midi {
					midi, confidence = a.midi, math.Max(a.confidence, b.confidence)
				}
				join(k, midi, confidence)
			case pitchClass(a.midi) == pitchClass(b.midi) && !b.struck:
				// The same note an octave away, and nothing struck in between: one note. No
				// hand jumps an octave without plucking; it is the sound that changes. A string
				// just struck can rattle so that every other wave differs, and looks an octave
				// lower; left ringing it loses its fundamental, and looks an octave higher. Sound
				// at the lower octave is evidence, its absence is not: where the lower one held
				// for a good while it is the note. Otherwise the note is what fits the whole.
				midi, confidence := pitchOf(a.from, b.to)
				low := a
				if b.midi < a.midi {
					low = b
				}
				switch {
				case length(low) >= steady:
					// long enough to be no accident: the lower octave was really there
					midi, confidence = low.midi, low.confidence
				case pitchClass(midi) != pitchClass(a.midi):
					midi, confidence = a.midi, a.confidence
					if length(b) > length(a) {
						midi, confidence = b.midi, b.confidence
					}
				}
				join(k, midi, confidence)
			case length(a) < fragment || (length(a) < blur && !b.struck && a.confidence < 0.85 && length(b) >= 2*length(a)):
				// the blur at the start of the next note: its pitch settles without anything
				// being struck again. A short note before a struck one is a note.
				join(k, b.midi, b.confidence)
			case (length(b) < fragment && (!b.struck || lastOfRun)) || (!b.struck && lastOfRun && (length(b) < short || (length(b) < release && b.confidence < 0.9))):
				// the pitch drifting as the note dies away
				join(k, a.midi, a.confidence)
			default:
				continue
			}
			changed = true
			k--
		}
	}
	events := make([]Event, 0, len(notes))
	for k, note := range notes {
		alone := !(k > 0 && notes[k-1].to == note.from) && !(k+1 < len(notes) && notes[k+1].from == note.to)
		if note.midi == 0 || length(note) < fragment || (alone && length(note) < short) {
			continue // a blip on its own in silence
		}
		events = append(events, Event{
			Start: float64(note.from) / fps, End: float64(note.to) / fps,
			Midi: note.midi, RawMidi: note.midi, Confidence: clamp(note.confidence, 0, 1), String: -1,
			Sure: note.confidence >= 0.75 && length(note) >= short,
		})
	}
	return events
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// --- A full mix: the bass has to be found among everything else, so notes are looked for
// where the low register gains energy, and each one's pitch is voted by a few short windows.

func rms(signal []float32, start, length int) float64 {
	end := min(len(signal), start+length)
	var sum float64
	for i := start; i < end; i++ {
		sum += float64(signal[i]) * float64(signal[i])
	}
	return math.Sqrt(sum / math.Max(1, float64(end-start)))
}

func onsets(signal []float32, rate int, sensitivity float64) []float64 {
	hop := max(1, int(math.Round(float64(rate)*0.01)))
	window := max(hop*3, int(math.Round(float64(rate)*0.04)))
	var energy []float64
	for position := 0; position+window < len(signal); position += hop {
		energy = append(energy, rms(signal, position, window))
	}
	noise, strong := percentile(energy, 0.32), percentile(energy, 0.91)
	threshold := noise + (strong-noise)*(1-sensitivity)*0.68
	flux := make([]float64, len(energy))
	var positive []float64
	for i := range energy {
		if i >= 2 {
			flux[i] = energy[i] - math.Max(energy[i-1], energy[i-2])
		}
		if flux[i] > 0 {
			positive = append(positive, flux[i])
		}
	}
	fluxThreshold := math.Max(0.000003, percentile(positive, 0.64)*(1.05-sensitivity*0.34))
	gap := max(1, int(math.Round(0.052*float64(rate)/float64(hop))))
	var result []float64
	last := -gap
	for i := 2; i < len(energy)-2; i++ {
		local := flux[i] >= flux[i-1] && flux[i] >= flux[i+1]
		if local && energy[i] > threshold && flux[i] > fluxThreshold && i-last >= gap {
			result = append(result, float64(i*hop)/float64(rate))
			last = i
		}
	}
	if len(result) == 0 || result[0] > 0.18 {
		result = append([]float64{0}, result...)
	}
	return result
}

func correlation(signal []float32, start, size, lag int) float64 {
	var xy, xx, yy float64
	end := min(len(signal), start+size-lag)
	for i := start; i < end; i++ {
		left, right := float64(signal[i]), float64(signal[i+lag])
		xy += left * right
		xx += left * left
		yy += right * right
	}
	if d := math.Sqrt(xx * yy); d != 0 {
		return xy / d
	}
	return xy
}

func estimateWindow(signal []float32, rate, start, size int) (pitched, bool) {
	minimumLag := max(2, rate/330)
	maximumLag := min(rate/31, size/2)
	bestLag, bestScore := -1, -1.0
	scores := make([]float64, maximumLag+1)
	for lag := minimumLag; lag <= maximumLag; lag++ {
		score := correlation(signal, start, size, lag)
		scores[lag] = score
		if score > bestScore {
			bestScore, bestLag = score, lag
		}
	}
	if bestLag < 0 || bestScore < 0.47 {
		return pitched{}, false
	}
	chosen := bestLag
	strong := math.Max(0.58, bestScore*0.91)
	for lag := minimumLag + 1; lag < bestLag; lag++ {
		if scores[lag] >= strong && scores[lag] >= scores[lag-1] && scores[lag] >= scores[lag+1] {
			chosen, bestScore = lag, scores[lag]
			break
		}
	}
	midi := int(math.Round(69 + 12*math.Log2(float64(rate)/float64(chosen)/440)))
	if midi < 23 || midi > 76 {
		return pitched{}, false
	}
	return pitched{midi: midi, confidence: bestScore}, true
}

// analysisOffsets are the places inside a note, as fractions of its length, where its pitch is read.
func analysisOffsets(start, end float64) []float64 {
	span := math.Max(0.055, end-start)
	var out []float64
	for _, ratio := range []float64{0.14, 0.34, 0.58, 0.8} {
		value := math.Min(span-0.018, math.Max(0.012, span*ratio))
		if value > 0 && (len(out) == 0 || value-out[len(out)-1] >= 0.012) {
			out = append(out, value)
		}
	}
	return out
}

// selectVotes picks the pitch most windows agree on, weighing agreement above a single confident outlier.
func selectVotes(votes []pitched) pitched {
	type group struct {
		score float64
		count int
	}
	groups := map[int]*group{}
	var order []int
	for _, vote := range votes {
		g := groups[vote.midi]
		if g == nil {
			g = &group{}
			groups[vote.midi] = g
			order = append(order, vote.midi)
		}
		g.score += vote.confidence
		g.count++
	}
	best, bestRank, bestConfidence := 0, math.Inf(-1), 0.0
	for _, midi := range order {
		g := groups[midi]
		confidence := g.score / float64(g.count)
		rank := float64(g.count)*0.32 + confidence
		if rank > bestRank || (rank == bestRank && confidence > bestConfidence) {
			best, bestRank, bestConfidence = midi, rank, confidence
		}
	}
	return pitched{midi: best, confidence: bestConfidence}
}

func mixNotes(signal []float32, rate int, sensitivity, duration float64, report func(float64)) []Event {
	points := onsets(signal, rate, sensitivity)
	var events []Event
	for i, start := range points {
		next := math.Min(duration, start+0.72)
		if i+1 < len(points) {
			next = points[i+1]
		}
		var votes []pitched
		for _, offset := range analysisOffsets(start, next) {
			from := max(0, int(math.Floor((start+offset)*float64(rate))))
			remaining := math.Max(0, next-start-offset-0.006)
			size := min(int(math.Round(float64(rate)*0.16)), int(math.Round(remaining*float64(rate))), len(signal)-from)
			if size < int(math.Round(float64(rate)*0.052)) {
				continue
			}
			if found, ok := estimateWindow(signal, rate, from, size); ok {
				votes = append(votes, found)
			}
		}
		if len(votes) > 0 {
			if found := selectVotes(votes); found.confidence >= 0.47 {
				events = append(events, Event{
					Start: start, End: math.Max(start+0.045, next), Midi: found.midi, RawMidi: found.midi,
					Confidence: clamp(found.confidence, 0, 1), String: -1,
				})
			}
		}
		if i%6 == 0 {
			report(float64(i+1) / float64(len(points)))
		}
	}
	return events
}

// dedupe drops the weaker of two detections less than 45 ms apart; real repeated notes are further apart.
func dedupe(events []Event) []Event {
	var out []Event
	for _, event := range events {
		if n := len(out); n > 0 && event.Start-out[n-1].Start < 0.045 {
			if event.Confidence > out[n-1].Confidence {
				out[n-1] = event
			}
		} else {
			out = append(out, event)
		}
	}
	return out
}

// Normalize sorts the notes, gives each an identity, and makes sure none overlaps the next.
func Normalize(events []Event, duration float64) []Event {
	out := make([]Event, 0, len(events))
	for i, event := range events {
		if math.IsNaN(event.Start) || math.IsInf(event.Start, 0) {
			continue
		}
		if event.ID == "" {
			event.ID = fmt.Sprintf("n-%d-%d", i, int(math.Round(event.Start*1000)))
		}
		event.Start = math.Max(0, event.Start)
		if event.End == 0 {
			event.End = event.Start + 0.25
		}
		event.End = math.Max(event.Start+0.04, event.End)
		if event.RawMidi == 0 {
			event.RawMidi = event.Midi
		}
		event.Confidence = clamp(event.Confidence, 0, 1)
		out = append(out, event)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	for i := range out {
		if i+1 < len(out) {
			out[i].End = math.Max(out[i].Start+0.04, math.Min(out[i].End, out[i+1].Start))
		} else if duration > 0 {
			out[i].End = math.Min(duration, math.Max(out[i].Start+0.15, out[i].End))
		}
	}
	return out
}

func octaveCandidates(raw, minimum, maximum int) []int {
	var out []int
	for shift := -36; shift <= 36; shift += 12 {
		if midi := raw + shift; midi >= minimum && midi <= maximum {
			out = append(out, midi)
		}
	}
	return out
}

func pitchClass(midi int) int { return ((midi % 12) + 12) % 12 }

// StabilizeOctaves corrects notes read an octave off without flattening real octave jumps:
// quick repeats of the same note prefer the same register, while a jump backed by high
// confidence and a breath in the rhythm is kept.
func StabilizeOctaves(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	const minimum, maximum = 23, 64
	layers := make([][]int, len(events))
	costs := make([][]float64, len(events))
	back := make([][]int, len(events))
	for i, event := range events {
		layers[i] = octaveCandidates(event.Midi, minimum, maximum)
		costs[i] = make([]float64, len(layers[i]))
		back[i] = make([]int, len(layers[i]))
		confidence := event.Confidence
		if confidence == 0 {
			confidence = 0.5
		}
		for c, candidate := range layers[i] {
			costs[i][c] = math.Inf(1)
			back[i][c] = -1
			octaves := math.Abs(float64(candidate-event.Midi)) / 12
			observation := octaves*(1.1+confidence*3.2) + math.Abs(float64(candidate-36))*0.006
			if event.Sure && octaves > 0 {
				observation += 100 // its octave was read from the whole note, and is not up for discussion
			}
			if i == 0 {
				costs[i][c] = observation
				continue
			}
			previousEvent := events[i-1]
			gap := math.Max(0, event.Start-previousEvent.End)
			onsetGap := math.Max(0, event.Start-previousEvent.Start)
			rawJump := abs(event.Midi - previousEvent.Midi)
			previousConfidence := previousEvent.Confidence
			if previousConfidence == 0 {
				previousConfidence = 0.5
			}
			for p, previous := range layers[i-1] {
				jump := abs(candidate - previous)
				repeated := pitchClass(candidate) == pitchClass(previous)
				quick := repeated && onsetGap <= 0.72
				transition := math.Min(float64(jump), 12)*0.075 + math.Max(0, float64(jump-7))*0.19
				if jump == 0 {
					if quick {
						transition -= 0.95
					} else {
						transition -= 0.25
					}
				}
				if quick && jump >= 12 {
					real := rawJump >= 11 && math.Min(confidence, previousConfidence) >= 0.82 && (gap >= 0.28 || onsetGap >= 0.9)
					if real {
						transition += 0.45
					} else {
						transition += 5.2
					}
				}
				if !repeated && jump > 16 {
					transition += float64(jump-16) * 0.28
				}
				if gap > 0.45 {
					transition *= 0.68
				}
				if value := costs[i-1][p] + observation + transition; value < costs[i][c] {
					costs[i][c] = value
					back[i][c] = p
				}
			}
		}
	}
	last := len(events) - 1
	cursor := 0
	for c, value := range costs[last] {
		if value < costs[last][cursor] {
			cursor = c
		}
	}
	out := append([]Event(nil), events...)
	for i := last; i >= 0; i-- {
		if len(layers[i]) == 0 { // out of every register: leave it as read
			continue
		}
		if out[i].RawMidi == 0 {
			out[i].RawMidi = out[i].Midi
		}
		out[i].Midi = layers[i][cursor]
		cursor = back[i][cursor]
		if cursor < 0 && i > 0 {
			cursor = 0
		}
	}
	return out
}
