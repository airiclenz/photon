package tune

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
)

var (
	// ErrUnsaved refuses an export while the storyboard has edits not saved yet: the GIF is
	// rendered from the storyboard on disk.
	ErrUnsaved = errors.New("the storyboard has unsaved edits; save them first")
	// ErrProblems refuses an export while an error stands on the storyboard or the take.
	ErrProblems = errors.New("the storyboard has errors; fix them first")
	// ErrExporting refuses an export while the last one is still rendering.
	ErrExporting = errors.New("an export is already running")
)

// ExportState is the session's GIF export as `/api/state` sends it: whether it is running, the
// frames it has streamed through ffmpeg's two passes so far (Done) out of all of them (Total), and,
// once it has finished, the GIF it wrote (Out) or why it failed (Error), one of the two. A session
// that never exported reports it idle, every field zero.
type ExportState struct {
	IsRunning bool   `json:"running"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
	Out       string `json:"out"`
	Error     string `json:"error"`
}

// exportJob is a session's one export: its state under a lock of its own, so a render's progress
// never waits on the session's lock, and the render's goroutine while one runs.
type exportJob struct {
	mu        sync.Mutex
	state     ExportState
	rendering sync.WaitGroup
}

// Export starts rendering the GIF of the storyboard on disk to its ship path in the background,
// just as `photon render` renders it, against the session's take, and returns at once;
// [Session.State] reports its progress and how it ended. The render ends early when the run that
// serves the session ends ([Run]). Neither the export nor its progress changes the history, the
// unsaved bytes or the state's version.
//
// It refuses with [ErrStale] when the storyboard on disk is not the one the session loaded or last
// saved, with [ErrUnsaved] while edits are unsaved, with [ErrProblems] while an error stands, and
// with [ErrExporting] while an export is still running. It fails too when the storyboard cannot be
// read.
func (s *Session) Export() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkDisk(); err != nil {
		return err
	}
	if s.isDirty() {
		return ErrUnsaved
	}
	if len(s.problems) > 0 {
		return ErrProblems
	}
	// Clean and free of problems, the current bytes are the ones on disk and built in full, so
	// the good pipeline's clip is the saved one.
	if !s.export.start() {
		return ErrExporting
	}
	go s.runExport(s.runContext, s.renderClip, s.good.loaded, s.takePath)
	return nil
}

// runExport renders loaded against the take at takePath with renderClip under ctx, reporting its
// progress to the export, and records how it ended.
func (s *Session) runExport(
	ctx context.Context,
	renderClip func(context.Context, *clip.Clip, render.Options, io.Writer, io.Writer) error,
	loaded *clip.Clip,
	takePath string,
) {
	options := render.Options{Take: takePath, Progress: s.export.progress}
	err := renderClip(ctx, loaded, options, io.Discard, io.Discard)
	s.export.finish(loaded.Storyboard.Ship, err)
}

// waitExport returns once no export is running.
func (s *Session) waitExport() {
	s.export.rendering.Wait()
}

// snapshot is the export's state as it stands.
func (j *exportJob) snapshot() ExportState {
	j.mu.Lock()
	defer j.mu.Unlock()

	return j.state
}

// start marks a new export running, its progress and outcome cleared, and reports true; with one
// running already it changes nothing and reports false. A started export must be finished.
func (j *exportJob) start() bool {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.state.IsRunning {
		return false
	}
	j.state = ExportState{IsRunning: true}
	j.rendering.Add(1)
	return true
}

// finish records how the running export ended: out, the GIF written, or err's text.
func (j *exportJob) finish(out string, err error) {
	defer j.rendering.Done()
	j.mu.Lock()
	defer j.mu.Unlock()

	j.state.IsRunning = false
	if err != nil {
		j.state.Error = err.Error()
		return
	}
	j.state.Out = out
}

// progress records that done of total frames have streamed.
func (j *exportJob) progress(done, total int) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.state.Done, j.state.Total = done, total
}
