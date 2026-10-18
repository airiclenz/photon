package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/clip"
)

// The lint fixtures under testdata.
var (
	everyFormStoryboard  = filepath.Join("testdata", "valid", "every-form.yaml")
	validConfig          = filepath.Join("testdata", "valid", "photon.yaml")
	minimalStoryboard    = filepath.Join("testdata", "minimal", "hello.yaml")
	invalidStoryboard    = filepath.Join("testdata", "invalid", "demo.yaml")
	undeclaredStoryboard = filepath.Join("testdata", "invalid", "undeclared.yaml")
	badDragStoryboard    = filepath.Join("testdata", "invalid", "drag.yaml")
	badCameraStoryboard  = filepath.Join("testdata", "invalid", "camera.yaml")
	cameraAreaStoryboard = filepath.Join("testdata", "invalid", "camera-area.yaml")
	badCaptionStoryboard = filepath.Join("testdata", "invalid", "captions.yaml")
	captionBoxStoryboard = filepath.Join("testdata", "invalid", "caption-box.yaml")
)

// okMarker is what lint prints for a clip with no problem.
const okMarker = ": ok"

func TestLint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantOK     bool
		wantStderr []string
	}{
		{
			name:     "every form the schema has lints clean",
			args:     []string{"lint", everyFormStoryboard},
			wantCode: exitOK,
			wantOK:   true,
		},
		{
			name:     "a minimal config and storyboard lint clean",
			args:     []string{"lint", minimalStoryboard},
			wantCode: exitOK,
			wantOK:   true,
		},
		{
			name:     "every storyboard and config problem is listed",
			args:     []string{"lint", invalidStoryboard},
			wantCode: exitRunFailed,
			wantStderr: []string{
				"clip: missing",
				"beat 1: title: missing",
				"launch.command: missing",
			},
		},
		{
			name:       "the config flag wins and an undeclared area is listed",
			args:       []string{"lint", undeclaredStoryboard, "--config", validConfig},
			wantCode:   exitRunFailed,
			wantStderr: []string{`beat 1: expect[0]: area: "sidebar" is not declared`},
		},
		{
			name:       "a bad drag is listed under its field",
			args:       []string{"lint", badDragStoryboard, "--config", validConfig},
			wantCode:   exitRunFailed,
			wantStderr: []string{"beat 1: do[0]: drag.to: want exactly one of text or offset"},
		},
		{
			name:     "every bad camera move is listed under its field",
			args:     []string{"lint", badCameraStoryboard, "--config", validConfig},
			wantCode: exitRunFailed,
			wantStderr: []string{
				"camera[0]: zoom.over: want >0, got 0s",
				"camera[1]: zoom.to: want in [1, 3], got 3.5",
				"camera[2]: pan.at: want a start in [0s, 4s), got 9s",
				"camera[4]: zoom: overlaps camera[3] on the zoom track",
			},
		},
		{
			name:       "an undeclared camera area is listed",
			args:       []string{"lint", cameraAreaStoryboard, "--config", validConfig},
			wantCode:   exitRunFailed,
			wantStderr: []string{`camera[0]: pan.to.area: "sidebar" is not declared`},
		},
		{
			name:     "every bad caption is listed under its field",
			args:     []string{"lint", badCaptionStoryboard, "--config", validConfig},
			wantCode: exitRunFailed,
			wantStderr: []string{
				"captions[0]: until: want a stop after the start 2s, got 1s",
				"captions[1]: at: want a start in [0s, 4s), got 9s",
				"captions[2]: at: beat 7 is not in the storyboard",
				"captions[3]: font.size: want in [6, 72], got 100",
				"captions[4]: fade: want in + out within the caption's length 1s, got 1.2s",
			},
		},
		{
			name:       "a caption box larger than the frame is listed under its caption",
			args:       []string{"lint", captionBoxStoryboard, "--config", validConfig},
			wantCode:   exitRunFailed,
			wantStderr: []string{"captions[0]: ", "larger than the"},
		},
		{
			name:       "no storyboard is a bad command line",
			args:       []string{"lint"},
			wantCode:   exitBadUsage,
			wantStderr: []string{"accepts 1 arg(s), received 0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, stdout, stderr := runLint(t, tt.args)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr)
			}
			if got := strings.Contains(stdout, okMarker); got != tt.wantOK {
				t.Errorf("ok printed = %t, want %t (stdout %q)", got, tt.wantOK, stdout)
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q does not list %q", stderr, want)
				}
			}
		})
	}
}

func TestLint_ValidClipLintsCleanWithoutAHomeDir(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	code, stdout, stderr := runLint(t, []string{"lint", minimalStoryboard})

	if code != exitOK || !strings.Contains(stdout, okMarker) {
		t.Errorf("exit code = %d, stdout %q, stderr %q; want 0 and ok", code, stdout, stderr)
	}
}

// runLint runs the real command tree on args and returns the exit code, stdout and stderr.
func runLint(t *testing.T, args []string) (code int, stdout, stderr string) {
	t.Helper()
	root := newRootCommand()
	var out, errOut bytes.Buffer
	root.SetOut(&out)

	code = execute(context.Background(), root, args, &errOut)

	return code, out.String(), errOut.String()
}
