package clip

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/storyboard"
)

// validStoryboard is a storyboard whose every named area valid/photon.yaml declares.
var validStoryboard = filepath.Join("testdata", "valid", "demo.yaml")

func TestLoad_WorkDirPrecedence(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve the home dir: %v", err)
	}
	withWork := filepath.Join("testdata", "valid", "with-work.yaml")
	configWork, err := filepath.Abs(filepath.Join("testdata", "valid", "rig"))
	if err != nil {
		t.Fatalf("resolve the config's work dir: %v", err)
	}
	relativeFlag, err := filepath.Abs("relative-work")
	if err != nil {
		t.Fatalf("resolve the relative flag: %v", err)
	}
	flagWork := filepath.Join(t.TempDir(), "flag")
	envWork := filepath.Join(t.TempDir(), "env")

	tests := []struct {
		name       string
		configFlag string
		workFlag   string
		env        string
		want       string
	}{
		{"the flag wins over the env and the config", withWork, flagWork, envWork, flagWork},
		{"a relative flag is made absolute", withWork, "relative-work", envWork, relativeFlag},
		{"the env wins over the config", withWork, "", envWork, envWork},
		{"the config's work, resolved against its dir", withWork, "", "", configWork},
		{"the default is under the home cache", "", "", "", filepath.Join(home, ".cache", "clip-demo-photon")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(WorkEnv, test.env)

			loaded, err := Load(validStoryboard, test.configFlag, test.workFlag)

			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			work, err := loaded.Work()
			if err != nil {
				t.Fatalf("Work: %v", err)
			}
			if work != test.want {
				t.Errorf("Work = %q, want %q", work, test.want)
			}
			take, err := loaded.Take()
			if err != nil {
				t.Fatalf("Take: %v", err)
			}
			if wantTake := filepath.Join(test.want, "demo.take"); take != wantTake {
				t.Errorf("Take = %q, want %q", take, wantTake)
			}
		})
	}
}

func TestLoad_WithoutAHomeDirStillLoadsAndDefersTheWorkDirFailure(t *testing.T) {
	t.Setenv(WorkEnv, "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	loaded, err := Load(validStoryboard, "", "")

	if err != nil {
		t.Fatalf("Load: %v, want the clip loaded without resolving its work dir", err)
	}
	if _, err := loaded.Work(); err == nil {
		t.Error("Work: no error, want the missing home dir reported")
	}
	if _, err := loaded.Take(); err == nil {
		t.Error("Take: no error, want the missing home dir reported")
	}
}

func TestLoad_FindsTheConfigBesideTheStoryboard(t *testing.T) {
	t.Setenv(WorkEnv, "")
	want, err := filepath.Abs(filepath.Join("testdata", "valid", config.FileName))
	if err != nil {
		t.Fatalf("resolve the config path: %v", err)
	}

	loaded, err := Load(validStoryboard, "", "")

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Config.Path != want {
		t.Errorf("Config.Path = %q, want %q", loaded.Config.Path, want)
	}
	if loaded.Storyboard.Clip != "demo" {
		t.Errorf("Storyboard.Clip = %q, want %q", loaded.Storyboard.Clip, "demo")
	}
}

func TestLoad_UndeclaredAreaFailsNamingIt(t *testing.T) {
	t.Setenv(WorkEnv, "")
	storyboardPath := filepath.Join("testdata", "undeclared", "demo.yaml")
	want := []string{
		`expect[0]: area: "footer" is not declared`,
		`beat 1: do[0]: click.target.area: "sidebar" is not declared`,
		`beat 1: do[1]: drag.via[1].area: "shelf" is not declared`,
		`beat 1: do[1]: drag.to.area: "trash" is not declared`,
	}

	_, err := Load(storyboardPath, "", "")

	var crossCheckErr *ValidationError
	if !errors.As(err, &crossCheckErr) {
		t.Fatalf("Load error = %v, want a *ValidationError", err)
	}
	got := make([]string, 0, len(crossCheckErr.Problems))
	for _, problem := range crossCheckErr.Problems {
		got = append(got, problem.Error())
	}
	if len(got) != len(want) {
		t.Fatalf("problems = %q, want %d of them", got, len(want))
	}
	for index, prefix := range want {
		if !strings.HasPrefix(got[index], prefix) {
			t.Errorf("problem %d = %q, want it to start %q", index, got[index], prefix)
		}
	}
	if !strings.Contains(err.Error(), storyboardPath) {
		t.Errorf("error %q does not name the storyboard %s", err, storyboardPath)
	}
}

func TestLoad_ReportsStoryboardAndConfigProblemsTogether(t *testing.T) {
	t.Setenv(WorkEnv, "")

	_, err := Load(filepath.Join("testdata", "broken", "demo.yaml"), "", "")

	var boardErr *storyboard.ValidationError
	if !errors.As(err, &boardErr) {
		t.Errorf("Load error = %v, want it to hold a *storyboard.ValidationError", err)
	}
	var configErr *config.ValidationError
	if !errors.As(err, &configErr) {
		t.Errorf("Load error = %v, want it to hold a *config.ValidationError", err)
	}
	var crossCheckErr *ValidationError
	if errors.As(err, &crossCheckErr) {
		t.Errorf("Load error = %v, want no cross-check while a file is broken", err)
	}
	for _, fragment := range []string{"clip: missing", "launch.command"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error %q does not mention %q", err, fragment)
		}
	}
}

