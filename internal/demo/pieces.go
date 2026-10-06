package demo

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/library"
	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// Piece is one of the recordings that come with the program.
type Piece struct {
	ID    string
	Title string // the style, which reads the same in both languages
	Key   string // the key, as a pitch name
	BPM   float64
	Beats int // in a bar
	// Bars is the chord of every bar: the root as a MIDI note in the bass register, and the
	// notes of the chord above it for the pad.
	Bars []chord
	// Line writes the bass part: for every bar, the notes to play.
	Line  func(bar int, c chord) []note
	Drums func(bar int, rng *rand.Rand) []hit
	Swing float64 // how late the second eighth of each beat falls, as a fraction of an eighth
}

type chord struct {
	root  int
	above []int // intervals of the pad voicing, in semitones from the root
}

// note is one note of the bass part, in beats from the start of its bar.
type note struct {
	at, length float64
	midi       int
	dead       bool // a muted note: heard, not written
}

// hit is one drum stroke, in beats from the start of its bar.
type hit struct {
	at    float64
	drum  string
	level float64
}

func (p Piece) duration() float64 {
	return (float64(len(p.Bars)+1)*float64(p.Beats) + 1) * 60 / p.BPM
}

// late moves an off-beat eighth later, for a swing feel.
func (p Piece) late(at float64) float64 {
	frac := at - math.Floor(at)
	if math.Abs(frac-0.5) < 0.01 {
		return at + p.Swing*0.5
	}
	return at
}

// Render makes the recording: the bass alone, the rest alone, and the part as written.
func (p Piece) Render() *project.Result {
	beat := 60 / p.BPM
	total := p.duration()
	bass, rest := newTrack(total), newTrack(total)
	rng := rand.New(rand.NewPCG(uint64(len(p.ID)), uint64(p.Beats)))
	var events []transcribe.Event
	// the first bar is the count-in: hi-hat alone
	for b := 0; b < p.Beats; b++ {
		rest.add(float64(b)*beat, hat(0.18, false, rng), 0.3)
	}
	for i, c := range p.Bars {
		start := float64(i+1) * float64(p.Beats) * beat
		for _, n := range p.Line(i, c) {
			at := start + p.late(n.at)*beat
			length := n.length * beat * 0.92
			if n.dead {
				bass.add(at, Bass(n.midi, math.Min(length, 0.12), 0.22, true, rng.Uint64()), 0)
				continue
			}
			bass.add(at, Bass(n.midi, length, 0.5, false, rng.Uint64()), 0)
			events = append(events, transcribe.Event{Start: at, End: at + length, Midi: n.midi, Confidence: 1})
		}
		for _, h := range p.Drums(i, rng) {
			at := start + p.late(h.at)*beat
			switch h.drum {
			case "kick":
				rest.add(at, kick(h.level), 0)
			case "snare":
				rest.add(at, snare(h.level, rng), 0.1)
			case "hat":
				rest.add(at, hat(h.level, false, rng), 0.35)
			case "open":
				rest.add(at, hat(h.level, true, rng), 0.35)
			case "rim":
				rest.add(at, rim(h.level), 0.1)
			case "ride":
				rest.add(at, ride(h.level, rng), -0.3)
			}
		}
		// the pad plays the chord on the first beat of the bar, and again halfway
		voicing := make([]int, len(c.above))
		for k, interval := range c.above {
			voicing[k] = c.root + 12 + interval
		}
		half := float64(p.Beats) * beat / 2
		rest.add(start, Pad(voicing, half*1.1, 0.07), -0.4)
		rest.add(start+half, Pad(voicing, half*1.1, 0.055), -0.4)
	}
	sort.Slice(events, func(a, b int) bool { return events[a].Start < events[b].Start })
	events = transcribe.Normalize(events, total)
	tuning := fretboard.TuningFor("4")
	events = fretboard.Finger(events, tuning, 12)
	return &project.Result{
		Project: &project.Project{
			Reader: project.Reader, Version: "demo", Title: p.Title, Key: p.Key, BuiltIn: true,
			Duration: total, Source: "bass", Tuning: tuning.Key, Frets: 12,
			Rhythm: rhythm.Steady(p.BPM, total, p.Beats), Events: events,
		},
		Bass: bass.buffer(), Backing: rest.buffer(),
	}
}

