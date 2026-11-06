package take

import "strings"

// blankGrapheme is what a cell with no grapheme shows on the text layer.
const blankGrapheme = " "

// TextRun is one stretch of a snapshot's text layer: its text, the [Box] it occupies on the grid,
// and where on the grid every byte of the text starts. Edges has one entry per byte of Text plus
// one for the end of the text, so the byte span [a, b) covers the grid columns
// [Edges[a], Edges[b]). A terminal row is one run; a browser page is a run per stretch of laid-out
// text.
type TextRun struct {
	Text  string `json:"text"`
	Box   Box    `json:"box"`
	Edges []int  `json:"edges"`
}

// Span returns the Box the byte span [a, b) of the run's text covers: the columns from Edges[a]
// to Edges[b], over the run's full height. It expects 0 <= a <= b <= len(Text).
func (r TextRun) Span(a, b int) Box {
	return Box{X: r.Edges[a], Y: r.Box.Y, W: r.Edges[b] - r.Edges[a], H: r.Box.H}
}

// Text returns the snapshot's text layer in reading order. A snapshot with cells derives it from
// them, one run per row, top to bottom: the row's text with trailing blanks trimmed, its Box the
// whole row and its Edges cell columns. A snapshot without cells returns its stored Runs.
func (s Snapshot) Text() []TextRun {
	if len(s.Cells) == 0 {
		return s.Runs
	}
	runs := make([]TextRun, len(s.Cells))
	for y, cells := range s.Cells {
		runs[y] = rowRun(y, cells)
	}
	return runs
}

// Bounds returns the size of the snapshot's grid as a Box from the origin. A snapshot with cells
// spans its longest row by its row count, so a ragged grid stays inside it; a snapshot without
// cells spans the union of its runs' boxes, which bounds the text but not the page, so it fits
// clamping a point onto the grid and never deciding what an area contains.
func (s Snapshot) Bounds() Box {
	if len(s.Cells) == 0 {
		return unionOf(s.Runs)
	}
	cols := 0
	for _, row := range s.Cells {
		cols = max(cols, len(row))
	}
	return Box{W: cols, H: len(s.Cells)}
}

// rowRun reads row y of a cell grid as one run: its text with trailing blanks trimmed, an empty
// grapheme read as a space, and the cells a wide grapheme covers after its first skipped, so a
// wide grapheme counts once. Each byte's edge is the column of the cell it came from; the end
// edge is the column just past the last non-blank grapheme (0 for a blank row). The run's Box is
// the whole row.
func rowRun(y int, cells []Cell) TextRun {
	var text strings.Builder
	edges := make([]int, 0, len(cells)+1)
	end, endCol := 0, 0
	for x, cell := range cells {
		if cell.Width == 0 {
			continue
		}
		grapheme := cell.Grapheme
		if grapheme == "" {
			grapheme = blankGrapheme
		}
		for range len(grapheme) {
			edges = append(edges, x)
		}
		text.WriteString(grapheme)
		if grapheme != blankGrapheme {
			end, endCol = text.Len(), x+cell.Width
		}
	}
	return TextRun{
		Text:  text.String()[:end],
		Box:   Box{X: 0, Y: y, W: len(cells), H: 1},
		Edges: append(edges[:end], endCol),
	}
}

// unionOf returns the smallest Box holding every run's Box, and the zero Box when there are no
// runs.
func unionOf(runs []TextRun) Box {
	if len(runs) == 0 {
		return Box{}
	}
	left, top := runs[0].Box.X, runs[0].Box.Y
	right, bottom := left+runs[0].Box.W, top+runs[0].Box.H
	for _, run := range runs[1:] {
		left, top = min(left, run.Box.X), min(top, run.Box.Y)
		right, bottom = max(right, run.Box.X+run.Box.W), max(bottom, run.Box.Y+run.Box.H)
	}
	return Box{X: left, Y: top, W: right - left, H: bottom - top}
}