func TestLoad_MissingConfigStillReportsTheStoryboard(t *testing.T) {
	t.Setenv(WorkEnv, "")
	dir := t.TempDir()
	broken, err := os.ReadFile(filepath.Join("testdata", "broken", "demo.yaml"))
	if err != nil {
		t.Fatalf("read the broken storyboard: %v", err)
	}
	storyboardPath := filepath.Join(dir, "demo.yaml")
	if err := os.WriteFile(storyboardPath, broken, 0o600); err != nil {
		t.Fatalf("write the storyboard: %v", err)
	}

	_, err = Load(storyboardPath, "", "")

	if !errors.Is(err, config.ErrNotFound) {
		t.Errorf("Load error = %v, want it to wrap config.ErrNotFound", err)
	}
	var boardErr *storyboard.ValidationError
	if !errors.As(err, &boardErr) {
		t.Errorf("Load error = %v, want it to hold a *storyboard.ValidationError", err)
	}
}

func TestLoad_ConfigFlagWinsOverTheNearestConfig(t *testing.T) {
	t.Setenv(WorkEnv, "")
	flag := filepath.Join("testdata", "valid", "with-work.yaml")

	loaded, err := Load(validStoryboard, flag, "")

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if filepath.Base(loaded.Config.Path) != "with-work.yaml" {
		t.Errorf("Config.Path = %q, want the flag's %s", loaded.Config.Path, flag)
	}
}

func TestWithStoryboard_UndeclaredAreaFailsNamingIt(t *testing.T) {
	t.Setenv(WorkEnv, "")
	loaded, err := Load(validStoryboard, "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	undeclaredPath := filepath.Join("testdata", "undeclared", "demo.yaml")
	undeclared, err := storyboard.Load(undeclaredPath)
	if err != nil {
		t.Fatalf("load the undeclared storyboard: %v", err)
	}

	_, err = loaded.WithStoryboard(undeclared)

	var crossCheckErr *ValidationError
	if !errors.As(err, &crossCheckErr) {
		t.Fatalf("WithStoryboard error = %v, want a *ValidationError", err)
	}
	for _, fragment := range []string{undeclaredPath, `"sidebar" is not declared`} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error %q does not mention %q", err, fragment)
		}
	}
	wantOwners := map[string]storyboard.Owner{
		`expect[0]: area: "footer"`:                   {Kind: storyboard.OwnerClip},
		`beat 1: do[0]: click.target.area: "sidebar"`: {Kind: storyboard.OwnerBeat, Index: 1},
	}
	for prefix, wantOwner := range wantOwners {
		problem := problemStarting(crossCheckErr.Problems, prefix)
		if problem == nil {
			t.Errorf("problems = %q, want one starting %q", crossCheckErr.Problems, prefix)
			continue
		}
		if got := storyboard.OwnerOf(problem); got != wantOwner {
			t.Errorf("owner of %q = %+v, want %+v", problem, got, wantOwner)
		}
	}
}

// problemStarting returns the first of problems whose message starts with prefix, or nil.
func problemStarting(problems []error, prefix string) error {
	for _, problem := range problems {
		if strings.HasPrefix(problem.Error(), prefix) {
			return problem
		}
	}
	return nil
}

