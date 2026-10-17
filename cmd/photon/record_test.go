//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/testkit"
)

// roleEnv names the variable that re-executes this test binary as one of the roles below instead
// of running the tests.
const roleEnv = "CMDTEST_ROLE"

// The roles the test binary is re-executed in: photon itself, a service that greets every
// connection on its PHOTON_PORT, and a client that dials a port and wants that greeting.
const (
	rolePhoton = "photon"
	roleServe  = "serve"
	roleDial   = "dial"
)

// greeting is what the serve role writes to every connection and the dial role wants back.
const greeting = "hello from the service\n"

// dialTimeout bounds the dial role's connection and read, and the test's own probe of a port.
const dialTimeout = 2 * time.Second

// helloStoryboard is the example clip, recorded end to end.
var helloStoryboard = filepath.Join("..", "..", "examples", "hello", "hello.yaml")

func TestMain(m *testing.M) {
	switch os.Getenv(roleEnv) {
	case rolePhoton:
		os.Exit(execute(context.Background(), newRootCommand(), os.Args[1:], os.Stderr))
	case roleServe:
		os.Exit(serve())
	case roleDial:
		os.Exit(dial(os.Args[1]))
	}
	os.Exit(m.Run())
}

// serve listens on 127.0.0.1:$PHOTON_PORT and greets every connection until it is killed.
func serve() int {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", os.Getenv("PHOTON_PORT")))
	if err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return 1
	}
	for {
		connection, err := listener.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, "serve:", err)
			return 1
		}
		if _, err := io.WriteString(connection, greeting); err != nil {
			fmt.Fprintln(os.Stderr, "serve:", err)
		}
		if err := connection.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "serve:", err)
		}
	}
}

// dial connects to 127.0.0.1:port and exits 0 only when the greeting comes back.
func dial(port string) int {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), dialTimeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		return 1
	}
	defer connection.Close() //nolint:errcheck // read-only; the reply is what is judged
	if err := connection.SetReadDeadline(time.Now().Add(dialTimeout)); err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		return 1
	}
	reply, err := io.ReadAll(connection)
	if err != nil || string(reply) != greeting {
		fmt.Fprintf(os.Stderr, "dial: reply %q, %v\n", reply, err)
		return 1
	}
	return 0
}

// testBinary is this test binary's path, which the commands of a test project re-execute.
func testBinary(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return path
}

// projectConfig is a test project's photon.yaml: the fixture TUI as the program, a quick end of
// take, then the verb's extra top-level fields.
const projectConfig = `name: record-test
launch:
  command: exec %q
end:
  idle: 100ms
%s`

// projectHeader is a test storyboard's header; each storyboard appends its expects and beats.
const projectHeader = `clip: demo
ship: out/demo.gif
frame: {cols: 60, rows: 12, padding: 16, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 24, max_colors: 128}
`

// openBeat waits for the fixture's first screen; a storyboard whose beats are only this passes
// every beat.
const openBeat = `beats:
  - id: 1
    title: open
    duration: 1s
    do:
      - wait: {screen: 'ready, terminal answered', timeout: 5s}
`

// passingExpect is a take-level expect every take of the fixture passes.
const passingExpect = "expect:\n  - seen: 'photon fixture'\n"

// failingBeat waits for a screen the fixture never shows.
const failingBeat = `beats:
  - id: 1
    title: wait in vain
    duration: 1s
    do:
      - wait: {screen: 'never shown', timeout: 300ms}
`

// writeProject writes a test project — its config, with extra appended, and each storyboard of
// boards by file name — into a fresh dir and returns that dir.
func writeProject(t *testing.T, extra string, boards map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "photon.yaml", fmt.Sprintf(projectConfig, testkit.BuildFixtureTUI(t), extra))
	for name, body := range boards {
		writeFile(t, dir, name, projectHeader+body)
	}
	return dir
}

