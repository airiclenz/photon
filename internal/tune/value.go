package tune

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/airiclenz/photon/internal/storyboard"
)

// maxMilliseconds is the longest duration in milliseconds a time.Duration holds.
const maxMilliseconds = math.MaxInt64 / int64(time.Millisecond)

// millisecondsPerSecond splits a duration into whole seconds and the milliseconds past them.
const millisecondsPerSecond = int64(time.Second / time.Millisecond)

var (
	// wholeNumber is the spelling of milliseconds and of a frame integer.
	wholeNumber = regexp.MustCompile(`^[0-9]+$`)
	// secondsForm is a duration written as plain seconds (`2s`, `2.35s`), which an edit keeps.
	secondsForm = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?s$`)
)

// parseMilliseconds reads a duration spelled as a decimal integer of milliseconds.
func parseMilliseconds(value string) (time.Duration, error) {
	spellingError := fmt.Errorf("want a whole number of milliseconds such as 1850, got %q", value)
	if !wholeNumber.MatchString(value) {
		return 0, spellingError
	}
	count, err := strconv.ParseInt(value, 10, 64)
	if err != nil || count > maxMilliseconds {
		return 0, spellingError
	}
	return time.Duration(count) * time.Millisecond, nil
}

// jsonNull is the JSON null literal.
const jsonNull = "null"

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

// decodeCount reads a frame field typed as a whole JSON number.
func decodeCount(value json.RawMessage) (int, error) {
	var count int
	err := json.Unmarshal(value, &count)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("want a whole number, got %s", value)
	}
	return count, nil
}

// decodeBool reads cut typed as a JSON bool.
func decodeBool(value json.RawMessage) (bool, error) {
	var isOn bool
	if err := json.Unmarshal(value, &isOn); err != nil {
		return false, fmt.Errorf("want true or false, got %s", value)
	}
	return isOn, nil
}

// decodeFactor reads a zoom factor typed as a JSON number, 0 for auto.
func decodeFactor(value json.RawMessage) (float64, error) {
	var factor float64
	err := json.Unmarshal(value, &factor)
	if err != nil || factor < 0 {
		return 0, fmt.Errorf("want a number above 0, or 0 for auto, got %s", value)
	}
	return factor, nil
}

// decodeTarget reads a target typed as a pick answers it, {"text", "nth", "area"}, refusing any
// other key.
func decodeTarget(value json.RawMessage) (storyboard.Target, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire pickTarget
	if err := decoder.Decode(&wire); err != nil {
		return storyboard.Target{}, fmt.Errorf(
			`want a target such as {"text": "ok", "nth": 2, "area": "list"}, got %s: %w`, value, err)
	}
	return wire.target(), nil
}

// wireMoment is a moment as an edit types it, {"beat": 3, "offset_ms": 200}: beat 0 is a clip
// time, offset_ms after the clip's first frame; a beat id from 1 is that beat's start plus
// offset_ms, which may be negative. Both keys are required.
type wireMoment struct {
	Beat     *int   `json:"beat"`
	OffsetMS *int64 `json:"offset_ms"`
}

// decodeMoment reads a moment typed as {"beat", "offset_ms"}, refusing any other key, a missing
// one, a negative beat, a negative clip time and an offset no time.Duration holds.
func decodeMoment(value json.RawMessage) (storyboard.Moment, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireMoment
	err := decoder.Decode(&wire)
	isWhole := err == nil && wire.Beat != nil && wire.OffsetMS != nil
	if !isWhole || *wire.Beat < 0 || (*wire.Beat == 0 && *wire.OffsetMS < 0) ||
		*wire.OffsetMS > maxMilliseconds || *wire.OffsetMS < -maxMilliseconds {
		return storyboard.Moment{}, fmt.Errorf(
			`want a moment such as {"beat": 3, "offset_ms": 200}, or beat 0 for a clip time from 0, got %s`, value)
	}
	offset := time.Duration(*wire.OffsetMS) * time.Millisecond
	return storyboard.Moment{Beat: *wire.Beat, Offset: offset}, nil
}

// wirePoint is a pan's destination as an edit types it: a target as a pick answers it,
// {"text", "nth", "area"}, or a cell, {"cell": {"x": 3, "y": 4}} — one of the two.
type wirePoint struct {
	pickTarget
	Cell *CellState `json:"cell"`
}

// decodePoint reads a pan's target or cell, refusing any other key and both at once, and spells
// it as the YAML flow mapping a camera.to writes: a target as [targetFlow] does, a cell as
// {cell: {x: 3, y: 4}}.
func decodePoint(value json.RawMessage) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wirePoint
	err := decoder.Decode(&wire)
	if err == nil && wire.Cell != nil && wire.pickTarget != (pickTarget{}) {
		err = errors.New("a target and a cell at once")
	}
	if err != nil {
		return "", fmt.Errorf(`want a target such as {"text": "ok", "nth": 2, "area": "list"} `+
			`or a cell such as {"cell": {"x": 3, "y": 4}}, got %s: %w`, value, err)
	}
	if wire.Cell != nil {
		cell := "{x: " + strconv.Itoa(wire.Cell.X) + ", y: " + strconv.Itoa(wire.Cell.Y) + "}"
		return "{cell: " + cell + "}", nil
	}
	return targetFlow(wire.target()), nil
}

// decodeText reads a camera or caption field's text, typed as a JSON string.
func decodeText(value json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", fmt.Errorf("want the field's text as a JSON string, got %s", value)
	}
	return text, nil
}

// parseMoment checks that value is a moment as one line of YAML — a {beat, offset} flow mapping
// or a clip time, with no comment — and returns it trimmed with the moment it reads as.
func parseMoment(value string) (string, storyboard.Moment, error) {
	trimmed := strings.TrimSpace(value)
	spellingError := fmt.Errorf("want a moment as YAML such as {beat: 3, offset: 200ms} or 4500ms, got %q", value)
	node, isOneLine := oneLineNode(trimmed)
	if !isOneLine {
		return "", storyboard.Moment{}, spellingError
	}
	isFlowMapping := node.Kind == yaml.MappingNode && node.Style&yaml.FlowStyle != 0
	isLineScalar := node.Kind == yaml.ScalarNode && node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) == 0
	if !isFlowMapping && !isLineScalar {
		return "", storyboard.Moment{}, spellingError
	}

	var moment storyboard.Moment
	if err := node.Decode(&moment); err != nil {
		return "", storyboard.Moment{}, fmt.Errorf("%w: %v", spellingError, err)
	}
	return trimmed, moment, nil
}

// oneLineNode parses text, which must hold no line break, as YAML and returns its one node;
// isOneLine is false when text spans lines, is not YAML, holds nothing, or carries a comment.
func oneLineNode(text string) (node *yaml.Node, isOneLine bool) {
	if strings.ContainsAny(text, "\r\n") {
		return nil, false
	}
	var file yaml.Node
	if err := yaml.Unmarshal([]byte(text), &file); err != nil || len(file.Content) == 0 {
		return nil, false
	}
	node = file.Content[0]
	for _, holder := range []*yaml.Node{&file, node} {
		if holder.HeadComment != "" || holder.LineComment != "" || holder.FootComment != "" {
			return nil, false
		}
	}
	return node, true
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
