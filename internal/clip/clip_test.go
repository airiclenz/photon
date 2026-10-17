package clip

import (
	"errors"
	"os"
	"path/filepath"
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
		`beat 1: zoom.target.area: "sidebar" is not declared`,
		`beat 1: do[0]: click.target.area: "sidebar" is not declared`,
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
