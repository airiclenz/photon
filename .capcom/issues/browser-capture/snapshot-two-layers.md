---
title: Split a snapshot into a text layer and an image layer
status: open
type: task
priority: 3
labels: [browser, take]
deps: [grid-surfaces-decision]
created: 2026-11-06T13:42:51Z
---

A `take.Snapshot` is today a grid of cells, and everything that reads it reads cells. Give it
two layers: a text layer (text runs with their Box) and an image layer. For a terminal
snapshot the text layer is the cells and the image layer is what `raster` draws from them,
so nothing a terminal take records or renders changes.

## Why

It is the seam a browser source plugs into. Done first and alone, it is checked by the
existing terminal tests before any browser code exists.

## Acceptance

- Readers that search text (`target`, `check`, the engine's waits) read the text layer; readers that draw (`render`, `compose`, tune's frames) read the image layer.
- A terminal take records, checks and renders byte-identically to before; the existing take, raster golden and render tests pass unchanged.
- Takes written before the change still load.
