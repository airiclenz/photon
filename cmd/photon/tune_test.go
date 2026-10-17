package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/airiclenz/photon/internal/clip"
)

// tuneURLPrefix starts the line tune prints once it serves the page.
const tuneURLPrefix = "photon tune: http://127.0.0.1:"

func TestTune(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")

	board := writeFile(t, t.TempDir(), "demo.yaml", renderStoryboard)
	configElsewhere := writeFile(t, t.TempDir(), "photon.yaml", renderConfig)

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr []string
	}{
		{
			name:       "no storyboard is a bad command line",
			args:       []string{"tune"},
			wantCode:   exitBadUsage,
			wantStderr: []string{"accepts between 1 and 2 arg(s), received 0"},
		},
		{
			name:       "a third argument is a bad command line",
			args:       []string{"tune", board, "a.take", "b.take"},
			wantCode:   exitBadUsage,
			wantStderr: []string{"accepts between 1 and 2 arg(s), received 3"},
		},
		{
			name:       "a --port that is not a number is a bad command line",
			args:       []string{"tune", board, "--port", "http"},
			wantCode:   exitBadUsage,
			wantStderr: []string{`invalid argument "http" for "--port"`},
		},
		{
			name:     "a --port is parsed and a missing take is a failed run",
			args:     []string{"tune", board, "--port", "0", "--config", configElsewhere, "--work", t.TempDir()},
			wantCode: exitRunFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runLint(t, tt.args)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stdout %q, stderr %q)", code, tt.wantCode, stdout, stderr)
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q does not carry %q", stderr, want)
				}
			}
		})
	}
}

// TestTuneServesUntilStopped runs tune on a take in --work and stops it once it prints the
// page's address, as Ctrl-C would: the run ends with exit status 0.
func TestTuneServesUntilStopped(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")
	board := writeFile(t, t.TempDir(), "demo.yaml", renderStoryboard)
	configElsewhere := writeFile(t, t.TempDir(), "photon.yaml", renderConfig)
	work := t.TempDir()
	saveCheckTake(t, filepath.Join(work, "demo.take"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout := &lineSignal{cancel: cancel}
	root := newRootCommand()
	root.SetOut(stdout)
	var stderr bytes.Buffer

	code := execute(ctx, root, []string{"tune", board, "--config", configElsewhere, "--work", work}, &stderr)

	if code != exitOK {
		t.Errorf("exit code = %d, want %d (stderr %q)", code, exitOK, stderr.String())
	}
	if got := stdout.String(); !strings.HasPrefix(got, tuneURLPrefix) {
		t.Errorf("stdout = %q, want it to start with %q", got, tuneURLPrefix)
	}
}

// lineSignal is a stdout that cancels the run on its first write and keeps what was written.
type lineSignal struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	once   sync.Once
	cancel context.CancelFunc
}

func (w *lineSignal) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	w.once.Do(w.cancel)
	return n, err
}

// String returns everything written so far.
func (w *lineSignal) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}
