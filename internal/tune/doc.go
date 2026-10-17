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
// The file map:
//   - doc.go: this package comment and file map.
//   - edit.go: the [Edit] and applying one to storyboard source, key by key.
//   - value.go: reading an edit's value spellings and writing a duration in its old unit.
package tune
