// Package player plays a separated recording: the bass and the rest each at its own volume,
// at any speed without changing the pitch, with a stretch to repeat.
package player

import (
	"encoding/binary"
	"math"
	"sort"
	"sync"

	"github.com/MassimoDanieli/c_bass/internal/audio"
)

const (
	// Rate is the sample rate of everything played.
	Rate = 44100
	// frame is the length of the pieces the recording is cut into: 80 ms, long enough to hold
	// two periods of the lowest string.
	frame = 3528
	hop   = frame / 2
	// reach is how far a piece may be moved to line its waves up with the piece before: 20 ms
	// each way, more than a period of the low E.
	reach = 882
)

// mark ties a moment of the output to a place in the recording.
type mark struct {
	out  int64   // output frame
	at   float64 // frame of the recording heard there
	rate float64 // frames of recording per output frame from there on
}

// Engine turns the two tracks into the sound to play. Slowing down is done by overlapping
// pieces of the recording (WSOLA): each piece is placed where its waves continue those of the
// piece before, so the pitch stays what it was. At full speed the pieces follow one another
// exactly and the recording comes out untouched.
//
// Slowing down means playing some of the recording twice, and an attack played twice is
// heard as two notes. So a piece only goes back a little, less than its own length, and only
// to a place that sounds like what would have come next: that is true of a note ringing,
// never of an attack. Around an attack the recording runs on at its own speed, and the time
// is made up in the note that follows.
type Engine struct {
	mu            sync.Mutex
	bass, backing *audio.Buffer
	length        int

	playing           bool
	speed             float64
	gainBass, gainMix float32
	loopOn            bool
	loopA, loopB      float64 // frames

	position float64 // where the next piece nominally starts, in frames
	natural  int     // where the previous piece carries on
	jumped   bool    // the next piece starts somewhere new: nothing to line up with
	tail     [2][hop]float32
	tailLive bool
	window   [frame]float32
	strip    []float32 // mono, around the nominal place
	model    []float32 // mono, the continuation to match
	hits     []int     // frames where a drum or a cymbal is struck, in order

	beats      []float64 // frames of the beats of the recording
	strong     []bool    // which of them start a bar
	metronome  bool
	nextBeat   int       // the next beat the metronome has to click
	tick, tock []float32 // the click, and the stronger one of a first beat
	voices     []voice
	count      []int64 // output frames of the clicks still to come before the recording starts
	counted    int     // how many clicks the count has in all
	musicAt    int64   // the output frame at which the recording starts after a count
	quicken    float64 // how much faster every time round a repeated stretch
	quickenTo  float64

	pending  []float32 // interleaved, generated and not yet read
	produced int64     // output frames generated
	consumed int64     // output frames read
	marks    []mark
}

// NewEngine prepares two tracks of the same length for playing. Both must be 44.1 kHz stereo.
func NewEngine(bass, backing *audio.Buffer) *Engine {
	e := &Engine{bass: bass, backing: backing, length: min(bass.Len(), backing.Len()), speed: 1, gainBass: 1, gainMix: 1, jumped: true}
	for i := range e.window {
		e.window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/frame))
	}
	e.strip = make([]float32, 2*hop)
	e.model = make([]float32, hop)
	e.hits = findHits(bass, backing, e.length)
	e.tick, e.tock = clickSound(1100, 0.3), clickSound(1650, 0.4)
	return e
}

// Duration of the recording in seconds.
func (e *Engine) Duration() float64 { return float64(e.length) / Rate }

// SetPlaying starts or stops. Starting at the very end starts again from the beginning (or
// from the start of the stretch being repeated).
func (e *Engine) SetPlaying(playing bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if playing && !e.playing {
		if e.position >= float64(e.length)-hop {
			e.position = 0
			if e.loopOn {
				e.position = e.loopA
			}
		}
		e.jumped = true
	}
	e.playing = playing
	if !playing {
		e.count, e.musicAt = e.count[:0], 0 // a count under way is dropped
	}
}

// Playing says whether the recording is running.
func (e *Engine) Playing() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.playing
}

// Seek moves to a time in seconds.
func (e *Engine) Seek(seconds float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.position = math.Max(0, math.Min(float64(e.length), seconds*Rate))
	e.jumped = true
}

// SetSpeed sets the speed, 1 being that of the recording. It is kept between 0.25 and 1.5.
func (e *Engine) SetSpeed(speed float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.speed = math.Max(0.25, math.Min(1.5, speed))
}

// Speed is the current speed.
func (e *Engine) Speed() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.speed
}

