package compose

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// The zoom's auto-fit rule: with no factor given, the target box plus autoFitMarginCells on each
// side fills autoFitShare of the frame width, the factor clamped to [autoFitMin, autoFitMax].
const (
	autoFitMarginCells = 2
	autoFitShare       = 0.70
	autoFitMin         = 1.25
	autoFitMax         = 2.5
)

// The Text of a drag's two target events in a take, naming which end of the drag the Box is.
const (
	dragFromText = "from"
	dragToText   = "to"
)

// zoomPlanner plans the camera of one clip from its parts: its beats' zooms and its storyboard's
// camera moves.
type zoomPlanner struct {
	board    *storyboard.Storyboard
	recorded *take.Take
	resolver target.Resolver
	layout   Layout
}

// plan plans every kept section's zoom onto the camera's tracks — its target box, the factor
// given or auto-fit, and its ramps (zero means [storyboard.DefaultZoomRamp]) shrunk in
// proportion when together they outrun the section — and then every camera move of the
// storyboard beside them, as [zoomPlanner.planMoves] does.
func (p zoomPlanner) plan(clipSchedule *schedule.Schedule) (camera, error) {
	sections := make(map[int]schedule.Section, len(clipSchedule.Sections))
	for _, section := range clipSchedule.Sections {
		sections[section.Beat] = section
	}
	var zooms []move[float64]
	var pans []move[pointF]
	for _, beat := range p.board.Beats {
		section, isScheduled := sections[beat.ID]
		if beat.Zoom == nil || !isScheduled || section.Cut {
			continue
		}
		box, err := p.zoomBox(beat, section)
		if err != nil {
			return camera{}, fmt.Errorf("compose: beat %d: zoom: %w", beat.ID, err)
		}
		push, pull, aim := p.planSection(beat.Zoom, section, box)
		zooms = append(zooms, push, pull)
		pans = append(pans, aim)
	}
	moveZooms, movePans, err := p.planMoves(clipSchedule)
	if err != nil {
		return camera{}, err
	}
	return newCamera(p.layout, append(zooms, moveZooms...), append(pans, movePans...)), nil
}

// planSection is the zoom of one section onto box as camera moves: aim sets the pan track to the
// box's centre at the section's start, push zooms from 1 to the factor over the in ramp from
// there, and pull zooms back to 1 over the out ramp, ending as the section ends.
func (p zoomPlanner) planSection(
	zoom *storyboard.Zoom,
	section schedule.Section,
	box take.Box,
) (push, pull move[float64], aim move[pointF]) {
	start, end := section.OutStart, section.OutStart+section.Duration
	in := cmp.Or(zoom.In, storyboard.DefaultZoomRamp)
	out := cmp.Or(zoom.Out, storyboard.DefaultZoomRamp)
	if ramps := in + out; ramps > section.Duration {
		share := float64(section.Duration) / float64(ramps)
		in = time.Duration(float64(in) * share)
		out = time.Duration(float64(out) * share)
	}
	factor := zoom.Factor
	if factor == 0 {
		factor = autoFitFactor(box, p.layout)
	}
	push = move[float64]{Start: start, Over: in, To: factor}
	pull = move[float64]{Start: end - out, Over: out, To: 1}
	aim = move[pointF]{Start: start, To: p.layout.boxRect(box).centre()}
	return push, pull, aim
}

// zoomBox finds the beat's zoom target: the box a click or a drag of the same beat resolved on
// the same target, as the take logged it, else the target resolved on the beat's snapshots —
// the latest one inside the beat's span that shows it, walking back to the snapshot in effect
// at the beat's start, so the push lands where the beat settles and a target gone by the beat's
// end still zooms.
func (p zoomPlanner) zoomBox(beat storyboard.Beat, section schedule.Section) (take.Box, error) {
	if box, isLogged := p.loggedBox(beat); isLogged {
		return box, nil
	}
	until := section.End
	if section.End <= section.Start {
		// A beat of no length has no snapshot of its own to stop before.
		until = math.MaxInt64
	}
	return p.resolveShown(beat.Zoom.Target, p.recorded.ScreensDuring(section.Start, until), "the beat")
}

// resolveShown resolves goal on the shown snapshots, indices into the take's oldest first: the
// newest one that shows it, walking back to the oldest. It fails with the last
// [target.ErrNotOnScreen] when none of them shows goal, at once on any other resolver error, and
// when shown is empty, naming the stretch where.
func (p zoomPlanner) resolveShown(goal storyboard.Target, shown []int, where string) (take.Box, error) {
	var lastErr error
	for _, index := range slices.Backward(shown) {
		box, err := p.resolver.Target(p.recorded.Snapshots[index], goal)
		if err == nil {
			return box, nil
		}
		if !errors.Is(err, target.ErrNotOnScreen) {
			return take.Box{}, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("target /%s/: the take has no snapshot in %s", goal.Text, where)
	}
	return take.Box{}, lastErr
}

// loggedBox is the box the take logged for a click of beat on the zoom's own target, or for a
// drag of beat whose `from` target, or `to` target, is the zoom's, if any.
func (p zoomPlanner) loggedBox(beat storyboard.Beat) (take.Box, bool) {
	for _, event := range p.recorded.Events {
		if event.Kind != take.EventTarget || event.Beat != beat.ID || event.Box == nil ||
			event.Action < 0 || event.Action >= len(beat.Do) {
			continue
		}
		action := beat.Do[event.Action]
		if click := action.Click; click != nil && click.Target == beat.Zoom.Target {
			return *event.Box, true
		}
		if drag := action.Drag; drag != nil && dragEndIs(*drag, event.Text, beat.Zoom.Target) {
			return *event.Box, true
		}
	}
	return take.Box{}, false
}

// dragEndIs reports whether the end of drag a target event's text names was resolved on
// zoomed: `from` its From target, `to` its To target — never an Offset, which has none.
func dragEndIs(drag storyboard.DragAction, text string, zoomed storyboard.Target) bool {
	switch text {
	case dragFromText:
		return drag.From == zoomed
	case dragToText:
		return drag.To.Offset == nil && drag.To.Target == zoomed
	default:
		return false
	}
}

// autoFitFactor is the magnification at which box, with autoFitMarginCells either side, spans
// autoFitShare of the frame width, clamped to [autoFitMin, autoFitMax].
func autoFitFactor(box take.Box, layout Layout) float64 {
	span := float64((box.W + 2*autoFitMarginCells) * layout.CellWidth)
	return math.Min(math.Max(autoFitShare*float64(layout.Size.X)/span, autoFitMin), autoFitMax)
}

// easeInOut is the smootherstep ease over progress clamped to [0, 1]: speed and acceleration
// are both zero at either end, so a glide or a zoom gathers speed and settles rather than
// starting and stopping on a jolt.
func easeInOut(progress float64) float64 {
	p := clamp01(progress)
	return p * p * p * (p*(6*p-15) + 10)
}

// clamp01 clamps f to [0, 1].
func clamp01(f float64) float64 { return math.Min(math.Max(f, 0), 1) }
