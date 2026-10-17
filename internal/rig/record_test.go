//go:build !windows

package rig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/check"
	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/engine"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
	"github.com/airiclenz/photon/internal/testkit"
)

// The rig's files in a test project dir, and the clip's name.
const (
	configFile     = "photon.yaml"
	storyboardFile = "demo.yaml"
	clipName       = "demo"
)

// The marker files the test project's commands leave in the work dir.
const (
	resetMarker     = "reset"
	stoppedMarker   = "stopped"
	succeededMarker = "succeeded"
	inputLog        = "input.log"
	appPID          = "app.pid"
)

// appGoneWait bounds the wait for a killed app to be reaped by whoever inherited it.
const appGoneWait = 5 * time.Second

// The launches a test project runs the fixture TUI with: straight, and compound with a shell
// left between the pty and the app, which records the app's pid on its way in.
const (
	execLaunch     = `exec "$FIXTURE_TUI"`
	compoundLaunch = `. ./env.sh && sh -c 'echo $$ > "$PHOTON_WORK/` + appPID + `"; exec "$FIXTURE_TUI"'`
)

// backendScript is the test project's one service: ready at once, and on SIGTERM it leaves the
// stopped marker, so a later command can tell the services were stopped before it.
const backendScript = `trap 'echo stopped > "$PHOTON_WORK/` + stoppedMarker + `"; exit 0' TERM
echo ready
while :; do sleep 0.05; done
`

// envScript is what a compound launch sources first: it points the fixture at its input log.
const envScript = `export FIXTURE_INPUT_LOG="$PHOTON_WORK/` + inputLog + `"
`

// projectConfig is the test project's photon.yaml; the verbs fill the launch command, the fixture
// path and the end of take. Its `demo` profile's on_success copies the stopped marker, so it
// fails when the services were not stopped before it.
const projectConfig = `name: rig-test
reset: echo reset ran > "$PHOTON_WORK/` + resetMarker + `"
launch:
  command: |-
    %s
  env:
    FIXTURE_TUI: %q
end:
  keys: [%s]
  idle: 100ms
  kill_after: %s
services:
  backend:
    command: sh ./backend.sh
    ready: {line: '^ready$'}
profiles:
  demo:
    on_success: cp "$PHOTON_WORK/` + stoppedMarker + `" "$PHOTON_WORK/` + succeededMarker + `"
`

// projectStoryboard is the test project's storyboard; the verbs fill the screen beat 2 waits for
// and how long it waits. The take-level run expect passes only while the services are up.
const projectStoryboard = `clip: ` + clipName + `
ship: out/demo.gif
frame: {cols: 60, rows: 12, padding: 16, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 30, max_colors: 128}
expect:
  - seen: 'photon fixture'
  - run: test ! -e "$PHOTON_WORK/` + stoppedMarker + `" && test -n "$PHOTON_PORT_BACKEND"
beats:
  - id: 1
    title: start
    duration: 1s
    do:
      - wait: {screen: 'ready, terminal answered', timeout: 5s}
    expect:
      - seen: 'ready, terminal answered'
  - id: 2
    title: type
    duration: 1s
    do:
      - type: {text: 'hi', humanize: false}
      - key: {name: enter}
      - wait: {screen: '%s', timeout: %s}
    expect:
      - seen: 'you typed: hi'
`

// project is one test project: how it launches and ends the fixture, and what beat 2 waits for.
type project struct {
	launch    string
	endKey    string
	killAfter string
	waitFor   string
	waitLimit string
}

// passing is a project whose every beat passes.
var passing = project{
	launch: execLaunch, endKey: "ctrl+c", killAfter: "10s", waitFor: "you typed: hi", waitLimit: "5s",
}

// writeProject writes p into a fresh dir and loads its clip, with the work dir under that dir.
func writeProject(t *testing.T, p project) (*clip.Clip, string) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		configFile:     fmt.Sprintf(projectConfig, p.launch, testkit.BuildFixtureTUI(t), p.endKey, p.killAfter),
		storyboardFile: fmt.Sprintf(projectStoryboard, p.waitFor, p.waitLimit),
		"backend.sh":   backendScript,
		"env.sh":       envScript,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	work := filepath.Join(dir, "work")
	loaded, err := clip.Load(filepath.Join(dir, storyboardFile), "", work)
	if err != nil {
		t.Fatalf("load the clip: %v", err)
	}
	return loaded, work
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func assertAllPass(t *testing.T, rows []check.Row, want int) {
	t.Helper()
	if len(rows) != want {
		t.Errorf("got %d rows, want %d: %v", len(rows), want, rows)
	}
	for _, row := range rows {
		if row.Verdict != check.Pass {
			t.Errorf("row %s %s: %s", row.Subject, row.Verdict, row.Detail)
		}
	}
}

// assertTakeWritten checks the take is at the clip's take path, named on out, and returns it.
func assertTakeWritten(t *testing.T, work, out string) *take.Take {
	t.Helper()
	takePath := filepath.Join(work, clipName+".take")
	if !strings.Contains(out, "take: "+takePath+"\n") {
		t.Errorf("output %q does not name the take %s", out, takePath)
	}
	recorded, err := take.Load(takePath)
	if err != nil {
		t.Fatalf("load the take: %v", err)
	}
	return recorded
}

