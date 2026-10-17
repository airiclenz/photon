package storyboard

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/airiclenz/photon/internal/keys"
)

// reporter collects one validation problem.
type reporter func(format string, args ...any)

// prefixed returns a reporter that files every problem under prefix.
func prefixed(report reporter, prefix string) reporter {
	return func(format string, args ...any) {
		report(prefix+format, args...)
	}
}

// Palette size bounds: a GIF palette holds at most 256 colours, and fewer than 2 draws nothing.
const (
	minMaxColors = 2
	maxMaxColors = 256
)

// Zoom factor bounds: 1 is no magnification, beyond 3 the terminal text breaks into pixels.
const (
	minZoomFactor = 1
	maxZoomFactor = 3
)

// areaName is the spelling of an area name. Whether the config declares the area is checked
// where the storyboard and the config meet, not here.
var areaName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// validate applies every schema rule and returns the ones the storyboard breaks.
func (s *Storyboard) validate() []error {
	var problems []error
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	s.validateHeader(report)
	for index, expect := range s.Expect {
		expect.validate(prefixed(report, fmt.Sprintf("expect[%d]: ", index)))
	}
	if len(s.Beats) == 0 {
		report("beats: none declared")
		return problems
	}
	s.validateIDs(report)
	for _, beat := range s.Beats {
		beat.validate(prefixed(report, fmt.Sprintf("beat %d: ", beat.ID)))
	}
	return problems
}

// validateHeader checks the top-level scalars: clip, ship and frame.
func (s *Storyboard) validateHeader(report reporter) {
	for _, required := range []struct{ name, value string }{{"clip", s.Clip}, {"ship", s.Ship}} {
		if required.value == "" {
			report("%s: missing", required.name)
		}
	}
	frame := s.Frame
	for _, positive := range []struct {
		name  string
		value int
	}{
		{"cols", frame.Cols}, {"rows", frame.Rows}, {"scale", frame.Scale},
		{"width", frame.Width}, {"fps", frame.FPS},
	} {
		if positive.value < 1 {
			report("frame.%s: want ≥1, got %d", positive.name, positive.value)
		}
	}
	if frame.Padding < 0 {
		report("frame.padding: want ≥0, got %d", frame.Padding)
	}
	if frame.FontSize <= 0 {
		report("frame.font_size: want >0, got %g", frame.FontSize)
	}
	if frame.LineHeight <= 0 {
		report("frame.line_height: want >0, got %g", frame.LineHeight)
	}
	if frame.MaxColors < minMaxColors || frame.MaxColors > maxMaxColors {
		report("frame.max_colors: want %d..%d, got %d", minMaxColors, maxMaxColors, frame.MaxColors)
	}
}

// validateIDs checks that beat ids are positive, unique and ascending.
func (s *Storyboard) validateIDs(report reporter) {
	seen := make(map[int]bool, len(s.Beats))
	previous := 0
	for _, beat := range s.Beats {
		switch {
		case beat.ID < 1:
			report("beat %d: id: want ≥1", beat.ID)
		case seen[beat.ID]:
			report("beat %d: id: duplicate", beat.ID)
		case beat.ID < previous:
			report("beat %d: id: not ascending (follows %d)", beat.ID, previous)
		}
		seen[beat.ID] = true
		previous = beat.ID
	}
}

// validate applies the per-beat rules: the title, the section timing, the zoom, the actions and
// the expects.
func (b Beat) validate(report reporter) {
	if b.Title == "" {
		report("title: missing")
	}
	if b.Hold < 0 {
		report("hold: want ≥0, got %s", b.Hold)
	}
	if !b.Cut {
		switch {
		case b.Duration <= 0:
			report("duration: want >0, got %s", b.Duration)
		case b.Hold >= b.Duration:
			report("hold: want less than duration %s, got %s", b.Duration, b.Hold)
		}
	}
	if b.Zoom != nil {
		b.Zoom.validate(prefixed(report, "zoom."))
	}
	for index, action := range b.Do {
		action.validate(prefixed(report, fmt.Sprintf("do[%d]: ", index)))
	}
	for index, expect := range b.Expect {
		expect.validate(prefixed(report, fmt.Sprintf("expect[%d]: ", index)))
	}
}

// validate checks the zoom's target, factor and ramps.
func (z Zoom) validate(report reporter) {
	z.Target.validate(prefixed(report, "target."))
	// An absent factor is auto-fit by the compositor.
	if z.Factor != 0 && (z.Factor < minZoomFactor || z.Factor > maxZoomFactor) {
		report("factor: want in [%d, %d], got %g", minZoomFactor, maxZoomFactor, z.Factor)
	}
	if z.In < 0 {
		report("in: want ≥0, got %s", z.In)
	}
	if z.Out < 0 {
		report("out: want ≥0, got %s", z.Out)
	}
}

