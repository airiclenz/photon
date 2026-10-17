//go:build !windows

package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// testRetryGap is the pause between retried launches in the retry tests.
const testRetryGap = time.Millisecond

// openPipe returns both ends of a fresh pipe, closed when the test ends if nothing closed them.
func openPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("open a pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	return reader, writer
}

// isFileClosed reports whether f has been closed.
func isFileClosed(f *os.File) bool {
	_, err := f.Stat()
	return errors.Is(err, os.ErrClosed)
}

func TestStartRetryingEPERM(t *testing.T) {
	t.Parallel()
	refused := &os.PathError{Op: "fork/exec", Path: "/bin/sh", Err: syscall.EPERM}
	other := errors.New("no such pty")
	const report = "probe on /dev/pts/9: true ok"
	for _, test := range []struct {
		name       string
		failures   []error // what the attempts return before one starts
		wantCalls  int
		wantProbes int
		wantErr    error
	}{
		{name: "the first try starts", wantCalls: 1},
		{name: "EPERM then a start", failures: []error{refused}, wantCalls: 2},
		{name: "EPERM on every try", failures: []error{refused, refused, refused}, wantCalls: startAttempts,
			wantProbes: 1, wantErr: syscall.EPERM},
		{name: "another error is not retried", failures: []error{other}, wantCalls: 1, wantErr: other},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls, probes := 0, 0
			var handedBack []*os.File
			var started time.Time
			var gaps []time.Duration
			start := func() launch {
				if calls > 0 {
					gaps = append(gaps, time.Since(started))
				}
				started = time.Now()
				calls++
				if calls <= len(test.failures) {
					master, tty := openPipe(t)
					handedBack = append(handedBack, master, tty)
					return launch{master: master, tty: tty, err: fmt.Errorf("start: %w", test.failures[calls-1])}
				}
				return launch{cmd: &exec.Cmd{}}
			}
			probe := func(*os.File) string {
				probes++
				return report
			}

			cmd, _, err := startRetryingEPERM(start, probe, startAttempts, testRetryGap)

			if calls != test.wantCalls {
				t.Errorf("start ran %d time(s), want %d", calls, test.wantCalls)
			}
			if probes != test.wantProbes {
				t.Errorf("probe ran %d time(s), want %d", probes, test.wantProbes)
			}
			for index, gap := range gaps {
				if gap < testRetryGap {
					t.Errorf("retry %d came %s after the try before it, want at least %s", index+1, gap, testRetryGap)
				}
			}
			for index, file := range handedBack {
				if !isFileClosed(file) {
					t.Errorf("file %d a failed attempt handed back is still open", index)
				}
			}
			if test.wantErr == nil && (err != nil || cmd == nil) {
				t.Errorf("got (%v, %v), want the started command", cmd, err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Errorf("err = %v, want %v", err, test.wantErr)
			}
			if hasReport := err != nil && strings.Contains(err.Error(), report); hasReport != (test.wantProbes > 0) {
				t.Errorf("err = %v; carries the probe report: %v, want %v", err, hasReport, test.wantProbes > 0)
			}
		})
	}
}

func TestStartRetryingEPERM_ClosesTheSlaveOfAStartedLaunch(t *testing.T) {
	t.Parallel()
	master, tty := openPipe(t)
	start := func() launch { return launch{cmd: &exec.Cmd{}, master: master, tty: tty} }
	_, gotMaster, err := startRetryingEPERM(start, probeReport, startAttempts, testRetryGap)
	if err != nil {
		t.Fatalf("startRetryingEPERM: %v", err)
	}
	if gotMaster != master || isFileClosed(master) {
		t.Error("the master was not handed back open")
	}
	if !isFileClosed(tty) {
		t.Error("the parent's copy of the slave is still open")
	}
}

// TestProbeLaunches_OnAHealthyPTY runs serially, before the parallel tests start: macOS reuses
// pty numbers and revokes a pty when its session leader exits, so a parallel test's child exiting
// late can revoke the pty this test was just handed, and its Setctty launch then fails with EBADF.
func TestProbeLaunches_OnAHealthyPTY(t *testing.T) {
	master, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open a pty: %v", err)
	}
	t.Cleanup(func() {
		_ = tty.Close()
		_ = master.Close()
	})

	probe := probeLaunches(tty)

	for step, err := range map[string]error{"plain": probe.plain, "setsid": probe.setsid, "setsid+setctty": probe.setctty} {
		if err != nil {
			t.Errorf("the %s launch failed: %v", step, err)
		}
	}
	if len(probe.owners) != 0 {
		t.Errorf("owners = %q, want none on a fresh pty", probe.owners)
	}
	if report := probeReport(tty); !strings.Contains(report, "true+setsid+setctty ok") {
		t.Errorf("report = %q, want every launch ok", report)
	}
}

func TestProbeReport_WithoutAPTY(t *testing.T) {
	t.Parallel()
	if got := probeReport(nil); got != noProbePTY {
		t.Errorf("report = %q, want %q", got, noProbePTY)
	}
}

func TestLaunchProbe_String(t *testing.T) {
	t.Parallel()
	refused := &os.PathError{Op: "fork/exec", Path: "/usr/bin/true", Err: syscall.EPERM}
	for _, test := range []struct {
		name  string
		probe launchProbe
		want  string
	}{
		{
			name:  "setctty refused, pty owned",
			probe: launchProbe{tty: "/dev/pts/3", setctty: refused, owners: []string{"pid 7 (bash) session 7"}},
			want: "probe on /dev/pts/3: true ok, true+setsid ok, true+setsid+setctty refused " +
				"(fork/exec /usr/bin/true: operation not permitted); controlling tty of: pid 7 (bash) session 7",
		},
		{
			name: "nothing starts, owner unknown",
			probe: launchProbe{tty: "/dev/ttys004", plain: refused, setsid: refused, setctty: refused,
				ownersErr: errors.New("owner unknown off linux")},
			want: "probe on /dev/ttys004: true refused (fork/exec /usr/bin/true: operation not permitted), " +
				"true+setsid refused (fork/exec /usr/bin/true: operation not permitted), " +
				"true+setsid+setctty refused (fork/exec /usr/bin/true: operation not permitted); " +
				"controlling tty of: unknown (owner unknown off linux)",
		},
		{
			name:  "every launch starts, nobody holds the pty",
			probe: launchProbe{tty: "/dev/pts/1"},
			want:  "probe on /dev/pts/1: true ok, true+setsid ok, true+setsid+setctty ok; controlling tty of: none",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.probe.String(); got != test.want {
				t.Errorf("report =\n %q\nwant\n %q", got, test.want)
			}
		})
	}
}