// lastScreen is the whole text of the take's last snapshot.
func lastScreen(t *testing.T, recorded *take.Take) string {
	t.Helper()
	if len(recorded.Snapshots) == 0 {
		t.Fatal("the take holds no snapshot")
	}
	text, err := target.Resolver{}.Text(recorded.Snapshots[len(recorded.Snapshots)-1], "")
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestRecord_PassingTake(t *testing.T) {
	t.Parallel()
	loaded, work := writeProject(t, passing)
	var out bytes.Buffer

	rows, err := Record(context.Background(), loaded, "demo", &out)

	if err != nil {
		t.Fatalf("Record: %v\noutput:\n%s", err, out.String())
	}
	assertAllPass(t, rows, 4)
	recorded := assertTakeWritten(t, work, out.String())
	// The fixture leaves its alternate screen on the way out; a frozen take never shows that.
	if screen := lastScreen(t, recorded); !strings.Contains(screen, "you typed: hi") {
		t.Errorf("the take's last screen is not the app's:\n%s", screen)
	}
	for _, marker := range []string{resetMarker, stoppedMarker, succeededMarker} {
		if !exists(filepath.Join(work, marker)) {
			t.Errorf("no %s marker in the work dir", marker)
		}
	}
}

func TestRecord_FailingBeatWritesTheTakeAndSkipsTheCheck(t *testing.T) {
	t.Parallel()
	failing := passing
	failing.waitFor, failing.waitLimit = "never on screen", "300ms"
	loaded, work := writeProject(t, failing)
	var out bytes.Buffer

	rows, err := Record(context.Background(), loaded, "demo", &out)

	var beatErr *engine.BeatError
	if !errors.As(err, &beatErr) || beatErr.Beat != 2 {
		t.Fatalf("Record = %v, want beat 2's BeatError", err)
	}
	if rows != nil {
		t.Errorf("a failed take was checked: %v", rows)
	}
	assertTakeWritten(t, work, out.String())
	if !exists(filepath.Join(work, stoppedMarker)) {
		t.Error("the services were not stopped")
	}
	if exists(filepath.Join(work, succeededMarker)) {
		t.Error("on_success ran after a failed beat")
	}
}

func TestRecord_CompoundLaunchRecordsAndStops(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		endKey    string
		killAfter string
	}{
		{name: "the app quits on its keys", endKey: "ctrl+c", killAfter: "10s"},
		{name: "the app ignores its keys and is killed with the shell", endKey: "enter", killAfter: "200ms"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			compound := passing
			compound.launch, compound.endKey, compound.killAfter = compoundLaunch, test.endKey, test.killAfter
			loaded, work := writeProject(t, compound)
			var out bytes.Buffer

			rows, err := Record(context.Background(), loaded, "", &out)

			if err != nil {
				t.Fatalf("Record: %v\noutput:\n%s", err, out.String())
			}
			assertAllPass(t, rows, 4)
			assertTakeWritten(t, work, out.String())
			if !exists(filepath.Join(work, inputLog)) {
				t.Error("the app never saw the env the launch sourced")
			}
			assertGone(t, filepath.Join(work, appPID))
		})
	}
}

// assertGone checks that the process whose pid the file at pidPath holds has exited and been
// reaped.
func assertGone(t *testing.T, pidPath string) {
	t.Helper()
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read the app's pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("the app's pid %q: %v", raw, err)
	}
	testkit.Eventually(t, appGoneWait, func() error {
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("the app (pid %d) outlived the take: signal 0 gave %v", pid, err)
		}
		return nil
	})
}

// setupConfig is a project whose setup reports the work dir it was handed and the dir it ran in.
const setupConfig = `name: rig-test
setup: |-
  printf '%s\n%s\n' "$PHOTON_WORK" "$(pwd)" > "$PHOTON_WORK/setup.out"
launch:
  command: exec true
`

// writeSetupProject writes a project with the given config into a fresh dir and returns the dir.
func writeSetupProject(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	board := fmt.Sprintf(projectStoryboard, passing.waitFor, passing.waitLimit)
	for name, body := range map[string]string{configFile: config, storyboardFile: board} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSetup_RelativeWorkDirIsMadeAbsoluteAndCreated(t *testing.T) {
	dir := writeSetupProject(t, setupConfig)
	t.Chdir(dir)
	t.Setenv(clip.WorkEnv, "/an/operator/work/dir")
	loaded, err := clip.Load(storyboardFile, "", filepath.Join("relative", "work"))
	if err != nil {
		t.Fatalf("load the clip: %v", err)
	}

	err = Setup(context.Background(), loaded, &bytes.Buffer{})

	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	work := filepath.Join(dir, "relative", "work")
	reported, err := os.ReadFile(filepath.Join(work, "setup.out"))
	if err != nil {
		t.Fatalf("setup left no report in the absolute work dir: %v", err)
	}
	if want := work + "\n" + dir + "\n"; string(reported) != want {
		t.Errorf("setup reported %q (PHOTON_WORK, then its dir), want %q", reported, want)
	}
}

func TestSetup_FailureWritesItsOutput(t *testing.T) {
	t.Parallel()
	dir := writeSetupProject(t, `name: rig-test
setup: echo the build broke; exit 3
launch:
  command: exec true
`)
	loaded, err := clip.Load(filepath.Join(dir, storyboardFile), "", filepath.Join(dir, "work"))
	if err != nil {
		t.Fatalf("load the clip: %v", err)
	}
	var out bytes.Buffer

	err = Setup(context.Background(), loaded, &out)

	if err == nil || !strings.Contains(err.Error(), "setup") {
		t.Fatalf("Setup = %v, want an error naming setup", err)
	}
	if !strings.Contains(out.String(), "the build broke") {
		t.Errorf("setup's output %q did not reach the writer", out.String())
	}
}
