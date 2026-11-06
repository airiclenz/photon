package browser

import (
	"reflect"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/take"
)

// The fixed metrics of the hermetic pages: every grapheme glyphWidth wide and glyphHeight tall.
const (
	glyphWidth  = 8
	glyphHeight = 18
)

// node lays text out as one text node on one line from (x, y), a grapheme per rune, each
// glyphWidth wide and glyphHeight tall.
func node(text string, x, y float64) []glyph {
	var glyphs []glyph
	for _, r := range text {
		glyphs = append(glyphs, glyph{Text: string(r), Rect: &rect{X: x, Y: y, W: glyphWidth, H: glyphHeight}})
		x += glyphWidth
	}
	return glyphs
}

// unrendered is text the page does not render: every grapheme without a rect.
func unrendered(text string) []glyph {
	var glyphs []glyph
	for _, r := range text {
		glyphs = append(glyphs, glyph{Text: string(r)})
	}
	return glyphs
}

// concat joins the graphemes of several stretches into one text node.
func concat(stretches ...[]glyph) []glyph {
	var glyphs []glyph
	for _, stretch := range stretches {
		glyphs = append(glyphs, stretch...)
	}
	return glyphs
}

// edgesFrom returns the edges of an ASCII run starting at x: one column per byte, then the end.
func edgesFrom(x, bytes int) []int {
	edges := make([]int, bytes+1)
	for i := range edges {
		edges[i] = x + i*glyphWidth
	}
	return edges
}

func TestRunsOf(t *testing.T) {
	tests := []struct {
		name  string
		nodes [][]glyph
		want  []take.TextRun
	}{
		{
			name:  "ascii",
			nodes: [][]glyph{node("Save file", 10, 20)},
			want: []take.TextRun{
				{Text: "Save file", Box: take.Box{X: 10, Y: 20, W: 72, H: 18}, Edges: edgesFrom(10, 9)},
			},
		},
		{
			name:  "multi-byte runes take one edge per byte",
			nodes: [][]glyph{node("café", 0, 0)},
			want: []take.TextRun{
				{Text: "café", Box: take.Box{X: 0, Y: 0, W: 32, H: 18}, Edges: []int{0, 8, 16, 24, 24, 32}},
			},
		},
		{
			name:  "an empty node and an unrendered node have no run",
			nodes: [][]glyph{{}, unrendered("hidden")},
			want:  nil,
		},
		{
			name:  "a node of rendered spaces has no run",
			nodes: [][]glyph{node("   ", 0, 0)},
			want:  nil,
		},
		{
			name: "a pretty-printed node holds only its rendered text",
			nodes: [][]glyph{concat(
				unrendered("\n      "),
				node("Pretty\n", 0, 0),
				unrendered("      "),
				node("printed\n", 56, 0),
				unrendered("    "),
			)},
			want: []take.TextRun{
				{Text: "Pretty printed", Box: take.Box{X: 0, Y: 0, W: 112, H: 18}, Edges: edgesFrom(0, 14)},
			},
		},
		{
			name:  "zero-width graphemes are dropped",
			nodes: [][]glyph{concat(node("a", 0, 0), []glyph{{Text: " ", Rect: &rect{X: 8, H: glyphHeight}}}, node("b", 8, 0))},
			want: []take.TextRun{
				{Text: "ab", Box: take.Box{X: 0, Y: 0, W: 16, H: 18}, Edges: edgesFrom(0, 2)},
			},
		},
		{
			name:  "a wrapped node is a run per line",
			nodes: [][]glyph{concat(node("one ", 0, 0), node("two", 0, 20))},
			want: []take.TextRun{
				{Text: "one", Box: take.Box{X: 0, Y: 0, W: 24, H: 18}, Edges: edgesFrom(0, 3)},
				{Text: "two", Box: take.Box{X: 0, Y: 20, W: 24, H: 18}, Edges: edgesFrom(0, 3)},
			},
		},
		{
			name: "runs that overlap vertically share the line's top and height, left to right",
			nodes: [][]glyph{
				node("Go", 60, 13),
				node("Name", 0, 10),
			},
			want: []take.TextRun{
				{Text: "Name", Box: take.Box{X: 0, Y: 10, W: 32, H: 21}, Edges: edgesFrom(0, 4)},
				{Text: "Go", Box: take.Box{X: 60, Y: 10, W: 16, H: 21}, Edges: edgesFrom(60, 2)},
			},
		},
		{
			name: "tight lines whose glyph boxes overlap by a sliver stay two lines",
			nodes: [][]glyph{
				node("top", 0, 0),
				node("next", 0, 16),
			},
			want: []take.TextRun{
				{Text: "top", Box: take.Box{X: 0, Y: 0, W: 24, H: 18}, Edges: edgesFrom(0, 3)},
				{Text: "next", Box: take.Box{X: 0, Y: 16, W: 32, H: 18}, Edges: edgesFrom(0, 4)},
			},
		},
		{
			name: "fractional rects round to whole pixels",
			nodes: [][]glyph{{
				{Text: "a", Rect: &rect{X: 0.4, Y: 9.6, W: 7.4, H: 17.8}},
				{Text: "b", Rect: &rect{X: 7.8, Y: 9.6, W: 7.4, H: 17.8}},
			}},
			want: []take.TextRun{
				{Text: "ab", Box: take.Box{X: 0, Y: 10, W: 15, H: 17}, Edges: []int{0, 8, 15}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runsOf(tt.nodes)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("runsOf =\n %+v\nwant\n %+v", got, tt.want)
			}
		})
	}
}

