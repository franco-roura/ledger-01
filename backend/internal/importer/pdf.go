package importer

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Some banks only offer statements as PDF (card statements, mostly). A PDF has no table: it
// places runs of text on a page. PDFLines reads them back as lines of positioned words, and a
// bank's statement reader (Bank.Statement) finds its columns by where its headings sit.

const (
	// maxPDFPages caps the pages read; statements are a few pages long.
	maxPDFPages = 30
	// maxPDFLines caps the lines kept across pages.
	maxPDFLines = 5_000
)

// IsPDF says whether content is a PDF file.
func IsPDF(content []byte) bool { return bytes.HasPrefix(content, []byte("%PDF-")) }

// Word is a run of text and where it starts and ends on its page, in points from the left edge.
type Word struct {
	X    float64
	End  float64
	Text string
}

// TextLine is the text on one baseline of a page, left to right. Y grows upwards, as in PDF.
type TextLine struct {
	Page  int
	Y     float64
	Words []Word
}

// Text is the line's words joined by single spaces.
func (l TextLine) Text() string {
	parts := make([]string, len(l.Words))
	for i, w := range l.Words {
		parts[i] = w.Text
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// PDFLines reads a PDF's text as lines, top to bottom, page by page. Glyphs on (nearly) the same
// baseline make a line; glyphs closer than a space apart make a word. It fails on PDFs without
// text (a scanned page) and on encrypted ones.
func PDFLines(content []byte) (lines []TextLine, err error) {
	if len(content) > MaxFileBytes {
		return nil, fmt.Errorf("the file is larger than %d MB", MaxFileBytes>>20)
	}
	// The reader panics on some malformed files; an upload must never take the API down.
	defer func() {
		if r := recover(); r != nil {
			lines, err = nil, fmt.Errorf("%w: the PDF can't be read", ErrUnsupported)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("%w: the PDF can't be read", ErrUnsupported)
	}
	for p := 1; p <= min(r.NumPage(), maxPDFPages); p++ {
		page := r.Page(p)
		if page.V.IsNull() {
			continue
		}
		lines = append(lines, pageLines(p, page.Content().Text)...)
		if len(lines) > maxPDFLines {
			return nil, fmt.Errorf("the PDF has more than %d lines of text", maxPDFLines)
		}
	}
	if len(lines) == 0 {
		return nil, errors.New("the PDF has no text (is it a scan?)")
	}
	return lines, nil
}

// pageLines groups a page's glyphs into lines and words.
func pageLines(page int, glyphs []pdf.Text) []TextLine {
	gs := make([]pdf.Text, 0, len(glyphs))
	var prev pdf.Text
	for _, g := range glyphs {
		if g.S == "" || g.FontSize <= 0 {
			continue
		}
		// The standard 14 fonts (Courier, Helvetica, …) carry no widths, so the reader leaves
		// every glyph of a run at the run's start with no width. Lay them out as monospace (a
		// Courier advance is 0.6 em), which is what statements printed in such fonts are.
		x := g.X
		if g.W == 0 && len(gs) > 0 && g.X == prev.X && g.Y == prev.Y && g.FontSize == prev.FontSize {
			x = gs[len(gs)-1].X + 0.6*g.FontSize
		}
		prev = g
		if g.W == 0 {
			g.W = 0.6 * g.FontSize
		}
		g.X = x
		gs = append(gs, g)
	}
	// Top to bottom, then left to right.
	sort.SliceStable(gs, func(i, j int) bool {
		if math.Abs(gs[i].Y-gs[j].Y) > 0.5 {
			return gs[i].Y > gs[j].Y
		}
		return gs[i].X < gs[j].X
	})
	var out []TextLine
	for i := 0; i < len(gs); {
		// A line: glyphs whose baselines are within a third of the font size of the first one.
		j, y := i, gs[i].Y
		for j < len(gs) && math.Abs(gs[j].Y-y) <= gs[i].FontSize/3 {
			j++
		}
		run := gs[i:j]
		sort.SliceStable(run, func(a, b int) bool { return run[a].X < run[b].X })
		line := TextLine{Page: page, Y: y}
		var cur strings.Builder
		start, end := 0.0, 0.0
		flush := func() {
			if t := strings.TrimSpace(cur.String()); t != "" {
				line.Words = append(line.Words, Word{X: start, End: end, Text: t})
			}
			cur.Reset()
		}
		for k, g := range run {
			// A gap of more than ~a third of the font size (or an actual space) splits words.
			if k > 0 && (g.X-end > g.FontSize*0.3 || g.S == " ") {
				flush()
			}
			if cur.Len() == 0 {
				start = g.X
			}
			if g.S != " " {
				cur.WriteString(g.S)
			}
			end = g.X + g.W
		}
		flush()
		if len(line.Words) > 0 {
			out = append(out, line)
		}
		i = j
	}
	return out
}
