package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/airiclenz/photon/internal/take"
)

// loadTimeout caps setting the viewport and loading the page once the browser runs; the browser's
// own start is capped by chromedp's wait for its DevTools address.
const loadTimeout = 30 * time.Second

// closeTimeout caps waiting for the browser to close gracefully before its process is stopped.
const closeTimeout = 10 * time.Second

// settlePollInterval is how often [Browser.Settle] looks at how long the page has been still.
const settlePollInterval = 20 * time.Millisecond

// deviceScaleFlag sets the browser window's own device scale. The emulated scale alone lays the
// page out at that scale, but the screencast would still push frames in CSS pixels.
const deviceScaleFlag = "force-device-scale-factor"

// rootUID is the user id of root, under which Chromium on Linux refuses to start with its
// sandbox.
const rootUID = 0

// Options is how [Start] runs the browser.
//
// URL is the page to load. ExecPath, when set, is the browser binary to run instead of the one
// [Start] finds. Width and Height are the viewport in CSS pixels and DeviceScale the device
// pixels per CSS pixel; all three must be positive. FPS caps how many snapshots a second the take
// keeps; it must be positive. Selectors are the selectors whose boxes every snapshot holds. Env
// is the browser process's whole env, as KEY=VALUE entries: nothing of photon's own env reaches
// the browser unless Env holds it.
type Options struct {
	URL         string
	ExecPath    string
	Width       int
	Height      int
	DeviceScale float64
	FPS         int
	Selectors   []string
	Env         []string
}

// Browser is a running headless Chromium with one tab at the page [Start] loaded, recording a
// take from the tab's screencast. It runs until [Browser.Close], or until the context [Start] was
// given ends.
type Browser struct {
	tab             context.Context
	cancelTab       context.CancelFunc
	cancelAllocator context.CancelFunc
	selectors       []string

	rec *recording
	// stopRecording ends the screencast's pumps; recordingDone closes once both have returned.
	stopRecording context.CancelFunc
	recordingDone chan struct{}
	// done closes once the tab or the browser is gone.
	done chan struct{}

	// frameMu guards nextFrame, the newest screencast frame received and not yet recorded, and
	// nextArrived, when it arrived; frameReady signals one is waiting.
	frameMu     sync.Mutex
	nextFrame   *page.EventScreencastFrame
	nextArrived time.Time
	frameReady  chan struct{}

	// errMu guards recordErr, the first error recording a frame met.
	errMu     sync.Mutex
	recordErr error

	closing  sync.Once
	closeErr error
}

// Start runs headless Chromium, sets its viewport to options' size and scale, loads options.URL
// and starts recording the page's screencast, returning once the page has loaded. The take's
// clock starts at the first frame the page paints after its load. It fails with an error wrapping
// [ErrNoBrowser] when it finds no browser to run, with the browser's own error when it does not
// start, the page does not load within a fixed time or its screencast does not start, and with
// [Browser.ReadText]'s error when the loaded page cannot be read or refuses one of
// options.Selectors, naming it; a failed Start leaves no browser running.
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
	b := &Browser{
		tab: tab, cancelTab: cancelTab, cancelAllocator: cancelAllocator,
		selectors: options.Selectors,
	}
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
	// Read once now, so a selector the page refuses fails Start rather than every snapshot.
	if _, _, err := b.ReadText(options.Selectors); err != nil {
		b.stop()
		return nil, err
	}
	if err := b.startRecording(options); err != nil {
		b.stop()
		return nil, err
	}
	return b, nil
}

// Current is what the page shows right now as a snapshot stamped on the take's clock: its text
// layer and selector boxes, read now. It is read, not recorded. A page that cannot be read gives
// the text layer of the newest frame: [Start] has read it once, so this covers a read that fails
// mid-recording.
func (b *Browser) Current() take.Snapshot {
	return b.rec.screen(time.Now(), b.readLayer())
}

// Match reports whether match holds for the page as it stands, and when it does records that
// screen into the take, stamped now. The take keeps one snapshot a frame interval, so a screen
// photon waited on could otherwise come and go between two, and what the take shows is what a
// check later judges. The screen recorded is exactly the screen that matched. Before the page's
// first frame, and after [Browser.Freeze], a match is reported and nothing is recorded.
func (b *Browser) Match(match func(take.Snapshot) bool) bool {
	layer := b.readLayer()
	matched, err := b.rec.match(time.Now(), layer, match)
	b.noteError(err)
	return matched
}

