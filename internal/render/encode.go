package render

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// ditherMode is the palette encode's dither: none. A palette entry is the mean of the colours it
// stands for, so the flat background rarely lands on one exactly, and an ordered dither then
// speckles every blank cell with near-background shades; mapped to the nearest entry instead, a
// flat colour stays flat. The frames are flat colours and glyph edges, with no gradient a dither
// would rescue.
const ditherMode = "none"

const (
	// ffmpegProgram encodes the frames into the GIF.
	ffmpegProgram = "ffmpeg"
	// gifsicleProgram shrinks the GIF afterwards, when it is on PATH.
	gifsicleProgram = "gifsicle"
	// gifsicleOptimize is the gifsicle pass: lossless. Not --lossy: its LZW matching trades exact
	// colours for near ones, which scatters near-background speckles over every flat area, and a
	// terminal clip is almost all flat area.
	gifsicleOptimize = "-O3"
	// optimizedSuffix ends the name of the file gifsicle writes before it replaces the GIF.
	optimizedSuffix = ".opt"
)

// shellSpecial are the characters that make shellLine quote an argument.
const shellSpecial = " '\";()"

// ffmpegArgs is the encode's command line: raw RGBA frames of size at frame.fps on stdin, a
// per-clip palette of frame.max_colors applied without dithering, the GIF written to out.
func ffmpegArgs(size image.Point, frame storyboard.Frame, out string) []string {
	graph := fmt.Sprintf(
		"[0:v]split[a][b];[a]palettegen=max_colors=%d[p];[b][p]paletteuse=dither=%s",
		frame.MaxColors, ditherMode,
	)
	return []string{
		ffmpegProgram, "-y", "-loglevel", "error",
		"-f", "rawvideo", "-pixel_format", "rgba",
		"-video_size", fmt.Sprintf("%dx%d", size.X, size.Y),
		"-framerate", strconv.Itoa(frame.FPS),
		"-i", "pipe:0",
		"-filter_complex", graph,
		out,
	}
}

// encode streams every scheduled frame, composed, into the encoder commandLine starts, as raw
// RGBA on its stdin. The encoder's own failure is reported first, with its stderr, since a
// broken pipe while streaming is only its symptom.
func (p *plan) encode(ctx context.Context, commandLine []string) error {
	//nolint:gosec // the program is fixed and every argument is built here
	cmd := exec.CommandContext(ctx, commandLine[0], commandLine[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", commandLine[0], err)
	}
	writeErr := p.writeFrames(stdin)
	closeErr := stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s: %w\n%s", commandLine[0], err, strings.TrimSpace(stderr.String()))
	}
	if writeErr != nil {
		return fmt.Errorf("stream frames to %s: %w", commandLine[0], writeErr)
	}
	return closeErr
}

// writeFrames writes each output frame's pixels, in order. A snapshot shown by consecutive
// frames is rasterized once.
func (p *plan) writeFrames(w io.Writer) error {
	shown := -1
	var source *image.RGBA
	for _, frame := range p.schedule.Frames(p.frame.FPS) {
		index := snapshotAt(p.recorded.Snapshots, frame.Source)
		if index < 0 {
			return fmt.Errorf("the take holds no snapshot to show at %s", frame.Source)
		}
		if index != shown {
			source, shown = p.rasterizer.Frame(p.recorded.Snapshots[index]), index
		}
		if _, err := w.Write(p.compositor.Compose(frame.Out, source).Pix); err != nil {
			return err
		}
	}
	return nil
}

// snapshotAt is the index of the snapshot on screen at source time t — the last one taken at or
// before it, or the first when t precedes them all — and -1 for a take without snapshots.
func snapshotAt(snapshots []take.Snapshot, t time.Duration) int {
	if len(snapshots) == 0 {
		return -1
	}
	after := sort.Search(len(snapshots), func(i int) bool { return snapshots[i].At > t })
	return max(after-1, 0)
}

// shellLine joins a command line for a human to read or paste: an argument the shell would split
// or unquote is wrapped in double quotes.
func shellLine(commandLine []string) string {
	words := make([]string, 0, len(commandLine))
	for _, argument := range commandLine {
		if strings.ContainsAny(argument, shellSpecial) {
			argument = `"` + argument + `"`
		}
		words = append(words, argument)
	}
	return strings.Join(words, " ")
}

// optimizeGIF shrinks the GIF at path in place through gifsicle when it is on PATH, and does
// nothing otherwise. The result replaces the GIF only once gifsicle has written it whole.
func optimizeGIF(ctx context.Context, path string) error {
	if _, err := exec.LookPath(gifsicleProgram); err != nil {
		// gifsicle is optional: without it the GIF ships as ffmpeg encoded it.
		return nil
	}
	optimized := path + optimizedSuffix
	if err := runQuiet(ctx, []string{gifsicleProgram, gifsicleOptimize, "-o", optimized, path}); err != nil {
		return err
	}
	return os.Rename(optimized, path)
}

// runQuiet runs one command and reports its stderr only when it fails.
func runQuiet(ctx context.Context, commandLine []string) error {
	//nolint:gosec // the program is fixed and every argument is built here
	cmd := exec.CommandContext(ctx, commandLine[0], commandLine[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w\n%s", commandLine[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
