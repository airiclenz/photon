package browser

import (
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/airiclenz/photon/internal/take"
)

// textLayerScript is the function the page runs to report its visible text nodes' graphemes and
// its selectors' boxes.
//
//go:embed textlayer.js
var textLayerScript string

// readTimeout caps one read of the page's text layer and selector boxes.
const readTimeout = 10 * time.Second

// renderedSpace is what a rendered whitespace grapheme reads as on the text layer: a newline or
// tab the page lays out as a gap between words shows as a space.
const renderedSpace = " "

// sameLineShare is how much of the shorter of two vertical extents they must overlap by to sit
// on one visual line. More than half keeps a label beside a padded button on one line while the
// glyph boxes of two tight lines (line-height below the font's own height), which overlap by a
// sliver, stay two.
const sameLineShare = 0.5

// ReadText reads the page's text layer as it is laid out now, and the boxes of the elements each
// of selectors picks, all in CSS pixels from the viewport's top left, rounded to whole pixels.
//
// The text layer is one [take.TextRun] per stretch of a text node on one visual line, in reading
// order. A run's Text is the text as the page renders it: collapsed whitespace dropped, a
// rendered newline or tab read as a space, no space at either end; its Edges give the column each
// byte's grapheme starts at, then the column just past its last grapheme. Runs whose vertical
// extents overlap by more than half the shorter one are one visual line, and every run on it
// takes the line's top and height, so the runs of a line share one Box.Y. Text that is hidden,
// not rendered or wholly outside the viewport has no run.
//
// The boxes are keyed by selector and hold, in document order, every element it matches that is
// visible, has an area and touches the viewport; a selector that matches none has no key. A
// selector the page refuses is an error naming it, as is a page that cannot be read within a
// fixed time.
func (b *Browser) ReadText(selectors []string) ([]take.TextRun, map[string][]take.Box, error) {
	call, err := textLayerCall(selectors)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(b.tab, readTimeout)
	defer cancel()
	page, err := chromedp.Run(ctx, chromedp.Evaluate[pageText](call))
	if err != nil {
		return nil, nil, fmt.Errorf("read the page's text layer: %w", err)
	}
	boxes, err := page.selectorBoxes(selectors)
	if err != nil {
		return nil, nil, err
	}
	return runsOf(page.Nodes), boxes, nil
}

// textLayerCall is the expression that calls textLayerScript with selectors.
func textLayerCall(selectors []string) (string, error) {
	// A nil slice would reach the page as null, which has no map.
	argument, err := json.Marshal(append([]string{}, selectors...))
	if err != nil {
		return "", fmt.Errorf("pass the selectors to the page: %w", err)
	}
	return "(" + textLayerScript + ")(" + string(argument) + ")", nil
}

// pageText is what textLayerScript reports: every visible text node's graphemes, in document
// order, and for each selector asked, in the order asked, its boxes or the page's refusal.
type pageText struct {
	Nodes     [][]glyph       `json:"nodes"`
	Selectors []selectorMatch `json:"selectors"`
}

// glyph is one grapheme of a text node and the rect it is laid out in, nil when the page does
// not render it or it lies wholly outside the viewport.
type glyph struct {
	Text string `json:"t"`
	Rect *rect  `json:"r"`
}

// selectorMatch is one selector's result on the page: the rects of the elements it matches, or
// the page's message when it refuses the selector.
type selectorMatch struct {
	Boxes []rect `json:"boxes"`
	Error string `json:"error"`
}

// rect is a rectangle on the page in fractional CSS pixels from the viewport's top left.
type rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// selectorBoxes returns the page's boxes keyed by the selectors asked, which name its
// selector results in order, leaving out a selector that matched nothing. It fails, naming each,
// when the page refused any selector.
func (p pageText) selectorBoxes(selectors []string) (map[string][]take.Box, error) {
	if len(p.Selectors) != len(selectors) {
		return nil, fmt.Errorf(
			"read selector boxes: the page answered %d selectors, want %d",
			len(p.Selectors),
			len(selectors),
		)
	}
	boxes := make(map[string][]take.Box, len(selectors))
	var refusals []error
	for i, match := range p.Selectors {
		switch {
		case match.Error != "":
			refusals = append(refusals, fmt.Errorf("selector %q: %s", selectors[i], match.Error))
		case len(match.Boxes) > 0:
			boxes[selectors[i]] = boxesOf(match.Boxes)
		}
	}
	if err := errors.Join(refusals...); err != nil {
		return nil, err
	}
	return boxes, nil
}

// boxesOf rounds rects to whole-pixel boxes, in order.
func boxesOf(rects []rect) []take.Box {
	boxes := make([]take.Box, len(rects))
	for i, r := range rects {
		left, top := pixel(r.X), pixel(r.Y)
		boxes[i] = take.Box{X: left, Y: top, W: pixel(r.X+r.W) - left, H: pixel(r.Y+r.H) - top}
	}
	return boxes
}

