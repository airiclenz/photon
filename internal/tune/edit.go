package tune

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/yamledit"
)

// Edit is one change the page asks for, a [storyboard.Edit], whose doc gives its wire shape and
// the beat and frame fields' values; [storyboard.Apply] applies those. The camera and caption
// fields, applied here, edit the camera move or caption by its index in the storyboard's camera
// or captions list, from 0, and type their values so:
//   - camera.at: a moment, {"beat": 3, "offset_ms": 200}, beat 0 for a clip time offset_ms
//     after the clip's first frame, which replaces the move's whole at;
//   - camera.over: a whole number of milliseconds;
//   - camera.ease: "smooth", which removes the key, or "linear";
//   - camera.to: a number, the factor, for a zoom move; for a pan, a target as zoom.target
//     takes it or a cell, {"cell": {"x": 3, "y": 4}}, refused on a pan along a via path;
//   - caption.at: a moment as camera.at takes it, which replaces the caption's whole at;
//   - caption.for: a whole number of milliseconds above 0, which removes the caption's until;
//   - caption.until: a moment, which removes the caption's for; null clears the until, the
//     caption then ending after a for of the length it shows now;
//   - caption.text: a string, its lines separated by \n;
//   - caption.place: "top", "middle" or "bottom", which removes the key;
//   - caption.align: "left", "middle", which removes the key, or "right".
//
// A value of another JSON type or spelling, a null or absent value but caption.until's, and an
// owner of another kind than the field's are refused with an error naming the field.
type Edit = storyboard.Edit

// editFields lists every field an Edit names that tune applies itself, as the unknown-field
// error offers them.
var editFields = []string{
	fieldCameraAt, fieldCameraOver, fieldCameraEase, fieldCameraTo, fieldCaptionAt, fieldCaptionFor,
	fieldCaptionUntil, fieldCaptionText, fieldCaptionPlace, fieldCaptionAlign,
}

// keyZoom is the storyboard key of a zoom move, the camera move a camera edit may address.
const keyZoom = "zoom"

// apply returns src, the valid storyboard at path, with edit spliced in, and the storyboard
// those bytes decode to. A beat or frame field goes to [storyboard.Apply], which documents how
// it is written; a camera or caption field is spliced here. Only the edited key's text changes.
// A duration keeps its unit: an old value in ms is written in ms, one in plain seconds in
// seconds with the fewest decimals, any other or new value as time.Duration.String(). A new key
// goes after the keys that precede it in the storyboard's field order; a value equal to the
// default of a key that is absent writes nothing; camera.ease smooth deletes its key, as do a
// caption.place of bottom and a caption.align of middle. A camera.at, caption.at or
// caption.until is written as a clip time in ms or a {beat: 3, offset: 200ms} flow mapping
// ([momentFlow]), a pan's target as [storyboard.Target.YAMLFlow] spells it, and a pan's cell as
// {cell: {x: 3, y: 4}}. A camera.at, a caption.at or caption.until, or a pan's camera.to
// replaces its whole node with the value's flow text, and a caption.text replaces its
// whole node, a block scalar too, with one double-quoted line; a caption.for deletes the
// caption's until and a caption.until its for, or, as null, writes the for that keeps the
// caption's stretch in place of its until. The edit is refused with an error naming the
// camera move or the caption, and field when its value is of the wrong JSON type or
// misspelled, when its owner is of another kind than the field's, when it names a camera move
// or caption that is not there, or when the result fails validation; the last wraps the
// *storyboard.ValidationError.
func apply(src []byte, path string, edit Edit) ([]byte, *storyboard.Storyboard, error) {
	if !isCameraField(edit.Field) && !isCaptionField(edit.Field) {
		return storyboard.Apply(src, path, edit)
	}
	board, err := storyboard.Parse(src, path)
	if err != nil {
		return nil, nil, fmt.Errorf("parse the storyboard before the edit: %w", err)
	}
	doc, err := yamledit.Parse(src)
	if err != nil {
		return nil, nil, fmt.Errorf("hold the storyboard for editing: %w", err)
	}

	edits := &splicer{doc: doc}
	if err := edits.apply(board, edit); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", editName(edit), err)
	}
	edited := doc.Bytes()
	next, err := storyboard.Parse(edited, path)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: refused: %w", editName(edit), err)
	}
	return edited, next, nil
}

