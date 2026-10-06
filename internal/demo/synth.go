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

// Sound is the version of the instruments below: when it goes up, the pieces already in a
// library are played again with the new ones.
const Sound = 2

// stereo is a sound with a left and a right.
type stereo struct{ l, r []float32 }

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
func (t *track) add(at float64, sound []float32, pan, gain float64) {
	start := int(at * Rate)
	l, r := float32(gain*math.Sqrt((1-pan)/2)), float32(gain*math.Sqrt((1+pan)/2))
	for i, v := range sound {
		if j := start + i; j >= 0 && j < len(t.left) {
			t.left[j] += v * l
			t.right[j] += v * r
		}
	}
}

// addStereo mixes in a sound that has its own left and right.
func (t *track) addStereo(at float64, sound stereo, gain float64) {
	start := int(at * Rate)
	g := float32(gain)
	for i := range sound.l {
		if j := start + i; j >= 0 && j < len(t.left) {
			t.left[j] += sound.l[i] * g
			t.right[j] += sound.r[i] * g
		}
	}
}

// filter is a two-pole filter (the usual "biquad").
type filter struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func newFilter(kind string, frequency, q float64) *filter {
	w := 2 * math.Pi * frequency / Rate
	alpha := math.Sin(w) / (2 * q)
	c := math.Cos(w)
	var b0, b1, b2 float64
	switch kind {
	case "low":
		b0, b1, b2 = (1-c)/2, 1-c, (1-c)/2
	case "high":
		b0, b1, b2 = (1+c)/2, -(1 + c), (1+c)/2
	default: // "band"
		b0, b1, b2 = alpha, 0, -alpha
	}
	a0 := 1 + alpha
	return &filter{b0: b0 / a0, b1: b1 / a0, b2: b2 / a0, a1: -2 * c / a0, a2: (1 - alpha) / a0}
}

