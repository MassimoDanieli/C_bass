// Package export writes a part out for other programs and for paper: MusicXML, which Guitar
// Pro, MuseScore and TuxGuitar open, and a PDF to print.
package export

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"sort"

	"github.com/MassimoDanieli/c_bass/internal/chords"
	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
)

// part is a project laid out in bars, ready to be written one way or another.
type part struct {
	project *project.Project
	pulse   *rhythm.Rhythm
	score   *rhythm.Score
	tuning  fretboard.Tuning
	bars    []bar
}

type bar struct {
	index   int // counted from 0; negative for a pickup
	beats   int
	symbols []rhythm.Symbol
	chords  []placed
	section string // the name of the section that starts with this bar, if one does
}

// placed is a chord with the sixteenth of its bar it starts on.
type placed struct {
	slot  int
	chord chords.Chord
}

func layout(p *project.Project) *part {
	pulse := p.Rhythm
	if pulse == nil || len(pulse.Beats) < 2 || pulse.PerBar < 1 {
		pulse = rhythm.Steady(120, math.Max(p.Duration, 1), 4)
	}
	notes := make([]rhythm.Note, len(p.Events))
	for i, event := range p.Events {
		notes[i] = rhythm.Note{Start: event.Start, End: event.End}
	}
	out := &part{project: p, pulse: pulse, score: pulse.Notate(notes), tuning: fretboard.TuningFor(p.Tuning)}
	first, last := 0, 0
	byBar := map[int][]rhythm.Symbol{}
	for _, symbol := range out.score.Symbols {
		byBar[symbol.Bar] = append(byBar[symbol.Bar], symbol)
		first, last = min(first, symbol.Bar), max(last, symbol.Bar)
	}
	harmony := map[int][]placed{}
	for _, chord := range p.Chords {
		at := int(math.Round((pulse.Position(chord.Start) - out.score.Shift) * rhythm.Division))
		index := pulse.BarAt(float64(at) / rhythm.Division)
		harmony[index] = append(harmony[index], placed{at - pulse.BarStart(index)*rhythm.Division, chord})
		first, last = min(first, index), max(last, index)
	}
	starting := map[int]string{}
	for _, section := range p.Sections {
		name := section.Name
		if name == "" {
			name = section.Kind
		}
		index := pulse.BarAt(pulse.Position(section.Start+0.01) - out.score.Shift + 1e-6)
		starting[index] = name
		first, last = min(first, index), max(last, index)
	}
	for index := first; index <= last; index++ {
		b := bar{index: index, beats: pulse.BeatsIn(index), symbols: byBar[index], chords: harmony[index], section: starting[index]}
		if len(b.symbols) == 0 { // a bar with nothing in it is a bar of rest
			for _, piece := range rhythm.SplitValues(0, b.beats*rhythm.Division, b.beats*rhythm.Division, true) {
				b.symbols = append(b.symbols, rhythm.Symbol{Bar: index, Slot: piece.Slot, Value: piece.Value, Rest: true, Index: -1, At: pulse.BarStart(index)*rhythm.Division + piece.Slot})
			}
		}
		sort.Slice(b.chords, func(i, j int) bool { return b.chords[i].slot < b.chords[j].slot })
		out.bars = append(out.bars, b)
	}
	return out
}

// continues says whether the note of a sign is carried on by the next sign.
func (p *part) continues(b, i int) bool {
	symbols := p.bars[b].symbols
	current := symbols[i]
	if i+1 < len(symbols) {
		return symbols[i+1].Tied && symbols[i+1].Index == current.Index
	}
	if b+1 < len(p.bars) && len(p.bars[b+1].symbols) > 0 {
		next := p.bars[b+1].symbols[0]
		return next.Tied && next.Index == current.Index
	}
	return false
}

var valueTypes = map[int]struct {
	name string
	dot  bool
}{16: {"whole", false}, 12: {"half", true}, 8: {"half", false}, 6: {"quarter", true}, 4: {"quarter", false}, 3: {"eighth", true}, 2: {"eighth", false}, 1: {"16th", false}}

var steps = [...]struct {
	step  string
	alter int
}{{"C", 0}, {"C", 1}, {"D", 0}, {"D", 1}, {"E", 0}, {"F", 0}, {"F", 1}, {"G", 0}, {"G", 1}, {"A", 0}, {"A", 1}, {"B", 0}}

var flats = map[int]string{3: "E", 8: "A", 10: "B"}

var kinds = map[string]string{"": "major", "m": "minor", "7": "dominant", "m7": "minor-seventh", "maj7": "major-seventh"}

func escape(s string) string {
	var out bytes.Buffer
	xml.EscapeText(&out, []byte(s))
	return out.String()
}

