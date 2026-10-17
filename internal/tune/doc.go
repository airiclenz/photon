// Package tune is the timing editor behind `photon tune`: it previews a storyboard's section
// timing, zoom and frame against an existing take, and saves the edits back into the storyboard.
//
// An [Edit] is one typed change — a beat's duration, hold, cut or zoom, the clip's frame.width
// or frame.fps, a camera move's start, length, ease or destination, or a caption's start, end,
// text, place or align — spelled as the page sends it. Applying one splices only the edited
// scalar, or the one node a moment, a pan destination or a caption's text replaces, into the
// storyboard's source through yamledit, so comments, quoting, key order and flow or block style
// stay as the person wrote them: a duration keeps its unit, a new key goes in the storyboard's
// field order, a default is never written out, switching cut off, the zoom factor to auto, a
// camera move's ease to smooth or a caption's place or align to its default removes the key, and
// a caption's for and until replace each other. The result is parsed again with the storyboard's
// own strict loader, so an edit that leaves the storyboard invalid is refused with the problems
// it causes.
//
// A [Session] holds one clip and take being tuned. Every applied edit, undo, redo or revert
// rebuilds the schedule and the compositor on the new bytes, as `photon render` builds them,
// reusing the rasterizer while the take and the frame's raster geometry stay; a build that fails
// leaves its error standing on the beat or the clip it names, and the last good pipeline stays
// the preview's. [Session.Refresh] takes in outside writes — a rewritten take is read again, a
// changed storyboard is loaded when nothing is unsaved and turns the session stale otherwise —
// and [Session.Save] writes in place, refused while stale ([ErrStale]) or while an error stands
// ([ErrInvalid]). [Session.State] reports everything the page shows, and [Session.Frame] the
// PNG of the frame `photon render` composes at an output time, from the last good pipeline, its
// rasterized snapshots kept in a cache bounded at 1 GiB that a new rasterizer starts afresh.
// [Session.Pick] turns cells picked on that frame into a [Proposal]: the picked text, escaped, as
// a target the resolver resolves back onto them, with its area and nth, and its YAML for a zoom,
// a click, a seen expect and either end of a drag.
//
// [Run] serves a session to a browser on this machine: it listens on 127.0.0.1 only, prints the
// page's address, and serves the embedded page under page/ and a JSON API over the session —
// its state, frames as PNG, edits, undo, redo, revert, reload, save and picks — until its
// context ends. A request naming any Host but the printed address or localhost on its port is
// refused, as is a POST whose body is not JSON, so another site's page reaches neither.
//
// The file map:
//   - cache.go: the bounded, least-recently-used cache of a rasterizer's snapshots.
//   - doc.go: this package comment and file map.
//   - edit.go: the [Edit] and applying one to storyboard source, key by key.
//   - edit_camera.go: an Edit of a camera move's at, over, ease or to.
//   - edit_caption.go: an Edit of a caption's at, for, until, text, place or align.
//   - frame.go: [Session.Frame], one composed frame of the preview as a PNG.
//   - history.go: a session's storyboard versions, for undo and redo.
//   - pick.go: [Session.Pick], a resolver-checked target [Proposal] from picked cells.
//   - run.go: [Run] and its [Options]: listen on 127.0.0.1, print the address, serve until done.
//   - server.go: the HTTP handler: the embedded page, the JSON API and its Host and POST guards.
//   - session.go: the [Session]: its pipelines, history, refresh from disk and guarded save.
//   - state.go: the [State] a session reports and the errors attributed to beats.
//   - value.go: reading an edit's value spellings and writing a duration in its old unit.
package tune
