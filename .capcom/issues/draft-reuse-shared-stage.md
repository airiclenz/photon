---
title: Share the exact stage on a draft-reusing rebuild
status: done
type: bug
priority: 3
labels: [render]
plan: 2026-10-19 - 01 - draft-reuse-shared-stage-plan
created: 2026-10-19T15:04:27Z
closed: 2026-10-19T17:47:22Z
reason: Resolved by plan 2026-10-19 - 01 - draft-reuse-shared-stage-plan (archived e22cd88)
---

Deferred from plan `2026-10-19 - 00 - draft-frame-speed-plan` (item 1).

A pipeline with its own draft stage, rebuilt into one whose draft geometry equals its new raster
geometry (for example `narrowedStoryboard` rebuilt as `rescaledStoryboard(1)` in
`internal/render/pipeline_draft_test.go:157`), keeps the previous pipeline's draft rasterizer and
draft cache instead of sharing the new exact stage. `draftRastersFrom`
(`internal/render/pipeline.go:215`) asks `canReuseDraftRasters` (`internal/render/pipeline.go:225`,
defined at `:258`) before it checks whether the draft geometry is the raster geometry
(`internal/render/pipeline.go:228`), and the previous draft stage passes that check because its
geometry matches.

Rendering is correct, but on that path the claim that the draft stage is the exact stage
(`internal/render/doc.go:28`, `internal/render/pipeline.go:75`) is false, and each snapshot is
rasterized twice: once into the exact cache and once into the draft cache.

## Acceptance

- A rebuild whose new draft geometry equals its new raster geometry uses the new exact
  rasterizer and cache as its draft stage, whatever draft stage the reused pipeline had.
- A case in `internal/render/pipeline_draft_test.go` rebuilds a pipeline with its own draft stage
  (such as `narrowedStoryboard`) into `rescaledStoryboard(1)` and asserts the draft stage is the
  exact stage.
