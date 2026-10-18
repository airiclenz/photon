package storyboard

import (
	"errors"
	"fmt"
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

	// Beats 1, 2 and 3 start at 0s, 2s and 6s on the output clock; beat 4 is cut.
	wantCamera := []CameraMove{
		{Zoom: &ZoomMove{
			To: 1.5, At: Moment{Offset: 200 * time.Millisecond}, Over: 600 * time.Millisecond, Ease: EaseLinear,
			start: 200 * time.Millisecond,
		}},
		{Zoom: &ZoomMove{
			To: 1, At: Moment{Beat: 1, Offset: time.Second}, Over: 500 * time.Millisecond, Ease: EaseSmooth,
			start: time.Second,
		}},
		{Pan: &PanMove{
			To: &Point{Cell: &Cell{X: 40, Y: 12}}, At: Moment{Beat: 1, Offset: 100 * time.Millisecond},
			Over: 500 * time.Millisecond, Ease: EaseSmooth, start: 100 * time.Millisecond,
		}},
		{Pan: &PanMove{
			To: &Point{Target: Target{Text: "Item 1", Nth: NthLast, Area: "list"}}, At: Moment{Offset: 600 * time.Millisecond},
			Over: 400 * time.Millisecond, Ease: EaseSmooth, start: 600 * time.Millisecond,
		}},
		{Pan: &PanMove{
			Via: []Point{{Target: Target{Text: "Items", Nth: NthFirst, Area: AreaAny}}, {Cell: &Cell{X: 10, Y: 5}}},
			At:  Moment{Offset: time.Second}, Over: 800 * time.Millisecond, Ease: EaseSmooth, start: time.Second,
		}},
	}
	if !reflect.DeepEqual(board.Camera, wantCamera) {
		t.Errorf("camera: want %+v with smooth by default and resolved starts, got %+v", wantCamera, board.Camera)
	}

	wantCaptions := []Caption{
		{
			Text: "Pick an item\nthen save it", At: Moment{Beat: 2, Offset: 200 * time.Millisecond}, Until: &Moment{Beat: 3},
			Font: CaptionFont{Size: 18, Weight: WeightBold}, Place: PlaceTop, Align: AlignLeft, Offset: Pixels{X: 8, Y: -4},
			Color: "#FFCC00", Box: CaptionBox{Color: "#202020", Opacity: new(0.0)},
			Fade:  Fade{In: 300 * time.Millisecond, Out: 200 * time.Millisecond},
			start: 2200 * time.Millisecond, stop: 6 * time.Second,
			textColor: RGB{R: 0xff, G: 0xcc}, boxColor: RGB{R: 0x20, G: 0x20, B: 0x20},
		},
		{
			Text: "Saved", At: Moment{Offset: 9 * time.Second}, For: 2 * time.Second,
			Font: CaptionFont{Size: 14, Weight: WeightRegular}, Position: &Pixels{X: 40, Y: 300},
			Color: "#ffffff", Box: CaptionBox{Color: "#000000", Opacity: new(0.6)},
			start: 9 * time.Second, stop: 11 * time.Second, textColor: RGB{R: 0xff, G: 0xff, B: 0xff},
		},
		{
			Text: "Goodbye", At: Moment{Beat: 3, Offset: -time.Second}, For: time.Second,
			Font: CaptionFont{Size: 14, Weight: WeightRegular}, Place: PlaceBottom, Align: AlignMiddle,
			Color: "#ffffff", Box: CaptionBox{Color: "#000000", Opacity: new(0.6)},
			start: 5 * time.Second, stop: 6 * time.Second, textColor: RGB{R: 0xff, G: 0xff, B: 0xff},
		},
	}
	if !reflect.DeepEqual(board.Captions, wantCaptions) {
		t.Errorf("captions: want %+v with their defaults and resolved values, got %+v", wantCaptions, board.Captions)
	}
}

