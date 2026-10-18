package target

import (
	"fmt"
	"regexp"

	"github.com/airiclenz/photon/internal/config"
)

// area is a config area with its anchors compiled. A nil edge is the screen's own edge.
type area struct {
	top    *anchor
	bottom *anchor
}

// compileArea compiles both edges of a declared area.
func compileArea(spec config.Area) (area, error) {
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

// rows returns the half-open row range [top, end) the area covers on screen. Both edges are
// inclusive, so an area whose two edges pick the same row is that one row.
func (a area) rows(screen []screenRow) (top, end int, err error) {
	top, bottom := 0, len(screen)-1
	if a.top != nil {
		if top, err = a.top.row(screen); err != nil {
			return 0, 0, err
		}
	}
	if a.bottom != nil {
		if bottom, err = a.bottom.row(screen); err != nil {
			return 0, 0, err
		}
	}
	if top > bottom {
		return 0, 0, fmt.Errorf("top row %d is below bottom row %d, so the area is %w",
			top, bottom, ErrNotOnScreen)
	}
	return top, bottom + 1, nil
}

// anchor is a config anchor with its regex compiled: it picks one row of a screen.
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

// row picks the anchor's row on screen: the first row whose text the pattern finds, or the last
// for [config.PickLast], searching only the rows strictly above or below the row the limiting
// anchor picks, then moved by the offset. A pick left empty is the first.
func (a *anchor) row(screen []screenRow) (int, error) {
	start, end := 0, len(screen)
	switch {
	case a.above != nil:
		limit, err := a.above.row(screen)
		if err != nil {
			return 0, err
		}
		end = limit
	case a.below != nil:
		limit, err := a.below.row(screen)
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
