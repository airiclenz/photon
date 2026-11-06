package target

import (
	"errors"
	"strings"
	"testing"
	"unicode"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// chromeScreen is an app's chrome in miniature: history, the heavy rule above the input box, a
// status row and a queued row under it, the input box, a footer row and the bottom hairline.
var chromeScreen = []string{
	"  ctx 99% Auto in the history",
	"  deploy here",
	"━━━━━━ title ━━━━━━",
	"  working  ctx 12%",
	"  queued: ctx 34%",
	"┌──────────────┐",
	"│ type here    │",
	"└──────────────┘",
	"  Auto 漢 model",
	"────────────────",
}

// screenCols is the width every test snapshot is padded to.
const screenCols = 32

// The anchors the test areas are built from.
var (
	bottomHairline = config.Anchor{Match: `^─+$`, Pick: config.PickLast, Offset: -1}
	inputBox       = config.Anchor{Match: `^┌`, Pick: config.PickLast}
	ruleAboveBox   = config.Anchor{Match: `^━`, Pick: config.PickLast, Above: &inputBox}
)

// chromeAreas declares the three areas of the chrome — the footer directly above the bottom
// hairline, the status row directly below the lowest rule above the input box and the history
// above that rule — plus a few that exercise the rest of an anchor.
func chromeAreas() map[string]config.Area {
	underRule := ruleAboveBox
	underRule.Offset = 1
	overRule := ruleAboveBox
	overRule.Offset = -1
	firstCtx := config.Anchor{Match: `ctx`}
	ctxBelowRule := config.Anchor{Match: `ctx`, Below: &config.Anchor{Match: `^━`}}
	return map[string]config.Area{
		"footer":      {Top: &bottomHairline, Bottom: &bottomHairline},
		"status":      {Top: &underRule, Bottom: &underRule},
		"history":     {Bottom: &overRule},
		"to-first":    {Bottom: &firstCtx},
		"below-rule":  {Top: &ctxBelowRule, Bottom: &ctxBelowRule},
		"above-top":   {Bottom: &config.Anchor{Match: `ctx 99`, Offset: -1}},
		"upside-down": {Top: &config.Anchor{Match: `^└`}, Bottom: &config.Anchor{Match: `^┌`}},
	}
}

// snapshotOf lays lines out as a snapshot screenCols wide: a Han character takes two cells, the
// second one covered, and the cells past a line's end are blank cells with no grapheme.
func snapshotOf(t *testing.T, lines []string) take.Snapshot {
	t.Helper()
	cells := make([][]take.Cell, len(lines))
	for y, line := range lines {
		row := make([]take.Cell, 0, screenCols)
		for _, r := range line {
			if unicode.Is(unicode.Han, r) {
				row = append(row, take.Cell{Grapheme: string(r), Width: 2}, take.Cell{})
				continue
			}
			row = append(row, take.Cell{Grapheme: string(r), Width: 1})
		}
		if len(row) > screenCols {
			t.Fatalf("line %d is %d cells, wider than the %d-column screen", y, len(row), screenCols)
		}
		for len(row) < screenCols {
			row = append(row, take.Cell{Width: 1})
		}
		cells[y] = row
	}
	return take.Snapshot{Cells: cells}
}

// newResolver builds a Resolver over chromeAreas.
func newResolver(t *testing.T) Resolver {
	t.Helper()
	resolver, err := New(chromeAreas())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return resolver
}

func TestResolver_Target(t *testing.T) {
	t.Parallel()
	resolver := newResolver(t)
	full := snapshotOf(t, chromeScreen)

	for _, tc := range []struct {
		name   string
		screen take.Snapshot
		target storyboard.Target
		want   take.Box
	}{
		{"first by default", full, storyboard.Target{Text: "ctx"}, take.Box{X: 2, Y: 0, W: 3, H: 1}},
		{"last is the lowest rightmost", full, storyboard.Target{Text: "ctx", Nth: storyboard.NthLast}, take.Box{X: 10, Y: 4, W: 3, H: 1}},
		{"nth counts in reading order", full, storyboard.Target{Text: "Auto", Nth: 2}, take.Box{X: 2, Y: 8, W: 4, H: 1}},
		{"a wide glyph counts once", full, storyboard.Target{Text: "model"}, take.Box{X: 10, Y: 8, W: 5, H: 1}},
		{"history stays above the rule", full, storyboard.Target{Text: "ctx", Nth: storyboard.NthLast, Area: "history"}, take.Box{X: 2, Y: 0, W: 3, H: 1}},
		{"status excludes the queued row", full, storyboard.Target{Text: `ctx \d+%`, Nth: storyboard.NthLast, Area: "status"}, take.Box{X: 11, Y: 3, W: 7, H: 1}},
		{"footer is above the hairline", full, storyboard.Target{Text: "Auto", Area: "footer"}, take.Box{X: 2, Y: 8, W: 4, H: 1}},
		{"any is the whole screen", full, storyboard.Target{Text: "Auto", Nth: storyboard.NthLast, Area: config.AreaAny}, take.Box{X: 2, Y: 8, W: 4, H: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			box, err := resolver.Target(tc.screen, tc.target)
			if err != nil || box != tc.want {
				t.Fatalf("Target = %+v, %v; want %+v", box, err, tc.want)
			}
		})
	}
}

func TestResolver_TargetFailures(t *testing.T) {
	t.Parallel()
	resolver := newResolver(t)
	full := snapshotOf(t, chromeScreen)
	noHairline := snapshotOf(t, chromeScreen[:len(chromeScreen)-1])
	noInputBox := snapshotOf(t, chromeScreen[:5])

	for _, tc := range []struct {
		name        string
		screen      take.Snapshot
		target      storyboard.Target
		says        []string
		notOnScreen bool
	}{
		{"no match", full, storyboard.Target{Text: "nowhere", Area: "footer"},
			[]string{"/nowhere/", "area footer", "not on screen"}, true},
		{"no match on the whole screen", full, storyboard.Target{Text: "nowhere"},
			[]string{"/nowhere/", "area any"}, true},
		{"nth past the matches", full, storyboard.Target{Text: "Auto", Nth: 3},
			[]string{"/Auto/", "area any", "nth 3", "only 2 match"}, true},
		{"anchor not on screen", noHairline, storyboard.Target{Text: "Auto", Area: "footer"},
			[]string{"/Auto/", "area footer", "top anchor /^─+$/", "not on screen"}, true},
		{"limiting anchor not on screen", noInputBox, storyboard.Target{Text: "ctx", Area: "status"},
			[]string{"/ctx/", "area status", "top.above anchor /^┌/"}, true},
		{"anchor offset off the screen", full, storyboard.Target{Text: "ctx", Area: "above-top"},
			[]string{"/ctx/", "area above-top", "bottom anchor /ctx 99/", "row 0 offset by -1"}, true},
		{"top edge below the bottom edge", full, storyboard.Target{Text: "type", Area: "upside-down"},
			[]string{"/type/", "area upside-down", "top row 7 is below bottom row 5"}, true},
		{"undeclared area", full, storyboard.Target{Text: "ctx", Area: "sidebar"},
			[]string{"/ctx/", "area sidebar", "not declared"}, false},
		{"bad regex", full, storyboard.Target{Text: "("},
			[]string{"/(/"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			box, err := resolver.Target(tc.screen, tc.target)
			if err == nil {
				t.Fatalf("Target = %+v, want an error", box)
			}
			for _, part := range tc.says {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error %q does not say %q", err, part)
				}
			}
			if errors.Is(err, ErrNotOnScreen) != tc.notOnScreen {
				t.Errorf("errors.Is(%q, ErrNotOnScreen) = %t, want %t", err, !tc.notOnScreen, tc.notOnScreen)
			}
		})
	}
}

