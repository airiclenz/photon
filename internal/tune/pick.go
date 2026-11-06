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
	"github.com/airiclenz/photon/internal/render"
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

// FrameView is which frame a pick is made on: the camera's frame [Session.Frame] draws, or the
// full view [Session.FullFrame] draws.
type FrameView int

const (
	// ViewCamera is the frame the camera shows, its crop zoomed and aimed.
	ViewCamera FrameView = iota
	// ViewFull is the full view: the whole frame, whatever the camera's zoom and aim.
	ViewFull
)

// cell is the terminal cell under p, a pixel of the frame v names at output time out, as
// [render.Pipeline.Cell] or [render.Pipeline.FullViewCell] maps it.
func (v FrameView) cell(
	frames *render.Pipeline, out time.Duration, p image.Point,
) (col, row int, ok bool) {
	if v == ViewFull {
		return frames.FullViewCell(p)
	}
	return frames.Cell(out, p)
}

// rect is the pixel rectangle of the frame v names at output time out that the cell box b
// covers, as [render.Pipeline.Rect] or [render.Pipeline.FullViewRect] maps it.
func (v FrameView) rect(frames *render.Pipeline, out time.Duration, b take.Box) image.Rectangle {
	if v == ViewFull {
		return frames.FullViewRect(b)
	}
	return frames.Rect(out, b)
}

// Proposal is the target a pick proposes: the Target, the cell Box it resolves to on the picked
// snapshot, that box's Rect in pixels of the picked frame at the picked time, and the YAML that
// writes the target in each place a storyboard takes one, keyed by place:
//   - zoom: the bare target flow mapping ({text: 'ok', nth: 2, area: 'list'});
//   - click: a click action, click: {target: {…}};
//   - seen: an expect, seen: '…', or {seen: '…', area: '…'} when the target names an area;
//   - drag-from and drag-to: a drag's two ends, from: {…} and to: {…}.
//
// Selector is the selector target a pick on a page proposes beside the text one, in the same
// shape, nil when the pick lies in no recorded box of the clip's selectors. Its snippets have no
// seen, which takes no selector, and it has no Selector of its own.
type Proposal struct {
	Target   storyboard.Target
	Box      take.Box
	Rect     image.Rectangle
	Snippets map[string]string
	Selector *Proposal
}

// Pick proposes a target for the cells a rectangle from from to to picks, in output-frame pixels on
// the frame view shows at output time out, mapped to cells through the last good pipeline's camera,
// or through the whole frame on the full view, and the proposal's Rect is mapped alike; the
// row is the one under from, the columns span from from to to, clamped to that row, and blank cells
// at either edge are dropped. The target is the picked text, regex-escaped. It is searched in the
// declared areas holding the row, fewest rows first and equal heights in name order, and the first
// where it is the only match names its area; else the smallest such area — or the whole screen when
// none holds the row — names it with nth counting to the picked match. The proposal is returned
// only when the resolver resolves that target to a box covering the picked cells. Its Selector is
// the smallest box recorded for one of the clip's selectors that holds the picked cells, equal
// sizes in selector order then reading order, as that selector with nth counting to the box over
// the whole screen, resolver-checked alike; a terminal take records no boxes. It fails with
// [render.ErrOutsideClip] when out is not in [0, length), with [ErrNoProposal] when the pick
// starts off the screen, picks only blanks or does not resolve, and with [ErrNoPreview] when the
// last good pipeline holds no frames.
func (s *Session) Pick(
	out time.Duration, view FrameView, from, to image.Point,
) (Proposal, error) {
	shown, err := s.previewed()
	if err != nil {
		return Proposal{}, err
	}

	screen, err := shown.frames.ScreenAt(out)
	if err != nil {
		return Proposal{}, err
	}
	startCol, row, ok := view.cell(shown.frames, out, from)
	if !ok {
		return Proposal{}, fmt.Errorf(
			"%w: the pick starts off the screen at %v", ErrNoProposal, from,
		)
	}
	endCol, _, _ := view.cell(shown.frames, out, to)
	picked, text, err := pickedCells(screen, row, min(startCol, endCol), max(startCol, endCol))
	if err != nil {
		return Proposal{}, err
	}

	resolver, err := target.New(shown.loaded.Config.Areas)
	if err != nil {
		return Proposal{}, err
	}
	proposer := proposer{
		resolver: resolver, screen: screen, picked: picked, text: regexp.QuoteMeta(text),
	}
	proposed, resolved, err := proposer.propose(shown.loaded.Config.Areas)
	if err != nil {
		return Proposal{}, err
	}
	proposal := proposalOf(shown.frames, out, view, proposed, resolved)

	selected, selectedBox, isSelected, err := proposer.proposeSelector(shown.loaded.Selectors())
	if err != nil {
		return Proposal{}, err
	}
	if isSelected {
		selectorProposal := proposalOf(shown.frames, out, view, selected, selectedBox)
		proposal.Selector = &selectorProposal
	}
	return proposal, nil
}

