package tune

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/yamledit"
)

// Edit is one change the page asks for, as it arrives on the wire:
// {"beat": <id>, "index": <n>, "field": "<name>", "value": "<string>"}. Beat is the id of the
// beat edited, and 0 for frame.width and frame.fps; a camera field ignores it. Index is the
// position of the move a camera field edits in the storyboard's camera list, from 0, and is
// ignored by every other field. Value is spelled per field:
//   - duration, hold, zoom.in, zoom.out: a decimal integer of milliseconds ("1850");
//   - cut: "true" or "false";
//   - zoom.factor: a decimal ("1.8") or "auto";
//   - frame.width, frame.fps: a decimal integer;
//   - zoom.target, and zoom when it adds one: a target as YAML flow text, the zoom snippet a
//     proposal returns ({text: 'ok', area: list, nth: 2});
//   - zoom with "" removes the beat's zoom;
//   - camera.at: a moment as one line of YAML, a beat's {beat: 3, offset: 200ms} or a clip
//     time 4500ms, which replaces the move's whole at;
//   - camera.over: a decimal integer of milliseconds;
//   - camera.ease: smooth, which removes the key, or linear;
//   - camera.to: a decimal for a zoom move; for a pan, a target as YAML flow text or a cell
//     ({cell: {x: 3, y: 4}}), refused on a pan along a via path.
//
// Any other spelling is refused with an error naming the field.
type Edit struct {
	Beat  int    `json:"beat"`
	Index int    `json:"index"`
	Field string `json:"field"`
	Value string `json:"value"`
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
	fieldCameraTo,
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

// The value spellings with a fixed text.
const (
	valueTrue  = "true"
	valueFalse = "false"
	valueAuto  = "auto"
)

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
// nothing; cut false, zoom.factor auto and camera.ease smooth delete their key. A zoom.target on a block zoom
// holding nothing but its target rewrites that zoom as a flow mapping, since removing its only
// key would leave nothing to insert into. A camera.at or a pan's camera.to replaces its whole
// node with the value's flow text. The edit is refused with an error naming the beat, or the
// camera move, and field when its value is misspelled, when it names a beat, zoom or camera move
// that is not there, or when the result fails validation; the last wraps the
// *storyboard.ValidationError.
func apply(src []byte, path string, edit Edit) ([]byte, *storyboard.Storyboard, error) {
	board, err := storyboard.Parse(src, path)
	if err != nil {
		return nil, nil, fmt.Errorf("parse the storyboard before the edit: %w", err)
	}
	doc, err := yamledit.Parse(src)
	if err != nil {
		return nil, nil, fmt.Errorf("hold the storyboard for editing: %w", err)
	}
	var tree yaml.Node
	if err := yaml.Unmarshal(src, &tree); err != nil {
		return nil, nil, fmt.Errorf("read the storyboard's nodes: %w", err)
	}

	edits := &splicer{doc: doc, root: tree.Content[0]}
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
// `camera[1]: camera.at`.
func editName(edit Edit) string {
	if isFrameField(edit.Field) {
		return edit.Field
	}
	if isCameraField(edit.Field) {
		return fmt.Sprintf("camera[%d]: %s", edit.Index, edit.Field)
	}
	return fmt.Sprintf("beat %d: %s", edit.Beat, edit.Field)
}

// isFrameField reports whether field is one of the clip's frame fields rather than a beat's.
func isFrameField(field string) bool {
	return field == fieldFrameWidth || field == fieldFrameFPS
}

// splicer applies one edit to doc. root is the storyboard's node tree as it stood before the
// edit, which says what keys are present and how the old values are written.
type splicer struct {
	doc  *yamledit.Document
	root *yaml.Node
}

// apply routes edit to the frame, to a camera move or to its beat in board, the storyboard doc
// holds.
func (s *splicer) apply(board *storyboard.Storyboard, edit Edit) error {
	if isFrameField(edit.Field) {
		return s.editFrame(edit)
	}
	if !slices.Contains(editFields, edit.Field) {
		return fmt.Errorf("unknown field, want one of %s", strings.Join(editFields, ", "))
	}
	if isCameraField(edit.Field) {
		return s.editCamera(board, edit)
	}
	index := slices.IndexFunc(board.Beats, func(beat storyboard.Beat) bool { return beat.ID == edit.Beat })
	if index < 0 {
		return fmt.Errorf("the storyboard has no beat with id %d", edit.Beat)
	}
	beat := yamledit.Path{keyBeats, strconv.Itoa(index)}
	return s.editBeat(beat, board.Beats[index].Zoom != nil, edit)
}

// editFrame sets frame.width or frame.fps.
func (s *splicer) editFrame(edit Edit) error {
	if edit.Beat != 0 {
		return fmt.Errorf("want beat 0 for a frame field, got %d", edit.Beat)
	}
	count, err := parseCount(edit.Value)
	if err != nil {
		return err
	}
	key := strings.TrimPrefix(edit.Field, keyFrame+".")
	return s.writeScalar(yamledit.Path{keyFrame}, key, frameKeyOrder, strconv.Itoa(count))
}

// editBeat applies a beat field's edit to the beat at beat; hasZoom says whether it zooms.
func (s *splicer) editBeat(beat yamledit.Path, hasZoom bool, edit Edit) error {
	switch edit.Field {
	case fieldDuration, fieldHold:
		length, err := parseMilliseconds(edit.Value)
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
	old := s.lookup(under(mapping, key))
	if old == nil {
		if length == absent {
			return nil
		}
		return s.doc.Insert(mapping, key, length.String(), keysBefore(order, key))
	}
	return s.doc.Set(under(mapping, key), formatDuration(length, old.Value))
}

// setRamp writes a zoom's in or out ramp. A ramp of 0 is refused: an absent ramp already runs
// the default, and an explicit 0 would cut the push in or out.
func (s *splicer) setRamp(zoom yamledit.Path, key, value string) error {
	length, err := parseMilliseconds(value)
	if err != nil {
		return err
	}
	if length == 0 {
		return fmt.Errorf("want a ramp longer than 0ms; a ramp left out runs the %s default", storyboard.DefaultZoomRamp)
	}
	return s.writeDuration(zoom, key, zoomKeyOrder, storyboard.DefaultZoomRamp, length)
}

// setCut writes `cut: true` or deletes the key for false.
func (s *splicer) setCut(beat yamledit.Path, value string) error {
	switch value {
	case valueTrue:
		return s.writeScalar(beat, keyCut, beatKeyOrder, valueTrue)
	case valueFalse:
		return s.deleteKey(under(beat, keyCut))
	default:
		return fmt.Errorf("want %s or %s, got %q", valueTrue, valueFalse, value)
	}
}

// setFactor writes a zoom's factor, or deletes the key for auto.
func (s *splicer) setFactor(zoom yamledit.Path, value string) error {
	if value == valueAuto {
		return s.deleteKey(under(zoom, keyFactor))
	}
	spellingError := fmt.Errorf("want a decimal above 0 or %s, got %q", valueAuto, value)
	if !decimal.MatchString(value) {
		return spellingError
	}
	factor, err := strconv.ParseFloat(value, 64)
	if err != nil || factor == 0 {
		return spellingError
	}
	return s.writeScalar(zoom, keyFactor, zoomKeyOrder, strconv.FormatFloat(factor, 'f', -1, 64))
}

// addOrRemoveZoom adds a zoom onto the target value spells, or removes the beat's zoom when
// value is empty.
func (s *splicer) addOrRemoveZoom(beat yamledit.Path, hasZoom bool, value string) error {
	if value == "" {
		if !hasZoom {
			return errors.New("the beat has no zoom to remove")
		}
		return s.doc.Delete(under(beat, keyZoom))
	}
	if hasZoom {
		return errors.New("the beat already zooms; edit its target instead")
	}
	target, err := parseTarget(value)
	if err != nil {
		return err
	}
	return s.doc.Insert(beat, keyZoom, zoomOnto(target), keysBefore(beatKeyOrder, keyZoom))
}

// replaceTarget swaps the zoom's target for the one value spells, in its place as the zoom's
// first key.
func (s *splicer) replaceTarget(beat yamledit.Path, value string) error {
	target, err := parseTarget(value)
	if err != nil {
		return err
	}
	zoomPath := under(beat, keyZoom)
	zoom := s.lookup(zoomPath)
	isOnlyTarget := zoom.Kind == yaml.MappingNode && len(zoom.Content) == 2
	if isOnlyTarget && zoom.Style&yaml.FlowStyle == 0 {
		// Deleting a block zoom's only key leaves an empty value no insert can fill.
		if err := s.doc.Delete(zoomPath); err != nil {
			return err
		}
		return s.doc.Insert(beat, keyZoom, zoomOnto(target), keysBefore(beatKeyOrder, keyZoom))
	}
	if err := s.deleteKey(under(zoomPath, keyTarget)); err != nil {
		return err
	}
	return s.doc.Insert(zoomPath, keyTarget, target, nil)
}

// writeScalar sets the key in mapping to text, inserting it in order when it is absent.
func (s *splicer) writeScalar(mapping yamledit.Path, key string, order []string, text string) error {
	if s.lookup(under(mapping, key)) == nil {
		return s.doc.Insert(mapping, key, text, keysBefore(order, key))
	}
	return s.doc.Set(under(mapping, key), text)
}

// deleteKey removes the key at path; an absent key is already what the edit wants.
func (s *splicer) deleteKey(path yamledit.Path) error {
	if s.lookup(path) == nil {
		return nil
	}
	return s.doc.Delete(path)
}

// lookup returns the node at path in the tree as it stood before the edit, or nil.
func (s *splicer) lookup(path yamledit.Path) *yaml.Node {
	node := s.root
	for _, step := range path {
		node = childNode(node, step)
		if node == nil {
			return nil
		}
	}
	return node
}

// childNode takes one step from node: a key of a mapping, an index of a sequence; nil when
// there is no such child.
func childNode(node *yaml.Node, step string) *yaml.Node {
	switch node.Kind {
	case yaml.MappingNode:
		for index := 0; index+1 < len(node.Content); index += 2 {
			if node.Content[index].Value == step {
				return node.Content[index+1]
			}
		}
	case yaml.SequenceNode:
		index, err := strconv.Atoi(step)
		if err == nil && index >= 0 && index < len(node.Content) {
			return node.Content[index]
		}
	}
	return nil
}

// under returns path extended by key, never sharing path's backing array.
func under(path yamledit.Path, key string) yamledit.Path {
	return append(append(yamledit.Path{}, path...), key)
}

// keysBefore returns the keys that precede key in order.
func keysBefore(order []string, key string) []string {
	return order[:slices.Index(order, key)]
}

// zoomOnto spells a zoom onto target, a target's flow text, with every other key left out.
func zoomOnto(target string) string {
	return "{" + keyTarget + ": " + target + "}"
}
