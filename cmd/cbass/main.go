// Command cbass turns a recording into a bass part: the bass isolated from the rest, its notes,
// the bars they fall in, and a tablature.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/demucs"
	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/provision"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/tab"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// version is set at build time.
var version = "dev"

// Project is everything worked out about one recording.
type Project struct {
	Version  string             `json:"cbass"`
	Title    string             `json:"title"`
	Duration float64            `json:"duration"`
	Source   string             `json:"source"` // "bass" when read from the isolated bass, "mix" otherwise
	Tuning   string             `json:"tuning"`
	Frets    int                `json:"frets"`
	Rhythm   *rhythm.Rhythm     `json:"rhythm,omitempty"`
	Events   []transcribe.Event `json:"events"`
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		usage()
		return
	}
	switch os.Args[1] {
	case "version", "--version":
		fmt.Println("cbass", version)
	case "analyse", "analyze":
		if err := analyse(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "cbass:", err)
			os.Exit(1)
		}
	default:
		// "analyse" may be left out: anything else is taken as its options and the recording.
		if err := analyse(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "cbass:", err)
			os.Exit(1)
		}
	}
}

func usage() {
	fmt.Print(`cbass ` + version + ` — from a recording to a bass part

  cbass [options] <recording.mp3|.wav>

Separates the bass from the recording, reads its notes, finds the bars, and writes
next to the recording (or in the folder given with -o):

  <name>.cbass.json    notes, bars and fingering
  <name>.tab.txt       the tablature
  <name>.no-bass.wav   the recording without its bass      (with -stems)
  <name>.bass.wav      the bass on its own                 (with -stems)

Options:
`)
	flags, _ := analyseFlags()
	flags.PrintDefaults()
}

type settings struct {
	out, tuning, bass   string
	frets, threads, bar int
	sensitivity         float64
	stems, mix, coreML  bool
}

func analyseFlags() (*flag.FlagSet, *settings) {
	s := &settings{}
	flags := flag.NewFlagSet("analyse", flag.ContinueOnError)
	flags.StringVar(&s.out, "o", "", "folder to write to (default: beside the recording)")
	flags.BoolVar(&s.stems, "stems", false, "also write the recording without its bass, and the bass alone")
	flags.BoolVar(&s.mix, "mix", false, "do not separate: read the notes from the full recording (faster, far less accurate)")
	flags.StringVar(&s.bass, "bass", "", "a bass already isolated, to use instead of separating")
	flags.StringVar(&s.tuning, "tuning", "4", "instrument: 4 (EADG), 5 (BEADG), 5c (EADGC), 6 (BEADGC)")
	flags.IntVar(&s.frets, "frets", 12, "highest fret to use")
	flags.IntVar(&s.bar, "beats", 4, "beats in a bar")
	flags.Float64Var(&s.sensitivity, "sensitivity", 0.72, "how readily a new attack starts a note, 0.55 to 0.90")
	flags.IntVar(&s.threads, "threads", 0, "processor threads for the separation (default: all)")
	flags.BoolVar(&s.coreML, "coreml", false, "macOS, experimental: let CoreML run the model (on the build machines it never finished)")
	return flags, s
}

func analyse(args []string) error {
	flags, s := analyseFlags()
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("give one recording to analyse")
	}
	path := flags.Arg(0)
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	out := s.out
	if out == "" {
		out = filepath.Dir(path)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	began := time.Now()
	step := func(format string, values ...any) {
		fmt.Printf("%6.1fs  %s\n", time.Since(began).Seconds(), fmt.Sprintf(format, values...))
	}

	mix, err := audio.Decode(path)
	if err != nil {
		return err
	}
	mix = audio.Resample(mix.Stereo(), demucs.SampleRate)
	step("read %s: %.1f seconds", filepath.Base(path), mix.Duration())

	source, isolated := mix, false
	switch {
	case s.bass != "":
		bass, err := audio.Decode(s.bass)
		if err != nil {
			return err
		}
		source, isolated = bass, true
	case !s.mix:
		stems, err := separate(mix, s)
		if err != nil {
			return err
		}
		step("separated the bass")
		source, isolated = stems["bass"], true
		if s.stems {
			backing := &audio.Buffer{SampleRate: mix.SampleRate, Channels: [][]float32{make([]float32, mix.Len()), make([]float32, mix.Len())}}
			for _, stem := range []string{"drums", "other", "vocals"} {
				for c, channel := range stems[stem].Channels {
					for i, v := range channel {
						backing.Channels[c][i] += v
					}
				}
			}
			if err := audio.WriteWAV(filepath.Join(out, name+".no-bass.wav"), backing); err != nil {
				return err
			}
			if err := audio.WriteWAV(filepath.Join(out, name+".bass.wav"), stems["bass"]); err != nil {
				return err
			}
			step("wrote the two recordings")
		}
	}

	events := transcribe.Transcribe(source, transcribe.Options{Isolated: isolated, Sensitivity: s.sensitivity})
	if len(events) == 0 {
		return fmt.Errorf("no notes were found in %s", filepath.Base(path))
	}
	tuning := fretboard.TuningFor(s.tuning)
	events = fretboard.Finger(events, tuning, s.frets)
	step("read %d notes", len(events))

	pulse := rhythm.Analyse(mix, s.bar)
	if pulse == nil {
		pulse = rhythm.Steady(120, mix.Duration(), s.bar)
		step("no steady beat was found: bars are written at 120 BPM")
	} else {
		step("found the beat: %.0f BPM, %d beats", pulse.Tempo(), len(pulse.Beats))
	}

	project := Project{
		Version: version, Title: name, Duration: mix.Duration(), Source: map[bool]string{true: "bass", false: "mix"}[isolated],
		Tuning: tuning.Key, Frets: s.frets, Rhythm: pulse, Events: events,
	}
	data, err := json.MarshalIndent(project, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, name+".cbass.json"), data, 0o644); err != nil {
		return err
	}
	text := tab.Text(name, events, pulse, tuning, 4)
	if err := os.WriteFile(filepath.Join(out, name+".tab.txt"), []byte(text), 0o644); err != nil {
		return err
	}
	step("wrote %s and %s in %s", name+".cbass.json", name+".tab.txt", out)
	return nil
}

func separate(mix *audio.Buffer, s *settings) (demucs.Stems, error) {
	shown := int64(-1)
	download := func(what string, done, total int64) {
		if done>>20 == shown && done != total {
			return
		}
		shown = done >> 20
		if total > 0 {
			fmt.Printf("\r        downloading %s: %d of %d MB", what, done>>20, total>>20)
		} else {
			fmt.Printf("\r        downloading %s: %d MB", what, done>>20)
		}
		if done == total {
			fmt.Println()
		}
	}
	library, err := provision.Runtime(download)
	if err != nil {
		return nil, err
	}
	model, err := provision.Model(download)
	if err != nil {
		return nil, err
	}
	separator, err := demucs.Open(demucs.Options{Library: library, Model: model, Threads: s.threads, CoreML: s.coreML})
	if err != nil {
		return nil, err
	}
	defer separator.Close()
	stems, err := separator.Separate(mix, func(done, total int) {
		fmt.Printf("\r        separating: %d of %d", done, total)
		if done == total {
			fmt.Println()
		}
	})
	return stems, err
}
