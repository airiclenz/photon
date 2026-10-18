// Package storyboard holds a clip's Storyboard — the YAML file that says what photon performs,
// how long each beat plays in the clip, where it zooms, how its camera moves, which captions it
// shows, and what a keeper take must show — and the strict loaders that are the only ways to get
// one. [Load] reads a file and [Parse] takes its bytes; both refuse an unknown field and report
// every rule the file breaks at once, in a [ValidationError]. A problem that belongs to one beat,
// camera move or caption is a [LocatedError], which [OwnerOf] reads its owner from.
//
// It imports only keys, to check that a `key` action names a key photon can send. Whether an
// area a target or expect names is declared in the config is checked where the storyboard and
// the config are loaded together, not here.
//
// gopkg.in/yaml.v3 is the decoder because it can refuse unknown fields (KnownFields) and hands a
// custom scalar its yaml.Node, which is how `nth: last` decodes and how a [Moment] takes either
// a duration or a `{beat, offset}` mapping.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - storyboard.go: the [Storyboard] and its [Frame], [Load], [Parse] and [ValidationError].
//   - beat.go: a [Beat], its [Zoom], [Action] forms, [Target] and [Nth], and [Expect].
//   - camera.go: a [CameraMove] — a [ZoomMove] or [PanMove] — its [Point], [Cell] and [Ease], and their rules.
//   - clock.go: the clip's [OutputClock], from [Storyboard.Clock]: section starts, moments and length.
//   - caption.go: a [Caption], its [CaptionFont] and [Weight], [Place], [Align], [Pixels], [CaptionBox] and [Fade], and their rules.
//   - located.go: a problem's [Owner] and [OwnerKind], the [LocatedError] that carries one, [AtBeat], [AtCamera], [AtCaption] and [OwnerOf].
//   - moment.go: a [Moment] on the clip's output clock and how it decodes.
//   - validate.go: every other validation rule, each reporting its problem under the field's path.
package storyboard
