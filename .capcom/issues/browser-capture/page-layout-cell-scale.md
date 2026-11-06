---
title: pageLayout cell ignores frame.scale
status: open
type: bug
priority: 3
labels: [browser]
created: 2026-11-06T15:52:57Z
---

`pageLayout` (`internal/render/check.go:63`) sets `Cell` to `device_scale` (`internal/render/check.go:70`) while `Size` includes the factor `frame.scale`, so at `frame.scale` > 1 cell boxes map to pixels at 1/scale. Lint only lays out captions today, but a browser render reusing this helper must use a cell of `device_scale` × `frame.scale`, or draw the image layer unscaled. Found closing out plan `2026-11-06 - 03 - grid-layers-plan` (item 13).

## Acceptance

- A box on a url-launch frame with `frame.scale` 2 maps to the pixel rectangle at the frame's scale, covered by a test.