// proposalOf is the proposal of target, resolved to box: box's rect on the frame view shows at
// output time out, and target's snippets.
func proposalOf(
	frames *render.Pipeline, out time.Duration, view FrameView,
	proposed storyboard.Target, box take.Box,
) Proposal {
	return Proposal{
		Target:   proposed,
		Box:      box,
		Rect:     view.rect(frames, out, box),
		Snippets: snippetsOf(proposed),
	}
}

// pickedCells returns the box and the text a pick of row from column first to last inclusive
// covers on the screen's text layer, blank graphemes at either edge dropped. It reads the first
// run, in reading order, whose box holds row and that shows a grapheme other than a blank within
// the pick. A grapheme is picked when its columns meet [first, last], so a wide grapheme counts
// whole when the pick touches any of its cells. It fails with [ErrNoProposal] when every picked
// grapheme is blank.
func pickedCells(screen take.Snapshot, row, first, last int) (take.Box, string, error) {
	for _, run := range screen.Text() {
		if row < run.Box.Y || row >= run.Box.Y+run.Box.H {
			continue
		}
		if start, end, ok := pickedSpan(run, first, last); ok {
			return run.Span(start, end), run.Text[start:end], nil
		}
	}
	return take.Box{}, "", fmt.Errorf(
		"%w: the picked cells of row %d are blank", ErrNoProposal, row,
	)
}

// pickedSpan returns the byte span [start, end) of run's text from the first to the last grapheme
// that is not blank and whose columns meet [first, last], and false when there is none. A
// grapheme's columns run from its first byte's edge to the edge of the byte after it.
func pickedSpan(run take.TextRun, first, last int) (start, end int, ok bool) {
	start = -1
	for at := 0; at < len(run.Text); {
		next := graphemeEnd(run, at)
		if run.Edges[at] <= last && run.Edges[next] > first && !isBlank(run.Text[at:next]) {
			if start < 0 {
				start = at
			}
			end = next
		}
		at = next
	}
	return start, end, start >= 0
}

// graphemeEnd returns the byte just past the grapheme of run that starts at byte at: the first
// byte after it that starts on another column, or the end of the text. The bytes of one grapheme
// share its edge.
func graphemeEnd(run take.TextRun, at int) int {
	next := at + 1
	for next < len(run.Text) && run.Edges[next] == run.Edges[at] {
		next++
	}
	return next
}

// isBlank reports whether grapheme shows nothing: it is empty or a space.
func isBlank(grapheme string) bool {
	return grapheme == "" || grapheme == " "
}

// proposer finds the target for one pick: text, the escaped picked text, resolved on screen to
// the picked box.
type proposer struct {
	resolver target.Resolver
	screen   take.Snapshot
	picked   take.Box
	text     string
}

// candidate is a declared area holding the picked row, and the height of its box.
type candidate struct {
	name   string
	height int
}

