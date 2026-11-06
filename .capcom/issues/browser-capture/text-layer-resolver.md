---
title: Resolve targets and areas on the text layer
status: open
type: task
priority: 3
labels: [browser, target]
deps: [snapshot-two-layers]
created: 2026-11-06T13:42:51Z
---

`internal/target` matches a target's regex over the cells' rows and finds areas by row
anchors. Move it onto the text layer: a regex over text runs that returns the run's Box.
Areas become a Box too: found by row anchors on a terminal text layer, and later by a
selector or text match on a browser one.

## Why

Clicks, drags, camera positions, `wait: screen` and `seen` all go through the resolver; once
it reads the text layer they work on any source that fills it.

## Acceptance

- The resolver takes a text layer and returns a Box; it no longer reads cells.
- Every existing target, area and pick test passes unchanged on terminal takes.
- A test resolves a target on a hand-built text layer whose runs do not sit on character cells.
