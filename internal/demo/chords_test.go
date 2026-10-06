package demo

import (
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/chords"
)

// The pieces that come with the program have their chords written down: read from the sound,
// the chords must be those, beat for beat, for nearly all of each piece.
func TestChordsAreReadFromTheSound(t *testing.T) {
	for _, p := range Pieces {
		r := p.Render()
		read := chords.Find(r.Backing, r.Project.Rhythm, r.Project.Events)
		beats := r.Project.Rhythm.Beats
		var total, root, whole int
		for b := p.Beats; b+1 < len(beats) && beats[b] < r.Project.Chords[len(r.Project.Chords)-1].End-0.01; b++ {
			middle := (beats[b] + beats[b+1]) / 2
			want := chords.At(r.Project.Chords, middle)
			if want == nil {
				continue
			}
			total++
			if got := chords.At(read, middle); got != nil && got.Root == want.Root {
				root++
				if got.Quality == want.Quality {
					whole++
				}
			}
		}
		t.Logf("%s: %d beats, root right on %d, whole chord on %d", p.ID, total, root, whole)
		if root*100 < total*90 {
			t.Errorf("%s: the root is right on %d beats of %d", p.ID, root, total)
		}
		if whole*100 < total*70 {
			t.Errorf("%s: the whole chord is right on %d beats of %d", p.ID, whole, total)
		}
	}
}
