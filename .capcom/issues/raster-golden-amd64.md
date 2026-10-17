---
title: Raster text goldens checked only on arm64
status: done
type: bug
priority: 2
created: 2026-10-17T12:24:02Z
closed: 2026-10-17T12:53:52Z
reason: "CI run 39034145942: internal/raster tests pass on ubuntu-latest (amd64), macos-latest (arm64) and windows-latest; the per-channel tolerance of 16 holds"
---

The raster text goldens are compared with a per-channel tolerance of 16
(`internal/raster/raster_test.go:26`, `goldenTolerance`; applied in `withinTolerance` at
`internal/raster/raster_test.go:472`). The goldens were generated and the tolerance was checked
only on arm64. CI also runs on amd64 runners (`.github/workflows/ci.yml`: ubuntu-latest,
windows-latest), where floating-point anti-aliasing of glyph edges may differ by more than 16 and
fail the golden tests; amd64 may need a wider tolerance or per-architecture goldens.

## Acceptance

- The raster golden tests pass on an amd64 runner as well as on arm64, either within the current
  tolerance (confirmed) or with a widened tolerance or per-architecture goldens.
