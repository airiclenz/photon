// Package tune is the timing editor behind `photon tune`: it previews a storyboard's section
// timing, zoom and frame against an existing take, and saves the edits back into the storyboard.
//
// An [Edit] is one typed change — a beat's duration, hold, cut or zoom, the clip's frame.width
// or frame.fps, a camera move's start, length, ease or destination, or a caption's start, end,
// text, place or align — spelled as the page sends it, and [storyboard.Apply] applies it: it
// splices only the edited scalar, or the one node a moment, a pan destination or a caption's text replaces,
// into the storyboard's source through yamledit, so comments, quoting, key order and flow or block style
// stay as the person wrote them: a duration keeps its unit, a new key goes in the storyboard's
// field order, a default is never written out, switching cut off, the zoom factor to auto, a
// camera move's ease to smooth or a caption's place or align to its default removes the key, and
// a caption's for and until replace each other. The result is parsed again with the storyboard's
// own strict loader, so an edit that leaves the storyboard invalid is refused with the problems
// it causes.
//
// A [Session] holds one clip and take being tuned. Every applied edit, undo, redo or revert
// rebuilds the render pipeline on the new bytes, as `photon render` builds it, handing render the
// last good pipeline to reuse: its rasterizer and rasterized snapshots while the take and the
// frame's raster geometry stay, and its caption faces, so the caption fonts load once a session; a
// build that fails leaves each of its errors standing on the beat, camera move or caption it is
// located on ([storyboard.OwnerOf]), or on the clip, and the last good pipeline stays the
// preview's. [Session.Refresh] takes in outside writes — a rewritten take is read again, a changed
// storyboard is loaded when nothing is unsaved and turns the session stale otherwise — and
// [Session.Save] writes in place, refused while stale ([ErrStale]) or while an error stands
// ([ErrInvalid]). [Session.State] reports everything the page shows, and [Session.Frame] the PNG of
// the frame `photon render` composes at an output time, from the last good pipeline, its rasterized
// snapshots kept in a cache bounded at 1 GiB that a new take or raster geometry starts afresh.
// [Session.Pick] turns cells picked on that frame into a [Proposal]: the picked text, escaped, as a
// target the resolver resolves back onto them, with its area and nth, and its YAML for a zoom, a
// click, a seen expect and either end of a drag. [Session.Path] maps a via pan's points onto that
// frame, through its camera's crop at the output time.
//
// [Run] serves a session to a browser on this machine: it listens on 127.0.0.1 only, prints the
// page's address, and serves the embedded page under page/ and a JSON API over the session —
// its state, frames as PNG, a via pan's path on a frame, edits, undo, redo, revert, reload, save
// and picks — until its context ends. A request naming any Host but the printed address or
// localhost on its port is refused, as is a POST whose body is not JSON, so another site's page
// reaches neither.
//
// The file map:
//   - cache.go: the bounded, least-recently-used cache of a take's rasterized snapshots.
//   - doc.go: this package comment and file map.
//   - edit.go: the [Edit], the storyboard.Edit the page sends and storyboard.Apply applies.
//   - frame.go: [Session.Frame], one composed frame of the preview as a PNG.
//   - history.go: a session's storyboard versions, for undo and redo.
//   - pick.go: [Session.Pick], a resolver-checked target [Proposal] from picked cells.
//   - run.go: [Run] and its [Options]: listen on 127.0.0.1, print the address, serve until done.
//   - server.go: the HTTP handler: the embedded page, the JSON API and its Host and POST guards.
//   - session.go: the [Session]: its pipelines, history, refresh from disk, guarded save and a via
//     pan's path.
//   - state.go: the [State] a session reports and the errors attributed to their owners.
//   - page/index.html, page/style.css: the page and its styles.
//   - page/app.js: the window.tune contract, the session's API, the selection and the wiring.
//   - page/transport.js: the frame clock, the transport and its keys.
//   - page/controls.js: the inspector's field controls and its error and refusal rows.
//   - page/panels.js: the inspector's beat and frame panels.
//   - page/entries.js: the inspector's camera move and caption panels.
//   - page/lanes.js, page/timeline.js, page/picker.js: the timeline, its lanes and the target picker.
package tune
