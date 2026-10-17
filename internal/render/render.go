package render

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/compose"
	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/raster"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

const (
	// outputDirMode is the permission a missing directory of the output path is created with.
	outputDirMode = 0o755
	// workDirMode is the permission a missing work dir is created with: the rig's own.
	workDirMode = 0o755
	// paletteSuffix ends the name of the palette file the palette pass writes into the work dir,
	// after the clip's name.
	paletteSuffix = ".palette.png"
)

// Options is one render's command line. An empty Take is the clip's take path, an empty Out the
// storyboard's ship path.
type Options struct {
	Take   string
	Out    string
	DryRun bool
}

// plan is everything a render needs once the clip and its take are read: the frame, the output
// clock, the rasterizer, the compositor, where the palette pass writes the palette and where the
// GIF goes.
type plan struct {
	frame      storyboard.Frame
	recorded   *take.Take
	schedule   *schedule.Schedule
	rasterizer *raster.Rasterizer
	compositor *compose.Compositor
	palette    string
	out        string
}

// Run renders loaded's take to its GIF and prints the summary to stdout: the GIF's path, size and
// length, then one row per beat with the speed its section plays at. On a dry run it prints the
// two ffmpeg command lines instead, the palette pass first, one per line, and runs and creates
// nothing. Schedule warnings go to stderr.
//
// The frames stream through ffmpeg twice, so memory stays flat however long the clip: pass 1
// builds the clip's palette into `<work>/<clip>.palette.png`, pass 2 maps the frames onto it into
// the GIF. Pass 2 never runs when pass 1 fails, and the palette file is removed before Run
// returns, whether the render succeeded or not.
//
// It fails when the work dir cannot be resolved, when the take was recorded at a size other than
// the storyboard's frame, when the take cannot be scheduled or composed, when ffmpeg or gifsicle
// fails, and when the palette file cannot be removed; a failing tool's error carries its stderr.
func Run(
	ctx context.Context,
	loaded *clip.Clip,
	options Options,
	stdout, stderr io.Writer,
) (err error) {
	rendering, err := newPlan(loaded, options, stderr)
	if err != nil {
		return err
	}
	size := rendering.compositor.Size()
	passes := [][]string{
		paletteArgs(size, rendering.frame, rendering.palette),
		encodeArgs(size, rendering.frame, rendering.palette, rendering.out),
	}
	if options.DryRun {
		return printCommandLines(stdout, passes)
	}
	if err := rendering.createDirs(); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, removePalette(rendering.palette)) }()
	for _, commandLine := range passes {
		if err := rendering.encode(ctx, commandLine); err != nil {
			return err
		}
	}
	if err := optimizeGIF(ctx, rendering.out); err != nil {
		return err
	}
	return writeSummary(stdout, rendering.out, rendering.schedule)
}

// printCommandLines prints each command line on its own line, for a human to read or paste.
func printCommandLines(w io.Writer, commandLines [][]string) error {
	for _, commandLine := range commandLines {
		if _, err := fmt.Fprintln(w, shellLine(commandLine)); err != nil {
			return err
		}
	}
	return nil
}

// createDirs creates the directory the GIF goes into and the work dir the palette goes into,
// where either is missing.
func (p *plan) createDirs() error {
	if err := os.MkdirAll(filepath.Dir(p.out), outputDirMode); err != nil {
		return fmt.Errorf("create the output dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.palette), workDirMode); err != nil {
		return fmt.Errorf("create the work dir: %w", err)
	}
	return nil
}

// removePalette removes the palette file at path; one that was never written is no error.
func removePalette(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the palette: %w", err)
	}
	return nil
}

// newPlan loads the take and builds the schedule, the rasterizer and the compositor. Frames are
// rasterized at frame.scale — padding and font size multiplied through — so a zoom has pixels to
// draw on; the compositor scales them to frame.width.
func newPlan(loaded *clip.Clip, options Options, stderr io.Writer) (*plan, error) {
	takePath, recorded, err := loadTake(loaded, options.Take)
	if err != nil {
		return nil, err
	}
	board := loaded.Storyboard
	frame := board.Frame
	if recorded.Cols != frame.Cols || recorded.Rows != frame.Rows {
		return nil, fmt.Errorf(
			"%s was recorded at %d×%d cells; the storyboard's frame is %d×%d — re-record it",
			takePath, recorded.Cols, recorded.Rows, frame.Cols, frame.Rows,
		)
	}
	clipSchedule, err := schedule.New(board, recorded)
	if err != nil {
		return nil, err
	}
	for _, warning := range clipSchedule.Warnings {
		if _, err := fmt.Fprintln(stderr, "warning:", warning); err != nil {
			return nil, err
		}
	}
	rasterizer, err := newRasterizer(loaded, frame)
	if err != nil {
		return nil, err
	}
	compositor, err := newCompositor(loaded, recorded, clipSchedule, rasterizer)
	if err != nil {
		return nil, err
	}
	palette, err := palettePath(loaded)
	if err != nil {
		return nil, err
	}
	out := options.Out
	if out == "" {
		out = board.Ship
	}
	return &plan{
		frame:      frame,
		recorded:   recorded,
		schedule:   clipSchedule,
		rasterizer: rasterizer,
		compositor: compositor,
		palette:    palette,
		out:        out,
	}, nil
}