// SetGains sets the volume of the bass and of the rest; 1 is the level of the recording.
func (e *Engine) SetGains(bass, rest float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.gainBass, e.gainMix = float32(bass), float32(rest)
}

// SetLoop repeats the stretch from a to b, in seconds; on false plays straight through.
func (e *Engine) SetLoop(a, b float64, on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.loopA, e.loopB, e.loopOn = a*Rate, b*Rate, on && b-a > 0.2
}

func (e *Engine) sample(channel, i int) float32 {
	if i < 0 || i >= e.length {
		return 0
	}
	return e.gainBass*e.bass.Channels[channel][i] + e.gainMix*e.backing.Channels[channel][i]
}

// mono fills out with the sum of the channels from frame i on.
func (e *Engine) mono(out []float32, i int) {
	for k := range out {
		at := i + k
		if at < 0 || at >= e.length {
			out[k] = 0
			continue
		}
		// Both tracks at full level whatever the volumes: the waves to line up are those of
		// the recording, and a muted track must not change where the pieces fall.
		out[k] = e.bass.Channels[0][at] + e.bass.Channels[1][at] + e.backing.Channels[0][at] + e.backing.Channels[1][at]
	}
}

const (
	// patience is how far the sound may run ahead of where it should be, or fall behind,
	// while waiting for a place where going back or skipping will not be heard: 50 ms. More
	// would spare more attacks, and make the rhythm uneven.
	patience = Rate * 5 / 100
	// least is the smallest step back or forward worth taking: 2.5 ms. Anything nearer is the
	// same wave barely moved, which always looks alike.
	least = Rate / 400
	// alike is how well a place must match what would have come next, from -1 to 1.
	alike = 0.5
	// settle is how long after a hit its sound is left alone: 10 ms.
	settle = Rate / 100
)

// align chooses where the next piece starts. While the sound is close to where it should be,
// the previous piece simply carries on. Once it has drifted, the piece goes back (or forward,
// when speeding up) by no more than its own half, to the place that best continues the
// waves; and if no place does, because an attack is in the way, it waits.
func (e *Engine) align(nominal int) int {
	ahead := e.natural - nominal
	if ahead >= -reach && ahead <= reach {
		return e.natural
	}
	waiting := ahead < patience && -ahead < patience
	var low, high, wanted int
	if ahead > 0 {
		low, high, wanted = e.natural-hop, e.natural-least, e.natural-min(ahead, hop)
		// A drum or a cymbal struck in what would be played again, or still to come in the
		// fading end of the previous piece, would be heard twice: start after it has rung.
		if hit, ok := e.hitBefore(e.natural + hop*7/8); ok && hit+settle > low && waiting {
			low = hit + settle
		}
	} else {
		low, high, wanted = e.natural+least, e.natural+hop, e.natural+min(-ahead, hop)
		// One struck in what would be skipped would be lost: keep it well inside the new piece.
		if hit, ok := e.hitAfter(e.natural + hop/8); ok && hit-hop*3/4 < high && waiting {
			high = hit - hop*3/4
		}
	}
	if low > high {
		return e.natural
	}
	start, likeness := e.search(low, high, wanted)
	if likeness < alike && waiting {
		return e.natural
	}
	return start
}

// findHits returns where something is struck: a sudden rise of the level high up, above the
// harmonics of the bass, where drums, cymbals and the click of a plucked string are. A plain
// rise of the whole level would not do: the level of a low note swells with every one of its
// waves. The attacks of the bass itself need no list, a note never looks like the one before.
func findHits(bass, backing *audio.Buffer, length int) []int {
	const step = 256 // 6 ms
	steps := length / step
	if steps < 8 {
		return nil
	}
	level := make([]float64, steps)
	smooth := 1 - math.Exp(-2*math.Pi*2000/Rate)
	var held, top float64
	for i := 0; i < steps*step; i++ {
		v := float64(bass.Channels[0][i] + bass.Channels[1][i] + backing.Channels[0][i] + backing.Channels[1][i])
		held += smooth * (v - held)
		level[i/step] += (v - held) * (v - held)
	}
	for _, v := range level {
		top = math.Max(top, v)
	}
	floor := top * 1e-4
	var hits []int
	last := -100
	for i := 6; i < steps; i++ {
		before := 0.0
		for _, v := range level[i-6 : i] {
			before = math.Max(before, v)
		}
		if level[i] > floor && level[i] > 2*before && i-last > 5 {
			hits = append(hits, i*step)
			last = i
		}
	}
	return hits
}

// hitBefore is the last hit at or before a frame.
func (e *Engine) hitBefore(frame int) (int, bool) {
	i := sort.SearchInts(e.hits, frame+1) - 1
	if i < 0 {
		return 0, false
	}
	return e.hits[i], true
}

