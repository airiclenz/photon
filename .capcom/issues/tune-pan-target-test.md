---
title: Test setting a pick as the pan target
status: open
type: task
priority: 3
labels: [tune]
plan: 2026-10-18 - 01 - open-issues-plan
created: 2026-10-17T22:00:33Z
---

The tune page's "Set as pan target" button (`internal/tune/page/picker.js:214`,
`setAsPanTarget`, wired at `internal/tune/page/picker.js:287`) sets a live pick proposal as the
selected pan move's `to`, but no test exercises that click with a pick proposal standing. Found
closing out plan `2026-10-17 - 05 - camera-captions-plan` (item 18).

## Acceptance

- A test drives a pick to a proposal, selects a pan move and clicks "Set as pan target", and
  asserts the edit the page sends (`camera[i]` `to` set to the proposal) and the report it shows.
