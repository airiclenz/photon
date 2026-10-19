---
title: Check the tune page scripts run
status: done
type: task
priority: 2
labels: [tune]
plan: 2026-10-19 - 02 - tune-panel-problems-plan
created: 2026-10-18T09:15:40Z
closed: 2026-10-19T18:31:38Z
reason: "Resolved by plan 2026-10-19 - 02 - tune-panel-problems-plan (0529867): goja harness runs every page script; TestPageJS_EditProblem_ShowsOnItsOwningPanel checks each edit's problem on its owning panel"
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
- The goja page test `TestPageJS_EditProblem_ShowsOnItsOwningPanel` (`internal/tune/pagejs_test.go`)
  sends an edit through the inspector that leaves a problem on a beat, the clip's frame, a camera
  move and a caption, and passes: each problem shows as an error row on its owning panel and not
  on another owner's panel. No manual `photon tune` session is needed.
