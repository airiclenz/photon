package tune

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

var (
	// ErrStale refuses a save while the storyboard on disk differs from the one the session
	// loaded or last saved: someone else wrote it, and [Session.Reload] must take it in first.
	ErrStale = errors.New("the storyboard changed on disk since it was loaded; reload it first")
	// ErrInvalid refuses a save while an error stands on a beat or the clip.
	ErrInvalid = errors.New("the storyboard has errors")
	// ErrNoUndo is Undo with no earlier version to go back to.
	ErrNoUndo = errors.New("nothing to undo")
	// ErrNoRedo is Redo with no undone version to go forward to.
	ErrNoRedo = errors.New("nothing to redo")
)

// tempPattern names the temporary file a save writes beside the storyboard before renaming it
// over the original; the `*` is the random part.
const tempPattern = ".*.tune"

// Session is one clip and take being tuned: the storyboard's current bytes, the pipeline built
// from them, the last good pipeline the preview keeps showing while an edit leaves an error
// standing, the edit history, and the on-disk baseline a save checks against.
//
// Every exported method takes the session's one lock, so the methods are safe to call from
// several goroutines; the lock guards the bytes, the pipelines, the history and the errors. An
// export keeps its progress under a lock of its own, so a render never waits on a rebuild.
type Session struct {
	mu sync.Mutex
	// path is the storyboard's file and takePath the take's.
	path     string
	takePath string
	// base is the clip as loaded: its config and `--work` flag carry over to every rebuild.
	base *clip.Clip
	// disk is the storyboard's bytes as last read from or written to path.
	disk []byte
	// takeStamp is the take file's size and modification time when it was last read.
	takeStamp fileStamp
	// takeErr is why the take could not be read again, or nil; it stands as a clip error.
	takeErr error
	// buildErr is the error the current bytes' build stopped at, or nil.
	buildErr error
	// recorded is the take every build schedules and composes.
	recorded *take.Take
	// buildPipeline is [render.Build]; a test wraps it to see what each build is given.
	buildPipeline func(
		*clip.Clip, string, *take.Take, render.BuildOptions,
	) (*render.Pipeline, error)
	history history
	// current is what the current bytes built, up to the stage that failed; good is the last
	// pipeline that built in full.
	current pipeline
	good    pipeline
	// problems are the errors standing on the current bytes, take and take size included; the
	// take size's comes last.
	problems []Problem
	// isStale says the storyboard changed on disk while edits were unsaved.
	isStale bool
	// version rises on every change to what [Session.State] reports.
	version int
	// photonBuild is the build string of the photon serving the page; [Run] sets it before serving.
	photonBuild string
	// runContext ends when the tuning run does, and an export's render with it; [Run] sets it
	// before serving, and [NewSession] starts it as context.Background.
	runContext context.Context
	// renderClip is [render.Run]; a test wraps it to hold an export running or see its error.
	renderClip func(context.Context, *clip.Clip, render.Options, io.Writer, io.Writer) error
	// export is the session's one GIF export, running or last finished.
	export exportJob
}

// pipeline is one version of the storyboard as the preview builds on it: the board parsed, the
// clip cross-checked, and the render pipeline every frame, screen, cell and rect of the preview
// comes from. A pipeline that failed to build holds the stages before the failure and nil for the
// rest; frames is nil when the take does not schedule, and partial when a later stage failed.
// loaded is the clip as the storyboard has it; when the take was recorded at another size than
// its frame, sizeErr says so and frames is built on the take's size instead, so the preview keeps
// drawing the cells the take holds — but for a terminal take under a url-launch clip, which draws
// a page's frames from an image layer the take does not hold: that pipeline builds no frames,
// built in full all the same, and undrawn holds the take's schedule in their place.
type pipeline struct {
	board   *storyboard.Storyboard
	loaded  *clip.Clip
	frames  *render.Pipeline
	undrawn *schedule.Schedule
	sizeErr error
}

// clipSchedule is the output clock p was built on: its frames', set on partial frames too, or
// undrawn's for a take the clip cannot draw; nil when the take does not schedule.
func (p pipeline) clipSchedule() *schedule.Schedule {
	if p.frames != nil {
		return p.frames.Schedule()
	}
	return p.undrawn
}

// fileStamp is what tells a rewritten take apart without reading it.
type fileStamp struct {
	size    int64
	modTime time.Time
}

// NewSession starts tuning loaded against the take at takePath, or at the clip's take path when
// takePath is empty. It fails when the take or the storyboard cannot be read, and when the
// storyboard does not schedule or compose against the take, just as `photon render` would. A take
// recorded at another size than the storyboard's frame does not fail it: the session opens
// previewing at the take's size, with that standing as a clip problem a save allows; a terminal
// take under a url-launch clip opens so too, with no preview ([ErrNoPreview]).
func NewSession(loaded *clip.Clip, takePath string) (*Session, error) {
	if takePath == "" {
		var err error
		if takePath, err = loaded.Take(); err != nil {
			return nil, err
		}
	}
	// The stamp is taken before the read, so a take rewritten in between reloads on Refresh.
	stamp, err := stampOf(takePath)
	if err != nil {
		return nil, err
	}
	recorded, err := take.Load(takePath)
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(loaded.Storyboard.Path)
	if err != nil {
		return nil, fmt.Errorf("read the storyboard: %w", err)
	}

	s := &Session{
		path:          loaded.Storyboard.Path,
		takePath:      takePath,
		base:          loaded,
		disk:          src,
		takeStamp:     stamp,
		recorded:      recorded,
		buildPipeline: render.Build,
		history:       newHistory(src),
		runContext:    context.Background(),
		renderClip:    render.Run,
	}
	built, err := s.build(src)
	if err != nil {
		return nil, err
	}
	s.settle(built, nil)
	return s, nil
}

