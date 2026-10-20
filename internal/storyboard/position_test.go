package storyboard

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// positionsBase is a camera that breaks no rule on validBase's 5s clip, whose beat 2 starts at 3s:
// cells, text targets, held bars and points, positions listed out of order and meeting with zero
// gaps — [0s, 1s), [1s, 3s), [3s, 3.5s), then a point at 3.5s — and two zooms. Each row of
// TestCameraValidate_Rejects swaps one fragment of it, which must occur in it exactly once.
const positionsBase = `positions:
  - {cell: {x: 40, y: 12}, at: 3s, hold: 500ms, ease: ease-in}
  - {text: 'Grip', area: list, at: 0s, hold: 1s}
  - {cell: {x: 2, y: 3}, at: {beat: 2, offset: 500ms}, ease: ease-out}
  - {text: 'Item', nth: 2, at: 1s, hold: 2s, ease: linear}
zooms:
  - {to: 2, at: {beat: 2, offset: 100ms}, over: 500ms, ease: smooth}
  - {to: 1.5, at: 1s, over: 1s}
`

// decodeCamera decodes source strictly, as [Parse] decodes a storyboard.
func decodeCamera(source string) (Camera, error) {
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(source)))
	decoder.KnownFields(true)
	var camera Camera
	err := decoder.Decode(&camera)
	return camera, err
}

// validateOnBase validates camera against validBase's frame and output clock and returns every
// problem it files.
func validateOnBase(t *testing.T, camera Camera) []error {
	t.Helper()
	board, err := Parse([]byte(validBase), "base.yaml")
	if err != nil {
		t.Fatalf("the base storyboard must parse clean: %v", err)
	}
	var problems []error
	camera.validate(board.Frame, board.Clock(), func(problem error) {
		problems = append(problems, problem)
	})
	return problems
}

func TestCameraValidate_AcceptsAValidCameraAndResolvesStarts(t *testing.T) {
	t.Parallel()
	camera, err := decodeCamera(positionsBase)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	problems := validateOnBase(t, camera)

	if len(problems) != 0 {
		t.Fatalf("want no problems, got %v", errors.Join(problems...))
	}
	wantPositions := []time.Duration{3 * time.Second, 0, 3500 * time.Millisecond, time.Second}
	for index, want := range wantPositions {
		if got := camera.Positions[index].Start(); got != want {
			t.Errorf("positions[%d].Start(): want %s, got %s", index, want, got)
		}
	}
	wantZooms := []time.Duration{3100 * time.Millisecond, time.Second}
	for index, want := range wantZooms {
		if got := camera.Zooms[index].Start(); got != want {
			t.Errorf("zooms[%d].Start(): want %s, got %s", index, want, got)
		}
	}
	if got := camera.Positions[1].Point.Target; got != (Target{Text: "Grip", Area: "list"}) {
		t.Errorf("positions[1] target: want its inline text and area, got %+v", got)
	}
	if got := camera.Positions[0].Cell; got == nil || *got != (Cell{X: 40, Y: 12}) {
		t.Errorf("positions[0] cell: want x 40, y 12, got %+v", got)
	}
}

