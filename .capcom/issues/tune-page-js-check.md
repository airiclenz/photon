---
title: Check the tune page scripts run
status: open
type: task
priority: 2
labels: [tune]
plan: 2026-10-19 - 02 - tune-panel-problems-plan
created: 2026-10-18T09:15:40Z
---

Plan `2026-10-18 - 00 - architecture-review-plan` split `internal/tune/page/app.js` into five
scripts (item 10), and then rewrote how the page sends edits (items 18-20) and builds its controls
from `state.fields` (item 24). This host has no JavaScript engine, so no script was parsed or run.
The only checks were the Go page tests (`internal/tune/page_test.go`), which grep the sources. A
syntax error or an undefined name in any script would leave `photon tune` showing a blank or
broken inspector, and the suite would still pass.

## Acceptance

- Every script under `internal/tune/page/` has been parsed by a JavaScript engine (for example
  `node --check`), either in CI or in a test that skips with a clear message when none is
  installed.
- A manual `photon tune` session on a sample storyboard has edited a beat, a frame field, a camera
  move and a caption, and each edit's problem showed on its owning panel. Its outcome is logged on
  this issue.
