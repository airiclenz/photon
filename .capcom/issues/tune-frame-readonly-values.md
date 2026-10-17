---
title: Show the read-only frame values in the tune inspector
status: open
type: bug
priority: 3
created: 2026-10-17T19:46:08Z
---

The `photon tune` inspector's "Clip frame" panel names `cols`, `rows`, `padding`, `font_size`,
`line_height`, `scale` and `max_colors` as read-only but cannot show their values, because the
session's `State` does not report them.

- `internal/tune/state.go:18`: `State` carries `fps` and `width` from the board's frame, and none
  of the other frame fields.
- `internal/tune/page/app.js:39` and `internal/tune/page/app.js:427`: the page prints only the
  field names (`READ_ONLY_FRAME_FIELDS`) with no values.

## Acceptance

- `State` reports the frame's `cols`, `rows`, `padding`, `font_size`, `line_height`, `scale` and
  `max_colors`, covered by a session test.
- The inspector's Clip frame panel shows each of them with its value, read-only.
