package project

import (
	"math"
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

const rate = 44100

func track(seconds float64, sample func(t float64) float64) *audio.Buffer {
	left := make([]float32, int(seconds*rate))
	for i := range left {
		left[i] = float32(sample(float64(i) / rate))
	}
	return &audio.Buffer{SampleRate: rate, Channels: [][]float32{left, append([]float32(nil), left...)}}
}

// four notes of half a second: A1 C2 D2 E1
func line(level float64) *audio.Buffer {
	pitches := []float64{55, 65.41, 73.42, 41.2}
	return track(3, func(t float64) float64 {
		n := int((t - 0.5) / 0.5)
		if t < 0.5 || n >= len(pitches) {
			return 0
		}
		in := t - 0.5 - float64(n)*0.5
		return level * math.Min(1, in/0.005) * math.Exp(-in*3) * (math.Sin(2*math.Pi*pitches[n]*in) + 0.4*math.Sin(4*math.Pi*pitches[n]*in))
	})
}

func keys(level float64) *audio.Buffer {
	return track(3, func(t float64) float64 {
		return level * (math.Sin(2*math.Pi*196*t) + math.Sin(2*math.Pi*247*t) + math.Sin(2*math.Pi*294*t)) / 3
	})
}

// A project written by an earlier reader is read again from its separated bass.
func TestRereadReplacesWhatAnOlderReaderWrote(t *testing.T) {
	result := &Result{
		Project: &Project{Tuning: "4", Frets: 12, Events: []transcribe.Event{{Midi: 60}, {Midi: 61}, {Midi: 62}, {Midi: 63}, {Midi: 64}, {Midi: 65}}},
		Bass:    line(0.4), Backing: keys(0.2),
	}
	if !Reread(result) {
		t.Fatal("an old project was left as it was")
	}
	var got []int
	for _, event := range result.Project.Events {
		got = append(got, event.Midi)
		if event.String < 0 {
			t.Fatalf("note %d has no place on the neck", event.Midi)
		}
	}
	if len(got) != 4 || got[0] != 33 || got[1] != 36 || got[2] != 38 || got[3] != 28 {
		t.Fatalf("A1 C2 D2 E1 were read as %v", got)
	}
	if result.Project.Reader != Reader || Reread(result) {
		t.Fatal("a project just read is read again")
	}
}

// A recording with no bass: the separation still leaves something in the bass track, a
// thousand times fainter than the recording. It is not a line to transcribe.
func TestNoBassIsNoNotes(t *testing.T) {
	result := &Result{
		Project: &Project{Tuning: "4", Frets: 12, Events: []transcribe.Event{{Midi: 40}}},
		Bass:    line(0.0004), Backing: keys(0.3),
	}
	Reread(result)
	if result.Project.Events == nil || len(result.Project.Events) != 0 {
		t.Fatalf("%d notes were read where there is no bass", len(result.Project.Events))
	}
}
