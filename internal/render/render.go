package render

import (
	"context"
	"fmt"
	"io"
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

// outputDirMode is the permission a missing directory of the output path is created with.
const outputDirMode = 0o755

// Options is one render's command line. An empty Take is the clip's take path, an empty Out the
// storyboard's ship path.
type Options struct {
	Take   string
	Out    string
	DryRun bool
}

// plan is everything a render needs once the clip and its take are read: the frame, the output
// clock, the rasterizer, the compositor and where the GIF goes.
type plan struct {
	frame      storyboard.Frame
	recorded   *take.Take
	schedule   *schedule.Schedule
	rasterizer *raster.Rasterizer
	compositor *compose.Compositor
	out        string
}

// Run renders loaded's take to its GIF and prints the summary to stdout: the GIF's path, size and
// length, then one row per beat with the speed its section plays at. On a dry run it prints the
// ffmpeg command line instead and runs nothing. Schedule warnings go to stderr.
//
// It fails when the take was recorded at a size other than the storyboard's frame, when the take
// cannot be scheduled or composed, and when ffmpeg or gifsicle fails; a failing tool's error
// carries its stderr.
func Run(ctx context.Context, loaded *clip.Clip, options Options, stdout, stderr io.Writer) error {
	rendering, err := newPlan(loaded, options, stderr)
	if err != nil {
		return err
	}
	commandLine := ffmpegArgs(rendering.compositor.Size(), rendering.frame, rendering.out)
	if options.DryRun {
		_, err := fmt.Fprintln(stdout, shellLine(commandLine))
		return err
	}
	if err := os.MkdirAll(filepath.Dir(rendering.out), outputDirMode); err != nil {
		return fmt.Errorf("create the output dir: %w", err)
	}
	if err := rendering.encode(ctx, commandLine); err != nil {
		return err
	}
	if err := optimizeGIF(ctx, rendering.out); err != nil {
		return err
	}
	return writeSummary(stdout, rendering.out, rendering.schedule)
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
