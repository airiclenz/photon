package tune

import (
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
	// decimal is the spelling of a zoom factor.
	decimal = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
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

// parseCount reads a frame field spelled as a decimal integer.
func parseCount(value string) (int, error) {
	spellingError := fmt.Errorf("want a decimal integer, got %q", value)
	if !wholeNumber.MatchString(value) {
		return 0, spellingError
	}
	count, err := strconv.Atoi(value)
	if err != nil {
		return 0, spellingError
	}
	return count, nil
}

// parseTarget checks that value is a target as one line of YAML flow text — a flow mapping of
// a target's own keys, with no comment — and returns it trimmed.
func parseTarget(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	spellingError := fmt.Errorf("want a target as YAML flow text such as {text: 'ok', area: list}, got %q", value)
	isBraced := strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")
	if !isBraced || strings.ContainsAny(trimmed, "\r\n") {
		return "", spellingError
	}
	var file yaml.Node
	if err := yaml.Unmarshal([]byte(trimmed), &file); err != nil || len(file.Content) == 0 {
		return "", spellingError
	}
	mapping := file.Content[0]
	hasComment := mapping.HeadComment != "" || mapping.LineComment != "" || mapping.FootComment != ""
	if mapping.Kind != yaml.MappingNode || mapping.Style&yaml.FlowStyle == 0 || hasComment {
		return "", spellingError
	}

	decoder := yaml.NewDecoder(strings.NewReader(trimmed))
	decoder.KnownFields(true)
	var target storyboard.Target
	if err := decoder.Decode(&target); err != nil {
		return "", fmt.Errorf("%w: %v", spellingError, err)
	}
	return trimmed, nil
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
