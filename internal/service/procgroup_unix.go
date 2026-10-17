//go:build unix

package service

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// group is a service's own process group, led by the process the shell runs in.
type group struct {
	process *os.Process
}

// prepareGroup makes cmd the leader of a new process group, so that a service a shell wraps, and
// whatever it forks, is stopped as one.
func prepareGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// joinGroup returns the group process leads; prepareGroup already made it the leader.
func joinGroup(process *os.Process) (*group, error) {
	return &group{process: process}, nil
}

// terminate asks the whole group to stop with SIGTERM, falling back to the process alone when
// the group cannot be signalled. A process already gone is not an error; one that cannot be
// signalled is, and the caller kills it instead.
func (g *group) terminate() error {
	if syscall.Kill(-g.process.Pid, syscall.SIGTERM) == nil {
		return nil
	}
	if err := g.process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// kill sends SIGKILL to the whole group, falling back to the process alone. A group or process
// that is already gone is not an error.
func (g *group) kill() error {
	groupErr := syscall.Kill(-g.process.Pid, syscall.SIGKILL)
	if groupErr == nil || errors.Is(groupErr, syscall.ESRCH) {
		return nil
	}
	if err := g.process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return errors.Join(groupErr, err)
	}
	return nil
}

// alive reports whether any process of the group is still there; the leader may have exited
// while a child it forked lives on.
func (g *group) alive() bool {
	return !errors.Is(syscall.Kill(-g.process.Pid, 0), syscall.ESRCH)
}

// release does nothing: a process group holds no resource of its own.
func (g *group) release() error {
	return nil
}
