package render

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/airiclenz/photon/internal/storyboard"
)

// ditherMode is the palette encode's dither: none. A palette entry is the mean of the colours it
// stands for, so the flat background rarely lands on one exactly, and an ordered dither then
// speckles every blank cell with near-background shades; mapped to the nearest entry instead, a
// flat colour stays flat. The frames are flat colours and glyph edges, with no gradient a dither
// would rescue.
const ditherMode = "none"

const (
	// ffmpegProgram runs both encode passes: the frames into a palette, then the frames through
	// that palette into the GIF.
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

// rawInputArgs starts both passes' command lines: raw RGBA frames of size at frame.fps read from
// stdin. The passes see the same frames through the same input, which keeps the palette pass 1
// builds the one pass 2 maps against.
func rawInputArgs(size image.Point, frame storyboard.Frame) []string {
	return []string{
		ffmpegProgram, "-y", "-loglevel", "error",
		"-f", "rawvideo", "-pixel_format", "rgba",
		"-video_size", fmt.Sprintf("%dx%d", size.X, size.Y),
		"-framerate", strconv.Itoa(frame.FPS),
		"-i", "pipe:0",
	}
}

// paletteArgs is the palette pass's command line: the frames on stdin reduced to a per-clip
// palette of frame.max_colors, written to the PNG at palette. ffmpeg keeps only its statistics
// while the frames stream, so the pass holds no frame past the one it reads.
func paletteArgs(size image.Point, frame storyboard.Frame, palette string) []string {
	return append(
		rawInputArgs(size, frame),
		"-vf", fmt.Sprintf("palettegen=max_colors=%d", frame.MaxColors),
		"-update", "1",
		palette,
	)
}

// encodeArgs is the GIF pass's command line: the frames on stdin mapped onto the palette pass's
// PNG at palette without dithering, the GIF written to out. Every frame is mapped as it arrives.
func encodeArgs(size image.Point, frame storyboard.Frame, palette, out string) []string {
	return append(
		rawInputArgs(size, frame),
		"-i", palette,
		"-filter_complex", fmt.Sprintf("[0:v][1:v]paletteuse=dither=%s", ditherMode),
		out,
	)
}

// encode streams every scheduled frame, composed, into the encoder commandLine starts, as raw
// RGBA on its stdin. Each pass calls it once, so the frames are composed afresh for each and none
// is kept between them; only the pipeline's last rasterized snapshot carries over. The encoder's own failure is reported first, with its stderr, since a
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

// writeFrames writes each output frame's pixels, in order, as the pipeline builds them.
func (p *plan) writeFrames(w io.Writer) error {
	for _, frame := range p.pipeline.Schedule().Frames(p.frame.FPS) {
		composed, err := p.pipeline.FrameAt(frame.Out)
		if err != nil {
			return err
		}
		if _, err := w.Write(composed.Pix); err != nil {
			return err
		}
	}
	return nil
}

// shellLine joins a command line for a human to read or paste: an argument the shell would break
// apart or unquote is wrapped in double quotes.
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
