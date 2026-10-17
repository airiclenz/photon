//go:build !unix

package service

import (
	"errors"
	"os"
	"os/exec"
)

// ownGroup does nothing off unix, which has no process group to put the service in; a service
// is stopped as its own process alone.
func ownGroup(*exec.Cmd) {}

// terminate asks the service to stop with an interrupt. Where the platform cannot deliver one
// (Windows) that is an error, and the caller kills the service instead.
func terminate(process *os.Process) error {
	if err := process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// kill kills the service process. A process already gone is not an error.
func kill(process *os.Process) error {
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// groupAlive reports false: off unix only the service process itself is tracked, and its exit
// is what the caller waits for.
func groupAlive(*os.Process) bool {
	return false
}
