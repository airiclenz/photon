---
title: Export the GIF from the tune page
status: open
type: task
priority: 2
labels: [tune]
plan: 2026-11-06 - 00 - authoring-loop-plan
created: 2026-11-06T07:11:17Z
---

After tuning, the author has to leave the page and run `photon render` to get the shipped GIF.
A button on the tune page that renders the GIF would close the loop.

Shelved as out of scope by `docs/plans/archived/2026-10-17 - 04 - timing-editor-plan.md`.

## Acceptance

- The tune page can render the clip to its configured output path.
- The result is byte-identical to `photon render` on the same storyboard and take.
- The page shows progress and reports a failed render without losing unsaved edits.
