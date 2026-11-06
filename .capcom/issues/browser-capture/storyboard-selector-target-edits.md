---
title: Camera position edits leave a selector target behind
status: done
type: bug
priority: 3
labels: [browser]
plan: 2026-11-06 - 04 - browser-capture-plan
created: 2026-11-06T15:52:57Z
closed: 2026-11-06T20:17:34Z
reason: resolved by 2026-11-06 - 04 - browser-capture-plan (e406ff0)
---

The `position.cell` and `position.target` edits in `internal/storyboard/edit_camera.go` clear the target keys in `targetKeys` (`internal/storyboard/edit_camera.go:69`), which does not list `selector`, so editing a position that names a selector target leaves the selector beside the new cell or text and validation refuses the result. `decodeTarget` (`internal/storyboard/edit_camera.go:291`) accepts a `selector` JSON key (it is a `Target` field) and silently drops it, since it requires `text`. Found closing out plan `2026-11-06 - 03 - grid-layers-plan` (item 11).

## Acceptance

- Setting `position.cell` or `position.target` on a position whose target names a selector removes the selector key, and the edited storyboard validates.
- `decodeTarget` either writes a selector target or refuses a `selector` key; it never drops one silently.
