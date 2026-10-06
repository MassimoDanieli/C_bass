package export

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"

	"github.com/MassimoDanieli/c_bass/internal/project"
	"github.com/MassimoDanieli/c_bass/internal/rhythm"
)

// Names says how the notes are called on the page: C D E, or Do Re Mi.
type Names func(midi int) string

// sheet is a PDF being written: pages of lines and text, in points from the top left corner.
type sheet struct {
	pages []*bytes.Buffer
	page  *bytes.Buffer
}

const (
	pageWidth, pageHeight = 595.0, 842.0 // A4
	margin                = 42.0
)

func (s *sheet) newPage() {
	s.page = &bytes.Buffer{}
	s.pages = append(s.pages, s.page)
}

func (s *sheet) line(x0, y0, x1, y1, width, grey float64) {
	fmt.Fprintf(s.page, "%.2f w %.2f G %.2f %.2f m %.2f %.2f l S\n", width, grey, x0, pageHeight-y0, x1, pageHeight-y1)
}

func (s *sheet) fill(x, y, w, h, grey float64) {
	fmt.Fprintf(s.page, "%.2f g %.2f %.2f %.2f %.2f re f\n", grey, x, pageHeight-y-h, w, h)
}

func (s *sheet) dot(x, y, r float64) { s.fill(x-r, y-r, 2*r, 2*r, 0) }

// curve draws a shallow arc from one point to another, hanging below them: a tie.
func (s *sheet) curve(x0, x1, y, drop float64) {
	fmt.Fprintf(s.page, "0.6 w 0 G %.2f %.2f m %.2f %.2f %.2f %.2f %.2f %.2f c S\n",
		x0, pageHeight-y, x0+(x1-x0)*0.25, pageHeight-y-drop, x0+(x1-x0)*0.75, pageHeight-y-drop, x1, pageHeight-y)
}

// encode turns text into what the standard fonts understand: Latin letters, accents included.
func encode(text string) string {
	var out strings.Builder
	for _, r := range text {
		switch {
		case r == '(' || r == ')' || r == '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case r >= 32 && r < 127:
			out.WriteRune(r)
		case r >= 160 && r <= 255:
			fmt.Fprintf(&out, "\\%03o", r)
		case r == '·':
			out.WriteString("\\267")
		case r == '–' || r == '—':
			out.WriteByte('-')
		case r == '’' || r == '‘':
			out.WriteByte('\'')
		default:
			out.WriteByte('?')
		}
	}
	return out.String()
}

// width of a line of text in Helvetica, near enough for placing it: digits and brackets
// exactly, everything else at the width of an average letter.
func width(text string, size float64) float64 {
	var units float64
	for _, r := range text {
		switch {
		case r >= '0' && r <= '9':
			units += 556
		case r == '(' || r == ')' || r == ' ':
			units += 333
		default:
			units += 580
		}
	}
	return units * size / 1000
}

// text writes a line: align is -1 for starting at x, 0 for centred on it, 1 for ending there.
func (s *sheet) text(text string, x, y, size float64, bold bool, grey float64, align int) {
	font := "F1"
	if bold {
		font = "F2"
	}
	switch align {
	case 0:
		x -= width(text, size) / 2
	case 1:
		x -= width(text, size)
	}
	fmt.Fprintf(s.page, "BT %.2f g /%s %.1f Tf %.2f %.2f Td (%s) Tj ET\n", grey, font, size, x, pageHeight-y, encode(text))
}

// bytes puts the pages together as a PDF file.
func (s *sheet) bytes() []byte {
	var out bytes.Buffer
	var offsets []int
	object := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	kids := make([]string, len(s.pages))
	for i := range s.pages {
		kids[i] = fmt.Sprintf("%d 0 R", 5+2*i)
	}
	object("<< /Type /Catalog /Pages 2 0 R >>")
	object(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(s.pages)))
	object("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	object("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	for i, page := range s.pages {
		object(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", pageWidth, pageHeight, 6+2*i))
		var packed bytes.Buffer
		z := zlib.NewWriter(&packed)
		z.Write(page.Bytes())
		z.Close()
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", len(offsets), packed.Len())
		out.Write(packed.Bytes())
		out.WriteString("\nendstream\nendobj\n")
	}
	start := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, start)
	return out.Bytes()
}

