---
title: Render and tune a browser take
status: open
type: task
priority: 3
labels: [browser, render, tune]
deps: [browser-capture-source]
plan: 2026-11-06 - 04 - browser-capture-plan
created: 2026-11-06T13:42:51Z
---

Render a browser take: the image layer passes through to compose (camera, zoom, cursor,
captions) and the encoder, with no rasterizing. In `photon tune`, timing, camera, zoom,
caption, `max_colors` and scale edits work; edits that reflow text (`font_size`,
`line_height`, `cols`, `rows`, `padding`) are refused on a browser take with a message that
says why. The target picker proposes text and selector targets from the picked pixels.

## Why

A recorded browser take is only useful once it renders to a GIF and its timing can be tuned
like a terminal take's.

## Acceptance

- `photon render` turns a browser take into a GIF with camera moves, a zoom and a caption.
- `photon tune` previews and saves timing edits on a browser take, and refuses a reflow edit with a located problem.
- Terminal render and tune behaviour is unchanged.