// serviceConfig declares one service, backend, on the given port: this test binary in its serve
// role, ready once its port accepts.
func serviceConfig(t *testing.T, port int) string {
	t.Helper()
	return fmt.Sprintf(`services:
  backend:
    command: %s=%s exec %q
    port: %d
    ready: {tcp: true}
`, roleEnv, roleServe, testBinary(t), port)
}

// freePort is a port nothing listened on a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close the listener: %v", err)
	}
	return port
}

// assertTake fails t unless a take that loads sits at path.
func assertTake(t *testing.T, path string) {
	t.Helper()
	if _, err := take.Load(path); err != nil {
		t.Errorf("take at %s: %v", path, err)
	}
}

func TestRecordHello(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")
	work := t.TempDir()

	code, stdout, stderr := runLint(t, []string{"setup", helloStoryboard, "--work", work})
	if code != exitOK {
		t.Fatalf("setup exit code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout, stderr)
	}

	code, stdout, stderr = runLint(t, []string{"record", helloStoryboard, "--work", work})

	takePath := filepath.Join(work, "hello.take")
	if code != exitOK {
		t.Errorf("record exit code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout, stderr)
	}
	assertTake(t, takePath)
	if !strings.Contains(stderr, "take: "+takePath) {
		t.Errorf("stderr %q does not name the take %s", stderr, takePath)
	}
	for _, want := range []string{"beat 3 | PASS | seen /selected: beta/ in area status", "take   | PASS | exit 0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q does not carry %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "FAIL") {
		t.Errorf("stdout %q carries a FAIL row", stdout)
	}
}

// dragBeat drags the fixture's alpha item 12 columns right, waits for the fixture to report the
// drag, and expects that report: press at alpha's centre 4,2, release at 16,2.
const dragBeat = `beats:
  - id: 1
    title: open
    duration: 1s
    do:
      - wait: {screen: 'ready, terminal answered', timeout: 5s}
  - id: 2
    title: drag alpha
    duration: 2s
    do:
      - drag: {from: {text: alpha}, to: {offset: {x: 12}}, steps: 6}
      - wait: {screen: 'dragged: 4,2 to 16,2', timeout: 5s}
    expect:
      - seen: 'dragged: 4,2 to 16,2'
`

// dragPress and dragRelease are the cells dragBeat presses and releases the button at.
var (
	dragPress   = take.Point{X: 4, Y: 2}
	dragRelease = take.Point{X: 16, Y: 2}
)

// assertDragEvents fails t unless the take at path logs one drag: a press at dragPress, at least
// one move, then a release at dragRelease.
func assertDragEvents(t *testing.T, path string) {
	t.Helper()
	recorded, err := take.Load(path)
	if err != nil {
		t.Fatalf("take at %s: %v", path, err)
	}
	var gesture []take.Event
	var kinds []take.EventKind
	for _, event := range recorded.Events {
		switch event.Kind {
		case take.EventPress, take.EventMove, take.EventRelease:
			gesture = append(gesture, event)
			kinds = append(kinds, event.Kind)
		}
	}
	last := len(gesture) - 1
	if last < 2 || kinds[0] != take.EventPress || kinds[last] != take.EventRelease {
		t.Fatalf("drag events %v: want a press, moves, then a release", kinds)
	}
	for _, kind := range kinds[1:last] {
		if kind != take.EventMove {
			t.Fatalf("drag events %v: want only moves between the press and the release", kinds)
		}
	}
	if press := gesture[0].Cell; press == nil || *press != dragPress {
		t.Errorf("press cell = %v, want %v", press, dragPress)
	}
	if release := gesture[last].Cell; release == nil || *release != dragRelease {
		t.Errorf("release cell = %v, want %v", release, dragRelease)
	}
}

// TestRecordDrag lints, records and renders a storyboard that drags in the fixture TUI: the take
// logs the drag, every beat and expect passes, and the GIF is written when ffmpeg is on PATH.
func TestRecordDrag(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")
	project := writeProject(t, "", map[string]string{"demo.yaml": dragBeat})
	board := filepath.Join(project, "demo.yaml")
	work := t.TempDir()

	code, stdout, stderr := runLint(t, []string{"lint", board})
	if code != exitOK {
		t.Fatalf("lint exit code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout, stderr)
	}

	code, stdout, stderr = runLint(t, []string{"record", board, "--work", work})

	takePath := filepath.Join(work, "demo.take")
	if code != exitOK {
		t.Fatalf("record exit code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout, stderr)
	}
	if want := "beat 2 | PASS | seen /dragged: 4,2 to 16,2/"; !strings.Contains(stdout, want) {
		t.Errorf("stdout %q does not carry %q", stdout, want)
	}
	if strings.Contains(stdout, "FAIL") {
		t.Errorf("stdout %q carries a FAIL row", stdout)
	}
	assertDragEvents(t, takePath)

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not on PATH")
	}
	gifPath := filepath.Join(t.TempDir(), "drag.gif")

	code, stdout, stderr = runLint(t, []string{"render", board, takePath, "-o", gifPath})

	if code != exitOK {
		t.Fatalf("render exit code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout, stderr)
	}
	info, err := os.Stat(gifPath)
	if err != nil {
		t.Fatalf("render wrote no GIF: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("GIF %s is empty", gifPath)
	}
}

func TestRecordServices(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")

	t.Run("a run expect reaches the service through its port variable", func(t *testing.T) {
		dialExpect := fmt.Sprintf("expect:\n  - run: %s=%s %q \"$PHOTON_PORT_BACKEND\"\n", roleEnv, roleDial, testBinary(t))
		project := writeProject(t, serviceConfig(t, 0), map[string]string{"demo.yaml": dialExpect + openBeat})
		work := t.TempDir()

		code, stdout, stderr := runLint(t, []string{"record", filepath.Join(project, "demo.yaml"), "--work", work})

		if code != exitOK {
			t.Errorf("exit code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout, stderr)
		}
		if !strings.Contains(stdout, "take | PASS | exit 0") {
			t.Errorf("stdout %q does not carry the run expect's PASS", stdout)
		}
	})

	t.Run("a failed take still stops its service", func(t *testing.T) {
		port := freePort(t)
		project := writeProject(t, serviceConfig(t, port), map[string]string{"demo.yaml": failingBeat})
		work := t.TempDir()

		code, stdout, stderr := runLint(t, []string{"record", filepath.Join(project, "demo.yaml"), "--work", work})

		if code != exitRunFailed {
			t.Errorf("exit code = %d, want %d (stdout %q, stderr %q)", code, exitRunFailed, stdout, stderr)
		}
		assertTake(t, filepath.Join(work, "demo.take"))
		connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), dialTimeout)
		if err == nil {
			connection.Close() //nolint:errcheck,gosec // the connection was the failure
			t.Fatalf("port %d still accepts after record returned", port)
		}
		if !errors.Is(err, syscall.ECONNREFUSED) {
			t.Errorf("dial port %d: %v, want the connection refused", port, err)
		}
	})
}

func TestRecordExitStatus(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")
	project := writeProject(t, "setup: exit 3\n", map[string]string{
		"passing.yaml":        passingExpect + openBeat,
		"failing-beat.yaml":   failingBeat,
		"failing-expect.yaml": "expect:\n  - seen: 'absent from the screen'\n" + openBeat,
	})
	board := func(name string) string { return filepath.Join(project, name) }
	noSetup := t.TempDir()
	writeFile(t, noSetup, "photon.yaml", "name: plain\nlaunch:\n  command: exit 0\n")
	writeFile(t, noSetup, "demo.yaml", projectHeader+openBeat)

	tests := []struct {
		name         string
		args         []string
		wantCode     int
		wantTake     bool
		wantStdout   []string
		wantStderr   []string
		forbidStdout string
	}{
		{
			name:       "every beat and expect passing exits 0",
			args:       []string{"record", board("passing.yaml")},
			wantCode:   exitOK,
			wantTake:   true,
			wantStdout: []string{"take | PASS | seen /photon fixture/"},
		},
		{
			name:         "a failed beat writes the take, skips the check and exits 1",
			args:         []string{"record", board("failing-beat.yaml")},
			wantCode:     exitRunFailed,
			wantTake:     true,
			wantStderr:   []string{"beat 1 (wait in vain)"},
			forbidStdout: "|",
		},
		{
			name:       "a failed expect exits 1",
			args:       []string{"record", board("failing-expect.yaml")},
			wantCode:   exitRunFailed,
			wantTake:   true,
			wantStdout: []string{"take | FAIL | not seen /absent from the screen/"},
			wantStderr: []string{"1 expect(s) failed"},
		},
		{
			name:       "an undeclared profile exits 1",
			args:       []string{"record", board("passing.yaml"), "--profile", "missing"},
			wantCode:   exitRunFailed,
			wantStderr: []string{`"missing"`},
		},
		{
			name:       "a missing storyboard exits 1",
			args:       []string{"record", board("absent.yaml")},
			wantCode:   exitRunFailed,
			wantStderr: []string{"absent.yaml"},
		},
		{
			name:       "no storyboard is a bad command line",
			args:       []string{"record"},
			wantCode:   exitBadUsage,
			wantStderr: []string{"accepts 1 arg(s), received 0"},
		},
		{
			name:       "an unknown flag is a bad command line",
			args:       []string{"record", board("passing.yaml"), "--retakes", "3"},
			wantCode:   exitBadUsage,
			wantStderr: []string{"unknown flag: --retakes"},
		},
		{
			name:       "a failing setup exits 1",
			args:       []string{"setup", board("passing.yaml")},
			wantCode:   exitRunFailed,
			wantStderr: []string{`setup "exit 3"`},
		},
		{
			name:     "a config with no setup exits 0",
			args:     []string{"setup", filepath.Join(noSetup, "demo.yaml")},
			wantCode: exitOK,
		},
		{
			name:       "setup with no storyboard is a bad command line",
			args:       []string{"setup"},
			wantCode:   exitBadUsage,
			wantStderr: []string{"accepts 1 arg(s), received 0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			work := t.TempDir()

			code, stdout, stderr := runLint(t, append(tt.args, "--work", work))

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stdout %q, stderr %q)", code, tt.wantCode, stdout, stderr)
			}
			if tt.wantTake {
				assertTake(t, filepath.Join(work, "demo.take"))
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout %q does not carry %q", stdout, want)
				}
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q does not carry %q", stderr, want)
				}
			}
			if tt.forbidStdout != "" && strings.Contains(stdout, tt.forbidStdout) {
				t.Errorf("stdout %q carries %q", stdout, tt.forbidStdout)
			}
		})
	}
}

