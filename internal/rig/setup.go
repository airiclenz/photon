package rig

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/airiclenz/photon/internal/clip"
)

const (
	// shell and shellFlag run every command line the rig runs, the launch included.
	shell     = "sh"
	shellFlag = "-c"
	// workDirMode is the permission a work dir the rig creates gets.
	workDirMode = 0o755
	// commandWaitDelay bounds how long a finished command's output is waited for, so a background
	// process it left holding the writer's pipe cannot hang the rig.
	commandWaitDelay = time.Second
)

// Setup runs the config's `setup` once, to build the rig: `sh -c <setup>` in the config's dir,
// with PHOTON_WORK set to the work dir, which is created first. Its output, both streams, goes to
// out. A config with no setup creates the work dir and runs nothing. A setup that exits non-zero
// is an error naming it.
func Setup(ctx context.Context, loaded *clip.Clip, out io.Writer) error {
	work, err := prepareWork(loaded)
	if err != nil {
		return err
	}
	return runCommand(ctx, "setup", loaded.Config.Setup, loaded.Config.Dir, workVars(work), out)
}

// prepareWork resolves the clip's work dir, absolute, and creates it, so setup, reset and the
// services all find it in place.
func prepareWork(loaded *clip.Clip) (string, error) {
	work, err := loaded.Work()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(work, workDirMode); err != nil {
		return "", fmt.Errorf("create the work dir: %w", err)
	}
	return work, nil
}

// workVars is the PHOTON_WORK entry every command the rig runs gets, over the operator's own.
func workVars(work string) []string {
	return []string{clip.WorkEnv + "=" + work}
}

// runCommand runs command, the config field named field, as `sh -c <command>` in dir with vars
// set over the operator's env and both output streams going to out. An empty command runs
// nothing.
func runCommand(ctx context.Context, field, command, dir string, vars []string, out io.Writer) error {
	if command == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, shell, shellFlag, command)
	cmd.Dir = dir
	cmd.Env = withVars(os.Environ(), vars)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = commandWaitDelay
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %q: %w", field, command, err)
	}
	return nil
}
