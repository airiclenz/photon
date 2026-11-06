package target

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// ErrNotOnScreen is wrapped by every failure that depends only on what the snapshot shows: a
// target with no match, an nth past the matches, an anchor that finds no line or lands off the
// screen, a selector the snapshot recorded no box for. A later snapshot may resolve where this
// one did not.
var ErrNotOnScreen = errors.New("not on screen")

// runSeparator joins the runs of one line into the line's text.
const runSeparator = " "

// Resolver resolves targets and areas on snapshots against one config's declared areas. The
// zero Resolver knows no areas, so it resolves only the whole screen (an empty area or
// [config.AreaAny]).
type Resolver struct {
	areas map[string]area
}

// New compiles the config's areas into a Resolver: an anchor area's anchors, or a selector area's
// selector beside them. A config from [config.Load] has every anchor regex checked already, so an
// error here means areas that did not come from one.
func New(areas map[string]config.Area) (Resolver, error) {
	compiled := make(map[string]area, len(areas))
	for name, spec := range areas {
		resolved, err := compileArea(spec)
		if err != nil {
			return Resolver{}, fmt.Errorf("area %s: %w", name, err)
		}
		compiled[name] = resolved
	}
	return Resolver{areas: compiled}, nil
}

// Target finds the target on screen and returns its box. A text target is every match of its
// regex within a run of the text layer; a selector target is every box the snapshot recorded for
// its selector ([take.Snapshot.Selectors]), a selector the snapshot holds no box for matching
// nothing. Only the matches inside the target's area count, in reading order (top to bottom,
// left to right), and Nth picks one — the first, the Nth, or the last, which is the lowest
// line's rightmost match. A text match's box spans the grid columns the match covers over its
// run's height. An empty text match never counts.
func (r Resolver) Target(screen take.Snapshot, target storyboard.Target) (take.Box, error) {
	var pattern *regexp.Regexp
	if target.Selector == "" {
		compiled, err := regexp.Compile(target.Text)
		if err != nil {
			return take.Box{}, fmt.Errorf("target %s: %w", labelOf(target), err)
		}
		pattern = compiled
	}
	lines := linesOf(screen)
	within, err := r.region(screen, lines, target.Area)
	if err != nil {
		return take.Box{}, fmt.Errorf("target %s: %w", labelOf(target), err)
	}
	if pattern == nil {
		return pickMatch(selectorMatches(screen, target.Selector, within), target)
	}
	return pickMatch(textMatches(lines, pattern, within), target)
}

// textMatches returns every non-empty match of pattern within a run the region holds, in reading
// order.
func textMatches(lines []line, pattern *regexp.Regexp, within region) []take.Box {
	var matches []take.Box
	for _, line := range lines {
		for _, run := range line.runs {
			if !within.holds(run.Box) {
				continue
			}
			for _, span := range pattern.FindAllStringIndex(run.Text, -1) {
				if span[0] == span[1] {
					continue
				}
				matches = append(matches, run.Span(span[0], span[1]))
			}
		}
	}
	return matches
}

// selectorMatches returns the boxes the snapshot recorded for selector that the region holds, in
// reading order.
func selectorMatches(screen take.Snapshot, selector string, within region) []take.Box {
	var matches []take.Box
	for _, box := range selectorBoxes(screen, selector) {
		if within.holds(box) {
			matches = append(matches, box)
		}
	}
	return matches
}

// selectorBoxes returns the boxes the snapshot recorded for selector in reading order; none when
// it recorded none.
func selectorBoxes(screen take.Snapshot, selector string) []take.Box {
	boxes := slices.Clone(screen.Selectors[selector])
	slices.SortStableFunc(boxes, readingOrder)
	return boxes
}

// labelOf names the target in an error: its regex between slashes, or its selector.
func labelOf(target storyboard.Target) string {
	if target.Selector != "" {
		return fmt.Sprintf("selector %q", target.Selector)
	}
	return "/" + target.Text + "/"
}

// Area returns the Box the named area covers on screen. An anchor area spans the full width of
// the grid, from the top of the line its top anchor picks to the bottom of the line its bottom
// anchor picks, and holds whatever lies within its top and bottom, whatever its columns. A
// selector area is the first box, in reading order, the snapshot recorded for its selector, and
// holds only what lies wholly inside it; a selector with no recorded box is not on screen. An
// empty name or [config.AreaAny] is the whole grid ([take.Snapshot.Bounds]).
func (r Resolver) Area(screen take.Snapshot, areaName string) (take.Box, error) {
	within, err := r.region(screen, linesOf(screen), areaName)
	return within.box, err
}

// Text returns what the named area shows on screen: each of its lines as plain text, the runs of
// the line the area holds joined by one space, the lines joined by newlines. A terminal line is a
// row, trailing blanks trimmed. An empty name or [config.AreaAny] is the whole screen.
func (r Resolver) Text(screen take.Snapshot, areaName string) (string, error) {
	lines := linesOf(screen)
	within, err := r.region(screen, lines, areaName)
	if err != nil {
		return "", err
	}
	shown := make([]string, 0, len(lines))
	for _, line := range lines {
		if text, isShown := line.textWithin(within); isShown {
			shown = append(shown, text)
		}
	}
	return strings.Join(shown, "\n"), nil
}

