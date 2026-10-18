---
title: Split the tune page's app.js below the file-size guide
status: open
type: task
priority: 3
labels: [tune]
plan: 2026-10-18 - 00 - architecture-review-plan
created: 2026-10-18T05:26:26Z
---

`internal/tune/page/app.js` is 908 lines, more than twice the ~400-line file guide in Note
`coding-conventions`. It was already over 400 lines before plan
`2026-10-17 - 05 - camera-captions-plan` and roughly doubled with items 17 and 18. It holds the
session API calls, the clip's frame clock, the transport, every inspector panel (beats, frame,
camera moves, captions) and the wiring. The drawing section alone runs about 350 lines (from
`internal/tune/page/app.js:388`). The open issues `tune-edit-pan-via`, `tune-add-remove-entries`
and `tune-caption-styling` all add inspector code, so the file keeps growing until it is split.

The `window.tune` contract at the top of `app.js` is what `lanes.js`, `timeline.js` and
`picker.js` build on. A split keeps that contract as it is and moves code into new page assets
loaded the same way (`internal/tune/page/index.html`), for example the inspector panels apart
from the API, clock and transport.

## Acceptance

- `internal/tune/page/app.js` and every page script the split adds are each under ~400 lines.
- The `window.tune` contract (names, arguments, return values) is unchanged; `lanes.js`,
  `timeline.js` and `picker.js` need no edits beyond what a renamed internal forces.
- The page behaves as before: `GOMEMLIMIT=2GiB go test ./internal/tune/` passes, with the page
  tests updated to read the new files where they grep `app.js`. A headless run of the page against
  a session still shows and edits beats, the frame, camera moves and captions.
- The `internal/tune` doc file map names each new asset.
