//go:build !unix && !windows

package service

import (
	"errors"
	"os"
	"os/exec"
)

// group is the service process alone: this platform has no way to stop a process tree, so a
// service is stopped as its own process.
type group struct {
	process *os.Process
}

// prepareGroup does nothing: there is no group to put the service in.
func prepareGroup(*exec.Cmd) {}

// joinGroup returns a group of process alone.
func joinGroup(process *os.Process) (*group, error) {
	return &group{process: process}, nil
}

// terminate asks the process to stop with an interrupt. Where the platform cannot deliver one,
// that is an error, and the caller kills the process instead.
func (g *group) terminate() error {
	if err := g.process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// kill kills the process. A process already gone is not an error.
func (g *group) kill() error {
	if err := g.process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// alive reports false: only the process itself is tracked, and its exit is what the caller
// waits for.
func (g *group) alive() bool {
	return false
}

// release does nothing: there is no resource to free.
func (g *group) release() error {
	return nil
}
