package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/testkit"
)

// watchTimeout bounds every wait on a watch: a round needs two polls to settle plus the render.
const watchTimeout = 10 * time.Second

// linesPerDryRun is what one dry-run round prints on stdout: the two ffmpeg command lines.
const linesPerDryRun = 2

// lockedBuffer is a bytes.Buffer a running watch writes to while the test reads it.
type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// watchProject is a project dir with the render tests' config and storyboard and a work dir
// holding the clip's default take.
type watchProject struct {
	config     string
	storyboard string
	take       string
	work       string
}

// newWatchProject writes a watchProject; without a config, the project dir has no photon.yaml.
func newWatchProject(t *testing.T, hasConfig bool) watchProject {
	t.Helper()
	dir := t.TempDir()
	project := watchProject{config: filepath.Join(dir, "photon.yaml"), work: t.TempDir()}
	if hasConfig {
		writeFile(t, dir, "photon.yaml", renderConfig)
	}
	project.storyboard = writeFile(t, dir, "rendered.yaml", renderStoryboard)
	project.take = saveRenderTake(t, project.work, 20)
	return project
}

// watching is a Watch running in the background over a project, as a dry run.
type watching struct {
	stdout *lockedBuffer
	stderr *lockedBuffer
	cancel context.CancelFunc
	done   chan error
}

// startWatch runs Watch over project until the test ends.
func startWatch(t *testing.T, project watchProject) *watching {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	running := &watching{
		stdout: &lockedBuffer{}, stderr: &lockedBuffer{}, cancel: cancel, done: make(chan error, 1),
	}
	inputs := WatchInputs{Storyboard: project.storyboard, WorkFlag: project.work}
	go func() {
		running.done <- Watch(ctx, inputs, Options{DryRun: true}, running.stdout, running.stderr)
	}()
	t.Cleanup(func() {
		cancel()
		<-running.done
	})
	return running
}

// waitForRounds waits until the watch has printed rounds dry runs on stdout.
func (w *watching) waitForRounds(t *testing.T, rounds int) {
	t.Helper()
	testkit.Eventually(t, watchTimeout, func() error {
		got := strings.Count(w.stdout.String(), "\n")
		if got != rounds*linesPerDryRun {
			return fmt.Errorf("want %d stdout lines, got %d; stderr:\n%s",
				rounds*linesPerDryRun, got, w.stderr.String())
		}
		return nil
	})
}

// waitForError waits until the watch has reported a failure on stderr.
func (w *watching) waitForError(t *testing.T) {
	t.Helper()
	testkit.Eventually(t, watchTimeout, func() error {
		if !strings.HasPrefix(w.stderr.String(), "photon: ") {
			return fmt.Errorf("want a `photon: ` error on stderr, got %q", w.stderr.String())
		}
		return nil
	})
}

// appendToFile appends a comment line to path, which changes its size.
func appendToFile(t *testing.T, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	_, writeErr := file.WriteString("\n# edited\n")
	if err := errors.Join(writeErr, file.Close()); err != nil {
		t.Fatalf("append to %s: %v", path, err)
	}
}

// rewriteTake saves the take at path again with its last screen grown by a cell.
func rewriteTake(t *testing.T, path string) {
	t.Helper()
	recorded, err := take.Load(path)
	if err != nil {
		t.Fatalf("take.Load: %v", err)
	}
	last := len(recorded.Snapshots) - 1
	recorded.Snapshots[last] = textSnapshot(recorded.Snapshots[last].At, "Tests PASS", "done!")
	if err := take.Save(path, recorded); err != nil {
		t.Fatalf("take.Save: %v", err)
	}
}

func TestWatch_AnInputRewrite_RendersAgain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rewrite func(t *testing.T, project watchProject)
	}{
		{"the storyboard", func(t *testing.T, p watchProject) { appendToFile(t, p.storyboard) }},
		{"the config", func(t *testing.T, p watchProject) { appendToFile(t, p.config) }},
		{"the take", func(t *testing.T, p watchProject) { rewriteTake(t, p.take) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			project := newWatchProject(t, true)
			running := startWatch(t, project)
			running.waitForRounds(t, 1)

			tt.rewrite(t, project)

			running.waitForRounds(t, 2)
			if stderr := running.stderr.String(); stderr != "" {
				t.Errorf("want nothing on stderr, got %q", stderr)
			}
		})
	}
}

func TestWatch_AConfigFixedAfterAFailedLoad_Renders(t *testing.T) {
	t.Parallel()
	project := newWatchProject(t, false)
	running := startWatch(t, project)
	running.waitForError(t)

	writeFile(t, filepath.Dir(project.config), filepath.Base(project.config), renderConfig)

	running.waitForRounds(t, 1)
}

func TestWatch_ABrokenStoryboard_IsReportedAndItsFixRenders(t *testing.T) {
	t.Parallel()
	project := newWatchProject(t, true)
	running := startWatch(t, project)
	running.waitForRounds(t, 1)
	dir, name := filepath.Dir(project.storyboard), filepath.Base(project.storyboard)

	writeFile(t, dir, name, "clip: [\n")
	running.waitForError(t)
	writeFile(t, dir, name, renderStoryboard)

	running.waitForRounds(t, 2)
}

func TestWatch_ContextCancel_ReturnsNil(t *testing.T) {
	t.Parallel()
	project := newWatchProject(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	inputs := WatchInputs{Storyboard: project.storyboard, WorkFlag: project.work}
	stdout := &lockedBuffer{}
	go func() { done <- Watch(ctx, inputs, Options{DryRun: true}, stdout, &lockedBuffer{}) }()
	testkit.Eventually(t, watchTimeout, func() error {
		if stdout.String() == "" {
			return fmt.Errorf("no first round yet")
		}
		return nil
	})

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Watch = %v, want nil", err)
		}
	case <-time.After(watchTimeout):
		t.Fatal("Watch did not return after its context ended")
	}
}