// PDF lays the part out on A4 paper: the tablature, the value of every note under it, the
// chords above, a few bars to a line.
func PDF(p *project.Project, names Names) []byte {
	part := layout(p)
	strings_ := len(part.tuning.Open)
	const (
		gutter   = 20.0 // for the names of the strings
		slot     = 6.6  // a sixteenth
		padding  = 9.0  // from a bar line to the first note of its bar
		spacing  = 9.0  // between two strings
		above    = 20.0 // for chords and bar numbers
		below    = 30.0 // for the values
		between  = 16.0
		usable   = pageWidth - 2*margin - gutter
		fretSize = 8.5
	)
	staff := float64(strings_-1) * spacing
	system := above + staff + below + between
	natural := func(b bar) float64 { return float64(b.beats*rhythm.Division)*slot + padding }

	s := &sheet{}
	s.newPage()
	s.text(p.Title, margin, margin+14, 18, true, 0, -1)
	var open []string
	for _, midi := range part.tuning.Open {
		open = append(open, names(midi))
	}
	s.text(fmt.Sprintf("%s  ·  %.0f BPM", strings.Join(open, " "), part.pulse.Tempo()), margin, margin+30, 10, false, 0.35, -1)
	y := margin + 52

	for from := 0; from < len(part.bars); {
		// as many bars as fit on the line
		to, used := from, 0.0
		for to < len(part.bars) && (to == from || used+natural(part.bars[to]) <= usable) {
			used += natural(part.bars[to])
			to++
		}
		stretch := usable / used
		if to == len(part.bars) && stretch > 1.6 {
			stretch = 1 // a short last line is left short
		}
		if y+system-between > pageHeight-margin-14 {
			s.newPage()
			y = margin
		}
		top := y + above
		x := margin + gutter
		for str := 0; str < strings_; str++ {
			lineY := top + staff - float64(str)*spacing
			s.line(x, lineY, x+used*stretch, lineY, 0.5, 0.55)
			s.text(names(part.tuning.Open[str]), margin+gutter-6, lineY+2.6, 7.5, false, 0.35, 1)
		}
		s.line(x, top, x, top+staff, 0.9, 0)
		for b := from; b < to; b++ {
			bar := part.bars[b]
			w := natural(bar) * stretch
			at := func(slotInBar int) float64 { return x + (padding+float64(slotInBar)*slot)*stretch }
			if bar.index >= 0 {
				s.text(fmt.Sprint(bar.index+1), x+2, top-2.5, 6, false, 0.45, -1)
			}
			for _, c := range bar.chords {
				s.text(chordName(c.chord.Root, c.chord.Quality, names), at(c.slot)-3, top-10, 9.5, true, 0, -1)
			}
			bottom := top + staff
			stemTop, stemBottom := bottom+7, bottom+21
			for i, symbol := range bar.symbols {
				if symbol.Rest {
					continue
				}
				sx := at(symbol.Slot)
				event := p.Events[symbol.Index]
				if event.String >= 0 && event.String < strings_ && (!symbol.Tied || symbol.Slot == 0) {
					label := fmt.Sprint(event.Fret)
					if symbol.Tied {
						label = "(" + label + ")"
					}
					lineY := top + staff - float64(event.String)*spacing
					half := width(label, fretSize)/2 + 1
					s.fill(sx-half, lineY-4.5, 2*half, 9, 1)
					s.text(label, sx, lineY+3, fretSize, true, 0, 0)
				}
				// the value: a stem, shorter for a half note, none for a whole one
				if symbol.Tied && i > 0 {
					s.curve(at(bar.symbols[i-1].Slot)+1, sx-1, stemBottom+3, 3.5)
				}
				switch {
				case symbol.Value >= 16:
					continue
				case symbol.Value >= 8:
					s.line(sx, stemTop+7, sx, stemBottom, 0.8, 0)
				default:
					s.line(sx, stemTop, sx, stemBottom, 0.8, 0)
				}
				if symbol.Value%3 == 0 {
					s.dot(sx+3, stemTop+4, 0.9)
				}
				if symbol.Value >= rhythm.Division {
					continue
				}
				short := func(v rhythm.Symbol) bool { return !v.Rest && v.Value < rhythm.Division }
				joined := func(a, b rhythm.Symbol) bool {
					return short(a) && short(b) && a.Slot/rhythm.Division == b.Slot/rhythm.Division && a.Slot+a.Value == b.Slot
				}
				before := i > 0 && joined(bar.symbols[i-1], symbol)
				after := i+1 < len(bar.symbols) && joined(symbol, bar.symbols[i+1])
				sixteenth := symbol.Value == 1
				switch {
				case after:
					nx := at(bar.symbols[i+1].Slot)
					s.fill(sx-0.4, stemBottom-1.8, nx-sx+0.8, 1.8, 0)
					if sixteenth && bar.symbols[i+1].Value == 1 {
						s.fill(sx-0.4, stemBottom-5.2, nx-sx+0.8, 1.8, 0)
					} else if sixteenth && !before {
						s.fill(sx, stemBottom-5.2, 4.5, 1.8, 0)
					}
				case before:
					if sixteenth && bar.symbols[i-1].Value != 1 {
						s.fill(sx-4.5, stemBottom-5.2, 4.5, 1.8, 0)
					}
				default:
					s.line(sx, stemBottom, sx+4, stemBottom-5, 1, 0)
					if sixteenth {
						s.line(sx, stemBottom-3.5, sx+4, stemBottom-8.5, 1, 0)
					}
				}
			}
			x += w
			s.line(x, top, x, top+staff, 0.9, 0)
		}
		y += system
		from = to
	}
	for i, page := range s.pages {
		s.page = page
		s.text("C_bass", margin, pageHeight-margin+14, 7, false, 0.5, -1)
		s.text(fmt.Sprintf("%d / %d", i+1, len(s.pages)), pageWidth-margin, pageHeight-margin+14, 7, false, 0.5, 1)
	}
	return s.bytes()
}

// chordName writes a chord with the root named as the page names notes.
func chordName(root int, quality string, names Names) string {
	return names(((root%12)+12)%12+12) + quality
}
