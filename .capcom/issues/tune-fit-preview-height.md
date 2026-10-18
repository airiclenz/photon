---
title: Shrink the tune preview to fit a short window
status: open
type: task
priority: 2
labels: [tune]
created: 2026-10-18T14:21:41Z
---

The `photon tune` page lays out the preview, the inspector, then the transport bar and timeline
under them (`internal/tune/page/style.css`). The preview's size comes from the window's width, so
in a short window the timeline is pushed below the fold and you have to scroll to reach it while
watching the preview.

## Acceptance

- When the window is too short for the preview at its width-fitted size plus the transport bar
  and timeline, the preview shrinks (keeping its aspect ratio) so the whole timeline stays on
  screen without scrolling.
- Resizing the window re-fits the preview, and a tall window still shows it at its full
  width-fitted size.
- Overlays drawn on the preview (pick box, via path) stay aligned at every size.
