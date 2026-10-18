---
title: Test that a changed fonts dir, theme or caption font is not reused
status: done
type: task
priority: 3
labels: [render]
plan: 2026-10-18 - 01 - open-issues-plan
created: 2026-10-18T08:54:57Z
closed: 2026-10-18T12:34:25Z
reason: Resolved by plan 2026-10-18 - 01 - open-issues-plan (archived in 70f28a8)
---

[Build] reuses the previous pipeline's rasterizer and rasterized snapshots only while
`canReuseRasters` (`internal/render/pipeline.go:132`) holds, which compares the raster geometry,
`Config.Fonts` and the theme's FG, BG and ANSI colours, and reuses its caption faces only while
`captionFacesFrom` (`internal/render/pipeline.go:144`) finds the same caption font files. The reuse
tests (`TestBuild_Reuse_SharesTheRastersOnlyWhileTheyHold`, `internal/render/pipeline_test.go:225`,
and `TestBuild_TenReusedBuilds_LoadCaptionFontsOnce`, `:273`) cover a changed take and raster
geometry, but none changes the fonts dir, the theme or `caption_font`, so a reuse check that
stopped comparing one of them would pass the suite and render with stale fonts or colours. Found
closing out plan `2026-10-18 - 00 - architecture-review-plan` (item 5).

## Acceptance

- A render test builds over a previous pipeline with a changed fonts dir, and one with a changed
  theme colour, and asserts a fresh rasterizer and cache rather than the previous ones.
- A render test builds over a previous pipeline with a changed `caption_font` and asserts the
  caption faces are loaded afresh.
