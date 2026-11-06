---
title: Image layer patches keep one bounding rectangle per frame
status: open
type: bug
priority: 3
labels: [browser]
plan: 2026-11-06 - 04 - browser-capture-plan
created: 2026-11-06T15:52:57Z
---

The image layer encoder (`internal/pixels/pixels.go:53`) writes one bounding rectangle of the changed pixels per patch, and takes a key frame when that rectangle covers more than half the frame. Two far-apart changes in one frame (typing plus a moving spinner) span most of the frame, so every frame becomes a key frame: measured 30.3 MB/min against the 25 MB budget. Multi-rectangle patches would fix it. Found closing out plan `2026-11-06 - 03 - grid-layers-plan` (item 7).

## Acceptance

- A patch can carry several changed rectangles, so two far-apart changes per frame encode as patches, not key frames.
- A size test with typing plus a moving spinner stays within the 25 MB/min budget.