// pixel rounds a CSS-pixel coordinate to the nearest whole pixel.
func pixel(coordinate float64) int {
	return int(math.Round(coordinate))
}

// extent is a vertical stretch of the page, from top to bottom in fractional CSS pixels.
type extent struct {
	top    float64
	bottom float64
}

// extentOf returns the vertical stretch of a rect.
func extentOf(r rect) extent {
	return extent{top: r.Y, bottom: r.Y + r.H}
}

// union returns the smallest extent holding both e and other.
func (e extent) union(other extent) extent {
	return extent{top: min(e.top, other.top), bottom: max(e.bottom, other.bottom)}
}

// sharesLine reports whether e and other sit on one visual line: they overlap by more than
// sameLineShare of the shorter one.
func (e extent) sharesLine(other extent) bool {
	overlap := min(e.bottom, other.bottom) - max(e.top, other.top)
	shorter := min(e.bottom-e.top, other.bottom-other.top)
	return overlap > sameLineShare*shorter
}

// piece is a text node's rendered graphemes on one visual line, trimmed of spaces at both ends,
// and their vertical extent; a run once the line it sits on is known.
type piece struct {
	glyphs []glyph
	extent extent
}

// runsOf converts the page's text nodes to the text layer: each node split into pieces, the
// pieces grouped into visual lines top to bottom, each line's pieces left to right as runs that
// take the line's top and height.
func runsOf(nodes [][]glyph) []take.TextRun {
	var pieces []piece
	for _, node := range nodes {
		pieces = append(pieces, piecesOf(node)...)
	}
	slices.SortStableFunc(pieces, func(a, b piece) int {
		return cmp.Compare(a.extent.top, b.extent.top)
	})

	var runs []take.TextRun
	for first := 0; first < len(pieces); {
		line, end := pieces[first].extent, first+1
		for end < len(pieces) && line.sharesLine(pieces[end].extent) {
			line = line.union(pieces[end].extent)
			end++
		}
		runs = appendLine(runs, pieces[first:end], line)
		first = end
	}
	return runs
}

// piecesOf splits one text node's graphemes into a piece per visual line it is laid out on,
// dropping the graphemes it does not render and reading rendered whitespace as a space.
func piecesOf(node []glyph) []piece {
	var pieces []piece
	var current []glyph
	var line extent
	for _, g := range node {
		if g.Rect == nil || g.Rect.W <= 0 || g.Rect.H <= 0 {
			continue
		}
		next := extentOf(*g.Rect)
		switch {
		case len(current) == 0:
			line = next
		case line.sharesLine(next):
			line = line.union(next)
		default:
			pieces = appendPiece(pieces, current)
			current, line = nil, next
		}
		if strings.TrimSpace(g.Text) == "" {
			g.Text = renderedSpace
		}
		current = append(current, g)
	}
	return appendPiece(pieces, current)
}

// appendPiece appends the graphemes of one line of a text node to pieces, trimmed of spaces at
// both ends, and appends nothing when only spaces remain.
func appendPiece(pieces []piece, glyphs []glyph) []piece {
	isSpace := func(g glyph) bool { return g.Text == renderedSpace }
	first, end := 0, len(glyphs)
	for first < end && isSpace(glyphs[first]) {
		first++
	}
	for end > first && isSpace(glyphs[end-1]) {
		end--
	}
	if first == end {
		return pieces
	}
	trimmed := glyphs[first:end]
	stretch := extentOf(*trimmed[0].Rect)
	for _, g := range trimmed[1:] {
		stretch = stretch.union(extentOf(*g.Rect))
	}
	return append(pieces, piece{glyphs: trimmed, extent: stretch})
}

// appendLine appends one visual line's pieces to runs, left to right, each taking the line's
// top and height.
func appendLine(runs []take.TextRun, pieces []piece, line extent) []take.TextRun {
	slices.SortStableFunc(pieces, func(a, b piece) int {
		return cmp.Compare(a.glyphs[0].Rect.X, b.glyphs[0].Rect.X)
	})
	top := pixel(line.top)
	height := pixel(line.bottom) - top
	for _, p := range pieces {
		runs = append(runs, p.run(top, height))
	}
	return runs
}

// run converts the piece to a run at the given top and height: its graphemes' text, an edge per
// byte at the column its grapheme starts at, and an end edge just past its last grapheme.
func (p piece) run(top, height int) take.TextRun {
	var text strings.Builder
	edges := make([]int, 0, len(p.glyphs)+1)
	for _, g := range p.glyphs {
		column := pixel(g.Rect.X)
		for range len(g.Text) {
			edges = append(edges, column)
		}
		text.WriteString(g.Text)
	}
	last := p.glyphs[len(p.glyphs)-1].Rect
	end := pixel(last.X + last.W)
	edges = append(edges, end)
	return take.TextRun{
		Text:  text.String(),
		Box:   take.Box{X: edges[0], Y: top, W: end - edges[0], H: height},
		Edges: edges,
	}
}