// cameraBase is validBase with a camera that breaks no rule: beat 1's zoom holds both tracks over
// [0s, 3s) of the 5s clip, so every move sits in beat 2's [3s, 5s). Each row of
// TestLoadRejects_Camera swaps one fragment of it, which must occur in it exactly once.
const cameraBase = validBase + `camera:
  - zoom: {to: 2, at: {beat: 2, offset: 100ms}, over: 500ms, ease: linear}
  - pan: {to: {cell: {x: 40, y: 12}}, at: 3s, over: 400ms}
  - pan: {via: [{text: 'Grip', area: list}, {cell: {x: 2, y: 3}}], at: 4s, over: 1s}
`

func TestLoadRejects_Camera(t *testing.T) {
	t.Parallel()
	const panToCell = "  - pan: {to: {cell: {x: 40, y: 12}}, at: 3s, over: 400ms}"

	cases := []struct {
		name     string
		old, new string
		want     string
		// decode marks a rule the strict decode enforces, which fails before validation runs.
		decode bool
	}{
		{"unknown move field", "ease: linear}", "ease: linear, speed: 2}", "field speed not found", true},
		{"unknown cell field", "{x: 40, y: 12}", "{x: 40, y: 12, z: 1}", "field z not found", true},
		{"both forms", "  - zoom: {to: 2,", "  - pan: {to: {cell: {x: 1, y: 1}}}\n    zoom: {to: 2,",
			"camera[0]: want exactly one of zoom or pan", false},
		{"no form", panToCell, "  - {}", "camera[1]: want exactly one of zoom or pan", false},
		{"zoom factor too large", "to: 2,", "to: 3.5,", "camera[0]: zoom.to: want in [1, 3], got 3.5", false},
		{"zoom factor missing", "to: 2, ", "", "camera[0]: zoom.to: want in [1, 3], got 0", false},
		{"length zero", "over: 500ms", "over: 0s", "camera[0]: zoom.over: want >0, got 0s", false},
		{"ease unknown", "ease: linear", "ease: bounce", `camera[0]: zoom.ease: want smooth or linear, got "bounce"`, false},
		{"start at an unknown beat", "beat: 2,", "beat: 9,", "camera[0]: zoom.at: beat 9 is not in the storyboard", false},
		{"start at the clip's end", "at: 3s", "at: 5s", "camera[1]: pan.at: want a start in [0s, 5s), got 5s", false},
		{"start before the clip", "at: 3s", "at: -1s", "camera[1]: pan.at: want a start in [0s, 5s), got -1s", false},
		{"end past the clip", "at: 4s, over: 1s", "at: 4s, over: 2s",
			"camera[2]: pan.over: want the move to end by the clip's end 5s, got an end at 6s", false},
		{"pan with to and via", "pan: {to: {cell: {x: 40", "pan: {via: [{cell: {x: 1, y: 1}}, {cell: {x: 2, y: 2}}], to: {cell: {x: 40",
			"camera[1]: pan: want exactly one of to or via", false},
		{"pan with neither", "to: {cell: {x: 40, y: 12}}, ", "", "camera[1]: pan: want exactly one of to or via", false},
		{"via of one point", "{text: 'Grip', area: list}, ", "", "camera[2]: pan.via: want at least 2 points, got 1", false},
		{"point with text and cell", "{x: 40, y: 12}}", "{x: 40, y: 12}, text: 'A'}", "camera[1]: pan.to: want exactly one of text or cell", false},
		{"point with neither", "to: {cell: {x: 40, y: 12}}", "to: {}", "camera[1]: pan.to: want exactly one of text or cell", false},
		{"cell past the last column", "{x: 40, y: 12}", "{x: 80, y: 12}",
			"camera[1]: pan.to.cell: want x in [0, 80) and y in [0, 24), got x 80, y 12", false},
		{"cell above the first row", "{x: 40, y: 12}", "{x: 40, y: -1}",
			"camera[1]: pan.to.cell: want x in [0, 80) and y in [0, 24), got x 40, y -1", false},
		{"cell with an nth", "{x: 40, y: 12}}", "{x: 40, y: 12}, nth: 2}", "camera[1]: pan.to.nth: only a text point takes an nth", false},
		{"cell with an area", "{x: 40, y: 12}}", "{x: 40, y: 12}, area: list}", "camera[1]: pan.to.area: only a text point takes an area", false},
		{"via point regex", "{text: 'Grip', area: list}", "{text: '(', area: list}", "camera[2]: pan.via[0].text: error parsing regexp", false},
		{"via point area", "area: list}, {cell", "area: 'a b'}, {cell",
			`camera[2]: pan.via[0].area: want an area name or any, got "a b"`, false},
		{"two zooms overlap", panToCell, "  - zoom: {to: 1, at: 3s, over: 400ms}",
			"camera[1]: zoom: overlaps camera[0] on the zoom track ([3.1s, 3.6s) and [3s, 3.4s))", false},
		{"two pans overlap", "at: 4s, over: 1s", "at: 3200ms, over: 1s",
			"camera[2]: pan: overlaps camera[1] on the pan track ([3s, 3.4s) and [3.2s, 4.2s))", false},
		{"pan in a beat's zoom", "at: 3s", "at: 2500ms",
			"camera[1]: pan: overlaps beat 1's zoom section [0s, 3s), which holds both tracks", false},
		{"zoom in a beat's zoom", "offset: 100ms", "offset: -100ms",
			"camera[0]: zoom: overlaps beat 1's zoom section [0s, 3s), which holds both tracks", false},
	}
	if _, err := Parse([]byte(cameraBase), "camera.yaml"); err != nil {
		t.Fatalf("the camera base storyboard must parse clean: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(cameraBase, tc.old); count != 1 {
				t.Fatalf("fragment %q occurs %d times in the base, want once", tc.old, count)
			}

			_, err := Parse([]byte(strings.Replace(cameraBase, tc.old, tc.new, 1)), "camera.yaml")

			if err == nil || !strings.Contains(err.Error(), tc.want) {
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
				t.Fatalf("problems: want exactly the one broken rule, got %d:\n%v", got, err)
			}
			owner := OwnerOf(invalid.Problems[0])
			if owner.Kind != OwnerCamera || !strings.HasPrefix(tc.want, fmt.Sprintf("camera[%d]: ", owner.Index)) {
				t.Errorf("owner: want the one %q names, got %+v", tc.want, owner)
			}
		})
	}
}

