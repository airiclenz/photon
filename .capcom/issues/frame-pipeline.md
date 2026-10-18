---
title: Deepen render into one frame pipeline
status: open
type: task
priority: 2
labels: [architecture]
plan: 2026-10-18 - 00 - architecture-review-plan
created: 2026-10-18T05:46:01Z
---

`internal/render` exports `NewRasterizer`, `NewCompositor`, `SnapshotAt` and `BeatReport` only so
`internal/tune` can rebuild the clip → schedule → raster → compose chain itself. The chain is wired
three times (render's `newPlan`, tune's `Session.build`, and the `renderedFrame` oracle in
`internal/tune/frame_test.go`). Tune must obey an ordering rule ("the rasterizer must be the one
NewRasterizer built for the same clip"), copies render's frame-size error text word for word, and
reloads the caption fonts on every edit. Architecture review 2026-10-18, finding 1.

Wanted: one frame pipeline module inside `internal/render` that both the GIF encode loop and tune
ask for frames, screens and cells by output time, with the raster cache as its seam (reuse-last for
render, LRU for tune).

## Acceptance

- `internal/render` exports a pipeline builder and no longer exports `NewRasterizer`,
  `NewCompositor` or `SnapshotAt`; tune builds frames only through the pipeline.
- The frame-size error text exists once in the tree.
- A test proves tune's frames equal render's frames by construction, without a copied oracle.
- Caption fonts load once per tune session, not per edit.