func TestResolver_Area(t *testing.T) {
	t.Parallel()
	resolver := newResolver(t)
	full := snapshotOf(t, chromeScreen)

	for _, tc := range []struct {
		area     string
		want     take.Box
		wantText string
	}{
		{"footer", take.Box{X: 0, Y: 8, W: screenCols, H: 1}, "  Auto 漢 model"},
		{"status", take.Box{X: 0, Y: 3, W: screenCols, H: 1}, "  working  ctx 12%"},
		{"history", take.Box{X: 0, Y: 0, W: screenCols, H: 2}, "  ctx 99% Auto in the history\n  deploy here"},
		{"to-first", take.Box{X: 0, Y: 0, W: screenCols, H: 1}, "  ctx 99% Auto in the history"},
		{"below-rule", take.Box{X: 0, Y: 3, W: screenCols, H: 1}, "  working  ctx 12%"},
		{"", take.Box{X: 0, Y: 0, W: screenCols, H: len(chromeScreen)}, strings.Join(chromeScreen, "\n")},
		{config.AreaAny, take.Box{X: 0, Y: 0, W: screenCols, H: len(chromeScreen)}, strings.Join(chromeScreen, "\n")},
	} {
		t.Run(tc.area, func(t *testing.T) {
			t.Parallel()
			box, err := resolver.Area(full, tc.area)
			if err != nil || box != tc.want {
				t.Fatalf("Area = %+v, %v; want %+v", box, err, tc.want)
			}
			text, err := resolver.Text(full, tc.area)
			if err != nil || text != tc.wantText {
				t.Fatalf("Text = %q, %v; want %q", text, err, tc.wantText)
			}
		})
	}
}

