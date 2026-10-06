// Package rhythm finds the beat of a recording and writes notes against it: which bar and
// which sixteenth each one starts on, and the note values a tablature needs to show how long
// it lasts.
package rhythm

import (
	"math"
	"sort"

	"github.com/MassimoDanieli/c_bass/internal/audio"
)

const (
	fps = 100
	// Division is the number of sixteenths in a beat.
	Division = 4
)

// Rhythm is the pulse of a recording.
type Rhythm struct {
	// Beats are the times of the beats, in seconds.
	Beats []float64 `json:"beats"`
	// PerBar is the number of beats in a bar.
	PerBar int `json:"perBar"`
	// Downbeat is which of the first beats starts a bar.
	Downbeat int `json:"downbeat"`
	// Odd lists the bars that do not have PerBar beats, in order: a bar of two in a piece in
	// four moves every bar line after it.
	Odd []OddBar `json:"odd,omitempty"`
}

// OddBar is a bar with its own number of beats. Bars are counted from 0.
type OddBar struct {
	Bar   int `json:"bar"`
	Beats int `json:"beats"`
}

// BeatsIn is the number of beats in a bar.
func (r *Rhythm) BeatsIn(bar int) int {
	for _, odd := range r.Odd {
		if odd.Bar == bar {
			return odd.Beats
		}
	}
	return r.PerBar
}

// BarStart is where a bar starts, in beats from the first bar line. Bars before the first
// bar line, where a pickup falls, have negative numbers and the usual length.
func (r *Rhythm) BarStart(bar int) int {
	start := bar * r.PerBar
	for _, odd := range r.Odd {
		if odd.Bar < bar {
			start += odd.Beats - r.PerBar
		}
	}
	return start
}

// BarAt is the bar a place falls in, the place being in beats from the first bar line.
func (r *Rhythm) BarAt(beats float64) int {
	if beats < 0 {
		return int(math.Floor(beats / float64(r.PerBar)))
	}
	// from the bar it would be with no odd bars, a step or two either way finds the real one
	bar := int(beats) / r.PerBar
	for r.BarStart(bar) > int(math.Floor(beats)) {
		bar--
	}
	for r.BarStart(bar+1) <= int(math.Floor(beats)) {
		bar++
	}
	return bar
}

// SetBeatsIn gives a bar its own number of beats, or the usual number back.
func (r *Rhythm) SetBeatsIn(bar, beats int) {
	if bar < 0 || beats < 1 {
		return
	}
	kept := r.Odd[:0:0]
	for _, odd := range r.Odd {
		if odd.Bar != bar {
			kept = append(kept, odd)
		}
	}
	if beats != r.PerBar {
		kept = append(kept, OddBar{bar, beats})
		sort.Slice(kept, func(a, b int) bool { return kept[a].Bar < kept[b].Bar })
	}
	r.Odd = kept
}

// fft transforms real and imag in place; their length must be a power of two.
func fft(real, imag []float64) {
	size := len(real)
	for index, reversed := 1, 0; index < size; index++ {
		bit := size >> 1
		for ; reversed&bit != 0; bit >>= 1 {
			reversed ^= bit
		}
		reversed ^= bit
		if index < reversed {
			real[index], real[reversed] = real[reversed], real[index]
			imag[index], imag[reversed] = imag[reversed], imag[index]
		}
	}
	for length := 2; length <= size; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		stepReal, stepImag := math.Cos(angle), math.Sin(angle)
		for start := 0; start < size; start += length {
			turnReal, turnImag := 1.0, 0.0
			for offset := 0; offset < length/2; offset++ {
				even, odd := start+offset, start+offset+length/2
				oddReal := real[odd]*turnReal - imag[odd]*turnImag
				oddImag := real[odd]*turnImag + imag[odd]*turnReal
				real[odd], imag[odd] = real[even]-oddReal, imag[even]-oddImag
				real[even] += oddReal
				imag[even] += oddImag
				turnReal, turnImag = turnReal*stepReal-turnImag*stepImag, turnReal*stepImag+turnImag*stepReal
			}
		}
	}
}

