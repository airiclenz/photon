package compose

import (
	"cmp"
	"errors"
	"fmt"
	"math"
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

// zoomPlan is one section's push into its target, on the output clock: the section's [Start,
// End), the ramps In and Out, the full Factor and the Centre the push aims at, in source pixels.
type zoomPlan struct {
	Start, End time.Duration
	In, Out    time.Duration
	Factor     float64
	Centre     pointF
}

// factorAt is the magnification at output time t: 1 outside the section, eased up over In from
// its start, held, and eased back to 1 over Out before its end. The ease runs on the factor's
// exponent, so each frame of a ramp scales by the same ratio at the same eased pace — a linear
// factor would rush the start of a push-in and crawl at its end.
func (z zoomPlan) factorAt(t time.Duration) float64 {
	if t < z.Start || t >= z.End {
		return 1
	}
	rise, fall := 1.0, 1.0
	if z.In > 0 {
		rise = easeInOut(float64(t-z.Start) / float64(z.In))
	}
	if z.Out > 0 {
		fall = easeInOut(float64(z.End-t) / float64(z.Out))
	}
	return math.Pow(z.Factor, min(rise, fall))
}

// zoomPlanner plans the zooms of one clip from its parts.
type zoomPlanner struct {
	board    *storyboard.Storyboard
	recorded *take.Take
	resolver target.Resolver
	layout   Layout
}

// plan plans every kept section's zoom: its target box, the factor given or auto-fit, and its
// ramps — zero means [storyboard.DefaultZoomRamp] — shrunk in proportion when together they
// outrun the section.
func (p zoomPlanner) plan(clipSchedule *schedule.Schedule) ([]zoomPlan, error) {
	sections := make(map[int]schedule.Section, len(clipSchedule.Sections))
	for _, section := range clipSchedule.Sections {
		sections[section.Beat] = section
	}
	var plans []zoomPlan
	for _, beat := range p.board.Beats {
		section, isScheduled := sections[beat.ID]
		if beat.Zoom == nil || !isScheduled || section.Cut {
			continue
		}
		box, err := p.zoomBox(beat, section)
		if err != nil {
			return nil, fmt.Errorf("compose: beat %d: zoom: %w", beat.ID, err)
		}
		plans = append(plans, p.planSection(beat.Zoom, section, box))
	}
	return plans, nil
}

// planSection is the zoom of one section onto box.
func (p zoomPlanner) planSection(zoom *storyboard.Zoom, section schedule.Section, box take.Box) zoomPlan {
	plan := zoomPlan{
		Start:  section.OutStart,
		End:    section.OutStart + section.Duration,
		In:     cmp.Or(zoom.In, storyboard.DefaultZoomRamp),
		Out:    cmp.Or(zoom.Out, storyboard.DefaultZoomRamp),
		Factor: zoom.Factor,
		Centre: p.layout.boxRect(box).centre(),
	}
	if plan.Factor == 0 {
		plan.Factor = autoFitFactor(box, p.layout)
	}
	if ramps := plan.In + plan.Out; ramps > section.Duration {
		share := float64(section.Duration) / float64(ramps)
		plan.In = time.Duration(float64(plan.In) * share)
		plan.Out = time.Duration(float64(plan.Out) * share)
	}
	return plan
}

// zoomBox finds the beat's zoom target: the box a click of the same beat resolved on the same
// target, as the take logged it, else the target resolved on the beat's snapshots — the latest
// one inside the beat's span that shows it, walking back to the snapshot in effect at the beat's
// start, so the push lands where the beat settles and a target gone by the beat's end still
// zooms.
func (p zoomPlanner) zoomBox(beat storyboard.Beat, section schedule.Section) (take.Box, error) {
	if box, isLogged := p.loggedBox(beat); isLogged {
		return box, nil
	}
	var lastErr error
	snapshots := p.recorded.Snapshots
	for index := len(snapshots) - 1; index >= 0; index-- {
		snapshot := snapshots[index]
		if snapshot.At >= section.End && section.End > section.Start {
			continue
		}
		box, err := p.resolver.Target(snapshot, beat.Zoom.Target)
		if err == nil {
			return box, nil
		}
		if !errors.Is(err, target.ErrNotOnScreen) {
			return take.Box{}, err
		}
		lastErr = err
		// The snapshot in effect at the beat's start is the earliest the beat shows.
		if snapshot.At <= section.Start {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("target /%s/: the take has no snapshot in the beat", beat.Zoom.Target.Text)
	}
	return take.Box{}, lastErr
}

// loggedBox is the box the take logged for a click of beat on the zoom's own target, if any.
func (p zoomPlanner) loggedBox(beat storyboard.Beat) (take.Box, bool) {
	for _, event := range p.recorded.Events {
		if event.Kind != take.EventTarget || event.Beat != beat.ID || event.Box == nil ||
			event.Action < 0 || event.Action >= len(beat.Do) {
			continue
		}
		if click := beat.Do[event.Action].Click; click != nil && click.Target == beat.Zoom.Target {
			return *event.Box, true
		}
	}
	return take.Box{}, false
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
