package testkit

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fixtureWebStartTimeout bounds how long the fixture web app may take to print its ready line.
const fixtureWebStartTimeout = 10 * time.Second

func TestBuildFixtureTUI_PathOutlivesTheTestThatBuiltIt(t *testing.T) {
	var built string
	t.Run("first build", func(t *testing.T) {
		built = BuildFixtureTUI(t)
	})

	t.Run("later call after the first one's cleanup", func(t *testing.T) {
		again := BuildFixtureTUI(t)

		if again != built {
			t.Fatalf("BuildFixtureTUI = %q, want the first call's %q", again, built)
		}
		info, err := os.Stat(again)
		if err != nil {
			t.Fatalf("stat the fixture: %v", err)
		}
		if info.IsDir() {
			t.Fatalf("%s is a directory, want the executable", again)
		}
	})
}

func TestBuildFixtureWeb_ServesThePageOnPhotonPort(t *testing.T) {
	path := BuildFixtureWeb(t)
	port := freeLoopbackPort(t)
	startFixtureWeb(t, path, port)

	response, err := http.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/")
	if err != nil {
		t.Fatalf("GET the page: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close the response: %v", err)
	}
	if readErr != nil {
		t.Fatalf("read the page: %v", readErr)
	}

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	for _, want := range []string{"<nav>", `id="input"`, ">Run</button>", `id="status"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the page lacks %q:\n%s", want, body)
		}
	}
}

// freeLoopbackPort asks the kernel for a free loopback port and releases it for the caller.
func freeLoopbackPort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release the free port: %v", err)
	}
	return strconv.Itoa(port)
}

// startFixtureWeb runs the fixture web app at path on port, waits for its ready line and stops it
// when t ends.
func startFixtureWeb(t *testing.T, path, port string) {
	t.Helper()

	command := exec.Command(path)
	command.Env = append(os.Environ(), "PHOTON_PORT="+port)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe the fixture's stdout: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start the fixture web app: %v", err)
	}
	t.Cleanup(func() {
		if err := command.Process.Kill(); err != nil {
			t.Errorf("stop the fixture web app: %v", err)
		}
		if err := command.Wait(); err != nil && command.ProcessState == nil {
			t.Errorf("wait for the fixture web app: %v", err)
		}
	})

	ready := make(chan string, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err != nil {
			line = "(no line: " + err.Error() + ")"
		}
		ready <- strings.TrimSpace(line)
	}()
	select {
	case line := <-ready:
		if line != "ready" {
			t.Fatalf("the fixture's first line = %q, want %q", line, "ready")
		}
	case <-time.After(fixtureWebStartTimeout):
		t.Fatalf("the fixture web app printed no ready line within %s", fixtureWebStartTimeout)
	}
}

func TestExecutableName_AddsTheWindowsSuffix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		goos string
		want string
	}{
		{goos: "linux", want: "fixturetui"},
		{goos: "darwin", want: "fixturetui"},
		{goos: "windows", want: "fixturetui.exe"},
	}

	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			t.Parallel()

			if got := executableName("fixturetui", tc.goos); got != tc.want {
				t.Errorf("executableName(%q) = %q, want %q", tc.goos, got, tc.want)
			}
		})
	}
}
