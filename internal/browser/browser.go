package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/chromedp/chromedp"
)

// loadTimeout caps setting the viewport and loading the page once the browser runs; the browser's
// own start is capped by chromedp's wait for its DevTools address.
const loadTimeout = 30 * time.Second

// closeTimeout caps waiting for the browser to close gracefully before its process is stopped.
const closeTimeout = 10 * time.Second

// rootUID is the user id of root, under which Chromium on Linux refuses to start with its
// sandbox.
const rootUID = 0

// Options is how [Start] runs the browser.
//
// URL is the page to load. ExecPath, when set, is the browser binary to run instead of the one
// [Start] finds. Width and Height are the viewport in CSS pixels and DeviceScale the device
// pixels per CSS pixel; all three must be positive. Env holds extra KEY=VALUE variables for the
// browser process, on top of photon's own environment.
type Options struct {
	URL         string
	ExecPath    string
	Width       int
	Height      int
	DeviceScale float64
	Env         []string
}

// Browser is a running headless Chromium with one tab at the page [Start] loaded. It runs until
// [Browser.Close], or until the context [Start] was given ends.
type Browser struct {
	tab             context.Context
	cancelTab       context.CancelFunc
	cancelAllocator context.CancelFunc
}

// Start runs headless Chromium, sets its viewport to options' size and scale and loads
// options.URL, returning once the page has loaded. It fails with an error wrapping [ErrNoBrowser]
// when it finds no browser to run, and with the browser's own error when it does not start or the
// page does not load within a fixed time; a failed Start leaves no browser running.
func Start(ctx context.Context, options Options) (*Browser, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	execPath, err := findBrowser(options.ExecPath, discoveryNames)
	if err != nil {
		return nil, err
	}

	allocator, cancelAllocator := chromedp.NewExecAllocator(
		ctx,
		allocatorOptions(execPath, options)...,
	)
	tab, cancelTab := chromedp.NewContext(allocator)
	b := &Browser{tab: tab, cancelTab: cancelTab, cancelAllocator: cancelAllocator}
	// The first action starts the browser process, which lives as long as the context it runs
	// on: that is the tab's own, so the load timeout below bounds the load and not the browser.
	if err := chromedp.Do(tab); err != nil {
		b.stop()
		return nil, fmt.Errorf("start %s: %w", execPath, err)
	}
	if err := b.load(options); err != nil {
		b.stop()
		return nil, err
	}
	return b, nil
}

// Close closes the browser and waits for its process to exit, returning any error the close
// met. A browser that does not close within a fixed time is stopped.
func (b *Browser) Close() error {
	ctx, cancel := context.WithTimeout(b.tab, closeTimeout)
	defer cancel()
	err := chromedp.Cancel(ctx)
	b.stop()
	if err != nil {
		return fmt.Errorf("close the browser: %w", err)
	}
	return nil
}

// load sets the tab's viewport and device scale, then navigates to the page and waits for its
// load event, within loadTimeout.
func (b *Browser) load(options Options) error {
	ctx, cancel := context.WithTimeout(b.tab, loadTimeout)
	defer cancel()
	err := chromedp.Do(ctx,
		chromedp.EmulateViewport(
			int64(options.Width),
			int64(options.Height),
			chromedp.EmulateScale(options.DeviceScale),
		),
		chromedp.Navigate(options.URL),
	)
	if err != nil {
		return fmt.Errorf("load %s: %w", options.URL, err)
	}
	return nil
}

// stop ends the tab and the browser process without a graceful close, waiting for the process
// to exit.
func (b *Browser) stop() {
	b.cancelTab()
	b.cancelAllocator()
}

// validate reports options that cannot run a browser: no url, or a viewport or device scale
// that is not positive.
func (o Options) validate() error {
	switch {
	case o.URL == "":
		return errors.New("browser: no url to load")
	case o.Width <= 0 || o.Height <= 0:
		return fmt.Errorf("browser: want a positive viewport, got %dx%d", o.Width, o.Height)
	case o.DeviceScale <= 0:
		return fmt.Errorf("browser: want a positive device scale, got %g", o.DeviceScale)
	}
	return nil
}

// allocatorOptions are the flags the browser at execPath runs with: chromedp's headless defaults,
// the GPU off, a window the viewport's size, options' extra environment, and no sandbox where
// Chromium cannot start with one.
func allocatorOptions(execPath string, options Options) []chromedp.ExecAllocatorOption {
	flags := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.DisableGPU,
		chromedp.WindowSize(options.Width, options.Height),
		chromedp.Env(options.Env...),
	)
	if isRootOnLinux() {
		flags = append(flags, chromedp.NoSandbox)
	}
	return flags
}

// isRootOnLinux reports whether photon runs as root on Linux, where Chromium will not start with
// its sandbox on.
func isRootOnLinux() bool {
	return runtime.GOOS == "linux" && os.Geteuid() == rootUID
}
