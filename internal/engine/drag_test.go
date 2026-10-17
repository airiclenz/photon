package engine

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/keys"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// stepsOf is an explicit drag steps value.
func stepsOf(steps int) *int { return &steps }

// dragEvent is a drag event of kind on beat 7's only action, with text and the box or cell it
// carries.
func dragEvent(kind take.EventKind, text string, box *take.Box, cell *take.Point) take.Event {
	return take.Event{Kind: kind, Beat: 7, Form: formDrag, Text: text, Box: box, Cell: cell}
}

func TestEngine_Run_Drag(t *testing.T) {
	t.Parallel()
	alphaBox := take.Box{X: 2, Y: 1, W: 5, H: 1}
	betaBox := take.Box{X: 2, Y: 2, W: 4, H: 1}
	alphaCentre, betaCentre := take.Point{X: 4, Y: 1}, take.Point{X: 3, Y: 2}
	rightEdge := take.Point{X: fakeCols - 1, Y: 1}

	tests := []struct {
		name       string
		action     storyboard.DragAction
		wantSent   []string
		wantEvents []take.Event
	}{
		{
			name: "to a target presses, moves and releases at its centre",
			action: storyboard.DragAction{
				From:  storyboard.Target{Text: "alpha"},
				To:    storyboard.DragEnd{Target: storyboard.Target{Text: "beta"}},
				Steps: stepsOf(2),
			},
			wantSent: []string{
				string(keys.Click(4, 1)), string(keys.Motion(3, 2)), string(keys.Release(3, 2)),
			},
			wantEvents: stepEvents(formDrag, "",
				dragEvent(take.EventTarget, dragFromText, &alphaBox, nil),
				dragEvent(take.EventTarget, dragToText, &betaBox, nil),
				dragEvent(take.EventPress, "", nil, &alphaCentre),
				dragEvent(take.EventMove, "", nil, &betaCentre),
				dragEvent(take.EventRelease, "", nil, &betaCentre),
			),
		},
		{
			name: "an offset past the right edge clamps to the last column",
			action: storyboard.DragAction{
				From:  storyboard.Target{Text: "alpha"},
				To:    storyboard.DragEnd{Offset: &storyboard.Offset{X: 100}},
				Steps: stepsOf(1),
			},
			wantSent: []string{
				string(keys.Click(4, 1)),
				string(keys.Motion(fakeCols-1, 1)),
				string(keys.Release(fakeCols-1, 1)),
			},
			wantEvents: stepEvents(formDrag, "",
				dragEvent(take.EventTarget, dragFromText, &alphaBox, nil),
				dragEvent(take.EventPress, "", nil, &alphaCentre),
				dragEvent(take.EventMove, "", nil, &rightEdge),
				dragEvent(take.EventRelease, "", nil, &rightEdge),
			),
		},
		{
			name: "a zero offset presses and releases on one cell with no move",
			action: storyboard.DragAction{
				From:  storyboard.Target{Text: "alpha"},
				To:    storyboard.DragEnd{Offset: &storyboard.Offset{}},
				Steps: stepsOf(3),
			},
			wantSent: []string{string(keys.Click(4, 1)), string(keys.Release(4, 1))},
			wantEvents: stepEvents(formDrag, "",
				dragEvent(take.EventTarget, dragFromText, &alphaBox, nil),
				dragEvent(take.EventPress, "", nil, &alphaCentre),
				dragEvent(take.EventRelease, "", nil, &alphaCentre),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			term := newFakeTerminal(fakeScreen...)
			action := storyboard.Action{Drag: &tt.action}

			err := New(term, target.Resolver{}).Run(context.Background(), []storyboard.Beat{beatOf(action)})

			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := term.sentChunks(); !slices.Equal(got, tt.wantSent) {
				t.Errorf("sent %q, want %q", got, tt.wantSent)
			}
			if got := term.loggedEvents(); !reflect.DeepEqual(got, tt.wantEvents) {
				t.Errorf("events:\n got %+v\nwant %+v", got, tt.wantEvents)
			}
		})
	}
}

