//go:build !windows

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/keys"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
	"github.com/airiclenz/photon/internal/terminal"
	"github.com/airiclenz/photon/internal/testkit"
)

// The fixture's terminal: its size, frame rate and the variable that names its input log.
const (
	fixtureCols     = 60
	fixtureRows     = 12
	fixtureFPS      = 30
	fixtureInputLog = "FIXTURE_INPUT_LOG"
)

// fixtureWait bounds every wait on the fixture, so one that never comes fails the test rather
// than hanging it.
const fixtureWait = 5 * time.Second

// The fixture's list rows: alpha, beta and gamma sit on rows 2-4, indented two columns, so
// alpha's centre is (4, 2), beta's (3, 3) and gamma's (4, 4).
var (
	alphaCentre = take.Point{X: 4, Y: 2}
	betaCentre  = take.Point{X: 3, Y: 3}
	gammaCentre = take.Point{X: 4, Y: 4}
)

// The fixture beats' plain drag: from alpha's centre dragOffset columns to the right in dragSteps
// steps, with the left button and no modifier.
const (
	dragOffset = 12
	dragSteps  = 6
)

// The fixture beats' via drag: from alpha's centre with the right button and ctrl held, by
// viaOffset to its via point, then by viaEndOffset to its release, dragSteps steps per segment.
var (
	viaOffset    = storyboard.Offset{X: 6, Y: 2}
	viaEndOffset = storyboard.Offset{X: 6, Y: -1}
	viaMouse     = keys.Mouse{Button: keys.RightButton, Mods: keys.Ctrl}
)

// The beat ids of the fixture beats' two drags.
const (
	plainDragBeat = 4
	viaDragBeat   = 5
)

func waitFor(screen string) storyboard.Action {
	return storyboard.Action{Wait: &storyboard.WaitAction{Screen: screen, Timeout: fixtureWait}}
}

func clickOn(text string, times int) storyboard.Action {
	return storyboard.Action{Click: &storyboard.ClickAction{Target: storyboard.Target{Text: text}, Times: times}}
}

// fixtureBeats drive the fixture through every action form.
func fixtureBeats() []storyboard.Beat {
	return []storyboard.Beat{
		{ID: 1, Title: "start", Do: []storyboard.Action{waitFor("ready, terminal answered")}},
		{ID: 2, Title: "type", Do: []storyboard.Action{
			{Type: &storyboard.TypeAction{Text: "hello"}},
			{Type: &storyboard.TypeAction{Text: " xy", Humanize: isFlat()}},
			waitFor("> hello xy"),
			{Key: &storyboard.KeyAction{Name: "backspace", Repeat: 3}},
			{Key: &storyboard.KeyAction{Name: "enter"}},
			waitFor("you typed: hello\n"),
		}},
		{ID: 3, Title: "click", Do: []storyboard.Action{
			clickOn("beta", 0),
			waitFor("selected: beta"),
			clickOn("gamma", 2),
			waitFor("selected: gamma"),
			{Pause: &storyboard.PauseAction{For: 50 * time.Millisecond}},
			{Wait: &storyboard.WaitAction{Screen: "selected: beta", Gone: true, Timeout: fixtureWait}},
		}},
		{ID: plainDragBeat, Title: "drag", Do: []storyboard.Action{
			dragFromAlpha(),
			waitFor(fmt.Sprintf("dragged: 4,2 to 16,2 after %d moves\n",
				len(dragRoute([]take.Point{alphaCentre, dragEnd()}, dragSteps)))),
		}},
		{ID: viaDragBeat, Title: "drag via", Do: []storyboard.Action{
			viaDragFromAlpha(),
			waitFor(fmt.Sprintf(`dragged: 4,2 to 16,3 after %d moves with right\+ctrl\n`,
				len(dragRoute(viaDragPoints(), dragSteps)))),
		}},
	}
}

func dragFromAlpha() storyboard.Action {
	steps := dragSteps
	return storyboard.Action{Drag: &storyboard.DragAction{
		From:  storyboard.Target{Text: "alpha"},
		To:    storyboard.DragEnd{Offset: &storyboard.Offset{X: dragOffset}},
		Steps: &steps,
	}}
}

// dragEnd is the cell the fixture beats' plain drag releases at.
func dragEnd() take.Point {
	return take.Point{X: alphaCentre.X + dragOffset, Y: alphaCentre.Y}
}

func viaDragFromAlpha() storyboard.Action {
	steps := dragSteps
	return storyboard.Action{Drag: &storyboard.DragAction{
		From:  storyboard.Target{Text: "alpha"},
		Via:   []storyboard.DragEnd{{Offset: &viaOffset}},
		To:    storyboard.DragEnd{Offset: &viaEndOffset},
		Steps: &steps,
		Press: storyboard.Press{Button: "right", With: "ctrl"},
	}}
}

// viaDragPoints are the cells the fixture beats' via drag presses at, passes and releases at.
func viaDragPoints() []take.Point {
	via := take.Point{X: alphaCentre.X + viaOffset.X, Y: alphaCentre.Y + viaOffset.Y}
	end := take.Point{X: via.X + viaEndOffset.X, Y: via.Y + viaEndOffset.Y}
	return []take.Point{alphaCentre, via, end}
}