// editName names what edit changes, as its errors lead with it: `beat 2: hold`, `frame.fps`,
// `camera[1]: camera.at`, `captions[0]: caption.text`.
func editName(edit Edit) string {
	switch edit.Owner.Kind {
	case storyboard.OwnerBeat:
		return fmt.Sprintf("beat %d: %s", edit.Owner.Index, edit.Field)
	case storyboard.OwnerCamera:
		return fmt.Sprintf("camera[%d]: %s", edit.Owner.Index, edit.Field)
	case storyboard.OwnerCaption:
		return fmt.Sprintf("captions[%d]: %s", edit.Owner.Index, edit.Field)
	default:
		return edit.Field
	}
}

// ownerKindOf is the kind of owner field, a camera or caption field, edits.
func ownerKindOf(field string) storyboard.OwnerKind {
	if isCameraField(field) {
		return storyboard.OwnerCamera
	}
	return storyboard.OwnerCaption
}

// splicer applies one edit to doc, which says what keys are present and how the old values are
// written as each splice of the edit lands.
type splicer struct {
	doc *yamledit.Document
}

// apply applies edit to its camera move or caption in board, the storyboard doc holds, once its
// field, its owner's kind and the presence of its value check out.
func (s *splicer) apply(board *storyboard.Storyboard, edit Edit) error {
	if !slices.Contains(editFields, edit.Field) {
		return fmt.Errorf("unknown field, want one of %s", strings.Join(editFields, ", "))
	}
	if want := ownerKindOf(edit.Field); edit.Owner.Kind != want {
		return fmt.Errorf("want a %s owner, got %q", want, edit.Owner.Kind)
	}
	if len(edit.Value) == 0 {
		return errors.New("want a value, got none")
	}
	if isNull(edit.Value) && edit.Field != fieldCaptionUntil {
		return errors.New("want a value, got null")
	}

	if edit.Owner.Kind == storyboard.OwnerCamera {
		return s.editCamera(board, edit.Owner.Index, edit.Field, edit.Value)
	}
	return s.editCaption(board, edit.Owner.Index, edit.Field, edit.Value)
}

// writeDuration writes length to the duration key in mapping, whose keys follow order; absent
// is the value the key means when it is left out.
func (s *splicer) writeDuration(
	mapping yamledit.Path,
	key string,
	order []string,
	absent time.Duration,
	length time.Duration,
) error {
	path := under(mapping, key)
	if !s.doc.Has(path) {
		if length == absent {
			return nil
		}
		return s.doc.Upsert(mapping, key, length.String(), order)
	}
	old, _ := s.doc.Scalar(path)
	return s.doc.Upsert(mapping, key, formatDuration(length, old), order)
}

// replaceOrInsert puts text, a value's one line of YAML, in place of the whole node at key in
// mapping, whatever its form, a block scalar too, or inserts `key: text` after the keys that
// precede it in order when the key is absent. Upsert serves only the absent case: its Set
// refuses a present block scalar.
func (s *splicer) replaceOrInsert(mapping yamledit.Path, key, text string, order []string) error {
	path := under(mapping, key)
	if s.doc.Has(path) {
		return s.doc.Replace(path, text)
	}
	return s.doc.Upsert(mapping, key, text, order)
}

// under returns path extended by key, never sharing path's backing array.
func under(path yamledit.Path, key string) yamledit.Path {
	return append(append(yamledit.Path{}, path...), key)
}