// monoSignal mixes the channels to mono at about 11 kHz: plenty for finding the beat.
func monoSignal(buffer *audio.Buffer) ([]float64, float64) {
	step := max(1, int(math.Round(float64(buffer.SampleRate)/11025)))
	out := make([]float64, buffer.Len()/step)
	scale := 1 / float64(step*len(buffer.Channels))
	for i := range out {
		var sum float64
		for j := i * step; j < i*step+step; j++ {
			for _, channel := range buffer.Channels {
				sum += float64(channel[j])
			}
		}
		out[i] = sum * scale
	}
	return out, float64(buffer.SampleRate) / float64(step)
}

type envelope struct {
	flux, low []float64
	offset    float64
}

// onsetEnvelope measures how much new sound arrives in each hundredth of a second (spectral
// flux), and how much of it is in the bass register: drums and chord changes stand out,
// sustained sound does not.
func onsetEnvelope(signal []float64, rate float64) envelope {
	const size = 256
	hopSize := rate / fps
	frames := max(0, int(math.Floor(float64(len(signal)-size)/hopSize)))
	flux := make([]float64, frames)
	low := make([]float64, frames)
	window := make([]float64, size)
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/size)
	}
	lowBins := max(2, int(math.Round(150/(rate/size))))
	real := make([]float64, size)
	imag := make([]float64, size)
	previous := make([]float64, size/2)
	current := make([]float64, size/2)
	for frame := 0; frame < frames; frame++ {
		start := int(math.Floor(float64(frame) * hopSize))
		for i := 0; i < size; i++ {
			// The reference implementation works in single precision; so does this, to agree with it.
			real[i] = float64(float32(signal[start+i] * window[i]))
			imag[i] = 0
		}
		fft(real, imag)
		var rise, lowRise float64
		for bin := 1; bin < size/2; bin++ {
			current[bin] = math.Log1p(100 * math.Sqrt(real[bin]*real[bin]+imag[bin]*imag[bin]))
			if change := current[bin] - previous[bin]; change > 0 {
				rise += change
				if bin <= lowBins {
					lowRise += change
				}
			}
		}
		flux[frame], low[frame] = rise, lowRise
		previous, current = current, previous
	}
	// Take away the local average, so loud and quiet passages weigh the same.
	flat := make([]float64, frames)
	var sum, square float64
	for frame := 0; frame < frames; frame++ {
		sum += flux[frame]
		if frame >= fps {
			sum -= flux[frame-fps]
		}
		flat[frame] = math.Max(0, flux[frame]-sum/float64(min(frame+1, fps)))
		square += flat[frame] * flat[frame]
	}
	scale := math.Sqrt(square / math.Max(1, float64(frames)))
	if scale == 0 {
		scale = 1
	}
	for i := range flat {
		flat[i] /= scale
	}
	return envelope{flux: flat, low: low, offset: size / 2 / rate}
}

// estimateTempo is the tempo whose beat, and its multiples, best repeat in the envelope;
// between 60 and 200 BPM.
func estimateTempo(flux []float64) float64 {
	const reach = fps * 4
	auto := make([]float64, reach+1)
	for lag := 1; lag <= reach; lag++ {
		var sum float64
		for i := 0; i+lag < len(flux); i++ {
			sum += flux[i] * flux[i+lag]
		}
		auto[lag] = sum
	}
	at := func(position float64) float64 {
		index := int(math.Floor(position))
		if index+1 > reach {
			return 0
		}
		return auto[index] + (auto[index+1]-auto[index])*(position-float64(index))
	}
	best, bestScore := 120.0, math.Inf(-1)
	for bpm := 60.0; bpm < 200; bpm += 0.5 {
		period := 60 * fps / bpm
		prior := math.Log2(bpm / 120)
		score := (at(period) + at(period*2)/2 + at(period*4)/4) * math.Exp(-0.5*prior*prior)
		if score > bestScore {
			bestScore, best = score, bpm
		}
	}
	return best
}