// hitAfter is the first hit at or after a frame.
func (e *Engine) hitAfter(frame int) (int, bool) {
	i := sort.SearchInts(e.hits, frame)
	if i >= len(e.hits) {
		return 0, false
	}
	return e.hits[i], true
}

// search finds, between low and high, the start whose waves best continue the previous
// piece, leaning towards the wanted one when several do as well. It also says how alike
// the two are, from -1 to 1.
func (e *Engine) search(low, high, wanted int) (int, float64) {
	span := high - low
	if need := span + hop; len(e.strip) < need {
		e.strip = make([]float32, need)
	}
	e.mono(e.model, e.natural)
	e.mono(e.strip[:span+hop], low)
	between := func(offset, step, from, to int) float64 {
		var dot, ours, theirs float64
		for k := from; k < to; k += step {
			m, c := float64(e.model[k]), float64(e.strip[offset+k])
			dot += m * c
			ours += m * m
			theirs += c * c
		}
		if ours < 1e-7 && theirs < 1e-7 {
			return 1 // silence continues silence
		}
		return dot / math.Sqrt(ours*theirs+1e-12)
	}
	score := func(offset, step int) float64 {
		return between(offset, step, 0, hop) - 0.15*math.Abs(float64(low+offset-wanted))/hop
	}
	best, bestScore := 0, math.Inf(-1)
	const coarse = 6
	for offset := 0; offset <= span; offset += coarse {
		if s := score(offset, coarse); s > bestScore {
			best, bestScore = offset, s
		}
	}
	fine, fineScore := best, math.Inf(-1)
	for offset := max(0, best-coarse); offset <= min(span, best+coarse); offset++ {
		if s := score(offset, 2); s > fineScore {
			fine, fineScore = offset, s
		}
	}
	// Alike all the way: a note starting late in the stretch must not hide behind the rest.
	likeness := 1.0
	const parts = 3
	for part := 0; part < parts; part++ {
		likeness = math.Min(likeness, between(fine, 1, part*hop/parts, (part+1)*hop/parts))
	}
	return low + fine, likeness
}

// block generates the next hop frames of output.
func (e *Engine) block() {
	base := len(e.pending)
	e.pending = append(e.pending, make([]float32, hop*2)...)
	out := e.pending[base:]
	defer func() {
		e.ring(out)
		e.produced += hop
	}()
	counting := e.playing && e.produced < e.musicAt
	if !e.playing || counting {
		if e.tailLive { // let the last piece fade out rather than cut it
			for i := 0; i < hop; i++ {
				out[2*i], out[2*i+1] = e.tail[0][i], e.tail[1][i]
			}
			e.tail = [2][hop]float32{}
			e.tailLive = false
		}
		e.note(mark{e.produced, e.position, 0})
		if counting {
			// the clicks of the bar counted in before the recording starts
			for len(e.count) > 0 && e.count[0] < e.produced+hop {
				e.strike(int(e.count[0]-e.produced), len(e.count) == e.counted)
				e.count = e.count[1:]
			}
			e.jumped = true
		}
		return
	}
	start := int(math.Round(e.position))
	if e.jumped {
		e.jumped = false
		// somewhere new: the next beat to click is the first one from here on
		e.nextBeat = sort.SearchFloat64s(e.beats, float64(start))
	} else {
		start = e.align(start)
	}
	for c := 0; c < 2; c++ {
		for i := 0; i < hop; i++ {
			out[2*i+c] = clip(e.tail[c][i] + e.window[i]*e.sample(c, start+i))
		}
		for i := hop; i < frame; i++ {
			e.tail[c][i-hop] = e.window[i] * e.sample(c, start+i)
		}
	}
	e.tailLive = true
	e.note(mark{e.produced, float64(start), e.speed})
	// The beats that fall in what this block plays, each at its place in the block. A block
	// plays its piece of the recording at the recording's own speed, whatever the speed asked:
	// slowing down is pieces played again, and a beat already clicked is not clicked again.
	for ; e.nextBeat < len(e.beats) && e.beats[e.nextBeat] < float64(start+hop); e.nextBeat++ {
		if e.metronome {
			e.strike(int(e.beats[e.nextBeat])-start, e.strong[e.nextBeat])
		}
	}
	e.natural = start + hop
	e.position += hop * e.speed
	switch {
	case e.loopOn && e.position >= e.loopB && e.position-hop*e.speed < e.loopB:
		e.position, e.jumped = e.loopA, true
		if e.quicken > 0 && e.speed < e.quickenTo { // every time round a little faster
			e.speed = math.Min(e.quickenTo, e.speed+e.quicken)
		}
	case e.position >= float64(e.length):
		e.position, e.playing = float64(e.length), false
	}
}

