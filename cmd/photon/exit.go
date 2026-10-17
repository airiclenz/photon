package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// The exit codes. A bad command line is told apart from a run that started and failed, so a
// script driving photon can react to each.
const (
	exitOK        = 0
	exitRunFailed = 1
	exitBadUsage  = 2
)

// runError marks a failure that happened after the command line was accepted.
type runError struct{ err error }

func (e runError) Error() string { return e.err.Error() }
func (e runError) Unwrap() error { return e.err }

// runE adapts a subcommand's body: usage stops being printed the moment the command line has
// been accepted, and every failure from there on is marked as a run failure rather than a usage
// one. Cobra's own flag and argument errors never pass through here, which is what makes them
// distinguishable.
func runE(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		if err := run(cmd, args); err != nil {
			return runError{err: err}
		}
		return nil
	}
}

// exitCodeFor reports the status an error asks for: none exits 0, a run that started and failed
// exits 1, and a command line that never started one exits 2.
func exitCodeFor(err error) int {
	if err == nil {
		return exitOK
	}
	var failed runError
	if errors.As(err, &failed) {
		return exitRunFailed
	}
	return exitBadUsage
}