// trackBeats returns the chain of envelope peaks about one beat apart that scores highest
// (Ellis, "Beat tracking by dynamic programming", 2007).
func trackBeats(flux []float64, bpm, offset float64) []float64 {
	period := 60 * fps / bpm
	count := len(flux)
	if float64(count) < period*2 {
		return nil
	}
	nearest := max(1, int(math.Round(period/2)))
	farthest := int(math.Round(period * 2))
	penalty := make([]float64, farthest+1)
	for lag := nearest; lag <= farthest; lag++ {
		l := math.Log(float64(lag) / period)
		penalty[lag] = -100 * l * l
	}
	score := make([]float64, count)
	from := make([]int, count)
	for frame := 0; frame < count; frame++ {
		best, source := 0.0, -1
		for lag := nearest; lag <= farthest && lag <= frame; lag++ {
			if value := score[frame-lag] + penalty[lag]; source < 0 || value > best {
				best, source = value, frame-lag
			}
		}
		score[frame] = flux[frame]
		if source >= 0 {
			score[frame] += best
		}
		from[frame] = source
	}
	last := count - 1
	for frame := max(0, count-farthest); frame < count; frame++ {
		if score[frame] > score[last] {
			last = frame
		}
	}
	var beats []float64
	for frame := last; frame >= 0; frame = from[frame] {
		beats = append(beats, float64(frame)/fps+offset)
	}
	for i, j := 0, len(beats)-1; i < j; i, j = i+1, j-1 {
		beats[i], beats[j] = beats[j], beats[i]
	}
	return beats
}

// estimateDownbeat guesses which of the first beats starts a bar: the one whose bars begin
// with the most bass energy.
func estimateDownbeat(beats, low []float64, perBar int, offset float64) int {
	totals := make([]float64, perBar)
	for index, time := range beats {
		frame := int(math.Round((time - offset) * fps))
		var peak float64
		for near := max(0, frame-3); near <= min(len(low)-1, frame+3); near++ {
			peak = math.Max(peak, low[near])
		}
		totals[index%perBar] += peak
	}
	best := 0
	for i, total := range totals {
		if total > totals[best] {
			best = i
		}
	}
	return best
}

// Analyse finds the pulse of a recording. It returns nil when there is too little to go on.
func Analyse(buffer *audio.Buffer, perBar int) *Rhythm {
	signal, rate := monoSignal(buffer)
	env := onsetEnvelope(signal, rate)
	bpm := estimateTempo(env.flux)
	beats := trackBeats(env.flux, bpm, env.offset)
	if len(beats) < 8 {
		return nil
	}
	downbeat := estimateDownbeat(beats, env.low, perBar, env.offset)
	for i := range beats {
		beats[i] = math.Round(beats[i]*1000) / 1000
	}
	return &Rhythm{Beats: beats, PerBar: perBar, Downbeat: downbeat}
}

// Steady is an even pulse, for a tempo known in advance.
func Steady(bpm, duration float64, perBar int) *Rhythm {
	r := &Rhythm{PerBar: perBar}
	for time := 0.0; time <= duration+60/bpm; time += 60 / bpm {
		r.Beats = append(r.Beats, math.Round(time*1000)/1000)
	}
	return r
}

// Rescale returns the pulse with twice (factor 2) or half as many beats, for when the tracker
// settled on the wrong level.
func (r *Rhythm) Rescale(double bool) *Rhythm {
	out := &Rhythm{PerBar: r.PerBar}
	if double {
		for i, time := range r.Beats {
			out.Beats = append(out.Beats, time)
			if i+1 < len(r.Beats) {
				out.Beats = append(out.Beats, math.Round((time+r.Beats[i+1])*500)/1000)
			}
		}
		out.Downbeat = r.Downbeat * 2
		return out
	}
	for i := r.Downbeat % 2; i < len(r.Beats); i += 2 {
		out.Beats = append(out.Beats, r.Beats[i])
	}
	out.Downbeat = (r.Downbeat / 2) % r.PerBar
	return out
}

