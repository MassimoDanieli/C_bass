package demo

import (
	"math"
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/library"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// Every piece must be playable as written: notes on the neck, inside the bars, never
// clipping, and read back by the reader as the notes that were written.
func TestPiecesAreSound(t *testing.T) {
	for _, p := range Pieces {
		r := p.Render()
		if len(r.Project.Events) == 0 {
			t.Fatalf("%s has no notes", p.ID)
		}
		var peak float32
		for _, channel := range [][]float32{r.Bass.Channels[0], r.Backing.Channels[0]} {
			for _, v := range channel {
				peak = max(peak, max(v, -v))
			}
		}
		if peak > 0.95 {
			t.Errorf("%s clips: peak %.2f", p.ID, peak)
		}
		for _, e := range r.Project.Events {
			if e.String < 0 || e.Fret > 12 {
				t.Errorf("%s: note %d has no place on a four-string neck within 12 frets", p.ID, e.Midi)
			}
			if e.Start < 60/p.BPM*float64(p.Beats)-0.01 || e.End > r.Project.Duration {
				t.Errorf("%s: note at %.2f falls outside the bars", p.ID, e.Start)
			}
		}
		read := transcribe.Transcribe(r.Bass, transcribe.Options{Isolated: true})
		found := 0
		for _, e := range r.Project.Events {
			for _, got := range read {
				if math.Abs(got.Start-e.Start) < 0.08 && got.Midi == e.Midi {
					found++
					break
				}
			}
		}
		if found < len(r.Project.Events) {
			t.Errorf("%s: %d of %d written notes read back", p.ID, found, len(r.Project.Events))
		}
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	lib := &library.Library{Dir: t.TempDir()}
	ids, err := Install(lib)
	if err != nil || len(ids) != len(Pieces) {
		t.Fatal(ids, err)
	}
	if again, err := Install(lib); err != nil || len(again) != len(ids) || len(lib.List()) != len(Pieces) {
		t.Fatal("installing twice changed the library", err)
	}
	for _, e := range lib.List() {
		if !e.BuiltIn || e.Key == "" {
			t.Errorf("%s is not marked as built in with a key", e.ID)
		}
	}
}