// captionBase is validBase with captions that break no rule, on its 5s clip. Each row of
// TestLoadRejects_Captions swaps one fragment of it, which must occur in it exactly once.
const captionBase = validBase + `captions:
  - text: 'Pick one'
    at: {beat: 1, offset: 500ms}
    until: {beat: 2}
    font: {size: 20, weight: bold}
    place: top
    align: right
    offset: {x: 4, y: 2}
    color: '#ffcc00'
    box: {color: '#101010', opacity: 0.5}
    fade: {in: 600ms, out: 400ms}
  - {text: 'Done', at: 4s, for: 500ms, position: {x: 10, y: 20}}
`

// captionCutBase is validBase with beat 2 cut and a kept beat 3 of 2s, so the clip keeps its 5s,
// and a caption that breaks no rule. Only a swapped fragment names the cut beat.
var captionCutBase = strings.Replace(validBase, "    title: next\n    duration: 2s\n",
	"    title: next\n    cut: true\n  - id: 3\n    title: last\n    duration: 2s\n", 1) + `captions:
  - {text: 'Cut', at: {beat: 1}, until: {beat: 3}}
`

func TestLoadRejects_Captions(t *testing.T) {
	t.Parallel()

	type rejectCase struct {
		name     string
		old, new string
		want     string
		// decode marks a rule the strict decode enforces, which fails before validation runs.
		decode bool
	}
	cases := []rejectCase{
		{"unknown caption field", "text: 'Done',", "text: 'Done', style: plain,", "field style not found", true},
		{"unknown box field", "opacity: 0.5}", "opacity: 0.5, blur: 2}", "field blur not found", true},
		{"text missing", "text: 'Done', ", "", "captions[1]: text: missing", false},
		{"text a lone newline", "text: 'Done'", `text: "\n"`, "captions[1]: text: missing", false},
		{"for and until", "until: {beat: 2}", "until: {beat: 2}\n    for: 2s", "captions[0]: want exactly one of for or until", false},
		{"neither for nor until", "    until: {beat: 2}\n", "", "captions[0]: want exactly one of for or until", false},
		{"for negative", "for: 500ms", "for: -1s", "captions[1]: for: want >0, got -1s", false},
		{"start at an unknown beat", "beat: 1, offset", "beat: 9, offset", "captions[0]: at: beat 9 is not in the storyboard", false},
		{"stop at an unknown beat", "until: {beat: 2}", "until: {beat: 7}", "captions[0]: until: beat 7 is not in the storyboard", false},
		{"start at the clip's end", "at: 4s", "at: 5s", "captions[1]: at: want a start in [0s, 5s), got 5s", false},
		{"start before the clip", "at: 4s", "at: -1s", "captions[1]: at: want a start in [0s, 5s), got -1s", false},
		{"stop before the start", "until: {beat: 2}", "until: 200ms", "captions[0]: until: want a stop after the start 500ms, got 200ms", false},
		{"stop at the start", "until: {beat: 2}", "until: 500ms", "captions[0]: until: want a stop after the start 500ms, got 500ms", false},
		{"for past the clip", "for: 500ms", "for: 2s", "captions[1]: for: want the caption to end by the clip's end 5s, got an end at 6s", false},
		{"until past the clip", "until: {beat: 2}", "until: {beat: 2, offset: 3s}",
			"captions[0]: until: want the caption to end by the clip's end 5s, got an end at 6s", false},
		{"size too small", "size: 20", "size: 5", "captions[0]: font.size: want in [6, 72], got 5", false},
		{"size too large", "size: 20", "size: 80", "captions[0]: font.size: want in [6, 72], got 80", false},
		{"weight unknown", "weight: bold", "weight: heavy", `captions[0]: font.weight: want regular or bold, got "heavy"`, false},
		{"place unknown", "place: top", "place: centre", `captions[0]: place: want top, middle or bottom, got "centre"`, false},
		{"align unknown", "align: right", "align: center", `captions[0]: align: want left, middle or right, got "center"`, false},
		{"position with place", "position: {x: 10, y: 20}", "position: {x: 10, y: 20}, place: top",
			"captions[1]: position: want it instead of place, align and offset, not with them", false},
		{"position with offset", "position: {x: 10, y: 20}", "position: {x: 10, y: 20}, offset: {y: 1}",
			"captions[1]: position: want it instead of place, align and offset, not with them", false},
		{"color misspelled", "color: '#ffcc00'", "color: 'yellow'", `captions[0]: color: want #rrggbb, got "yellow"`, false},
		{"box color short", "color: '#101010'", "color: '#111'", `captions[0]: box.color: want #rrggbb, got "#111"`, false},
		{"box opacity too large", "opacity: 0.5", "opacity: 1.5", "captions[0]: box.opacity: want in [0, 1], got 1.5", false},
		{"box opacity negative", "opacity: 0.5", "opacity: -0.1", "captions[0]: box.opacity: want in [0, 1], got -0.1", false},
		{"fade in negative", "in: 600ms, out: 400ms", "in: -1s, out: 400ms", "captions[0]: fade.in: want ≥0, got -1s", false},
		{"fade out negative", "in: 600ms, out: 400ms", "in: 600ms, out: -1s", "captions[0]: fade.out: want ≥0, got -1s", false},
		{"fades longer than the caption", "in: 600ms, out: 400ms", "in: 2s, out: 600ms",
			"captions[0]: fade: want in + out within the caption's length 2.5s, got 2.6s", false},
	}
	// cutBeatCases swap fragments of captionCutBase, whose beat 2 is cut.
	cutBeatCases := []rejectCase{
		{"start at a cut beat", "at: {beat: 1}", "at: {beat: 2}",
			"captions[0]: at: beat 2 is cut, so it has no clip time", false},
		{"stop at a cut beat", "until: {beat: 3}", "until: {beat: 2}",
			"captions[0]: until: beat 2 is cut, so it has no clip time", false},
	}
	type basedCase struct {
		base string
		rejectCase
	}
	var runs []basedCase
	for _, base := range []string{captionBase, captionCutBase} {
		if _, err := Parse([]byte(base), "captions.yaml"); err != nil {
			t.Fatalf("the caption base storyboard must parse clean: %v\n%s", err, base)
		}
	}
	for _, tc := range cases {
		runs = append(runs, basedCase{captionBase, tc})
	}
	for _, tc := range cutBeatCases {
		runs = append(runs, basedCase{captionCutBase, tc})
	}
	for _, tc := range runs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(tc.base, tc.old); count != 1 {
				t.Fatalf("fragment %q occurs %d times in the base, want once", tc.old, count)
			}

			_, err := Parse([]byte(strings.Replace(tc.base, tc.old, tc.new, 1)), "captions.yaml")

			if err == nil || !strings.Contains(err.Error(), tc.want) {
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
				t.Fatalf("problems: want exactly the one broken rule, got %d:\n%v", got, err)
			}
			owner := OwnerOf(invalid.Problems[0])
			if owner.Kind != OwnerCaption || !strings.HasPrefix(tc.want, fmt.Sprintf("captions[%d]: ", owner.Index)) {
				t.Errorf("owner: want the one %q names, got %+v", tc.want, owner)
			}
		})
	}
}

