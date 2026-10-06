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
	Edited bool `json:"edited,omitempty"`
}

// Options for Transcribe.
type Options struct {
	// Isolated says the recording is a bass on its own, as separated by Demucs.
	Isolated bool
	// Sensitivity to new attacks, from 0.55 (fewer notes) to 0.90 (more); 0.72 if zero.
	Sensitivity float64
	// Progress, if set, is called with a value from 0 to 1.
	Progress func(float64)
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
		events = isolatedNotes(signal, rate, options.Sensitivity, options.Progress)
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
}

// framePitch finds the pitch of a short window by normalised autocorrelation, preferring the
// shortest period among those that fit almost as well as the best, which is the fundamental.
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
	bestLag, bestScore := -1, -1.0
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
		score := xy / math.Sqrt(base*energy+1e-20)
		scores[lag] = score
		if score > bestScore {
			bestScore, bestLag = score, lag
		}
	}
	if bestScore < 0.5 {
		return pitched{}, false
	}
	chosen := bestLag
	strong := math.Max(0.6, bestScore*0.92)
	for lag := minimumLag + 1; lag < bestLag; lag++ {
		if scores[lag] >= strong && scores[lag] >= scores[lag-1] && scores[lag] >= scores[lag+1] {
			chosen = lag
			break
		}
	}
	midi := int(math.Round(69 + 12*math.Log2(float64(rate)/float64(chosen)/440)))
	if midi < 23 || midi > 76 {
		return pitched{}, false
	}
	return pitched{midi, scores[chosen]}, true
}

// isolatedNotes follows the note of a bass on its own instead of looking for bursts of energy.
// A held note keeps its level; a plucked one dips and comes back within a few hundredths of a
// second; a slurred one changes pitch without either. Those three are told apart here.
func isolatedNotes(signal []float32, rate int, sensitivity float64, report func(float64)) []Event {
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
	gate := reference * 0.07
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
	for _, i := range attacks {
		bounds[i+2] = true
	}

	size := int(math.Round(float64(rate) * 0.085))
	minimumLag := max(2, rate/330)
	maximumLag := rate / 31
	scores := make([]float64, maximumLag+1)
	raw := make([]int, count)
	sure := make([]float64, count)
	for i := 0; i < count; i++ {
		if active(i) {
			if found, ok := framePitch(signal, rate, i*hop-size/2, size, minimumLag, maximumLag, scores); ok {
				raw[i], sure[i] = found.midi, found.confidence
			}
		}
		if i%400 == 0 {
			report(float64(i) / float64(count))
		}
	}
	// The median of five frames steadies the pitch.
	pitch := make([]int, count)
	near := make([]int, 0, 5)
	for i := 0; i < count; i++ {
		near = near[:0]
		for j := max(0, i-2); j <= min(count-1, i+2); j++ {
			if raw[j] != 0 {
				near = append(near, raw[j])
			}
		}
		if len(near) >= 3 {
			sort.Ints(near)
			if len(near)%2 == 1 {
				pitch[i] = near[len(near)/2]
			} else {
				pitch[i] = int(math.Round(float64(near[len(near)/2-1]+near[len(near)/2]) / 2))
			}
		}
	}
	// A new pitch that holds for four hundredths (twice that for an octave, the usual misreading) starts a note.
	current, run, candidate := 0, 0, 0
	for i := 0; i < count; i++ {
		p := pitch[i]
		if !active(i) {
			current, run, candidate = 0, 0, 0
			continue
		}
		if p == 0 {
			continue
		}
		if bounds[i] {
			current, run, candidate = p, 0, 0
			continue
		}
		if current == 0 {
			current = p
			continue
		}
		if p != current {
			if p == candidate {
				run++
			} else {
				run = 1
			}
			candidate = p
			need := 4
			if (p-current)%12 == 0 {
				need = 8
			}
			if run >= need {
				bounds[i-run+1] = true
				current, run, candidate = p, 0, 0
			}
		} else {
			run, candidate = 0, 0
		}
	}

	type piece struct {
		from, to   int
		midi       int
		confidence float64
	}
	var pieces [][2]int
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
				pieces = append(pieces, [2]int{from, k})
				from = k
			}
		}
		i = j
	}
	pitchOf := func(from, to int) (int, float64) {
		votes := map[int]int{}
		best, bestCount := 0, 0
		for i := min(from+2, to-1); i < to; i++ {
			if pitch[i] == 0 {
				continue
			}
			votes[pitch[i]]++
			if c := votes[pitch[i]]; c > bestCount || (c == bestCount && pitch[i] < best) {
				best, bestCount = pitch[i], c
			}
		}
		var total float64
		n := 0
		for i := from; i < to; i++ {
			if sure[i] != 0 {
				total += sure[i]
				n++
			}
		}
		if n == 0 {
			return best, 0
		}
		return best, total / float64(n)
	}
	const minimum = 5
	var notes []piece
	for _, p := range pieces {
		from, to := p[0], p[1]
		midi, confidence := pitchOf(from, to)
		// A fragment too short to be a note belongs to its neighbour, which keeps the pitch of the longer part.
		if n := len(notes); n > 0 && notes[n-1].to == from && (to-from < minimum || notes[n-1].to-notes[n-1].from < minimum) {
			last := notes[n-1]
			if last.to-last.from >= to-from && last.midi != 0 {
				notes[n-1] = piece{last.from, to, last.midi, last.confidence}
			} else {
				notes[n-1] = piece{last.from, to, midi, confidence}
			}
			continue
		}
		if midi == 0 || to-from < minimum {
			continue
		}
		notes = append(notes, piece{from, to, midi, confidence})
	}
	// Two leftovers that are not notes: the blur of a slide into the next note, and a blip on its own in silence.
	var kept []piece
	for k := 0; k < len(notes); k++ {
		note := notes[k]
		short := note.to-note.from < 8
		hasNext := k+1 < len(notes) && notes[k+1].from == note.to
		if short && hasNext && notes[k+1].midi != note.midi && abs(notes[k+1].midi-note.midi) <= 2 {
			notes[k+1].from = note.from
			continue
		}
		if short && !hasNext && !(len(kept) > 0 && kept[len(kept)-1].to == note.from) {
			continue
		}
		kept = append(kept, note)
	}
	events := make([]Event, 0, len(kept))
	for _, note := range kept {
		if note.midi == 0 {
			continue
		}
		events = append(events, Event{
			Start: float64(note.from) / fps, End: float64(note.to) / fps,
			Midi: note.midi, RawMidi: note.midi, Confidence: clamp(note.confidence, 0, 1), String: -1,
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
	return pitched{midi, bestScore}, true
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
	return pitched{best, bestConfidence}
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
