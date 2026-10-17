// Package clip loads one clip: its storyboard and the project config it uses, together, so that
// lint, record, check and render all start from one loading path. [Load] finds the config (the
// `--config` flag, else the nearest photon.yaml at or above the storyboard), loads both files
// and cross-checks them. The clip's work dir and `<work>/<clip>.take` resolve on demand, so a
// command that needs neither, such as lint, does not fail when the work dir cannot be found.
// [Clip.WithStoryboard] swaps in a storyboard edited in memory, cross-checked against the same
// config.
//
// A broken storyboard and a broken config are reported together, so one lint pass shows every
// problem of both files. The cross-checks — every area the storyboard names is one the config
// declares — run only once both files load clean, since a half-loaded file has nothing reliable
// to check against.
//
// It imports only storyboard and config; it runs no command and touches no file beyond reading
// the two it loads.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - clip.go: the [Clip], [Load], [Clip.WithStoryboard], the lazy work dir and take path with
//     their precedence, the cross-checks and [ValidationError].
package clip
