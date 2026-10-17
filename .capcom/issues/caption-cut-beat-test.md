---
title: Test a caption anchored to a cut beat
status: open
type: task
priority: 3
labels: [storyboard]
created: 2026-10-17T22:00:33Z
---

A caption whose `at` or `until` is a beat moment naming a cut beat is refused through
`Storyboard.ClipTime` (`internal/storyboard/moment.go:73`, called from
`internal/storyboard/caption.go:192` and `:208`), but no storyboard caption test covers it:
`TestLoadRejects_Captions` (`internal/storyboard/storyboard_test.go:299`) has no case anchored to
a cut beat. Found closing out plan `2026-10-17 - 05 - camera-captions-plan` (item 10).

## Acceptance

- `TestLoadRejects_Captions` gains cases for a caption whose `at`, and one whose `until`, names a
  cut beat, each asserting the problem the loader reports.
