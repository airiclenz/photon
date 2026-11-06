package engine

import (
	"context"
	"fmt"
	"math"

	"github.com/airiclenz/photon/internal/keys"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// The Text of a drag's target events, naming which point of the drag the Box is.
const (
	dragFromText = "from"
	dragViaText  = "via"
	dragToText   = "to"
)

// dragMove is one cell of a drag's path, the 1-based step it lands on, counted across all of the
// drag's segments, and whether it is the move that reaches a via point.
type dragMove struct {
	step  int
	cell  take.Point
	isVia bool
}

// drag resolves every point of the drag on the screen as it stands at the press — the From
// target, each via point and the to end, an offset counting from the point before it — clamps
// each to the screen and logs each resolved target's box, then sends the press with the drag's
// button and modifiers, the eased moves through every via point and the release,
// logging each cell before it is sent. A point that does not resolve fails the drag before
// anything is sent.
func (e *Engine) drag(ctx context.Context, step take.Event, action storyboard.DragAction) error {
	mouse, err := action.Mouse()
	if err != nil {
		return fmt.Errorf("drag: %w", err)
	}
	screen := e.source.Current()
	fromBox, err := e.resolver.Target(screen, action.From)
	if err != nil {
		return fmt.Errorf("drag: from: %w", err)
	}
	e.logTarget(step, dragFromText, fromBox)
	points := make([]take.Point, 0, len(action.Via)+2)
	points = append(points, clampToScreen(centreOf(fromBox), screen))

	for index, via := range action.Via {
		point, err := e.dragPoint(screen, step, points[len(points)-1], via, dragViaText)
		if err != nil {
			return fmt.Errorf("drag: via[%d]: %w", index, err)
		}
		points = append(points, clampToScreen(point, screen))
	}
	to, err := e.dragPoint(screen, step, points[len(points)-1], action.To, dragToText)
	if err != nil {
		return fmt.Errorf("drag: to: %w", err)
	}
	points = append(points, clampToScreen(to, screen))
	return e.sendDrag(ctx, step, points, mouse, action.StepCount())
}

// dragPoint is the cell a drag point resolves to, before clamping: previous, the point before it,
// moved by its Offset, or the centre of its target resolved on screen, whose box it logs with
// text naming the point.
func (e *Engine) dragPoint(
	screen take.Snapshot,
	step take.Event,
	previous take.Point,
	point storyboard.DragEnd,
	text string,
) (take.Point, error) {
	if point.Offset != nil {
		return take.Point{X: previous.X + point.Offset.X, Y: previous.Y + point.Offset.Y}, nil
	}
	box, err := e.resolver.Target(screen, point.Target)
	if err != nil {
		return take.Point{}, err
	}
	e.logTarget(step, text, box)
	return centreOf(box), nil
}

// sendDrag presses mouse's button, its modifiers held, at the first of points, then moves it
// along each segment between one point and the next in turn — steps steps per segment, each
// dragStepGap apart, on the path [dragRoute] eases — and dragStepGap after the last step releases
// it at the last point. A step whose cell repeats the last one sends nothing but still waits its
// gap, so the timing stays even and the ease reads in the motion. The move that reaches a via
// point is logged with the Text [take.MoveVia]. points holds at least two cells.
func (e *Engine) sendDrag(
	ctx context.Context,
	step take.Event,
	points []take.Point,
	mouse keys.Mouse,
	steps int,
) error {
	from, to := points[0], points[len(points)-1]
	e.logCell(take.EventPress, step, from)
	press := PointerEvent{Kind: keys.PointerPress, X: from.X, Y: from.Y, Mouse: mouse}
	if err := e.source.Pointer(press); err != nil {
		return err
	}

	moves := dragRoute(points, steps)
	for index := 1; index <= steps*(len(points)-1); index++ {
		if err := sleepContext(ctx, dragStepGap); err != nil {
			return err
		}
		if len(moves) == 0 || moves[0].step != index {
			continue
		}
		move := moves[0]
		moves = moves[1:]
		moved := step
		if move.isVia {
			moved.Text = take.MoveVia
		}
		e.logCell(take.EventMove, moved, move.cell)
		motion := PointerEvent{Kind: keys.PointerMove, X: move.cell.X, Y: move.cell.Y, Mouse: mouse}
		if err := e.source.Pointer(motion); err != nil {
			return err
		}
	}

	if err := sleepContext(ctx, dragStepGap); err != nil {
		return err
	}
	e.logCell(take.EventRelease, step, to)
	release := PointerEvent{Kind: keys.PointerRelease, X: to.X, Y: to.Y, Mouse: mouse}
	return e.source.Pointer(release)
}

// logTarget logs a drag's target event: the resolved box, with text naming the end it is.
func (e *Engine) logTarget(step take.Event, text string, box take.Box) {
	resolved := step
	resolved.Text = text
	resolved.Box = &box
	e.logStep(take.EventTarget, resolved)
}

// logCell logs an event of kind carrying the cell it is sent to.
func (e *Engine) logCell(kind take.EventKind, step take.Event, cell take.Point) {
	sent := step
	sent.Cell = &cell
	e.logStep(kind, sent)
}

// dragRoute is the moves of a drag through points, steps steps per segment between one point and
// the next: each segment's [dragPath], its steps numbered on from the segments before it, and the
// last move of every segment but the final one marked as reaching a via point. A segment that
// moves no cell adds no move, so it marks no via point.
func dragRoute(points []take.Point, steps int) []dragMove {
	segments := len(points) - 1
	moves := make([]dragMove, 0, steps*segments)
	for segment := range segments {
		path := dragPath(points[segment], points[segment+1], steps)
		for _, move := range path {
			move.step += segment * steps
			moves = append(moves, move)
		}
		if len(path) > 0 && segment < segments-1 {
			moves[len(moves)-1].isVia = true
		}
	}
	return moves
}

// dragPath is the cells a drag from one cell to another moves onto in steps steps: step i of n
// lands dragEase(i/n) of the way along the straight line, rounded to a cell. A step that lands
// on the cell before it (from, for the first) is dropped, since a terminal reports motion only
// when the pointer changes cell, so from == to yields no moves and the last move, if any, is to.
func dragPath(from, to take.Point, steps int) []dragMove {
	moves := make([]dragMove, 0, steps)
	last := from
	for index := 1; index <= steps; index++ {
		progress := dragEase(float64(index) / float64(steps))
		cell := take.Point{
			X: from.X + int(math.Round(progress*float64(to.X-from.X))),
			Y: from.Y + int(math.Round(progress*float64(to.Y-from.Y))),
		}
		if cell == last {
			continue
		}
		moves = append(moves, dragMove{step: index, cell: cell})
		last = cell
	}
	return moves
}

// dragEase is smootherstep, the curve compose's drawn cursor eases with: speed and acceleration
// are zero at both ends, so a drag gathers speed and settles rather than jerking into motion.
// progress must lie in [0, 1]; dragEase(1) is exactly 1.
func dragEase(progress float64) float64 {
	return progress * progress * progress * (progress*(6*progress-15) + 10)
}

// clampToScreen moves cell onto the screen's grid, the snapshot's [take.Snapshot.Bounds]: its
// column into the bounds' columns and its row into their rows. An empty grid clamps every cell to
// the origin.
func clampToScreen(cell take.Point, screen take.Snapshot) take.Point {
	bounds := screen.Bounds()
	if bounds.W == 0 || bounds.H == 0 {
		return take.Point{}
	}
	return take.Point{
		X: clampInt(cell.X, bounds.X, bounds.X+bounds.W-1),
		Y: clampInt(cell.Y, bounds.Y, bounds.Y+bounds.H-1),
	}
}

// clampInt is value limited to [low, high].
func clampInt(value, low, high int) int { return min(max(value, low), high) }
