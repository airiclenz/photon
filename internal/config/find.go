package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileName is the name of a project's config file, the one [Find] walks up looking for.
const FileName = "photon.yaml"

// ErrNotFound is returned, wrapped, when no flag names a config and no [FileName] sits at or
// above the storyboard's directory.
var ErrNotFound = errors.New("no " + FileName + " found")

// Find returns the path of the config a storyboard in storyboardDir uses. A non-empty flag wins
// and is returned as given, without checking that it exists ([Load] reports that). Otherwise
// Find walks up from storyboardDir — made absolute first, so a relative dir such as "." still
// reaches its parents — and returns the first [FileName] it meets, at the dir itself or the
// nearest ancestor. A directory that happens to be named photon.yaml is not a config and is
// passed over. When the walk reaches the filesystem root without a match, the error wraps
// [ErrNotFound].
func Find(flag, storyboardDir string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	dir, err := filepath.Abs(storyboardDir)
	if err != nil {
		return "", fmt.Errorf("find %s: %w", FileName, err)
	}
	start := dir
	for {
		candidate := filepath.Join(dir, FileName)
		info, err := os.Stat(candidate)
		switch {
		case err == nil && !info.IsDir():
			return candidate, nil
		case err != nil && !errors.Is(err, os.ErrNotExist):
			return "", fmt.Errorf("find %s: %w", FileName, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w at or above %s (name one with --config)", ErrNotFound, start)
		}
		dir = parent
	}
}
