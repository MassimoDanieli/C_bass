// Command demosong puts a made-up recording in the library, with its bass and its drums
// already apart, so that the window can be tried without the model: the build uses it to
// take a picture of the window on each kind of machine.
//
//	CBASS_LIBRARY=folder go run ./tools/demosong     prints the name of the recording in the library
package main

import (
	"fmt"
	"math"
	"os"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/library"
	"github.com/MassimoDanieli/c_bass/internal/project"
)

const (
	rate = 44100
	beat = 60.0 / 104
)

// One bar: when each note starts and how long it lasts, in sixteenths, and how far above
// the root of the bar it is.
var figure = []struct{ at, length, above int }{
	{0, 3, 0}, {3, 1, 0}, {4, 2, 7}, {6, 2, 12}, {8, 4, 10}, {12, 1, 7}, {13, 1, 5}, {14, 2, 3},
}

var roots = []int{33, 33, 36, 38, 33, 31, 29, 28} // A A C D A G F E

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demosong:", err)
		os.Exit(1)
	}
}

func run() error {
	bars := 16
	total := 2*beat + float64(bars)*4*beat + 1
	frames := int(total * rate)
	track := func() *audio.Buffer {
		return &audio.Buffer{SampleRate: rate, Channels: [][]float32{make([]float32, frames), make([]float32, frames)}}
	}
	bass, drums := track(), track()
	add := func(to *audio.Buffer, at, seconds float64, sample func(t float64) float64) {
		start := int(at * rate)
		for i := 0; i < int(seconds*rate) && start+i < frames; i++ {
			v := float32(sample(float64(i) / rate))
			to.Channels[0][start+i] += v
			to.Channels[1][start+i] += v
		}
	}
	sixteenth := beat / 4
	for bar := 0; bar < bars; bar++ {
		for _, n := range figure {
			if bar%4 == 3 && n.at >= 8 { // every fourth bar ends on a long note
				if n.at > 8 {
					continue
				}
				n.length = 8
			}
			f := 440 * math.Pow(2, float64(roots[bar%len(roots)]+n.above-69)/12)
			length := float64(n.length) * sixteenth * 0.92
			add(bass, 2*beat+(float64(bar)*16+float64(n.at))*sixteenth, length, func(t float64) float64 {
				envelope := math.Min(1, t/0.006) * math.Exp(-t*1.4) * math.Min(1, (length-t)/0.02)
				return 0.3 * envelope * (math.Sin(2*math.Pi*f*t) + 0.5*math.Sin(4*math.Pi*f*t) + 0.2*math.Sin(6*math.Pi*f*t))
			})
		}
	}
	seed := uint32(1)
	noise := func() float64 {
		seed = seed*1664525 + 1013904223
		return float64(seed>>8)/float64(1<<23) - 1
	}
	for n := 0; float64(n)*beat < total-0.3; n++ {
		at := float64(n) * beat
		if n%2 == 0 { // bass drum
			add(drums, at, 0.2, func(t float64) float64 {
				return 0.5 * math.Exp(-t*22) * math.Sin(2*math.Pi*(48+70*math.Exp(-t*38))*t)
			})
		} else { // snare
			add(drums, at, 0.14, func(t float64) float64 { return 0.3 * math.Exp(-t*30) * noise() })
		}
		for half := 0; half < 2; half++ { // hi-hat on the eighths
			add(drums, at+float64(half)*beat/2, 0.04, func(t float64) float64 { return 0.07 * math.Exp(-t*90) * noise() })
		}
	}
	mix := track()
	for c := range mix.Channels {
		for i := range mix.Channels[c] {
			mix.Channels[c][i] = bass.Channels[c][i] + drums.Channels[c][i]
		}
	}
	result, err := project.Analyse(mix, "Giro di prova", project.Options{Version: "demo", Bass: bass}, nil)
	if err != nil {
		return err
	}
	result.Bass, result.Backing = bass, drums
	lib, err := library.Open()
	if err != nil {
		return err
	}
	id := "demo"
	if err := lib.Save(id, result); err != nil {
		return err
	}
	fmt.Println(id)
	return nil
}
