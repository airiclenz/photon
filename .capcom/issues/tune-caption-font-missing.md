---
title: Caption font controls missing in the tune inspector
status: done
type: bug
priority: 2
labels: [tune]
plan: 2026-10-18 - 02 - tune-polish-plan
created: 2026-10-18T14:38:11Z
closed: 2026-10-18T16:38:38Z
reason: Resolved by plan 2026-10-18 - 02 - tune-polish-plan (1cba598)
---

In a `photon tune` session on 2026-10-18 (macOS), a selected caption's inspector showed no font
control. On `main` the caption panel builds `font size` and `weight` rows
(`internal/tune/page/entries.js`, `styleRows`, called from `captionPanel`). The session first ran
an older build, so check this on a binary built from current `main` before changing anything.

## Acceptance

- On a binary built from current `main`, selecting a caption shows its font size and weight
  controls, and changing them re-renders the preview. Either confirm that and close this as fixed
  by the older-build finding, or find and fix why the rows are missing.
- If there was a real bug, a page test (`internal/tune/pagejs_test.go`) selects a caption and asserts
  the font size and weight controls are there.