func TestResolver_TextNamesTheArea(t *testing.T) {
	t.Parallel()
	noHairline := snapshotOf(t, chromeScreen[:len(chromeScreen)-1])

	_, err := newResolver(t).Text(noHairline, "footer")
	if err == nil || !strings.Contains(err.Error(), "area footer") || !errors.Is(err, ErrNotOnScreen) {
		t.Fatalf("Text = %v, want a not-on-screen error naming area footer", err)
	}
}

func TestResolver_ZeroValueKnowsOnlyTheWholeScreen(t *testing.T) {
	t.Parallel()
	full := snapshotOf(t, chromeScreen)
	var resolver Resolver

	if box, err := resolver.Target(full, storyboard.Target{Text: "deploy"}); err != nil || box != (take.Box{X: 2, Y: 1, W: 6, H: 1}) {
		t.Fatalf("Target = %+v, %v; want deploy on row 1", box, err)
	}
	if _, err := resolver.Area(full, "footer"); err == nil {
		t.Fatal("Area(footer) on the zero Resolver succeeded, want an undeclared-area error")
	}
}

func TestNew_RefusesABadAnchor(t *testing.T) {
	t.Parallel()
	areas := map[string]config.Area{
		"broken": {Bottom: &config.Anchor{Match: `x`, Above: &config.Anchor{Match: `(`}}},
	}

	_, err := New(areas)
	if err == nil || !strings.Contains(err.Error(), "area broken: bottom.above anchor") {
		t.Fatalf("New = %v, want an error naming area broken's bottom.above anchor", err)
	}
}

// pageScreen is a page's text layer laid out in CSS pixels, off any character cell: a heading, a
// line holding two runs (stored right one first) and a footer run that starts at a fractional
// cell. Each run's edges are the x of every byte, then its right edge.
func pageScreen() take.Snapshot {
	return take.Snapshot{Runs: []take.TextRun{
		{Text: "Inbox", Box: take.Box{X: 13, Y: 7, W: 41, H: 18}, Edges: []int{13, 21, 26, 33, 44, 54}},
		{Text: "Cancel", Box: take.Box{X: 90, Y: 40, W: 47, H: 18}, Edges: []int{90, 98, 106, 114, 122, 129, 137}},
		{Text: "Save", Box: take.Box{X: 12, Y: 40, W: 30, H: 18}, Edges: []int{12, 20, 27, 34, 42}},
		{Text: "Save draft", Box: take.Box{X: 12, Y: 300, W: 70, H: 16}, Edges: []int{12, 19, 26, 33, 40, 44, 51, 58, 65, 72, 82}},
	}}
}

// pageAreas declares the line holding Cancel as an area, and one from it to the page's end.
func pageAreas() map[string]config.Area {
	cancel := config.Anchor{Match: `Cancel`}
	return map[string]config.Area{
		"actions": {Top: &cancel, Bottom: &cancel},
		"lower":   {Top: &cancel},
	}
}

