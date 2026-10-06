// Package project is the whole road from a recording to a bass part: separate the bass, read
// its notes, find the bars, choose a fingering. The command line and the application both go
// through it.
package project

import (
	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/chords"
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
	// Reader is the version of the note reader that wrote Events: see Reader.
	Reader int `json:"reader,omitempty"`
	// Chords are the chords of the piece; nil when they have not been looked for yet.
	Chords []chords.Chord `json:"chords"`
	// Low keeps the fingering near the nut, where the line allows it.
	Low bool `json:"lowPosition,omitempty"`
	// Sensitivity is how readily the reader took a rise in level for a note struck again,
	// when it is not the usual: see Sensitivities.
	Sensitivity float64 `json:"sensitivity,omitempty"`
	// Key of the piece as a pitch name, when it is known: the pieces that come with the program have one.
	Key string `json:"key,omitempty"`
	// BuiltIn marks a piece that came with the program.
	BuiltIn bool `json:"builtIn,omitempty"`
}

// Reader is the current version of the note reader. A project written by an older one is
// read again from its separated bass when it is opened: that takes a second, the separation
// is not done again.
//
//	2: the pitch of a note is read from the whole of it; a note is not split where only its
//	   sound changes; pitch is followed between semitones; what is left of a recording with
//	   no bass is not read as notes.
const Reader = 2

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
	// Low keeps the fingering near the nut, where the line allows it.
	Low bool
	// Stop, if set, is asked now and then whether to give up; Analyse then returns ErrStopped.
	Stop func() bool
}

// ErrStopped is what Analyse returns when it was asked to stop.
var ErrStopped = demucs.ErrStopped

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
		// the rest is the recording with that bass taken away, when the two line up
		if options.Bass.SampleRate == mix.SampleRate && len(options.Bass.Channels) == len(mix.Channels) {
			backing := &audio.Buffer{SampleRate: mix.SampleRate}
			for c, channel := range mix.Channels {
				rest := make([]float32, len(channel))
				for i := range rest {
					rest[i] = channel[i]
					if i < len(options.Bass.Channels[c]) {
						rest[i] -= options.Bass.Channels[c][i]
					}
				}
				backing.Channels = append(backing.Channels, rest)
			}
			result.Bass, result.Backing = options.Bass, backing
		}
	case !options.Mix:
		stems, err := separate(mix, options, report)
		if err != nil {
			return nil, err
		}
		report(Step{Stage: Separated})
		source, isolated = stems["bass"], true
		result.Bass, result.Backing = stems["bass"], stems["rest"]
	}

	tuning := fretboard.TuningFor(options.Tuning)
	events := read(source, mix, isolated && options.Bass == nil, tuning, options.Frets, options.Low, options.Sensitivity, func(done float64) {
		report(Step{Stage: ReadingNotes, Done: int64(done * 1000), Total: 1000})
	})
	report(Step{Stage: NotesRead, Done: int64(len(events))})

	pulse := rhythm.Analyse(mix, options.Beats)
	if pulse == nil {
		pulse = rhythm.Steady(120, mix.Duration(), options.Beats)
		report(Step{Stage: BeatFound})
	} else {
		pulse.Downbeat = chords.FirstBeat(result.Backing, pulse, events)
		report(Step{Stage: BeatFound, Done: int64(pulse.Tempo() + 0.5), Total: int64(len(pulse.Beats))})
	}

	result.Project = &Project{
		Reader:  Reader,
		Version: options.Version, Title: title, Duration: mix.Duration(), Source: map[bool]string{true: "bass", false: "mix"}[isolated],
		Tuning: tuning.Key, Frets: options.Frets, Rhythm: pulse, Events: events, Low: options.Low,
	}
	Harmonise(result)
	return result, nil
}

// read finds the notes in a bass and chooses where to play them. A bass separated from a
// recording is held against that recording: 34 dB under it there is no bass, only what the
// separation left behind.
func read(bass, mix *audio.Buffer, separated bool, tuning fretboard.Tuning, frets int, low bool, sensitivity float64, progress func(float64)) []transcribe.Event {
	var floor float64
	if separated {
		floor = transcribe.Level(mix) * 0.02
	}
	events := transcribe.Transcribe(bass, transcribe.Options{
		Isolated: bass != mix, Sensitivity: sensitivity, Floor: floor, Lowest: tuning.Open[0], Progress: progress,
	})
	events = fretboard.FingerWith(events, tuning, frets, low)
	if events == nil {
		events = []transcribe.Event{} // no bass in the recording: nothing to read, still something to play along to
	}
	return events
}

// Sensitivities are the three settings of the reader for notes struck again on the same
// pitch. A note repeated without letting go hardly dips in level, less still once the bass
// has been through the separation: the usual setting then writes one long note where several
// were played. The others take a smaller rise for a new note. Measured on a song of straight
// eighths, the middle one found 47 of 126 merged notes again and the last 64, with nothing
// made up on the recordings there is a score for; on a real bass with no score they add
// notes that cannot be told from splits without listening. Hence a choice, not a default.
var Sensitivities = []float64{0.72, 0.80, 0.86}

// Reread reads the notes of a separated recording again, with the reader of this version of
// the program, keeping its bars and its instrument. It reports whether anything was done.
func Reread(result *Result) bool {
	p := result.Project
	if p.Reader >= Reader || result.Bass == nil || result.Backing == nil {
		return false
	}
	for _, event := range p.Events {
		if event.Edited { // corrected by hand: what a person wrote is not read over
			p.Reader = Reader
			return true
		}
	}
	p.Events = ReadWith(result.Bass, result.Backing, p, p.Sensitivity)
	p.Reader = Reader
	return true
}

// ReadWith reads the notes of a separated bass for a project's instrument, with a
// sensitivity to notes struck again (0 for the usual).
func ReadWith(bass, backing *audio.Buffer, p *Project, sensitivity float64) []transcribe.Event {
	result := &Result{Bass: bass, Backing: backing}
	mix := &audio.Buffer{SampleRate: result.Bass.SampleRate, Channels: make([][]float32, len(result.Bass.Channels))}
	for c := range mix.Channels {
		mix.Channels[c] = make([]float32, min(result.Bass.Len(), result.Backing.Len()))
		for i := range mix.Channels[c] {
			mix.Channels[c][i] = result.Bass.Channels[c][i] + result.Backing.Channels[c][i]
		}
	}
	frets := p.Frets
	if frets <= 0 {
		frets = 12
	}
	return read(result.Bass, mix, true, fretboard.TuningFor(p.Tuning), frets, p.Low, sensitivity, nil)
}

// Harmonise reads the chords of a recording that has none written down yet, from what is left
// of it without the bass. It reports whether anything was done.
func Harmonise(result *Result) bool {
	p := result.Project
	if p.Chords != nil || result.Backing == nil || p.Rhythm == nil {
		return false
	}
	p.Chords = chords.Find(result.Backing, p.Rhythm, p.Events)
	return true
}

// Refinger chooses the fingering again, for another instrument or another reach.
func (p *Project) Refinger(tuning string, frets int) {
	t := fretboard.TuningFor(tuning)
	p.Tuning, p.Frets = t.Key, frets
	if len(p.Events) > 0 {
		p.Events = fretboard.FingerWith(p.Events, t, frets, p.Low)
	}
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
	separator.Stop = options.Stop
	return separator.Separate(mix, func(done, total int) {
		report(Step{Stage: Separating, Done: int64(done), Total: int64(total)})
	})
}
