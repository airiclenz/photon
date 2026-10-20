package storyboard

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// transitionEditBoard is editBaseBoard — beat 1 of 1s, beat 2 of 1.5s — with five positions
// listed out of start order. By start they are [0] held over [100ms, 400ms), [2] over
// [600ms, 800ms), [1] over [1.2s, 1.7s) by beat 2, [3] a point at 2s by beat 2 and [4] held over
// [2.3s, 2.4s), so the transitions run into [2] from 400ms to 600ms, into [1] from 800ms to 1.2s,
// into [3] from 1.7s to 2s and into [4] from 2s to 2.3s.
const transitionEditBoard = editBaseBoard + `camera:
  positions:
    - {cell: {x: 1, y: 1}, at: 100ms, hold: 300ms}
    - cell: {x: 5, y: 2}
      at: {beat: 2, offset: 200ms} # past the result
      hold: 500ms
    - {cell: {x: 9, y: 3}, at: 600ms, hold: 200ms}
    - {cell: {x: 2, y: 0}, at: {beat: 2, offset: 1s}}
    - {cell: {x: 3, y: 0}, at: 2300ms, hold: 100ms}
`

// transitionEdit is a position.transition edit of the transition into the position at index,
// from startMS to endMS.
func transitionEdit(index int, startMS, endMS int64) Edit {
	return positionEdit(index, FieldPositionTransition,
		map[string]int64{"start_ms": startMS, "end_ms": endMS})
}

// textChange is one piece of a storyboard's text and what an edit turns it into.
type textChange struct {
	old, new string
}

func TestApply_Transition_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    Edit
		changes []textChange
	}{
		{"a transition between two held positions moves both", transitionEdit(1, 700, 1100), []textChange{
			{"at: 600ms, hold: 200ms}", "at: 600ms, hold: 100ms}"},
			{"at: {beat: 2, offset: 200ms} # past the result\n      hold: 500ms",
				"at: {beat: 2, offset: 100ms} # past the result\n      hold: 600ms"},
		}},
		{"a start edge moves a point and an end edge a held position", transitionEdit(4, 1900, 2350), []textChange{
			{"at: {beat: 2, offset: 1s}}", "at: {beat: 2, offset: 900ms}}"},
			{"at: 2300ms, hold: 100ms}", "at: 2350ms, hold: 50ms}"},
		}},
		{"an end edge moves a point keeping its beat", transitionEdit(3, 1600, 1900), []textChange{
			{"      hold: 500ms\n", "      hold: 400ms\n"},
			{"at: {beat: 2, offset: 1s}}", "at: {beat: 2, offset: 900ms}}"},
		}},
		{"a transition shrinks to a cut", transitionEdit(2, 500, 500), []textChange{
			{"at: 100ms, hold: 300ms}", "at: 100ms, hold: 400ms}"},
			{"at: 600ms, hold: 200ms}", "at: 500ms, hold: 300ms}"},
		}},
		{"a start edge on the earlier start makes it a point", transitionEdit(2, 100, 600), []textChange{
			{"at: 100ms, hold: 300ms}", "at: 100ms}"},
		}},
		{"an end edge left in place writes only the start edge", transitionEdit(1, 750, 1200), []textChange{
			{"at: 600ms, hold: 200ms}", "at: 600ms, hold: 150ms}"},
		}},
		{"the transition as it stands writes nothing", transitionEdit(1, 800, 1200), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := transitionEditBoard
			for _, change := range tc.changes {
				if count := strings.Count(want, change.old); count != 1 {
					t.Fatalf("the board holds %q %d times, want once", change.old, count)
				}
				want = strings.Replace(want, change.old, change.new, 1)
			}

			got, _, err := Apply([]byte(transitionEditBoard), cameraEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_Transition_MovesTheTwoPositionsOnTheClock(t *testing.T) {
	t.Parallel()

	_, board, err := Apply([]byte(transitionEditBoard), cameraEditPath, transitionEdit(1, 700, 1100))

	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	edges, hasTransition := currentTransition(board.Camera.Positions, 1)
	if want := (transition{start: 700 * time.Millisecond, end: 1100 * time.Millisecond}); !hasTransition || edges != want {
		t.Errorf("the transition into camera.positions[1] = %+v, %t, want %+v", edges, hasTransition, want)
	}
	if end := board.Camera.Positions[1].Start() + board.Camera.Positions[1].Hold; end != 1700*time.Millisecond {
		t.Errorf("camera.positions[1] ends at %s, want 1.7s", end)
	}
}

func TestApply_TransitionOutsideTheRules_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    Edit
		message string
	}{
		{"the first position by start", transitionEdit(0, 0, 100),
			"camera.positions[0]: position.transition: the position is the first by start"},
		{"a start after the end", transitionEdit(1, 1100, 900),
			"position.transition: want a start_ms no later than its end_ms"},
		{"a start before the earlier position starts", transitionEdit(1, 500, 1200),
			"position.transition: want a start_ms no earlier than camera.positions[2] starts, 600ms, got 500ms"},
		{"an end past the later position's end", transitionEdit(1, 800, 1800),
			"position.transition: want an end_ms no later than camera.positions[1] ends, 1700ms, got 1800ms"},
		{"an index past the last position", transitionEdit(5, 0, 100),
			"camera.positions[5]: position.transition: the storyboard has no camera position at index 5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(transitionEditBoard), cameraEditPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
			}
		})
	}
}

func TestApply_TransitionThatOverlaps_FailsValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    Edit
		message string
	}{
		{"a point moved into the position before it", transitionEdit(4, 1600, 2350),
			"camera.positions[3]: overlaps camera.positions[1]"},
		{"a cut after a point starts both together", transitionEdit(4, 2100, 2100),
			"overlaps camera.positions[3]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(transitionEditBoard), cameraEditPath, tc.edit)

			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("Apply(%+v) = %v, want a *ValidationError", tc.edit, err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %q, want it to contain %q", tc.edit, err, tc.message)
			}
		})
	}
}

func TestApply_TransitionWireSpellings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		value      any
		isAccepted bool
	}{
		{"a transition", map[string]int64{"start_ms": 750, "end_ms": 1150}, true},
		{"a transition without an end", map[string]int64{"start_ms": 750}, false},
		{"a transition without a start", map[string]int64{"end_ms": 1150}, false},
		{"a transition with another key", map[string]int64{"start_ms": 750, "end_ms": 1150, "hold_ms": 0}, false},
		{"a negative start", map[string]int64{"start_ms": -1, "end_ms": 1150}, false},
		{"a fractional end", map[string]float64{"start_ms": 750, "end_ms": 1150.5}, false},
		{"edges as strings", map[string]string{"start_ms": "750", "end_ms": "1150"}, false},
		{"a pair of numbers", []int{750, 1150}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			edit := positionEdit(1, FieldPositionTransition, tc.value)

			_, _, err := Apply([]byte(transitionEditBoard), cameraEditPath, edit)

			if tc.isAccepted && err != nil {
				t.Fatalf("Apply(%+v): %v", edit, err)
			}
			if !tc.isAccepted && (err == nil || !strings.Contains(err.Error(), FieldPositionTransition+": want a transition such as")) {
				t.Errorf("Apply(%+v) = %v, want a spelling error naming %s", edit, err, FieldPositionTransition)
			}
		})
	}
}
