package tune

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/yamledit"
)

// Edit is one change the page asks for, as it arrives on the wire:
// {"owner": {"kind": "<kind>", "index": <n>}, "field": "<name>", "value": <JSON>}. Owner is
// what the field edits: the beat by its id for a beat field, the clip with index 0 for
// frame.width and frame.fps, and the camera move or caption by its index in the storyboard's
// camera or captions list, from 0, for a camera or caption field. Value is typed per field:
//   - duration, hold, zoom.in, zoom.out: a whole number of milliseconds (1850);
//   - cut: true or false;
//   - zoom.factor: a number, 0 for auto, fitted to the target (1.8);
//   - frame.width, frame.fps: a whole number;
//   - zoom.target, and zoom when it adds one: a target, {"text": "ok", "nth": 2, "area":
//     "list"}, its nth a whole number from 1 or "last" and left out for the first match, its
//     area left out for the whole screen — the target a pick answers;
//   - zoom as null removes the beat's zoom;
//   - every camera and caption field: a JSON string holding the text below.
//
// A camera or caption field's text is spelled:
//   - camera.at: a moment as one line of YAML, a beat's {beat: 3, offset: 200ms} or a clip
//     time 4500ms, which replaces the move's whole at;
//   - camera.over: a decimal integer of milliseconds;
//   - camera.ease: smooth, which removes the key, or linear;
//   - camera.to: a decimal for a zoom move; for a pan, a target as YAML flow text or a cell
//     ({cell: {x: 3, y: 4}}), refused on a pan along a via path;
//   - caption.at: a moment, as camera.at spells it;
//   - caption.for: a decimal integer of milliseconds above 0, which removes the caption's until;
//   - caption.until: a moment, as camera.at spells it, which removes the caption's for;
//   - caption.text: the text itself, its lines separated by \n;
//   - caption.place: top, middle or bottom, which removes the key;
//   - caption.align: left, middle, which removes the key, or right.
//
// A value of another JSON type or spelling, a null or absent value but zoom's, and an owner of
// another kind than the field's are refused with an error naming the field.
type Edit struct {
	Owner storyboard.Owner `json:"owner"`
	Field string           `json:"field"`
	Value json.RawMessage  `json:"value"`
}

// The fields an Edit names.
const (
	fieldDuration   = "duration"
	fieldHold       = "hold"
	fieldCut        = "cut"
	fieldZoom       = "zoom"
	fieldZoomTarget = "zoom.target"
	fieldZoomFactor = "zoom.factor"
	fieldZoomIn     = "zoom.in"
	fieldZoomOut    = "zoom.out"
	fieldFrameWidth = "frame.width"
	fieldFrameFPS   = "frame.fps"
)

// editFields lists every field an Edit can name, as the unknown-field error offers them.
var editFields = []string{
	fieldDuration, fieldHold, fieldCut, fieldZoom, fieldZoomTarget, fieldZoomFactor, fieldZoomIn,
	fieldZoomOut, fieldFrameWidth, fieldFrameFPS, fieldCameraAt, fieldCameraOver, fieldCameraEase,
	fieldCameraTo, fieldCaptionAt, fieldCaptionFor, fieldCaptionUntil, fieldCaptionText,
	fieldCaptionPlace, fieldCaptionAlign,
}

// The storyboard keys an edit addresses directly; a field's own key is its name past the dot.
const (
	keyBeats  = "beats"
	keyFrame  = "frame"
	keyCut    = "cut"
	keyZoom   = "zoom"
	keyTarget = "target"
	keyFactor = "factor"
)

// valueTrue is the YAML text of cut switched on.
const valueTrue = "true"

// beatKeyOrder, zoomKeyOrder and frameKeyOrder are the storyboard's keys in its struct field
// order: a new key goes after whichever of the keys before it the mapping holds.
var (
	beatKeyOrder  = []string{"id", "title", "why", "notes", "do", "duration", "hold", "cut", "zoom"}
	zoomKeyOrder  = []string{"target", "factor", "in", "out"}
	frameKeyOrder = []string{
		"cols", "rows", "padding", "font_size", "line_height", "scale", "width", "fps", "max_colors",
	}
)

