// Package storyboard holds a clip's Storyboard — the YAML file that says what photon performs,
// how long each beat plays in the clip, where its camera aims and how far it zooms, which captions
// it shows, and what a keeper take must show — and the strict loaders that are the only ways to
// get one. [Load] reads a file and [Parse] takes its bytes; both refuse an unknown field and
// report every rule the file breaks at once, in a [ValidationError]. A problem that belongs to one
// beat, camera position, camera zoom or caption is a [LocatedError], which [OwnerOf] reads its
// owner from.
//
// Parse is the static tier of the three a storyboard is checked in: every rule the file decides
// on its own. The font-bound tier lays the captions out with the clip's real fonts (package
// render's CheckCaptions, which `photon lint` runs), and the take-bound tier resolves the camera
// positions' text targets on a take's screens (package compose's New).
//
// [Apply] changes one field by a typed [Edit] — a beat's duration, hold or cut, the clip's
// frame.padding, frame.font_size, frame.line_height, frame.scale, frame.width or frame.fps, a
// camera position's start, hold — or both at once — ease, cell or text target, a camera zoom's
// start, length, ease or factor, or a caption's start, end — or both at once, shifted — text,
// place, align, font, colours, box or fade, or adds, removes or reorders a caption — splicing the
// new value into the source so only the edited key's text changes, a new key placed by the order
// of the storyboard's yaml struct tags, and refuses an edit whose result fails Parse. A value
// equal to what an absent key means — a position's hold of 0, an ease of smooth — deletes its
// key, and a position's cell replaces its text, nth and area, and its target its cell.
// [Fields] describes every field of a storyboard's owners as photon tune's page shows them: each
// editable field an Edit names and the frame's read-only fields, with its [FieldKind], an enum's
// choices and the default an absent key means.
//
// It imports keys, to check that a `key` action names a key photon can send, and yamledit, to
// splice an edit into the source. Whether an area a target or expect names is declared in the
// config is checked where the storyboard and the config are loaded together, not here.
//
// gopkg.in/yaml.v3 is the decoder because it can refuse unknown fields (KnownFields) and hands a
// custom scalar its yaml.Node, which is how `nth: last` decodes and how a [Moment] takes either
// a duration or a `{beat, offset}` mapping.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - storyboard.go: the [Storyboard] and its [Frame], [Load], [Parse] and [ValidationError].
//   - beat.go: a [Beat], its [Action] forms, [Target] and [Nth], and [Expect].
//   - camera.go: the [Camera] of positions and zooms, a [ZoomMove], a [Point], [Cell] and [Ease],
//     and the rules that check and place them.
//   - position.go: a camera [Position], its point, hold and ease, and its rules.
//   - edit.go: an [Edit], [Apply] and the splicing every edit shares: field and owner checks,
//     durations, moments, key order from yaml tags.
//   - edit_beat.go: the beat and frame edits — duration, hold, cut, frame.padding,
//     frame.font_size, frame.line_height, frame.scale, frame.width and frame.fps — and
//     [Target.YAMLFlow].
//   - edit_camera.go: the camera position and zoom edits — position.at, .hold, .span, .ease,
//     .cell and .target, and zoom.at, .over, .ease and .to.
//   - edit_caption.go: the caption edits — caption.at, caption.for, caption.until,
//     caption.shift, caption.text, caption.place and caption.align.
//   - edit_caption_style.go: the caption style edits — caption.font.size and .weight,
//     caption.color, caption.box.color and .opacity, caption.fade.in and .out — and caption.offset
//     and caption.position.
//   - edit_list.go: the list edits that add, remove and reorder captions —
//     caption.insert|remove|move.
//   - fields.go: [Fields], each owner's editable and read-only [Field] with its [FieldKind],
//     choices and default.
//   - clock.go: the clip's [OutputClock], from [Storyboard.Clock]: section starts, moments and
//     length.
//   - caption.go: a [Caption], its [CaptionFont] and [Weight], [Place], [Align], [Pixels],
//     [CaptionBox], [Fade] and [RGB], and their rules.
//   - located.go: a problem's [Owner] and [OwnerKind], the [LocatedError] that carries one,
//     [AtBeat], [AtPosition], [AtZoom], [AtCaption] and [OwnerOf].
//   - moment.go: a [Moment] on the clip's output clock and how it decodes.
//   - validate.go: every other validation rule, each reporting its problem under the field's path.
package storyboard
