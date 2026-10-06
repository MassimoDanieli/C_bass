package library

import (
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

func TestSaveListLoadRemove(t *testing.T) {
	l := &Library{Dir: t.TempDir()}
	tone := &audio.Buffer{SampleRate: 44100, Channels: [][]float32{make([]float32, 4410), make([]float32, 4410)}}
	tone.Channels[0][100] = 0.5
	result := &project.Result{
		Project: &project.Project{Title: "Prova", Duration: 0.1, Rhythm: rhythm.Steady(120, 2, 4), Events: []transcribe.Event{{Start: 0, End: 0.05, Midi: 33, Fret: 0, String: 1}}},
		Bass:    tone, Backing: tone,
	}
	id := ID([]byte("the file"))
	if l.Has(id) {
		t.Fatal("an empty library has the recording")
	}
	if err := l.Save(id, result); err != nil {
		t.Fatal(err)
	}
	list := l.List()
	if len(list) != 1 || list[0].ID != id || list[0].Title != "Prova" || list[0].Notes != 1 || list[0].Tempo < 119 || list[0].Tempo > 121 {
		t.Fatalf("list: %+v", list)
	}
	back, err := l.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if back.Project.Title != "Prova" || len(back.Project.Events) != 1 || back.Bass.Len() != 4410 || back.Backing.Channels[0][100] < 0.49 {
		t.Fatalf("loaded: %+v", back.Project)
	}
	if err := l.Remove(id); err != nil || l.Has(id) || len(l.List()) != 0 {
		t.Fatal("the recording is still there", err)
	}
	if l.Remove("../x") == nil {
		t.Fatal("a path outside the library was accepted")
	}
}

func TestSaveNeedsTheTracks(t *testing.T) {
	l := &Library{Dir: t.TempDir()}
	if l.Save("x", &project.Result{Project: &project.Project{}}) == nil {
		t.Fatal("saved a recording with nothing to play")
	}
}
