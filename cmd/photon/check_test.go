package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/take"
)

// checkConfig is the check tests' photon config: one area, the rows below a `---` rule.
const checkConfig = `name: checked
launch:
  command: ./app
areas:
  footer:
    top: {match: '^---$', offset: 1}
`

// checkStoryboardHeader is the check tests' storyboard header; each test appends its beats.
const checkStoryboardHeader = `clip: demo
ship: out/demo.gif
frame: {cols: 20, rows: 3, padding: 16, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 24, max_colors: 128}
`

// passingBeats are two beats every expect of which the synthetic take passes.
const passingBeats = `beats:
  - id: 1
    title: open
    duration: 2s
    expect:
      - seen: ready
  - id: 2
    title: save
    duration: 2s
    expect:
      - seen: mode view
        area: footer
      - run: printf %s "$PHOTON_WORK"
`

// failingBeats are two beats, the first of whose expects the synthetic take fails.
const failingBeats = `beats:
  - id: 1
    title: open
    duration: 2s
    expect:
      - seen: absent
  - id: 2
    title: save
    duration: 2s
`

// checkTake is a synthetic take of two beats, starting at 0 and 2s: `ready`, then a footer
// reading `mode view` from 2.5s.
func checkTake() *take.Take {
	cells := func(line string) []take.Cell {
		row := make([]take.Cell, 0, len(line))
		for _, r := range line {
			row = append(row, take.Cell{Grapheme: string(r), Width: 1})
		}
		return row
	}
	return &take.Take{
		Cols: 20, Rows: 3, FPS: 24,
		Snapshots: []take.Snapshot{
			{At: 0, Cells: [][]take.Cell{cells("ready")}},
			{At: 2500 * time.Millisecond, Cells: [][]take.Cell{cells("saved"), cells("---"), cells("mode view")}},
		},
		Events: []take.Event{
			{At: 0, Kind: take.EventBeatStart, Beat: 1},
			{At: 2 * time.Second, Kind: take.EventBeatStart, Beat: 2},
		},
	}
}

// writeFile writes content to dir/name and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// saveCheckTake saves the synthetic take at path.
func saveCheckTake(t *testing.T, path string) string {
	t.Helper()
	if err := take.Save(path, checkTake()); err != nil {
		t.Fatalf("take.Save: %v", err)
	}
	return path
}

func TestCheck(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")

	project := t.TempDir()
	writeFile(t, project, "photon.yaml", checkConfig)
	passing := writeFile(t, project, "passing.yaml", checkStoryboardHeader+passingBeats)
	failing := writeFile(t, project, "failing.yaml", checkStoryboardHeader+failingBeats)
	elsewhere := t.TempDir()
	configElsewhere := writeFile(t, elsewhere, "photon.yaml", checkConfig)
	unconfigured := t.TempDir()
	passingUnconfigured := writeFile(t, unconfigured, "passing.yaml", checkStoryboardHeader+passingBeats)
	work := t.TempDir()
	defaultTake := saveCheckTake(t, filepath.Join(work, "demo.take"))
	explicitTake := saveCheckTake(t, filepath.Join(t.TempDir(), "explicit.take"))

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout []string
		wantStderr []string
	}{
		{
			name:     "a passing take exits 0",
			args:     []string{"check", passing, explicitTake, "--work", work},
			wantCode: exitOK,
			wantStdout: []string{
				"beat 1 | PASS | seen /ready/ at 0s",
				"beat 2 | PASS | seen /mode view/ in area footer at 2.5s",
				"beat 2 | PASS | " + work,
			},
		},
		{
			name:       "the take defaults into the work dir",
			args:       []string{"check", passing, "--work", work},
			wantCode:   exitOK,
			wantStdout: []string{"beat 1 | PASS |"},
		},
		{
			name:       "the config flag names the config",
			args:       []string{"check", passingUnconfigured, defaultTake, "--config", configElsewhere, "--work", work},
			wantCode:   exitOK,
			wantStdout: []string{"in area footer"},
		},
		{
			name:       "a failing take exits 1",
			args:       []string{"check", failing, defaultTake, "--work", work},
			wantCode:   exitRunFailed,
			wantStdout: []string{"beat 1 | FAIL | not seen /absent/"},
			wantStderr: []string{"1 expect(s) failed"},
		},
		{
			name:       "a missing take is a failed run",
			args:       []string{"check", passing, filepath.Join(work, "missing.take")},
			wantCode:   exitRunFailed,
			wantStderr: []string{"open the take"},
		},
		{
			name:       "no storyboard is a bad command line",
			args:       []string{"check"},
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
