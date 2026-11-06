//go:build browser

package tune

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// The browser test's environment: browserPathEnv names the Chromium to drive in place of the one
// found on PATH, and browserRequiredEnv, set to anything, turns a missing Chromium from a skip
// into a failure, as CI sets it.
const (
	browserPathEnv     = "PHOTON_TEST_BROWSER"
	browserRequiredEnv = "PHOTON_TEST_BROWSER_REQUIRED"
)

// browserNames are the names and paths a Chromium is looked up under on a unix-like host, in the
// order chromedp's own allocator tries them.
var browserNames = []string{
	"headless_shell",
	"headless-shell",
	"chromium",
	"chromium-browser",
	"google-chrome",
	"google-chrome-stable",
	"google-chrome-beta",
	"google-chrome-unstable",
	"/usr/bin/google-chrome",
	"/usr/local/bin/chrome",
	"/snap/bin/chromium",
	"chrome",
}

// The browser's limits: a small window keeps a Pi host's memory in bounds, browserTimeout caps
// the whole drive, and pollInterval is how often a wait asks the page again.
const (
	windowWidth    = 1024
	windowHeight   = 768
	browserTimeout = 2 * time.Minute
	pollInterval   = 50 * time.Millisecond
	// editedMS is beat 1's duration the edit types, which the storyboard saves as beatOneNew.
	editedMS = 2000
)

// The scrub's press and release, as fractions of the axis track's width from its left edge.
const (
	scrubFrom = 0.25
	scrubTo   = 0.75
)

// TestBrowser_LoadPlayScrubEditSave_WritesTheStoryboard drives the page in headless Chromium
// against a real tune server: it loads, plays, scrubs the time axis, edits beat 1's duration in
// the inspector and saves, and the storyboard on disk carries the edit.
func TestBrowser_LoadPlayScrubEditSave_WritesTheStoryboard(t *testing.T) {
	browser := findBrowser(t)
	f := newSessionFixture(t)
	addr := serveFixture(t, f.session)
	ctx := newBrowser(t, browser)

	steps := []struct {
		name   string
		action chromedp.Action[chromedp.Void]
	}{
		{"load", chromedp.Steps(
			chromedp.Navigate("http://"+addr+"/"),
			waitFor(`window.tune && window.tune.state() !== null && window.tune.frame.naturalWidth > 0`),
		)},
		{"play", chromedp.Steps(
			chromedp.Click(chromedp.ID("play")),
			waitFor(`window.tune.time() > 0`),
			evaluate(`window.tune.pause()`),
		)},
		{"scrub", scrub()},
		{"edit", chromedp.Steps(
			evaluate(changeScript(`document.getElementById("beat-select")`, "1")),
			evaluate(changeScript(durationInput, fmt.Sprint(editedMS))),
			waitFor(fmt.Sprintf(
				`window.tune.state().dirty && window.tune.state().beats[0].duration_ms === %d`,
				editedMS,
			)),
		)},
		{"save", chromedp.Steps(
			chromedp.Click(chromedp.ID("save")),
			waitFor(`!window.tune.state().dirty`),
		)},
	}
	for _, step := range steps {
		if err := chromedp.Do(ctx, step.action); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}

	if got := f.readDisk(t); !strings.Contains(got, beatOneNew) {
		t.Errorf("the saved storyboard does not carry beat 1's %dms:\n%s", editedMS, got)
	}
}

// durationInput is the inspector's duration field for the beat it shows.
const durationInput = `document.evaluate('//label[span[text()="duration (ms)"]]/input', document, ` +
	`null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue`

// changeScript sets the value of the element the expression control names and fires its change
// event, as a person's typing and leaving the field does.
func changeScript(control, value string) string {
	return fmt.Sprintf(
		`(() => { const c = %s; c.value = %q; c.dispatchEvent(new Event("change", {bubbles: true})); })()`,
		control, value,
	)
}

// axisBoxScript measures the time axis's track: its left edge, its width and its middle row, in
// the viewport's pixels.
const axisBoxScript = `(() => {
	const box = document.querySelector(".lane-track.axis").getBoundingClientRect();
	return [box.left, box.width, box.top + box.height / 2];
})()`