// loadTake loads the take at takePath, or at the clip's take path when takePath is empty, and
// returns the path it read.
func loadTake(loaded *clip.Clip, takePath string) (string, *take.Take, error) {
	if takePath == "" {
		var err error
		if takePath, err = loaded.Take(); err != nil {
			return "", nil, err
		}
	}
	recorded, err := take.Load(takePath)
	if err != nil {
		return "", nil, err
	}
	return takePath, recorded, nil
}

// palettePath is where the palette pass writes the clip's palette: `<work>/<clip>.palette.png`,
// with the work dir of [clip.Clip.Work]. Every render resolves it, an explicit take included.
func palettePath(loaded *clip.Clip) (string, error) {
	work, err := loaded.Work()
	if err != nil {
		return "", err
	}
	return filepath.Join(work, loaded.Storyboard.Clip+paletteSuffix), nil
}

// newRasterizer builds the rasterizer over the config's fonts and theme at the frame's geometry,
// scaled by frame.scale.
func newRasterizer(loaded *clip.Clip, frame storyboard.Frame) (*raster.Rasterizer, error) {
	faces, err := fonts.Load(loaded.Config.Fonts)
	if err != nil {
		return nil, err
	}
	return raster.New(faces, loaded.Config.Theme, raster.Geometry{
		Cols:       frame.Cols,
		Rows:       frame.Rows,
		Padding:    frame.Padding * frame.Scale,
		FontSize:   frame.FontSize * float64(frame.Scale),
		LineHeight: frame.LineHeight,
	})
}

// newCompositor plans the zooms and the cursor over the rasterizer's cell layout, resolving zoom
// targets through the config's areas and clearing a zoomed frame to the theme background.
func newCompositor(
	loaded *clip.Clip,
	recorded *take.Take,
	clipSchedule *schedule.Schedule,
	rasterizer *raster.Rasterizer,
) (*compose.Compositor, error) {
	resolver, err := target.New(loaded.Config.Areas)
	if err != nil {
		return nil, err
	}
	frame := loaded.Storyboard.Frame
	layout := compose.Layout{
		Size:       rasterizer.Size(),
		Padding:    frame.Padding * frame.Scale,
		CellWidth:  rasterizer.CellWidth(),
		LineHeight: rasterizer.LineHeight(),
		Scale:      frame.Scale,
	}
	background := loaded.Config.Theme.BG.RGBA()
	return compose.New(loaded.Storyboard, recorded, clipSchedule, resolver, layout, background)
}

// writeSummary prints what was rendered: path, size and the clip's length — the schedule's
// total — then one row per beat with the speed its section plays at, or `cut`.
func writeSummary(w io.Writer, path string, clipSchedule *schedule.Schedule) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	seconds := strconv.FormatFloat(clipSchedule.Length.Seconds(), 'f', -1, 64)
	if _, err := fmt.Fprintf(w, "%s  %s  %ss\n", path, humanSize(stat.Size()), seconds); err != nil {
		return err
	}
	for _, section := range clipSchedule.Sections {
		row := fmt.Sprintf("  beat %d  cut", section.Beat)
		if !section.Cut {
			taken := (section.End - section.Start).Round(time.Millisecond)
			row = fmt.Sprintf("  beat %d  %.2f×  %s of take in %s", section.Beat, section.Speed, taken, section.Duration)
		}
		if _, err := fmt.Fprintln(w, row); err != nil {
			return err
		}
	}
	return nil
}

const (
	// sizeUnit is the step between byte-count suffixes.
	sizeUnit = 1024
	// decimalBelow is the size under which a byte count keeps one decimal.
	decimalBelow = 10
)

// sizeSuffixes are the byte-count units past bytes, in order.
var sizeSuffixes = []string{"K", "M", "G"}

// humanSize spells a byte count the way `du -h` does: one decimal below ten units, whole units
// above, in K, M or G.
func humanSize(bytes int64) string {
	if bytes < sizeUnit {
		return fmt.Sprintf("%dB", bytes)
	}
	index := 0
	size := float64(bytes) / sizeUnit
	for size >= sizeUnit && index < len(sizeSuffixes)-1 {
		size /= sizeUnit
		index++
	}
	if size < decimalBelow {
		return fmt.Sprintf("%.1f%s", size, sizeSuffixes[index])
	}
	return fmt.Sprintf("%.0f%s", size, sizeSuffixes[index])
}
