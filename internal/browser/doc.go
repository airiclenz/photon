// Package browser is the Chromium capture source behind a url launch: it starts headless Chromium
// at the launch's url with the clip's viewport and device scale, and closes it when the take ends.
//
// [Start] finds the browser first: the path [Options.ExecPath] names (the config's
// launch.browser) when it is set, and otherwise the first of a fixed list of Chromium and Chrome
// names and paths found on PATH or on disk. When none is found it fails with [ErrNoBrowser]. The
// browser runs headless with the GPU off and a throwaway profile; it runs without Chromium's
// sandbox only as root on Linux, where Chromium refuses to start with it. The page's viewport is
// exactly [Options.Width] by [Options.Height] CSS pixels at [Options.DeviceScale] device pixels
// each, set before the page loads, and [Start] returns once the page has fired its load event.
//
// [Browser.ReadText] reads what the page shows at a moment as a take's snapshot holds it: its
// text layer, one [take.TextRun] per stretch of a text node on one visual line, and the boxes of
// the elements each selector picks, all in CSS pixels. The runs of one visual line share one top
// and height, so the target resolver groups them as one line, and a run's text is what the page
// renders, collapsed whitespace dropped. A script the page runs, embedded from textlayer.js,
// measures the text and the boxes; the Go side turns its rects into runs and boxes.
//
// It imports chromedp, and cdproto through it, to drive the browser over the Chrome DevTools
// Protocol: the protocol's pipe, its typed commands and the browser's process lifetime are what
// chromedp owns, and photon would otherwise write them itself.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - browser.go: [Options], and [Start] and [Browser.Close], which run headless Chromium at a
//     url and viewport.
//   - find.go: [ErrNoBrowser] and the lookup of the browser binary from a named path or the fixed
//     list.
//   - textlayer.go: [Browser.ReadText], which runs textlayer.js on the page and converts its
//     graphemes and rects to the text layer's runs and the selectors' boxes.
package browser