// Install puts the pieces in a library. Pieces already there are left alone.
func Install(lib *library.Library) ([]string, error) {
	var ids []string
	for _, p := range Pieces {
		if !lib.Has(p.ID) {
			if err := lib.Save(p.ID, p.Render()); err != nil {
				return ids, err
			}
		}
		ids = append(ids, p.ID)
	}
	return ids, nil
}

// Buffers of the two tracks, for the build's trial of the window.
func Buffers(r *project.Result) (*audio.Buffer, *audio.Buffer) { return r.Bass, r.Backing }

// --- the music ---

const (
	e1, f1, fs1, g1, gs1, a1, as1, b1 = 28, 29, 30, 31, 32, 33, 34, 35
	c2, cs2, d2, ds2, e2, f2, fs2, g2 = 36, 37, 38, 39, 40, 41, 42, 43
	gs2, a2, as2, b2, c3, d3          = 44, 45, 46, 47, 48, 50
)

var (
	dom7  = []int{4, 7, 10}
	dom79 = []int{4, 10, 14}
	min7  = []int{3, 7, 10}
	maj7  = []int{4, 7, 11}
	maj   = []int{4, 7}
	minor = []int{3, 7}
)

// Pieces that come with the program, in the order of the list.
var Pieces = []Piece{
	{
		ID: "demo-blues", Title: "Blues", Key: "E", BPM: 96, Beats: 4, Swing: 0.3,
		Bars: twice([]chord{{e1, dom7}, {e1, dom7}, {e1, dom7}, {e1, dom7}, {a1, dom7}, {a1, dom7}, {e1, dom7}, {e1, dom7}, {b1, dom7}, {a1, dom7}, {e1, dom7}, {b1, dom7}}),
		Line: func(bar int, c chord) []note {
			// the boogie line: root, third, fifth, sixth up, flat seventh, and back down
			steps := []int{0, 4, 7, 9, 10, 9, 7, 4}
			if bar%12 == 11 { // the turnaround: a walk up to the five
				return []note{{0, 1, c.root, false}, {1, 1, c.root + 2, false}, {2, 1, c.root + 4, false}, {3, 1, c.root + 5, false}}
			}
			var out []note
			for i, s := range steps {
				out = append(out, note{float64(i) * 0.5, 0.5, c.root + s, false})
			}
			return out
		},
		Drums: func(bar int, rng *rand.Rand) []hit {
			out := []hit{{0, "kick", 0.55}, {1, "snare", 0.4}, {2, "kick", 0.55}, {2.5, "kick", 0.35}, {3, "snare", 0.4}}
			for b := 0.0; b < 4; b += 0.5 {
				out = append(out, hit{b, "ride", 0.14})
			}
			return out
		},
	},
	{
		ID: "demo-funk", Title: "Funk", Key: "E", BPM: 104, Beats: 4,
		Bars: repeat(16, func(i int) chord {
			if i%8 >= 4 && i%8 < 6 {
				return chord{a1, dom79}
			}
			return chord{e1, dom79}
		}),
		Line: func(bar int, c chord) []note {
			r := c.root
			if bar%2 == 0 {
				return []note{{0, 0.75, r, false}, {0.75, 0.25, r, true}, {1, 0.25, r, true}, {1.5, 0.5, r + 12, false}, {2, 0.5, r + 10, false}, {2.75, 0.25, r, true}, {3, 0.5, r + 7, false}, {3.5, 0.5, r + 10, false}}
			}
			return []note{{0, 0.5, r, false}, {0.5, 0.25, r, true}, {0.75, 0.25, r + 12, false}, {1.25, 0.25, r + 12, false}, {1.5, 0.5, r + 10, false}, {2, 0.25, r + 7, false}, {2.5, 0.5, r + 5, false}, {3, 0.25, r + 3, true}, {3.25, 0.25, r + 3, false}, {3.5, 0.5, r + 2, false}}
		},
		Drums: func(bar int, rng *rand.Rand) []hit {
			out := []hit{{0, "kick", 0.6}, {0.75, "kick", 0.4}, {1, "snare", 0.45}, {2.5, "kick", 0.5}, {3, "snare", 0.45}, {3.75, "kick", 0.3}}
			for b := 0.0; b < 4; b += 0.25 {
				level := 0.1
				if math.Mod(b, 0.5) == 0 {
					level = 0.16
				}
				out = append(out, hit{b, "hat", level})
			}
			out = append(out, hit{3.5, "open", 0.12})
			return out
		},
	},
	{
		ID: "demo-bossa", Title: "Bossa nova", Key: "A", BPM: 132, Beats: 4,
		Bars: twice([]chord{{a1, min7}, {a1, min7}, {d2, min7}, {d2, min7}, {e1, dom7}, {e1, dom7}, {a1, min7}, {a1, min7}}),
		Line: func(bar int, c chord) []note {
			fifth := c.root + 7
			if fifth > b1+5 { // keep the fifth under the root where it would climb too high
				fifth -= 12
			}
			return []note{{0, 1.5, c.root, false}, {1.5, 0.5, fifth, false}, {2, 1.5, fifth, false}, {3.5, 0.5, c.root, false}}
		},
		Drums: func(bar int, rng *rand.Rand) []hit {
			out := []hit{{0, "kick", 0.45}, {1.5, "kick", 0.3}, {2, "kick", 0.45}, {3.5, "kick", 0.3}}
			// the clave on the rim, over two bars
			if bar%2 == 0 {
				out = append(out, hit{0, "rim", 0.3}, hit{1.5, "rim", 0.3}, hit{3, "rim", 0.3})
			} else {
				out = append(out, hit{1, "rim", 0.3}, hit{2, "rim", 0.3})
			}
			for b := 0.0; b < 4; b += 0.5 {
				out = append(out, hit{b, "hat", 0.1})
			}
			return out
		},
	},
	{
		ID: "demo-walking", Title: "Walking", Key: "F", BPM: 140, Beats: 4, Swing: 0.33,
		Bars: twice([]chord{{f1, dom7}, {as1, dom7}, {f1, dom7}, {c2, min7}, {as1, dom7}, {as1, dom7}, {f1, dom7}, {d2, dom7}, {g1, min7}, {c2, dom7}, {f1, dom7}, {c2, dom7}}),
		Line: func(bar int, c chord) []note {
			// a walking line: root, third, fifth, then a step into the next bar's root
			r := c.root
			third := 4
			if len(c.above) > 0 && c.above[0] == 3 {
				third = 3
			}
			var fourth int
			switch bar % 12 {
			case 0, 2, 6, 10:
				fourth = r + 5 // up to the fourth, which leads to the next chord
			case 3, 4, 7, 8:
				fourth = r - 1 // a half step under the root that follows
			default:
				fourth = r + 10
			}
			if fourth < e1 {
				fourth += 12
			}
			return []note{{0, 1, r, false}, {1, 1, r + third, false}, {2, 1, r + 7, false}, {3, 1, fourth, false}}
		},
		Drums: func(bar int, rng *rand.Rand) []hit {
			out := []hit{{1, "hat", 0.22}, {3, "hat", 0.22}, {0, "kick", 0.2}, {2, "kick", 0.2}}
			for _, b := range []float64{0, 1, 1.5, 2, 3, 3.5} {
				out = append(out, hit{b, "ride", 0.16})
			}
			return out
		},
	},
	{
		ID: "demo-rock", Title: "Rock", Key: "G", BPM: 120, Beats: 4,
		Bars: twice([]chord{{g1, maj}, {d2, maj}, {e1 + 12, minor}, {c2, maj}, {g1, maj}, {d2, maj}, {c2, maj}, {d2, maj}}),
		Line: func(bar int, c chord) []note {
			r := c.root
			if r > d2 { // the E of the E minor is played low
				r -= 12
			}
			out := []note{}
			for i := 0; i < 6; i++ {
				out = append(out, note{float64(i) * 0.5, 0.5, r, false})
			}
			if bar%4 == 3 {
				return append(out, note{3, 0.5, r + 12, false}, note{3.5, 0.5, r + 7, false})
			}
			return append(out, note{3, 0.5, r + 12, false}, note{3.5, 0.5, r, false})
		},
		Drums: func(bar int, rng *rand.Rand) []hit {
			out := []hit{{0, "kick", 0.6}, {1, "snare", 0.5}, {2, "kick", 0.6}, {2.5, "kick", 0.45}, {3, "snare", 0.5}}
			for b := 0.0; b < 4; b += 0.5 {
				out = append(out, hit{b, "hat", 0.17})
			}
			if bar%4 == 3 {
				out = append(out, hit{3.5, "snare", 0.35}, hit{3.75, "snare", 0.3})
			}
			return out
		},
	},
}

func twice(bars []chord) []chord { return append(append([]chord(nil), bars...), bars...) }

func repeat(n int, f func(i int) chord) []chord {
	out := make([]chord, n)
	for i := range out {
		out[i] = f(i)
	}
	return out
}
