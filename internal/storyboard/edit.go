package storyboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/airiclenz/photon/internal/yamledit"
)

// Edit is one typed change to a storyboard field, as photon tune's page sends it:
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
//   - caption.align: "left", "middle", which removes the key, or "right";
//   - caption.font.size: a number above 0, frame.font_size removing the key;
//   - caption.font.weight: "regular", which removes the key, or "bold";
//   - caption.color, caption.box.color: a colour, "#ffcc00", its default — "#ffffff" for the
//     text and "#000000" for the box, in either case — removing the key;
//   - caption.box.opacity: a number from 0 to 1, 0.6 removing the key;
//   - caption.fade.in, caption.fade.out: a whole number of milliseconds, 0 removing the key;
//   - camera.insert, caption.insert: a new entry, {"kind": "zoom", "at": {"beat": 0,
//     "offset_ms": 1200}}, its kind zoom or pan for the camera and caption for the captions and
//     its at a moment, added so that it is the entry at the owner's index, 0 through the list's
//     length;
//   - camera.remove, caption.remove: null, which removes the owner's entry;
//   - camera.move, caption.move: a whole number, the index the owner's entry moves to.
//
// A value of another JSON type or spelling, a null or absent value but zoom's, caption.until's
// and a remove's, and an owner of another kind than the field's are refused with an error
// naming the field.
type Edit struct {
	Owner Owner           `json:"owner"`
	Field string          `json:"field"`
	Value json.RawMessage `json:"value"`
}

// The beat and frame fields an [Edit] names for [Apply].
const (
	FieldDuration   = "duration"
	FieldHold       = "hold"
	FieldCut        = "cut"
	FieldZoom       = "zoom"
	FieldZoomTarget = "zoom.target"
	FieldZoomFactor = "zoom.factor"
	FieldZoomIn     = "zoom.in"
	FieldZoomOut    = "zoom.out"
	FieldFrameWidth = "frame.width"
	FieldFrameFPS   = "frame.fps"
)

// editFields lists every field Apply takes, as the unknown-field error offers them.
var editFields = []string{
	FieldDuration, FieldHold, FieldCut, FieldZoom, FieldZoomTarget, FieldZoomFactor, FieldZoomIn,
	FieldZoomOut, FieldFrameWidth, FieldFrameFPS, FieldCameraAt, FieldCameraOver, FieldCameraEase,
	FieldCameraTo, FieldCaptionAt, FieldCaptionFor, FieldCaptionUntil, FieldCaptionText,
	FieldCaptionPlace, FieldCaptionAlign, FieldCameraInsert, FieldCameraRemove, FieldCameraMove,
	FieldCaptionInsert, FieldCaptionRemove, FieldCaptionMove, FieldCaptionFontSize,
	FieldCaptionFontWeight, FieldCaptionColor, FieldCaptionBoxColor, FieldCaptionBoxOpacity,
	FieldCaptionFadeIn, FieldCaptionFadeOut,
}

// EditFields returns every field an [Edit] names for [Apply], in a new slice.
func EditFields() []string {
	return slices.Clone(editFields)
}

// maxMilliseconds is the longest duration in milliseconds a time.Duration holds.
const maxMilliseconds = math.MaxInt64 / int64(time.Millisecond)

// millisecondsPerSecond splits a duration into whole seconds and the milliseconds past them.
const millisecondsPerSecond = int64(time.Second / time.Millisecond)

