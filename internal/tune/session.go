package tune

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
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
// several goroutines; the lock guards the bytes, the pipelines, the history and the errors.
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
	// recorded is the take every build schedules and composes.
	recorded *take.Take
	// buildPipeline is [render.Build]; a test wraps it to see what each build is given.
	buildPipeline func(*clip.Clip, string, *take.Take, render.BuildOptions) (*render.Pipeline, error)
	history       history
	// current is what the current bytes built, up to the stage that failed; good is the last
	// pipeline that built in full.
	current pipeline
	good    pipeline
	// problems are the errors standing on the current bytes, take included.
	problems []Problem
	// isStale says the storyboard changed on disk while edits were unsaved.
	isStale bool
	// version rises on every change to what [Session.State] reports.
	version int
}

// pipeline is one version of the storyboard as the preview builds on it: the board parsed, the
// clip cross-checked, and the render pipeline every frame, screen, cell and rect of the preview
// comes from. A pipeline that failed to build holds the stages before the failure and nil for the
// rest; frames is nil when the take was recorded at another frame size or does not schedule, and
// partial when a later stage failed.
type pipeline struct {
	board  *storyboard.Storyboard
	loaded *clip.Clip
	frames *render.Pipeline
}

// fileStamp is what tells a rewritten take apart without reading it.
type fileStamp struct {
	size    int64
	modTime time.Time
}

// NewSession starts tuning loaded against the take at takePath, or at the clip's take path when
// takePath is empty. It fails when the take or the storyboard cannot be read, when the take was
// recorded at a size other than the storyboard's frame, and when the storyboard does not
// schedule or compose against the take, just as `photon render` would.
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
	}
	built, err := s.build(src)
	if err != nil {
		return nil, err
	}
	s.current, s.good = built, built
	return s, nil
}

// Apply splices edit into the current storyboard and rebuilds the pipeline on the result. An
// edit [storyboard.Apply] refuses — misspelled, aimed at a missing beat, camera move or caption,
// or leaving the storyboard invalid — changes nothing and is returned. An accepted edit is kept in the history even when the
// rebuild fails: the error then stands on its beat or the clip until an edit, undo or revert
// clears it, and the preview keeps the last good pipeline.
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
// one the session loaded or last saved, and with [ErrInvalid] while an error stands or the bytes
// fail validation again. With nothing unsaved it writes nothing. The bytes written become the
// on-disk baseline, so the next Refresh sees no change.
func (s *Session) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	onDisk, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read the storyboard: %w", err)
	}
	if !bytes.Equal(onDisk, s.disk) {
		if !s.isStale {
			s.isStale = true
			s.version++
		}
		return ErrStale
	}
	if len(s.problems) > 0 {
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
	built, err := s.build(s.history.current())
	s.current = built
	if err == nil {
		s.good = built
	}
	// A storyboard that no longer parses still has its problems shown on the beats, camera moves
	// and captions last seen, or on the clip for those it adds.
	board := built.board
	if board == nil {
		board = s.good.board
	}
	s.problems = problemsOf(errors.Join(err, s.takeErr), board)
	s.version++
}

// build parses src and builds the render pipeline on it against the take. On an error the
// pipeline returned holds the stages built before it. The build is given the good pipeline to
// reuse, so its rasterizer, raster cache and caption faces carry over where render finds them
// unchanged, and the caption fonts load once a session.
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
	built.frames, err = s.buildPipeline(built.loaded, s.takePath, s.recorded, render.BuildOptions{
		Cache: newRasterCache(rasterCacheBytes),
		Reuse: s.good.frames,
	})
	return built, err
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
