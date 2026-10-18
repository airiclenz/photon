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

	"github.com/airiclenz/photon/internal/storyboard"
)

// maxMilliseconds is the longest duration in milliseconds a time.Duration holds.
const maxMilliseconds = math.MaxInt64 / int64(time.Millisecond)

// millisecondsPerSecond splits a duration into whole seconds and the milliseconds past them.
const millisecondsPerSecond = int64(time.Second / time.Millisecond)

// secondsForm is a duration written as plain seconds (`2s`, `2.35s`), which an edit keeps.
var secondsForm = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?s$`)

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
// it as the YAML flow mapping a camera.to writes: a target as [storyboard.Target.YAMLFlow] does, a cell as
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
	return wire.target().YAMLFlow(), nil
}

// decodeText reads a caption's text, typed as a JSON string.
func decodeText(value json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", fmt.Errorf("want the field's text as a JSON string, got %s", value)
	}
	return text, nil
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
