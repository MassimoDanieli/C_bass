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

// loudest returns the offset of the loudest sample of a stretch.
func loudest(signal []float32) (int, float32) {
	at, peak := 0, float32(0)
	for i, v := range signal {
		if v < 0 {
			v = -v
		}
		if v > peak {
			at, peak = i, v
		}
	}
	return at, peak
}

// taps is a recording with a short tap on each of the beats given.
func taps(seconds float64, beats []float64) *audio.Buffer {
	out := silence(seconds)
	for _, beat := range beats {
		start := int(beat * Rate)
		for i := 0; i < Rate*3/100; i++ {
			t := float64(i) / Rate
			v := float32(0.4 * math.Exp(-t*120) * math.Sin(2*math.Pi*300*t))
			out.Channels[0][start+i], out.Channels[1][start+i] = v, v
		}
	}
	return out
}

// starts returns where the sound comes back after a silence, in frames.
func starts(signal []float32, threshold float32) []int {
	var out []int
	quiet := 0
	for i, v := range signal {
		if v < 0 {
			v = -v
		}
		if v > threshold {
			if quiet > Rate/20 || len(out) == 0 && quiet == i {
				out = append(out, i)
			}
			quiet = 0
		} else {
			quiet++
		}
	}
	return out
}

// The metronome clicks with the beats of the recording as they are heard: slowed down, the
// recording does not run evenly from piece to piece, and a click placed by the clock would
// part from the drum it belongs to.
func TestTheMetronomeClicksWithTheRecording(t *testing.T) {
	beats := []float64{0.5, 1, 1.5, 2, 2.5, 3, 3.5}
	strong := []bool{true, false, false, false, true, false, false}
	for _, speed := range []float64{1, 0.8, 0.5, 1.2} {
		play := func(metronome bool) []float32 {
			e := NewEngine(taps(5, beats), silence(5))
			e.SetBeats(beats, strong)
			e.SetMetronome(metronome)
			e.SetSpeed(speed)
			e.SetPlaying(true)
			return pull(e, int(4.5/speed*Rate))
		}
		without, with := play(false), play(true)
		clicks := make([]float32, len(with))
		for i := range with {
			clicks[i] = with[i] - without[i] // what the metronome added
		}
		heard, clicked := starts(without, 0.05), starts(clicks, 0.05)
		if len(heard) != len(beats) || len(clicked) != len(beats) {
			t.Fatalf("at %.0f%%: %d taps heard, %d clicks, for %d beats", speed*100, len(heard), len(clicked), len(beats))
		}
		for i := range beats {
			if off := clicked[i] - heard[i]; off < -Rate/200 || off > Rate/200 {
				t.Errorf("at %.0f%% the click of beat %d is %d ms from its tap", speed*100, i+1, off*1000/Rate)
			}
		}
	}
	// turned off, it stops
	e := NewEngine(silence(3), silence(3))
	e.SetBeats(beats, strong)
	e.SetMetronome(true)
	e.SetPlaying(true)
	pull(e, Rate/4)
	e.SetMetronome(false)
	if _, peak := loudest(pull(e, 2*Rate)[Rate/10:]); peak > 0.01 {
		t.Error("the metronome goes on after being turned off")
	}
}

// Counted in, the recording waits for the clicks and then starts from where it was.
func TestTheCountComesBeforeTheRecording(t *testing.T) {
	e := NewEngine(tone(8, 110, 0.3), silence(8))
	e.Seek(2)
	e.PlayCounted(4, 0.5)
	if !e.Counting() {
		t.Fatal("no count under way")
	}
	out := pull(e, 2*Rate-hop)
	for click := 0; click < 4; click++ {
		if _, peak := loudest(out[click*Rate/2 : click*Rate/2+Rate/10]); peak < 0.15 {
			t.Errorf("click %d of the count is missing", click+1)
		}
	}
	if _, peak := loudest(out[Rate/4 : Rate/2-Rate/20]); peak > 0.01 {
		t.Error("the recording sounds during the count")
	}
	if at := e.Target(); math.Abs(at-2) > 1e-6 {
		t.Fatalf("the recording moved to %.2f during the count", at)
	}
	pull(e, Rate)
	if e.Counting() {
		t.Fatal("still counting after four clicks")
	}
	if at := e.Target(); at < 2.7 || at > 3.1 {
		t.Fatalf("a second after the count the recording is at %.2f", at)
	}
	// stopping during a count drops it
	e.SetPlaying(false)
	e.PlayCounted(4, 0.5)
	pull(e, Rate/2)
	e.SetPlaying(false)
	if e.Counting() {
		t.Fatal("the count survived a stop")
	}
	if _, peak := loudest(pull(e, 2*Rate)[Rate/2:]); peak > 0.01 {
		t.Error("clicks go on after a stop")
	}
}

