package player

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/audio"
)

func tone(seconds, hz float64, level float32) *audio.Buffer {
	n := int(seconds * Rate)
	left, right := make([]float32, n), make([]float32, n)
	for i := range left {
		v := level * float32(math.Sin(2*math.Pi*hz*float64(i)/Rate))
		left[i], right[i] = v, v
	}
	return &audio.Buffer{SampleRate: Rate, Channels: [][]float32{left, right}}
}

func silence(seconds float64) *audio.Buffer {
	n := int(seconds * Rate)
	return &audio.Buffer{SampleRate: Rate, Channels: [][]float32{make([]float32, n), make([]float32, n)}}
}

// pull reads frames of output and returns the left channel.
func pull(e *Engine, frames int) []float32 {
	raw := make([]byte, frames*8)
	e.Read(raw)
	out := make([]float32, frames)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*8:]))
	}
	return out
}

func TestFullSpeedIsTheRecording(t *testing.T) {
	bass, rest := tone(2, 55, 0.3), tone(2, 440, 0.2)
	e := NewEngine(bass, rest)
	e.SetPlaying(true)
	out := pull(e, Rate)
	for i := hop; i < len(out); i++ { // the first piece fades in
		want := bass.Channels[0][i] + rest.Channels[0][i]
		if math.Abs(float64(out[i]-want)) > 1e-5 {
			t.Fatalf("frame %d: %f, want %f", i, out[i], want)
		}
	}
	if at := e.At(Rate / 2); math.Abs(at-0.5) > 0.001 {
		t.Fatalf("half a second in, the place is %f", at)
	}
}

func TestMutingTheBassLeavesTheRest(t *testing.T) {
	bass, rest := tone(1, 55, 0.3), tone(1, 440, 0.2)
	e := NewEngine(bass, rest)
	e.SetGains(0, 1)
	e.SetPlaying(true)
	out := pull(e, Rate/2)
	for i := hop; i < len(out); i++ {
		if math.Abs(float64(out[i]-rest.Channels[0][i])) > 1e-5 {
			t.Fatalf("frame %d: %f, want %f", i, out[i], rest.Channels[0][i])
		}
	}
}

// At half speed a low note must last twice as long, at the same pitch and without the
// wobble in level that pieces put together out of step would give.
func TestHalfSpeedKeepsThePitch(t *testing.T) {
	for _, hz := range []float64{41.2, 55, 98, 196} {
		e := NewEngine(tone(3, hz, 0.5), silence(3))
		e.SetSpeed(0.5)
		e.SetPlaying(true)
		out := pull(e, 5*Rate)
		if at := e.At(4 * Rate); math.Abs(at-2) > 0.05 {
			t.Fatalf("%g Hz: after 4 seconds at half speed the place is %f, want 2", hz, at)
		}
		body := out[Rate : 4*Rate]
		crossings := 0
		for i := 1; i < len(body); i++ {
			if body[i-1] < 0 && body[i] >= 0 {
				crossings++
			}
		}
		if got := float64(crossings) / 3; math.Abs(got-hz) > hz*0.02 {
			t.Errorf("%g Hz came out as %.1f Hz", hz, got)
		}
		low, high := math.Inf(1), 0.0
		for start := 0; start+hop <= len(body); start += hop {
			var sum float64
			for _, v := range body[start : start+hop] {
				sum += float64(v) * float64(v)
			}
			level := math.Sqrt(sum / hop)
			low, high = math.Min(low, level), math.Max(high, level)
		}
		if low < 0.3 || high > 0.4 { // a sine of 0.5 has a level of 0.354
			t.Errorf("%g Hz: the level moves between %.3f and %.3f", hz, low, high)
		}
	}
}

func TestStopsAtTheEndAndStartsAgain(t *testing.T) {
	e := NewEngine(tone(1, 110, 0.3), silence(1))
	e.SetPlaying(true)
	pull(e, 2*Rate)
	if e.Playing() {
		t.Fatal("still playing after the end")
	}
	if at := e.Target(); math.Abs(at-1) > 0.001 {
		t.Fatalf("stopped at %f", at)
	}
	e.SetPlaying(true)
	pull(e, Rate/4)
	if at := e.Target(); at > 0.5 {
		t.Fatalf("started again from %f", at)
	}
}

func TestRepeatsAStretch(t *testing.T) {
	e := NewEngine(tone(10, 110, 0.3), silence(10))
	e.SetLoop(2, 3, true)
	e.Seek(2)
	e.SetPlaying(true)
	for second := 0; second < 6; second++ {
		pull(e, Rate/2)
		if at := e.Target(); at < 1.95 || at > 3.05 {
			t.Fatalf("left the stretch: %f", at)
		}
	}
	e.SetLoop(0, 0, false)
	pull(e, 2*Rate)
	if at := e.Target(); at < 3.5 {
		t.Fatalf("did not go on past the stretch: %f", at)
	}
}

func TestPausedIsSilentAndStaysPut(t *testing.T) {
	e := NewEngine(tone(2, 110, 0.3), silence(2))
	e.Seek(1)
	out := pull(e, Rate/4)
	for i, v := range out {
		if v != 0 {
			t.Fatalf("frame %d sounds while paused", i)
		}
	}
	if at := e.At(e.Consumed()); math.Abs(at-1) > 1e-9 {
		t.Fatalf("moved to %f while paused", at)
	}
}
