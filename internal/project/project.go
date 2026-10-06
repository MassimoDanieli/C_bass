// Package project is the whole road from a recording to a bass part: separate the bass, read
// its notes, find the bars, choose a fingering. The command line and the application both go
// through it.
package project

import (
	"fmt"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/demucs"
	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/provision"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

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

// Options for Analyse. The zero value separates the bass and writes for a four-string bass.
type Options struct {
	Version     string
	Tuning      string  // "4", "5", "5c" or "6"
	Frets       int     // highest fret to use; 12 if zero
	Beats       int     // beats in a bar; 4 if zero
	Sensitivity float64 // to new attacks; 0.72 if zero
	Threads     int     // for the separation; all the cores if zero
	CoreML      bool
	// Mix reads the notes from the full recording, without separating.
	Mix bool
	// Bass is a bass already isolated, to use instead of separating.
	Bass *audio.Buffer
}

// Stage is one part of the work.
type Stage int

const (
	Downloading Stage = iota // Detail says what; Done and Total are bytes (Total 0 if unknown)
	Separating               // Done and Total are passes of the model
	Separated
	ReadingNotes // Done of Total, as a fraction of 1000
	NotesRead    // Done is the number of notes
	BeatFound    // Done is the tempo in BPM, Total the number of beats; both 0 if no steady beat was found
)

// Step reports progress.
type Step struct {
	Stage       Stage
	Detail      string
	Done, Total int64
}

// Result is a project with the two recordings that go with it. Bass and Backing are nil when
// nothing was separated.
type Result struct {
	Project *Project
	Bass    *audio.Buffer
	Backing *audio.Buffer
}

// Prepare brings a decoded recording to what the model and the analysis expect.
func Prepare(recording *audio.Buffer) *audio.Buffer {
	return audio.Resample(recording.Stereo(), demucs.SampleRate)
}

// Analyse works out the bass part of a recording already brought to 44.1 kHz stereo by Prepare.
func Analyse(mix *audio.Buffer, title string, options Options, report func(Step)) (*Result, error) {
	if report == nil {
		report = func(Step) {}
	}
	if options.Frets == 0 {
		options.Frets = 12
	}
	if options.Beats == 0 {
		options.Beats = 4
	}
	result := &Result{}
	source, isolated := mix, false
	switch {
	case options.Bass != nil:
		source, isolated = options.Bass, true
	case !options.Mix:
		stems, err := separate(mix, options, report)
		if err != nil {
			return nil, err
		}
		report(Step{Stage: Separated})
		source, isolated = stems["bass"], true
		backing := &audio.Buffer{SampleRate: mix.SampleRate, Channels: [][]float32{make([]float32, mix.Len()), make([]float32, mix.Len())}}
		for _, stem := range []string{"drums", "other", "vocals"} {
			for c, channel := range stems[stem].Channels {
				for i, v := range channel {
					backing.Channels[c][i] += v
				}
			}
		}
		result.Bass, result.Backing = stems["bass"], backing
	}

	events := transcribe.Transcribe(source, transcribe.Options{
		Isolated: isolated, Sensitivity: options.Sensitivity,
		Progress: func(done float64) { report(Step{Stage: ReadingNotes, Done: int64(done * 1000), Total: 1000}) },
	})
	if len(events) == 0 {
		return nil, fmt.Errorf("no notes were found in %s", title)
	}
	tuning := fretboard.TuningFor(options.Tuning)
	events = fretboard.Finger(events, tuning, options.Frets)
	report(Step{Stage: NotesRead, Done: int64(len(events))})

	pulse := rhythm.Analyse(mix, options.Beats)
	if pulse == nil {
		pulse = rhythm.Steady(120, mix.Duration(), options.Beats)
		report(Step{Stage: BeatFound})
	} else {
		report(Step{Stage: BeatFound, Done: int64(pulse.Tempo() + 0.5), Total: int64(len(pulse.Beats))})
	}

	result.Project = &Project{
		Version: options.Version, Title: title, Duration: mix.Duration(), Source: map[bool]string{true: "bass", false: "mix"}[isolated],
		Tuning: tuning.Key, Frets: options.Frets, Rhythm: pulse, Events: events,
	}
	return result, nil
}

// Refinger chooses the fingering again, for another instrument or another reach.
func (p *Project) Refinger(tuning string, frets int) {
	t := fretboard.TuningFor(tuning)
	p.Tuning, p.Frets = t.Key, frets
	p.Events = fretboard.Finger(p.Events, t, frets)
}

func separate(mix *audio.Buffer, options Options, report func(Step)) (demucs.Stems, error) {
	download := func(what string, done, total int64) {
		report(Step{Stage: Downloading, Detail: what, Done: done, Total: total})
	}
	library, err := provision.Runtime(download)
	if err != nil {
		return nil, err
	}
	model, err := provision.Model(download)
	if err != nil {
		return nil, err
	}
	separator, err := demucs.Open(demucs.Options{Library: library, Model: model, Threads: options.Threads, CoreML: options.CoreML})
	if err != nil {
		return nil, err
	}
	defer separator.Close()
	return separator.Separate(mix, func(done, total int) {
		report(Step{Stage: Separating, Done: int64(done), Total: int64(total)})
	})
}