// Event logs event into the take, stamped now on the take's clock: whatever At it carries is
// replaced. An event logged before the page's first frame lands at 0 s; one logged after
// [Browser.Freeze] is dropped.
func (b *Browser) Event(event take.Event) { b.rec.event(time.Now(), event) }

// Done closes once the tab or the browser is gone: the page closed, the browser died, or
// [Browser.Close] closed it.
func (b *Browser) Done() <-chan struct{} { return b.done }

// Settle waits until the page has painted no new frame for quiet, and every frame it painted has
// been taken into the take, and reports whether it did so, or went away, within timeout. A page
// with work in flight is seldom still that long: it animates while it is busy and stops painting
// when it is idle. The quiet spell counts from Settle's call at the earliest: the page pushes a
// change's frame a moment after it paints it, so a page still before the call may have a frame
// on its way. A cancelled ctx reports false.
func (b *Browser) Settle(ctx context.Context, quiet, timeout time.Duration) bool {
	called := time.Now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(settlePollInterval)
	defer poll.Stop()
	for {
		now := time.Now()
		if min(b.rec.quiet(now), now.Sub(called)) >= quiet {
			return true
		}
		select {
		case <-poll.C:
		case <-b.done:
			return true
		case <-deadline.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// Freeze ends the take where it stands: a frame held for its frame interval is recorded, and from
// then on no snapshot or event reaches the take, though the page keeps running. What a page shows
// on its way out is how the recording ended, not part of the clip.
func (b *Browser) Freeze() { b.noteError(b.rec.freeze()) }

// Close ends the recording, closes the browser and waits for its process to exit, returning the
// take and any error recording or closing met. A frame received and not yet recorded, and one
// held for its frame interval, are flushed into the take first, unless [Browser.Freeze] ended the
// take earlier. A browser that does not close within a fixed time is stopped. A second call
// returns the same take and error.
func (b *Browser) Close() (*take.Take, error) {
	b.closing.Do(func() {
		b.stopRecording()
		<-b.recordingDone
		if !b.rec.isFrozen() {
			_, _, err := b.recordNextFrame()
			b.noteError(err)
		}
		b.Freeze()

		ctx, cancel := context.WithTimeout(b.tab, closeTimeout)
		defer cancel()
		err := chromedp.Cancel(ctx)
		b.stop()
		<-b.done
		if err != nil {
			err = fmt.Errorf("close the browser: %w", err)
		}
		b.errMu.Lock()
		b.closeErr = errors.Join(b.recordErr, err)
		b.errMu.Unlock()
	})
	return b.rec.result(), b.closeErr
}

// readLayer reads the page's text layer and the boxes of the browser's selectors now, or gives
// the newest frame's when the page cannot be read. [Start] fails on a selector the page refuses,
// so the fallback covers only a read that fails mid-recording.
func (b *Browser) readLayer() textLayer {
	runs, boxes, err := b.ReadText(b.selectors)
	if err != nil {
		return b.rec.latest()
	}
	return textLayer{runs: runs, selectors: boxes}
}

// noteError keeps err when it is the first error recording met.
func (b *Browser) noteError(err error) {
	if err == nil {
		return
	}
	b.errMu.Lock()
	defer b.errMu.Unlock()
	if b.recordErr == nil {
		b.recordErr = fmt.Errorf("record the page: %w", err)
	}
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

// validate reports options that cannot run a browser: no url, or a viewport, device scale or fps
// that is not positive.
func (o Options) validate() error {
	switch {
	case o.URL == "":
		return errors.New("browser: no url to load")
	case o.Width <= 0 || o.Height <= 0:
		return fmt.Errorf("browser: want a positive viewport, got %dx%d", o.Width, o.Height)
	case o.DeviceScale <= 0:
		return fmt.Errorf("browser: want a positive device scale, got %g", o.DeviceScale)
	case o.FPS <= 0:
		return fmt.Errorf("browser: want a positive fps, got %d", o.FPS)
	}
	return nil
}

// allocatorOptions are the flags the browser at execPath runs with: chromedp's headless defaults,
// the GPU off, a window the viewport's size at options' device scale, options.Env as the browser's
// whole env, and no sandbox where Chromium cannot start with one.
func allocatorOptions(execPath string, options Options) []chromedp.ExecAllocatorOption {
	flags := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.DisableGPU,
		chromedp.WindowSize(options.Width, options.Height),
		chromedp.Flag(deviceScaleFlag, strconv.FormatFloat(options.DeviceScale, 'f', -1, 64)),
		chromedp.Env(options.Env...),
		chromedp.NoInheritEnv,
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
