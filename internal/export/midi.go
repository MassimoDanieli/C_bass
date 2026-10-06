package export

import (
	"bytes"
	"encoding/binary"
	"math"
	"sort"

	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
)

// ticks in a quarter note.
const ticks = 480

// MIDI writes the part as a standard MIDI file that keeps the time of the recording: every
// beat has the length it has there, so that in a sequencer the notes fall where the bass
// plays them and the file lines up with the recording laid beside it from the start. The
// bars, with any that have a length of their own, and the names of the sections and of the chords are in it too.
func MIDI(p *project.Project) []byte {
	pulse := p.Rhythm
	if pulse == nil || len(pulse.Beats) < 2 || pulse.PerBar < 1 {
		pulse = rhythm.Steady(120, math.Max(p.Duration, 1), 4)
	}
	// The file starts where the recording does, which is before the first bar line or on it:
	// whole bars are put in front so that the bar lines of the file are those of the part.
	lead := 0
	if before := -pulse.Position(0); before > 1e-6 {
		lead = int(math.Ceil(before/float64(pulse.PerBar))) * pulse.PerBar
	}
	tick := func(seconds float64) int {
		return max(0, int(math.Round((pulse.Position(seconds)+float64(lead))*ticks)))
	}
	type event struct {
		at, order int
		data      []byte
	}
	var events []event
	add := func(at, order int, data ...byte) { events = append(events, event{at, order, data}) }
	meta := func(at, order int, kind byte, body []byte) {
		add(at, order, append(append([]byte{0xff, kind}, varlen(len(body))...), body...)...)
	}
	meta(0, 0, 0x03, []byte(p.Title))
	// the tempo of every beat, from how far apart the beats are in the recording
	tempo := func(at int, seconds float64) {
		micro := uint32(math.Max(1, math.Min(16777215, math.Round(seconds*1e6))))
		meta(at, 1, 0x51, []byte{byte(micro >> 16), byte(micro >> 8), byte(micro)})
	}
	beats := pulse.Beats
	tempo(0, beats[1]-beats[0])
	for i := 0; i+1 < len(beats); i++ {
		if at := (i - pulse.Downbeat + lead) * ticks; at > 0 {
			tempo(at, beats[i+1]-beats[i])
		}
	}
	// the bars: four in a bar, or whatever it is, and any bar that differs
	signature := func(at, beatsInBar int) { meta(at, 0, 0x58, []byte{byte(beatsInBar), 2, 24, 8}) }
	signature(0, pulse.PerBar)
	last := pulse.BarAt(pulse.Position(math.Max(p.Duration, beats[len(beats)-1])))
	for bar, current := 0, pulse.PerBar; bar <= last; bar++ {
		if now := pulse.BeatsIn(bar); now != current {
			signature((pulse.BarStart(bar)+lead)*ticks, now)
			current = now
		}
	}
	// the sections as markers, which a sequencer shows along the top; the chords as text
	for _, section := range p.Sections {
		name := section.Name
		if name == "" {
			name = section.Kind
		}
		meta(tick(section.Start), 2, 0x06, []byte(name))
	}
	for _, chord := range p.Chords {
		meta(tick(chord.Start), 2, 0x01, []byte(chord.Name(false)))
	}
	add(0, 2, 0xc0, 33) // a fingered electric bass
	for _, note := range p.Events {
		if note.Midi < 0 || note.Midi > 127 {
			continue
		}
		on := tick(note.Start)
		off := max(on+1, tick(note.End))
		add(on, 4, 0x90, byte(note.Midi), 96)
		add(off, 3, 0x80, byte(note.Midi), 0) // a note ends before the next one on the same tick starts
	}
	sort.SliceStable(events, func(a, b int) bool {
		if events[a].at != events[b].at {
			return events[a].at < events[b].at
		}
		return events[a].order < events[b].order
	})
	var track bytes.Buffer
	now := 0
	for _, e := range events {
		track.Write(varlen(e.at - now))
		track.Write(e.data)
		now = e.at
	}
	track.Write([]byte{0, 0xff, 0x2f, 0})
	var out bytes.Buffer
	out.WriteString("MThd")
	binary.Write(&out, binary.BigEndian, uint32(6))
	binary.Write(&out, binary.BigEndian, [3]uint16{0, 1, ticks}) // one track, 480 ticks to the quarter
	out.WriteString("MTrk")
	binary.Write(&out, binary.BigEndian, uint32(track.Len()))
	out.Write(track.Bytes())
	return out.Bytes()
}

// varlen writes a number the way MIDI files do: seven bits to a byte, the last byte marked.
func varlen(n int) []byte {
	out := []byte{byte(n & 0x7f)}
	for n >>= 7; n > 0; n >>= 7 {
		out = append([]byte{byte(n&0x7f) | 0x80}, out...)
	}
	return out
}