func TestWithStoryboard_UndeclaredCameraAreaFailsNamingIt(t *testing.T) {
	t.Setenv(WorkEnv, "")
	loaded, err := Load(validStoryboard, "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	raw, err := os.ReadFile(validStoryboard)
	if err != nil {
		t.Fatalf("read the storyboard: %v", err)
	}
	// A second beat lengthens the clip to hold the positions and zooms, which start at 4s.
	withCamera := string(raw) + `  - id: 2
    title: look around
    duration: 4s
camera:
  positions:
    - {text: 'Saved', area: footer, at: 4s, hold: 1s}
    - {text: 'Item', area: list, at: 5s}
    - {text: 'Bin', area: trash, at: 6s}
    - {cell: {x: 1, y: 1}, at: 7s}
  zooms:
    - {to: 2, at: 5s, over: 1s}
    - {to: 1, at: 7s, over: 500ms}
`
	board, err := storyboard.Parse([]byte(withCamera), validStoryboard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{
		`camera.positions[0]: area: "footer" is not declared`,
		`camera.positions[2]: area: "trash" is not declared`,
	}

	_, err = loaded.WithStoryboard(board)

	var crossCheckErr *ValidationError
	if !errors.As(err, &crossCheckErr) {
		t.Fatalf("WithStoryboard error = %v, want a *ValidationError", err)
	}
	if len(crossCheckErr.Problems) != len(want) {
		t.Fatalf("problems = %q, want %d of them", crossCheckErr.Problems, len(want))
	}
	for index, prefix := range want {
		if got := crossCheckErr.Problems[index].Error(); !strings.HasPrefix(got, prefix) {
			t.Errorf("problem %d = %q, want it to start %q", index, got, prefix)
		}
	}
	wantOwners := []storyboard.Owner{
		{Kind: storyboard.OwnerPosition, Index: 0},
		{Kind: storyboard.OwnerPosition, Index: 2},
	}
	for index, wantOwner := range wantOwners {
		if got := storyboard.OwnerOf(crossCheckErr.Problems[index]); got != wantOwner {
			t.Errorf("problem %d owner = %+v, want %+v", index, got, wantOwner)
		}
	}
}

func TestWithStoryboard_KeepsTheConfigAndWorkFlag(t *testing.T) {
	t.Setenv(WorkEnv, "")
	flagWork := filepath.Join(t.TempDir(), "flag")
	loaded, err := Load(validStoryboard, "", flagWork)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	raw, err := os.ReadFile(validStoryboard)
	if err != nil {
		t.Fatalf("read the storyboard: %v", err)
	}
	edited := strings.Replace(string(raw), "duration: 4s", "duration: 5s", 1)
	board, err := storyboard.Parse([]byte(edited), validStoryboard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	replaced, err := loaded.WithStoryboard(board)

	if err != nil {
		t.Fatalf("WithStoryboard: %v", err)
	}
	if replaced.Storyboard != board || replaced.Config != loaded.Config {
		t.Errorf("WithStoryboard kept storyboard %p and config %p, want %p and %p",
			replaced.Storyboard, replaced.Config, board, loaded.Config)
	}
	if loaded.Storyboard == board {
		t.Errorf("WithStoryboard changed the original clip's storyboard")
	}
	for _, accessor := range []struct {
		name string
		get  func(*Clip) (string, error)
	}{{"Work", (*Clip).Work}, {"Take", (*Clip).Take}} {
		want, wantErr := accessor.get(loaded)
		got, gotErr := accessor.get(replaced)
		if wantErr != nil || gotErr != nil || got != want {
			t.Errorf("%s = %q (%v), want %q (%v)", accessor.name, got, gotErr, want, wantErr)
		}
	}
}

// commandConfig and urlConfig are a project config of each launch kind, declaring no area.
const (
	commandConfig = "name: clip-demo\nlaunch:\n  command: ./app --demo\n"
	urlConfig     = "name: clip-demo\nlaunch:\n  url: http://127.0.0.1:8080/\n"
)

// terminalFrame and pageFrame are a storyboard frame of each kind.
const (
	terminalFrame = "{cols: 80, rows: 24, padding: 16, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 24, max_colors: 128}"
	pageFrame     = "{viewport: {width: 1280, height: 800}, scale: 1, width: 640, fps: 24, max_colors: 128}"
)

// launchBoard is a storyboard of frame whose one beat clicks target and whose one camera
// position aims at point.
func launchBoard(frame, target, point string) string {
	return "clip: demo\nship: out/demo.gif\nframe: " + frame + `
beats:
  - id: 1
    title: click
    duration: 2s
    do:
      - click: {target: ` + target + `}
      - drag: {from: {text: 'Grip'}, to: {offset: {x: 4}}}
camera:
  positions:
    - ` + point + `
`
}

// writeClip writes config and board as photon.yaml and demo.yaml in a fresh temp dir and returns
// the storyboard's path.
func writeClip(t *testing.T, config, board string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "photon.yaml"), []byte(config), 0o600); err != nil {
		t.Fatalf("write the config: %v", err)
	}
	path := filepath.Join(dir, "demo.yaml")
	if err := os.WriteFile(path, []byte(board), 0o600); err != nil {
		t.Fatalf("write the storyboard: %v", err)
	}
	return path
}

func TestLoad_CrossChecksTheLaunchKind(t *testing.T) {
	wholeClip := storyboard.Owner{Kind: storyboard.OwnerClip}
	textTarget, textPoint := "{text: 'Save'}", "{text: 'Save', at: 1s}"
	cases := []struct {
		name   string
		config string
		board  string
		// want is the start of each problem's message, in order; none when the clip loads.
		want []string
		// owners is the owner of each wanted problem.
		owners []storyboard.Owner
	}{
		{name: "a command launch with a terminal frame", config: commandConfig,
			board: launchBoard(terminalFrame, textTarget, textPoint)},
		{name: "a url launch with a viewport and selector targets", config: urlConfig,
			board: launchBoard(pageFrame, "{selector: '#save'}", "{selector: '#save', at: 1s}")},
		{name: "a url launch with a device scale and text targets", config: urlConfig,
			board: launchBoard(strings.Replace(pageFrame, "scale: 1", "scale: 1, device_scale: 2", 1), textTarget, textPoint)},
		{
			name: "a url launch without a viewport", config: urlConfig,
			board: launchBoard(terminalFrame, textTarget, textPoint),
			want: []string{
				"frame.viewport: missing",
				"frame.cols: a url launch takes no cols",
				"frame.rows: a url launch takes no rows",
				"frame.padding: a url launch takes no padding",
				"frame.font_size: a url launch takes no font_size",
				"frame.line_height: a url launch takes no line_height",
			},
			owners: slices.Repeat([]storyboard.Owner{wholeClip}, 6),
		},
		{
			name: "a command launch with a viewport", config: commandConfig,
			board:  launchBoard(pageFrame, textTarget, textPoint),
			want:   []string{"frame.viewport: only a url launch takes a viewport"},
			owners: []storyboard.Owner{wholeClip},
		},
		{
			name: "a command launch with a device scale", config: commandConfig,
			board:  launchBoard(strings.Replace(terminalFrame, "scale: 2", "scale: 2, device_scale: 2", 1), textTarget, textPoint),
			want:   []string{"frame.device_scale: only a url launch takes a device_scale"},
			owners: []storyboard.Owner{wholeClip},
		},
		{
			name: "a command launch with selector targets", config: commandConfig,
			board: strings.Replace(launchBoard(terminalFrame, "{selector: '#save'}", "{selector: '#save', at: 1s}"),
				"to: {offset: {x: 4}}", "to: {selector: '#drop'}", 1),
			want: []string{
				"beat 1: do[0]: click.target.selector: only a url launch takes a selector",
				"beat 1: do[1]: drag.to.selector: only a url launch takes a selector",
				"camera.positions[0]: selector: only a url launch takes a selector",
			},
			owners: []storyboard.Owner{
				{Kind: storyboard.OwnerBeat, Index: 1},
				{Kind: storyboard.OwnerBeat, Index: 1},
				{Kind: storyboard.OwnerPosition, Index: 0},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeClip(t, tc.config, tc.board)

			_, err := Load(path, "", "")

			if len(tc.want) == 0 {
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				return
			}
			var crossCheckErr *ValidationError
			if !errors.As(err, &crossCheckErr) {
				t.Fatalf("Load error = %v, want a *ValidationError", err)
			}
			if len(crossCheckErr.Problems) != len(tc.want) {
				t.Fatalf("problems = %q, want %q", crossCheckErr.Problems, tc.want)
			}
			for index, problem := range crossCheckErr.Problems {
				if !strings.HasPrefix(problem.Error(), tc.want[index]) {
					t.Errorf("problem %d = %q, want it to start %q", index, problem, tc.want[index])
				}
				if got := storyboard.OwnerOf(problem); got != tc.owners[index] {
					t.Errorf("owner of %q = %+v, want %+v", problem, got, tc.owners[index])
				}
			}
		})
	}
}

func TestSelectors_ListsEverySelectorTheClipNames(t *testing.T) {
	config := urlConfig + `areas:
  panel:
    selector: '.panel'
  header:
    top: {match: 'Inbox'}
    bottom: {match: 'Inbox'}
`
	board := "clip: demo\nship: out/demo.gif\nframe: " + pageFrame + `
beats:
  - id: 1
    title: click and drag
    duration: 2s
    do:
      - click: {target: {selector: '#save', area: panel}}
      - drag: {from: {selector: '.grip'}, via: [{selector: '.lane'}, {offset: {x: 4}}], to: {selector: '#drop'}}
      - click: {target: {text: 'Save'}}
    expect:
      - {seen: 'Saved', area: panel}
camera:
  positions:
    - {selector: '#save', at: 1s}
    - {selector: '.toolbar', at: 1500ms}
`
	loaded, err := Load(writeClip(t, config, board), "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := loaded.Selectors()

	want := []string{"#drop", "#save", ".grip", ".lane", ".panel", ".toolbar"}
	if !slices.Equal(got, want) {
		t.Errorf("Selectors = %q, want %q", got, want)
	}
}

func TestSelectors_NoneOnATextClip(t *testing.T) {
	loaded, err := Load(writeClip(t, commandConfig,
		launchBoard(terminalFrame, "{text: 'Save'}", "{text: 'Save', at: 1s}")), "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := loaded.Selectors(); got != nil {
		t.Errorf("Selectors = %q, want none", got)
	}
}
