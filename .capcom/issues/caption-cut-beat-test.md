---
title: Test a caption anchored to a cut beat
status: done
type: task
priority: 3
labels: [storyboard]
plan: 2026-10-18 - 01 - open-issues-plan
created: 2026-10-17T22:00:33Z
closed: 2026-10-18T12:34:25Z
reason: Resolved by plan 2026-10-18 - 01 - open-issues-plan (archived in 70f28a8)
---

A caption whose `at` or `until` is a beat moment naming a cut beat is refused through
`Storyboard.ClipTime` (`internal/storyboard/moment.go:73`, called from
`internal/storyboard/caption.go:192` and `:208`), but no storyboard caption test covers it:
`TestLoadRejects_Captions` (`internal/storyboard/storyboard_test.go:299`) has no case anchored to
a cut beat. Found closing out plan `2026-10-17 - 05 - camera-captions-plan` (item 10).

## Acceptance

- `TestLoadRejects_Captions` gains cases for a caption whose `at`, and one whose `until`, names a
  cut beat, each asserting the problem the loader reports.

## Log

- 2026-10-18 Airic Lenz: body is stale: Storyboard.ClipTime was removed by plan 2026-10-18 - 00 - architecture-review-plan (item 8); a cut-beat moment is now refused by OutputClock.Resolve (internal/storyboard/clock.go:59), called from Caption.validateTiming (internal/storyboard/caption.go:226 and :242)
