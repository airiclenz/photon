package service

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/testkit"
)

// shortTimeout is the readiness timeout of a service a test expects never to be ready.
const shortTimeout = 300 * time.Millisecond

// goneTimeout bounds the wait for a stopped service's process to be gone, reaped included.
const goneTimeout = 5 * time.Second

// svcBinary is the path of the test service (testdata/svc), built once by TestMain.
var svcBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "photon-service-svc-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create the build dir:", err)
		os.Exit(1)
	}
	svcBinary = filepath.Join(dir, "svc")
	if runtime.GOOS == "windows" {
		svcBinary += ".exe"
	}
	if output, err := exec.Command("go", "build", "-o", svcBinary, "./testdata/svc").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build the test service: %v\n%s", err, output)
		os.Exit(1)
	}

	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, "remove the build dir:", err)
	}
	os.Exit(code)
}

func TestStart_TCPReadiness_ServesUntilStop(t *testing.T) {
	cfg := configWith(map[string]config.Service{
		"api": {Command: svc("-listen -delay 200ms"), Ready: config.Ready{TCP: true}},
	})

	set, _ := start(t, cfg, "")

	port := portOf(t, set, "PHOTON_PORT_API")
	if got := greet(t, port); got != "hello from svc" {
		t.Errorf("service answered %q, want the greeting", got)
	}
	if err := set.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if connection, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", port)); err == nil {
		connection.Close() //nolint:errcheck // the test fails either way
		t.Error("service still accepts connections after Stop")
	}
}

func TestStop_SendsSIGTERMFirst(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no SIGTERM; Stop kills the service there")
	}
	cfg := configWith(map[string]config.Service{
		"api": {Command: svc("-listen"), Ready: config.Ready{TCP: true}},
	})
	set, work := start(t, cfg, "")

	began := time.Now()
	err := set.Stop()

	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(began); elapsed >= stopTimeout {
		t.Errorf("Stop took %v, want a graceful exit well before the %v kill", elapsed, stopTimeout)
	}
	if log := readLog(t, work, "api"); !strings.Contains(log, "terminated\n") {
		t.Errorf("log = %q, want the service to have reported SIGTERM", log)
	}
}

func TestStop_KillsAServiceIgnoringSIGTERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no SIGTERM; Stop kills the service there")
	}
	cfg := configWith(map[string]config.Service{
		"api": {Command: svc("-listen -ignore-term"), Ready: config.Ready{TCP: true}},
	})
	set, _ := start(t, cfg, "")
	port := portOf(t, set, "PHOTON_PORT_API")

	began := time.Now()
	err := set.Stop()

	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(began); elapsed < stopTimeout {
		t.Errorf("Stop took %v, want the kill only after %v", elapsed, stopTimeout)
	}
	if connection, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", port)); err == nil {
		connection.Close() //nolint:errcheck // the test fails either way
		t.Error("service still accepts connections after Stop")
	}
}

func TestStop_ShellWrappedServiceLeavesNoProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are unix")
	}
	command := "'" + svcBinary + `' -listen & echo $! > "$PHOTON_WORK/child.pid"; wait`
	cfg := configWith(map[string]config.Service{
		"api": {Command: command, Ready: config.Ready{TCP: true}},
	})
	set, work := start(t, cfg, "")

	err := set.Stop()

	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	child, err := os.FindProcess(childPID(t, work))
	if err != nil {
		t.Fatalf("find the service's child: %v", err)
	}
	testkit.Eventually(t, goneTimeout, func() error {
		if child.Signal(syscall.Signal(0)) == nil {
			return fmt.Errorf("the service's child %d is still there", child.Pid)
		}
		return nil
	})
}

func TestStart_LineReadiness_ReturnsOnceTheLineIsOut(t *testing.T) {
	cfg := configWith(map[string]config.Service{
		"relay": {Command: svc("-delay 200ms -line 'relay listening'"), Ready: config.Ready{Line: `^relay listening$`}},
	})

	_, work := start(t, cfg, "")

	if log := readLog(t, work, "relay"); !strings.Contains(log, "relay listening\n") {
		t.Errorf("log = %q, want the ready line already written when Start returns", log)
	}
}

func TestStart_NotReady_NamesTheServiceAndItsLog(t *testing.T) {
	tests := []struct {
		name    string
		service config.Service
		want    string
	}{
		{"port never accepts", config.Service{Command: svc(""), Ready: config.Ready{TCP: true}, Timeout: shortTimeout},
			"not ready: service api within 300ms; its log is "},
		{"line never printed", config.Service{Command: svc("-line other"), Ready: config.Ready{Line: "^ready$"}, Timeout: shortTimeout},
			"not ready: service api within 300ms; its log is "},
		{"exits first", config.Service{Command: svc("-exit 3"), Ready: config.Ready{TCP: true}},
			"not ready: service api exited before it was ready (exit status 3); its log is "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := configWith(map[string]config.Service{"api": test.service})
			work := t.TempDir()

			set, err := Start(t.Context(), cfg, "", work)

			if set != nil {
				t.Error("Start returned a Set alongside its error")
			}
			if !errors.Is(err, ErrNotReady) {
				t.Fatalf("Start error = %v, want ErrNotReady", err)
			}
			if want := test.want + filepath.Join(work, "api.log"); err.Error() != want {
				t.Errorf("Start error = %q, want %q", err, want)
			}
		})
	}
}