// validate checks that exactly one action form is set and that its fields fit that form.
func (a Action) validate(report reporter) {
	forms := 0
	for _, isSet := range []bool{a.Type != nil, a.Key != nil, a.Click != nil, a.Drag != nil, a.Wait != nil, a.Pause != nil} {
		if isSet {
			forms++
		}
	}
	if forms != 1 {
		report("want exactly one of type, key, click, drag, wait or pause")
		return
	}
	switch {
	case a.Type != nil:
		if a.Type.Text == "" {
			report("type.text: missing")
		}
	case a.Key != nil:
		a.Key.validate(prefixed(report, "key."))
	case a.Click != nil:
		a.Click.Target.validate(prefixed(report, "click.target."))
		if a.Click.Times < 0 {
			report("click.times: want ≥0, got %d", a.Click.Times)
		}
	case a.Drag != nil:
		a.Drag.validate(prefixed(report, "drag."))
	case a.Wait != nil:
		a.Wait.validate(prefixed(report, "wait."))
	case a.Pause != nil:
		if a.Pause.For <= 0 {
			report("pause.for: want >0, got %s", a.Pause.For)
		}
	}
}

// validate checks that the key is one the keys package can send and that its repeat count is
// not negative.
func (k KeyAction) validate(report reporter) {
	switch _, known := keys.Lookup(k.Name); {
	case k.Name == "":
		report("name: missing")
	case !known:
		report("name: unknown key %q, want one of %s", k.Name, strings.Join(keys.Names(), ", "))
	}
	if k.Repeat < 0 {
		report("repeat: want ≥0, got %d", k.Repeat)
	}
}

// validate checks the drag's from target, its to end and that an explicit steps is at least 1.
func (d DragAction) validate(report reporter) {
	d.From.validate(prefixed(report, "from."))
	d.To.validate(report)
	if d.Steps != nil && *d.Steps < 1 {
		report("steps: want ≥1, got %d", *d.Steps)
	}
}

// validate checks that the drag end sets exactly one of text or offset, then the target's fields
// or the offset's. An offset is relative to the press cell, so a target's nth and area do not
// apply to it. Problems are filed under to, as the drag's own field.
func (e DragEnd) validate(report reporter) {
	if (e.Text == "") == (e.Offset == nil) {
		report("to: want exactly one of text or offset")
		return
	}
	if e.Offset == nil {
		e.Target.validate(prefixed(report, "to."))
		return
	}
	if *e.Offset == (Offset{}) {
		report("to.offset: want a non-zero x or y")
	}
	if e.Nth != 0 {
		report("to.nth: only a to.text takes an nth")
	}
	if e.Area != "" {
		report("to.area: only a to.text takes an area")
	}
}

// validate checks the wait's regex and that its timeout is set and bounded.
func (w WaitAction) validate(report reporter) {
	validateRegex("screen", w.Screen, report)
	if w.Timeout <= 0 || w.Timeout > MaxWaitTimeout {
		report("timeout: want in (0, %s], got %s", MaxWaitTimeout, w.Timeout)
	}
}

// validate checks the target's regex and area.
func (t Target) validate(report reporter) {
	validateRegex("text", t.Text, report)
	validateArea(t.Area, report)
}

// validate checks that the expect sets exactly one of seen or run, that a seen regex compiles,
// that an area goes with a seen and that a run names a command.
func (e Expect) validate(report reporter) {
	if (e.Seen == "") == (e.Run == "") {
		report("want exactly one of seen or run")
		return
	}
	if e.Seen != "" {
		validateRegex("seen", e.Seen, report)
		validateArea(e.Area, report)
		return
	}
	if e.Area != "" {
		report("area: only a seen expect takes an area")
	}
	if strings.TrimSpace(e.Run) == "" {
		report("run: want a command, got %q", e.Run)
	}
}

// validateRegex reports a missing or non-compiling regex under the field's name.
func validateRegex(field, pattern string, report reporter) {
	if pattern == "" {
		report("%s: missing", field)
		return
	}
	if _, err := regexp.Compile(pattern); err != nil {
		report("%s: %v", field, err)
	}
}

// validateArea reports an area name that is neither empty, [AreaAny] nor a well-formed name.
func validateArea(area string, report reporter) {
	if area == "" || area == AreaAny || areaName.MatchString(area) {
		return
	}
	report("area: want an area name or %s, got %q", AreaAny, area)
}