// secondsForm is a duration written as plain seconds (`2s`, `2.35s`), which an edit keeps.
var secondsForm = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?s$`)

// jsonNull is the JSON null literal.
const jsonNull = "null"

// The yaml tag parts keyOrder reads: the tag key, the name of a field the decoder skips, and the
// option that folds a field's keys into its parent's.
const (
	yamlTag      = "yaml"
	yamlSkipped  = "-"
	yamlInline   = "inline"
	tagSeparator = ","
)

// Apply returns src, the valid storyboard at path, with edit spliced in, and the storyboard
// those bytes decode to; path names the file in both decodes' errors and is the result's Path.
// Only the edited key's text changes. A duration keeps its unit: an old value in ms is written in
// ms, one in plain seconds in seconds with the fewest decimals, any other or new value as
// time.Duration.String(). A new key goes after the keys that precede it in the storyboard's
// field order, its yaml struct tags; a value equal to the default of a key that is absent writes
// nothing; cut false, zoom.factor 0 (auto), camera.ease smooth, caption.place bottom,
// caption.align middle and a caption's font, colour, box or fade at the default [Load] fills in
// delete their key, and a caption's font, box or fade mapping left holding no key goes with it,
// while a style value a caption lacks the mapping for creates it as a one-key flow mapping. A
// colour is written quoted, '#ffcc00', and compared to its default case-insensitively. A target is written as the flow mapping
// [Target.YAMLFlow] spells, and a zoom.target on a zoom holding nothing but its target rewrites
// that zoom as a flow mapping. A camera.at, caption.at or caption.until is written as a clip time
// in ms or a {beat: 3, offset: 200ms} flow mapping, and a pan's cell as {cell: {x: 3, y: 4}}; a
// camera.at, a caption.at or caption.until, or a pan's camera.to replaces its whole node with the
// value's flow text, and a caption.text replaces its whole node, a block scalar too, with one
// double-quoted line; a caption.for deletes the caption's until and a caption.until its for, or,
// as null, writes the for that keeps the caption's stretch in place of its until. An insert writes
// its entry as one flow line from presets — a zoom to 1.5 or a pan to the frame's centre cell,
// over 600ms, or a caption "Caption" for 2s — and creates a camera or captions key the storyboard
// lacks as a flow list after the keys that precede it; a remove or move splices only its list's
// items, and removing a list's only entry deletes its key. The edit is
// refused with an error led by its owner and field — the beat, camera move or caption, or
// nothing for a frame field — when its value is of the wrong JSON type or misspelled, when its
// owner is of another kind than the field's, when it names a beat, zoom, camera move or caption
// that is not there, an insert index past the list's length among them, or when the result fails
// validation, an inserted move overlapping another on its track among them; the last wraps the
// *[ValidationError].
func Apply(src []byte, path string, edit Edit) ([]byte, *Storyboard, error) {
	board, err := Parse(src, path)
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
	next, err := Parse(edited, path)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: refused: %w", editName(edit), err)
	}
	return edited, next, nil
}

// editName names what edit changes, as its errors lead with it: `beat 2: hold`, `frame.fps`,
// `camera[1]: camera.at`, `captions[0]: caption.text`.
func editName(edit Edit) string {
	switch edit.Owner.Kind {
	case OwnerBeat:
		return fmt.Sprintf("beat %d: %s", edit.Owner.Index, edit.Field)
	case OwnerCamera:
		return fmt.Sprintf("camera[%d]: %s", edit.Owner.Index, edit.Field)
	case OwnerCaption:
		return fmt.Sprintf("captions[%d]: %s", edit.Owner.Index, edit.Field)
	default:
		return edit.Field
	}
}

// ownerKindOf is the kind of owner field edits: the clip for a frame field, a camera move for a
// camera field, a caption for a caption field and a beat for every other.
func ownerKindOf(field string) OwnerKind {
	switch {
	case field == FieldFrameWidth || field == FieldFrameFPS:
		return OwnerClip
	case isCameraField(field):
		return OwnerCamera
	case isCaptionField(field):
		return OwnerCaption
	default:
		return OwnerBeat
	}
}

// splicer applies one edit to doc, which says what keys are present and how the old values are
// written as each splice of the edit lands.
type splicer struct {
	doc *yamledit.Document
}

// apply routes edit to the frame or to its beat, camera move or caption in board, the storyboard doc holds, once its
// field, its owner's kind and the presence of its value check out.
func (s *splicer) apply(board *Storyboard, edit Edit) error {
	if !slices.Contains(editFields, edit.Field) {
		return fmt.Errorf("unknown field, want one of %s", strings.Join(editFields, ", "))
	}
	if want := ownerKindOf(edit.Field); edit.Owner.Kind != want {
		return fmt.Errorf("want a %s owner, got %q", want, edit.Owner.Kind)
	}
	if len(edit.Value) == 0 {
		return errors.New("want a value, got none")
	}
	isNullTaken := edit.Field == FieldZoom || edit.Field == FieldCaptionUntil || isRemoveField(edit.Field)
	if isNull(edit.Value) && !isNullTaken {
		return errors.New("want a value, got null")
	}

	switch edit.Owner.Kind {
	case OwnerClip:
		return s.editFrame(edit)
	case OwnerCamera:
		return s.editCamera(board, edit.Owner.Index, edit.Field, edit.Value)
	case OwnerCaption:
		return s.editCaption(board, edit.Owner.Index, edit.Field, edit.Value)
	}
	index := slices.IndexFunc(board.Beats, func(beat Beat) bool { return beat.ID == edit.Owner.Index })
	if index < 0 {
		return fmt.Errorf("the storyboard has no beat with id %d", edit.Owner.Index)
	}
	beat := yamledit.Path{keyBeats, strconv.Itoa(index)}
	return s.editBeat(beat, board.Beats[index].Zoom != nil, edit)
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

// setMoment replaces the whole at of mapping, a camera move or a caption, whatever its form,
// with the moment value types, spelled as [momentFlow] does. A clip start on a mapping without at
// writes nothing: an absent at already starts it there.
func (s *splicer) setMoment(mapping yamledit.Path, order []string, value json.RawMessage) error {
	moment, err := decodeMoment(value)
	if err != nil {
		return err
	}
	if moment == (Moment{}) && !s.doc.Has(under(mapping, keyAt)) {
		return nil
	}
	return s.replaceOrInsert(mapping, keyAt, momentFlow(moment), order)
}

// momentFlow spells moment as the storyboard writes one: a clip time in milliseconds (4500ms),
// or a beat's {beat: 3, offset: 200ms} with an offset of 0 left out.
func momentFlow(moment Moment) string {
	offset := strconv.FormatInt(moment.Offset.Milliseconds(), 10) + "ms"
	if moment.Beat == 0 {
		return offset
	}
	beat := "beat: " + strconv.Itoa(moment.Beat)
	if moment.Offset == 0 {
		return "{" + beat + "}"
	}
	return "{" + beat + ", offset: " + offset + "}"
}

// under returns path extended by key, never sharing path's backing array.
func under(path yamledit.Path, key string) yamledit.Path {
	return append(append(yamledit.Path{}, path...), key)
}

// keyOrder is the YAML keys of the struct type of, in its field order, as its yaml tags name
// them: an edit inserts a new key after whichever of the keys before it the mapping holds. A
// field the decoder skips — unexported, or tagged `yaml:"-"` — or folds into its parent
// (`yaml:",inline"`) has no key of its own and is left out; a field with no tag name is keyed by
// its lowercased name, as the decoder keys it.
func keyOrder(of reflect.Type) []string {
	keys := make([]string, 0, of.NumField())
	for field := range of.Fields() {
		if !field.IsExported() {
			continue
		}
		name, options, _ := strings.Cut(field.Tag.Get(yamlTag), tagSeparator)
		if name == yamlSkipped || slices.Contains(strings.Split(options, tagSeparator), yamlInline) {
			continue
		}
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		keys = append(keys, name)
	}
	return keys
}

// isNull reports whether value is the JSON null.
func isNull(value json.RawMessage) bool {
	return string(bytes.TrimSpace(value)) == jsonNull
}

// decodeMilliseconds reads a duration typed as a whole JSON number of milliseconds.
func decodeMilliseconds(value json.RawMessage) (time.Duration, error) {
	var count int64
	err := json.Unmarshal(value, &count)
	if err != nil || count < 0 || count > maxMilliseconds {
		return 0, fmt.Errorf("want a whole number of milliseconds such as 1850, got %s", value)
	}
	return time.Duration(count) * time.Millisecond, nil
}

// wireMoment is a moment as an [Edit] types it, {"beat": 3, "offset_ms": 200}: beat 0 is a clip
// time, offset_ms after the clip's first frame; a beat id from 1 is that beat's start plus
// offset_ms, which may be negative. Both keys are required.
type wireMoment struct {
	Beat     *int   `json:"beat"`
	OffsetMS *int64 `json:"offset_ms"`
}

// decodeMoment reads a moment typed as {"beat", "offset_ms"}, refusing any other key, a missing
// one, a negative beat, a negative clip time and an offset no time.Duration holds.
func decodeMoment(value json.RawMessage) (Moment, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireMoment
	err := decoder.Decode(&wire)
	isWhole := err == nil && wire.Beat != nil && wire.OffsetMS != nil
	if !isWhole || *wire.Beat < 0 || (*wire.Beat == 0 && *wire.OffsetMS < 0) ||
		*wire.OffsetMS > maxMilliseconds || *wire.OffsetMS < -maxMilliseconds {
		return Moment{}, fmt.Errorf(
			`want a moment such as {"beat": 3, "offset_ms": 200}, or beat 0 for a clip time from 0, got %s`, value)
	}
	offset := time.Duration(*wire.OffsetMS) * time.Millisecond
	return Moment{Beat: *wire.Beat, Offset: offset}, nil
}

// formatDuration spells length in the unit old, the value it replaces, is written in.
func formatDuration(length time.Duration, old string) string {
	switch {
	case strings.HasSuffix(old, "ms"):
		return strconv.FormatInt(length.Milliseconds(), 10) + "ms"
	case secondsForm.MatchString(old):
		return formatSeconds(length)
	default:
		return length.String()
	}
}

// formatSeconds spells length, a whole number of milliseconds, in seconds with the fewest
// decimals: 2s, 2.35s.
func formatSeconds(length time.Duration) string {
	milliseconds := length.Milliseconds()
	whole, fraction := milliseconds/millisecondsPerSecond, milliseconds%millisecondsPerSecond
	if fraction == 0 {
		return strconv.FormatInt(whole, 10) + "s"
	}
	digits := strings.TrimRight(fmt.Sprintf("%03d", fraction), "0")
	return strconv.FormatInt(whole, 10) + "." + digits + "s"
}