// MusicXML writes the part as a tablature staff with its chords, in MusicXML 4.0.
func MusicXML(p *project.Project) []byte {
	part := layout(p)
	var out bytes.Buffer
	w := func(format string, values ...any) { fmt.Fprintf(&out, format+"\n", values...) }
	w(`<?xml version="1.0" encoding="UTF-8"?>`)
	w(`<!DOCTYPE score-partwise PUBLIC "-//Recordare//DTD MusicXML 4.0 Partwise//EN" "http://www.musicxml.org/dtds/partwise.dtd">`)
	w(`<score-partwise version="4.0">`)
	w(`  <work><work-title>%s</work-title></work>`, escape(p.Title))
	w(`  <identification><encoding><software>C_bass</software></encoding></identification>`)
	w(`  <part-list><score-part id="P1"><part-name>Bass</part-name></score-part></part-list>`)
	w(`  <part id="P1">`)
	strings := len(part.tuning.Open)
	beats := 0
	for b, bar := range part.bars {
		if bar.index < 0 {
			w(`    <measure number="0" implicit="yes">`)
		} else {
			w(`    <measure number="%d">`, bar.index+1)
		}
		if b == 0 || bar.beats != beats {
			w(`      <attributes>`)
			if b == 0 {
				w(`        <divisions>%d</divisions>`, rhythm.Division)
				w(`        <key><fifths>0</fifths></key>`)
			}
			w(`        <time><beats>%d</beats><beat-type>4</beat-type></time>`, bar.beats)
			if b == 0 {
				w(`        <clef><sign>TAB</sign><line>5</line></clef>`)
				w(`        <staff-details><staff-lines>%d</staff-lines>`, strings)
				for line, open := range part.tuning.Open {
					s := steps[open%12]
					alter := ""
					if s.alter != 0 {
						alter = fmt.Sprintf("<tuning-alter>%d</tuning-alter>", s.alter)
					}
					w(`          <staff-tuning line="%d"><tuning-step>%s</tuning-step>%s<tuning-octave>%d</tuning-octave></staff-tuning>`, line+1, s.step, alter, open/12-1)
				}
				w(`        </staff-details>`)
			}
			w(`      </attributes>`)
			beats = bar.beats
		}
		if b == 0 {
			tempo := math.Round(part.pulse.Tempo())
			w(`      <direction placement="above"><direction-type><metronome><beat-unit>quarter</beat-unit><per-minute>%.0f</per-minute></metronome></direction-type><sound tempo="%.0f"/></direction>`, tempo, tempo)
		}
		if bar.section != "" {
			var name bytes.Buffer
			xml.EscapeText(&name, []byte(bar.section))
			w(`      <direction placement="above"><direction-type><rehearsal>%s</rehearsal></direction-type></direction>`, name.String())
		}
		for _, c := range bar.chords {
			root := steps[((c.chord.Root%12)+12)%12]
			if flat, ok := flats[((c.chord.Root%12)+12)%12]; ok { // chords are written Bb, Eb, Ab
				root.step, root.alter = flat, -1
			}
			alter := ""
			if root.alter != 0 {
				alter = fmt.Sprintf("<root-alter>%d</root-alter>", root.alter)
			}
			offset := ""
			if c.slot > 0 {
				offset = fmt.Sprintf("<offset>%d</offset>", c.slot)
			}
			w(`      <harmony><root><root-step>%s</root-step>%s</root><kind text="%s">%s</kind>%s</harmony>`, root.step, alter, c.chord.Quality, kinds[c.chord.Quality], offset)
		}
		for i, symbol := range bar.symbols {
			kind := valueTypes[symbol.Value]
			dot := ""
			if kind.dot {
				dot = "<dot/>"
			}
			if symbol.Rest {
				w(`      <note><rest/><duration>%d</duration><voice>1</voice><type>%s</type>%s</note>`, symbol.Value, kind.name, dot)
				continue
			}
			event := p.Events[symbol.Index]
			pitch := steps[((event.Midi%12)+12)%12]
			alter := ""
			if pitch.alter != 0 {
				alter = fmt.Sprintf("<alter>%d</alter>", pitch.alter)
			}
			ties, tied := "", ""
			if symbol.Tied {
				ties, tied = `<tie type="stop"/>`, `<tied type="stop"/>`
			}
			if part.continues(b, i) {
				ties, tied = ties+`<tie type="start"/>`, tied+`<tied type="start"/>`
			}
			technical := ""
			if event.String >= 0 && event.String < strings {
				// MusicXML counts the strings from the highest
				technical = fmt.Sprintf("<technical><string>%d</string><fret>%d</fret></technical>", strings-event.String, event.Fret)
			}
			notations := ""
			if tied != "" || technical != "" {
				notations = "<notations>" + tied + technical + "</notations>"
			}
			w(`      <note><pitch><step>%s</step>%s<octave>%d</octave></pitch><duration>%d</duration>%s<voice>1</voice><type>%s</type>%s%s</note>`,
				pitch.step, alter, event.Midi/12-1, symbol.Value, ties, kind.name, dot, notations)
		}
		w(`    </measure>`)
	}
	w(`  </part>`)
	w(`</score-partwise>`)
	return out.Bytes()
}
