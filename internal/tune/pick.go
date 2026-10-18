package tune

import (
	"cmp"
	"errors"
	"fmt"
	"image"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
	"github.com/airiclenz/photon/internal/yamledit"
)

// ErrNoProposal is wrapped by every pick that proposes no target: one starting off the screen,
// one over blank cells only, and one whose target the resolver does not resolve onto the picked
// cells.
var ErrNoProposal = errors.New("no target for the picked cells")

// The snippet keys of a [Proposal], one per place a target is written in a storyboard.
const (
	snippetZoom     = "zoom"
	snippetClick    = "click"
	snippetSeen     = "seen"
	snippetDragFrom = "drag-from"
	snippetDragTo   = "drag-to"
)

// Proposal is the target a pick proposes: the Target, the cell Box it resolves to on the picked
// snapshot, that box's Rect in output-frame pixels at the picked time, and the YAML that writes
// the target in each place a storyboard takes one, keyed by place:
//   - zoom: the bare target flow mapping ({text: 'ok', nth: 2, area: 'list'}), the text a
//     zoom or zoom.target [Edit] writes;
//   - click: a click action, click: {target: {…}};
//   - seen: an expect, seen: '…', or {seen: '…', area: '…'} when the target names an area;
//   - drag-from and drag-to: a drag's two ends, from: {…} and to: {…}.
type Proposal struct {
	Target   storyboard.Target
	Box      take.Box
	Rect     image.Rectangle
	Snippets map[string]string
}

// Pick proposes a target for the cells a rectangle from from to to picks, in output-frame pixels on
// the frame shown at output time out, mapped to cells through the last good pipeline's camera: the
// row is the one under from, the columns span from from to to, clamped to that row, and blank cells
// at either edge are dropped. The target is the picked text, regex-escaped. It is searched in the
// declared areas holding the row, fewest rows first and equal heights in name order, and the first
// where it is the only match names its area; else the smallest such area — or the whole screen when
// none holds the row — names it with nth counting to the picked match. The proposal is returned
// only when the resolver resolves that target to a box covering the picked cells. It fails with
// [render.ErrOutsideClip] when out is not in [0, length) and with [ErrNoProposal] when the pick
// starts off the screen, picks only blanks or does not resolve.
func (s *Session) Pick(out time.Duration, from, to image.Point) (Proposal, error) {
	shown := s.goodPipeline()

	screen, err := shown.frames.ScreenAt(out)
	if err != nil {
		return Proposal{}, err
	}
	startCol, row, ok := shown.frames.Cell(out, from)
	if !ok {
		return Proposal{}, fmt.Errorf("%w: the pick starts off the screen at %v", ErrNoProposal, from)
	}
	endCol, _, _ := shown.frames.Cell(out, to)
	picked, text, err := pickedCells(screen, row, min(startCol, endCol), max(startCol, endCol))
	if err != nil {
		return Proposal{}, err
	}

	resolver, err := target.New(shown.loaded.Config.Areas)
	if err != nil {
		return Proposal{}, err
	}
	proposer := proposer{resolver: resolver, screen: screen, picked: picked, text: regexp.QuoteMeta(text)}
	proposed, resolved, err := proposer.propose(shown.loaded.Config.Areas)
	if err != nil {
		return Proposal{}, err
	}
	return Proposal{
		Target:   proposed,
		Box:      resolved,
		Rect:     shown.frames.Rect(out, resolved),
		Snippets: snippetsOf(proposed),
	}, nil
}

// pickedCells returns the box and the text of the cells of row from column first to last
// inclusive, blank cells at either edge dropped. A wide grapheme counts whole when the pick
// touches any of its cells. It fails with [ErrNoProposal] when every picked cell is blank.
func pickedCells(screen take.Snapshot, row, first, last int) (take.Box, string, error) {
	var cells []take.Cell
	if row < len(screen.Cells) {
		cells = screen.Cells[row]
	}
	// A pick that starts on the trailing cell of a wide grapheme starts on the grapheme.
	for first > 0 && first < len(cells) && cells[first].Width == 0 {
		first--
	}
	last = min(last, len(cells)-1)
	for first <= last && isBlank(cells[first]) {
		first++
	}
	for last >= first && isBlank(cells[last]) {
		last--
	}
	if first > last {
		return take.Box{}, "", fmt.Errorf("%w: the picked cells of row %d are blank", ErrNoProposal, row)
	}
	end := last + 1
	for end < len(cells) && cells[end].Width == 0 {
		end++
	}
	var text strings.Builder
	for _, cell := range cells[first:end] {
		if cell.Width == 0 {
			continue
		}
		text.WriteString(cmp.Or(cell.Grapheme, " "))
	}
	return take.Box{X: first, Y: row, W: end - first, H: 1}, text.String(), nil
}

// isBlank reports whether cell shows nothing: an empty or space grapheme. The trailing cells of a
// wide grapheme are not blank; they belong to it.
func isBlank(cell take.Cell) bool {
	return cell.Width != 0 && (cell.Grapheme == "" || cell.Grapheme == " ")
}

