---
title: photon render buffers every frame and OOMs on long clips
status: open
type: bug
priority: 2
created: 2026-10-17T14:04:09Z
---

`photon render` holds every frame of a clip in memory inside ffmpeg, so a long clip runs the
host out of memory. On the Raspberry Pi 5 (8 GB), rendering a 1250x826, 24 fps clip of about
54 s of output fails with `photon: ffmpeg: signal: killed`. dmesg shows a global OOM kill with
ffmpeg at an anon-rss of 4.25 GB.

## Cause

`internal/render/encode.go` builds the filter graph
`[0:v]split[a][b];[a]palettegen=max_colors=%d[p];[b][p]paletteuse=dither=%s`. palettegen emits
its palette only at end of input, so `split` buffers every raw RGBA frame (W*H*4 bytes, about
4.1 MB per frame at 1250x826) until then. Memory grows linearly with clip length.

## Wanted

Render memory stays bounded by a constant, not by clip length, and the output GIF stays
byte-identical. Possible approaches:

- Two ffmpeg passes: pass 1 runs `palettegen` into a PNG in a temp dir (a histogram, so
  constant memory); pass 2 runs `paletteuse` with that PNG as a second input. The frames are
  streamed to each pass, either re-rastered (twice the raster CPU) or spooled to a file under
  the work dir. Never spool under `/tmp`, which is RAM on the Pi.
- Compute the palette in Go from the frames as they are rastered.

palettegen's default `stats_mode=full` gives the same palette either way, so the raster goldens
should still hold. `--dry-run`'s printed argv and `internal/render/doc.go` change with it.

## Acceptance

- A long render (a test, or a recorded manual check) shows ffmpeg's peak RSS staying flat as
  the frame count grows.
- The render goldens pass unchanged.
- `photon render --dry-run` prints the new argv, and `internal/render/doc.go` describes the
  new pipeline.
