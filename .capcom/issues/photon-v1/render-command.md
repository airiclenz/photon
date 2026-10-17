---
title: "photon render: schedule, raster, compose, encode"
status: done
type: feature
priority: 2
deps: [take-format, storyboard-schema, project-config, declarative-areas, embedded-fonts]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
closed: 2026-10-17T12:25:04Z
reason: Resolved by plan "2026-10-17 - 00 - photon-v1-plan" (archived in bae9f7a)
---

`photon render <storyboard> [<take>]` turns a take into the shipped GIF. Each stage is its own
package (Note `package-layout`):
- schedule: each beat's stretch of the take onto its section `duration`, with `hold` and `cut`;
  1× when it fits, then the last frame frozen; a warning above 6×.
- raster: cells to pixels at `scale`, colours from the config's `theme`, box-drawing and
  block-element glyphs drawn rather than looked up, fonts from issue `embedded-fonts`.
- compose: an eased zoom onto the beat's target, and a drawn cursor gliding to each click with
  a ring.
- encode: frames piped to ffmpeg's palette encode at `width`/`fps`/`max_colors`, without
  dithering, then a lossless `gifsicle -O3` pass when gifsicle is on PATH.

It writes the storyboard's `ship:` path unless `-o` names another, prints the clip's path, size
and length and one speed row per beat, and `--dry-run` prints the ffmpeg command line.

## Acceptance

- Unit tests per package: the schedule maths, raster goldens for text and box drawing, zoom and
  cursor positions over time.
- A render test on a small fixture take produces a GIF of the expected size and length; it
  skips when ffmpeg is absent.
- `--dry-run` is tested without ffmpeg.

## Log

- 2026-10-17 Airic Lenz: Deferred by plan 2026-10-17 - 00 - photon-v1-plan (bae9f7a): raster text goldens compare per channel within a tolerance of 16 and were only run on arm64; amd64 CI may need a wider tolerance or per-architecture goldens
