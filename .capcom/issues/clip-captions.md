---
title: Captions that explain what the clip shows
status: done
type: feature
priority: 3
labels: [storyboard, render]
created: 2026-10-17T17:28:47Z
closed: 2026-10-17T22:01:25Z
reason: Resolved by the camera-captions plan (2026-10-17 - 05), archived in 3beb6aa (items 9-13)
---

A clip can show what the app does but cannot say why. A viewer who does not know the app sees
keys typed and panes change with no explanation, and the only way to add one today is to edit the
GIF in another tool after every render, which breaks the one-command re-record that photon exists
for.

Wanted: captions declared in the storyboard and drawn by `photon render` over the composed frame.
Each caption sets:

- **when**: a start and a stop time. Absolute clip times go stale as soon as a beat's `duration`
  changes, so the times should anchor to a beat (for example the beat's start plus an offset,
  running for a length or until a later beat), with an absolute clip time as an option. The
  design settles which forms exist.
- **text**: one or more lines.
- **font**: a face and a size. The embedded faces are monospace; a caption may want a
  proportional face, either a font file named in the config or a new embedded default whose
  licence then travels in the archives like the others.
- **location**: a preset (top, bottom, centre, with left/centre/right alignment) plus an optional
  offset, or explicit coordinates, kept inside the frame.
- **colour**: the text colour and an optional background box with its own colour and opacity, so
  the text stays readable over any screen.
- **fade**: optional fade-in and fade-out durations; without them the caption cuts in and out.

Captions sit in output space: a zoom does not scale or move them, and the cursor draws beneath
them. Nothing about the take changes, so adding or editing a caption needs only a new
`photon render`, never a new `photon record`.

`photon lint` validates captions: a stop after the start, times inside the clip, a known beat, a
readable font, a size and fades that fit. The README's storyboard reference and the `compose` and
`storyboard` doc maps describe them.

## Acceptance

- A storyboard with two captions, one with fades and one without, renders a GIF in which each
  caption appears and disappears at its declared times, at its position and in its colours.
- Changing a beat's `duration` moves a beat-anchored caption with that beat.
- A zoomed section leaves captions unscaled and in place.
- Unit tests cover the caption timing (anchors, fade alpha at the ramp edges), placement and
  clamping to the frame, and lint errors for a bad caption.
- `go vet ./...` and `go test ./...` pass.