func TestStart_Profile_ChoosesTheCommandAndEnv(t *testing.T) {
	cfg := configWith(map[string]config.Service{
		"api": {
			Command: svc("-line declared"),
			Env:     map[string]string{"SVC_MODE": "declared", "SVC_KEPT": "kept"},
			Ready:   config.Ready{Line: "^(declared|captured)$"},
		},
	})
	cfg.Profiles = map[string]config.Profile{
		"capture": {Services: map[string]config.ServiceOverride{
			"api": {Command: svc("-line captured"), Env: map[string]string{"SVC_MODE": "capture"}},
		}},
	}
	tests := []struct {
		profile string
		want    []string
	}{
		{"", []string{"SVC_KEPT=kept\n", "SVC_MODE=declared\n", "\ndeclared\n"}},
		{"capture", []string{"SVC_KEPT=kept\n", "SVC_MODE=capture\n", "\ncaptured\n"}},
	}
	for _, test := range tests {
		t.Run("profile "+strconv.Quote(test.profile), func(t *testing.T) {
			_, work := start(t, cfg, test.profile)

			log := readLog(t, work, "api")
			for _, want := range test.want {
				if !strings.Contains(log, want) {
					t.Errorf("log = %q, want it to hold %q", log, want)
				}
			}
		})
	}
}

func TestStart_UnknownProfile_NamesTheDeclaredOnes(t *testing.T) {
	tests := []struct {
		name     string
		profiles map[string]config.Profile
		want     string
	}{
		{"some declared", map[string]config.Profile{"replay": {}, "capture": {}}, `unknown profile "nope"; declared: capture, replay`},
		{"none declared", nil, `unknown profile "nope"; declared: none`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := configWith(map[string]config.Service{"api": {Command: svc("-listen"), Ready: config.Ready{TCP: true}}})
			cfg.Profiles = test.profiles

			_, err := Start(t.Context(), cfg, "nope", t.TempDir())

			if !errors.Is(err, ErrUnknownProfile) || err.Error() != test.want {
				t.Errorf("Start error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStart_ExportsPortsToLaterServices(t *testing.T) {
	webPort, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	cfg := configWith(map[string]config.Service{
		"api": {Command: svc("-listen"), Ready: config.Ready{TCP: true}},
		"web": {
			Command: svc("-listen"),
			Port:    webPort,
			Env:     map[string]string{"SVC_UPSTREAM": "http://localhost:${PHOTON_PORT_API}"},
			Ready:   config.Ready{TCP: true},
		},
	})

	set, work := start(t, cfg, "")

	apiPort := portOf(t, set, "PHOTON_PORT_API")
	wantEnv := []string{"PHOTON_PORT_API=" + apiPort, "PHOTON_PORT_WEB=" + strconv.Itoa(webPort)}
	if got := set.Env(); !slices.Equal(got, wantEnv) {
		t.Errorf("Env = %q, want %q", got, wantEnv)
	}
	webLog := readLog(t, work, "web")
	for _, want := range []string{
		"PHOTON_PORT=" + strconv.Itoa(webPort), "PHOTON_PORT_API=" + apiPort,
		"PHOTON_WORK=" + work, "SVC_UPSTREAM=http://localhost:" + apiPort,
	} {
		if !strings.Contains(webLog, want+"\n") {
			t.Errorf("web log = %q, want it to hold %q", webLog, want)
		}
	}
	if apiLog := readLog(t, work, "api"); strings.Contains(apiLog, "PHOTON_PORT_WEB=") {
		t.Errorf("api log = %q, want no port of a service started after it", apiLog)
	}
}

// configWith returns a config declaring services, run in the system temporary dir.
func configWith(services map[string]config.Service) *config.Config {
	return &config.Config{Dir: os.TempDir(), Services: services}
}

// svc returns a command line running the test service with args.
func svc(args string) string {
	return "exec '" + svcBinary + "' " + args
}

// start starts cfg's services under profile in a fresh work dir, stopping them when the test
// ends, and returns the set and the work dir.
func start(t *testing.T, cfg *config.Config, profile string) (*Set, string) {
	t.Helper()

	work := t.TempDir()
	set, err := Start(t.Context(), cfg, profile, work)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := set.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	return set, work
}

// portOf returns the port set.Env gives the named variable.
func portOf(t *testing.T, set *Set, name string) string {
	t.Helper()

	for _, entry := range set.Env() {
		if port, isMatch := strings.CutPrefix(entry, name+"="); isMatch {
			return port
		}
	}
	t.Fatalf("Env = %q, want an entry for %s", set.Env(), name)
	return ""
}

// greet connects to the service on port and returns the line it answers with.
func greet(t *testing.T, port string) string {
	t.Helper()

	connection, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Fatalf("dial the service: %v", err)
	}
	defer connection.Close() //nolint:errcheck // a read-side close reports nothing the read has not
	line, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil {
		t.Fatalf("read the greeting: %v", err)
	}
	return strings.TrimSuffix(line, "\n")
}

// readLog returns the named service's log in work.
func readLog(t *testing.T, work, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(work, name+logExtension))
	if err != nil {
		t.Fatalf("read the log: %v", err)
	}
	return string(data)
}

// childPID returns the pid a shell-wrapped service wrote to child.pid in work.
func childPID(t *testing.T, work string) int {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(work, "child.pid"))
	if err != nil {
		t.Fatalf("read the child's pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse the child's pid: %v", err)
	}
	return pid
}
