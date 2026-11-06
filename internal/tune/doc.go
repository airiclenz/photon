// Package tune is the timing editor behind `photon tune`: it previews a storyboard's section
// timing, camera and frame against an existing take, and saves the edits back into the
// storyboard.
//
// An [Edit] is one typed change — a beat's duration, hold or cut, the clip's frame.width,
// frame.fps, frame.font_size, frame.line_height, frame.padding or frame.scale, a camera
// position's or zoom's timing, ease, aim or factor, or a caption's start, end, text, place or
// align — spelled as the page sends it, and [storyboard.Apply] applies
// it: it splices only the edited scalar, or the one node a moment or a caption's text replaces,
// into the storyboard's source through yamledit, so comments, quoting, key order and flow or block
// style stay as the person wrote them: a duration keeps its unit, a new key goes in the
// storyboard's field order, a default is never written out, switching cut off or a caption's place
// or align to its default removes the key, and a caption's for and until replace each other. The
// result is parsed again with the storyboard's own strict loader, so an edit that leaves the
// storyboard invalid is refused with the problems it causes.
//
// A [Session] holds one clip and take being tuned. Every applied edit, undo, redo or revert
// rebuilds the render pipeline on the new bytes, as `photon render` builds it, handing render the
// last good pipeline to reuse: its rasterizer and rasterized snapshots while the take and the
// frame's raster geometry stay, and its caption faces, so the caption fonts load once a session; a
// build that fails leaves each of its errors standing on the beat, camera position, zoom or
// caption it is located on ([storyboard.OwnerOf]), or on the clip, and the last good pipeline
// stays the preview's. A take recorded at another cols×rows than the storyboard's frame
// ([render.ErrTakeSize]) builds no less: the pipeline is built at the take's size, so the preview
// keeps drawing its cells, and a clip problem naming both sizes stands, telling the user to
// re-record the take with `photon record`.
// [Session.Refresh] takes in outside writes — a rewritten take is read again, a changed storyboard
// is loaded when nothing is unsaved and turns the session stale otherwise — and [Session.Save]
// writes in place, refused while stale ([ErrStale]) or while an error other than the take's size
// stands ([ErrInvalid]); `photon render`, and so [Session.Export], still refuses the mismatch.
// [Session.State] reports everything the page shows — the beats, camera positions and zooms,
// captions and schedule — and [Session.Frame] the PNG of the exact frame at an output time: the
// frame `photon render` builds, quantized to frame.max_colors colours by [render.Quantize] as
// the preview's stand-in for the GIF's palette, from the last good pipeline, its rasterized
// snapshots kept in a cache bounded at 1 GiB that a new take or raster geometry starts afresh;
// [Session.DraftFrame] the PNG of the draft frame the page shows while it plays or scrubs, drawn
// from a raster at the shipped size with a fast scaler and sent uncompressed, its draft rasters
// kept in a draft cache bounded at 256 MiB that carries across rebuilds as the exact one does; once
// stopped the page settles on the exact frame. Draft frames are never quantized.
// [Session.FullFrame] is the PNG of the full view, the whole frame at the exact frame's size
// whatever the camera's zoom and aim and quantized as the exact frame is, and [Session.Camera]
// where the camera looks over it at an output time — its crop, the cell it centres on and the
// grid's place — and the cell a camera position aims at. [Session.Pick] turns cells picked on the
// camera's frame or on the full view into a [Proposal]: the picked text, escaped, as a target the
// resolver resolves back onto them, with its area and nth, and its YAML for the camera, a click, a
// seen expect and either end of a drag. [Session.Export] renders the GIF of the saved storyboard in
// the background, as `photon render` renders it, refused while the storyboard changed on disk, has
// unsaved edits or an error, or another export runs; the state reports its progress and outcome.
//
// [Run] serves a session to a browser on this machine: it listens on 127.0.0.1 only, prints the
// page's address, and serves the embedded page under page/ and a JSON API over the session — its
// state, exact, draft and full-view frames as PNG, the camera's view, edits, undo, redo, revert,
// reload, save, picks and the GIF export — until its context ends, then waits for an export still
// rendering. A request naming any Host but the printed address or localhost on its port is
// refused, as is a POST whose body is not JSON, so another site's page reaches neither.
//
// The file map:
//   - cache.go: the bounded, least-recently-used cache of a take's rasterized snapshots, sized for
//     the exact frames' 1 GiB raster cache and [Session.DraftFrame]'s 256 MiB draft cache.
//   - doc.go: this package comment and file map.
//   - edit.go: the [Edit], the storyboard.Edit the page sends and storyboard.Apply applies.
//   - export.go: [Session.Export], the background render of the saved storyboard's GIF, and its
//     [ExportState].
//   - frame.go: [Session.Frame], [Session.DraftFrame], [Session.FullFrame] and
//     [Session.FullDraftFrame], one exact, draft, full-view or full-view draft frame of the
//     preview as a PNG, and [Session.Camera], the camera's [CameraView].
//   - history.go: a session's storyboard versions, for undo and redo.
//   - pick.go: [Session.Pick], a resolver-checked target [Proposal] from picked cells.
//   - run.go: [Run] and its [Options]: listen on 127.0.0.1, print the address, serve until done.
//   - server.go: the HTTP handler: the embedded page, the JSON API and its Host and POST guards.
//   - session.go: the [Session]: its pipelines, history, refresh from disk and guarded save.
//   - state.go: the [State] a session reports and the errors attributed to their owners.
//   - page/index.html, page/style.css: the page and its styles.
//   - page/app.js: the window.tune contract, the session's API, the selection and the wiring.
//   - page/kinds.js: the inspector's entry kinds — positions, transitions, zooms, captions —
//     and their lookups.
//   - page/fit.js: the preview shrunk to fit a short window.
//   - page/frames.js: the preview's frames at the playhead: drafts while it moves, then exact.
//   - page/transport.js: the frame clock, the transport and its keys.
//   - page/export.js: the transport bar's Export GIF button, its progress and outcome.
//   - page/controls.js: the inspector's field controls and its error and refusal rows.
//   - page/panels.js: the inspector's beat and frame panels.
//   - page/entries.js: the inspector's position, transition, zoom and caption panels.
//   - page/lanes.js, page/timeline.js, page/picker.js: the timeline, its lanes and the target
//     picker.
//   - page/drag.js: the timeline's pointer drag, snapped, sending one edit when let go.
//   - page/marker.js: the selected position's full view, crop and aim marker, dragged to aim it.
package tune
