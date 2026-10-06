package export

import (
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"testing"

	"github.com/MassimoDanieli/c_bass/internal/chords"
	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// A piece at 120 BPM: a pickup, a note held across a bar line, a bar of two beats, a chord.
func piece() *project.Project {
	pulse := rhythm.Steady(120, 12, 4)
	pulse.Downbeat = 1 // the first beat is a pickup
	pulse.SetBeatsIn(1, 2)
	events := []transcribe.Event{
		{Start: 0, End: 0.5, Midi: 28},    // the pickup: E1
		{Start: 0.5, End: 1.5, Midi: 33},  // bar 1: A1, a half note
		{Start: 1.5, End: 3.0, Midi: 36},  // C2 from beat 3 of bar 1 across the short bar 2
		{Start: 3.5, End: 3.75, Midi: 43}, // bar 3
		{Start: 3.75, End: 4.0, Midi: 45},
	}
	events = fretboard.Finger(events, fretboard.TuningFor("4"), 12)
	return &project.Project{
		Title: "Prova & <riprova>", Tuning: "4", Frets: 12, Duration: 12, Rhythm: pulse, Events: events,
		Chords: []chords.Chord{{Start: 0.5, End: 2.5, Root: 9, Quality: "m7"}, {Start: 3.5, End: 5.5, Root: 2, Quality: "7"}},
	}
}

type xmlNote struct {
	Rest     *struct{} `xml:"rest"`
	Duration int       `xml:"duration"`
	Step     string    `xml:"pitch>step"`
	Octave   int       `xml:"pitch>octave"`
	Ties     []struct {
		Type string `xml:"type,attr"`
	} `xml:"tie"`
	String int `xml:"notations>technical>string"`
	Fret   int `xml:"notations>technical>fret"`
}

type xmlMeasure struct {
	Number string `xml:"number,attr"`
	Beats  int    `xml:"attributes>time>beats"`
	Tuning []struct {
		Step string `xml:"tuning-step"`
	} `xml:"attributes>staff-details>staff-tuning"`
	Notes   []xmlNote `xml:"note"`
	Harmony []struct {
		Step string `xml:"root>root-step"`
		Kind string `xml:"kind"`
	} `xml:"harmony"`
}

func TestMusicXML(t *testing.T) {
	data := MusicXML(piece())
	var score struct {
		Title    string       `xml:"work>work-title"`
		Measures []xmlMeasure `xml:"part>measure"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	if err := decoder.Decode(&score); err != nil {
		t.Fatalf("not well formed: %v\n%s", err, data)
	}
	if tuning := score.Measures[0].Tuning; score.Title != "Prova & <riprova>" || len(tuning) != 4 || tuning[0].Step != "E" {
		t.Errorf("title %q, tuning %+v", score.Title, tuning)
	}
	// every bar must hold exactly its beats
	beats := 4
	for _, m := range score.Measures {
		if m.Beats != 0 {
			beats = m.Beats
		}
		total := 0
		for _, n := range m.Notes {
			total += n.Duration
		}
		if total != beats*rhythm.Division {
			t.Errorf("bar %s holds %d sixteenths, not %d", m.Number, total, beats*rhythm.Division)
		}
	}
	if len(score.Measures) != 4 || score.Measures[0].Number != "0" || score.Measures[2].Beats != 2 || score.Measures[3].Beats != 4 {
		t.Fatalf("bars: %d, first %q, beats of the third %d", len(score.Measures), score.Measures[0].Number, score.Measures[2].Beats)
	}
	// the note held across the short bar: started in bar 1, carried through bar 2
	first := score.Measures[1].Notes
	held := first[len(first)-1]
	if held.Step != "C" || held.Octave != 2 || len(held.Ties) != 1 || held.Ties[0].Type != "start" {
		t.Errorf("the held note in bar 1: %+v", held)
	}
	carried := score.Measures[2].Notes[0]
	if carried.Step != "C" || len(carried.Ties) == 0 || carried.Ties[0].Type != "stop" {
		t.Errorf("the held note in bar 2: %+v", carried)
	}
	// A1 on a four-string bass is the open third string counted from the top
	if a := first[0]; a.Step != "A" || a.String != 3 || a.Fret != 0 {
		t.Errorf("A1 is written %+v", a)
	}
	if h := score.Measures[1].Harmony; len(h) != 1 || h[0].Step != "A" || h[0].Kind != "minor-seventh" {
		t.Errorf("the chord of bar 1: %+v", h)
	}
}

func TestPDF(t *testing.T) {
	data := PDF(piece(), func(midi int) string { return fretboard.PitchName(midi) })
	if !bytes.HasPrefix(data, []byte("%PDF-1.4")) || !bytes.HasSuffix(data, []byte("%%EOF\n")) {
		t.Fatal("not a PDF")
	}
	// the table at the end must point at every object
	at := bytes.LastIndex(data, []byte("startxref\n"))
	fields := bytes.Fields(data[at+len("startxref\n"):])
	table, err := strconv.Atoi(string(fields[0]))
	if err != nil || !bytes.HasPrefix(data[table:], []byte("xref\n")) {
		t.Fatalf("the table is not where the file says: %v", err)
	}
	lines := bytes.Split(data[table:], []byte("\n"))
	for n, line := range lines[3:] {
		if len(line) != 19 || line[17] != 'n' {
			break
		}
		offset, _ := strconv.Atoi(string(line[:10]))
		if want := []byte(strconv.Itoa(n+1) + " 0 obj"); !bytes.HasPrefix(data[offset:], want) {
			t.Errorf("object %d is not at %d", n+1, offset)
		}
	}
	_ = io.Discard
}
