//go:build browser && !windows

package rig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/browser"
	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/testkit"
)

// The browser test's environment: browserPathEnv names the Chromium to drive in place of the one
// photon finds, and browserRequiredEnv, set to anything, turns a missing Chromium from a skip
// into a failure, as CI sets it.
const (
	browserPathEnv     = "PHOTON_TEST_BROWSER"
	browserRequiredEnv = "PHOTON_TEST_BROWSER_REQUIRED"
)

// pageConfig is the url-launch test project's photon.yaml: the fixture web app as its one
// service, the page at that service's port, and a `demo` profile whose on_success leaves a marker
// naming the take it was handed. The verb fills the launch's browser line.
const pageConfig = `name: rig-test
reset: echo reset ran > "$PHOTON_WORK/` + resetMarker + `"
launch:
  url: 'http://127.0.0.1:${PHOTON_PORT_WEB}/'
%s
end:
  idle: 300ms
services:
  web:
    command: exec %q
    ready: {line: '^ready$'}
profiles:
  demo:
    on_success: echo "$PHOTON_TAKE" > "$PHOTON_WORK/` + succeededMarker + `"
`

// pageStoryboard films the fixture page: it types into the input, clicks the nav's second item
// by selector and the Run button by text, and waits on each answer. The take-level run expect
// passes only while the web service is up and after reset ran.
const pageStoryboard = `clip: ` + clipName + `
ship: out/demo.gif
frame: {viewport: {width: 640, height: 400}, scale: 1, width: 640, fps: 10, max_colors: 128}
expect:
  - seen: 'fixture web'
  - run: test -n "$PHOTON_PORT_WEB" && test -e "$PHOTON_WORK/` + resetMarker + `"
beats:
  - id: 1
    title: type
    duration: 1s
    do:
      - wait: {screen: 'ready', timeout: 10s}
      - click: {target: {selector: '#input'}}
      - type: {text: 'hi', humanize: false}
      - wait: {screen: 'you typed: hi', timeout: 10s}
    expect:
      - seen: 'you typed: hi'
  - id: 2
    title: pick
    duration: 1s
    do:
      - click: {target: {selector: 'nav a', nth: 2}}
      - wait: {screen: 'selected: beta', timeout: 10s}
    expect:
      - seen: 'selected: beta'
  - id: 3
    title: run
    duration: 1s
    do:
      - click: {target: {text: '^Run$'}}
      - wait: {screen: 'clicked: Run', timeout: 10s}
    expect:
      - seen: 'clicked: Run'
`

// writePageProject writes the url-launch test project into a fresh dir and loads its clip, with
// the work dir under that dir.
func writePageProject(t *testing.T) (*clip.Clip, string) {
	t.Helper()
	dir := t.TempDir()
	browserLine := ""
	if execPath := os.Getenv(browserPathEnv); execPath != "" {
		browserLine = fmt.Sprintf("  browser: %q", execPath)
	}
	files := map[string]string{
		configFile:     fmt.Sprintf(pageConfig, browserLine, testkit.BuildFixtureWeb(t)),
		storyboardFile: pageStoryboard,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	work := filepath.Join(dir, "work")
	loaded, err := clip.Load(filepath.Join(dir, storyboardFile), "", work)
	if err != nil {
		t.Fatalf("load the clip: %v", err)
	}
	return loaded, work
}

func TestBrowser_Record_URLLaunchPassesItsCheck(t *testing.T) {
	loaded, work := writePageProject(t)
	var out bytes.Buffer

	rows, err := Record(context.Background(), loaded, "demo", &out)

	if errors.Is(err, browser.ErrNoBrowser) && os.Getenv(browserRequiredEnv) == "" {
		t.Skipf("%v (set %s to name one)", err, browserPathEnv)
	}
	if err != nil {
		t.Fatalf("Record: %v\noutput:\n%s", err, out.String())
	}
	assertAllPass(t, rows, 5)
	recorded := assertTakeWritten(t, work, out.String())
	if recorded.Cols != 640 || recorded.Rows != 400 || len(recorded.Snapshots) == 0 ||
		recorded.Snapshots[0].Image == nil {
		t.Errorf("take = %dx%d with %d snapshots; want the 640x400 viewport with an image layer",
			recorded.Cols, recorded.Rows, len(recorded.Snapshots))
	}
	if screen := lastScreen(t, recorded); !strings.Contains(screen, "clicked: Run") {
		t.Errorf("the take's last screen is not the page's settled one:\n%s", screen)
	}
	named, err := os.ReadFile(filepath.Join(work, succeededMarker))
	if err != nil {
		t.Fatalf("on_success left no marker: %v", err)
	}
	if want := filepath.Join(work, clipName+".take") + "\n"; string(named) != want {
		t.Errorf("on_success was handed the take %q, want %q", named, want)
	}
}
