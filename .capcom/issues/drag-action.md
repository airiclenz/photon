---
title: "A drag action: press, move and release the mouse across the screen"
status: open
type: feature
priority: 2
labels: [storyboard]
created: 2026-10-17T13:25:28Z
---

A storyboard can click but not drag. `click` sends one SGR press and its release on the same
cell, so a gesture that moves with the button held never reaches the program: dragging a pane
divider, a scrollbar thumb or a slider, or drag-selecting text. A clip that wants to show one of
these has to fake it with keys, and the drawn cursor never shows the movement.

Wanted: a `drag` action beside `click`, for example

```yaml
- drag: {from: {text: '│', area: panes}, to: {offset: {x: 12}}, steps: 8}
```

- `from` is a target, resolved on the screen as it stands, as `click`
  resolves one. `to` is a target or an offset in cells from the press cell; exactly one of the
  two.
- The engine sends the left-button press at `from`'s centre cell, then motion reports with the
  button held (SGR button code 32) at evenly spaced cells along the path, paced so the program
  reads each as its own report, then the release at the end cell. The cells stay on the screen
  (clamped to `frame.cols` × `frame.rows`).
- The take logs the resolved boxes and every cell sent, as new event kinds beside `target` and
  `click`, so `check` and `render` can read the gesture back; a take holding them is still read by
  this photon, and the take format version moves only if an older photon must refuse it.
- The drawn cursor glides to the press point as it does for a click, follows the drag along its
  path while the button is held, and rings on the press and on the release. A zoom on a beat
  whose drag targets the zoom's own target aims at the logged box, as it does for a click.
- `photon lint` validates the new action: one of `to.text` and `to.offset`, a known area, `steps`
  ≥1.
- The README's actions table, the storyboard reference and the `engine`, `compose` and `take`
  doc maps describe it.

## Acceptance

- A storyboard that drags in the fixture TUI records a take whose beats and expects all pass,
  and the fixture shows it received the press, at least one held-button motion report and the
  release, in that order (fixture test, no network, no ffmpeg needed).
- Unit tests cover the SGR motion report, the path's cells and clamping, lint errors for a bad
  `drag`, and the cursor's position and rings across a drag.
- `photon render` of that take draws the cursor following the drag.
- `go vet ./...` and `go test ./...` pass.
