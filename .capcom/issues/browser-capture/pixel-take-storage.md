---
title: Store a browser take's image frames compactly
status: done
type: task
priority: 3
labels: [browser, take]
deps: [snapshot-two-layers]
plan: 2026-11-06 - 04 - browser-capture-plan
created: 2026-11-06T13:42:51Z
closed: 2026-11-06T20:17:34Z
reason: resolved by 2026-11-06 - 04 - browser-capture-plan (e406ff0)
---

A terminal snapshot is a few kilobytes of cells; a browser frame is a full screenshot. Add a
take format version whose image layer holds frames compactly (for example key frames plus
changed regions, or an encoded intermediate beside the take), with the text layer stored
per snapshot as today.

## Why

A minute of screenshots stored raw would make takes hundreds of megabytes and slow every
tune preview and render.

## Acceptance

- A take with image frames writes and reads back losslessly at the frame the source captured.
- A one-minute 1280×800 take at 24 fps stays within a size the decision Note sets.
- Terminal takes keep their current format, and old takes still load.

## Log

- 2026-11-06 Airic Lenz: grid-layers-plan (6999cd1) landed internal/pixels (key frames + one-rectangle PNG patches, take v2); the synthetic page measures 4.8 MB/min, but two distant changes per frame (typing plus a spinner) make every frame a key frame at 30.3 MB/min against the 25 MB budget; multi-rectangle patches remain