func TestResolver_TargetOnRuns(t *testing.T) {
	t.Parallel()
	resolver, err := New(pageAreas())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	page := pageScreen()

	for _, tc := range []struct {
		name   string
		target storyboard.Target
		want   take.Box
	}{
		{"box from the run's edges", storyboard.Target{Text: "Cancel"}, take.Box{X: 90, Y: 40, W: 47, H: 18}},
		{"a span inside a run", storyboard.Target{Text: "nc"}, take.Box{X: 106, Y: 40, W: 16, H: 18}},
		{"first of two runs on one line", storyboard.Target{Text: "a", Area: "actions"}, take.Box{X: 20, Y: 40, W: 7, H: 18}},
		{"nth counts across runs", storyboard.Target{Text: "a", Nth: 2, Area: "actions"}, take.Box{X: 98, Y: 40, W: 8, H: 18}},
		{"last across runs on one line", storyboard.Target{Text: "a", Nth: storyboard.NthLast, Area: "actions"}, take.Box{X: 98, Y: 40, W: 8, H: 18}},
		{"last is the lowest line", storyboard.Target{Text: "Save", Nth: storyboard.NthLast}, take.Box{X: 12, Y: 300, W: 28, H: 16}},
		{"area keeps to its lines", storyboard.Target{Text: "Save", Nth: storyboard.NthLast, Area: "actions"}, take.Box{X: 12, Y: 40, W: 30, H: 18}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			box, err := resolver.Target(page, tc.target)
			if err != nil || box != tc.want {
				t.Fatalf("Target = %+v, %v; want %+v", box, err, tc.want)
			}
		})
	}
}

func TestResolver_RegexNeverSpansRuns(t *testing.T) {
	t.Parallel()
	var resolver Resolver

	box, err := resolver.Target(pageScreen(), storyboard.Target{Text: "Save Cancel"})
	if !errors.Is(err, ErrNotOnScreen) {
		t.Fatalf("Target = %+v, %v; want a not-on-screen error", box, err)
	}
}

func TestResolver_AreaOnRuns(t *testing.T) {
	t.Parallel()
	resolver, err := New(pageAreas())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	page := pageScreen()

	for _, tc := range []struct {
		area     string
		want     take.Box
		wantText string
	}{
		{"actions", take.Box{X: 12, Y: 40, W: 125, H: 18}, "Save Cancel"},
		{"lower", take.Box{X: 12, Y: 40, W: 125, H: 276}, "Save Cancel\nSave draft"},
		{config.AreaAny, take.Box{X: 12, Y: 7, W: 125, H: 309}, "Inbox\nSave Cancel\nSave draft"},
	} {
		t.Run(tc.area, func(t *testing.T) {
			t.Parallel()
			box, err := resolver.Area(page, tc.area)
			if err != nil || box != tc.want {
				t.Fatalf("Area = %+v, %v; want %+v", box, err, tc.want)
			}
			text, err := resolver.Text(page, tc.area)
			if err != nil || text != tc.wantText {
				t.Fatalf("Text = %q, %v; want %q", text, err, tc.wantText)
			}
		})
	}
}

