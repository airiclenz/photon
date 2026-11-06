//go:build !windows

package browser

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The variables the env case sets: inheritedEntry in photon's own env only, markerEntry in
// Options.Env only.
const (
	inheritedEntry = "PHOTON_TEST_INHERITED=from-photon"
	markerEntry    = "PHOTON_TEST_MARKER=from-options"
)

func TestStart_Env_IsTheBrowsersWholeEnv(t *testing.T) {
	inheritedName, inheritedValue, _ := strings.Cut(inheritedEntry, "=")
	t.Setenv(inheritedName, inheritedValue)
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env")
	browser := filepath.Join(dir, "browser")
	script := "#!/bin/sh\n/usr/bin/env > '" + envFile + "'\nexit 1\n"
	if err := os.WriteFile(browser, []byte(script), 0o755); err != nil {
		t.Fatalf("write the browser script: %v", err)
	}

	b, err := Start(context.Background(), Options{
		URL:         "about:blank",
		ExecPath:    browser,
		Width:       320,
		Height:      240,
		DeviceScale: 1,
		FPS:         10,
		Env:         []string{markerEntry},
	})
	if err == nil {
		if _, closeErr := b.Close(); closeErr != nil {
			t.Errorf("Close: %v", closeErr)
		}
		t.Fatal("Start succeeded with a browser that exits at once, want an error")
	}

	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read the browser's env: %v", err)
	}
	entries := strings.Split(strings.TrimSpace(string(data)), "\n")
	if !slices.Contains(entries, markerEntry) {
		t.Errorf("browser env = %q, want it to hold %q", entries, markerEntry)
	}
	if slices.Contains(entries, inheritedEntry) {
		t.Errorf("browser env = %q, want it without photon's own %q", entries, inheritedEntry)
	}
}
