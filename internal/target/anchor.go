package target

import (
	"fmt"
	"regexp"

	"github.com/airiclenz/photon/internal/config"
)

// area is a config area compiled: a selector area's selector, or an anchor area's anchors, a nil
// edge being the screen's own edge.
type area struct {
	selector string
	top      *anchor
	bottom   *anchor
}

// compileArea compiles a declared area: a selector area keeps its selector, an anchor area has
// both its edges compiled.
func compileArea(spec config.Area) (area, error) {
	if spec.Selector != nil {
		return area{selector: *spec.Selector}, nil
	}
	top, err := compileAnchor("top", spec.Top)
	if err != nil {
		return area{}, err
	}
	bottom, err := compileAnchor("bottom", spec.Bottom)
	if err != nil {
		return area{}, err
	}
	return area{top: top, bottom: bottom}, nil
}

// lines returns the half-open range [top, end) of the screen's lines an anchor area covers. Both
// edges are inclusive, so an area whose two edges pick the same line is that one line.
func (a area) lines(screen []line) (top, end int, err error) {
	top, bottom := 0, len(screen)-1
	if a.top != nil {
		if top, err = a.top.line(screen); err != nil {
			return 0, 0, err
		}
	}
	if a.bottom != nil {
		if bottom, err = a.bottom.line(screen); err != nil {
			return 0, 0, err
		}
	}
	if top > bottom {
		return 0, 0, fmt.Errorf("top row %d is below bottom row %d, so the area is %w",
			top, bottom, ErrNotOnScreen)
	}
	return top, bottom + 1, nil
}

// anchor is a config anchor with its regex compiled: it picks one line of a screen.
type anchor struct {
	// path names the anchor within its area the way the config nests it, such as top or
	// bottom.above, so an error points at the field to fix.
	path    string
	pattern *regexp.Regexp
	pick    config.Pick
	above   *anchor
	below   *anchor
	offset  int
}

// compileAnchor compiles spec and the anchor it is limited by; a nil spec is a nil anchor.
func compileAnchor(path string, spec *config.Anchor) (*anchor, error) {
	if spec == nil {
		return nil, nil
	}
	pattern, err := regexp.Compile(spec.Match)
	if err != nil {
		return nil, fmt.Errorf("%s anchor: %w", path, err)
	}
	above, err := compileAnchor(path+".above", spec.Above)
	if err != nil {
		return nil, err
	}
	below, err := compileAnchor(path+".below", spec.Below)
	if err != nil {
		return nil, err
	}
	return &anchor{
		path: path, pattern: pattern, pick: spec.Pick,
		above: above, below: below, offset: spec.Offset,
	}, nil
}

// line picks the anchor's line on screen: the first line whose text the pattern finds, or the
// last for [config.PickLast], searching only the lines strictly above or below the line the
// limiting anchor picks, then moved by the offset in lines. A pick left empty is the first. On a
// terminal a line is a row, so errors count rows.
func (a *anchor) line(screen []line) (int, error) {
	start, end := 0, len(screen)
	switch {
	case a.above != nil:
		limit, err := a.above.line(screen)
		if err != nil {
			return 0, err
		}
		end = limit
	case a.below != nil:
		limit, err := a.below.line(screen)
		if err != nil {
			return 0, err
		}
		start = limit + 1
	}
	found := -1
	for y := start; y < end; y++ {
		if !a.pattern.MatchString(screen[y].text) {
			continue
		}
		found = y
		if a.pick != config.PickLast {
			break
		}
	}
	if found < 0 {
		return 0, fmt.Errorf("%s anchor /%s/: %w", a.path, a.pattern, ErrNotOnScreen)
	}
	moved := found + a.offset
	if moved < 0 || moved >= len(screen) {
		return 0, fmt.Errorf("%s anchor /%s/: row %d offset by %d is %w",
			a.path, a.pattern, found, a.offset, ErrNotOnScreen)
	}
	return moved, nil
}
