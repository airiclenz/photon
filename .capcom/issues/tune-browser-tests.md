---
title: Headless browser tests for the tune page
status: done
type: task
priority: 2
labels: [tune]
plan: 2026-11-06 - 01 - tune-editing-plan
created: 2026-11-06T07:11:17Z
closed: 2026-11-06T10:52:28Z
reason: "Resolved by plan 2026-11-06 - 01 - tune-editing-plan (archived 5f51cdf): item 8 adds a chromedp browser test (load, play, scrub, edit and Save) behind the browser tag, with a ubuntu CI job."
---

The tune page's JavaScript has no automated tests in a browser. Its behaviour is checked
server-side and by manual `photon tune` sessions only.

Shelved as out of scope by `docs/plans/archived/2026-10-17 - 04 - timing-editor-plan.md`.
The Pi host's memory limits apply to whatever browser runs them.

## Acceptance

- A headless browser test drives the page against a real `photon tune` server: load, play,
  scrub, and one edit written back.
- The tests run in CI and can be run locally under the Pi host's limits.
