package browser

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The names the hermetic cases look up: each exists only in, or is absent from, the test's own
// PATH directory, never a browser the host may have installed.
const (
	presentName = "photon-test-browser-present"
	otherName   = "photon-test-browser-other"
	absentName  = "photon-test-browser-absent"
)

func TestFindBrowser_NamedOrListed_FindsTheBrowser(t *testing.T) {
	dir := pathDir(t, presentName, otherName)
	present := filepath.Join(dir, executableName(presentName))
	other := filepath.Join(dir, executableName(otherName))

	cases := []struct {
		name     string
		execPath string
		names    []string
		want     string
	}{
		{"exec path set", other, []string{presentName}, other},
		{"exec path as a bare name", otherName, []string{presentName}, other},
		{"first listed name found", "", []string{presentName, otherName}, present},
		{"absent names skipped", "", []string{absentName, otherName}, other},
		{"listed path found", "", []string{absentName, present}, present},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := findBrowser(c.execPath, c.names)

			if err != nil {
				t.Fatalf("findBrowser: %v", err)
			}
			if !samePath(t, got, c.want) {
				t.Errorf("findBrowser = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFindBrowser_NoneFound_FailsWithErrNoBrowser(t *testing.T) {
	dir := pathDir(t, presentName)

	cases := []struct {
		name     string
		execPath string
		names    []string
	}{
		{"no names", "", nil},
		{"every name absent", "", []string{absentName, filepath.Join(dir, absentName)}},
		{"exec path absent, listed name present", filepath.Join(dir, absentName),
			[]string{presentName}},
		{"exec path not executable", filepath.Join(dir, "plain.txt"), []string{presentName}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := findBrowser(c.execPath, c.names)

			if !errors.Is(err, ErrNoBrowser) {
				t.Errorf("findBrowser error = %v, want ErrNoBrowser", err)
			}
		})
	}
}

// pathDir makes a temp directory holding an executable fake for each of names, plus a plain,
// non-executable file plain.txt, and makes it the test's whole PATH.
func pathDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, executableName(name))
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	plain := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(plain, []byte("not a browser\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", plain, err)
	}
	t.Setenv("PATH", dir)
	return dir
}

// executableName is name as an executable file is named on this host: with .exe on Windows.
func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// samePath reports whether got and want name the same file, so a case passes whatever form
// (case, short name) the host's lookup returns the path in.
func samePath(t *testing.T, got, want string) bool {
	t.Helper()
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat %s: %v", got, err)
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat %s: %v", want, err)
	}
	return os.SameFile(gotInfo, wantInfo)
}
