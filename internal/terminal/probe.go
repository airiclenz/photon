//go:build !windows

package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// probeBinary is the program the EPERM probe launches: it starts, does nothing and exits 0, so
// any failure is the launch's.
const probeBinary = "true"

// noProbePTY is the probe's report on a launch that failed before it opened a pty.
const noProbePTY = "no probe: the launch failed before it opened a pty"

// launch is one attempt at starting the program: the command, the pty's master and the
// parent's copy of its slave, and how the start went. A field is nil when the attempt failed
// before it existed.
type launch struct {
	cmd    *exec.Cmd
	master *os.File
	tty    *os.File
	err    error
}

// keep closes the parent's copy of the slave of a launch that started and hands back the running
// command and the master, so once the program and everything it started are gone, reading the
// master ends. A slave that does not close leaves a launch nothing can record: the program is
// killed and the error returned.
func (l launch) keep() (*exec.Cmd, *os.File, error) {
	if l.tty == nil {
		return l.cmd, l.master, nil
	}
	if err := l.tty.Close(); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("close the parent's end of the pty: %w", err),
			abandon(l.cmd), closeIfOpen(l.master))
	}
	return l.cmd, l.master, nil
}

// discard closes both ends of the pty of a launch that failed and returns its error, joined with
// any close that failed too.
func (l launch) discard() error {
	return errors.Join(l.err, closeIfOpen(l.tty), closeIfOpen(l.master))
}

// closeIfOpen closes f unless it is nil.
func closeIfOpen(f *os.File) error {
	if f == nil {
		return nil
	}
	return f.Close()
}

// startRetryingEPERM runs start up to attempts times, pausing gap between tries, for as long as
// it fails with EPERM. A launch right after another is now and then refused that way
// (`fork/exec /bin/sh: operation not permitted`) without the cause reproducing on demand. A
// fork/exec error comes from the child before it runs the program, so nothing has run twice, and
// start builds a fresh command on a fresh pty each time. Any other error is returned as it is.
// The last EPERM carries probe's report on the slave it was refused on (see [probeLaunches]), so
// the error names the step the kernel refused.
//
// start hands back the slave it launched on, open, and startRetryingEPERM closes it; on failure it
// closes the master too and returns neither.
func startRetryingEPERM(start func() launch, probe func(tty *os.File) string,
	attempts int, gap time.Duration,
) (*exec.Cmd, *os.File, error) {
	for attempt := 1; ; attempt++ {
		attemptLaunch := start()
		if attemptLaunch.err == nil {
			return attemptLaunch.keep()
		}
		isRefused := errors.Is(attemptLaunch.err, syscall.EPERM)
		isLast := attempt >= attempts
		if isRefused && isLast {
			attemptLaunch.err = fmt.Errorf("%w; %s", attemptLaunch.err, probe(attemptLaunch.tty))
		}
		err := attemptLaunch.discard()
		if !isRefused || isLast {
			return nil, nil, err
		}
		time.Sleep(gap)
	}
}

// launchProbe is what [probeLaunches] found on a pty a launch was refused on: whether `true`
// starts on it at all, as a session leader, and as a session leader taking it as controlling
// terminal (the three steps of a launch), and which processes already hold it as theirs. Plain
// failing means the environment refuses clone or execve outright; only setctty failing means the
// pty belongs to another session.
type launchProbe struct {
	tty     string
	plain   error
	setsid  error
	setctty error
	// owners are the processes whose controlling terminal the pty is; ownersErr says why they
	// could not be listed.
	owners    []string
	ownersErr error
}

// probeReport is [probeLaunches]' report on tty, as one clause of a launch error. A launch that
// failed before it opened a pty has nothing to probe, and says so.
func probeReport(tty *os.File) string {
	if tty == nil {
		return noProbePTY
	}
	return probeLaunches(tty).String()
}

// probeLaunches runs `true` on tty three ways, with no attributes, with Setsid, and with Setsid
// and Setctty, after first listing the processes that hold tty as their controlling terminal, so
// the list shows the pty as the refused launch found it. The Setctty launch runs last: on macOS
// the pty is revoked once that controlling process exits, so a pty is probed only once.
func probeLaunches(tty *os.File) launchProbe {
	probe := launchProbe{tty: tty.Name()}
	probe.owners, probe.ownersErr = ttyOwners(tty)
	binary, err := exec.LookPath(probeBinary)
	if err != nil {
		probe.plain, probe.setsid, probe.setctty = err, err, err
		return probe
	}
	run := func(attrs *syscall.SysProcAttr) error {
		cmd := exec.Command(binary)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
		cmd.SysProcAttr = attrs
		return cmd.Run()
	}
	probe.plain = run(nil)
	probe.setsid = run(&syscall.SysProcAttr{Setsid: true})
	probe.setctty = run(&syscall.SysProcAttr{Setsid: true, Setctty: true})
	return probe
}

// String is the probe as one clause of a launch error.
func (p launchProbe) String() string {
	outcome := func(err error) string {
		if err == nil {
			return "ok"
		}
		return "refused (" + err.Error() + ")"
	}
	owners := "none"
	switch {
	case p.ownersErr != nil:
		owners = "unknown (" + p.ownersErr.Error() + ")"
	case len(p.owners) > 0:
		owners = strings.Join(p.owners, ", ")
	}
	return fmt.Sprintf("probe on %s: true %s, true+setsid %s, true+setsid+setctty %s; controlling tty of: %s",
		p.tty, outcome(p.plain), outcome(p.setsid), outcome(p.setctty), owners)
}