func TestEngine_Run_DragFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		action     storyboard.DragAction
		wantMsg    string
		wantEvents int
	}{
		{
			name: "missing from target",
			action: storyboard.DragAction{
				From: storyboard.Target{Text: "delta"},
				To:   storyboard.DragEnd{Target: storyboard.Target{Text: "beta"}},
			},
			wantMsg:    "drag: from: target /delta/",
			wantEvents: 2,
		},
		{
			name: "missing to target",
			action: storyboard.DragAction{
				From: storyboard.Target{Text: "alpha"},
				To:   storyboard.DragEnd{Target: storyboard.Target{Text: "delta"}},
			},
			wantMsg:    "drag: to: target /delta/",
			wantEvents: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			term := newFakeTerminal(fakeScreen...)
			action := storyboard.Action{Drag: &tt.action}

			err := New(term, target.Resolver{}).Run(context.Background(), []storyboard.Beat{beatOf(action)})

			if !errors.Is(err, target.ErrNotOnScreen) {
				t.Errorf("Run = %v, want it to wrap %v", err, target.ErrNotOnScreen)
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %v does not say %q", err, tt.wantMsg)
			}
			if sent := term.sentChunks(); len(sent) != 0 {
				t.Errorf("sent %q after the failure, want nothing", sent)
			}
			if events := term.loggedEvents(); len(events) != tt.wantEvents {
				t.Errorf("logged %d events %+v, want %d", len(events), events, tt.wantEvents)
			}
		})
	}
}

func TestDragPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		from, to take.Point
		steps    int
		want     []dragMove
	}{
		{
			name: "straight horizontal",
			from: take.Point{}, to: take.Point{X: 10}, steps: 4,
			want: []dragMove{
				{step: 1, cell: take.Point{X: 1}},
				{step: 2, cell: take.Point{X: 5}},
				{step: 3, cell: take.Point{X: 9}},
				{step: 4, cell: take.Point{X: 10}},
			},
		},
		{
			name: "diagonal",
			from: take.Point{X: 2, Y: 1}, to: take.Point{X: 10, Y: 5}, steps: 2,
			want: []dragMove{
				{step: 1, cell: take.Point{X: 6, Y: 3}},
				{step: 2, cell: take.Point{X: 10, Y: 5}},
			},
		},
		{
			name: "leftward and up",
			from: take.Point{X: 10, Y: 5}, to: take.Point{X: 2, Y: 1}, steps: 2,
			want: []dragMove{
				{step: 1, cell: take.Point{X: 6, Y: 3}},
				{step: 2, cell: take.Point{X: 2, Y: 1}},
			},
		},
		{
			name: "from equals to has no moves",
			from: take.Point{X: 5, Y: 5}, to: take.Point{X: 5, Y: 5}, steps: 12,
			want: []dragMove{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := dragPath(tt.from, tt.to, tt.steps)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dragPath(%v, %v, %d) = %+v, want %+v", tt.from, tt.to, tt.steps, got, tt.want)
			}
		})
	}
}

func TestDragPath_MoreStepsThanCells_NoDuplicatesAndEndsAtTo(t *testing.T) {
	t.Parallel()
	from, to := take.Point{X: 1, Y: 1}, take.Point{X: 4, Y: 1}

	moves := dragPath(from, to, 12)

	if len(moves) == 0 || moves[len(moves)-1].cell != to {
		t.Fatalf("dragPath = %+v, want its last cell %v", moves, to)
	}
	previous := from
	for _, move := range moves {
		if move.cell == previous {
			t.Errorf("dragPath = %+v repeats cell %v", moves, previous)
		}
		previous = move.cell
	}
}

func TestDragPath_Eased_EndGapsNoWiderThanMiddle(t *testing.T) {
	t.Parallel()
	from, to, steps := take.Point{}, take.Point{X: 120}, 6

	moves := dragPath(from, to, steps)

	if len(moves) != steps {
		t.Fatalf("dragPath = %+v, want one move per step", moves)
	}
	gaps := make([]int, 0, steps)
	previous := from
	for _, move := range moves {
		gaps = append(gaps, move.cell.X-previous.X)
		previous = move.cell
	}
	middle := gaps[steps/2]
	if gaps[0] > middle || gaps[steps-1] > middle {
		t.Errorf("gaps %v: the first and last exceed the middle %d", gaps, middle)
	}
}
