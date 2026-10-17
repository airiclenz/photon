//go:build !windows

package engine

import (
	"context"
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

// The fixture beats' drag: from alpha's centre dragOffset columns to the right in dragSteps steps.
const (
	dragOffset = 12
	dragSteps  = 6
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
		{ID: 4, Title: "drag", Do: []storyboard.Action{
			dragFromAlpha(),
			waitFor("dragged: 4,2 to 16,2"),
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

// dragEnd is the cell the fixture beats' drag releases at.
func dragEnd() take.Point {
	return take.Point{X: alphaCentre.X + dragOffset, Y: alphaCentre.Y}
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
	drag := string(keys.Click(alphaCentre.X, alphaCentre.Y))
	for _, move := range dragPath(alphaCentre, dragEnd(), dragSteps) {
		drag += string(keys.Motion(move.cell.X, move.cell.Y))
	}
	drag += string(keys.Release(dragEnd().X, dragEnd().Y))
	want := "hello xy" + strings.Repeat(keyBytes(t, "backspace"), 3) + keyBytes(t, "enter") +
		press(betaCentre) + press(gammaCentre) + press(gammaCentre) + drag

	if got := readInputLog(t, inputLog); got != want {
		t.Errorf("the fixture read %q, want %q", got, want)
	}
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
// ended, the clicks landed on the cells the targets resolved to, and the drag pressed, moved and
// released in that order.
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
	if counts[take.EventTarget] != 3 {
		t.Errorf("%d target events, want 3", counts[take.EventTarget])
	}
	if want := []take.Point{betaCentre, gammaCentre, gammaCentre}; !slices.Equal(cells, want) {
		t.Errorf("clicked %v, want %v", cells, want)
	}
	assertDragOrder(t, events)
}

// assertDragOrder checks that the log holds one press at alpha's centre, then at least one move,
// then one release at the drag's end, with no other gesture event between them.
func assertDragOrder(t *testing.T, events []take.Event) {
	t.Helper()
	var gesture []take.Event
	for _, event := range events {
		switch event.Kind {
		case take.EventPress, take.EventMove, take.EventRelease:
			gesture = append(gesture, event)
		}
	}

	if len(gesture) < 3 {
		t.Fatalf("%d press, move and release events, want a press, at least one move and a release",
			len(gesture))
	}
	first, last := gesture[0], gesture[len(gesture)-1]
	if first.Kind != take.EventPress || *first.Cell != alphaCentre {
		t.Errorf("the drag began with %s at %v, want press at %v", first.Kind, *first.Cell, alphaCentre)
	}
	if last.Kind != take.EventRelease || *last.Cell != dragEnd() {
		t.Errorf("the drag ended with %s at %v, want release at %v", last.Kind, *last.Cell, dragEnd())
	}
	for _, move := range gesture[1 : len(gesture)-1] {
		if move.Kind != take.EventMove {
			t.Errorf("%s between the press and the release, want only moves", move.Kind)
		}
	}
}