// Halfway returns the pulse moved half a beat later: every beat where the "and" after it
// was. It is for when the pulse was followed on the off-beats, which a shaker, a hi-hat or an
// off-beat guitar louder than the beats will bring about.
func (r *Rhythm) Halfway() *Rhythm {
	out := &Rhythm{PerBar: r.PerBar, Downbeat: r.Downbeat}
	for i := 0; i+1 < len(r.Beats); i++ {
		out.Beats = append(out.Beats, math.Round((r.Beats[i]+r.Beats[i+1])*500)/1000)
	}
	if n := len(r.Beats); n >= 2 { // and one more, as far after the last as the last two are apart
		out.Beats = append(out.Beats, math.Round((r.Beats[n-1]+(r.Beats[n-1]-r.Beats[n-2])/2)*1000)/1000)
	}
	return out
}

// Tempo is the typical tempo, in beats per minute.
func (r *Rhythm) Tempo() float64 {
	if len(r.Beats) < 2 {
		return 0
	}
	gaps := make([]float64, 0, len(r.Beats)-1)
	for i := 1; i < len(r.Beats); i++ {
		gaps = append(gaps, r.Beats[i]-r.Beats[i-1])
	}
	sort.Float64s(gaps)
	return 60 / gaps[len(gaps)/2]
}

// Position turns a time in seconds into a position in beats, counted from the first bar
// line. Before the first beat and after the last the pulse carries on at the nearest tempo.
func (r *Rhythm) Position(time float64) float64 {
	beats := r.Beats
	last := len(beats) - 1
	var index int
	switch {
	case time <= beats[0]:
		index = 0
	case time >= beats[last]:
		index = last - 1
	default:
		index = sort.SearchFloat64s(beats, time)
		if index == len(beats) || beats[index] > time {
			index--
		}
	}
	return float64(index) + (time-beats[index])/(beats[index+1]-beats[index]) - float64(r.Downbeat)
}

// Time is the inverse of Position.
func (r *Rhythm) Time(position float64) float64 {
	absolute := position + float64(r.Downbeat)
	index := max(0, min(len(r.Beats)-2, int(math.Floor(absolute))))
	return r.Beats[index] + (absolute-float64(index))*(r.Beats[index+1]-r.Beats[index])
}

// Note is anything with a start and an end in seconds.
type Note struct{ Start, End float64 }

// Calibrate measures how far, in beats, the notes sit from the pulse on average: an onset is
// heard a little after the drum that marks the beat, and that lag would tip notes onto the
// wrong sixteenth.
func (r *Rhythm) Calibrate(notes []Note) float64 {
	var real, imag float64
	for _, note := range notes {
		phase := r.Position(note.Start) * 2 * 2 * math.Pi // against the eighths
		real += math.Cos(phase)
		imag += math.Sin(phase)
	}
	if len(notes) == 0 || math.Hypot(real, imag)/float64(len(notes)) < 0.12 {
		return 0
	}
	return math.Atan2(imag, real) / (2 * math.Pi) / 2
}

// Placed is a note on the grid of sixteenths.
type Placed struct {
	// Index of the note in the list given to Quantize.
	Index int
	// Slot is the sixteenth it starts on, counted from the first bar line.
	Slot int
	// Slots is how many sixteenths it lasts: until the next note, or until it stops if a real silence follows.
	Slots int
}

// Quantize puts the notes on the grid of sixteenths.
func (r *Rhythm) Quantize(notes []Note) []Placed {
	shift := r.Calibrate(notes)
	placed := make([]Placed, len(notes))
	ends := make([]int, len(notes))
	previous := math.MinInt
	for i, note := range notes {
		slot := int(math.Round((r.Position(note.Start) - shift) * Division))
		if slot <= previous {
			slot = previous + 1 // two notes never share a place
		}
		placed[i] = Placed{Index: i, Slot: slot}
		ends[i] = int(math.Round((r.Position(note.End) - shift) * Division))
		previous = slot
	}
	for i := range placed {
		// A gap of a single sixteenth is how a note is let go, not a rest worth writing.
		if i+1 < len(placed) && placed[i+1].Slot-ends[i] <= 1 {
			placed[i].Slots = placed[i+1].Slot - placed[i].Slot
		} else {
			placed[i].Slots = max(1, ends[i]-placed[i].Slot)
		}
	}
	return placed
}

