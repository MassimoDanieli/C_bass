// Command testtone makes a short recording with a known bass line, and checks that an
// analysis of it found that line. It is what the build uses to try the whole program, model
// included, on each kind of machine.
//
//	go run ./tools/testtone trial.wav             writes the recording
//	go run ./tools/testtone -check trial.cbass.json   checks the analysis
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/MassimoDanieli/c_bass/internal/audio"
)

// The line: quarter notes at 120 BPM, one bar each of A1, C2, D2 and E1, played three times
// after a bar of drums alone. The last note of each bar is held for two beats.
var line = []int{33, 36, 38, 28}

const (
	rate  = 44100
	beat  = 0.5
	total = 26.0
)

type note struct {
	start, length float64
	midi          int
}

func notes() []note {
	var out []note
	for bar := 0; bar < 12; bar++ {
		at := 2 + float64(bar)*4*beat
		midi := line[bar%len(line)]
		out = append(out, note{at, beat * 0.9, midi}, note{at + beat, beat * 0.9, midi}, note{at + 2*beat, 2 * beat * 0.95, midi})
	}
	return out
}

func write(path string) error {
	left := make([]float32, int(total*rate))
	right := make([]float32, len(left))
	add := func(at float64, seconds float64, sample func(t float64) float64) {
		start := int(at * rate)
		for i := 0; i < int(seconds*rate) && start+i < len(left); i++ {
			v := float32(sample(float64(i) / rate))
			left[start+i] += v
			right[start+i] += v
		}
	}
	// The bass: a plucked string, a fundamental with a few harmonics.
	for _, n := range notes() {
		f := 440 * math.Pow(2, float64(n.midi-69)/12)
		length := n.length
		add(n.start, length, func(t float64) float64 {
			envelope := math.Min(1, t/0.008) * math.Exp(-t*1.6) * math.Min(1, (length-t)/0.03)
			return 0.32 * envelope * (math.Sin(2*math.Pi*f*t) + 0.5*math.Sin(4*math.Pi*f*t) + 0.25*math.Sin(6*math.Pi*f*t))
		})
	}
	// Drums: a kick on every beat, a snare of noise on two and four, a hi-hat on the eighths.
	seed := uint32(1)
	noise := func() float64 { seed = seed*1664525 + 1013904223; return float64(seed>>8)/float64(1<<23) - 1 }
	for i := 0; float64(i)*beat < total-0.3; i++ {
		at := float64(i) * beat
		add(at, 0.18, func(t float64) float64 {
			return 0.5 * math.Exp(-t*22) * math.Sin(2*math.Pi*(48+90*math.Exp(-t*40))*t)
		})
		if i%2 == 1 {
			add(at, 0.16, func(t float64) float64 { return 0.25 * math.Exp(-t*28) * noise() })
		}
		for _, off := range []float64{0, beat / 2} {
			add(at+off, 0.04, func(t float64) float64 { return 0.06 * math.Exp(-t*120) * noise() })
		}
	}
	// A chord held above it all, so there is something else to tell the bass from.
	for _, f := range []float64{329.63, 415.3, 493.88} {
		add(0, total, func(t float64) float64 { return 0.04 * math.Sin(2*math.Pi*f*t) })
	}
	return audio.WriteWAV(path, &audio.Buffer{SampleRate: rate, Channels: [][]float32{left, right}})
}

func check(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var project struct {
		Source string
		Rhythm struct{ Beats []float64 }
		Events []struct {
			Start, End float64
			Midi       int
		}
	}
	if err := json.Unmarshal(data, &project); err != nil {
		return err
	}
	if project.Source != "bass" {
		return fmt.Errorf("the notes were read from %q, not from the isolated bass", project.Source)
	}
	want := notes()
	found := 0
	for _, n := range want {
		for _, event := range project.Events {
			if math.Abs(event.Start-n.start) < 0.08 && event.Midi%12 == n.midi%12 {
				found++
				break
			}
		}
	}
	fmt.Printf("%d of %d notes found, among %d read; %d beats\n", found, len(want), len(project.Events), len(project.Rhythm.Beats))
	if found < len(want)*9/10 {
		return fmt.Errorf("only %d of the %d notes of the line were found", found, len(want))
	}
	if len(project.Events) > len(want)*13/10 {
		return fmt.Errorf("%d notes were read where the line has %d", len(project.Events), len(want))
	}
	if n := len(project.Rhythm.Beats); n < 44 || n > 60 {
		return fmt.Errorf("%d beats were found in 26 seconds at 120 BPM", n)
	}
	return nil
}

func main() {
	var err error
	switch {
	case len(os.Args) == 3 && os.Args[1] == "-check":
		err = check(os.Args[2])
	case len(os.Args) == 2:
		err = write(os.Args[1])
	default:
		err = fmt.Errorf("usage: testtone <out.wav> | testtone -check <analysis.cbass.json>")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "testtone:", err)
		os.Exit(1)
	}
}
