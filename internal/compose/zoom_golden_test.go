package compose

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// update rewrites the golden crops instead of comparing against them:
// `go test ./internal/compose/ -run Golden -update`.
var update = flag.Bool("update", false, "rewrite the golden crops under testdata")

// beatZoomCropsGolden is the golden of beatZoomBoard's crop at every frame.
const beatZoomCropsGolden = "beat-zoom-crops.golden"

// goldenFPS is the frame rate the golden samples the clip at.
const goldenFPS = 25

// goldenTolerance is how far, in source pixels, a crop edge may sit from the golden's.
const goldenTolerance = 1e-6

// beatZoomBoard is a four-beat clip of 9 s with two zoomed beats, every beat fitting its
// duration so source and output clocks agree. Beat 2 clicks a target left of centre and pushes
// in 2× on it, its 5 s of ramps shrunk to fit its 3 s section; beat 4 auto-fits a target its
// snapshot shows near the frame's right edge, so the crop clamps against that edge.
func beatZoomBoard() (*storyboard.Storyboard, *take.Take) {
	clicked := storyboard.Target{Text: "target"}
	gauge := storyboard.Target{Text: `context window \d+%`}
	board := &storyboard.Storyboard{Beats: []storyboard.Beat{
		{ID: 1, Duration: sec},
		{
			ID: 2, Duration: 3 * sec,
			Do:   []storyboard.Action{{Click: &storyboard.ClickAction{Target: clicked}}},
			Zoom: &storyboard.Zoom{Target: clicked, Factor: 2, In: 2 * sec, Out: 3 * sec},
		},
		{ID: 3, Duration: sec},
		{ID: 4, Duration: 4 * sec, Zoom: &storyboard.Zoom{Target: gauge}},
	}}
	// The gauge spans columns 30 to 47 of row 5: (18 + 4) cells × 20 px = 440 px filling 70 % of
	// 1000 px auto-fits 1.59×.
	shown := map[int]string{5: strings.Repeat(" ", 30) + "context window 42%"}
	events := []take.Event{beatStart(0, 1), beatStart(sec, 2)}
	events = append(events, clickAt(2*sec, 2, take.Box{X: 20, Y: 4, W: 3, H: 1})...)
	events = append(events, beatStart(4*sec, 3), beatStart(5*sec, 4))
	recorded := &take.Take{
		Snapshots: []take.Snapshot{screen(0, nil), screen(5*sec, shown), screen(9*sec, shown)},
		Events:    events,
	}
	return board, recorded
}

func TestComposeBeatZoomCropsMatchTheGolden(t *testing.T) {
	t.Parallel()
	board, recorded := beatZoomBoard()
	compositor, clipSchedule := newTestCompositor(t, board, recorded)

	var lines []string
	for _, frame := range clipSchedule.Frames(goldenFPS) {
		lines = append(lines, cropLine(frame.Out, compositor.crop(frame.Out)))
	}

	path := filepath.Join("testdata", beatZoomCropsGolden)
	if *update {
		writeCropGolden(t, path, lines)
		return
	}
	assertCropsMatch(t, lines, readCropGolden(t, path))
}

// cropLine is one golden line: the frame's output time in milliseconds and its crop rectangle.
func cropLine(out time.Duration, crop rectF) string {
	return fmt.Sprintf("%d %.6f %.6f %.6f %.6f", out.Milliseconds(), crop.X, crop.Y, crop.W, crop.H)
}

// writeCropGolden saves lines, one per frame, as the golden at path.
func writeCropGolden(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// readCropGolden loads the golden at path, one line per frame.
func readCropGolden(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open golden (rewrite with -update): %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	}()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return lines
}

// assertCropsMatch fails on a frame count that differs from the golden's, or on any frame whose
// time differs or whose crop edge sits further than goldenTolerance from the golden's.
func assertCropsMatch(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("frames: want %d, got %d", len(want), len(got))
	}
	for index := range want {
		gotMillis, gotCrop := parseCropLine(t, got[index])
		wantMillis, wantCrop := parseCropLine(t, want[index])
		if gotMillis != wantMillis {
			t.Fatalf("frame %d: want time %d ms, got %d ms", index, wantMillis, gotMillis)
		}
		for edge := range wantCrop {
			if math.Abs(gotCrop[edge]-wantCrop[edge]) > goldenTolerance {
				t.Errorf("frame %d at %d ms: want crop %q, got %q", index, wantMillis, want[index], got[index])
				break
			}
		}
	}
}

// parseCropLine splits a golden line into its time in milliseconds and its X, Y, W and H.
func parseCropLine(t *testing.T, line string) (int64, [4]float64) {
	t.Helper()
	var millis int64
	var crop [4]float64
	if _, err := fmt.Sscanf(line, "%d %f %f %f %f", &millis, &crop[0], &crop[1], &crop[2], &crop[3]); err != nil {
		t.Fatalf("parse golden line %q: %v", line, err)
	}
	return millis, crop
}