func (f *filter) run(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1, f.y2, f.y1 = f.x1, x, f.y1, y
	return y
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

// --- strings ---

// a string as it is plucked and heard: where along its length the finger takes it, where it
// is listened to (a pickup; 0 for none), how fast it dies away and how much faster its
// overtones do.
type stringKind struct {
	pluck, pickup float64
	slope         float64 // how fast overtones fall off in level: 1 for a pickup, more for a mellow string
	decay, higher float64 // per second, for the fundamental; and added for each overtone, by its number squared
	top           float64 // Hz: nothing above this
	stiff         float64 // how far the overtones are stretched sharp
}

var (
	bassString  = stringKind{pluck: 0.16, pickup: 0.23, slope: 1, decay: 0.7, higher: 0.03, top: 5000, stiff: 0.00006}
	nylonString = stringKind{pluck: 0.27, slope: 1.45, decay: 1.5, higher: 0.03, top: 6000, stiff: 0.00003}
)

// vibrate is a plucked string, built from its overtones: each one a sine that starts where
// the pluck leaves it and dies away at its own speed, the high ones first. This is what a
// string does; made this way it is in tune and has no hiss in it.
func vibrate(kind stringKind, frequency, seconds float64, damped bool) []float64 {
	n := int(seconds * Rate)
	out := make([]float64, n)
	partials := min(32, int(kind.top/frequency))
	for h := 1; h <= partials; h++ {
		number := float64(h)
		level := math.Sin(number*math.Pi*kind.pluck) / math.Pow(number, kind.slope)
		if kind.pickup > 0 {
			level *= math.Sin(number * math.Pi * kind.pickup)
		}
		if math.Abs(level) < 0.002 {
			continue
		}
		f := number * frequency * math.Sqrt(1+kind.stiff*number*number)
		decay := kind.decay + frequency/160 + kind.higher*number*number
		if damped { // the hand on the string: everything dies at once
			decay = 38 + 9*number
		}
		// a dying sine, sample by sample: s[i] = 2g·cos(w)·s[i-1] − g²·s[i-2]
		w := 2 * math.Pi * f / Rate
		g := math.Exp(-decay / Rate)
		k1, k2 := 2*g*math.Cos(w), g*g
		s2, s1 := 0.0, level*g*math.Sin(w)
		for i := 1; i < n; i++ {
			out[i] += s1
			s2, s1 = s1, k1*s1-k2*s2
		}
	}
	return out
}

// Bass is a note of an electric bass played with the fingers: the string, the touch of the
// finger leaving it, and a little warmth from the amplifier. Dead notes are the string
// struck with the hand on it.
func Bass(midi int, seconds, level float64, dead bool, seed uint64) []float32 {
	rng := rand.New(rand.NewPCG(seed, 7))
	frequency := 440 * math.Pow(2, float64(midi-69)/12)
	wave := vibrate(bassString, frequency, seconds, dead)
	touch := newFilter("low", 900, 0.7)
	speaker := newFilter("low", 4200, 0.7)
	rumble := newFilter("high", 28, 0.7)
	thump := 0.10
	if dead {
		thump = 0.22
	}
	out := make([]float32, len(wave))
	for i, v := range wave {
		t := float64(i) / Rate
		v = 0.62*v + thump*math.Exp(-t/0.005)*touch.run(rng.Float64()*2-1)
		v = math.Tanh(1.5*v) / 1.5
		out[i] = float32(level * rumble.run(speaker.run(v)))
	}
	shape(out, 0.0015, 0.025)
	return out
}

// guitar is a chord on a nylon-string guitar, plucked with the fingers: the strings leave a
// few thousandths of a second apart, low to high.
func guitar(midis []int, seconds, level float64) stereo {
	n := int(seconds * Rate)
	sum := make([]float64, n)
	for k, midi := range midis {
		frequency := 440 * math.Pow(2, float64(midi-69)/12)
		late := int(float64(k) * 0.007 * Rate)
		if late >= n {
			break
		}
		for i, v := range vibrate(nylonString, frequency, seconds-float64(late)/Rate, false) {
			sum[late+i] += v
		}
	}
	body := newFilter("band", 190, 1.2) // the box of the guitar answers low down
	air := newFilter("low", 5200, 0.7)
	out := stereo{make([]float32, n), make([]float32, n)}
	for i, v := range sum {
		v = air.run(v + 0.5*body.run(v))
		out.l[i], out.r[i] = float32(level*v*0.8), float32(level*v*0.6)
	}
	shape(out.l, 0.001, 0.05)
	shape(out.r, 0.001, 0.05)
	return out
}

// --- keys ---

// piano is a chord on an electric piano: a tine that barks when struck hard and rings soft
// after, a bell on top of the attack, and the slow side-to-side sway such pianos have.
func piano(midis []int, seconds, level, hardness, at float64) stereo {
	n := int(seconds * Rate)
	out := stereo{make([]float32, n), make([]float32, n)}
	for k, midi := range midis {
		f := 440 * math.Pow(2, float64(midi-69)/12)
		late := float64(k) * 0.004
		die := 0.9 + f/450
		for i := 0; i < n; i++ {
			t := float64(i)/Rate - late
			if t < 0 {
				continue
			}
			phase := 2 * math.Pi * f * t
			bark := (0.5+1.7*hardness)*math.Exp(-t*6) + 0.22
			v := math.Sin(phase + bark*math.Sin(phase))
			v += 0.10 * hardness * math.Exp(-t*22) * math.Sin(14*phase)
			v *= math.Exp(-t*die) * math.Min(1, t/0.003)
			sway := 0.16 * math.Sin(2*math.Pi*4.6*(at+float64(i)/Rate))
			out.l[i] += float32(level * v * (1 + sway))
			out.r[i] += float32(level * v * (1 - sway))
		}
	}
	shape(out.l, 0.001, 0.07)
	shape(out.r, 0.001, 0.07)
	return out
}

// organ is a chord on a drawbar organ through a turning speaker: a few pure pipes for each
// key, the speaker's slow wobble of pitch and loudness, and as much grit as asked for.
func organ(midis []int, seconds, level, grit, turn, at float64) stereo {
	n := int(seconds * Rate)
	out := stereo{make([]float32, n), make([]float32, n)}
	bars := [][2]float64{{1, 1}, {2, 0.62}, {3, 0.4}, {4, 0.2}, {6, 0.08}}
	click := rand.New(rand.NewPCG(uint64(len(midis)), 3))
	for i := 0; i < n; i++ {
		t := float64(i) / Rate
		spin := 2 * math.Pi * turn * (at + t)
		var l, r float64
		for _, midi := range midis {
			f := 440 * math.Pow(2, float64(midi-69)/12)
			for _, bar := range bars {
				phase := 2 * math.Pi * f * bar[0] * t
				wobble := 0.0016 * f * bar[0] / turn
				l += bar[1] * math.Sin(phase+wobble*math.Sin(spin))
				r += bar[1] * math.Sin(phase+wobble*math.Sin(spin+2.1))
			}
			// the knock of the key going down
			pop := 0.5 * math.Exp(-t*11) * math.Sin(2*math.Pi*f*2*t)
			l, r = l+pop, r+pop
		}
		scale := 1 / (2.3 * float64(len(midis)))
		l, r = l*scale*(1+0.14*math.Sin(spin)), r*scale*(1+0.14*math.Sin(spin+2.1))
		if t < 0.003 {
			tick := 0.08 * (click.Float64()*2 - 1)
			l, r = l+tick, r+tick
		}
		if grit > 0 {
			l, r = math.Tanh(grit*l)/grit, math.Tanh(grit*r)/grit
		}
		out.l[i], out.r[i] = float32(level*l), float32(level*r)
	}
	shape(out.l, 0.006, 0.05)
	shape(out.r, 0.006, 0.05)
	return out
}

// --- drums ---

func kick(level float64, rng *rand.Rand) []float32 {
	out := make([]float32, int(0.34*Rate))
	beater := newFilter("band", 2600, 0.8)
	phase := 0.0
	for i := range out {
		t := float64(i) / Rate
		f := 54 + 90*math.Exp(-t/0.02) + 160*math.Exp(-t/0.004)
		phase += 2 * math.Pi * f / Rate
		v := math.Exp(-t/0.1)*math.Sin(phase) + 0.35*math.Exp(-t/0.004)*beater.run(rng.Float64()*2-1)
		out[i] = float32(level * math.Tanh(1.6*v) / 1.6 * 1.25)
	}
	shape(out, 0.0005, 0.05)
	return out
}

func snare(level float64, rng *rand.Rand) []float32 {
	out := make([]float32, int(0.30*Rate))
	low, high := newFilter("high", 1400, 0.7), newFilter("low", 9000, 0.7)
	p1, p2 := 0.0, 0.0
	for i := range out {
		t := float64(i) / Rate
		p1 += 2 * math.Pi * (186 + 60*math.Exp(-t/0.012)) / Rate
		p2 += 2 * math.Pi * (331 + 90*math.Exp(-t/0.012)) / Rate
		skin := math.Exp(-t/0.055) * (0.6*math.Sin(p1) + 0.35*math.Sin(p2))
		wires := (0.75*math.Exp(-t/0.045) + 0.25*math.Exp(-t/0.14)) * high.run(low.run(rng.Float64()*2-1))
		out[i] = float32(level * (0.55*skin + 0.8*wires))
	}
	shape(out, 0.0005, 0.03)
	return out
}

// metal is the raw ring of a cymbal: six tones that agree on nothing, as struck brass does.
func metal(n int, pitch float64, rng *rand.Rand) []float64 {
	out := make([]float64, n)
	for _, f := range []float64{205.3, 304.4, 369.6, 522.7, 540, 800} {
		step := f * pitch * (1 + 0.01*(rng.Float64()-0.5)) / Rate
		at := rng.Float64()
		for i := range out {
			at += step
			at -= math.Floor(at)
			if at < 0.5 {
				out[i] += 1.0 / 6
			} else {
				out[i] -= 1.0 / 6
			}
		}
	}
	return out
}

func hat(level float64, open bool, rng *rand.Rand) []float32 {
	length, fall := 0.09, 0.022
	if open {
		length, fall = 0.42, 0.13
	}
	out := make([]float32, int(length*Rate))
	ring := metal(len(out), 10.5, rng)
	h1, h2 := newFilter("high", 7200, 0.8), newFilter("high", 6000, 0.7)
	for i := range out {
		t := float64(i) / Rate
		v := h2.run(h1.run(ring[i] + 0.35*(rng.Float64()*2-1)))
		out[i] = float32(level * 1.6 * math.Exp(-t/fall) * v)
	}
	shape(out, 0.0005, 0.02)
	return out
}

func ride(level float64, rng *rand.Rand) []float32 {
	out := make([]float32, int(1.1*Rate))
	ring := metal(len(out), 7.3, rng)
	wash, floor := newFilter("band", 5600, 0.6), newFilter("high", 2800, 0.7)
	for i := range out {
		t := float64(i) / Rate
		v := floor.run(wash.run(ring[i] + 0.2*(rng.Float64()*2-1)))
		// the tap of the stick, then the cymbal ringing on, with the bell just heard in it
		bell := 0.05 * math.Exp(-t/0.25) * (math.Sin(2*math.Pi*3120*t) + 0.6*math.Sin(2*math.Pi*4710*t))
		out[i] = float32(level * 2.2 * ((0.7*math.Exp(-t/0.018)+0.3*math.Exp(-t/0.38))*v + bell))
	}
	shape(out, 0.0005, 0.1)
	return out
}

// rim is the stick laid across the snare and tapped on its edge.
func rim(level float64, rng *rand.Rand) []float32 {
	out := make([]float32, int(0.09*Rate))
	knock := newFilter("band", 1900, 2.5)
	for i := range out {
		t := float64(i) / Rate
		v := 0.9*math.Exp(-t/0.002)*knock.run(rng.Float64()*2-1)*3 + math.Exp(-t/0.016)*(0.6*math.Sin(2*math.Pi*470*t)+0.4*math.Sin(2*math.Pi*1730*t))
		out[i] = float32(level * 0.8 * v)
	}
	shape(out, 0.0003, 0.02)
	return out
}

// shaker is a handful of seeds thrown forward and caught.
func shaker(level float64, rng *rand.Rand) []float32 {
	out := make([]float32, int(0.11*Rate))
	high, low := newFilter("high", 4800, 0.7), newFilter("low", 11000, 0.7)
	for i := range out {
		t := float64(i) / Rate
		env := math.Min(1, t/0.018) * math.Exp(-math.Max(0, t-0.018)/0.022)
		out[i] = float32(level * 1.3 * env * low.run(high.run(rng.Float64()*2-1)))
	}
	return out
}

// --- the room ---

// room gives a track the sound of a room: its echoes, too close together to be told apart,
// added quietly behind it. Only what is above the bass register is sent there, or it booms.
func (t *track) room(amount float64) {
	n := len(t.left)
	send := make([]float64, n)
	thin := newFilter("high", 320, 0.7)
	for i := range send {
		send[i] = thin.run(float64(t.left[i]+t.right[i]) * 0.5)
	}
	for side, channel := range [][]float32{t.left, t.right} {
		wet := make([]float64, n)
		for _, length := range []int{1557, 1617, 1491, 1422, 1277, 1356} {
			line := make([]float64, length+side*23)
			k, damp := 0, 0.0
			for i, x := range send {
				y := line[k]
				damp = 0.7*y + 0.3*damp // the walls keep the highs
				line[k] = x + 0.74*damp
				wet[i] += y
				k = (k + 1) % len(line)
			}
		}
		for _, length := range []int{225, 556, 441} {
			line := make([]float64, length+side*11)
			k := 0
			for i, x := range wet {
				delayed := line[k]
				line[k] = x + 0.5*delayed
				wet[i] = delayed - 0.5*x
				k = (k + 1) % len(line)
			}
		}
		for i := range channel {
			channel[i] += float32(amount * wet[i] / 6)
		}
	}
}

// level brings a track to a loudness (its average, as a fraction of full scale), rounding
// off the peaks that would not fit instead of cutting them.
func (t *track) level(loudness, ceiling float64) {
	var sum float64
	for i := range t.left {
		sum += float64(t.left[i])*float64(t.left[i]) + float64(t.right[i])*float64(t.right[i])
	}
	if sum == 0 {
		return
	}
	gain := loudness / math.Sqrt(sum/float64(2*len(t.left)))
	for _, channel := range [][]float32{t.left, t.right} {
		for i, v := range channel {
			channel[i] = float32(ceiling * math.Tanh(float64(v)*gain/ceiling))
		}
	}
}