// Apply splices edit into the current storyboard and rebuilds the pipeline on the result. An
// edit [storyboard.Apply] refuses — misspelled, aimed at a missing beat, camera position, zoom or
// caption, or leaving the storyboard invalid — changes nothing and is returned. An accepted edit
// is kept in the history even when the rebuild fails: the error then stands on its beat or the
// clip until an edit, undo or revert clears it, and the preview keeps the last good pipeline.
func (s *Session) Apply(edit Edit) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	edited, _, err := storyboard.Apply(s.history.current(), s.path, edit)
	if err != nil {
		return err
	}
	if bytes.Equal(edited, s.history.current()) {
		return nil
	}
	s.history.push(edited)
	s.rebuild()
	return nil
}

// Undo steps back to the version before the current one, or returns [ErrNoUndo].
func (s *Session) Undo() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.history.canUndo() {
		return ErrNoUndo
	}
	s.history.at--
	s.rebuild()
	return nil
}

// Redo steps forward to the version the last undo left, or returns [ErrNoRedo].
func (s *Session) Redo() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.history.canRedo() {
		return ErrNoRedo
	}
	s.history.at++
	s.rebuild()
	return nil
}

// Revert goes back to the storyboard as it is on disk, discarding the unsaved edits as one more
// version, so an undo brings them back. With nothing unsaved it does nothing.
func (s *Session) Revert() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isDirty() {
		return
	}
	s.history.push(s.disk)
	s.rebuild()
}

// Refresh takes in what changed on disk since the session last looked. A rewritten take is
// read again and the pipeline rebuilt on it; a take that cannot be read stands as a clip error
// until it is rewritten again. A storyboard changed on disk is loaded, history dropped, when
// nothing is unsaved; with edits unsaved the session turns stale instead, and a save is refused
// until [Session.Reload]. It fails only when the storyboard cannot be read.
func (s *Session) Refresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	onDisk, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read the storyboard: %w", err)
	}
	isTakeChanged := s.refreshTake()
	isBoardChanged := !bytes.Equal(onDisk, s.disk)
	if isBoardChanged && !s.isDirty() {
		s.adopt(onDisk)
		return nil
	}
	// Stale while the disk differs; an outside write undone again lifts it.
	if isBoardChanged != s.isStale {
		s.isStale = isBoardChanged
		s.version++
	}
	if isTakeChanged {
		s.rebuild()
	}
	return nil
}

// Reload loads the storyboard from disk, dropping the history and any unsaved edits, and clears
// stale. It fails only when the storyboard cannot be read.
func (s *Session) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	onDisk, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read the storyboard: %w", err)
	}
	s.adopt(onDisk)
	return nil
}

// Save writes the current storyboard over the file in place: a temporary file beside it, given
// the file's mode, renamed over it. It refuses with [ErrStale] when the file on disk is not the
// one the session loaded or last saved, and with [ErrInvalid] while an error other than the take's
// size against the frame stands or the bytes fail validation again: a frame of another size is
// saved, and the take re-recorded at it after. With nothing unsaved it writes nothing. The bytes
// written become the on-disk baseline, so the next Refresh sees no change.
func (s *Session) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkDisk(); err != nil {
		return err
	}
	// The take size's problem comes last, so the first problem is one of the errors that block.
	if s.buildErr != nil || s.takeErr != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, s.problems[0].Message)
	}
	if !s.isDirty() {
		return nil
	}
	src := s.history.current()
	if err := s.validate(src); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := writeInPlace(s.path, src); err != nil {
		return err
	}
	s.disk = src
	s.version++
	return nil
}

// checkDisk reads the storyboard on disk again and refuses with [ErrStale], turning the session
// stale, when it is not the one the session loaded or last saved. It fails too when the storyboard
// cannot be read.
func (s *Session) checkDisk() error {
	onDisk, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read the storyboard: %w", err)
	}
	if bytes.Equal(onDisk, s.disk) {
		return nil
	}
	if !s.isStale {
		s.isStale = true
		s.version++
	}
	return ErrStale
}

// validate checks src once more as the storyboard loader and the clip's cross-check do.
func (s *Session) validate(src []byte) error {
	board, err := storyboard.Parse(src, s.path)
	if err != nil {
		return err
	}
	_, err = s.base.WithStoryboard(board)
	return err
}

// adopt makes src, the storyboard just read from disk, the baseline and the only version.
func (s *Session) adopt(src []byte) {
	s.disk = src
	s.history = newHistory(src)
	s.isStale = false
	s.rebuild()
}

