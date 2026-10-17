// Package tune is the timing editor behind `photon tune`: it previews a storyboard's section
// timing, zoom and frame against an existing take, and saves the edits back into the storyboard.
//
// An [Edit] is one typed change — a beat's duration, hold, cut or zoom, or the clip's
// frame.width or frame.fps — spelled as the page sends it. Applying one splices only the edited
// scalar into the storyboard's source through yamledit, so comments, quoting, key order and flow
// or block style stay as the person wrote them: a duration keeps its unit, a new key goes in the
// storyboard's field order, a default is never written out, and switching cut off or the zoom
// factor to auto removes the key. The result is parsed again with the storyboard's own strict
// loader, so an edit that leaves the storyboard invalid is refused with the problems it causes.
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
// The file map:
//   - cache.go: the bounded, least-recently-used cache of a rasterizer's snapshots.
//   - doc.go: this package comment and file map.
//   - edit.go: the [Edit] and applying one to storyboard source, key by key.
//   - frame.go: [Session.Frame], one composed frame of the preview as a PNG.
//   - history.go: a session's storyboard versions, for undo and redo.
//   - pick.go: [Session.Pick], a resolver-checked target [Proposal] from picked cells.
//   - session.go: the [Session]: its pipelines, history, refresh from disk and guarded save.
//   - state.go: the [State] a session reports and the errors attributed to beats.
//   - value.go: reading an edit's value spellings and writing a duration in its old unit.
package tune
