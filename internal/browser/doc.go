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
// From its load on, the browser records a take from the page's screencast, which pushes a PNG
// frame each time the page paints. A frame that changes the picture or the text layer is a
// snapshot: its entry in the take's image layer, encoded by [pixels.Encoder] (none when only the
// text changed), its text runs and its selector boxes, read as the frame arrives. The take's
// clock starts at the first frame, and it keeps at most [Options.FPS] snapshots a second: a frame
// sooner than that after the last snapshot waits for the interval's boundary, a newer one
// replacing it, so the last picture before the page goes still is always kept.
// [Browser.Current], [Browser.Match], [Browser.Event], [Browser.Settle], [Browser.Freeze] and
// [Browser.Done] work as a terminal's do, but that a quiet spell counts from Settle's call at the
// earliest, since a change's frame arrives a moment after the page paints it; [Browser.Close]
// returns the take, its grid the viewport in CSS pixels and its cell the device scale.
//
// [Browser.Type], [Browser.Key] and [Browser.Pointer] drive the page as the engine's Source drives
// any program, through the DevTools input events a real keyboard and mouse would raise.
// Typing sends each rune as a keyDown, a char and a keyUp, as a US keyboard sends them. A named key
// is one [keys.Lookup] accepts, pressed with the modifiers its name holds; the table of the keys a
// page can receive covers every one of them, and any other name is an error naming it. A pointer
// event is a press, a move with the button held or a release at its cell, one CSS pixel of the
// viewport, with its button and its modifier keys, converted from the SGR bits [keys.Mods] holds
// to the protocol's own.
//
// It imports chromedp, and cdproto through it, to drive the browser over the Chrome DevTools
// Protocol: the protocol's pipe, its typed commands and the browser's process lifetime are what
// chromedp owns, and photon would otherwise write them itself.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - browser.go: [Options], [Start] and [Browser.Close], which run headless Chromium at a url
//     and viewport, and the methods a recording is driven and read through.
//   - find.go: [ErrNoBrowser] and the lookup of the browser binary from a named path or the fixed
//     list.
//   - input.go: [Browser.Type], [Browser.Key] and [Browser.Pointer], which dispatch typed text,
//     named keys and pointer events to the page.
//   - keymap.go: the table from photon's key names to the keystrokes a page receives, and the
//     key events a press or a typed rune dispatches.
//   - record.go: the screencast's pumps and the recording that keeps its frames as snapshots,
//     one frame interval apart at most.
//   - textlayer.go: [Browser.ReadText], which runs textlayer.js on the page and converts its
//     graphemes and rects to the text layer's runs and the selectors' boxes.
package browser