// voice is a click still sounding.
type voice struct {
	sound []float32
	at    int // the next sample to play; negative while it has not started in this block
}

// strike starts a click at an offset into the block being generated.
func (e *Engine) strike(offset int, strong bool) {
	sound := e.tick
	if strong {
		sound = e.tock
	}
	e.voices = append(e.voices, voice{sound, -max(0, min(hop-1, offset))})
}

// ring adds the clicks that are sounding to a block of output.
func (e *Engine) ring(out []float32) {
	kept := e.voices[:0]
	for _, v := range e.voices {
		for i := 0; i < hop; i++ {
			at := v.at + i
			if at < 0 {
				continue
			}
			if at >= len(v.sound) {
				break
			}
			out[2*i] = clip(out[2*i] + v.sound[at])
			out[2*i+1] = clip(out[2*i+1] + v.sound[at])
		}
		if v.at += hop; v.at < len(v.sound) {
			kept = append(kept, v)
		}
	}
	e.voices = kept
}

// clickSound is a short tick: a sine that dies away in a few hundredths of a second.
func clickSound(hz, level float64) []float32 {
	out := make([]float32, Rate*5/100)
	for i := range out {
		t := float64(i) / Rate
		out[i] = float32(level * math.Exp(-t*90) * math.Sin(2*math.Pi*hz*t) * math.Min(1, t/0.0005))
	}
	return out
}

// SetBeats tells the engine where the beats of the recording are, in seconds, and which of
// them start a bar: what the metronome plays.
func (e *Engine) SetBeats(beats []float64, strong []bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.beats = make([]float64, len(beats))
	for i, beat := range beats {
		e.beats[i] = beat * Rate
	}
	e.strong = append([]bool(nil), strong...)
	e.nextBeat = sort.SearchFloat64s(e.beats, e.position)
}

// SetMetronome turns the click on every beat on or off.
func (e *Engine) SetMetronome(on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metronome = on && len(e.beats) > 0
}

// PlayCounted starts after a count: so many clicks, so far apart in seconds as heard, the
// first one stronger. The recording starts where the next click would have been.
func (e *Engine) PlayCounted(clicks int, apart float64) {
	e.SetPlaying(true)
	e.mu.Lock()
	defer e.mu.Unlock()
	every := int64(apart * Rate)
	first := e.produced + int64(len(e.pending)/2) // after what has been generated and not yet read
	e.count = e.count[:0]
	for i := 0; i < clicks; i++ {
		e.count = append(e.count, first+int64(i)*every)
	}
	e.counted = clicks
	e.musicAt = first + int64(clicks)*every
}

// Counting says whether the count before the recording is still going.
func (e *Engine) Counting() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.playing && e.produced < e.musicAt
}

// SetQuicken makes a stretch being repeated go a little faster every time round, by step, up
// to a top speed; a step of 0 leaves the speed alone.
func (e *Engine) SetQuicken(step, top float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.quicken, e.quickenTo = step, top
}

func clip(v float32) float32 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

func (e *Engine) note(m mark) {
	if len(e.marks) >= 1024 {
		e.marks = append(e.marks[:0], e.marks[512:]...)
	}
	e.marks = append(e.marks, m)
}

// Read fills p with 32-bit float stereo samples, little-endian, as the sound device wants them.
func (e *Engine) Read(p []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	frames := len(p) / 8
	for len(e.pending) < frames*2 {
		e.block()
	}
	for i := 0; i < frames*2; i++ {
		binary.LittleEndian.PutUint32(p[i*4:], math.Float32bits(e.pending[i]))
	}
	e.pending = append(e.pending[:0], e.pending[frames*2:]...)
	e.consumed += int64(frames)
	return frames * 8, nil
}

// Consumed is the number of output frames read so far.
func (e *Engine) Consumed() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.consumed
}

// At says which moment of the recording, in seconds, is heard at a given output frame.
func (e *Engine) At(out int64) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.marks) == 0 {
		return e.position / Rate
	}
	m := e.marks[0]
	for i := len(e.marks) - 1; i >= 0; i-- {
		if e.marks[i].out <= out {
			m = e.marks[i]
			break
		}
	}
	at := m.at + float64(max(0, min(out-m.out, hop)))*m.rate
	return math.Max(0, math.Min(float64(e.length), at)) / Rate
}

// Target is where the recording will be once everything already generated has been heard:
// the place to show straight after a jump.
func (e *Engine) Target() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.position / Rate
}
