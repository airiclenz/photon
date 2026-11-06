package storyboard

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/airiclenz/photon/internal/keys"
)

// reporter collects one validation problem.
type reporter func(format string, args ...any)

// collector gathers finished validation problems.
type collector func(problem error)

// unlocated returns a reporter that files every problem as phrased, on the clip as a whole.
func (c collector) unlocated() reporter {
	return func(format string, args ...any) {
		c(fmt.Errorf(format, args...))
	}
}

// at returns a reporter that files every problem under owner, as a *[LocatedError] whose own
// message carries the owner's prefix.
func (c collector) at(owner Owner) reporter {
	return func(format string, args ...any) {
		c(&LocatedError{Owner: owner, Err: fmt.Errorf(format, args...)})
	}
}

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

// validate applies every schema rule and returns the ones the storyboard breaks. On the way it
// keeps what the caption and camera rules resolve: their places on the output clock and the
// captions' colours.
func (s *Storyboard) validate() []error {
	var problems []error
	collect := collector(func(problem error) { problems = append(problems, problem) })
	report := collect.unlocated()

	s.validateHeader(report)
	for index, expect := range s.Expect {
		expect.validate(prefixed(report, fmt.Sprintf("expect[%d]: ", index)))
	}
	if len(s.Beats) == 0 {
		report("beats: none declared")
		return problems
	}
	if !slices.ContainsFunc(s.Beats, func(beat Beat) bool { return !beat.Cut }) {
		report("beats: every beat is cut")
	}
	s.validateIDs(collect)
	for _, beat := range s.Beats {
		beat.validate(collect.at(Owner{Kind: OwnerBeat, Index: beat.ID}))
	}
	s.Camera.validate(s.Frame, s.Clock(), collect)
	s.validateCaptions(collect)
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
	positives := []frameCount{{"scale", frame.Scale}, {"width", frame.Width}, {"fps", frame.FPS}}
	if !frame.HasViewport() {
		positives = append([]frameCount{{"cols", frame.Cols}, {"rows", frame.Rows}}, positives...)
	}
	for _, positive := range positives {
		if positive.value < 1 {
			report("frame.%s: want ≥1, got %d", positive.name, positive.value)
		}
	}
	if frame.HasViewport() {
		frame.validatePage(report)
	} else {
		frame.validateTerminal(report)
	}
	if frame.DeviceScale != 0 &&
		(frame.DeviceScale < DefaultDeviceScale || frame.DeviceScale > maxDeviceScale) {
		report("frame.device_scale: want in [%d, %d], got %g",
			DefaultDeviceScale, maxDeviceScale, frame.DeviceScale)
	}
	if frame.MaxColors < minMaxColors || frame.MaxColors > maxMaxColors {
		report("frame.max_colors: want %d..%d, got %d", minMaxColors, maxMaxColors, frame.MaxColors)
	}
}

// frameCount is one whole-number frame key with its value.
type frameCount struct {
	name  string
	value int
}

// validateTerminal checks the terminal keys of a frame without a viewport beyond its cols and
// rows: its padding, font size and line height.
func (f Frame) validateTerminal(report reporter) {
	if f.Padding < 0 {
		report("frame.padding: want ≥0, got %d", f.Padding)
	}
	switch {
	case f.FontSize <= 0:
		report("frame.font_size: want >0, got %g", f.FontSize)
	case f.FontSize < minCaptionSize || f.FontSize > maxCaptionSize:
		report("frame.font_size: want in [%d, %d], got %g",
			minCaptionSize, maxCaptionSize, f.FontSize)
	}
	if f.LineHeight <= 0 {
		report("frame.line_height: want >0, got %g", f.LineHeight)
	}
}

// validatePage checks the page grid of a frame with a viewport: the viewport's width and height,
// and that none of the terminal's keys is set beside it.
func (f Frame) validatePage(report reporter) {
	if f.Viewport.Width < 1 {
		report("frame.viewport.width: want ≥1, got %d", f.Viewport.Width)
	}
	if f.Viewport.Height < 1 {
		report("frame.viewport.height: want ≥1, got %d", f.Viewport.Height)
	}
	for _, terminal := range []struct {
		name  string
		isSet bool
	}{
		{"cols", f.Cols != 0}, {"rows", f.Rows != 0}, {"padding", f.Padding != 0},
		{"font_size", f.FontSize != 0}, {"line_height", f.LineHeight != 0},
	} {
		if terminal.isSet {
			report("frame.%s: a frame with a viewport takes no %s", terminal.name, terminal.name)
		}
	}
}

// validateIDs checks that beat ids are positive, unique and ascending. Problems are filed under
// the beat.
func (s *Storyboard) validateIDs(collect collector) {
	seen := make(map[int]bool, len(s.Beats))
	previous := 0
	for _, beat := range s.Beats {
		report := collect.at(Owner{Kind: OwnerBeat, Index: beat.ID})
		switch {
		case beat.ID < 1:
			report("id: want ≥1")
		case seen[beat.ID]:
			report("id: duplicate")
		case beat.ID < previous:
			report("id: not ascending (follows %d)", previous)
		}
		seen[beat.ID] = true
		previous = beat.ID
	}
}