func TestCameraValidate_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		old, new string
		want     string
		// decode marks a rule the strict decode enforces, which fails before validation runs.
		decode bool
	}{
		{"unknown position field", "ease: ease-in}", "ease: ease-in, speed: 2}", "field speed not found", true},
		{"pan is not a position field", "hold: 1s}", "hold: 1s, pan: {}}", "field pan not found", true},
		{"text and cell", "{cell: {x: 40, y: 12}, at: 3s", "{cell: {x: 40, y: 12}, text: 'A', at: 3s",
			"camera.positions[0]: want exactly one of text or cell", false},
		{"neither text nor cell", "{text: 'Grip', area: list, at: 0s", "{at: 0s",
			"camera.positions[1]: want exactly one of text or cell", false},
		{"cell with an nth", "{x: 40, y: 12},", "{x: 40, y: 12}, nth: 2,",
			"camera.positions[0]: nth: only a text point takes an nth", false},
		{"cell with an area", "{x: 40, y: 12},", "{x: 40, y: 12}, area: list,",
			"camera.positions[0]: area: only a text point takes an area", false},
		{"cell past the last column", "{x: 40, y: 12}", "{x: 80, y: 12}",
			"camera.positions[0]: cell: want x in [0, 80) and y in [0, 24), got x 80, y 12", false},
		{"cell past the last row", "{x: 40, y: 12}", "{x: 40, y: 24}",
			"camera.positions[0]: cell: want x in [0, 80) and y in [0, 24), got x 40, y 24", false},
		{"text regex", "text: 'Grip'", "text: '('", "camera.positions[1]: text: error parsing regexp", false},
		{"text area", "area: list", "area: 'a b'",
			`camera.positions[1]: area: want an area name or any, got "a b"`, false},
		{"hold below zero", "hold: 500ms", "hold: -1s", "camera.positions[0]: hold: want ≥0, got -1s", false},
		{"start at an unknown beat", "beat: 2, offset: 500ms", "beat: 9, offset: 500ms",
			"camera.positions[2]: at: beat 9 is not in the storyboard", false},
		{"start at the clip's end", "at: 3s", "at: 5s",
			"camera.positions[0]: at: want a start in [0s, 5s), got 5s", false},
		{"start before the clip", "at: 3s", "at: -1s",
			"camera.positions[0]: at: want a start in [0s, 5s), got -1s", false},
		{"end past the clip", "hold: 500ms", "hold: 3s",
			"camera.positions[0]: hold: want the position to end by the clip's end 5s, got an end at 6s", false},
		{"ease unknown", "ease: ease-in", "ease: bounce",
			`camera.positions[0]: ease: want smooth, linear, ease-in or ease-out, got "bounce"`, false},
		{"overlap, filed under the later by start", "at: 1s, hold: 2s", "at: 1s, hold: 2100ms",
			"camera.positions[0]: overlaps camera.positions[3] ([1s, 3.1s) and [3s, 3.5s))", false},
		{"equal starts overlap", "offset: 500ms", "offset: 0s",
			"camera.positions[2]: overlaps camera.positions[0] ([3s, 3.5s) and [3s, 3s))", false},
		{"zoom factor too large", "to: 2,", "to: 3.5,", "camera.zooms[0]: to: want in [1, 3], got 3.5", false},
		{"zoom length zero", "over: 500ms", "over: 0s", "camera.zooms[0]: over: want >0, got 0s", false},
		{"zoom ease unknown", "ease: smooth", "ease: bounce",
			`camera.zooms[0]: ease: want smooth, linear, ease-in or ease-out, got "bounce"`, false},
		{"zoom start at an unknown beat", "beat: 2, offset: 100ms", "beat: 9, offset: 100ms",
			"camera.zooms[0]: at: beat 9 is not in the storyboard", false},
		{"zoom start at the clip's end", "at: 1s, over: 1s", "at: 5s, over: 1s",
			"camera.zooms[1]: at: want a start in [0s, 5s), got 5s", false},
		{"zoom end past the clip", "at: 1s, over: 1s", "at: 4s, over: 2s",
			"camera.zooms[1]: over: want the move to end by the clip's end 5s, got an end at 6s", false},
		{"two zooms overlap", "at: 1s, over: 1s", "at: 3s, over: 1s",
			"camera.zooms[0]: overlaps camera.zooms[1] ([3s, 4s) and [3.1s, 3.6s))", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(positionsBase, tc.old); count != 1 {
				t.Fatalf("fragment %q occurs %d times in the base, want once", tc.old, count)
			}

			camera, err := decodeCamera(strings.Replace(positionsBase, tc.old, tc.new, 1))

			if tc.decode {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want a decode error containing %q, got %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			problems := validateOnBase(t, camera)
			if len(problems) != 1 {
				t.Fatalf("problems: want exactly the one broken rule, got %d:\n%v",
					len(problems), errors.Join(problems...))
			}
			if got := problems[0].Error(); !strings.Contains(got, tc.want) {
				t.Fatalf("want an error containing %q, got %q", tc.want, got)
			}
			if got := OwnerOf(problems[0]); !strings.HasPrefix(tc.want, ownerPrefix(got)) {
				t.Errorf("owner: want the one %q names, got %+v", tc.want, got)
			}
		})
	}
}

// ownerPrefix spells a position's or zoom's owner as its problems start, and nothing else's.
func ownerPrefix(owner Owner) string {
	switch owner.Kind {
	case OwnerPosition:
		return fmt.Sprintf("camera.positions[%d]: ", owner.Index)
	case OwnerZoom:
		return fmt.Sprintf("camera.zooms[%d]: ", owner.Index)
	default:
		return fmt.Sprintf("unexpected owner %+v", owner)
	}
}

func TestAtPositionAndAtZoom_FileUnderTheirEntry(t *testing.T) {
	t.Parallel()
	problem := errors.New("hold: missing")
	cases := []struct {
		name  string
		err   error
		want  string
		owner Owner
	}{
		{"position", AtPosition(2, problem), "camera.positions[2]: hold: missing", Owner{Kind: OwnerPosition, Index: 2}},
		{"zoom", AtZoom(1, problem), "camera.zooms[1]: hold: missing", Owner{Kind: OwnerZoom, Index: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error(): want %q, got %q", tc.want, got)
			}
			if got := OwnerOf(tc.err); got != tc.owner {
				t.Errorf("OwnerOf: want %+v, got %+v", tc.owner, got)
			}
			if !errors.Is(tc.err, problem) {
				t.Errorf("want errors.Is to reach the wrapped problem through %v", tc.err)
			}
		})
	}
}