// isDirty says the current bytes are not the ones on disk.
func (s *Session) isDirty() bool {
	return !bytes.Equal(s.history.current(), s.disk)
}

// refreshTake reads the take again when its file changed, and says whether it did.
func (s *Session) refreshTake() bool {
	// A take that cannot be stat'ed stamps zero, so it is retried only once it is back.
	stamp, err := stampOf(s.takePath)
	if stamp == s.takeStamp {
		return false
	}
	s.takeStamp = stamp
	var recorded *take.Take
	if err == nil {
		recorded, err = take.Load(s.takePath)
	}
	if err != nil {
		s.takeErr = fmt.Errorf("reload the take: %w", err)
		return true
	}
	s.recorded, s.takeErr = recorded, nil
	return true
}

// rebuild builds the pipeline on the current bytes, keeps it as the good one when it built in
// full, and records the errors standing.
func (s *Session) rebuild() {
	s.settle(s.build(s.history.current()))
	s.version++
}

// settle makes built, what the current bytes built up to err, the current pipeline, and the good
// one too when it built in full, and records the errors standing on it: err, the take's and the
// take size's, in that order.
func (s *Session) settle(built pipeline, err error) {
	s.current, s.buildErr = built, err
	if err == nil {
		s.good = built
	}
	// A storyboard that no longer parses still has its problems shown on the beats, camera
	// positions, zooms and captions last seen, or on the clip for those it adds.
	board := built.board
	if board == nil {
		board = s.good.board
	}
	s.problems = problemsOf(errors.Join(err, s.takeErr, built.sizeErr), board)
}

// build parses src and builds the render pipeline on it against the take. On an error the
// pipeline returned holds the stages built before it. The build is given the good pipeline to
// reuse, so its rasterizer, raster cache, draft rasterizer, draft raster cache and caption faces
// carry over where render finds them unchanged, and the caption fonts load once a session. A take
// recorded at another size than the storyboard's frame is no build error: the pipeline is built
// at the take's size and the mismatch kept as the pipeline's sizeErr. A url-launch clip cannot
// draw a terminal take, one with no cell size, at any size: its pipeline builds no frames, only
// the take's schedule, whose error is the build's.
func (s *Session) build(src []byte) (pipeline, error) {
	var built pipeline
	board, err := storyboard.Parse(src, s.path)
	if err != nil {
		return built, err
	}
	built.board = board
	if built.loaded, err = s.base.WithStoryboard(board); err != nil {
		return built, err
	}
	previewed := built.loaded
	if sizeErr := render.CheckFrame(s.takePath, s.recorded, board.Frame); sizeErr != nil {
		built.sizeErr = fmt.Errorf("%w with `photon record`", sizeErr)
		if built.loaded.Config.IsBrowser() && s.recorded.Cell == (take.CellSize{}) {
			built.undrawn, err = schedule.New(board, s.recorded)
			return built, err
		}
		previewed = atTakeSize(built.loaded, s.recorded)
	}
	built.frames, err = s.buildPipeline(previewed, s.takePath, s.recorded, render.BuildOptions{
		Cache:      newRasterCache(rasterCacheBytes),
		DraftCache: newRasterCache(draftCacheBytes),
		Reuse:      s.good.frames,
	})
	return built, err
}

// atTakeSize returns a copy of loaded whose storyboard frame is recorded's size, the one the take's
// cells were laid out at: for a take recorded from a page, one that holds a cell size, the
// viewport of its grid at the device scale of its cell, else cols × rows; loaded itself is left as
// it is.
func atTakeSize(loaded *clip.Clip, recorded *take.Take) *clip.Clip {
	board := *loaded.Storyboard
	if recorded.Cell != (take.CellSize{}) {
		board.Frame.Viewport = storyboard.Viewport{Width: recorded.Cols, Height: recorded.Rows}
		board.Frame.DeviceScale = recorded.Cell.W
	} else {
		board.Frame.Cols, board.Frame.Rows = recorded.Cols, recorded.Rows
	}
	previewed := *loaded
	previewed.Storyboard = &board
	return &previewed
}

// stampOf reads the size and modification time of the file at path.
func stampOf(path string) (fileStamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}, fmt.Errorf("stat the take: %w", err)
	}
	return fileStamp{size: info.Size(), modTime: info.ModTime()}, nil
}

// writeInPlace replaces the file at path with src through a temporary file in the same
// directory, renamed over it, so a reader never sees half a file; the file keeps its mode.
func writeInPlace(path string, src []byte) (err error) {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("save the storyboard: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+tempPattern)
	if err != nil {
		return fmt.Errorf("save the storyboard: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, removeTemp(temp.Name()))
		}
	}()
	_, err = temp.Write(src)
	err = errors.Join(err, temp.Chmod(info.Mode()&fs.ModePerm), temp.Close())
	if err != nil {
		return fmt.Errorf("save the storyboard: %w", err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("save the storyboard: %w", err)
	}
	return nil
}

// removeTemp removes a save's temporary file after the save failed.
func removeTemp(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the temporary file: %w", err)
	}
	return nil
}
