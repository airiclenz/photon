package render

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/config"
)

// watchInterval is how often Watch stats the files a render was made from.
const watchInterval = 500 * time.Millisecond

// watchErrorFormat reports a failed round on stderr the way photon reports any failure.
const watchErrorFormat = "photon: %v\n"

// WatchInputs names what a watch loads its clip from, as [clip.Load] takes them: the storyboard's
// path and the `--config` and `--work` flags, an empty flag being unset.
type WatchInputs struct {
	Storyboard string
	ConfigFlag string
	WorkFlag   string
}

// Watch loads the clip and renders it as [Run] does, then again each time a file that round was
// made from changes: the storyboard, the config, the take ([Options.Take], else the clip's
// default take) and the config's caption font files. Each round loads the clip afresh, so a config
// edit that moves the work dir moves the watched take with it. A change is rendered once the files
// have held still across two polls, [watchInterval] apart, so a save written in parts renders
// once. Files are told apart by size and modification time; a missing file counts as one more
// state, so its return is a change.
//
// A failed load or render is printed to stderr as `photon: <err>` and the watch goes on. After a
// failed load it watches the files of the last good round; when there was none, the storyboard and
// the config it would use: inputs.ConfigFlag, else the nearest photon.yaml at or above the
// storyboard's dir, else that dir's photon.yaml. Watch returns nil once ctx ends, a render it cuts
// short reported as nothing, and an error only when stderr cannot be written.
func Watch(
	ctx context.Context,
	inputs WatchInputs,
	options Options,
	stdout, stderr io.Writer,
) error {
	watching := &watcher{inputs: inputs, options: options, stdout: stdout, stderr: stderr}
	rendered, err := watching.round(ctx)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	previous := rendered
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		current := stampFiles(watching.files)
		isSettledChange := !maps.Equal(current, rendered) && maps.Equal(current, previous)
		previous = current
		if !isSettledChange {
			continue
		}
		if rendered, err = watching.round(ctx); err != nil {
			return err
		}
		previous = rendered
	}
}

// watcher is one watch's state: its inputs and the files the last round was made from.
type watcher struct {
	inputs  WatchInputs
	options Options
	stdout  io.Writer
	stderr  io.Writer
	files   []string
}

// round loads the clip and renders it, and returns the stamps of the files it was made from,
// taken before the render so an edit made while it runs starts the next round. A failed load or
// render is reported on stderr, unless ctx ended; only a failed write there is returned.
func (w *watcher) round(ctx context.Context) (map[string]fileStamp, error) {
	loaded, err := clip.Load(w.inputs.Storyboard, w.inputs.ConfigFlag, w.inputs.WorkFlag)
	switch {
	case err == nil:
		w.files, err = w.filesOf(loaded)
	case w.files == nil:
		w.files = w.unloadedFiles()
	}
	stamps := stampFiles(w.files)
	if err == nil {
		err = Run(ctx, loaded, w.options, w.stdout, w.stderr)
	}
	if err == nil || ctx.Err() != nil {
		return stamps, nil
	}
	if _, writeErr := fmt.Fprintf(w.stderr, watchErrorFormat, err); writeErr != nil {
		return stamps, writeErr
	}
	return stamps, nil
}

// filesOf lists the files loaded was made from: its storyboard, its config, the config's caption
// font files and the take. It fails when the take's default path cannot be resolved, listing the
// rest.
func (w *watcher) filesOf(loaded *clip.Clip) ([]string, error) {
	files := []string{loaded.Storyboard.Path, loaded.Config.Path}
	for _, font := range []string{
		loaded.Config.CaptionFont.Regular,
		loaded.Config.CaptionFont.Bold,
	} {
		if font != "" {
			files = append(files, font)
		}
	}
	takePath := w.options.Take
	if takePath == "" {
		var err error
		if takePath, err = loaded.Take(); err != nil {
			return files, err
		}
	}
	return append(files, takePath), nil
}

// unloadedFiles lists what to watch when no clip has loaded yet: the storyboard and the config
// [clip.Load] would read.
func (w *watcher) unloadedFiles() []string {
	storyboardDir := filepath.Dir(w.inputs.Storyboard)
	configPath, err := config.Find(w.inputs.ConfigFlag, storyboardDir)
	if err != nil {
		// No config was found, which the load has already reported: watch the one a fix would
		// most likely create.
		configPath = filepath.Join(storyboardDir, config.FileName)
	}
	return []string{w.inputs.Storyboard, configPath}
}

// fileStamp tells two states of a file apart; a missing or unreadable file is the zero stamp.
type fileStamp struct {
	size     int64
	modified int64
}

// stampFiles stamps each of files by its path.
func stampFiles(files []string) map[string]fileStamp {
	stamps := make(map[string]fileStamp, len(files))
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			// A file that cannot be stat'ed is the zero stamp, so its return reads as a change.
			stamps[file] = fileStamp{}
			continue
		}
		stamps[file] = fileStamp{size: info.Size(), modified: info.ModTime().UnixNano()}
	}
	return stamps
}
