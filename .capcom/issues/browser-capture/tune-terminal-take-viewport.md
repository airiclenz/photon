---
title: tune fails on a terminal take under a viewport frame
status: open
type: bug
priority: 3
created: 2026-11-06T20:12:42Z
---

`photon tune` fails to open, instead of opening with a size problem, on a terminal take (one with no cell size) under a storyboard frame that sets `frame.viewport`. `atTakeSize` (internal/tune/session.go:408) writes the take's cols × rows into the previewed frame for such a take (internal/tune/session.go:410 takes the cols × rows branch), so the viewport stays and `render.CheckFrame` (internal/render/pipeline.go:385) refuses the frame again; the url-launch pipeline then has no image layer to draw from (internal/render/pipeline.go:207, `errNoImageLayer`). `NewSession` returns the error rather than a session showing the size problem, as it does for a mismatched take of any other kind.

## Why

A stale take should open in tune with its size problem shown, so the user is told to re-record rather than tune refusing to start.

## Acceptance

- photon tune opens on a terminal take under a frame with frame.viewport and reports the take-size problem, with a test in internal/tune/session_test.go
