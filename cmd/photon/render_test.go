package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/clip"
)

// renderConfig is the render command tests' photon config.
const renderConfig = `name: rendered
launch:
  command: ./app
`

// renderStoryboard is the render command tests' storyboard, framed like checkTake: two beats of
// a 20 × 3 terminal.
const renderStoryboard = `clip: demo
ship: out/demo.gif
frame: {cols: 20, rows: 3, padding: 4, font_size: 10, line_height: 1.2, scale: 1, width: 120, fps: 10, max_colors: 16}
beats:
  - id: 1
    title: open
    duration: 2s
  - id: 2
    title: save
    duration: 2s
`

func TestRender(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")

	unconfigured := t.TempDir()
	board := writeFile(t, unconfigured, "demo.yaml", renderStoryboard)
	configElsewhere := writeFile(t, t.TempDir(), "photon.yaml", renderConfig)
	work := t.TempDir()
	saveCheckTake(t, filepath.Join(work, "demo.take"))
	explicitTake := saveCheckTake(t, filepath.Join(t.TempDir(), "explicit.take"))
	gifPath := filepath.Join(t.TempDir(), "out.gif")

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout []string
		wantStderr []string
	}{
		{
			name:     "a dry run reads --config and the take in --work, and writes the ship path",
			args:     []string{"render", board, "--config", configElsewhere, "--work", work, "--dry-run"},
			wantCode: exitOK,
			wantStdout: []string{
				"ffmpeg -y -loglevel error -f rawvideo -pixel_format rgba -video_size 120x",
				" " + filepath.Join(unconfigured, "out", "demo.gif") + "\n",
			},
		},
		{
			name:       "the take and the output are named on the command line",
			args:       []string{"render", board, explicitTake, "-o", gifPath, "--config", configElsewhere, "--dry-run"},
			wantCode:   exitOK,
			wantStdout: []string{" " + gifPath + "\n"},
		},
		{
			name:       "without --config the storyboard's dir has no photon.yaml",
			args:       []string{"render", board, explicitTake, "--dry-run"},
			wantCode:   exitRunFailed,
			wantStderr: []string{"photon.yaml"},
		},
		{
			name:       "a missing take is a failed run",
			args:       []string{"render", board, "--config", configElsewhere, "--work", t.TempDir(), "--dry-run"},
			wantCode:   exitRunFailed,
			wantStderr: []string{"open the take"},
		},
		{
			name:       "no storyboard is a bad command line",
			args:       []string{"render"},
			wantCode:   exitBadUsage,
			wantStderr: []string{"accepts between 1 and 2 arg(s), received 0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runLint(t, tt.args)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stdout %q, stderr %q)", code, tt.wantCode, stdout, stderr)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout %q does not carry %q", stdout, want)
				}
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q does not carry %q", stderr, want)
				}
			}
		})
	}
}