// apply returns src, the valid storyboard at path, with edit spliced in, and the storyboard
// those bytes decode to. Only the edited key's text changes. A duration keeps its unit: an old
// value in ms is written in ms, one in plain seconds in seconds with the fewest decimals, any
// other or new value as time.Duration.String(). A new key goes after the keys that precede it in
// the storyboard's field order; a value equal to the default of a key that is absent writes
// nothing; cut false, zoom.factor 0 (auto) and camera.ease smooth delete their key, as do a
// caption.place of bottom and a caption.align of middle. A target is written as the flow
// mapping a pick's zoom snippet spells ([targetFlow]), and a zoom.target on a zoom holding
// nothing but its target rewrites that zoom as a flow mapping. A camera.at, a caption.at or caption.until, or a pan's
// camera.to replaces its whole node with the value's flow text, and a caption.text replaces its
// whole node, a block scalar too, with one double-quoted line; a caption.for deletes the
// caption's until and a caption.until its for. The edit is refused with an error naming the
// beat, the camera move or the caption, and field when its value is of the wrong JSON type or
// misspelled, when its owner is of another kind than the field's, when it names a beat, zoom,
// camera move or caption that is not there, or when the result fails validation; the last wraps
// the *storyboard.ValidationError.
func apply(src []byte, path string, edit Edit) ([]byte, *storyboard.Storyboard, error) {
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

// ownerKindOf is the kind of owner field edits: the clip for a frame field, a camera move or a
// caption for theirs, and a beat for every other.
func ownerKindOf(field string) storyboard.OwnerKind {
	switch {
	case isFrameField(field):
		return storyboard.OwnerClip
	case isCameraField(field):
		return storyboard.OwnerCamera
	case isCaptionField(field):
		return storyboard.OwnerCaption
	default:
		return storyboard.OwnerBeat
	}
}

// isFrameField reports whether field is one of the clip's frame fields rather than a beat's.
func isFrameField(field string) bool {
	return field == fieldFrameWidth || field == fieldFrameFPS
}

// splicer applies one edit to doc, which says what keys are present and how the old values are
// written as each splice of the edit lands.
type splicer struct {
	doc *yamledit.Document
}

// apply routes edit to the frame, to a camera move, to a caption or to its beat in board, the
// storyboard doc holds, once its field, its owner's kind and the presence of its value check out.
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
	if isNull(edit.Value) && edit.Field != fieldZoom {
		return errors.New("want a value, got null")
	}

	switch edit.Owner.Kind {
	case storyboard.OwnerClip:
		return s.editFrame(edit)
	case storyboard.OwnerCamera, storyboard.OwnerCaption:
		return s.editEntry(board, edit)
	}
	index := slices.IndexFunc(board.Beats, func(beat storyboard.Beat) bool { return beat.ID == edit.Owner.Index })
	if index < 0 {
		return fmt.Errorf("the storyboard has no beat with id %d", edit.Owner.Index)
	}
	beat := yamledit.Path{keyBeats, strconv.Itoa(index)}
	return s.editBeat(beat, board.Beats[index].Zoom != nil, edit)
}

// editEntry applies a camera move's or a caption's field, whose value is a JSON string holding
// the field's text.
func (s *splicer) editEntry(board *storyboard.Storyboard, edit Edit) error {
	text, err := decodeText(edit.Value)
	if err != nil {
		return err
	}
	if edit.Owner.Kind == storyboard.OwnerCamera {
		return s.editCamera(board, edit.Owner.Index, edit.Field, text)
	}
	return s.editCaption(board, edit.Owner.Index, edit.Field, text)
}

// editFrame sets frame.width or frame.fps.
func (s *splicer) editFrame(edit Edit) error {
	if edit.Owner.Index != 0 {
		return fmt.Errorf("want the clip at index 0, got index %d", edit.Owner.Index)
	}
	count, err := decodeCount(edit.Value)
	if err != nil {
		return err
	}
	key := strings.TrimPrefix(edit.Field, keyFrame+".")
	return s.doc.Upsert(yamledit.Path{keyFrame}, key, strconv.Itoa(count), frameKeyOrder)
}

