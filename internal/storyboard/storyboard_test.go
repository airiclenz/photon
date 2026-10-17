package storyboard

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// validBase is a small storyboard that breaks no rule. Each row of TestLoadRejects swaps one
// fragment of it, which must occur in it exactly once, to break one rule.
const validBase = `clip: base
ship: base.gif
frame: {cols: 80, rows: 24, padding: 16, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 24, max_colors: 128}
expect:
  - seen: 'done'
beats:
  - id: 1
    title: open
    duration: 3s
    hold: 1s
    zoom: {target: {text: 'Item', nth: 1, area: list}, factor: 2, in: 1s, out: 1s}
    do:
      - type: {text: 'hi'}
      - key: {name: enter, repeat: 1}
      - click: {target: {text: 'Item', area: list}, times: 1}
      - wait: {screen: 'Ready', timeout: 5s}
      - pause: {for: 1s}
      - drag: {from: {text: 'Grip'}, to: {offset: {x: 4}}, steps: 2}
    expect:
      - seen: 'Item'
        area: list
      - run: 'true'
  - id: 2
    title: next
    duration: 2s
`

// writeStoryboard writes body to a storyboard file in a fresh temp dir and returns its path.
func writeStoryboard(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "board.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write storyboard: %v", err)
	}
	return path
}

