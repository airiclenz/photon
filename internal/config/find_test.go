package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// projectTree makes a temp project: a photon.yaml at its root and an empty storyboards/demo dir
// below it. It returns the root and the storyboard dir.
func projectTree(t *testing.T) (root, storyboardDir string) {
	t.Helper()
	root = t.TempDir()
	storyboardDir = filepath.Join(root, "storyboards", "demo")
	if err := os.MkdirAll(storyboardDir, 0o700); err != nil {
		t.Fatalf("make the storyboard dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("name: demo\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return root, storyboardDir
}

func TestFindFlagWins(t *testing.T) {
	t.Parallel()
	_, storyboardDir := projectTree(t)
	flag := filepath.Join(t.TempDir(), "other.yaml")

	found, err := Find(flag, storyboardDir)

	if err != nil || found != flag {
		t.Fatalf("want the flag %q, got %q, %v", flag, found, err)
	}
}

func TestFindWalksUp(t *testing.T) {
	t.Parallel()
	root, storyboardDir := projectTree(t)

	found, err := Find("", storyboardDir)

	if want := filepath.Join(root, FileName); err != nil || found != want {
		t.Fatalf("want %q, got %q, %v", want, found, err)
	}
}

func TestFindPrefersTheNearest(t *testing.T) {
	t.Parallel()
	_, storyboardDir := projectTree(t)
	nearest := filepath.Join(storyboardDir, FileName)
	if err := os.WriteFile(nearest, []byte("name: near\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	found, err := Find("", storyboardDir)

	if err != nil || found != nearest {
		t.Fatalf("want %q, got %q, %v", nearest, found, err)
	}
}

func TestFindSkipsADirectoryNamedLikeTheFile(t *testing.T) {
	t.Parallel()
	root, storyboardDir := projectTree(t)
	if err := os.Mkdir(filepath.Join(storyboardDir, FileName), 0o700); err != nil {
		t.Fatalf("make the decoy dir: %v", err)
	}

	found, err := Find("", storyboardDir)

	if want := filepath.Join(root, FileName); err != nil || found != want {
		t.Fatalf("want %q, got %q, %v", want, found, err)
	}
}

// TestFindFromARelativeDir changes the working directory, so it cannot run in parallel.
func TestFindFromARelativeDir(t *testing.T) {
	root, storyboardDir := projectTree(t)
	t.Chdir(storyboardDir)

	found, err := Find("", ".")

	if want := filepath.Join(root, FileName); err != nil || found != want {
		t.Fatalf("want %q, got %q, %v", want, found, err)
	}
}

func TestFindNoneFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := Find("", dir)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want an error wrapping ErrNotFound, got %v", err)
	}
}
