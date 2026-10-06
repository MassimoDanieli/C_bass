// Package tab writes a line out as plain-text tablature, bar by bar.
package tab

import (
	"fmt"
	"strings"

	"github.com/MassimoDanieli/c_bass/internal/fretboard"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
	"github.com/MassimoDanieli/c_bass/internal/transcribe"
)

// valueLetters is the usual shorthand for note values under a text tablature.
var valueLetters = map[int]string{16: "w", 12: "h.", 8: "h", 6: "q.", 4: "q", 3: "e.", 2: "e", 1: "s"}

// Text renders the line as text: one row per string with the highest on top, a column per
// sixteenth, bar lines, and under the staff the value of every note (w h q e s, a dot for
// dotted). A held note is written once; where it runs on, the fret comes back in brackets at
// the start of a bar and the value row shows a tie (~).
func Text(title string, events []transcribe.Event, pulse *rhythm.Rhythm, tuning fretboard.Tuning, barsPerLine int) string {
	notes := make([]rhythm.Note, len(events))
	for i, event := range events {
		notes[i] = rhythm.Note{Start: event.Start, End: event.End}
	}
	score := pulse.Notate(notes)
	const cell = 3 // characters per sixteenth
	width := score.BarSlots * cell
	names := tuning.StringNames()
	strings_ := len(tuning.Open)

	type bar struct {
		rows   [][]byte
		values []byte
	}
	bars := map[int]*bar{}
	first, last := 0, -1
	get := func(index int) *bar {
		b := bars[index]
		if b == nil {
			b = &bar{rows: make([][]byte, strings_), values: []byte(strings.Repeat(" ", width))}
			for s := range b.rows {
				b.rows[s] = []byte(strings.Repeat("-", width))
			}
			bars[index] = b
			if last < first || index < first {
				if last < first {
					last = index
				}
				first = index
			}
			if index > last {
				last = index
			}
		}
		return b
	}
	put := func(row []byte, at int, text string) {
		for i := 0; i < len(text) && at+i < len(row); i++ {
			row[at+i] = text[i]
		}
	}
	for _, symbol := range score.Symbols {
		b := get(symbol.Bar)
		at := symbol.Slot * cell
		if symbol.Rest {
			put(b.values, at, "r"+valueLetters[symbol.Value])
			continue
		}
		event := events[symbol.Index]
		letter := valueLetters[symbol.Value]
		if symbol.Tied {
			letter = "~" + letter
		}
		put(b.values, at, letter)
		if event.String < 0 || event.String >= strings_ {
			continue
		}
		switch {
		case !symbol.Tied:
			put(b.rows[event.String], at, fmt.Sprint(event.Fret))
		case symbol.Slot == 0:
			put(b.rows[event.String], at, fmt.Sprintf("(%d)", event.Fret))
		}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "%s\n%s · %.0f BPM · %d/4\n", title, tuning.Label, pulse.Tempo(), pulse.PerBar)
	out.WriteString("w whole  h half  q quarter  e eighth  s sixteenth  . dotted  r rest  ~ tied\n\n")
	for start := first; start <= last; start += barsPerLine {
		end := min(last, start+barsPerLine-1)
		out.WriteString("  ")
		for index := start; index <= end; index++ {
			label := ""
			if index >= 0 {
				label = fmt.Sprint(index + 1)
			}
			fmt.Fprintf(&out, " %-*s", width, label)
		}
		out.WriteString("\n")
		for s := strings_ - 1; s >= 0; s-- {
			fmt.Fprintf(&out, "%-2s", names[s])
			for index := start; index <= end; index++ {
				out.WriteString("|")
				if b := bars[index]; b != nil {
					out.Write(b.rows[s])
				} else {
					out.WriteString(strings.Repeat("-", width))
				}
			}
			out.WriteString("|\n")
		}
		out.WriteString("  ")
		for index := start; index <= end; index++ {
			out.WriteString(" ")
			if b := bars[index]; b != nil {
				out.Write(b.values)
			} else {
				out.WriteString(strings.Repeat(" ", width))
			}
		}
		out.WriteString("\n\n")
	}
	return out.String()
}