// editBeat applies a beat field's edit to the beat at beat; hasZoom says whether it zooms.
func (s *splicer) editBeat(beat yamledit.Path, hasZoom bool, edit Edit) error {
	switch edit.Field {
	case fieldDuration, fieldHold:
		length, err := decodeMilliseconds(edit.Value)
		if err != nil {
			return err
		}
		return s.writeDuration(beat, edit.Field, beatKeyOrder, 0, length)
	case fieldCut:
		return s.setCut(beat, edit.Value)
	case fieldZoom:
		return s.addOrRemoveZoom(beat, hasZoom, edit.Value)
	}

	if !hasZoom {
		return errors.New("the beat has no zoom; add one first")
	}
	zoom := under(beat, keyZoom)
	switch edit.Field {
	case fieldZoomTarget:
		return s.replaceTarget(beat, edit.Value)
	case fieldZoomFactor:
		return s.setFactor(zoom, edit.Value)
	default:
		return s.setRamp(zoom, strings.TrimPrefix(edit.Field, keyZoom+"."), edit.Value)
	}
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

// setRamp writes a zoom's in or out ramp. A ramp of 0 is refused: an absent ramp already runs
// the default, and an explicit 0 would cut the push in or out.
func (s *splicer) setRamp(zoom yamledit.Path, key string, value json.RawMessage) error {
	length, err := decodeMilliseconds(value)
	if err != nil {
		return err
	}
	if length == 0 {
		return fmt.Errorf("want a ramp longer than 0ms; a ramp left out runs the %s default", storyboard.DefaultZoomRamp)
	}
	return s.writeDuration(zoom, key, zoomKeyOrder, storyboard.DefaultZoomRamp, length)
}

// setCut writes `cut: true` or deletes the key for false.
func (s *splicer) setCut(beat yamledit.Path, value json.RawMessage) error {
	isCut, err := decodeBool(value)
	if err != nil {
		return err
	}
	if isCut {
		return s.doc.Upsert(beat, keyCut, valueTrue, beatKeyOrder)
	}
	return s.doc.DeleteIfPresent(under(beat, keyCut))
}

// setFactor writes a zoom's factor, or deletes the key for 0, auto.
func (s *splicer) setFactor(zoom yamledit.Path, value json.RawMessage) error {
	factor, err := decodeFactor(value)
	if err != nil {
		return err
	}
	if factor == 0 {
		return s.doc.DeleteIfPresent(under(zoom, keyFactor))
	}
	return s.doc.Upsert(zoom, keyFactor, strconv.FormatFloat(factor, 'f', -1, 64), zoomKeyOrder)
}

// addOrRemoveZoom adds a zoom onto the target value holds, or removes the beat's zoom when
// value is null.
func (s *splicer) addOrRemoveZoom(beat yamledit.Path, hasZoom bool, value json.RawMessage) error {
	if isNull(value) {
		if !hasZoom {
			return errors.New("the beat has no zoom to remove")
		}
		return s.doc.Delete(under(beat, keyZoom))
	}
	if hasZoom {
		return errors.New("the beat already zooms; edit its target instead")
	}
	target, err := decodeTarget(value)
	if err != nil {
		return err
	}
	return s.doc.Upsert(beat, keyZoom, zoomOnto(targetFlow(target)), beatKeyOrder)
}

// replaceTarget swaps the zoom's target for the one value holds, in its place. A zoom holding
// nothing but its target is replaced whole by a flow zoom onto the new target, the form a zoom
// is added in.
func (s *splicer) replaceTarget(beat yamledit.Path, value json.RawMessage) error {
	decoded, err := decodeTarget(value)
	if err != nil {
		return err
	}
	target := targetFlow(decoded)
	zoom := under(beat, keyZoom)
	isOnlyTarget := true
	for _, key := range zoomKeyOrder[1:] {
		isOnlyTarget = isOnlyTarget && !s.doc.Has(under(zoom, key))
	}
	if isOnlyTarget {
		return s.doc.Replace(zoom, zoomOnto(target))
	}
	return s.doc.Replace(under(zoom, keyTarget), target)
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

// zoomOnto spells a zoom onto target, a target's flow text, with every other key left out.
func zoomOnto(target string) string {
	return "{" + keyTarget + ": " + target + "}"
}
