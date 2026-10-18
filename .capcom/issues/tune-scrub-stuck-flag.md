---
title: Scrub can leave the tune preview stuck on draft frames
status: open
type: bug
priority: 3
plan: 2026-10-18 - 04 - open-issues-plan
created: 2026-10-18T18:40:19Z
---

A scrub on `photon tune`'s timeline axis ends only through `pointerup` or `pointercancel` on
the axis track (`internal/tune/page/timeline.js`, `axisRow`). If the timeline is rebuilt during
a scrub, for example on a window resize, the pointer capture is lost with the old track node.
If the pointer is then released off the new track, `endScrub` never runs. Both `isScrubbing`
flags, in timeline.js and transport.js, stay set, so the preview keeps showing draft frames and
never settles on the exact frame until the next scrub ends.

Found during verification of the tune-playback plan (item 4); not reproduced in a browser yet.

## Acceptance

- A scrub whose axis track is rebuilt mid-drag still ends on pointer release, wherever it
  happens: both scrub flags clear and the preview settles on the exact frame.
- A page test in `internal/tune` (goja) rebuilds the axis during a scrub, releases the pointer
  off the track, and checks that the exact frame is requested.
