// Package player plays a separated recording: the bass and the rest each at its own volume,
// at any speed without changing the pitch, with a stretch to repeat.
package player

import (
	"encoding/binary"
	"math"
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
	e.strip = make([]float32, 2*reach+hop)
	e.model = make([]float32, hop)
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

// align finds, near the nominal place, the start whose waves best continue the previous piece.
func (e *Engine) align(nominal int) int {
	if d := e.natural - nominal; d >= -reach && d <= reach {
		return e.natural // the previous piece simply carries on
	}
	e.mono(e.model, e.natural)
	e.mono(e.strip, nominal-reach)
	score := func(offset, step int) float64 {
		var dot, energy float64
		for k := 0; k < hop; k += step {
			c := float64(e.strip[offset+k])
			dot += float64(e.model[k]) * c
			energy += c * c
		}
		return dot / math.Sqrt(energy+1e-9)
	}
	best, bestScore := reach, math.Inf(-1)
	const coarse = 6
	for offset := 0; offset <= 2*reach; offset += coarse {
		if s := score(offset, coarse); s > bestScore {
			best, bestScore = offset, s
		}
	}
	fine, fineScore := best, math.Inf(-1)
	for offset := max(0, best-coarse); offset <= min(2*reach, best+coarse); offset++ {
		if s := score(offset, 2); s > fineScore {
			fine, fineScore = offset, s
		}
	}
	return nominal - reach + fine
}

// block generates the next hop frames of output.
func (e *Engine) block() {
	base := len(e.pending)
	e.pending = append(e.pending, make([]float32, hop*2)...)
	out := e.pending[base:]
	if !e.playing {
		if e.tailLive { // let the last piece fade out rather than cut it
			for i := 0; i < hop; i++ {
				out[2*i], out[2*i+1] = e.tail[0][i], e.tail[1][i]
			}
			e.tail = [2][hop]float32{}
			e.tailLive = false
		}
		e.note(mark{e.produced, e.position, 0})
		e.produced += hop
		return
	}
	start := int(math.Round(e.position))
	if e.jumped {
		e.jumped = false
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
	e.produced += hop
	e.natural = start + hop
	e.position += hop * e.speed
	switch {
	case e.loopOn && e.position >= e.loopB && e.position-hop*e.speed < e.loopB:
		e.position, e.jumped = e.loopA, true
	case e.position >= float64(e.length):
		e.position, e.playing = float64(e.length), false
	}
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