func TestLoadEveryForm(t *testing.T) {
	t.Parallel()

	board, err := Load(filepath.Join("testdata", "every-form.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if want := filepath.Join("testdata", "out", "every-form.gif"); board.Ship != want {
		t.Errorf("ship: want %q, got %q", want, board.Ship)
	}
	wantFrame := Frame{Cols: 80, Rows: 24, Padding: 16, FontSize: 14, LineHeight: 1.2, Scale: 2, Width: 600, FPS: 24, MaxColors: 128}
	if board.Frame != wantFrame {
		t.Errorf("frame: want %+v, got %+v", wantFrame, board.Frame)
	}
	wantTakeExpects := []Expect{{Seen: "Goodbye"}, {Run: `test -f "$PHOTON_WORK/saved.txt"`}}
	if !slices.Equal(board.Expect, wantTakeExpects) {
		t.Errorf("take expects: want %+v, got %+v", wantTakeExpects, board.Expect)
	}
	if got := len(board.Beats); got != 4 {
		t.Fatalf("beats: want 4, got %d", got)
	}

	open, pick, note, quit := board.Beats[0], board.Beats[1], board.Beats[2], board.Beats[3]
	if open.Hold != 0 || open.Duration != 2*time.Second || open.Why == "" || open.Notes == "" {
		t.Errorf("beat 1: want 2s, no hold, why and notes, got %+v", open)
	}
	if pick.Hold != time.Second {
		t.Errorf("beat 2 hold: want 1s, got %s", pick.Hold)
	}
	wantZoom := Zoom{Target: Target{Text: `Item \d`, Nth: NthLast, Area: "list"}, Factor: 1.5, In: DefaultZoomRamp, Out: DefaultZoomRamp}
	if pick.Zoom == nil || *pick.Zoom != wantZoom {
		t.Errorf("beat 2 zoom: want %+v with default ramps, got %+v", wantZoom, pick.Zoom)
	}
	wantClicks := []ClickAction{
		{Target: Target{Text: "Item 2", Nth: NthFirst, Area: "list"}, Times: 2},
		{Target: Target{Text: "Item", Nth: 3, Area: AreaAny}},
		{Target: Target{Text: "Save"}},
	}
	for index, want := range wantClicks {
		if click := pick.Do[index].Click; click == nil || *click != want {
			t.Errorf("beat 2 do[%d]: want click %+v, got %+v", index, want, pick.Do[index])
		}
	}
	wantDrags := []struct {
		drag  DragAction
		steps int
	}{
		{DragAction{
			From: Target{Text: "Item 1", Area: "list"}, To: DragEnd{Target: Target{Text: "Item 3", Nth: NthLast, Area: "list"}},
			Steps: new(4),
		}, 4},
		{DragAction{From: Target{Text: "Item 2"}, To: DragEnd{Offset: &Offset{X: -3, Y: 1}}}, DefaultDragSteps},
	}
	for offset, want := range wantDrags {
		index := len(wantClicks) + offset
		if drag := pick.Do[index].Drag; drag == nil || !reflect.DeepEqual(*drag, want.drag) || drag.StepCount() != want.steps {
			t.Errorf("beat 2 do[%d]: want drag %+v with %d steps, got %+v", index, want.drag, want.steps, pick.Do[index])
		}
	}
	wantPickExpects := []Expect{{Seen: "Item 2 selected", Area: "status"}, {Run: `grep -q 'Item 2' "$PHOTON_WORK/saved.txt"`}}
	if !slices.Equal(pick.Expect, wantPickExpects) {
		t.Errorf("beat 2 expects: want %+v, got %+v", wantPickExpects, pick.Expect)
	}

	if note.Zoom == nil || note.Zoom.Factor != 0 || note.Zoom.In != 500*time.Millisecond || note.Zoom.Out != 250*time.Millisecond {
		t.Errorf("beat 3 zoom: want an auto-fit zoom with 500ms and 250ms ramps, got %+v", note.Zoom)
	}
	actions := note.Do
	if typed := actions[0].Type; typed == nil || typed.Text != "hello there" || !typed.IsHumanized() {
		t.Errorf("beat 3 do[0]: want humanized typing by default, got %+v", actions[0])
	}
	if typed := actions[1].Type; typed == nil || typed.IsHumanized() {
		t.Errorf("beat 3 do[1]: want humanize: false honoured, got %+v", actions[1])
	}
	if key := actions[2].Key; key == nil || *key != (KeyAction{Name: "enter"}) {
		t.Errorf("beat 3 do[2]: want enter once, got %+v", actions[2])
	}
	if key := actions[3].Key; key == nil || *key != (KeyAction{Name: "down", Repeat: 3}) {
		t.Errorf("beat 3 do[3]: want down three times, got %+v", actions[3])
	}
	if wait := actions[4].Wait; wait == nil || *wait != (WaitAction{Screen: "Saving", Gone: true, Timeout: 3 * time.Minute}) {
		t.Errorf("beat 3 do[4]: want a gone wait with a 3m timeout, got %+v", actions[4])
	}
	if pause := actions[5].Pause; pause == nil || pause.For != 800*time.Millisecond {
		t.Errorf("beat 3 do[5]: want an 800ms pause, got %+v", actions[5])
	}
	if !slices.Equal(note.Expect, []Expect{{Seen: "Saved", Area: AreaAny}}) {
		t.Errorf("beat 3 expects: want a seen in any area, got %+v", note.Expect)
	}

	if !quit.Cut || quit.Duration != 0 || quit.Zoom != nil {
		t.Errorf("beat 4: want a cut beat without a duration, got %+v", quit)
	}
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	beatsSection := validBase[strings.Index(validBase, "beats:"):]

	cases := []struct {
		name     string
		old, new string
		want     string
		// decode marks a rule the strict decode enforces, which fails before validation runs and
		// so is not a *ValidationError.
		decode bool
	}{
		{"unknown header field", "clip: base", "clip: base\ntape: old.tape", "field tape not found", true},
		{"unknown beat field", "title: next", "title: next\n    anchor: top", "field anchor not found", true},
		{"unknown expect field", "run: 'true'", "run: 'true'\n        contains: x", "field contains not found", true},
		{"duration without a unit", "duration: 2s", "duration: 2", "into time.Duration", true},
		{"nth zero", "nth: 1,", "nth: 0,", `nth: want first, last or a positive integer, got "0"`, true},
		{"nth word", "nth: 1,", "nth: second,", `nth: want first, last or a positive integer, got "second"`, true},
		{"nth sequence", "nth: 1,", "nth: [1],", "nth: want first, last or a positive integer, got a sequence", true},
		{"clip missing", "clip: base\n", "", "clip: missing", false},
		{"ship missing", "ship: base.gif\n", "", "ship: missing", false},
		{"frame cols", "cols: 80", "cols: 0", "frame.cols: want ≥1, got 0", false},
		{"frame fps", "fps: 24", "fps: 0", "frame.fps: want ≥1, got 0", false},
		{"frame padding", "padding: 16", "padding: -1", "frame.padding: want ≥0, got -1", false},
		{"frame font size", "font_size: 14", "font_size: 0", "frame.font_size: want >0, got 0", false},
		{"frame line height", "line_height: 1.2", "line_height: -1", "frame.line_height: want >0, got -1", false},
		{"frame max colors", "max_colors: 128", "max_colors: 300", "frame.max_colors: want 2..256, got 300", false},
		{"no beats", beatsSection, "beats: []\n", "beats: none declared", false},
		{"id below one", "  - id: 2\n", "  - id: 0\n", "beat 0: id: want ≥1", false},
		{"id duplicate", "  - id: 2\n", "  - id: 1\n", "beat 1: id: duplicate", false},
		{"id not ascending", "  - id: 1\n", "  - id: 3\n", "beat 2: id: not ascending (follows 3)", false},
		{"title missing", "    title: next\n", "", "beat 2: title: missing", false},
		{"hold negative", "hold: 1s", "hold: -1s", "beat 1: hold: want ≥0, got -1s", false},
		{"duration missing", "    duration: 2s\n", "", "beat 2: duration: want >0, got 0s", false},
		{"hold not under duration", "hold: 1s", "hold: 3s", "beat 1: hold: want less than duration 3s, got 3s", false},
		{"zoom factor", "factor: 2", "factor: 4", "beat 1: zoom.factor: want in [1, 3], got 4", false},
		{"zoom in negative", "in: 1s, out: 1s", "in: -1s, out: 1s", "beat 1: zoom.in: want ≥0, got -1s", false},
		{"zoom out negative", "in: 1s, out: 1s", "in: 1s, out: -1s", "beat 1: zoom.out: want ≥0, got -1s", false},
		{"zoom target regex", "text: 'Item', nth: 1", "text: '(', nth: 1", "beat 1: zoom.target.text: error parsing regexp", false},
		{"two action forms", "      - pause: {for: 1s}", "      - pause: {for: 1s}\n        type: {text: x}",
			"beat 1: do[4]: want exactly one of type, key, click, drag, wait or pause", false},
		{"no action form", "      - pause: {for: 1s}", "      - {}",
			"beat 1: do[4]: want exactly one of type, key, click, drag, wait or pause", false},
		{"type text missing", "text: 'hi'", "text: ''", "beat 1: do[0]: type.text: missing", false},
		{"key name missing", "name: enter, repeat: 1", "repeat: 1", "beat 1: do[1]: key.name: missing", false},
		{"key name unknown", "name: enter, repeat: 1", "name: return, repeat: 1",
			`beat 1: do[1]: key.name: unknown key "return", want one of enter, esc,`, false},
		{"key repeat negative", "repeat: 1", "repeat: -1", "beat 1: do[1]: key.repeat: want ≥0, got -1", false},
		{"click times negative", "times: 1", "times: -1", "beat 1: do[2]: click.times: want ≥0, got -1", false},
		{"target text missing", "{text: 'Item', area: list}", "{area: list}", "beat 1: do[2]: click.target.text: missing", false},
		{"target area", "{text: 'Item', area: list}", "{text: 'Item', area: 'the list'}",
			`beat 1: do[2]: click.target.area: want an area name or any, got "the list"`, false},
		{"wait screen missing", "screen: 'Ready', ", "", "beat 1: do[3]: wait.screen: missing", false},
		{"wait timeout zero", "timeout: 5s", "timeout: 0s", "beat 1: do[3]: wait.timeout: want in (0, 3m0s], got 0s", false},
		{"wait timeout too long", "timeout: 5s", "timeout: 3m1s", "beat 1: do[3]: wait.timeout: want in (0, 3m0s], got 3m1s", false},
		{"pause not positive", "for: 1s", "for: 0s", "beat 1: do[4]: pause.for: want >0, got 0s", false},
		{"drag end with both forms", "offset: {x: 4}}", "text: 'Edge', offset: {x: 4}}",
			"beat 1: do[5]: drag.to: want exactly one of text or offset", false},
		{"drag end with neither form", "to: {offset: {x: 4}}", "to: {}", "beat 1: do[5]: drag.to: want exactly one of text or offset", false},
		{"drag offset zero", "{x: 4}", "{x: 0}", "beat 1: do[5]: drag.to.offset: want a non-zero x or y", false},
		{"drag offset with an nth", "to: {offset", "to: {nth: 2, offset", "beat 1: do[5]: drag.to.nth: only a to.text takes an nth", false},
		{"drag offset with an area", "to: {offset", "to: {area: list, offset",
			"beat 1: do[5]: drag.to.area: only a to.text takes an area", false},
		{"drag to regex", "to: {offset: {x: 4}}", "to: {text: '('}", "beat 1: do[5]: drag.to.text: error parsing regexp", false},
		{"drag from text missing", "from: {text: 'Grip'}", "from: {area: list}", "beat 1: do[5]: drag.from.text: missing", false},
		{"drag steps zero", "steps: 2", "steps: 0", "beat 1: do[5]: drag.steps: want ≥1, got 0", false},
		{"unknown offset field", "{x: 4}", "{x: 4, z: 1}", "field z not found", true},
		{"expect without a form", "- run: 'true'", "- area: list", "beat 1: expect[1]: want exactly one of seen or run", false},
		{"expect with both forms", "run: 'true'", "run: 'true'\n        seen: x", "beat 1: expect[1]: want exactly one of seen or run", false},
		{"seen regex", "- seen: 'Item'", "- seen: '('", "beat 1: expect[0]: seen: error parsing regexp", false},
		{"seen area", "        area: list\n", "        area: 9lives\n", `beat 1: expect[0]: area: want an area name or any, got "9lives"`, false},
		{"area on a run", "run: 'true'", "run: 'true'\n        area: list", "beat 1: expect[1]: area: only a seen expect takes an area", false},
		{"run blank", "run: 'true'", "run: '  '", `beat 1: expect[1]: run: want a command, got "  "`, false},
		{"take-level expect", "  - seen: 'done'", "  - seen: 'done'\n    run: 'true'", "\n  expect[0]: want exactly one of seen or run", false},
	}
	if _, err := Load(writeStoryboard(t, validBase)); err != nil {
		t.Fatalf("the base storyboard must load clean: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(validBase, tc.old); count != 1 {
				t.Fatalf("fragment %q occurs %d times in the base, want once", tc.old, count)
			}

			_, err := Load(writeStoryboard(t, strings.Replace(validBase, tc.old, tc.new, 1)))

			if err == nil {
				t.Fatalf("want an error containing %q, got none", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got:\n%v", tc.want, err)
			}
			var invalid *ValidationError
			isValidation := errors.As(err, &invalid)
			if tc.decode {
				if isValidation {
					t.Fatalf("want a decode error, got a *ValidationError:\n%v", err)
				}
				return
			}
			if !isValidation {
				t.Fatalf("want a *ValidationError, got %T: %v", err, err)
			}
			if got := len(invalid.Problems); got != 1 {
				t.Errorf("problems: want exactly the one broken rule, got %d:\n%v", got, err)
			}
		})
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	t.Parallel()
	path := writeStoryboard(t, `clip: many
frame: {cols: 80, rows: 24, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 24, max_colors: 1}
beats:
  - id: 1
    title: open
    duration: 1s
    do: [{wait: {screen: "x", timeout: 0s}}, {key: {name: hyper}}]
    expect: [{run: ""}]
`)

	_, err := Load(path)

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want a *ValidationError, got %v", err)
	}
	want := []string{
		"ship: missing",
		"frame.max_colors: want 2..256, got 1",
		"beat 1: do[0]: wait.timeout",
		`beat 1: do[1]: key.name: unknown key "hyper"`,
		"beat 1: expect[0]: want exactly one of seen or run",
	}
	if got := len(invalid.Problems); got != len(want) {
		t.Fatalf("problems: want %d, got %d:\n%v", len(want), got, err)
	}
	for index, problem := range invalid.Problems {
		if !strings.Contains(problem.Error(), want[index]) {
			t.Errorf("problem %d: want %q, got %q", index, want[index], problem)
		}
	}
	if !strings.HasPrefix(err.Error(), path+": 5 problem(s)\n") {
		t.Errorf("want the message headed by the path and the count, got:\n%v", err)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	t.Parallel()

	_, err := Load(writeStoryboard(t, ""))

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want a *ValidationError, got %v", err)
	}
	for _, want := range []string{"clip: missing", "ship: missing", "frame.cols", "beats: none declared"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("want a problem containing %q, got:\n%v", want, err)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()

	_, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))

	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want an error wrapping fs.ErrNotExist, got %v", err)
	}
}

func TestLoadResolvesShip(t *testing.T) {
	t.Parallel()
	absolute := filepath.Join(t.TempDir(), "shipped.gif")

	cases := []struct {
		name string
		ship string
		want func(dir string) string
	}{
		{"relative to the storyboard", "out/clip.gif", func(dir string) string { return filepath.Join(dir, "out", "clip.gif") }},
		{"absolute kept", absolute, func(string) string { return absolute }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := writeStoryboard(t, strings.Replace(validBase, "ship: base.gif", "ship: '"+tc.ship+"'", 1))

			board, err := Load(path)

			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if want := tc.want(filepath.Dir(path)); board.Ship != want {
				t.Errorf("ship: want %q, got %q", want, board.Ship)
			}
		})
	}
}