func TestParse_ResolvesCaptionValues(t *testing.T) {
	t.Parallel()
	// Beat 1 runs [0s, 3s); beat 2 is cut; beat 3 runs [3s, 5s).
	const board = `clip: resolve
ship: resolve.gif
frame: {cols: 80, rows: 24, padding: 16, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 24, max_colors: 128}
beats:
  - {id: 1, title: open, duration: 3s}
  - {id: 2, title: skipped, cut: true}
  - {id: 3, title: close, duration: 2s}
captions:
`
	white, black := RGB{R: 0xff, G: 0xff, B: 0xff}, RGB{}
	cases := []struct {
		name                string
		caption             string
		wantStart, wantStop time.Duration
		wantText, wantBox   RGB
	}{
		{"clip time and for", "{text: a, at: 1s, for: 500ms}", time.Second, 1500 * time.Millisecond, white, black},
		{"beat moment and until", "{text: a, at: {beat: 3, offset: 200ms}, until: {beat: 3, offset: 1s}}",
			3200 * time.Millisecond, 4 * time.Second, white, black},
		{"beat moment and for", "{text: a, at: {beat: 1}, for: 2s}", 0, 2 * time.Second, white, black},
		{"until a clip time", "{text: a, at: 2s, until: 4500ms}", 2 * time.Second, 4500 * time.Millisecond, white, black},
		{"explicit colours", "{text: a, at: 1s, for: 1s, color: '#0a1B2c', box: {color: '#FF8000'}}",
			time.Second, 2 * time.Second, RGB{R: 0x0a, G: 0x1b, B: 0x2c}, RGB{R: 0xff, G: 0x80}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := Parse([]byte(board+"  - "+tc.caption+"\n"), "resolve.yaml")

			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			caption := parsed.Captions[0]
			if caption.Start() != tc.wantStart || caption.Stop() != tc.wantStop {
				t.Errorf("start, stop: want %s, %s, got %s, %s", tc.wantStart, tc.wantStop, caption.Start(), caption.Stop())
			}
			if caption.TextColor() != tc.wantText || caption.BoxColor() != tc.wantBox {
				t.Errorf("colours: want text %+v and box %+v, got %+v and %+v",
					tc.wantText, tc.wantBox, caption.TextColor(), caption.BoxColor())
			}
		})
	}
}

