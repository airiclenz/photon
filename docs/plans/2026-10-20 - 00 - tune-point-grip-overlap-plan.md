# Tune point grip overlap plan

**Goal:** on `photon tune`'s position lane, a press on a point marker selects that position and a
drag sends `position.at`; no transition grip sits over the marker.
**Date:** 2026-10-20
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 3f29fd2
**Closes:** tune-point-grip-overlap

**Regression check (2026-10-20, 3f29fd2):**
- all items SAFE

**Sources:**
- `.capcom/issues/tune-point-grip-overlap.md`
- `internal/storyboard/edit_transition.go` — `editTransition`, `endPositionAt`, `startPositionAt`:
  a transition edge that meets a point moves that point's `at`, exactly as the marker drag does.

**Ratified design calls** (Airic Lenz, 2026-10-20):
- **Grips at a point:** a transition gets no grip on an edge that meets a point (a position
  holding for 0); the marker's own shift drag moves that edge. The point keeps its end grip.
  Every other grip is unchanged. This supersedes the issue's "grips stay reachable beside it".

**Standing requirements:**
- skills: coding-standards
- Pi 5 host: single packages with `GOMEMLIMIT=2GiB`; never `-race` on `./...`; one test command
  at a time.
- Any authorized deviation from item text lands as a dated NOTES line under the item.

**Out of scope:**
- Grip placement for hard cuts and points (`beside-start`/`beside-end` in `style.css`).
- Any storyboard or edit-layer change.

## 1. Drop a transition's grip on an edge that meets a point

**What:** fix for issue `tune-point-grip-overlap`: a transition's grip centred on a point marker
takes the press, so the point cannot be selected or moved from its marker.

**Goal:** on the position lane, a transition bar has a grip on its start edge only when the
position it leaves from holds for more than 0, and on its end edge only when the position it
leads into holds for more than 0; position bars' grips are unchanged.

**Approach (assumed at the header base):** in `internal/tune/page/lanes.js`, `positionBars` builds
each transition bar's `edges` as `[EDGE_START, EDGE_END]`. Build it from the two positions the
transition meets instead: `transition.from` and `transition.index` (from kinds.js's
`transitions`) are indices into `state.positions`; a position is a point when its `end_ms`
equals its `start_ms` (the test `pointClass` already uses). Keep the logic in lanes.js; no new
module. Update the comments that describe the grips: `positionBars`' doc comment and the file
header in lanes.js, and the grip sentences in `internal/tune/page/timeline.js`'s header and
`gripNode`'s comment — find every site with `grep -n "grip\|edges" internal/tune/page/*.js`.
Update the README's position-lane paragraph (`grep -n "grips" README.md`) to say a transition
has no grip where it meets a point, which moves with its marker. The item's CHANGELOG sidecar
amends the unreleased position-lane drag entry ("Each bar has a grip on each edge") to match.

**Files:** internal/tune/page/lanes.js; internal/tune/page/timeline.js; internal/tune/pagejs_test.go; README.md
**Read first:** internal/tune/page/lanes.js — positionBars, pointClass; internal/tune/page/kinds.js — transitions; internal/tune/page/timeline.js — gripNode;
internal/tune/pagejs_test.go — TestPageJS_PositionLane_GripsEveryEdgeButAPointsStart, TestPageJS_DragAPositionOrTransitionGrip_SendsOneResizeEdit,
TestPageJS_DragAPositionOrTransitionBar_SendsOneEdit, positionedStoryboard

**Tests:** in `internal/tune/pagejs_test.go`:
- `TestPageJS_PositionLane_GripsEveryEdgeButAPointsStart`: rename to say transitions lose the
  edge at a point; the want list drops `transition:<pointPosition>/end` and
  `transition:<textPosition>/start` (the fixture's point at 700ms sits between them).
- `TestPageJS_DragAPositionOrTransitionGrip_SendsOneResizeEdit`: the "a transition's end" case
  drags a grip that no longer exists; move it to the transition into `textPosition`, whose end
  meets the held text target at `textStartMS`.
- Add a case to `TestPageJS_DragAPositionOrTransitionBar_SendsOneEdit` (or a new test) dragging
  the point position's bar: one `position.at` edit, the point selected.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPageJS_' -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/tune -count=1`
- `go vet ./internal/tune`

**Commit:** `fix(tune): drop a transition's grip where it meets a point`
