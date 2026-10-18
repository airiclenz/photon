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
// what the field edits: the beat by its id for a beat field, and the clip with index 0 for
// frame.width and frame.fps. Value is typed per field:
//   - duration, hold, zoom.in, zoom.out: a whole number of milliseconds (1850);
//   - cut: true or false;
//   - zoom.factor: a number, 0 for auto, fitted to the target (1.8);
//   - frame.width, frame.fps: a whole number;
//   - zoom.target, and zoom when it adds one: a target, {"text": "ok", "nth": 2, "area":
//     "list"}, its nth a whole number from 1 or "last" and left out for the first match, its
//     area left out for the whole screen — the target a pick answers;
//   - zoom as null removes the beat's zoom.
//
// A value of another JSON type or spelling, a null or absent value but zoom's, and an owner of
// another kind than the field's are refused with an error naming the field.
type Edit struct {
	Owner Owner           `json:"owner"`
	Field string          `json:"field"`
	Value json.RawMessage `json:"value"`
}

// The fields an [Edit] names for [Apply].
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
	FieldZoomOut, FieldFrameWidth, FieldFrameFPS,
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
// nothing; cut false and zoom.factor 0 (auto) delete their key. A target is written as the flow
// mapping [Target.YAMLFlow] spells, and a zoom.target on a zoom holding nothing but its target
// rewrites that zoom as a flow mapping. The edit is refused with an error led by the beat and
// field, or the frame field, when its value is of the wrong JSON type or misspelled, when its
// owner is of another kind than the field's, when it names a beat or zoom that is not there, or
// when the result fails validation; the last wraps the *[ValidationError].
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
// `camera[1]: duration`, `captions[0]: hold`.
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

// ownerKindOf is the kind of owner field edits: the clip for a frame field and a beat for every
// other.
func ownerKindOf(field string) OwnerKind {
	if field == FieldFrameWidth || field == FieldFrameFPS {
		return OwnerClip
	}
	return OwnerBeat
}

// splicer applies one edit to doc, which says what keys are present and how the old values are
// written as each splice of the edit lands.
type splicer struct {
	doc *yamledit.Document
}

// apply routes edit to the frame or to its beat in board, the storyboard doc holds, once its
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
	if isNull(edit.Value) && edit.Field != FieldZoom {
		return errors.New("want a value, got null")
	}

	if edit.Owner.Kind == OwnerClip {
		return s.editFrame(edit)
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

// under returns path extended by key, never sharing path's backing array.
func under(path yamledit.Path, key string) yamledit.Path {
	return append(append(yamledit.Path{}, path...), key)
}

// keyOrder is the YAML keys of the struct type of, in its field order, as its yaml tags name
// them: an edit inserts a new key after whichever of the keys before it the mapping holds. A
// field the decoder skips (`yaml:"-"`) or folds into its parent (`yaml:",inline"`) has no key of
// its own and is left out; a field with no tag name is keyed by its lowercased name, as the
// decoder keys it.
func keyOrder(of reflect.Type) []string {
	keys := make([]string, 0, of.NumField())
	for field := range of.Fields() {
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
