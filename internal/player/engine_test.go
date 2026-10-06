package player

import (
	"encoding/binary"
	"math"
	"sort"
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
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

// groove makes a bass line of notes of several lengths, one straight after the other, and
// the drums to go with it: the kind of thing that is slowed down to be learned.
func groove(bars int) (bass, drums *audio.Buffer) {
	const beat = 60.0 / 104
	total := float64(bars)*4*beat + 1
	bass, drums = silence(total), silence(total)
	add := func(to *audio.Buffer, at, seconds float64, sample func(t float64) float64) {
		start := int(at * Rate)
		for i := 0; i < int(seconds*Rate) && start+i < to.Len(); i++ {
			v := float32(sample(float64(i) / Rate))
			to.Channels[0][start+i] += v
			to.Channels[1][start+i] += v
		}
	}
	figure := []struct{ at, length, above int }{{0, 3, 0}, {3, 1, 0}, {4, 2, 7}, {6, 2, 12}, {8, 4, 10}, {12, 1, 7}, {13, 1, 5}, {14, 2, 3}}
	roots := []int{33, 36, 38, 31}
	for bar := 0; bar < bars; bar++ {
		for _, n := range figure {
			hz := 440 * math.Pow(2, float64(roots[bar%len(roots)]+n.above-69)/12)
			length := float64(n.length) * beat / 4 * 0.92
			add(bass, 0.3+(float64(bar)*16+float64(n.at))*beat/4, length, func(t float64) float64 {
				envelope := math.Min(1, t/0.006) * math.Exp(-t*1.4) * math.Min(1, (length-t)/0.02)
				return 0.3 * envelope * (math.Sin(2*math.Pi*hz*t) + 0.5*math.Sin(4*math.Pi*hz*t) + 0.2*math.Sin(6*math.Pi*hz*t))
			})
		}
	}
	seed := uint32(1)
	noise := func() float64 {
		seed = seed*1664525 + 1013904223
		return float64(seed>>8)/float64(1<<23) - 1
	}
	for n := 0; float64(n)*beat/2 < total-0.3; n++ {
		at := 0.3 + float64(n)*beat/2
		add(drums, at, 0.04, func(t float64) float64 { return 0.07 * math.Exp(-t*90) * noise() })
		if n%4 == 0 {
			add(drums, at, 0.2, func(t float64) float64 {
				return 0.5 * math.Exp(-t*22) * math.Sin(2*math.Pi*(48+70*math.Exp(-t*38))*t)
			})
		}
	}
	return bass, drums
}

// played returns the bass as it comes out at a speed, the drums being there but silent.
func played(bass, drums *audio.Buffer, speed float64) *audio.Buffer {
	e := NewEngine(bass, drums)
	e.SetGains(1, 0)
	e.SetSpeed(speed)
	e.SetPlaying(true)
	left := pull(e, int(bass.Duration()/speed*Rate))
	return &audio.Buffer{SampleRate: Rate, Channels: [][]float32{left, left}}
}

// Slowed down, every note must still be struck once. Slowing down plays some of the
// recording twice, and if that is the start of a note, two notes are heard: the notes are
// read back from what comes out, and there must be as many as went in, at an even pace.
func TestSlowedDownEachNoteIsStruckOnce(t *testing.T) {
	bass, drums := groove(8)
	written := transcribe.Transcribe(bass, transcribe.Options{Isolated: true})
	if len(written) != 64 {
		t.Fatalf("the line has 64 notes, %d were read", len(written))
	}
	for _, speed := range []float64{0.9, 0.8, 0.7, 0.6, 0.5, 1.2} {
		heard := transcribe.Transcribe(played(bass, drums, speed), transcribe.Options{Isolated: true})
		if len(heard) != len(written) {
			t.Errorf("at %.0f%% the %d notes came out as %d", speed*100, len(written), len(heard))
			continue
		}
		var early []float64
		for i := range heard {
			if heard[i].Midi != written[i].Midi {
				t.Errorf("at %.0f%% note %d came out as %d, not %d", speed*100, i, heard[i].Midi, written[i].Midi)
			}
			early = append(early, written[i].Start/speed-heard[i].Start)
		}
		sort.Float64s(early)
		if spread := early[len(early)-1] - early[0]; spread > 0.12 {
			t.Errorf("at %.0f%% the notes are up to %.0f ms out of step with one another", speed*100, spread*1000)
		}
	}
}