func TestParse_ResolvesMoveStarts(t *testing.T) {
	t.Parallel()
	// Beat 1 runs [0s, 3s); beat 2 is cut; beat 3 runs [3s, 5s).
	body := `clip: resolve
ship: resolve.gif
frame: {cols: 80, rows: 24, padding: 16, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 24, max_colors: 128}
beats:
  - {id: 1, title: open, duration: 3s}
  - {id: 2, title: skipped, cut: true}
  - {id: 3, title: close, duration: 2s}
camera:
  - zoom: {to: 2, over: 1s}
  - zoom: {to: 1, at: {beat: 3, offset: 500ms}, over: 1s}
  - pan: {to: {cell: {x: 1, y: 1}}, at: 1500ms, over: 1s}
`

	parsed, err := Parse([]byte(body), "resolve.yaml")

	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := []time.Duration{parsed.Camera[0].Zoom.Start(), parsed.Camera[1].Zoom.Start(), parsed.Camera[2].Pan.Start()}
	want := []time.Duration{0, 3500 * time.Millisecond, 1500 * time.Millisecond}
	if !slices.Equal(got, want) {
		t.Errorf("starts: want %v, got %v", want, got)
	}
}

func TestParse_RefusesEveryBeatCut(t *testing.T) {
	t.Parallel()
	body := `clip: cut
ship: cut.gif
frame: {cols: 80, rows: 24, padding: 16, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 24, max_colors: 128}
beats:
  - {id: 1, title: open, cut: true}
  - {id: 2, title: close, cut: true}
`

	_, err := Parse([]byte(body), "cut.yaml")

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want a *ValidationError, got %v", err)
	}
	if len(invalid.Problems) != 1 || invalid.Problems[0].Error() != "beats: every beat is cut" {
		t.Fatalf("problems: want only %q, got:\n%v", "beats: every beat is cut", err)
	}
	if got := OwnerOf(invalid.Problems[0]); got != (Owner{Kind: OwnerClip}) {
		t.Errorf("owner: want the clip, got %+v", got)
	}
}

