//go:build browser

package browser

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// The browser test's environment: browserPathEnv names the Chromium to drive in place of the one
// found on PATH, and browserRequiredEnv, set to anything, turns a missing Chromium from a skip
// into a failure, as CI sets it.
const (
	browserPathEnv     = "PHOTON_TEST_BROWSER"
	browserRequiredEnv = "PHOTON_TEST_BROWSER_REQUIRED"
)

// The viewport the test asks for: small, to keep a Pi host's memory in bounds, and a device
// scale other than 1, so a scale left at the browser's default shows; and the most snapshots a
// second the take keeps.
const (
	viewportWidth  = 640
	viewportHeight = 480
	deviceScale    = 2
	testFPS        = 24
)

// testTitle is the title of testPage, the page the test loads; it tells the page apart from
// about:blank.
const (
	testTitle = "photon browser test"
	testPage  = `<!doctype html><title>` + testTitle + `</title><p>hello</p>`
)

// refusedSelector is a selector every page refuses as invalid.
const refusedSelector = "button >"

func TestBrowser_Start_LoadsThePageAtTheViewportAndScale(t *testing.T) {
	execPath := browserOrSkip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, testPage)
	}))
	t.Cleanup(server.Close)

	b, err := Start(context.Background(), Options{
		URL:         server.URL,
		ExecPath:    execPath,
		Width:       viewportWidth,
		Height:      viewportHeight,
		DeviceScale: deviceScale,
		FPS:         testFPS,
		Env:         os.Environ(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if _, err := b.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	got, err := chromedp.Run(b.tab, chromedp.Evaluate[pageState](
		`({title: document.title, width: window.innerWidth, height: window.innerHeight, `+
			`scale: window.devicePixelRatio})`,
	))
	if err != nil {
		t.Fatalf("read the page: %v", err)
	}
	if got.Title != testTitle || got.Width != viewportWidth ||
		got.Height != viewportHeight || got.Scale != deviceScale {
		t.Errorf("page = %+v, want the test page at %dx%d, scale %d",
			got, viewportWidth, viewportHeight, deviceScale)
	}
}

func TestBrowser_Start_RefusedSelectorFailsNamingIt(t *testing.T) {
	execPath := browserOrSkip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, testPage); err != nil {
			t.Errorf("serve the page: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	b, err := Start(context.Background(), Options{
		URL:         server.URL,
		ExecPath:    execPath,
		Width:       viewportWidth,
		Height:      viewportHeight,
		DeviceScale: deviceScale,
		FPS:         testFPS,
		Env:         os.Environ(),
		Selectors:   []string{"p", refusedSelector},
	})

	if err == nil {
		if _, closeErr := b.Close(); closeErr != nil {
			t.Errorf("Close: %v", closeErr)
		}
		t.Fatalf("Start succeeded, want an error naming selector %q", refusedSelector)
	}
	if !strings.Contains(err.Error(), `selector "`+refusedSelector+`"`) {
		t.Errorf("Start error = %v, want one naming selector %q", err, refusedSelector)
	}
}

// pageState is what the test reads back from the loaded page.
type pageState struct {
	Title  string  `json:"title"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float64 `json:"scale"`
}

// browserOrSkip is the ExecPath the test runs with: the browser browserPathEnv names, or none,
// leaving Start to find one. With no browser to find it skips the test, or fails it when
// browserRequiredEnv is set.
func browserOrSkip(t *testing.T) string {
	t.Helper()
	execPath := os.Getenv(browserPathEnv)
	_, err := findBrowser(execPath, discoveryNames)
	if err == nil {
		return execPath
	}
	if !errors.Is(err, ErrNoBrowser) || os.Getenv(browserRequiredEnv) != "" {
		t.Fatalf("%v (set %s to name one)", err, browserPathEnv)
	}
	t.Skipf("%v (set %s to name one)", err, browserPathEnv)
	return ""
}
