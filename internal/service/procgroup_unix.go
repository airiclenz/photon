//go:build unix

package service

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// ownGroup makes cmd the leader of a new process group, so that a service a shell wraps, and
// whatever it forks, is stopped as one.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminate asks the service's whole group to stop with SIGTERM, falling back to the process
// alone when the group cannot be signalled. A process already gone is not an error; one that
// cannot be signalled is, and the caller kills it instead.
func terminate(process *os.Process) error {
	if syscall.Kill(-process.Pid, syscall.SIGTERM) == nil {
		return nil
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// kill sends SIGKILL to the service's whole group, falling back to the process alone. A group
// or process that is already gone is not an error.
func kill(process *os.Process) error {
	groupErr := syscall.Kill(-process.Pid, syscall.SIGKILL)
	if groupErr == nil || errors.Is(groupErr, syscall.ESRCH) {
		return nil
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return errors.Join(groupErr, err)
	}
	return nil
}

// groupAlive reports whether any process of the service's group is still there; the leader may
// have exited while a child it forked lives on.
func groupAlive(process *os.Process) bool {
	return !errors.Is(syscall.Kill(-process.Pid, 0), syscall.ESRCH)
}