var values = [...]int{16, 12, 8, 6, 4, 3, 2, 1}

// Piece is a stretch of one note value inside a bar.
type Piece struct{ Slot, Value int }

// SplitValues splits a stretch inside one bar into note values that read naturally against
// the beat. Rests are kept plainer than notes: never dotted, never across a beat.
func SplitValues(start, length, barSlots int, rest bool) []Piece {
	var pieces []Piece
	end := start + length
	for at := start; at < end; {
		inBeat := at % Division
		room := end - at
		switch {
		case inBeat == 2 && room == 4 && at%(Division*2) == 2 && !rest:
			// a syncopated quarter
		case inBeat != 0:
			room = min(room, Division-inBeat)
		case at%(Division*2) != 0:
			// from beats 2 and 4, up to the next strong beat
			if room == 6 && !rest {
				room = 6
			} else {
				room = min(room, 4)
			}
		case at != 0 && at*2 != barSlots:
			room = min(room, 8)
		}
		value := 1
		for _, candidate := range values {
			if candidate <= room && candidate <= barSlots && !(rest && candidate%3 == 0) {
				value = candidate
				break
			}
		}
		pieces = append(pieces, Piece{at, value})
		at += value
	}
	return pieces
}

// Symbol is one written sign: a note or a rest, with its place and its value.
type Symbol struct {
	// Bar it belongs to, counted from 0; pickup notes before the first bar line are in bar -1.
	Bar int
	// Slot is its place in the bar, in sixteenths.
	Slot int
	// Value in sixteenths: 16 a whole note, 4 a quarter, 3 a dotted eighth, 1 a sixteenth.
	Value int
	Rest  bool
	// Index of the note it writes; meaningless for a rest.
	Index int
	// Tied marks a note that only continues the one before it.
	Tied bool
	// At is its place in sixteenths from the first bar line, whatever the bars before it hold.
	At int
}

// Score is the written music.
type Score struct {
	Symbols  []Symbol
	BarSlots int
	Placed   []Placed
	// Shift is the calibration used, in beats: a time t sounds at Position(t)-Shift on the page.
	Shift float64
}

// Notate writes the notes out: for every bar, its symbols in order.
func (r *Rhythm) Notate(notes []Note) *Score {
	score := &Score{BarSlots: r.PerBar * Division, Placed: r.Quantize(notes), Shift: r.Calibrate(notes)}
	// where the bar holding a sixteenth starts, and how long that bar is, in sixteenths
	barOf := func(at int) (bar, start, length int) {
		bar = r.BarAt(float64(at) / Division)
		return bar, r.BarStart(bar) * Division, r.BeatsIn(bar) * Division
	}
	push := func(slot, length int, rest bool, index int) {
		first := true
		for at := slot; at < slot+length; {
			bar, start, barSlots := barOf(at)
			inBar := at - start
			span := min(slot+length-at, barSlots-inBar)
			for _, piece := range SplitValues(inBar, span, barSlots, rest) {
				score.Symbols = append(score.Symbols, Symbol{
					Bar: bar, Slot: piece.Slot, Value: piece.Value, Rest: rest, Index: index, Tied: !first && !rest, At: start + piece.Slot,
				})
				first = false
			}
			at += span
		}
	}
	cursor := 0
	if len(score.Placed) > 0 {
		_, cursor, _ = barOf(score.Placed[0].Slot)
	}
	for _, note := range score.Placed {
		if note.Slot > cursor {
			push(cursor, note.Slot-cursor, true, -1)
		}
		push(note.Slot, note.Slots, false, note.Index)
		cursor = note.Slot + note.Slots
	}
	if _, start, barSlots := barOf(cursor); cursor > start {
		push(cursor, start+barSlots-cursor, true, -1)
	}
	return score
}
