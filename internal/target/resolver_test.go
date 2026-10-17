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

func TestResolver_Rows(t *testing.T) {
	t.Parallel()
	resolver := newResolver(t)
	full := snapshotOf(t, chromeScreen)

	for _, tc := range []struct {
		area     string
		wantTop  int
		wantEnd  int
		wantText string
	}{
		{"footer", 8, 9, "  Auto 漢 model"},
		{"status", 3, 4, "  working  ctx 12%"},
		{"history", 0, 2, "  ctx 99% Auto in the history\n  deploy here"},
		{"to-first", 0, 1, "  ctx 99% Auto in the history"},
		{"below-rule", 3, 4, "  working  ctx 12%"},
		{"", 0, len(chromeScreen), strings.Join(chromeScreen, "\n")},
		{config.AreaAny, 0, len(chromeScreen), strings.Join(chromeScreen, "\n")},
	} {
		t.Run(tc.area, func(t *testing.T) {
			t.Parallel()
			top, end, err := resolver.Rows(full, tc.area)
			if err != nil || top != tc.wantTop || end != tc.wantEnd {
				t.Fatalf("Rows = [%d, %d), %v; want [%d, %d)", top, end, err, tc.wantTop, tc.wantEnd)
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
	if _, _, err := resolver.Rows(full, "footer"); err == nil {
		t.Fatal("Rows(footer) on the zero Resolver succeeded, want an undeclared-area error")
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
