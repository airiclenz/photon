---
title: Film web apps as well as terminal apps, on one grid model
status: open
type: feature
priority: 3
labels: [browser]
deps: [film-tune-page]
created: 2026-11-06T13:42:51Z
---

Groups the work that lets photon film a web app in a browser, not only a terminal app in a pty.

A terminal snapshot and a browser frame are both grids. Downstream of the take, photon
already works in grid coordinates: a target resolves to a `take.Box`, and compose turns a Box
into pixels in one place (`compose.Layout.boxRect`, from the cell width and line height).
`schedule`, the camera, zoom, the drawn cursor, captions, the encoder and tune's timeline are
blind to what a cell holds. So the plan is a snapshot with two layers on one grid: a text
layer (text with its Box) that targets, `wait: screen` and `seen` search, and an image layer
that is drawn. A terminal derives both from its cells; a browser takes the image from a
screenshot and the text from the page's layout.

It closes when its children are done; it waits on the last of them, `film-tune-page`.

The children land in dependency order: the decision first, then refactors that change no
terminal behaviour, then the browser source, then tune and a first clip.

## Why

photon cannot film its own `photon tune` page, or any web UI. Today the project has to reach
for a screen recorder and loses the storyboard, the checked take and the timing editor.
