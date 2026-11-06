---
title: Re-render on change with render --watch
status: done
type: task
priority: 2
labels: [render]
plan: 2026-11-06 - 00 - authoring-loop-plan
created: 2026-11-06T07:11:17Z
closed: 2026-11-06T09:25:15Z
reason: Resolved by plan 2026-11-06 - 00 - authoring-loop-plan (archived at 8e4aeec)
---

`photon render` runs once. A `--watch` flag that re-renders when the storyboard, config or take
changes would shorten the edit-and-look loop for authors who do not use `photon tune`.

Shelved as out of scope by `docs/plans/archived/2026-10-17 - 04 - timing-editor-plan.md`.

## Acceptance

- `photon render --watch` renders once, then again each time an input file changes.
- A failed render prints its error and keeps watching.
- Ctrl+C stops it cleanly.