func TestLoad_CaptionSizeFromTheFrame(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		frameSize string
		want      string
	}{
		{"in bounds", "font_size: 14", ""},
		{"out of bounds", "font_size: 80", "captions[0]: font.size: want in [6, 72], got 80 from frame.font_size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := strings.Replace(validBase, "font_size: 14", tc.frameSize, 1) + "captions:\n  - {text: 'Hi', at: 1s, for: 1s}\n"

			board, err := Parse([]byte(body), "captions.yaml")

			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want an error containing %q, got:\n%v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := board.Captions[0].Font.Size; got != 14 {
				t.Errorf("font size: want frame.font_size 14, got %g", got)
			}
		})
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
	clip, beat := Owner{Kind: OwnerClip}, Owner{Kind: OwnerBeat, Index: 1}
	wantOwners := []Owner{clip, clip, beat, beat, beat}
	if got := len(invalid.Problems); got != len(want) {
		t.Fatalf("problems: want %d, got %d:\n%v", len(want), got, err)
	}
	for index, problem := range invalid.Problems {
		if !strings.Contains(problem.Error(), want[index]) {
			t.Errorf("problem %d: want %q, got %q", index, want[index], problem)
		}
		if got := OwnerOf(problem); got != wantOwners[index] {
			t.Errorf("problem %d: owner: want %+v, got %+v", index, wantOwners[index], got)
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

func TestParseMatchesLoad(t *testing.T) {
	t.Parallel()
	everyForm, err := os.ReadFile(filepath.Join("testdata", "every-form.yaml"))
	if err != nil {
		t.Fatalf("read the every-form fixture: %v", err)
	}

	cases := []struct {
		name string
		body string
		// isValid marks a storyboard both must accept; every other row both must refuse.
		isValid bool
	}{
		{"valid", string(everyForm), true},
		{"unknown key", strings.Replace(validBase, "clip: base", "clip: base\ntape: old.tape", 1), false},
		{"empty", "", false},
		{"invalid zoom", strings.Replace(validBase, "factor: 2", "factor: 4", 1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := writeStoryboard(t, tc.body)

			parsed, parseErr := Parse([]byte(tc.body), path)
			loaded, loadErr := Load(path)

			if tc.isValid != (parseErr == nil) {
				t.Fatalf("Parse: want valid = %v, got error %v", tc.isValid, parseErr)
			}
			if !reflect.DeepEqual(parsed, loaded) {
				t.Errorf("storyboard: Parse gave %+v, Load gave %+v", parsed, loaded)
			}
			if (parseErr == nil) != (loadErr == nil) || parseErr != nil && parseErr.Error() != loadErr.Error() {
				t.Errorf("error: Parse gave %v, Load gave %v", parseErr, loadErr)
			}
			var parseInvalid, loadInvalid *ValidationError
			if errors.As(parseErr, &parseInvalid) != errors.As(loadErr, &loadInvalid) {
				t.Errorf("error type: Parse gave %T, Load gave %T", parseErr, loadErr)
			}
		})
	}
}
