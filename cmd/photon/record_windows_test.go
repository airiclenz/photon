//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noPTYWording is the part of the refusal a Windows user reads.
const noPTYWording = "recording needs a unix pty"

func TestRecordAndSetupRefuseOnWindows(t *testing.T) {
	for _, command := range []string{"record", "setup"} {
		t.Run(command, func(t *testing.T) {
			work := filepath.Join(t.TempDir(), "work")

			code, _, stderr := runLint(t, []string{command, "demo.yaml", "--work", work})

			if code != exitRunFailed {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, exitRunFailed, stderr)
			}
			if !strings.Contains(stderr, noPTYWording) {
				t.Errorf("stderr %q does not carry %q", stderr, noPTYWording)
			}
			if _, err := os.Stat(work); !os.IsNotExist(err) {
				t.Errorf("the work dir %s was touched (stat: %v); the refusal comes before the rig", work, err)
			}
		})
	}
}