// propose returns the target and the box it resolves to: the first area holding the picked row,
// smallest first, where the text matches only once, else the smallest such area, or the whole
// screen, with the nth of the picked match. The target is checked by resolving it again; it fails
// with [ErrNoProposal] when no match of the text covers the picked cells.
func (p proposer) propose(areas map[string]config.Area) (storyboard.Target, take.Box, error) {
	candidates, err := p.areasHolding(areas)
	if err != nil {
		return storyboard.Target{}, take.Box{}, err
	}
	for _, area := range candidates {
		matches, err := p.matches(storyboard.Target{Text: p.text, Area: area.name})
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
	matches, err := p.matches(storyboard.Target{Text: p.text, Area: smallest})
	if err != nil {
		return storyboard.Target{}, take.Box{}, err
	}
	at := slices.IndexFunc(matches, func(match take.Box) bool { return covers(match, p.picked) })
	if at < 0 {
		return storyboard.Target{}, take.Box{}, fmt.Errorf(
			"%w: no match of /%s/ covers the picked cells %+v", ErrNoProposal, p.text, p.picked,
		)
	}
	proposed := storyboard.Target{Text: p.text, Area: smallest}
	if at > 0 {
		proposed.Nth = storyboard.Nth(at + 1)
	}
	return p.checked(proposed)
}

// areasHolding returns the declared areas whose box on the screen holds the picked cells within
// its top and bottom, whatever their columns, shortest first and equal heights in name order. An
// area the screen does not show holds nothing.
func (p proposer) areasHolding(areas map[string]config.Area) ([]candidate, error) {
	var holding []candidate
	for name := range areas {
		box, err := p.resolver.Area(p.screen, name)
		if errors.Is(err, target.ErrNotOnScreen) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if box.Y <= p.picked.Y && p.picked.Y+p.picked.H <= box.Y+box.H {
			holding = append(holding, candidate{name: name, height: box.H})
		}
	}
	slices.SortFunc(holding, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(a.height, b.height), strings.Compare(a.name, b.name))
	})
	return holding, nil
}

// proposeSelector returns the selector target for the smallest box, by area, that one of
// selectors has on the screen holding the picked cells, equal sizes in the order selectors lists
// them and then in reading order, and the box it resolves to. Its nth counts to that box among
// the selector's matches over the whole screen, and it is checked by resolving it again. It
// reports false when no box of selectors holds the picked cells.
func (p proposer) proposeSelector(
	selectors []string,
) (storyboard.Target, take.Box, bool, error) {
	var proposed storyboard.Target
	smallest := take.Box{}
	for _, selector := range selectors {
		matches, err := p.matches(storyboard.Target{Selector: selector})
		if err != nil {
			return storyboard.Target{}, take.Box{}, false, err
		}
		for at, match := range matches {
			isSmaller := proposed.Selector == "" || match.W*match.H < smallest.W*smallest.H
			if !covers(match, p.picked) || !isSmaller {
				continue
			}
			proposed, smallest = storyboard.Target{Selector: selector}, match
			if at > 0 {
				proposed.Nth = storyboard.Nth(at + 1)
			}
		}
	}
	if proposed.Selector == "" {
		return storyboard.Target{}, take.Box{}, false, nil
	}
	checked, box, err := p.checked(proposed)
	return checked, box, err == nil, err
}

// matches returns every match of wanted, whatever its nth, in the resolver's reading order: the
// resolver is asked for the first, the second and on until it runs out.
func (p proposer) matches(wanted storyboard.Target) ([]take.Box, error) {
	var found []take.Box
	for nth := storyboard.NthFirst; ; nth++ {
		wanted.Nth = nth
		box, err := p.resolver.Target(p.screen, wanted)
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
		return storyboard.Target{}, take.Box{}, fmt.Errorf(
			"%w: %s resolves to %+v, not the picked cells %+v",
			ErrNoProposal, proposed.YAMLFlow(), box, p.picked,
		)
	}
	return proposed, box, nil
}

// covers reports whether box holds every cell of inner.
func covers(box, inner take.Box) bool {
	return box.X <= inner.X && box.Y <= inner.Y &&
		box.X+box.W >= inner.X+inner.W && box.Y+box.H >= inner.Y+inner.H
}

// snippetsOf writes proposed as YAML for each place a storyboard takes a target. A seen expect
// has no nth: it passes on any match, so it carries the text and the area alone, and it takes no
// selector, so a selector target has no seen snippet.
func snippetsOf(proposed storyboard.Target) map[string]string {
	flow := proposed.YAMLFlow()
	snippets := map[string]string{
		snippetZoom:     flow,
		snippetClick:    "click: {target: " + flow + "}",
		snippetDragFrom: "from: " + flow,
		snippetDragTo:   "to: " + flow,
	}
	if proposed.Selector != "" {
		return snippets
	}
	seen := "seen: " + yamledit.Quote(proposed.Text)
	if proposed.Area != "" {
		seen = "{" + seen + ", area: " + yamledit.Quote(proposed.Area) + "}"
	}
	snippets[snippetSeen] = seen
	return snippets
}