func TestEngine_Run_FixtureTUI(t *testing.T) {
	inputLog := filepath.Join(t.TempDir(), "input.log")
	term, err := terminal.Start(testkit.BuildFixtureTUI(t), nil, terminal.Options{
		Cols: fixtureCols, Rows: fixtureRows, FPS: fixtureFPS,
		Env: []string{fixtureInputLog + "=" + inputLog},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { term.Close() })
	beats := fixtureBeats()

	err = New(term, target.Resolver{}).Run(context.Background(), beats)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	recorded := term.Close()
	assertFixtureInput(t, inputLog)
	assertFixtureEvents(t, recorded.Events, beats)
}

// assertFixtureInput checks that the fixture read exactly the bytes the beats send, in order.
func assertFixtureInput(t *testing.T, inputLog string) {
	t.Helper()
	press := func(cell take.Point) string {
		return string(keys.Click(cell.X, cell.Y)) + string(keys.Release(cell.X, cell.Y))
	}
	want := "hello xy" + strings.Repeat(keyBytes(t, "backspace"), 3) + keyBytes(t, "enter") +
		press(betaCentre) + press(gammaCentre) + press(gammaCentre) +
		dragBytes([]take.Point{alphaCentre, dragEnd()}, keys.Mouse{}) +
		dragBytes(viaDragPoints(), viaMouse)

	if got := readInputLog(t, inputLog); got != want {
		t.Errorf("the fixture read %q, want %q", got, want)
	}
}

// dragBytes is the press at the first of points, a motion to every cell of the eased route through
// them and the release at the last, each report carrying mouse's button and modifiers.
func dragBytes(points []take.Point, mouse keys.Mouse) string {
	from, to := points[0], points[len(points)-1]
	drag := string(keys.MousePress(from.X, from.Y, mouse))
	for _, move := range dragRoute(points, dragSteps) {
		drag += string(keys.MouseMotion(move.cell.X, move.cell.Y, mouse))
	}
	return drag + string(keys.MouseRelease(to.X, to.Y, mouse))
}

// readInputLog joins the input chunks the fixture logged, one Go-quoted chunk per line.
func readInputLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the input log: %v", err)
	}
	var input strings.Builder
	for line := range strings.Lines(string(data)) {
		chunk, err := strconv.Unquote(strings.TrimSuffix(line, "\n"))
		if err != nil {
			t.Fatalf("input log line %q: %v", line, err)
		}
		input.WriteString(chunk)
	}
	return input.String()
}

// assertFixtureEvents checks the take's event log: every beat started, every action started and
// ended, the clicks landed on the cells the targets resolved to, and each drag pressed, moved and
// released in that order, only the via drag marking a move as reaching its via point.
func assertFixtureEvents(t *testing.T, events []take.Event, beats []storyboard.Beat) {
	t.Helper()
	counts := map[take.EventKind]int{}
	var cells []take.Point
	for _, event := range events {
		counts[event.Kind]++
		if event.Kind == take.EventClick {
			cells = append(cells, *event.Cell)
		}
	}
	actions := 0
	for _, beat := range beats {
		actions += len(beat.Do)
	}

	if counts[take.EventBeatStart] != len(beats) {
		t.Errorf("%d beat-start events, want %d", counts[take.EventBeatStart], len(beats))
	}
	if counts[take.EventActionStart] != actions || counts[take.EventActionEnd] != actions {
		t.Errorf("%d action-start and %d action-end events, want %d of each",
			counts[take.EventActionStart], counts[take.EventActionEnd], actions)
	}
	if counts[take.EventTarget] != 4 {
		t.Errorf("%d target events, want 4", counts[take.EventTarget])
	}
	if want := []take.Point{betaCentre, gammaCentre, gammaCentre}; !slices.Equal(cells, want) {
		t.Errorf("clicked %v, want %v", cells, want)
	}
	plainPoints, viaPoints := []take.Point{alphaCentre, dragEnd()}, viaDragPoints()
	assertDragGesture(t, events, plainDragBeat, plainPoints[0], plainPoints[1], nil)
	assertDragGesture(t, events, viaDragBeat, viaPoints[0], viaPoints[2], viaPoints[1:2])
}

// assertDragGesture checks that beat's events hold one press at from, then at least one move, then
// one release at to, with no other gesture event between them, and that the moves marked
// [take.MoveVia] land on vias, in order.
func assertDragGesture(
	t *testing.T,
	events []take.Event,
	beat int,
	from, to take.Point,
	vias []take.Point,
) {
	t.Helper()
	var gesture []take.Event
	for _, event := range events {
		switch event.Kind {
		case take.EventPress, take.EventMove, take.EventRelease:
			if event.Beat == beat {
				gesture = append(gesture, event)
			}
		}
	}

	if len(gesture) < 3 {
		t.Fatalf("beat %d: %d press, move and release events, want a press, a move and a release",
			beat, len(gesture))
	}
	first, last := gesture[0], gesture[len(gesture)-1]
	if first.Kind != take.EventPress || *first.Cell != from {
		t.Errorf("beat %d: the drag began with %s at %v, want press at %v", beat, first.Kind, *first.Cell, from)
	}
	if last.Kind != take.EventRelease || *last.Cell != to {
		t.Errorf("beat %d: the drag ended with %s at %v, want release at %v", beat, last.Kind, *last.Cell, to)
	}
	var gotVias []take.Point
	for _, move := range gesture[1 : len(gesture)-1] {
		if move.Kind != take.EventMove {
			t.Errorf("beat %d: %s between the press and the release, want only moves", beat, move.Kind)
		}
		if move.Text == take.MoveVia {
			gotVias = append(gotVias, *move.Cell)
		}
	}
	if !slices.Equal(gotVias, vias) {
		t.Errorf("beat %d: via moves at %v, want %v", beat, gotVias, vias)
	}
}