// proposer finds the target for one pick: text, the escaped picked text, resolved on screen to
// the picked box.
type proposer struct {
	resolver target.Resolver
	screen   take.Snapshot
	picked   take.Box
	text     string
}

// candidate is a declared area holding the picked row, and how many rows it covers.
type candidate struct {
	name   string
	height int
}

// propose returns the target and the box it resolves to: the first area holding the picked row, smallest first, where the
// text matches only once, else the smallest such area, or the whole screen, with the nth of the
// picked match. The target is checked by resolving it again; it fails with [ErrNoProposal] when
// no match of the text covers the picked cells.
func (p proposer) propose(areas map[string]config.Area) (storyboard.Target, take.Box, error) {
	candidates, err := p.areasHolding(areas)
	if err != nil {
		return storyboard.Target{}, take.Box{}, err
	}
	for _, area := range candidates {
		matches, err := p.matches(area.name)
		if err != nil {
			return storyboard.Target{}, take.Box{}, err
		}
		if len(matches) == 1 && covers(matches[0], p.picked) {
			return p.checked(storyboard.Target{Text: p.text, Area: area.name})
		}
	}

	smallest := ""
	if len(candidates) > 0 {
		smallest = candidates[0].name
	}
	matches, err := p.matches(smallest)
	if err != nil {
		return storyboard.Target{}, take.Box{}, err
	}
	at := slices.IndexFunc(matches, func(match take.Box) bool { return covers(match, p.picked) })
	if at < 0 {
		return storyboard.Target{}, take.Box{}, fmt.Errorf("%w: no match of /%s/ covers the picked cells %+v",
			ErrNoProposal, p.text, p.picked)
	}
	proposed := storyboard.Target{Text: p.text, Area: smallest}
	if at > 0 {
		proposed.Nth = storyboard.Nth(at + 1)
	}
	return p.checked(proposed)
}

// areasHolding returns the declared areas whose rows on the screen hold the picked row, fewest
// rows first and equal heights in name order. An area the screen does not show holds nothing.
func (p proposer) areasHolding(areas map[string]config.Area) ([]candidate, error) {
	var holding []candidate
	for name := range areas {
		top, end, err := p.resolver.Rows(p.screen, name)
		if errors.Is(err, target.ErrNotOnScreen) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if top <= p.picked.Y && p.picked.Y < end {
			holding = append(holding, candidate{name: name, height: end - top})
		}
	}
	slices.SortFunc(holding, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(a.height, b.height), strings.Compare(a.name, b.name))
	})
	return holding, nil
}

// matches returns every match of the text in the area, in the resolver's reading order: the
// resolver is asked for the first, the second and on until it runs out.
func (p proposer) matches(area string) ([]take.Box, error) {
	var found []take.Box
	for nth := storyboard.NthFirst; ; nth++ {
		box, err := p.resolver.Target(p.screen, storyboard.Target{Text: p.text, Nth: nth, Area: area})
		if errors.Is(err, target.ErrNotOnScreen) {
			return found, nil
		}
		if err != nil {
			return nil, err
		}
		found = append(found, box)
	}
}

// checked returns proposed and its box when the resolver resolves it on the screen to a box
// covering the picked cells, and fails with [ErrNoProposal] naming why otherwise.
func (p proposer) checked(proposed storyboard.Target) (storyboard.Target, take.Box, error) {
	box, err := p.resolver.Target(p.screen, proposed)
	if err != nil {
		return storyboard.Target{}, take.Box{}, fmt.Errorf("%w: %w", ErrNoProposal, err)
	}
	if !covers(box, p.picked) {
		return storyboard.Target{}, take.Box{}, fmt.Errorf("%w: /%s/ resolves to %+v, not the picked cells %+v",
			ErrNoProposal, proposed.Text, box, p.picked)
	}
	return proposed, box, nil
}

// covers reports whether box holds every cell of inner.
func covers(box, inner take.Box) bool {
	return box.X <= inner.X && box.Y <= inner.Y &&
		box.X+box.W >= inner.X+inner.W && box.Y+box.H >= inner.Y+inner.H
}

// snippetsOf writes proposed as YAML for each place a storyboard takes a target. A seen expect
// has no nth: it passes on any match, so it carries the text and the area alone.
func snippetsOf(proposed storyboard.Target) map[string]string {
	flow := proposed.YAMLFlow()
	seen := "seen: " + yamledit.Quote(proposed.Text)
	if proposed.Area != "" {
		seen = "{" + seen + ", area: " + yamledit.Quote(proposed.Area) + "}"
	}
	return map[string]string{
		snippetZoom:     flow,
		snippetClick:    "click: {target: " + flow + "}",
		snippetSeen:     seen,
		snippetDragFrom: "from: " + flow,
		snippetDragTo:   "to: " + flow,
	}
}

// nthLast spells the last match's nth on the wire.
const nthLast = "last"
