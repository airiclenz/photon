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
// screen. A later snapshot may resolve where this one did not.
var ErrNotOnScreen = errors.New("not on screen")

// runSeparator joins the runs of one line into the line's text.
const runSeparator = " "

// Resolver resolves targets and areas on snapshots against one config's declared areas. The
// zero Resolver knows no areas, so it resolves only the whole screen (an empty area or
// [config.AreaAny]).
type Resolver struct {
	areas map[string]area
}

// New compiles the config's areas into a Resolver. A config from [config.Load] has every anchor
// regex checked already, so an error here means areas that did not come from one.
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

// Target finds the target on screen: every match of its regex within a run of the text layer
// that lies inside its area, in reading order (top to bottom, left to right), from which Nth picks
// one — the first, the Nth, or the last, which is the lowest line's rightmost match. The box spans
// the grid columns the match covers over its run's height. An empty match never counts.
func (r Resolver) Target(screen take.Snapshot, target storyboard.Target) (take.Box, error) {
	pattern, err := regexp.Compile(target.Text)
	if err != nil {
		return take.Box{}, fmt.Errorf("target /%s/: %w", target.Text, err)
	}
	lines := linesOf(screen)
	box, err := r.box(screen, lines, target.Area)
	if err != nil {
		return take.Box{}, fmt.Errorf("target /%s/: %w", target.Text, err)
	}
	var matches []take.Box
	for _, line := range lines {
		for _, run := range line.runs {
			if !withinBand(run.Box, box) {
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
	return pickMatch(matches, target)
}

// Area returns the Box the named area covers on screen: the full width of the grid, from the top
// of the line its top anchor picks to the bottom of the line its bottom anchor picks. An empty
// name or [config.AreaAny] is the whole grid ([take.Snapshot.Bounds]). The area holds whatever
// lies within its top and bottom, whatever its columns.
func (r Resolver) Area(screen take.Snapshot, areaName string) (take.Box, error) {
	return r.box(screen, linesOf(screen), areaName)
}

// Text returns what the named area shows on screen: each of its lines as plain text, the line's
// runs joined by one space, the lines joined by newlines. A terminal line is a row, trailing
// blanks trimmed. An empty name or [config.AreaAny] is the whole screen.
func (r Resolver) Text(screen take.Snapshot, areaName string) (string, error) {
	lines := linesOf(screen)
	box, err := r.box(screen, lines, areaName)
	if err != nil {
		return "", err
	}
	shown := make([]string, 0, len(lines))
	for _, line := range lines {
		if text, isShown := line.textWithin(box); isShown {
			shown = append(shown, text)
		}
	}
	return strings.Join(shown, "\n"), nil
}

// box resolves the named area on the screen's lines, every error naming the area.
func (r Resolver) box(screen take.Snapshot, lines []line, areaName string) (take.Box, error) {
	grid := screen.Bounds()
	if areaName == "" || areaName == config.AreaAny {
		return grid, nil
	}
	declared, isDeclared := r.areas[areaName]
	if !isDeclared {
		return take.Box{}, fmt.Errorf("area %s: not declared in the config", areaName)
	}
	top, end, err := declared.lines(lines)
	if err != nil {
		return take.Box{}, fmt.Errorf("area %s: %w", areaName, err)
	}
	return take.Box{
		X: grid.X,
		Y: lines[top].top,
		W: grid.W,
		H: lines[end-1].bottom - lines[top].top,
	}, nil
}

// withinBand reports whether inner lies within the rows of area, whatever its columns.
func withinBand(inner, area take.Box) bool {
	return area.Y <= inner.Y && inner.Y+inner.H <= area.Y+area.H
}

// pickMatch applies the target's Nth to its matches.
func pickMatch(matches []take.Box, target storyboard.Target) (take.Box, error) {
	where := target.Area
	if where == "" {
		where = config.AreaAny
	}
	switch {
	case len(matches) == 0:
		return take.Box{}, fmt.Errorf("target /%s/ (area %s): %w",
			target.Text, where, ErrNotOnScreen)
	case target.Nth == storyboard.NthLast:
		return matches[len(matches)-1], nil
	case target.Nth <= storyboard.NthFirst:
		return matches[0], nil
	case int(target.Nth) > len(matches):
		return take.Box{}, fmt.Errorf("target /%s/ (area %s): nth %d is %w, only %d match",
			target.Text, where, target.Nth, ErrNotOnScreen, len(matches))
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
	slices.SortStableFunc(runs, func(a, b take.TextRun) int {
		return cmp.Or(cmp.Compare(a.Box.Y, b.Box.Y), cmp.Compare(a.Box.X, b.Box.X))
	})
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

// textWithin returns the text of the line's runs that lie within the rows of area, joined by one
// space, and whether any does.
func (l line) textWithin(area take.Box) (string, bool) {
	var texts []string
	for _, run := range l.runs {
		if withinBand(run.Box, area) {
			texts = append(texts, run.Text)
		}
	}
	return strings.Join(texts, runSeparator), texts != nil
}