func TestResolver_RaggedRowFoundInItsArea(t *testing.T) {
	t.Parallel()
	ragged := take.Snapshot{Cells: [][]take.Cell{cellsOf("top"), cellsOf("a row longer than row 0 ok")}}
	resolver, err := New(map[string]config.Area{
		"under-top": {Top: &config.Anchor{Match: `^top$`, Offset: 1}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	box, err := resolver.Target(ragged, storyboard.Target{Text: "ok", Area: "under-top"})

	if want := (take.Box{X: 24, Y: 1, W: 2, H: 1}); err != nil || box != want {
		t.Fatalf("Target = %+v, %v; want %+v", box, err, want)
	}
}

func TestResolver_OffsetAcrossABlankRow(t *testing.T) {
	t.Parallel()
	screen := snapshotOf(t, []string{"head", "", "body", "end"})
	resolver, err := New(map[string]config.Area{
		"body":    {Top: &config.Anchor{Match: `^head`, Offset: 2}, Bottom: &config.Anchor{Match: `^end`, Offset: -1}},
		"to-body": {Bottom: &config.Anchor{Match: `^body`}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	box, err := resolver.Area(screen, "body")
	if want := (take.Box{X: 0, Y: 2, W: screenCols, H: 1}); err != nil || box != want {
		t.Fatalf("Area(body) = %+v, %v; want %+v", box, err, want)
	}
	text, err := resolver.Text(screen, "to-body")
	if err != nil || text != "head\n\nbody" {
		t.Fatalf("Text(to-body) = %q, %v; want %q", text, err, "head\n\nbody")
	}
}

// cellsOf lays line out as one-cell graphemes with no blank cells after it, so rows of different
// lengths make a ragged grid.
func cellsOf(line string) []take.Cell {
	cells := make([]take.Cell, 0, len(line))
	for _, r := range line {
		cells = append(cells, take.Cell{Grapheme: string(r), Width: 1})
	}
	return cells
}

// selectorScreen is pageScreen with the boxes a source recorded for its selectors: every button,
// stored out of reading order; two forms, the lower one stored first, the upper one holding Save
// but not Cancel; and a selector that matched nothing.
func selectorScreen() take.Snapshot {
	page := pageScreen()
	page.Selectors = map[string][]take.Box{
		"button": {
			{X: 90, Y: 40, W: 47, H: 18},
			{X: 12, Y: 300, W: 70, H: 16},
			{X: 12, Y: 40, W: 30, H: 18},
		},
		"form": {
			{X: 0, Y: 290, W: 200, H: 40},
			{X: 0, Y: 30, W: 100, H: 40},
		},
		".empty": {},
	}
	return page
}

// selectorAreas is pageAreas plus an area from the form selector and one from a selector the
// snapshot recorded nothing for.
func selectorAreas() map[string]config.Area {
	areas := pageAreas()
	form, gone := "form", "#gone"
	areas["form"] = config.Area{Selector: &form}
	areas["gone"] = config.Area{Selector: &gone}
	return areas
}

func TestResolver_TargetBySelector(t *testing.T) {
	t.Parallel()
	resolver, err := New(selectorAreas())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	page := selectorScreen()

	for _, tc := range []struct {
		name   string
		target storyboard.Target
		want   take.Box
	}{
		{"first in reading order", storyboard.Target{Selector: "button"}, take.Box{X: 12, Y: 40, W: 30, H: 18}},
		{"nth in reading order", storyboard.Target{Selector: "button", Nth: 2}, take.Box{X: 90, Y: 40, W: 47, H: 18}},
		{"last is the lowest", storyboard.Target{Selector: "button", Nth: storyboard.NthLast}, take.Box{X: 12, Y: 300, W: 70, H: 16}},
		{"inside an anchor area", storyboard.Target{Selector: "button", Nth: storyboard.NthLast, Area: "actions"}, take.Box{X: 90, Y: 40, W: 47, H: 18}},
		{"inside a selector area", storyboard.Target{Selector: "button", Nth: storyboard.NthLast, Area: "form"}, take.Box{X: 12, Y: 40, W: 30, H: 18}},
		{"text inside a selector area", storyboard.Target{Text: "a", Nth: storyboard.NthLast, Area: "form"}, take.Box{X: 20, Y: 40, W: 7, H: 18}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			box, err := resolver.Target(page, tc.target)
			if err != nil || box != tc.want {
				t.Fatalf("Target = %+v, %v; want %+v", box, err, tc.want)
			}
		})
	}
}

func TestResolver_SelectorNotOnScreen(t *testing.T) {
	t.Parallel()
	resolver, err := New(selectorAreas())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	page := selectorScreen()

	for _, tc := range []struct {
		name   string
		target storyboard.Target
		want   string
	}{
		{"a selector the snapshot never recorded", storyboard.Target{Selector: "#gone"}, `target selector "#gone" (area any)`},
		{"a selector that matched nothing", storyboard.Target{Selector: ".empty"}, `target selector ".empty" (area any)`},
		{"an nth past the boxes", storyboard.Target{Selector: "button", Nth: 4}, "nth 4"},
		{"a selector area with no box", storyboard.Target{Text: "Save", Area: "gone"}, `area gone: selector "#gone"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			box, err := resolver.Target(page, tc.target)
			if !errors.Is(err, ErrNotOnScreen) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Target = %+v, %v; want a not-on-screen error saying %q", box, err, tc.want)
			}
		})
	}
}

func TestResolver_SelectorArea(t *testing.T) {
	t.Parallel()
	resolver, err := New(selectorAreas())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	page := selectorScreen()

	box, boxErr := resolver.Area(page, "form")
	text, textErr := resolver.Text(page, "form")

	if want := (take.Box{X: 0, Y: 30, W: 100, H: 40}); boxErr != nil || box != want {
		t.Errorf("Area = %+v, %v; want %+v", box, boxErr, want)
	}
	if textErr != nil || text != "Save" {
		t.Errorf("Text = %q, %v; want %q", text, textErr, "Save")
	}
}
