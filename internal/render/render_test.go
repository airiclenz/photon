package render

import (
	"bytes"
	"context"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/take"
)

// sec is one second, to keep the take timings readable.
const sec = time.Second

// renderConfig is the render tests' photon config: nothing but what validation requires.
const renderConfig = `name: rendered
launch:
  command: ./app
`

// renderStoryboard is the render tests' storyboard: a tiny terminal shipped 160 pixels wide,
// beat 1 lasting 1s and beat 2 1.5s.
const renderStoryboard = `clip: rendered
ship: out/rendered.gif
frame: {cols: 20, rows: 4, padding: 4, font_size: 10, line_height: 1.2, scale: 2, width: 160, fps: 10, max_colors: 16}
beats:
  - id: 1
    title: typing
    duration: 1s
  - id: 2
    title: hold
    duration: 1500ms
`

// The shipped GIF's width and the clip's scheduled length, as renderStoryboard sets them.
const (
	wantWidth  = 160
	wantLength = 2500 * time.Millisecond
	// frameSlack is one output frame at the storyboard's 10 fps.
	frameSlack = 100 * time.Millisecond
)

// writeFile writes content to dir/name and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// textSnapshot is a snapshot at at showing one line of text per row.
func textSnapshot(at time.Duration, lines ...string) take.Snapshot {
	rows := make([][]take.Cell, 0, len(lines))
	for _, line := range lines {
		row := make([]take.Cell, 0, len(line))
		for _, r := range line {
			row = append(row, take.Cell{Grapheme: string(r), Width: 1})
		}
		rows = append(rows, row)
	}
	return take.Snapshot{At: at, Cells: rows}
}

// loadRenderClip writes the config and storyboard into a fresh project dir and loads the clip,
// with its work dir in a temp dir.
func loadRenderClip(t *testing.T) *clip.Clip {
	t.Helper()
	project := t.TempDir()
	writeFile(t, project, "photon.yaml", renderConfig)
	board := writeFile(t, project, "rendered.yaml", renderStoryboard)
	loaded, err := clip.Load(board, "", t.TempDir())
	if err != nil {
		t.Fatalf("clip.Load: %v", err)
	}
	return loaded
}

// saveRenderTake writes a take into dir and returns its path: beat 1 runs 2s of take (played at
// 2×), beat 2 1s (played at 1× and frozen for the rest of its 1.5s).
func saveRenderTake(t *testing.T, dir string, cols int) string {
	t.Helper()
	recorded := &take.Take{
		Cols: cols, Rows: 4, FPS: 10,
		Snapshots: []take.Snapshot{
			textSnapshot(0, "$ app"),
			textSnapshot(sec, "> tests are failing"),
			textSnapshot(2500*time.Millisecond, "Tests PASS"),
			textSnapshot(3*sec, "Tests PASS", "done"),
		},
		Events: []take.Event{
			{At: 0, Kind: take.EventBeatStart, Beat: 1},
			{At: 2 * sec, Kind: take.EventBeatStart, Beat: 2},
		},
	}
	path := filepath.Join(dir, "rendered.take")
	if err := take.Save(path, recorded); err != nil {
		t.Fatalf("take.Save: %v", err)
	}
	return path
}

func TestRunDryRunPrintsTheFfmpegLine(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClip(t)
	dir := t.TempDir()
	gifPath := filepath.Join(dir, "out.gif")
	options := Options{Take: saveRenderTake(t, dir, 20), Out: gifPath, DryRun: true}
	var stdout, stderr bytes.Buffer

	err := Run(context.Background(), loaded, options, &stdout, &stderr)

	if err != nil {
		t.Fatalf("Run: %v\n%s", err, stderr.String())
	}
	for _, want := range []string{
		"ffmpeg -y -loglevel error -f rawvideo -pixel_format rgba -video_size 160x",
		"-framerate 10 -i pipe:0",
		`"[0:v]split[a][b];[a]palettegen=max_colors=16[p];[b][p]paletteuse=dither=none"`,
		" " + gifPath + "\n",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("want the ffmpeg line to carry %q, got:\n%s", want, stdout.String())
		}
	}
	if _, err := os.Stat(gifPath); !os.IsNotExist(err) {
		t.Errorf("a dry run wrote %s", gifPath)
	}
}

