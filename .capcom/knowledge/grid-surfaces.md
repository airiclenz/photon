---
title: "Decision: one grid, a text layer and an image layer, for terminal and browser takes"
labels: [decision, architecture]
---

**Decision (2026-11-06):** photon films more than terminals. A snapshot is one grid carrying two
layers: a **text layer** that targets, `wait: screen`, `seen` and areas search, and an **image
layer** that render and tune draw. Each capture source fills both layers on the same grid.

**Why.** Downstream of the take, photon already works in grid coordinates: a target resolves to a
`take.Box`, compose turns a Box into pixels in one place, and the schedule, camera, zoom, drawn
cursor, captions, encoder and tune's timeline are blind to what a cell holds. Splitting the
snapshot into a searched layer and a drawn layer lets a web app be filmed with the same
storyboard, checked take and timing editor as a terminal app, instead of a screen recorder that
loses all three. The boundary changes every later browser issue, so it is settled once, here.

## The model

- **Snapshot** = text layer + image layer on one grid, with its time.
- **Terminal take:** the pty source derives both layers from the emulator's cells; the text layer
  is the cells' text, the image layer is the cells themselves, rasterized as today. A terminal
  take records, checks and renders exactly as before.
- **Browser take:** the image layer is screenshot frames of the page; the text layer is the
  page's layout text, recorded as **text runs** (a run of text with its Box on the grid).

## Capture sources

- **pty terminal** (`internal/terminal`): the program in a pty under the vt emulator.
- **headless Chromium**, driven over the DevTools Protocol through chromedp (planned
  `internal/browser`). The config's `launch.url` starts it (the alternative to `launch.command`,
  exactly one set; `${VAR}` in the url expands from the launch env; optional `launch.browser`
  overrides Chromium discovery; `strip` removes its globs from the browser's env as from a
  command's). The storyboard sets the page size with `frame.viewport: {width, height}` and
  `frame.device_scale`.

## Grid unit and cell geometry

On a browser take one cell is one CSS pixel. The take records its cell geometry, the pixel width
W and height H of one cell; a browser take records W = H = device scale. A terminal take's
geometry comes from its font, as today.

## Frame storage

Pixel frames live inside the take file. A take with an image layer is take format v2 (a terminal
take stays v1). Frames are PNG key frames plus patch entries holding one PNG rectangle for each
far-apart change, so typing and a spinner at opposite corners stay two small patches; a key
frame is written every 2 s, or whenever the patches together would cover more than half the
frame. Budget: a one-minute 1280×800, 24 fps browser take is at most 25 MB.

## Selectors

A target or an area can name a CSS selector instead of text (`areas.<name>.selector`, a selector
target in the storyboard). The source records, with each snapshot, the box of every selector the
clip names (storyboard targets plus config areas); check, render and tune resolve selectors from
those recorded boxes and never need the page again. Tune proposes text targets plus the
selectors the clip already names. Lint checks a selector's shape only: non-empty, balanced
brackets, quotes and parens, and on a url-launch clip only.

Text areas on a browser take: `top`/`bottom` anchors read as y-bands over text runs; `offset` is
refused on a url-launch clip.

## Edits a pixel take refuses

Anything that reflows text cannot be applied to recorded pixels: `frame.font_size`,
`frame.line_height`, `frame.cols`, `frame.rows` and `frame.padding` are refused on a take with an
image layer. On a url-launch clip a caption's `font.size` is in CSS px, scaled by device_scale ×
scale; default 16, bounds [6, 72].

## The engine's Source seam

`engine.Terminal` becomes `engine.Source`: `Type(text)`, `Key(name)`, `Pointer(event)` for input,
with `Current`, `Match`, `Done` and `Event` unchanged. The pty adapter encodes text, named keys
and SGR mouse events to bytes; the browser adapter sends them as browser input events. `engine`
imports neither adapter.

## Package ownership

- `internal/take`: both layers, text runs, recorded selector boxes and the cell geometry; it stays
  free of image code.
- `internal/pixels` (new): encoding, decoding and seeking the image layer's frames.
- `internal/browser` (planned): the Chromium source, importing `take`, `pixels` and `keys`.
- `internal/engine`: defines `Source`; never imports `terminal` or `browser`.

Projects still plug in through config and storyboards only (Note `extension-model`); the browser
is a launch kind, not a Go hook. The package rows themselves change in Note `package-layout` as
the code lands.