// extent says what a region holds.
type extent int

// The three extents: the whole screen holds everything, a band of lines whatever lies within its
// rows, an element's box only what lies wholly inside it.
const (
	wholeScreen extent = iota
	lineBand
	elementBox
)

// region is a resolved area: the box it covers and what it holds.
type region struct {
	box    take.Box
	extent extent
}

// holds reports whether inner lies in the region.
func (r region) holds(inner take.Box) bool {
	switch r.extent {
	case lineBand:
		return r.box.Y <= inner.Y && inner.Y+inner.H <= r.box.Y+r.box.H
	case elementBox:
		return r.box.X <= inner.X && inner.X+inner.W <= r.box.X+r.box.W &&
			r.box.Y <= inner.Y && inner.Y+inner.H <= r.box.Y+r.box.H
	}
	return true
}

// region resolves the named area on the screen's lines, every error naming the area.
func (r Resolver) region(screen take.Snapshot, lines []line, areaName string) (region, error) {
	grid := screen.Bounds()
	if areaName == "" || areaName == config.AreaAny {
		return region{box: grid, extent: wholeScreen}, nil
	}
	declared, isDeclared := r.areas[areaName]
	if !isDeclared {
		return region{}, fmt.Errorf("area %s: not declared in the config", areaName)
	}
	if declared.selector != "" {
		boxes := selectorBoxes(screen, declared.selector)
		if len(boxes) == 0 {
			return region{}, fmt.Errorf("area %s: selector %q: %w",
				areaName, declared.selector, ErrNotOnScreen)
		}
		return region{box: boxes[0], extent: elementBox}, nil
	}
	top, end, err := declared.lines(lines)
	if err != nil {
		return region{}, fmt.Errorf("area %s: %w", areaName, err)
	}
	band := take.Box{
		X: grid.X,
		Y: lines[top].top,
		W: grid.W,
		H: lines[end-1].bottom - lines[top].top,
	}
	return region{box: band, extent: lineBand}, nil
}

// pickMatch applies the target's Nth to its matches.
func pickMatch(matches []take.Box, target storyboard.Target) (take.Box, error) {
	where := target.Area
	if where == "" {
		where = config.AreaAny
	}
	switch {
	case len(matches) == 0:
		return take.Box{}, fmt.Errorf("target %s (area %s): %w",
			labelOf(target), where, ErrNotOnScreen)
	case target.Nth == storyboard.NthLast:
		return matches[len(matches)-1], nil
	case target.Nth <= storyboard.NthFirst:
		return matches[0], nil
	case int(target.Nth) > len(matches):
		return take.Box{}, fmt.Errorf("target %s (area %s): nth %d is %w, only %d match",
			labelOf(target), where, target.Nth, ErrNotOnScreen, len(matches))
	}
	return matches[target.Nth-1], nil
}

// line is one line of the text layer: the runs sharing a top edge, left to right, and the text
// an anchor reads, the runs' text joined by one space. A terminal row is one line holding one
// run, a blank row included.
type line struct {
	runs   []take.TextRun
	text   string
	top    int
	bottom int
}

// linesOf groups the screen's text layer into lines, top to bottom.
func linesOf(screen take.Snapshot) []line {
	runs := slices.Clone(screen.Text())
	slices.SortStableFunc(runs, func(a, b take.TextRun) int { return readingOrder(a.Box, b.Box) })
	var lines []line
	for first := 0; first < len(runs); {
		end := first + 1
		for end < len(runs) && runs[end].Box.Y == runs[first].Box.Y {
			end++
		}
		lines = append(lines, lineOf(runs[first:end]))
		first = end
	}
	return lines
}

// readingOrder orders boxes top to bottom by their top edge, then left to right.
func readingOrder(a, b take.Box) int {
	return cmp.Or(cmp.Compare(a.Y, b.Y), cmp.Compare(a.X, b.X))
}

// lineOf makes one line of runs that share a top edge and are in x order.
func lineOf(runs []take.TextRun) line {
	texts := make([]string, len(runs))
	bottom := runs[0].Box.Y
	for i, run := range runs {
		texts[i] = run.Text
		bottom = max(bottom, run.Box.Y+run.Box.H)
	}
	return line{
		runs:   runs,
		text:   strings.Join(texts, runSeparator),
		top:    runs[0].Box.Y,
		bottom: bottom,
	}
}

// textWithin returns the text of the line's runs the region holds, joined by one space, and
// whether it holds any.
func (l line) textWithin(within region) (string, bool) {
	var texts []string
	for _, run := range l.runs {
		if within.holds(run.Box) {
			texts = append(texts, run.Text)
		}
	}
	return strings.Join(texts, runSeparator), texts != nil
}