// validate applies the per-beat rules: the title, the section timing, the removed zoom, the
// actions and the expects.
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
	if b.RemovedZoom {
		report("zoom: removed; zoom with camera.zooms and aim with camera.positions")
	}
	for index, action := range b.Do {
		action.validate(prefixed(report, fmt.Sprintf("do[%d]: ", index)))
	}
	for index, expect := range b.Expect {
		expect.validate(prefixed(report, fmt.Sprintf("expect[%d]: ", index)))
	}
}

// validate checks that exactly one action form is set and that its fields fit that form.
func (a Action) validate(report reporter) {
	forms := 0
	formsSet := []bool{
		a.Type != nil, a.Key != nil, a.Click != nil, a.Drag != nil, a.Wait != nil, a.Pause != nil,
	}
	for _, isSet := range formsSet {
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
		a.Click.Press.validate(prefixed(report, "click."))
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

// validate checks the drag's from target, each via point, its to end, that an explicit steps is
// at least 1, and its button and modifiers.
func (d DragAction) validate(report reporter) {
	d.From.validate(prefixed(report, "from."))
	for index, via := range d.Via {
		via.validate(fmt.Sprintf("via[%d]", index), report)
	}
	d.To.validate("to", report)
	if d.Steps != nil && *d.Steps < 1 {
		report("steps: want ≥1, got %d", *d.Steps)
	}
	d.Press.validate(report)
}

// validate checks that the drag point sets exactly one of text, selector or offset, then the
// target's fields or the offset's. An offset is relative to the drag's previous point, so a
// target's nth and area do not apply to it. Problems are filed under field, the point's own field
// of the drag (`to`, `via[1]`).
func (e DragEnd) validate(field string, report reporter) {
	if e.Target.isSet() == (e.Offset != nil) {
		if e.Selector == "" {
			report("%s: want exactly one of text or offset", field)
		} else {
			report("%s: want exactly one of text, selector or offset", field)
		}
		return
	}
	if e.Offset == nil {
		e.Target.validate(prefixed(report, field+"."))
		return
	}
	if *e.Offset == (Offset{}) {
		report("%s.offset: want a non-zero x or y", field)
	}
	if e.Nth != 0 {
		report("%s.nth: only a %s.text takes an nth", field, field)
	}
	if e.Area != "" {
		report("%s.area: only a %s.text takes an area", field, field)
	}
}

// validate checks that the button and the modifiers are spellings the keys package accepts.
func (p Press) validate(report reporter) {
	if _, err := keys.ParseButton(p.Button); err != nil {
		report("button: %v", err)
	}
	if _, err := keys.ParseMods(p.With); err != nil {
		report("with: %v", err)
	}
}

// validate checks the wait's regex and that its timeout is set and bounded.
func (w WaitAction) validate(report reporter) {
	validateRegex("screen", w.Screen, report)
	if w.Timeout <= 0 || w.Timeout > MaxWaitTimeout {
		report("timeout: want in (0, %s], got %s", MaxWaitTimeout, w.Timeout)
	}
}

// validate checks the target's regex, or its selector's shape, and its area. A target with
// neither is missing its text; one with both is one problem, filed under selector.
func (t Target) validate(report reporter) {
	switch {
	case t.Selector == "":
		validateRegex("text", t.Text, report)
	case t.Text != "":
		report("selector: want a text or a selector, not both")
	default:
		if err := CheckSelector(t.Selector); err != nil {
			report("selector: %v", err)
		}
	}
	validateArea(t.Area, report)
}

// isSet reports whether the target names a place: it has a text or a selector.
func (t Target) isSet() bool {
	return t.Text != "" || t.Selector != ""
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

// selectorClosers maps each bracket a CSS selector opens to the one that closes it.
var selectorClosers = map[rune]rune{'[': ']', '(': ')'}

// CheckSelector checks a CSS selector's shape, not its meaning: it is not blank, every `[` and
// `(` is closed by its own `]` or `)` in order, and every quote is closed. Brackets inside a
// quote and any character after a backslash are text. The problem it returns reads without a
// field name, so the caller files it under its own.
func CheckSelector(selector string) error {
	if strings.TrimSpace(selector) == "" {
		return fmt.Errorf("want a CSS selector, got %q", selector)
	}
	var open []rune
	var quote rune
	isEscaped := false
	for _, char := range selector {
		switch {
		case isEscaped:
			isEscaped = false
		case char == '\\':
			isEscaped = true
		case quote != 0:
			if char == quote {
				quote = 0
			}
		case char == '\'' || char == '"':
			quote = char
		case selectorClosers[char] != 0:
			open = append(open, char)
		case char == ']' || char == ')':
			if len(open) == 0 || selectorClosers[open[len(open)-1]] != char {
				return fmt.Errorf("unbalanced %q in %q", char, selector)
			}
			open = open[:len(open)-1]
		}
	}
	if quote != 0 {
		return fmt.Errorf("unclosed %q in %q", quote, selector)
	}
	if len(open) > 0 {
		return fmt.Errorf("unclosed %q in %q", open[len(open)-1], selector)
	}
	return nil
}
