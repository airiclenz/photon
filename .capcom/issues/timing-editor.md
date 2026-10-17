---
title: A visual timing editor that previews edits against the take
status: done
type: feature
priority: 3
labels: [render]
plan: 2026-10-17 - 04 - timing-editor-plan
created: 2026-10-17T17:28:47Z
closed: 2026-10-17T19:51:09Z
reason: Resolved by the timing-editor plan (2026-10-17 - 04 - timing-editor-plan), archived at 207e422
---

Tuning a clip's timing is slow. Every change ("20% slower", "the opening is too long", "the
zooms slower still") means editing the storyboard's `duration`, `hold`, `zoom.in`/`zoom.out` or
`cut`, rendering, extracting frames with ffmpeg and looking at them, round after round.

Most of those edits need no new recording. `photon render` reads the take only: the schedule maps
take time to output time from the storyboard alone, and the compositor is a pure function of
output time and source frame. So `duration`, `hold`, `cut`, `zoom` (target, factor, in, out) and
`frame.width`/`fps`/`max_colors` can all be previewed against the existing take. Only `do`
actions change what the take holds and need a new `photon record`.

Wanted: a command, for example `photon tune <storyboard> [<take>]`, that serves a local page for
tuning those values against the take:

- Bound to 127.0.0.1 with the URL printed; the page is embedded in the binary and loads no
  external assets.
- A timeline of beats as sections: drag a section's end (`duration`), its `hold` and the zoom's
  in/out ramps, toggle `cut`. A scrubber and a play button show the composed frame at any output
  time.
- Each edit rebuilds the schedule and compositor and serves frames on demand. It shows the
  render's per-beat report live (`1.00× 481ms of take in 3s`) and flags a section that would play
  faster than render's speed warning.
- Save writes the changed values back into the storyboard by editing the YAML node tree, so
  comments, key order and flow-style entries survive; the struct is never re-marshalled.
- Previewing on a slow host is not a goal: the expected workflow tunes on a faster machine and
  pushes the storyboard back.

A new package sits beside `render` and nothing imports it; `cmd/photon` gets only the command
wiring. The `package-layout` Note, the doc maps, the README's command table and "The commands"
section describe it.

Open before planning:

- The command's name: `photon tune`, `photon edit` or `photon preview`.
- Whether `photon render --watch` (re-render on save, at a lower scale) is wanted first, as a
  cheaper partial step, or as well.
- Whether `pause` values may be edited too, marked as needing a new record, or the editor stays
  to what the take can preview.

## Acceptance

- Against a recorded fixture take, the command serves the page, and changing a beat's duration,
  hold, cut or zoom ramp changes the served frame at a given output time as `photon render` would.
- Saving a storyboard that has comments and flow-style entries yields a file byte-identical to the
  original except for the edited scalars (round-trip test).
- The server listens on 127.0.0.1 only, and the page requests nothing from outside it.
- `go vet ./...` and `go test ./...` pass.

## Log

- 2026-10-17 Airic Lenz: Known gap deferred by the plan run: /api/state does not report cols/rows/padding/font_size/line_height/scale/max_colors, so the inspector names them read-only without values