// retakeScript retakes until a take passes, giving up after five, and prints how many takes
// failed first. It is the loop the record command's exit status exists for.
const retakeScript = `failed=0
until "$PHOTON_BIN" record "$BOARD" --work "$WORK" >/dev/null 2>&1; do
  failed=$((failed + 1))
  [ "$failed" -lt 5 ] || exit 9
done
echo "failed takes: $failed"
`

// flakyExpect fails the first take it judges and passes every later one: it leaves a marker in
// the work dir on its way out.
const flakyExpect = `expect:
  - run: test -e "$PHOTON_WORK/retaken" || { touch "$PHOTON_WORK/retaken"; exit 1; }
`

func TestRetakeLoop(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")
	project := writeProject(t, "", map[string]string{"flaky.yaml": flakyExpect + openBeat})

	loop := exec.Command("sh", "-c", retakeScript)
	loop.Env = append(os.Environ(),
		roleEnv+"="+rolePhoton,
		"PHOTON_BIN="+testBinary(t),
		"BOARD="+filepath.Join(project, "flaky.yaml"),
		"WORK="+t.TempDir(),
	)
	output, err := loop.CombinedOutput()

	if err != nil {
		t.Fatalf("the retake loop: %v\n%s", err, output)
	}
	if got, want := strings.TrimSpace(string(output)), "failed takes: 1"; got != want {
		t.Errorf("the retake loop printed %q, want %q", got, want)
	}
}