// scrub presses the time axis a quarter of the way in, drags to three quarters and lets go: the
// playhead follows the pointer, and the preview settles on the exact frame where it was let go.
func scrub() chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, _ *chromedp.Target) error {
		box, err := chromedp.Run(ctx, chromedp.Evaluate[[]float64](axisBoxScript))
		if err != nil {
			return fmt.Errorf("measure the axis: %w", err)
		}
		left, width, y := box[0], box[1], box[2]
		from, to := left+width*scrubFrom, left+width*scrubTo

		if err := chromedp.Do(ctx,
			chromedp.MouseEvent(input.DispatchMouseEventTypeMousePressed, from, y, chromedp.ButtonLeft),
		); err != nil {
			return fmt.Errorf("press the axis: %w", err)
		}
		pressed, err := chromedp.Run(ctx, chromedp.Evaluate[float64](`window.tune.time()`))
		if err != nil {
			return fmt.Errorf("read the playhead: %w", err)
		}
		if err := chromedp.Do(ctx,
			chromedp.MouseEvent(input.DispatchMouseEventTypeMouseMoved, to, y, chromedp.ButtonLeft),
			chromedp.MouseEvent(input.DispatchMouseEventTypeMouseReleased, to, y, chromedp.ButtonLeft),
			waitFor(fmt.Sprintf(`window.tune.time() > %v`, pressed)),
			waitFor(settledScript),
		); err != nil {
			return fmt.Errorf("drag the axis from %vms: %w", pressed, err)
		}
		return nil
	})
}

// settledScript is true once the preview shows the exact frame under the playhead: the frame
// starting at or before it, less than one frame's length back.
const settledScript = `(() => {
	const at = window.tune.time(), shown = window.tune.frameTime();
	return shown !== null && shown <= at && at - shown < 1000 / window.tune.state().fps &&
		window.tunePage.frameQuality() === "exact";
})()`

// waitFor waits until the expression is truthy on the page.
func waitFor(expression string) chromedp.Action[chromedp.Void] {
	return chromedp.Poll[chromedp.Void](expression, chromedp.WithPollingInterval(pollInterval))
}

// evaluate runs the expression on the page for its effect alone.
func evaluate(expression string) chromedp.Action[chromedp.Void] {
	return chromedp.Evaluate[chromedp.Void](expression)
}

// findBrowser is the Chromium the test drives: the one browserPathEnv names, or else the first
// of browserNames found. With none it skips the test, or fails it when browserRequiredEnv is set.
func findBrowser(t *testing.T) string {
	t.Helper()
	names := browserNames
	if path := os.Getenv(browserPathEnv); path != "" {
		names = []string{path}
	}
	for _, name := range names {
		if found, err := exec.LookPath(name); err == nil {
			return found
		}
	}
	reason := fmt.Sprintf("no Chromium found (tried %s; set %s to name one)",
		strings.Join(names, ", "), browserPathEnv)
	if os.Getenv(browserRequiredEnv) != "" {
		t.Fatalf("%s, and %s requires one", reason, browserRequiredEnv)
	}
	t.Skip(reason)
	return ""
}

// serveFixture serves session on a free loopback port until the test ends and returns the
// address it listens on.
func serveFixture(t *testing.T, session *Session) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- serve(ctx, listener, session, io.Discard, io.Discard) }()
	t.Cleanup(func() {
		cancel()
		if err := <-served; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	return listener.Addr().String()
}

// newBrowser starts browser headless and returns the context of its first tab, which every step
// runs on. The browser stops when the test ends, failing it with any exception the page threw.
func newBrowser(t *testing.T, browser string) context.Context {
	t.Helper()
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.DisableGPU,
		// The browser only ever loads the test's own loopback page, and a CI container may
		// refuse the sandbox's namespaces.
		chromedp.NoSandbox,
		chromedp.WindowSize(windowWidth, windowHeight),
	)
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), options...)
	tab, cancelTab := chromedp.NewContext(allocator, chromedp.WithErrorf(t.Logf))
	stopped, cancelTimeout := context.WithTimeout(tab, browserTimeout)

	var (
		mu         sync.Mutex
		exceptions []string
	)
	if err := chromedp.Listen(tab, func(event any) {
		if thrown, ok := event.(*runtime.EventExceptionThrown); ok {
			mu.Lock()
			exceptions = append(exceptions, exceptionText(thrown.ExceptionDetails))
			mu.Unlock()
		}
	}); err != nil {
		cancelTimeout()
		cancelTab()
		cancelAllocator()
		t.Fatalf("start %s: %v", browser, err)
	}
	t.Cleanup(func() {
		cancelTimeout()
		cancelTab()
		cancelAllocator()
		mu.Lock()
		defer mu.Unlock()
		for _, exception := range exceptions {
			t.Errorf("the page threw: %s", exception)
		}
	})
	return stopped
}

// exceptionText is what an uncaught exception says: its message where the page sent one.
func exceptionText(details *runtime.ExceptionDetails) string {
	if details == nil {
		return "an exception without details"
	}
	if details.Exception != nil && details.Exception.Description != "" {
		return details.Exception.Description
	}
	return details.Text
}
