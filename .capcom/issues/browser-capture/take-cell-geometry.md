---
title: Record the grid's cell geometry in the take
status: open
type: task
priority: 3
labels: [browser, take, render]
deps: [grid-surfaces-decision]
created: 2026-11-06T13:42:51Z
---

`compose.Layout.boxRect` turns a Box into pixels with the rasterizer's cell width and line
height. Make the grid-to-pixel geometry a property of the take's grid instead: from the
font metrics for a terminal take, one device pixel per cell (or a device-pixel scale) for a
browser take. compose reads it from there and stops depending on `raster` for it.

## Why

A browser take has no font metrics; without this, every camera move, zoom and caption on one
would be placed by a terminal's cell size.

## Acceptance

- compose gets cell geometry from the take's grid, not from the rasterizer.
- Terminal renders are byte-identical to before.
- A test places a camera box on a grid with 1×1 cells and gets the matching pixel rectangle.