// A stretch repeated goes faster every time round, up to the speed asked and no further.
func TestARepeatedStretchQuickens(t *testing.T) {
	e := NewEngine(tone(10, 110, 0.3), silence(10))
	e.SetLoop(2, 3, true)
	e.Seek(2)
	e.SetSpeed(0.6)
	e.SetQuicken(0.1, 0.9)
	e.SetPlaying(true)
	pull(e, 2*Rate) // once round at 60% takes 1.7 s
	if got := e.Speed(); math.Abs(got-0.7) > 1e-9 {
		t.Fatalf("after once round the speed is %.2f", got)
	}
	pull(e, 12*Rate)
	if got := e.Speed(); math.Abs(got-0.9) > 1e-9 {
		t.Fatalf("after many times round the speed is %.2f, not the 0.90 asked", got)
	}
}

// pitchOf is the frequency of a steady tone, from where it crosses zero going up.
func pitchOf(signal []float32) float64 {
	first, last, count := -1.0, 0.0, 0
	for i := 1; i < len(signal); i++ {
		if signal[i-1] < 0 && signal[i] >= 0 {
			at := float64(i-1) + float64(-signal[i-1])/float64(signal[i]-signal[i-1])
			if first < 0 {
				first = at
			}
			last = at
			count++
		}
	}
	if count < 2 {
		return 0
	}
	return float64(count-1) * Rate / (last - first)
}

// In another key the recording is higher or lower by so many semitones and takes the time it
// took: a second of listening is a second of the recording. Back in its own key it is the
// recording again, sample for sample.
func TestAnotherKeyKeepsTheTime(t *testing.T) {
	bass, rest := tone(6, 110, 0.3), silence(6)
	for _, semitones := range []int{3, -2} {
		e := NewEngine(bass, rest)
		e.SetBeats([]float64{1, 2, 3, 4}, []bool{true, false, false, false})
		e.SetPitch(semitones)
		if e.Pitch() != semitones || math.Abs(e.Duration()-6) > 1e-9 {
			t.Fatalf("pitch %d, duration %f", e.Pitch(), e.Duration())
		}
		e.SetPlaying(true)
		out := pull(e, 3*Rate)
		want := 110 * math.Pow(2, float64(semitones)/12)
		if got := pitchOf(out[Rate/2:]); math.Abs(got/want-1) > 0.004 {
			t.Errorf("%+d semitones: %.2f Hz, want %.2f", semitones, got, want)
		}
		if at := e.At(2 * Rate); math.Abs(at-2) > 0.05 {
			t.Errorf("%+d semitones: two seconds in, the place is %.3f", semitones, at)
		}
		var peak float32
		for _, v := range out[Rate:] {
			peak = max(peak, max(v, -v))
		}
		if peak < 0.2 || peak > 0.4 {
			t.Errorf("%+d semitones: the level went from 0.3 to %.2f", semitones, peak)
		}
		e.Seek(4)
		if at := e.Target(); math.Abs(at-4) > 0.01 {
			t.Errorf("%+d semitones: sent to 4 s, it is at %.3f", semitones, at)
		}
		e.SetLoop(1, 2, true)
		e.Seek(1.5)
		pull(e, Rate)
		if at := e.Target(); at < 1 || at > 2.05 {
			t.Errorf("%+d semitones: repeating from 1 to 2, it is at %.3f", semitones, at)
		}
		e.SetLoop(0, 0, false)
		e.SetPitch(0)
		e.Seek(0)
		back := pull(e, Rate)
		for i := hop; i < len(back); i++ {
			if math.Abs(float64(back[i]-bass.Channels[0][i])) > 1e-5 {
				t.Fatalf("back in its own key, frame %d is %f, not %f", i, back[i], bass.Channels[0][i])
			}
		}
	}
}

// The metronome still clicks on the beats of the recording when the key is changed.
func TestTheMetronomeInAnotherKey(t *testing.T) {
	beats := []float64{0.5, 1.0, 1.5, 2.0, 2.5}
	e := NewEngine(silence(4), silence(4))
	e.SetBeats(beats, make([]bool, len(beats)))
	e.SetMetronome(true)
	e.SetPitch(4)
	e.SetPlaying(true)
	found := starts(pull(e, 3*Rate), 0.05)
	if len(found) != len(beats) {
		t.Fatalf("%d clicks for %d beats", len(found), len(beats))
	}
	for i, at := range found {
		if math.Abs(float64(at)/Rate-beats[i]) > 0.03 {
			t.Errorf("click %d at %.3f, beat at %.3f", i, float64(at)/Rate, beats[i])
		}
	}
}
