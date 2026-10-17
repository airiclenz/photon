---
title: "Camera moves: zoom and pan as independent, overlapping tracks"
status: open
type: feature
priority: 2
labels: [storyboard]
created: 2026-10-17T18:33:47Z
---

A beat's `zoom` is the only camera move a clip has. It pushes into one target, holds still and
eases back out before its section ends. A clip cannot zoom in, pan the zoomed view to another
spot, and then zoom out. It cannot move the camera across beats either, or zoom while it pans.
Two neighbouring beats zooming into different targets don't help, because the view returns to
full size between them. A demo that wants to follow the eye from a status line to the row it
changed, and on to a detail pane, has no way to film it.

Wanted: a camera made of two independent tracks, **zoom** (magnification) and **pan** (where the
view is centred). Each track is a chain of moves placed on the clip's timeline, and moves on the
two tracks may overlap freely. For example: a zoom-in starts, a pan starts while it is still
ramping, the zoom settles, the pan arrives and then goes on to a third spot, and a zoom-out
starts while that second pan is still moving. A sketch:

```yaml
camera:
  - zoom: {to: 1.8, at: {beat: 3, offset: 200ms}, over: 700ms}
  - pan:  {to: {text: 'claimed .*', area: status}, at: {beat: 3, offset: 500ms}, over: 1s}
  - pan:
      via: [{text: 'abuse-reports'}, {text: 'Ship tern v1'}]   # a curved path through waypoints
      at: {beat: 4}
      over: 2500ms
  - zoom: {to: 1, at: {beat: 5, offset: -1s}, over: 3s}
```

- **Moves.** A `zoom` move eases the magnification to `to` (1 is full frame). A `pan` move eases
  the view's centre to a target, or along a smooth curve through `via` waypoints (targets or
  cell points) to the last one. Each move has a start (`at`) and a length (`over`), and the
  easing is the same as today's ramps unless the move picks another.
- **Across beats.** A move can start in one beat and end in a later one. The camera holds its
  last state between moves, so a clip can stay zoomed in across several beats.
- **Today's `zoom` still works.** A beat's `zoom` keeps rendering exactly as now, as a shorthand
  for an in, hold and out on the two tracks.
- **Design calls for the plan:**
  - the timeline a move's `at` and `over` are in: clip time (after the speed-up of long beats),
    beat-relative, or both;
  - which screen a target is resolved on: the screen at the move's start, or at its end;
  - two moves on the same track that overlap: an error from lint, or the later one taking over;
  - the curve through `via` (for example Catmull-Rom), and whether a pan has a constant speed or
    eases at every waypoint;
  - keeping the view inside the frame near its edges (clamping the centre at each
    magnification);
  - where `camera` lives: at the storyboard's top level, or on the beats.
- `photon lint` validates moves: known areas, `over` > 0, `to` for a zoom from 1 to 3, a start
  inside the clip, and the overlap rule.
- The README ("Sections, zoom and cursor" and the storyboard reference) and the `compose` doc map
  describe the camera. The drawn cursor keeps following the camera as it follows a zoom today.

## Acceptance

- A storyboard whose moves overlap across the two tracks (a zoom-in, a pan that starts during it,
  a second pan along a `via` path, a zoom-out that starts during that pan, spanning at least two
  beats) renders, and compose tests check the camera's centre and magnification at the start,
  middle and end of each move.
- A pan through `via` waypoints follows a smooth curve: there is no sharp corner at a waypoint
  (a test on the sampled path).
- A storyboard that uses only a beat's `zoom` renders frame for frame as before (golden or box
  test).
- `photon lint` rejects an unknown area, `over` ≤ 0, a zoom outside 1 to 3, a move that starts
  outside the clip, and an overlap the chosen rule forbids.
- The README describes the camera, and a CHANGELOG line goes under Unreleased.
