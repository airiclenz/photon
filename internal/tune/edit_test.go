package tune

import (
	"encoding/json"
	"fmt"
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

// cameraEdit is an Edit of the camera move at index, its value typed as value encodes.
func cameraEdit(index int, field string, value any) Edit {
	owner := storyboard.Owner{Kind: storyboard.OwnerCamera, Index: index}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

// captionEdit is an Edit of the caption at index, its value typed as value encodes.
func captionEdit(index int, field string, value any) Edit {
	owner := storyboard.Owner{Kind: storyboard.OwnerCaption, Index: index}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

func TestSession_CaptionEdit_SwapsTheEndAndUndoes(t *testing.T) {
	t.Parallel()
	f := newSessionFixtureOf(t, cameraCaptionsStoryboard)

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
