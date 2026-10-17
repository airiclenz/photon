package engine

import (
	"context"
	"fmt"
	"math"

	"github.com/airiclenz/photon/internal/keys"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// The Text of a drag's two target events, naming which end of the drag the Box is.
const (
	dragFromText = "from"
	dragToText   = "to"
)

// dragMove is one cell of a drag's path and the 1-based step it lands on.
type dragMove struct {
	step int
	cell take.Point
}

// drag resolves the From target on the screen as it stands and logs its box, works out the end
// cell — a resolved To target's centre (its box logged too) or the press cell moved by the
// Offset — clamps both cells to the screen, then sends the press, the eased motion reports and
// the release, logging each cell before it is sent. A target that does not resolve fails the
// drag before anything is sent.
func (e *Engine) drag(ctx context.Context, step take.Event, action storyboard.DragAction) error {
	screen := e.term.Current()
	fromBox, err := e.resolver.Target(screen, action.From)
	if err != nil {
		return fmt.Errorf("drag: from: %w", err)
	}
	e.logTarget(step, dragFromText, fromBox)
	from := clampToScreen(centreOf(fromBox), screen)

	to, err := e.dragEnd(screen, step, from, action.To)
	if err != nil {
		return err
	}
	return e.sendDrag(ctx, step, from, clampToScreen(to, screen), action.StepCount())
}

// dragEnd is the cell a drag pressed at from releases at, before clamping: from moved by the
// end's Offset, or the centre of its target resolved on screen, whose box it logs.
func (e *Engine) dragEnd(
	screen take.Snapshot,
	step take.Event,
	from take.Point,
	end storyboard.DragEnd,
) (take.Point, error) {
	if end.Offset != nil {
		return take.Point{X: from.X + end.Offset.X, Y: from.Y + end.Offset.Y}, nil
	}
	toBox, err := e.resolver.Target(screen, end.Target)
	if err != nil {
		return take.Point{}, fmt.Errorf("drag: to: %w", err)
	}
	e.logTarget(step, dragToText, toBox)
	return centreOf(toBox), nil
}

// sendDrag presses the left button at from, then over steps steps each dragStepGap apart moves
// it along the eased path to to, and dragStepGap after the last step releases it at to. A step
// whose cell repeats the last one sends nothing but still waits its gap, so the timing stays
// even and the ease reads in the motion.
func (e *Engine) sendDrag(ctx context.Context, step take.Event, from, to take.Point, steps int) error {
	e.logCell(take.EventPress, step, from)
	if err := e.term.Send(keys.Click(from.X, from.Y)); err != nil {
		return err
	}

	moves := dragPath(from, to, steps)
	for index := 1; index <= steps; index++ {
		if err := sleepContext(ctx, dragStepGap); err != nil {
			return err
		}
		if len(moves) == 0 || moves[0].step != index {
			continue
		}
		cell := moves[0].cell
		moves = moves[1:]
		e.logCell(take.EventMove, step, cell)
		if err := e.term.Send(keys.Motion(cell.X, cell.Y)); err != nil {
			return err
		}
	}

	if err := sleepContext(ctx, dragStepGap); err != nil {
		return err
	}
	e.logCell(take.EventRelease, step, to)
	return e.term.Send(keys.Release(to.X, to.Y))
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

// clampToScreen moves cell onto the screen's grid: its column into [0, cols-1] and its row into
// [0, rows-1], where rows is the snapshot's row count and cols its first row's width.
func clampToScreen(cell take.Point, screen take.Snapshot) take.Point {
	rows := len(screen.Cells)
	if rows == 0 || len(screen.Cells[0]) == 0 {
		return take.Point{}
	}
	cols := len(screen.Cells[0])
	return take.Point{X: clampInt(cell.X, 0, cols-1), Y: clampInt(cell.Y, 0, rows-1)}
}

// clampInt is value limited to [low, high].
func clampInt(value, low, high int) int { return min(max(value, low), high) }
