---
title: Wrap long caption text
status: done
type: task
priority: 2
labels: [render]
plan: 2026-11-06 - 00 - authoring-loop-plan
created: 2026-11-06T07:11:17Z
closed: 2026-11-06T09:25:16Z
reason: Resolved by plan 2026-11-06 - 00 - authoring-loop-plan (archived at 8e4aeec)
---

A caption's text is drawn on one line per source line. Long text runs past the box instead of
wrapping.

Shelved as out of scope by `docs/plans/archived/2026-10-17 - 05 - camera-captions-plan.md`.

## Acceptance

- Caption text wraps at word boundaries to fit the frame (or a configured width).
- Placement and alignment still apply to the wrapped block.
- Render goldens cover a wrapped caption.
