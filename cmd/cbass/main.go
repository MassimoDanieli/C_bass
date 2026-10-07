// Command cbass turns a recording into a bass part: the bass isolated from the rest, its notes,
// the bars they fall in, and a tablature.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/tab"
)

// version is set at build time.
var version = "dev"

func main() {
	// The recordings held in memory are few and large: tidying up sooner costs little time and
	// keeps the program from holding twice what it needs.
	debug.SetGCPercent(25)
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

  cbass [options] <recording.mp3|.wav|folder> [more...]

Separates the bass from the recording, reads its notes, finds the bars, and writes
next to the recording (or in the folder given with -o). Given several recordings, or a
folder, it does them one after the other:

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

// recordings lists what there is to analyse at some paths: the files themselves, and the
// audio files in the folders, folders within folders included.
func recordings(paths []string) ([]string, error) {
	var found []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			found = append(found, path)
			continue
		}
		err = filepath.WalkDir(path, func(inside string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			hidden := strings.HasPrefix(entry.Name(), ".") && inside != path
			if entry.IsDir() {
				if hidden {
					return filepath.SkipDir
				}
				return nil
			}
			if !hidden && audio.Readable(entry.Name()) {
				found = append(found, inside)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

func analyse(args []string) error {
	flags, s := analyseFlags()
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return fmt.Errorf("give a recording to analyse, or several, or a folder of them")
	}
	paths, err := recordings(flags.Args())
	if err != nil {
		return err
	}
	switch {
	case len(paths) == 0:
		return fmt.Errorf("no audio files were found there")
	case len(paths) == 1:
		return analyseOne(paths[0], s)
	case s.bass != "":
		return fmt.Errorf("-bass goes with one recording, not with %d", len(paths))
	}
	// several: one that fails does not stop the others
	var failed []string
	for i, path := range paths {
		fmt.Printf("\n[%d of %d] %s\n", i+1, len(paths), path)
		if err := analyseOne(path, s); err != nil {
			fmt.Fprintf(os.Stderr, "cbass: %s: %v\n", filepath.Base(path), err)
			failed = append(failed, filepath.Base(path))
		}
	}
	fmt.Printf("\n%d of %d done\n", len(paths)-len(failed), len(paths))
	if len(failed) > 0 {
		return fmt.Errorf("%d did not work: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

func analyseOne(path string, s *settings) error {
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

	recording, err := audio.Decode(path)
	if err != nil {
		return err
	}
	mix := project.Prepare(recording)
	step("read %s: %.1f seconds", filepath.Base(path), mix.Duration())

	options := project.Options{
		Version: version, Tuning: s.tuning, Frets: s.frets, Beats: s.bar, Sensitivity: s.sensitivity,
		Threads: s.threads, CoreML: s.coreML, Mix: s.mix,
	}
	if s.bass != "" {
		if options.Bass, err = audio.Decode(s.bass); err != nil {
			return err
		}
	}
	var failed error
	shown := int64(-1)
	result, err := project.Analyse(mix, name, options, func(at project.Step) {
		switch at.Stage {
		case project.Downloading:
			if at.Done>>20 == shown && at.Done != at.Total {
				return
			}
			shown = at.Done >> 20
			if at.Total > 0 {
				fmt.Printf("\r        downloading %s: %d of %d MB", at.Detail, at.Done>>20, at.Total>>20)
			} else {
				fmt.Printf("\r        downloading %s: %d MB", at.Detail, at.Done>>20)
			}
			if at.Done == at.Total {
				fmt.Println()
			}
		case project.Separating:
			fmt.Printf("\r        separating: %d of %d", at.Done, at.Total)
			if at.Done == at.Total {
				fmt.Println()
			}
		case project.Separated:
			step("separated the bass")
		case project.NotesRead:
			step("read %d notes", at.Done)
		case project.BeatFound:
			if at.Total == 0 {
				step("no steady beat was found: bars are written at 120 BPM")
			} else {
				step("found the beat: %d BPM, %d beats", at.Done, at.Total)
			}
		}
	})
	if err != nil {
		return err
	}
	if len(result.Project.Events) == 0 && result.Bass == nil {
		return fmt.Errorf("no notes were found in %s", filepath.Base(path))
	}
	if s.stems && result.Bass != nil {
		if err := audio.WriteWAV(filepath.Join(out, name+".no-bass.wav"), result.Backing); err != nil {
			failed = err
		}
		if err := audio.WriteWAV(filepath.Join(out, name+".bass.wav"), result.Bass); err != nil {
			failed = err
		}
		if failed != nil {
			return failed
		}
		step("wrote the two recordings")
	}

	if len(result.Project.Events) == 0 {
		return fmt.Errorf("there is no bass in %s: the recording without it is all there is", filepath.Base(path))
	}
	data, err := json.MarshalIndent(result.Project, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, name+".cbass.json"), data, 0o644); err != nil {
		return err
	}
	text := tab.Text(name, result.Project.Events, result.Project.Rhythm, fretboard.TuningFor(result.Project.Tuning), 4)
	if err := os.WriteFile(filepath.Join(out, name+".tab.txt"), []byte(text), 0o644); err != nil {
		return err
	}
	step("wrote %s and %s in %s", name+".cbass.json", name+".tab.txt", out)
	return nil
}
