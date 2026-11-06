package tune

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/storyboard"
)

// typedValue is value as an Edit's JSON value; nil is the JSON null.
func typedValue(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode the test value %v: %v", value, err))
	}
	return encoded
}

// beatEdit is an Edit of field on the beat with id, its value typed as value encodes.
func beatEdit(id int, field string, value any) Edit {
	owner := storyboard.Owner{Kind: storyboard.OwnerBeat, Index: id}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

// frameEdit is an Edit of the clip's frame field, its value typed as value encodes.
func frameEdit(field string, value any) Edit {
	return Edit{Owner: storyboard.Owner{Kind: storyboard.OwnerClip}, Field: field, Value: typedValue(value)}
}

// captionEdit is an Edit of the caption at index, its value typed as value encodes.
func captionEdit(index int, field string, value any) Edit {
	owner := storyboard.Owner{Kind: storyboard.OwnerCaption, Index: index}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

// tooWideCaptionEdit is an Edit setting the caption at index to [tooWideText], which leaves the
// storyboard valid but its pipeline unbuilt.
func tooWideCaptionEdit(index int) Edit {
	return captionEdit(index, storyboard.FieldCaptionText, tooWideText)
}

func TestSession_CaptionEdit_SwapsTheEndAndUndoes(t *testing.T) {
	t.Parallel()
	f := newSessionFixtureOf(t, captionsStoryboard)

	f.apply(t, captionEdit(1, storyboard.FieldCaptionFor, 1000))
	edited := f.session.State()
	undoErr := f.session.Undo()
	undone := f.session.State()

	if caption := edited.Captions[1]; caption.ForMS != 1000 || caption.Until != nil || caption.StopMS != 2000 || !edited.IsDirty {
		t.Errorf("after the edit: captions[1] %+v, dirty %v; want for 1s, no until, a stop at 2s, dirty", caption, edited.IsDirty)
	}
	if undoErr != nil {
		t.Fatalf("Undo: %v", undoErr)
	}
	if caption := undone.Captions[1]; caption.ForMS != 0 || caption.Until == nil || undone.IsDirty {
		t.Errorf("after the undo: captions[1] %+v, dirty %v; want the until back, the bytes on disk", caption, undone.IsDirty)
	}
}

func TestSession_APageTake_RefusesAReflowEdit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		field, key string
		value      any
	}{
		{storyboard.FieldFrameFontSize, "font_size", 12},
		{storyboard.FieldFrameLineHeight, "line_height", 1.2},
		{storyboard.FieldFrameCols, "cols", 20},
		{storyboard.FieldFrameRows, "rows", 4},
		{storyboard.FieldFramePadding, "padding", 4},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			t.Parallel()
			f := newPageSessionFixture(t, pageSessionStoryboard)

			err := f.session.Apply(frameEdit(test.field, test.value))

			want := "a frame with a viewport takes no " + test.key
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("Apply(%s: %v): got %v, want a refusal naming %q", test.field, test.value, err, want)
			}
			if state := f.session.State(); state.IsDirty || state.CanUndo {
				t.Errorf("after the refusal: dirty %v, can undo %v; want neither", state.IsDirty, state.CanUndo)
			}
		})
	}
}

// A viewport frame refuses a terminal key only when it is set: a padding of 0 is the key left
// unset, so the edit stands.
func TestSession_APageTake_TakesAZeroPadding(t *testing.T) {
	t.Parallel()
	f := newPageSessionFixture(t, pageSessionStoryboard)

	err := f.session.Apply(frameEdit(storyboard.FieldFramePadding, 0))

	if err != nil {
		t.Fatalf("Apply(frame.padding: 0): %v", err)
	}
	if state := f.session.State(); len(state.Errors) != 0 {
		t.Errorf("errors after the zero padding: %+v", state.Errors)
	}
}

func TestSession_APageTake_TakesMaxColorsAndScaleEdits(t *testing.T) {
	t.Parallel()
	f := newPageSessionFixture(t, pageSessionStoryboard)

	f.apply(t, frameEdit(storyboard.FieldFrameMaxColors, 8))
	f.apply(t, frameEdit(storyboard.FieldFrameScale, 3))

	state := f.session.State()
	if state.MaxColors != 8 || state.Scale != 3 || len(state.Errors) != 0 {
		t.Errorf("max_colors %d, scale %d, errors %+v; want 8, 3, none", state.MaxColors, state.Scale, state.Errors)
	}
	if _, err := f.session.Frame(0); err != nil {
		t.Errorf("Frame(0) after the edits: %v", err)
	}
}
