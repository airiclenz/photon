---
title: Make the tune preview play smoothly
status: open
type: bug
priority: 1
labels: [tune]
plan: 2026-10-18 - 03 - tune-playback-plan
created: 2026-10-18T17:04:06Z
---

`photon tune`'s preview plays far below the clip's fps and stutters: every frame the page shows
costs the server about 0.75–1.1 s on the Raspberry Pi 5 host, against the 42 ms a 24 fps clip
allows. Stepping a frame or scrubbing waits as long. Measured 2026-10-18 on a 135×44 clip
(`font_size: 30`, `scale: 2`, `width: 1250`, 702 snapshots, 53 s):

- `compose.Compositor.Compose`: ~700 ms per frame, nearly all of it the CatmullRom downscale of
  the 4988×3296 raster to the 1250×826 shipped frame (`x/image/draw` `scaleX_RGBA`/`scaleY`).
- `raster.Rasterizer.Frame` on a snapshot not in the cache: ~100 ms; each raster is 66 MB, so the
  session's 1 GiB raster cache holds about 16 of the take's 702 snapshots.
- PNG encode: ~26 ms.
- The page (`internal/tune/page/transport.js`, `pumpFrames`) keeps one frame request in flight
  and fetches nothing ahead of the playhead, so it shows at most one frame per server round
  trip.

The preview is the tool's whole point: timing a beat by eye needs the clip to play at its pace.

## Acceptance

- Playing the clip above in `photon tune` on the Pi 5 host shows frames at the clip's fps or
  close to it, without visible stalls once playback has started; the frames shown while playing
  may be draft quality.
- Paused, stepped or after a scrub, the preview settles on the exact frame `photon render`
  composes at that time.
- A draft frame request on the clip above answers in well under 100 ms on the Pi 5 host.

## Log

- 2026-10-18 Airic Lenz: Plan 2026-10-18 - 03 - tune-playback-plan landed (741d7aa): draft frames at scale 1 with ApproxBiLinear, GET /api/frame?t=<ms>&draft=1, exact-frame settle on pause/step/scrub release, 4-in-flight prefetch with a 48-entry client cache. Left open: the on-host acceptance (fps playback without stalls, draft request well under 100 ms on the 135x44 clip on the Pi 5) was not measured by the run.
