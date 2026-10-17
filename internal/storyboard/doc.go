// Package storyboard holds a clip's Storyboard — the YAML file that says what photon performs,
// how long each beat plays in the clip, where it zooms, and what a keeper take must show — and
// the strict loader that is the only way to get one. [Load] refuses an unknown field and reports
// every rule the file breaks at once, in a [ValidationError].
//
// It imports only keys, to check that a `key` action names a key photon can send. Whether an
// area a target or expect names is declared in the config is checked where the storyboard and
// the config are loaded together, not here.
//
// gopkg.in/yaml.v3 is the decoder because it can refuse unknown fields (KnownFields) and hands a
// custom scalar its yaml.Node, which is how `nth: last` decodes.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - storyboard.go: the [Storyboard] and its [Frame], [Load] and [ValidationError].
//   - beat.go: a [Beat], its [Zoom], [Action] forms, [Target] and [Nth], and [Expect].
//   - validate.go: every validation rule, each reporting its problem under the field's path.
package storyboard