func TestRunDryRunDefaultsToTheShipPath(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClip(t)
	options := Options{Take: saveRenderTake(t, t.TempDir(), 20), DryRun: true}
	var stdout bytes.Buffer

	err := Run(context.Background(), loaded, options, &stdout, &bytes.Buffer{})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := " " + loaded.Storyboard.Ship + "\n"; !strings.HasSuffix(stdout.String(), want) {
		t.Errorf("want the ffmpeg line to end in the ship path %q, got:\n%s", want, stdout.String())
	}
}

func TestRunRefusesATakeOfAnotherFrame(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClip(t)
	options := Options{Take: saveRenderTake(t, t.TempDir(), 30), DryRun: true}

	err := Run(context.Background(), loaded, options, &bytes.Buffer{}, &bytes.Buffer{})

	if err == nil || !strings.Contains(err.Error(), "recorded at 30×4 cells; the storyboard's frame is 20×4") {
		t.Fatalf("want the frame mismatch named, got %v", err)
	}
}

// TestRunWritesAGIFOfTheScheduledLength renders the two-beat take for real into the default
// ship path, whose directory does not exist yet: the GIF runs the beats' summed durations at the
// frame's width, and the summary names the path, the schedule's total and each beat's speed.
func TestRunWritesAGIFOfTheScheduledLength(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath(ffmpegProgram); err != nil {
		t.Skip("ffmpeg is not on PATH")
	}
	loaded := loadRenderClip(t)
	options := Options{Take: saveRenderTake(t, t.TempDir(), 20)}
	var stdout, stderr bytes.Buffer

	err := Run(context.Background(), loaded, options, &stdout, &stderr)

	if err != nil {
		t.Fatalf("Run: %v\n%s", err, stderr.String())
	}
	gifPath := loaded.Storyboard.Ship
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], gifPath+"  ") || !strings.HasSuffix(lines[0], "  2.5s") {
		t.Fatalf("summary: want `<path>  <size>  2.5s` and two beat rows, got:\n%s", stdout.String())
	}
	if !strings.Contains(lines[1], "beat 1  2.00×  2s of take in 1s") ||
		!strings.Contains(lines[2], "beat 2  1.00×  1s of take in 1.5s") {
		t.Errorf("beat rows: want each section's speed, got:\n%s", stdout.String())
	}
	decoded := decodeGIF(t, gifPath)
	var centiseconds int
	for _, delay := range decoded.Delay {
		centiseconds += delay
	}
	got := time.Duration(centiseconds) * 10 * time.Millisecond
	if got < wantLength-frameSlack || got > wantLength+frameSlack {
		t.Errorf("GIF length: want %s within a frame, got %s over %d frame(s)", wantLength, got, len(decoded.Delay))
	}
	if width := decoded.Image[0].Bounds().Dx(); width != wantWidth {
		t.Errorf("GIF width: want frame.width %d, got %d", wantWidth, width)
	}
}

// decodeGIF decodes every frame of the GIF at path.
func decodeGIF(t *testing.T, path string) *gif.GIF {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open the GIF: %v", err)
	}
	defer f.Close() //nolint:errcheck // read-only
	decoded, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatalf("decode the GIF: %v", err)
	}
	return decoded
}

func TestSnapshotAt(t *testing.T) {
	t.Parallel()
	snapshots := []take.Snapshot{{At: sec}, {At: 2 * sec}, {At: 4 * sec}}
	cases := map[time.Duration]int{0: 0, sec: 0, 1999 * time.Millisecond: 0, 2 * sec: 1, 3 * sec: 1, 9 * sec: 2}
	for at, want := range cases {
		if got := snapshotAt(snapshots, at); got != want {
			t.Errorf("snapshotAt(%s): want %d, got %d", at, want, got)
		}
	}
	if got := snapshotAt(nil, 0); got != -1 {
		t.Errorf("snapshotAt on no snapshots: want -1, got %d", got)
	}
}

func TestHumanSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		bytes int64
		want  string
	}{
		{bytes: 512, want: "512B"},
		{bytes: 1536, want: "1.5K"},
		{bytes: 20 * 1024, want: "20K"},
		{bytes: 3 * 1024 * 1024, want: "3.0M"},
		{bytes: 5 * 1024 * 1024 * 1024 * 1024, want: "5120G"},
	}
	for _, tt := range tests {
		if got := humanSize(tt.bytes); got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
