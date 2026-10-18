package target

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// ErrNotOnScreen is wrapped by every failure that depends only on what the snapshot shows: a
// target with no match, an nth past the matches, an anchor that finds no row or lands off the
// screen. A later snapshot may resolve where this one did not.
var ErrNotOnScreen = errors.New("not on screen")

// blankGrapheme is what a cell with no grapheme shows.
const blankGrapheme = " "

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

// Target finds the target on screen: every match of its regex within its area's rows, in reading
// order (top to bottom, left to right), from which Nth picks one — the first, the Nth, or the
// last, which is the lowest row's rightmost match. The box is one row high and spans the columns
// the match covers. An empty match never counts.
func (r Resolver) Target(screen take.Snapshot, target storyboard.Target) (take.Box, error) {
	pattern, err := regexp.Compile(target.Text)
	if err != nil {
		return take.Box{}, fmt.Errorf("target /%s/: %w", target.Text, err)
	}
	rows := rowsOf(screen)
	top, end, err := r.band(rows, target.Area)
	if err != nil {
		return take.Box{}, fmt.Errorf("target /%s/: %w", target.Text, err)
	}
	var matches []take.Box
	for y := top; y < end; y++ {
		row := rows[y]
		for _, span := range pattern.FindAllStringIndex(row.text, -1) {
			if span[0] == span[1] {
				continue
			}
			x := row.cols[span[0]]
			matches = append(matches, take.Box{X: x, Y: y, W: row.cols[span[1]] - x, H: 1})
		}
	}
	return pickMatch(matches, target)
}

// Rows returns the half-open row range [top, end) the named area covers on screen. An empty
// name or [config.AreaAny] is every row.
func (r Resolver) Rows(screen take.Snapshot, areaName string) (top, end int, err error) {
	return r.band(rowsOf(screen), areaName)
}

// Text returns what the named area shows on screen: its rows as plain text, trailing blanks
// trimmed, joined by newlines. An empty name or [config.AreaAny] is the whole screen.
func (r Resolver) Text(screen take.Snapshot, areaName string) (string, error) {
	rows := rowsOf(screen)
	top, end, err := r.band(rows, areaName)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, end-top)
	for _, row := range rows[top:end] {
		lines = append(lines, row.text)
	}
	return strings.Join(lines, "\n"), nil
}

// band resolves the named area on rows, every error naming the area.
func (r Resolver) band(rows []screenRow, areaName string) (top, end int, err error) {
	if areaName == "" || areaName == config.AreaAny {
		return 0, len(rows), nil
	}
	declared, isDeclared := r.areas[areaName]
	if !isDeclared {
		return 0, 0, fmt.Errorf("area %s: not declared in the config", areaName)
	}
	top, end, err = declared.rows(rows)
	if err != nil {
		return 0, 0, fmt.Errorf("area %s: %w", areaName, err)
	}
	return top, end, nil
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

// screenRow is one snapshot row as plain text, trailing blanks trimmed, with the terminal column
// every byte of the text starts in: cols has one entry per byte plus one for the end of the text,
// so a match's byte span [start, end) covers the columns [cols[start], cols[end]). The cells a
// wide grapheme covers after its first contribute no text, so a wide grapheme counts once.
type screenRow struct {
	text string
	cols []int
}

// rowsOf reads every row of screen.
func rowsOf(screen take.Snapshot) []screenRow {
	rows := make([]screenRow, len(screen.Cells))
	for y, cells := range screen.Cells {
		var text strings.Builder
		cols := make([]int, 0, len(cells)+1)
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
				cols = append(cols, x)
			}
			text.WriteString(grapheme)
			if grapheme != blankGrapheme {
				end, endCol = text.Len(), x+cell.Width
			}
		}
		rows[y] = screenRow{text: text.String()[:end], cols: append(cols[:end], endCol)}
	}
	return rows
}
