---
title: pageLayout cell ignores frame.scale
status: done
type: bug
priority: 3
labels: [browser]
plan: 2026-11-06 - 04 - browser-capture-plan
created: 2026-11-06T15:52:57Z
closed: 2026-11-06T20:17:34Z
reason: resolved by 2026-11-06 - 04 - browser-capture-plan (e406ff0)
---

`pageLayout` (`internal/render/check.go:63`) sets `Cell` to `device_scale` (`internal/render/check.go:70`) while `Size` includes the factor `frame.scale`, so at `frame.scale` > 1 cell boxes map to pixels at 1/scale. Lint only lays out captions today, but a browser render reusing this helper must use a cell of `device_scale` × `frame.scale`, or draw the image layer unscaled. Found closing out plan `2026-11-06 - 03 - grid-layers-plan` (item 13).

## Acceptance

- A box on a url-launch frame with `frame.scale` 2 maps to the pixel rectangle at the frame's scale, covered by a test.
