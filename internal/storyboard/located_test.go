package storyboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestLocatedError_Error_PrefixesTheOwner(t *testing.T) {
	t.Parallel()
	problem := errors.New("text: missing")

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"beat", AtBeat(3, problem), "beat 3: text: missing"},
		{"position", AtPosition(1, problem), "camera.positions[1]: text: missing"},
		{"caption", AtCaption(0, problem), "captions[0]: text: missing"},
		{"clip", &LocatedError{Owner: Owner{Kind: OwnerClip}, Err: problem}, "text: missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.err.Error()

			if got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestOwnerOf(t *testing.T) {
	t.Parallel()
	problem := errors.New("over: want >0, got 0s")

	cases := []struct {
		name string
		err  error
		want Owner
	}{
		{"plain error", problem, Owner{Kind: OwnerClip}},
		{"located", AtBeat(2, problem), Owner{Kind: OwnerBeat, Index: 2}},
		{"wrapped by a caller", fmt.Errorf("compose: %w", AtPosition(4, problem)), Owner{Kind: OwnerPosition, Index: 4}},
		{"outermost of two", AtCaption(1, AtBeat(2, problem)), Owner{Kind: OwnerCaption, Index: 1}},
		{"leaf of a joined error", errors.Join(problem, AtCaption(5, problem)), Owner{Kind: OwnerCaption, Index: 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := OwnerOf(tc.err)

			if got != tc.want {
				t.Errorf("want %+v, got %+v", tc.want, got)
			}
		})
	}
}

func TestAtPosition_ErrorsIs_ReachesAWrappedSentinel(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("not on screen")

	err := AtPosition(0, fmt.Errorf("target: %w", sentinel))

	if !errors.Is(err, sentinel) {
		t.Errorf("want errors.Is to reach the sentinel through %v", err)
	}
}

func TestOwner_JSON_SpellsKindAndIndex(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(Owner{Kind: OwnerPosition, Index: 2})

	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := `{"kind":"position","index":2}`; string(encoded) != want {
		t.Errorf("want %s, got %s", want, encoded)
	}
}
