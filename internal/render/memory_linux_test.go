package render

import (
	"bytes"
	"image"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/airiclenz/photon/internal/storyboard"
)

// The peak-memory test's synthetic clip: frames of memoryFrameSize at memoryFPS, reduced to
// memoryMaxColors.
const (
	memoryFPS       = 10
	memoryMaxColors = 256
)

// memoryFrameSize is the synthetic frames' size in pixels.
var memoryFrameSize = image.Point{X: 320, Y: 200}

// The two clip lengths, in frames, whose peaks the test compares.
const (
	shortClipFrames = 40
	longClipFrames  = 400
)

// The growth bounds, in bytes, from the short clip's peak to the long clip's. A bounded pass stays
// under flatGrowth; the one-pass graph buffers every frame (400 frames are ~100 MB of RGBA), so its
// growth must clear bufferedGrowth or the measurement has stopped biting.
const (
	flatGrowth     = 16 << 20
	bufferedGrowth = 50 << 20
)

// The synthetic frames: a flat background with a bar of a few colours that moves barStep pixels a
// frame. The colours are a small fixed set on purpose: random noise would grow palettegen's
// histogram and paletteuse's colour cache with every distinct colour, which is a different cost
// from buffering frames and would swamp it.
const (
	barWidth = 48
	barStep  = 7
)

// rssUnit is the unit of Rusage.Maxrss on Linux: kibibytes.
const rssUnit = 1024

var (
	memoryBackground = [4]byte{0x1e, 0x1e, 0x2e, 0xff}
	memoryBarColours = [][4]byte{
		{0xf3, 0x8b, 0xa8, 0xff},
		{0xa6, 0xe3, 0xa1, 0xff},
		{0x89, 0xb4, 0xfa, 0xff},
		{0xf9, 0xe2, 0xaf, 0xff},
	}
)

// TestEncodePeakMemory measures the ffmpeg peak RSS of each encode pass at a short and a long
// clip: each pass holds a constant amount of memory however long the clip, while the one-pass
// graph they replaced grows with it, which proves the measurement can tell the two apart. The
// subtests run one at a time, since the long one-pass encode alone holds ~100 MB.
func TestEncodePeakMemory(t *testing.T) {
	if _, err := exec.LookPath(ffmpegProgram); err != nil {
		t.Skip("ffmpeg is not on PATH")
	}
	frame := storyboard.Frame{FPS: memoryFPS, MaxColors: memoryMaxColors}
	cases := []struct {
		name        string
		commandLine func(t *testing.T, frames int, dir string) []string
		buffers     bool
	}{
		{
			name: "palette pass",
			commandLine: func(_ *testing.T, _ int, dir string) []string {
				return paletteArgs(memoryFrameSize, frame, filepath.Join(dir, "clip.palette.png"))
			},
		},
		{
			name: "GIF pass",
			commandLine: func(t *testing.T, frames int, dir string) []string {
				t.Helper()
				palette := filepath.Join(dir, "clip.palette.png")
				streamSyntheticFrames(t, paletteArgs(memoryFrameSize, frame, palette), frames)
				return encodeArgs(memoryFrameSize, frame, palette, filepath.Join(dir, "clip.gif"))
			},
		},
		{
			name: "one-pass graph",
			commandLine: func(_ *testing.T, _ int, dir string) []string {
				return onePassArgs(memoryFrameSize, frame, filepath.Join(dir, "clip.gif"))
			},
			buffers: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			short := streamSyntheticFrames(t, tc.commandLine(t, shortClipFrames, t.TempDir()), shortClipFrames)
			long := streamSyntheticFrames(t, tc.commandLine(t, longClipFrames, t.TempDir()), longClipFrames)

			growth := long - short

			t.Logf("peak RSS: %d frames %.1f MiB, %d frames %.1f MiB",
				shortClipFrames, mebibytes(short), longClipFrames, mebibytes(long))
			if !tc.buffers && growth >= flatGrowth {
				t.Errorf("peak grew %.1f MiB from %d to %d frames; want under %.1f MiB",
					mebibytes(growth), shortClipFrames, longClipFrames, mebibytes(flatGrowth))
			}
			if tc.buffers && growth <= bufferedGrowth {
				t.Errorf("peak grew %.1f MiB from %d to %d frames; want over %.1f MiB, or the measurement no longer bites",
					mebibytes(growth), shortClipFrames, longClipFrames, mebibytes(bufferedGrowth))
			}
		})
	}
}

// streamSyntheticFrames runs commandLine with frames synthetic frames on its stdin as raw RGBA,
// and returns the process's peak RSS in bytes.
func streamSyntheticFrames(t *testing.T, commandLine []string, frames int) int64 {
	t.Helper()
	//nolint:gosec // the program is fixed and every argument is built by the test
	cmd := exec.Command(commandLine[0], commandLine[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", commandLine[0], err)
	}
	pixels := make([]byte, 4*memoryFrameSize.X*memoryFrameSize.Y)
	var writeErr error
	for i := 0; i < frames && writeErr == nil; i++ {
		paintSyntheticFrame(pixels, i)
		_, writeErr = stdin.Write(pixels)
	}
	closeErr := stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("%s: %v\n%s", commandLine[0], err, strings.TrimSpace(stderr.String()))
	}
	if writeErr != nil {
		t.Fatalf("stream frames to %s: %v", commandLine[0], writeErr)
	}
	if closeErr != nil {
		t.Fatalf("close the stdin of %s: %v", commandLine[0], closeErr)
	}
	usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		t.Fatalf("%s: no resource usage reported", commandLine[0])
	}
	return usage.Maxrss * rssUnit
}

// paintSyntheticFrame fills pixels with the synthetic frame at index: the background, then the bar
// at the column it has moved to, one vertical stripe per bar colour.
func paintSyntheticFrame(pixels []byte, index int) {
	width := memoryFrameSize.X
	left := index * barStep % width
	stripe := barWidth / len(memoryBarColours)
	for y := range memoryFrameSize.Y {
		for x := range width {
			colour := memoryBackground
			if offset := (x - left + width) % width; offset < barWidth {
				colour = memoryBarColours[offset/stripe]
			}
			copy(pixels[4*(y*width+x):], colour[:])
		}
	}
}

// mebibytes is bytes in MiB, for the log and failure lines.
func mebibytes(size int64) float64 {
	return float64(size) / (1 << 20)
}
