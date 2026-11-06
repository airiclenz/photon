//go:build browser && !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/browser"
	"github.com/airiclenz/photon/internal/clip"
)

// browserRequiredEnv, set to anything, turns a missing Chromium from a skip into a failure, as CI
// sets it.
const browserRequiredEnv = "PHOTON_TEST_BROWSER_REQUIRED"

// TestRecordTunePage sets up and records the tune-page example in a fresh work dir: setup builds
// photon and a take of the hello example, and the take films photon tune on it, every expect
// passing.
func TestRecordTunePage(t *testing.T) {
	t.Setenv(clip.WorkEnv, "")
	work := t.TempDir()

	code, stdout, stderr := runLint(t, []string{"setup", tunePageStoryboard, "--work", work})
	if code != exitOK {
		t.Fatalf("setup exit code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout, stderr)
	}

	code, stdout, stderr = runLint(t, []string{"record", tunePageStoryboard, "--work", work})

	if code != exitOK && strings.Contains(stderr, browser.ErrNoBrowser.Error()) &&
		os.Getenv(browserRequiredEnv) == "" {
		t.Skipf("%s (set %s to fail instead)", stderr, browserRequiredEnv)
	}
	if code != exitOK {
		t.Errorf("record exit code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout, stderr)
	}
	assertTake(t, filepath.Join(work, "tune-page.take"))
	for _, want := range []string{"beat 5 | PASS | seen /Saved/ in area transport", "take   | PASS | exit 0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q does not carry %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "FAIL") {
		t.Errorf("stdout %q carries a FAIL row", stdout)
	}
}
