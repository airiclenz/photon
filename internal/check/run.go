package check

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The variables photon hands every `run` expect.
const (
	// envWork names the absolute work dir the command runs in.
	envWork = "PHOTON_WORK"
	// envTake names the absolute path of the take being judged.
	envTake = "PHOTON_TAKE"
	// envBeat names the id of the beat whose expect this is; a take-level expect has none.
	envBeat = "PHOTON_BEAT"
)

const (
	// shell and shellFlag run an expect's command line, like every command photon runs.
	shell     = "sh"
	shellFlag = "-c"
	// workDirMode is the permission a work dir photon creates gets.
	workDirMode = 0o755
	// outputWaitDelay bounds how long a finished command's output is waited for, so a background
	// process it left holding the output open cannot hang the check.
	outputWaitDelay = time.Second
	// passedDetail is a passing command's detail when it printed nothing.
	passedDetail = "exit 0"
)

// runner runs `run` expects: each in the absolute work dir, with the caller's extra env and
// photon's own variables.
type runner struct {
	work     string
	takePath string
	extra    []string
}

// newRunner makes work and takePath absolute against the working directory, so a command in the
// work dir reads the same take whatever directory photon was started in.
func newRunner(work, takePath string, extra []string) (*runner, error) {
	absoluteWork, err := filepath.Abs(work)
	if err != nil {
		return nil, fmt.Errorf("resolve the work dir %s: %w", work, err)
	}
	absoluteTake, err := filepath.Abs(takePath)
	if err != nil {
		return nil, fmt.Errorf("resolve the take %s: %w", takePath, err)
	}
	return &runner{work: absoluteWork, takePath: absoluteTake, extra: extra}, nil
}

// makeWork creates the work dir, so a `run` expect has a dir to run in even when no take was
// recorded there.
func (r *runner) makeWork() error {
	if err := os.MkdirAll(r.work, workDirMode); err != nil {
		return fmt.Errorf("create the work dir: %w", err)
	}
	return nil
}

// run runs one `run` expect's command, for beat (nil for a take-level expect), and judges it:
// PASS on exit 0, FAIL otherwise, the trimmed combined output the detail. A command that exits
// non-zero silently gets its exit status as the detail; a shell that cannot be started at all,
// such as no `sh` on PATH, is a FAIL naming it.
func (r *runner) run(ctx context.Context, command string, beat *int) (Verdict, string) {
	cmd := exec.CommandContext(ctx, shell, shellFlag, command)
	cmd.Dir = r.work
	cmd.Env = r.env(beat)
	cmd.WaitDelay = outputWaitDelay
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()

	detail := strings.TrimSpace(output.String())
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return Fail, fmt.Sprintf("run: %s not found on PATH: %v", shell, err)
	case cmd.ProcessState == nil:
		return Fail, fmt.Sprintf("run: start %s: %v", shell, err)
	case cmd.ProcessState.Success():
		if detail == "" {
			detail = passedDetail
		}
		return Pass, detail
	}
	if detail == "" {
		detail = cmd.ProcessState.String()
	}
	return Fail, detail
}

// env is the command's environment: photon's own, then the caller's extra entries, then
// PHOTON_WORK, PHOTON_TAKE and, for a beat's expect, PHOTON_BEAT. Any inherited or extra entry
// for one of those three is dropped first, so a take-level expect never sees a stale beat id.
func (r *runner) env(beat *int) []string {
	inherited := append(os.Environ(), r.extra...)
	env := make([]string, 0, len(inherited)+3)
	for _, entry := range inherited {
		name, _, _ := strings.Cut(entry, "=")
		if name == envWork || name == envTake || name == envBeat {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, envWork+"="+r.work, envTake+"="+r.takePath)
	if beat != nil {
		env = append(env, envBeat+"="+strconv.Itoa(*beat))
	}
	return env
}