func TestRunsOf_EveryRunHasAnEdgePerByteAndOne(t *testing.T) {
	nodes := [][]glyph{concat(unrendered("\n  "), node("naïve 日本\t", 0, 0), unrendered("\n"))}

	for _, run := range runsOf(nodes) {
		if len(run.Edges) != len(run.Text)+1 {
			t.Errorf("run %q has %d edges, want %d", run.Text, len(run.Edges), len(run.Text)+1)
		}
	}
}

func TestSelectorBoxes(t *testing.T) {
	page := pageText{Selectors: []selectorMatch{
		{Boxes: []rect{{X: 0.5, Y: 10.2, W: 100.4, H: 20}, {X: 0, Y: 40, W: 100, H: 20}}},
		{Boxes: []rect{}},
	}}

	got, err := page.selectorBoxes([]string{".item", "#missing"})

	if err != nil {
		t.Fatalf("selectorBoxes: %v", err)
	}
	want := map[string][]take.Box{".item": {{X: 1, Y: 10, W: 100, H: 20}, {X: 0, Y: 40, W: 100, H: 20}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("selectorBoxes = %+v, want %+v", got, want)
	}
}

func TestSelectorBoxes_RefusedSelectorIsAnErrorNamingIt(t *testing.T) {
	page := pageText{Selectors: []selectorMatch{
		{Boxes: []rect{{W: 1, H: 1}}},
		{Error: "'button >' is not a valid selector."},
	}}

	_, err := page.selectorBoxes([]string{"button", "button >"})

	if err == nil || !strings.Contains(err.Error(), `selector "button >"`) {
		t.Errorf("selectorBoxes error = %v, want one naming selector %q", err, "button >")
	}
}

func TestSelectorBoxes_AnswerCountMismatchIsAnError(t *testing.T) {
	page := pageText{Selectors: []selectorMatch{{}}}

	_, err := page.selectorBoxes([]string{"a", "b"})

	if err == nil {
		t.Error("selectorBoxes with fewer answers than selectors: got no error")
	}
}

func TestTextLayerCall_PassesSelectorsAsAnArray(t *testing.T) {
	tests := []struct {
		name      string
		selectors []string
		want      string
	}{
		{name: "nil", selectors: nil, want: ")([])"},
		{name: "quoted", selectors: []string{`a[title="x"]`}, want: `)(["a[title=\"x\"]"])`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := textLayerCall(tt.selectors)

			if err != nil {
				t.Fatalf("textLayerCall: %v", err)
			}
			if !strings.HasSuffix(got, tt.want) {
				t.Errorf("textLayerCall ends %q, want suffix %q", got[max(0, len(got)-40):], tt.want)
			}
		})
	}
}
