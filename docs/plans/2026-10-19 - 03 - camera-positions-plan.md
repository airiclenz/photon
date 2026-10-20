# Camera positions plan

**Goal:** the storyboard aims the camera with `camera.positions` and zooms with `camera.zooms`,
the render follows positions and transitions, and `photon tune` edits them on a position lane
with transition bars, a draggable preview marker and the picker. `pan` moves and `via` paths are
gone from the schema, the code and the docs.
**Date:** 2026-10-19
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** c096441
**Closes:** position-schema; tune-position-lane; camera-positions

**Sources:**
- `.capcom/issues/camera-positions.md` and its children under `.capcom/issues/camera-positions/`
- Note `glossary`, Note `package-layout`, Note `coding-conventions`

**Ratified design calls** (Airic Lenz, 2026-10-19):
- **Scope:** one plan for all four child issues; it also closes the parent `camera-positions`.
- **Glide:** a transition glides from the crop centre shown at its start to the one shown at its
  end, each clamped at that moment's zoom, and the crop is clamped again every frame.
- **Ease curves:** `ease-in` is t³, `ease-out` is 1−(1−t)³, `smooth` stays smootherstep.
- **List order:** `camera.positions` may be listed in any order; render and lane sort by start;
  lint refuses only overlaps (two positions starting at the same time overlap).

**Standing requirements:**
- skills: coding-standards
- Pi 5 host: single packages with `GOMEMLIMIT=2GiB`; never `-race` on `./...`; one test command
  at a time.
- Any authorized deviation from item text lands as a dated NOTES line under the item.
- Items 1–3 strip the old camera from tune and the edit layer, so tune shows no camera until
  item 14; that gap is intended (the `camera:` list is unreleased).

**Out of scope:**
- Any compatibility with the `camera:` list, `pan` moves or `via` (they fail as ordinary schema
  errors).
- A full-view draft frame; schedule, captions and cursor behaviour.
- Version or release changes.

**Regression check (2026-10-19, c096441):**
- 1: guard folded
- 2: guard folded
- 3: guard folded
- 4: guard folded
- 5: guard folded
- 6: recast
- 7: recast; guard folded (validate.go, the `camera:` fixture sweep, doc links, comment and identifier greps); supersedes the CHANGELOG.md [Unreleased] removed-zoom bullets (:12-13) through item 20's sidecar
- 8: guard folded
- 9: guard folded (insert kinds and defaults, doc.go)
- 10: guard folded
- 12: recast; `centre` is the position track's pre-clamp value
- 13: recast; guard folded (each bad query's answer, full-view pick Rect)
- 14: guard folded (every app.js entry-kind site)
- 15: guard folded
- 17: guard folded; `+` takes item 12's pre-clamp `centre` (replaces the earlier rejection)
- 18: recast; guard folded (overlay and pick follow the shown frame's view)
- 19: guard folded
- 20: guard folded (narrowed grep, binding over Files); supersedes the CHANGELOG.md [Unreleased] pan, via, pick and `camera:` list bullets

## 1. Tune page drops the old camera lanes, panels, pan pick and via path — ✅ DONE (2026-10-20)

NOTES (2026-10-20): consequential edit — internal/tune/page/controls.js: made necessary by dropping the camera move panel (three comments named a camera move's owner; code unchanged)
NOTES (2026-10-20): TestPageJS_RemoveAndMoveACameraMove is recast as TestPageJS_RemoveAndMoveACaption (caption 0 moved down, then removed) instead of deleted, so the inspector's list buttons keep a page test; the camera-only TestPageJS_RefusedLaneAdd_* and TestPageJS_RefusedBarDrag_* are deleted, leaving no page test of a refused lane add or bar drag until a later lane brings one back
NOTES (2026-10-20): lanes.js `adding` and entries.js `entryPanel` now look their list and panel up by entry kind (STATE_LISTS, PANELS), each holding only the caption row, so the later position and zoom lanes add a row each
NOTES (2026-10-20): picker.js keeps pickProposal and reportPick on window.tunePage with no reader left, and controls.js keeps targetInputs/targetRows/targetOf with no caller left, for the position target items (15, 19); the unused test consts elementPickStatus and elementStatus went with their tests

**What:**
**Goal:** the tune page has no `camera zoom` or `pan` lane, camera move panel, "Set as pan
target" button or via-path overlay; `internal/tune/page/path.js` is gone; no page script reads
`state.camera`; no page test fixture holds a `camera:` key.
**Approach (assumed at the header base):** `lanes.js` drops the two camera lanes and
`cameraBars`/`zoomLabel`/`panLabel`; `entries.js` drops `cameraPanel`, `aimRows`, `waypointRows`
and their helpers; `picker.js` drops `#pick-pan`, `setAsPanTarget`, `offerPan`, `selectedPan`
(keeps `pickProposal`); delete `path.js` and its `<script>` tag, `#via-path` and `#pick-pan` from
`index.html`, `--lane-pan`/`.via-*` from `style.css`; drop the camera branch of `timeline.js`
`shiftEdit` and `app.js` `entryOf`/`renderBeatSelect`, and via handling from `fit.js`. In
`pagejs_test.go` delete the camera-only tests and helpers (`isViaPathDrawn`, `assertViaPath`,
`elementViaPath`) and the camera subtest of `TestPageJS_EditProblem_ShowsOnItsOwningPanel`, keep
(renamed) the pick-box half of `TestPageJS_FitPreview_KeepsThePickBoxAndViaPathOnTheShrunkFrame`,
remove `camera:` from every fixture (`roomyCameraStoryboard` included); adjust
`TestPage_Lanes_ReadCameraAndCaptions`.
**Regression guard.** Every row, assert, helper and const in `page_test.go`/`pagejs_test.go` naming camera, pan, via, `pick-pan`, `OwnerCamera`, `FieldCamera` or `State.Camera` goes (`entryReaders`' move row, `TestPageJS_LoadsAndWires`' `#pick-pan` assert, `TestPageJS_MoveButtons_DisabledAtTheListEnds`' camera rows, unused helpers such as `barLeft`, `laneCameraZoom`, `lanePan`, `viaPan`, `panToTarget`); `isEntryField` drops its `camera.` half.
`splitScripts` drops `"path.js"` and its comment's "the via path overlay's"; `registeredLanes` keeps only the captions row; `style.css` drops `.block.camera-zoom`/`--lane-zoom` with `--lane-pan`.
The remaining page tests run on a captions-only fixture (`sessionStoryboard` plus the two captions) added in `pagejs_test.go`, not on `session_test.go`'s `cameraCaptionsStoryboard`/`aimedStoryboard`; `internal/tune/doc.go`'s `page/path.js` and `page/entries.js` map lines are updated.
**Files:** internal/tune/page/lanes.js, internal/tune/page/entries.js, internal/tune/page/picker.js, internal/tune/page/path.js, internal/tune/page/index.html, internal/tune/page/style.css, internal/tune/page/timeline.js, internal/tune/page/app.js, internal/tune/page/fit.js, internal/tune/pagejs_test.go, internal/tune/page_test.go, internal/tune/doc.go
**Read first:** internal/tune/page_test.go — splitScripts, registeredLanes, entryReaders, TestPage_EntryFields_AreEditFields, TestPage_Lanes_ReadCameraAndCaptions; internal/tune/pagejs_test.go — loadPage, TestPageJS_LoadsAndWires, TestPageJS_MoveButtons_DisabledAtTheListEnds, TestPageJS_FitPreview_KeepsThePickBoxAndViaPathOnTheShrunkFrame;
internal/tune/page/app.js — page (window.tunePage), ownerKey, renderBeatSelect, entryOf; internal/tune/page/lanes.js — cameraBars, window.tuneLanes; internal/tune/page/timeline.js — shiftEdit; internal/tune/page/picker.js — wire, pickProposal;
internal/storyboard/fields.go — Fields
**Tests:** the remaining page tests, unchanged in intent, on the captions-only fixture.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPage'`
- `grep -rnE 'state\.camera|via|pick-pan|camera\.' internal/tune/page/*.js internal/tune/page/index.html` prints nothing
- `grep -nE 'Camera|camera|via|pick-pan' internal/tune/page*_test.go` prints nothing
**Commit:** `refactor(tune): drop the pan and zoom move lanes from the page`

## 2. Tune server drops camera state and the path route — ✅ DONE (2026-10-20)

NOTES (2026-10-20): re-derived from the assumption that page_test.go's entryReaders holds a CameraState row: item 1 already removed it, so page_test.go needed no change and is not in FILES.
NOTES (2026-10-20): consequential edit — internal/tune/pick.go: made necessary by the Acceptance grep for `camera:`, which matched the Pick doc comment's "pipeline's camera: the row"; the colon became a semicolon, no wording changed.
NOTES (2026-10-20): the tests that broke the pipeline with a pan target not on screen (CaptionBoxes_AbsentWithoutAFullPipeline, EveryEditsBuild_ReusesTheGoodPipeline, and the former PanTargetNotOnScreen_KeepsTheLastGoodPipelineAndBlocksSave, renamed BuildError_KeepsTheLastGoodPipelineAndBlocksSave) now break it with a caption text wider than the frame (new tooWideText and tooWideCaptionEdit helpers).
NOTES (2026-10-20): renamed the fixture cameraCaptionsStoryboard to captionsStoryboard and TestSession_State_ReportsCameraMovesAndCaptions to TestSession_State_ReportsCaptions, since neither holds a camera any more; TestHandler_EditsAsThePageSendsThem_AreApplied keeps only its beat drag (its camera.to half went with the camera edits).
NOTES (2026-10-20): kept TargetState, CellState and PointState in state.go (no longer read by State) because item 11 builds position state on TargetState.

Depends on item 1.

**What:**
**Goal:** `internal/tune` has no `CameraState`, `camera` state key, `GET /api/path` route,
`Session.Path`, `ErrNoPath`, or Go test fixture with a `camera:` key; `go vet ./internal/tune`
passes.
**Approach (assumed at the header base):** `state.go` drops `State.Camera`, `CameraState`,
`cameraStates`, `pointState`, `movedState` and `hasOwner`'s `OwnerCamera` case; `server.go` drops
`routePath`, `pathCameraParam`, `handlePath`, `pathResponse`; `session.go` drops `Path` and
`ErrNoPath`. Delete the camera and pan tests in `session_test.go`, `server_test.go`,
`pick_test.go` (the pan-destination proposals; item 19 restores the round trip for positions)
and `edit_test.go`'s `cameraEdit`; strip `camera:` from the remaining fixtures; `doc.go` stops
naming pans, moves and the path route.
**Regression guard.** `internal/tune/page_test.go` joins Files: its `entryReaders` row for `CameraState` goes with the type, or the tune test package stops compiling.
**Files:** internal/tune/state.go, internal/tune/server.go, internal/tune/session.go, internal/tune/doc.go, internal/tune/session_test.go, internal/tune/server_test.go, internal/tune/pick_test.go, internal/tune/edit_test.go, internal/tune/frame_test.go, internal/tune/page_test.go
**Read first:** internal/tune/state.go — State, CameraState, cameraStates, hasOwner, ownerOn; internal/tune/server.go — newHandler, handlePath, statusOf; internal/tune/session.go — Path, ErrNoPath, rebuild;
internal/tune/session_test.go — cameraCaptionsStoryboard, aimedStoryboard, TestSession_PanTargetNotOnScreen_KeepsTheLastGoodPipelineAndBlocksSave, TestSession_EveryEditsBuild_ReusesTheGoodPipeline, TestSession_State_CaptionBoxes_AbsentWithoutAFullPipeline;
internal/tune/page_test.go — entryReaders; internal/tune/pick_test.go — pickStoryboard, newPickSession; internal/tune/edit_test.go — cameraEdit; internal/tune/doc.go — file map
**Tests:** remaining tune tests.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune`
- `grep -rnE 'CameraState|api/path|ErrNoPath|camera:' internal/tune/*.go` prints nothing
**Commit:** `refactor(tune): drop camera move state and the via path route`

## 3. Storyboard drops the camera move edits — ✅ DONE (2026-10-20)

NOTES (2026-10-20): consequential edit — internal/storyboard/edit_beat.go: made necessary by deleting edit_camera.go (keyZoom lost its last reader and is deleted; the Target.YAMLFlow comment no longer names a pan's camera.to or waypoint edit)
NOTES (2026-10-20): consequential edit — internal/storyboard/edit_caption.go: made necessary by dropping the camera list (the key-block and isCaptionField comments no longer name a camera move; the editList call drops the board argument editList and insertEntry no longer read)
NOTES (2026-10-20): consequential edit — internal/storyboard/beat.go: made necessary by deleting camera.to (the Target comment no longer names a pan's camera.to edit)
NOTES (2026-10-20): consequential edit — internal/tune/pick.go: made necessary by deleting camera.to (the Proposal comment no longer names the text a pan's camera.to edit writes)
NOTES (2026-10-20): editName drops its camera-owner branch along with ownerKindOf's, so a camera owner on a beat or caption field is now named by the field alone; edit_test.go asserts that and that camera.at is an unknown field. KindPoint is kept: exported and named by no camera field in its comment, left for the position edits.
NOTES (2026-10-20): the camera-only overlap insert test became TestApply_InsertEndingPastTheClip_IsRefusedNamingIt, the same ValidationError path on a caption; cameraEditBoard stays for the "captions after the camera" row.
NOTES (2026-10-20): `make lint` reports one issue, an ineffassign at internal/tune/pagejs_test.go:1068, which predates this run (2d20645, an ancestor of base c096441); the storyboard package lints clean.

Depends on item 2.

**What:**
**Goal:** `storyboard.Apply` and `storyboard.Fields` know no camera field: no `camera.*` field
constant, no `edit_via.go`, no camera entry in the list edits (captions keep `caption.insert`,
`caption.remove`, `caption.move`); the `Edit` doc comment names no camera field.
**Approach (assumed at the header base):** delete `edit_via.go`, `edit_via_test.go` and
`edit_camera.go` (`wirePoint`, `decodePoint`, `pointFlow`, `centreCell` go with it unless a
caption edit uses them); `edit_list.go` drops `cameraList`, `entryZoom`/`entryPan`,
`FieldCameraInsert/Remove/Move`, `newZoomFactor`/`newMoveLength`; `fields.go` drops
`cameraFields`/`pathFields`/`KindWaypoint` and the camera insert field; `edit.go` drops the camera
branch of `ownerKindOf`/`apply` and the `Edit` comment's camera lines. `edit_list_test.go`'s
list-mechanics tests on camera entries are rewritten on captions; camera-only tests in
`edit_test.go`/`fields_test.go` go; `doc.go`'s map follows.
**Regression guard.** Move `keyAt` (the one key const caption code reads: `edit_caption.go`, `edit.go` `setMoment`, `edit_list.go` `entryFlow`) into `edit.go` before deleting `edit_camera.go`; delete `keyCamera`/`keyPan`/`keyTo`/`keyOver`/`keyEase` with it.
Delete every unexported identifier left without a reader (`easeChoices`, `KindWaypointRemoval` with `KindWaypoint`, the camera test helpers; keep `cameraEditBoard` only while `edit_list_test`'s "captions after the camera" row uses it), then run `make lint` clean (golangci `unused`).
Every comment in `internal/storyboard/edit*.go` and `fields.go` naming a camera field, pan, via or waypoint is rewritten (the `Apply`, `editName`, `setMoment`, `Field`/`Fields`/`KindEntry`/`KindPoint` and `entryList`/`listFields` comments included, not only `Edit` and `doc.go`).
**Files:** internal/storyboard/edit_camera.go, internal/storyboard/edit_via.go, internal/storyboard/edit_via_test.go, internal/storyboard/edit_list.go, internal/storyboard/edit_list_test.go, internal/storyboard/edit.go, internal/storyboard/fields.go, internal/storyboard/fields_test.go, internal/storyboard/edit_test.go, internal/storyboard/doc.go
**Read first:** internal/storyboard/edit_camera.go — keyAt, wirePoint, decodePoint, pointFlow, centreCell; internal/storyboard/edit.go — editFields, ownerKindOf, splicer.apply, Apply, setMoment, editName; internal/storyboard/edit_list.go — cameraList, listFields, isRemoveField, entryFlow;
internal/storyboard/fields.go — Fields, cameraFields, pathFields, easeChoices; internal/storyboard/edit_list_test.go — TestApply_ListEdit_ChangesOnlyTheListsText, TestApply_InsertIntoAnAbsentOrEmptyList_WritesAFlowList;
internal/storyboard/fields_test.go — defaultRefusals, TestFields_DescribeEachOwnersFields; internal/storyboard/doc.go — file map
**Tests:** storyboard and tune tests.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard ./internal/tune`
- `grep -rn 'FieldCamera\|editVia\|cameraList' internal/` prints nothing
- `grep -nE 'camera\.|camera move|\bpan|via|waypoint' internal/storyboard/{edit,edit_list,fields}.go` prints nothing
- `make lint`
**Commit:** `refactor(storyboard): drop the camera move edits`

## 4. Storyboard: position and zoom schema with validation — ✅ DONE (2026-10-20)

NOTES (2026-10-20): to file a position's point and timing problems under the entry itself (`camera.positions[i]: cell: …` rather than `…: .cell: …`), `Point.validate` now takes an empty field for keys that sit at the owner's top level, and `placeMove` takes a reporter its caller has already prefixed (the old moves pass `zoom.`/`pan.`). The old moves' messages are unchanged apart from the four-ease wording.
NOTES (2026-10-20): zoom overlaps now go through the same helper as position overlaps, so they are filed under the later zoom by start rather than the later one in the list. The rule itself is unchanged, and the old `camera:` list keeps its own overlap check until item 7.
NOTES (2026-10-20): a `target:` key on an entry under `camera.zooms` still decodes into `ZoomMove.RemovedTarget`, and the new validation ignores it. Item 7 drops that field, after which the strict decode refuses the key.

Depends on item 3.

**What:**
**Goal:** `internal/storyboard` declares `Camera{Positions []Position; Zooms []ZoomMove}` (yaml
`positions`, `zooms`); `Position` (yaml `at`, `hold`, `text`, `nth`, `area`, `cell`, `ease`) with a
resolved `Start()`; the eases `linear`, `smooth`, `ease-in`, `ease-out`; owner kinds `position`
and `zoom` (prefixes `camera.positions[i]: `, `camera.zooms[i]: `, constructors
`AtPosition`/`AtZoom`); and a validation of a `Camera` against a frame and an output clock.
`Storyboard.Camera` is untouched (item 7 wires it); until then only tests reach the new code.
**Approach (assumed at the header base):** new `internal/storyboard/position.go` (`Position`
embedding `Point` inline; `hold` absent means 0); the `Camera` type beside the existing moves in
`camera.go`. Refusals, each under its entry: `exactly one of text or cell`; nth/area on a cell; a
cell outside `frame.cols`/`rows`; `hold` below 0; a start outside `[0, clip length)`; a position
ending after the clip (`start+hold > length`); an unknown ease (message lists the four);
overlapping positions (half-open `[start, start+hold)`; equal starts overlap), filed under the
later by start and naming the other; for zooms today's checks (factor `[1, 3]`, `over > 0`,
placement, no two zooms overlap). `Ease` gains `EaseIn = "ease-in"`, `EaseOut = "ease-out"`, and
`placeMove`'s ease check takes all four.
**Regression guard.** `internal/storyboard/storyboard_test.go` joins Files: `TestLoadRejects_Camera`'s "ease unknown" row's want is updated to the four-ease wording (`placeMove` still serves the old camera moves until item 7).
**Files:** internal/storyboard/position.go, internal/storyboard/position_test.go, internal/storyboard/camera.go, internal/storyboard/located.go, internal/storyboard/doc.go, internal/storyboard/storyboard_test.go
**Read first:** internal/storyboard/camera.go — placeMove, validateCamera, Point.validate, span.overlaps, ZoomMove.validate; internal/storyboard/validate.go — collector.at, prefixed, reporter; internal/storyboard/located.go — Owner.prefix, AtCamera;
internal/storyboard/storyboard_test.go — TestLoadRejects_Camera, cameraBase; internal/storyboard/clock.go — OutputClock.Resolve, OutputClock.Length; internal/storyboard/doc.go — file map
**Tests:** table-driven tests of each refusal, naming the offending entry, and of a valid camera
(cells, text targets, points, held bars, a zero gap, positions listed out of order); the old
moves' "ease unknown" row with the four-ease message.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard`
**Commit:** `feat(storyboard): add camera positions and zooms with their validation`

## 5. Compose: the position track and the four eases — ✅ DONE (2026-10-20)

NOTES (2026-10-20): doc.go's package comment now says every move's progress runs by its ease (four curves) and that a zoom eases its exponent, replacing "smootherstep, or straight when linear"; the position track is not described there yet (item 7 wires it in), only listed in the file map.
NOTES (2026-10-20): `make lint` fails outside this item: internal/tune/pagejs_test.go:1068 ineffassign on `height`, left behind when da397bc (item 1) removed the `wantPath` line that read it.

Depends on item 4.

**What:**
**Goal:** `internal/compose` holds a position track: positions on the output clock (start, end,
aim point in source pixels, ease), sorted by start, whose centre at t is: before the first, the
first's aim; during a hold, that position's aim; across a gap from P to N, a glide from
`cropAround(size, factor(P.end), P.aim).centre()` to `cropAround(size, factor(N.start),
N.aim).centre()` eased by N's ease; a gap of 0 a cut at N.start; after the last, the last's aim;
no positions, the frame's centre. Every camera progress eases by `linear`, `smooth`
(smootherstep), `ease-in` (t³) or `ease-out` (1−(1−t)³).
**Approach (assumed at the header base):** a new `positionTrack` in `internal/compose/position.go`,
built from compose-internal values (no storyboard read yet), with a `centreAt(t)` taking the zoom
track's `factorAt`; `move.progress` switches over the four eases through one `eased(ease, share)`
helper. The old pan track stays until item 7.
**Regression guard.** `eased` treats an empty ease as smooth, as `move` documents (most existing moves in `camera_test.go` leave `Ease` empty). The helper goes in `internal/compose/camera.go`, not `cursor.go` (outside Files), calling `easeInOut` from there.
**Files:** internal/compose/position.go, internal/compose/position_test.go, internal/compose/camera.go, internal/compose/camera_test.go, internal/compose/doc.go
**Read first:** internal/compose/camera.go — move.progress, trackAt, camera.factorAt, newCamera; internal/compose/compose.go — cropAround, rectF.centre, Compositor.crop; internal/compose/cursor.go — easeInOut, clamp01, lerp;
internal/compose/camera_test.go — cameraLayout, newCamera table tests; internal/compose/doc.go — file map
**Tests:** centre at a hold, mid-transition for each ease (at a share where smooth and linear
differ), at a cut, before the first and after the last, with none, a glide from a crop clamped at
an edge (no dead time at either end), positions given out of order; zoom progress for each ease,
and a row with `Ease` "" easing as smooth.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/compose`
**Commit:** `feat(compose): add the position track and the ease-in and ease-out curves`

## 6. README storyboard reference and glossary describe positions and zooms — ✅ DONE (2026-10-20)

NOTES (2026-10-20): per the regression guard, README keeps the old zoom/pan/via move text verbatim under a new `#### The list form` subheading at the end of `### Camera` (its own stale "`smooth` or `linear`" ease lines included); the new Positions, Transitions, Zooms and Eases subsections come first, and the storyboard example and header `camera` row now show only the new shape.
NOTES (2026-10-20): left for later items: README's Features bullet (line 45, "Zoom in and pan"), the `## Sections and cursor` camera paragraph (names `pan` moves and the beat `zoom` load error) and the `photon tune` rows still describe camera moves; Targets and areas now names camera positions alongside pans, since `pan` stays loadable until item 7.

Depends on item 4.

**What:** Recast at the regression check (2026-10-19).
**Goal:** README's storyboard reference (the `### Camera` section, the storyboard example's
`camera:` block, the header table's `camera` row and the `## Targets and areas` section)
describes `camera.positions` (`at`, `hold`, `cell` or `text`/`nth`/`area`, `ease`), transitions,
`camera.zooms` (`to`, `at`, `over`, `ease`) and the four eases; Note `glossary` has entries for
**Position**, **Transition**, **Zoom** and **Cell** and its Target entry no longer says pan.
**Approach (assumed at the header base):** prose follows the header's ratified calls and items 4
and 5. Edit `.capcom/knowledge/glossary.md` by hand, then `capcom check`.
**Regression guard.** README's TestREADMEDocumentsEveryField needs every yaml tag that decodes at each commit, so item 6 ADDS the positions/zooms/transitions reference and the four eases to README while leaving the old zoom/pan/via move text in place; its README goal no longer says "names neither pan nor via" (the glossary part is unchanged).
**Files:** README.md, .capcom/knowledge/glossary.md
**Read first:** cmd/photon/readme_test.go — TestREADMEDocumentsEveryField, yamlTags; README.md — "### Camera", header table `camera` row, "## Targets and areas"; .capcom/knowledge/glossary.md — Target entry; internal/storyboard/camera.go — Point, Cell, ZoomMove (yaml tags the README test walks)
**Tests:** `cmd/photon` README tests.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./cmd/photon -run TestREADME`
- `capcom check`
- `grep -nE '\bpan\b|\bvia\b' .capcom/knowledge/glossary.md` prints nothing
**Commit:** `docs: describe camera positions, transitions and zooms`

## 7. The storyboard camera is positions and zooms; render follows the position track — ✅ DONE (2026-10-20)

NOTES (2026-10-20): re-derived from the assumed `validateCamera` delegation: `Storyboard.validate` now calls item 4's `Camera.validate(s.Frame, s.Clock(), collect)` directly, and `validateCamera` is deleted along with `CameraMove`, `PanMove`, `trackZoom`/`trackPan`, `minViaPoints` and the move-form helpers; `moveTiming`/`placeMove` stay for the zooms.
NOTES (2026-10-20): compose's `resolveShown` (the walk back across a move's screens) is deleted as dead; `zoomPlanner.aim` resolves a text target on `take.ScreenAt(shownAt(start))` only. `trackAt` lost its `timing()` constraint parameter, which existed only for the deleted pan segments.
NOTES (2026-10-20): internal/storyboard/edit_list_test.go, located in Files, needed no change: its "captions after the camera" row passes on the rewritten `cameraEditBoard` mapping, and the table's shared check reads only `board.Captions`, never `board.Camera`. position.go, edit_beat.go and edit_caption.go carried no stale camera-move text either.
NOTES (2026-10-20): `Point.validate` keeps its `field` parameter although its only remaining caller (`Position.validate`, item 4's position.go, outside Files) passes "".
NOTES (2026-10-20): pan/via/camera-move wording outside this item's packages stays for its owners: internal/caption/doc.go, internal/target/doc.go, internal/check/doc.go, cmd/photon/tune.go and README's Features/tune/Captions lines (item 20), internal/tune/state.go and session.go (items 11 and 20). CHANGELOG [Unreleased] bullets naming pan moves and `camera[i]: zoom: target: removed` are stale until item 20's sidecar supersedes them, as the plan says.

Depends on items 5, 6.

**What:** Recast at the regression check (2026-10-19).
**Goal:** `Storyboard.Camera` is a `Camera` decoded from a `camera:` mapping and validated as
item 4 defines; a `camera:` list, a `pan` key, a `via` key or a zoom `target` key fails to load
with the ordinary strict-decode error; `CameraMove`, `PanMove`, `OwnerCamera`, `AtCamera`,
compose's pan track, `path.go` and `Compositor.Path`/`Pipeline.Path` are gone; the render aims by
the position track and zooms by `camera.zooms`, a text target resolving on the screen shown at
its position's start only and failing the render under `camera.positions[i]` wrapping
`target.ErrNotOnScreen` when absent there; `photon lint` refuses an undeclared position `area`
under its position.
**Approach (assumed at the header base):** `storyboard.go` field type and `applyDefaults` (smooth
on positions and zooms); `validateCamera` delegates to item 4's validation; move `removedKey` to
`beat.go` (beats still use it) and drop `ZoomMove.RemovedTarget`. In compose, `zoomPlanner`
resolves each position (cell → `cellCentre`; text → the resolver on the snapshot at
`shownAt(schedule, start)`) into item 5's track; `camera` drops `pan`/`waypoints`.
`clip.cameraAreas` checks positions. Rewrite `internal/storyboard/testdata/every-form.yaml`,
`cmd/photon/testdata/invalid/camera.yaml` and `camera-area.yaml` with `lint_test.go`'s
expectations, and the compose, render and clip test fixtures.
**Regression guard.** README.md joins Files: item 7 deletes README's storyboard-reference text for the camera list, pan moves and via (the text item 6 left); the tune section and other README mentions stay for item 20. The removed-zoom load messages it changes are CHANGELOG.md [Unreleased] :12-13 (`camera[i]: zoom: target: removed; aim with a pan move…`, `beat <id>: zoom: removed; use a camera zoom move and a pan move`); item 20's sidecar supersedes them, stale until then.
`validate.go` joins Files: `Beat.validate`'s removed-zoom report (built from `trackZoom`/`trackPan`) names `camera.zooms`, and `storyboard_test.go`'s "beat zoom" row's want changes with it. Every storyboard fixture, Go string or yaml, with a `camera:` list is rewritten, found with `grep -rnE '^\s*camera:\s*$|camera: *\[|- (zoom|pan):' --include=*.go --include=*.yaml internal cmd` (not `yamledit/sequence_test.go`): `edit_test.go`'s `cameraEditBoard` becomes a `camera: {positions: …}` mapping and `edit_list_test.go`'s "captions after the camera" row checks `len(board.Camera.Positions)+len(board.Captions)`.
Every doc link to a deleted identifier and every comment in storyboard, compose, render and clip naming pans, via or camera moves is rewritten (`storyboard/doc.go`, `compose/doc.go`, `render/doc.go`, `render/render.go`, `storyboard/caption.go`, `edit_beat.go`, `edit_caption.go` among them), checked by the Acceptance greps.
**Files:** internal/storyboard/camera.go, internal/storyboard/storyboard.go, internal/storyboard/beat.go, internal/storyboard/located.go, internal/storyboard/validate.go, internal/storyboard/caption.go, internal/storyboard/edit_beat.go, internal/storyboard/edit_caption.go, internal/storyboard/doc.go, internal/storyboard/storyboard_test.go, internal/storyboard/located_test.go, internal/storyboard/edit_test.go, internal/storyboard/edit_list_test.go, internal/storyboard/testdata/every-form.yaml, internal/compose/camera.go, internal/compose/path.go, internal/compose/path_test.go, internal/compose/compose.go, internal/compose/doc.go, internal/compose/camera_test.go, internal/compose/compose_test.go, internal/render/pipeline.go, internal/render/render.go, internal/render/doc.go, internal/render/pipeline_test.go, internal/clip/clip.go, internal/clip/clip_test.go, cmd/photon/lint_test.go, cmd/photon/testdata/invalid/camera.yaml, cmd/photon/testdata/invalid/camera-area.yaml, README.md
**Read first:** internal/compose/camera.go — zoomPlanner.planMoves, planPan, panPoint, resolveShown, shownAt, newCamera; internal/storyboard/validate.go — Beat.validate; internal/storyboard/camera.go — trackZoom, trackPan, validateCamera, CameraMove.validateForm;
internal/storyboard/edit_list_test.go — TestApply_InsertIntoAnAbsentOrEmptyList_WritesAFlowList; internal/storyboard/edit_test.go — cameraEditBoard; internal/clip/clip.go — cameraAreas;
internal/render/pipeline.go — Pipeline.Path; cmd/photon/lint_test.go — badCameraStoryboard, cameraAreaStoryboard
**Tests:** load of every form; a `pan` list, a `via` key and a list-form `camera:` each fail to
load; render of positions (cell, text, point, held bar, zero gap) and zooms pins the crop centre;
a text target off screen at its start fails under its position; cursor maps through a crop aimed
by a position; lint names an undeclared position area; the "beat zoom" row names `camera.zooms`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard ./internal/compose ./internal/clip`
- `GOMEMLIMIT=2GiB go test ./internal/render ./cmd/photon`
- `go build ./... && go vet ./...`
- `grep -rnE 'CameraMove|PanMove|OwnerCamera|AtCamera|Compositor\.Path|Pipeline\.Path|path\.go|\) Path\(' internal cmd --include='*.go' | grep -v _test` prints nothing, and `test ! -e internal/compose/path.go`
- `grep -rniE '\bpans?\b|\bvia\b|camera move|PanMove|CameraMove|Compositor\.Path' internal/storyboard internal/compose internal/render internal/clip --include=*.go | grep -v _test.go` prints nothing
**Commit:** `feat(storyboard)!: aim the camera with positions and zoom with camera.zooms`

## 8. Storyboard edits for position and zoom fields — ✅ DONE (2026-10-20)

NOTES (2026-10-20): the position key order is the struct order text, nth, area, cell, at, hold, ease (Point leads Position, so its keys sit "at the Point's place" as the guard says), not the guard's parenthetical (at, hold, text, nth, area, cell, ease), which contradicts that wording and the every-form fixture; `positionKeyOrder` is pinned in `TestKeyOrder_StructTags_GiveTheStoryboardKeyOrder` beside a `keyOrder(Position)` row.
NOTES (2026-10-20): `KindPoint` (`{"cell": {...}}`, left by item 3 for the position edits) is removed: no field takes that shape. `position.cell` gets the new `KindCell` (`{"x", "y"}`), `position.span` the new `KindSpan`, and `position.target` reuses `KindTarget`; `position.cell` and `position.target` default to null, which Apply refuses, so they sit in `defaultRefusals` with samples, as do `zoom.over` and `zoom.to` (default 0).
NOTES (2026-10-20): `editName` is now `Owner.prefix() + field`, so a camera owner on a beat or caption field is named by its owner again (`camera.positions[0]: duration: …`), which reverses item 3's note; the existing `edit_test.go` assertions match by substring and still pass.

Depends on item 7.

**What:**
**Goal:** `storyboard.Apply` takes, with owner kind `position` or `zoom` and the entry's list
index: `position.at` (a moment), `position.hold` (whole ms ≥ 0; 0 removes the key),
`position.ease` and `zoom.ease` (`smooth` removes the key; the other three are written),
`position.cell` (`{"x", "y"}`; replaces any `text`/`nth`/`area`), `position.target`
(`{"text", "nth", "area"}` as a pick answers it; replaces any `cell`), `position.span`
(`{"at": <moment>, "hold_ms": n}`, both at once), `zoom.at`, `zoom.over`, `zoom.to`; each changes
only the edited text, is refused when the result fails validation, and `storyboard.Fields`
describes each with the ease field's four choices. The `Edit` doc comment lists them.
**Approach (assumed at the header base):** a new `edit_camera.go` dispatching on the field
prefix like the caption edits, splicing under `camera/positions/<i>` and `camera/zooms/<i>` with
key orders from `keyOrder` (`ZoomMove`'s; the position order per the guard); `ownerKindOf` and the edit label (`camera.positions[i]: <field>`)
follow. Reuse `setMoment` and `decodeMilliseconds`.
**Regression guard.** `position.cell` and `position.target` write the new key before deleting the old ones: a block sequence item's only key cannot be deleted (yamledit `Delete` answers `ErrUnsupported`), and `Insert` lands after the item's first key.
`keyOrder` skips inline fields, so the position key order is `keyOrder(Target)` and `cell` spliced at the Point's place in `Position` (at, hold, text, nth, area, cell, ease), pinned in `TestKeyOrder_StructTags_GiveTheStoryboardKeyOrder`.
Every doc comment in `edit.go` and `doc.go` that enumerates an edit's write rules (`grep -n 'smooth\|deletes\|replaces' internal/storyboard/edit.go internal/storyboard/doc.go`) covers the new fields, `Apply`'s included.
**Files:** internal/storyboard/edit_camera.go, internal/storyboard/edit_camera_test.go, internal/storyboard/edit.go, internal/storyboard/fields.go, internal/storyboard/fields_test.go, internal/storyboard/doc.go, internal/storyboard/edit_test.go
**Read first:** internal/storyboard/edit.go — splicer.apply, ownerKindOf, setMoment, replaceOrInsert, keyOrder, decodeMilliseconds; internal/yamledit/document.go — Delete, Insert; internal/yamledit/upsert.go — Upsert, keysBefore; internal/storyboard/edit_caption.go — editCaption;
internal/storyboard/fields.go — Fields, easeChoices, listEntryFields; internal/storyboard/fields_test.go — defaultRefusals, TestFields_EveryEditableField_RoundTripsThroughApply, TestFields_DescribeEachOwnersFields;
internal/storyboard/edit_test.go — TestKeyOrder_StructTags_GiveTheStoryboardKeyOrder, editBaseBoard, moment; internal/storyboard/located.go — Owner.prefix
**Tests:** per field: changes only the edited text; to-the-default removes the key; wire
spellings refused; an out-of-range index and a wrong owner kind refused; an edit making positions
overlap refused naming both; `TestFields_*` cover the new fields; `position.cell` on a one-key
block position `- text: ok` and `position.target` on a one-key `- cell: {x: 1, y: 1}`; the
position key order row in `TestKeyOrder_StructTags_GiveTheStoryboardKeyOrder`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard`
**Commit:** `feat(storyboard): edit camera position and zoom fields`

## 9. Storyboard: insert and remove positions and zooms — ✅ DONE (2026-10-20)

NOTES (2026-10-20): re-derived from the assumption that position and zoom edits route through `edit.go`'s `splicer.apply`; since item 8 `editPosition` and `editZoom` in `internal/storyboard/edit_camera.go` dispatch them, so the list-field branch (as `editCaption` has) lands there, and `setPositionCell`'s `{x: 3, y: 4}` spelling is extracted there as `cellFlow` so `position.insert` shares it.
NOTES (2026-10-20): `entryList` carries a `holder` path (nil for the top level, `{camera}` for the camera lists), the list's `key`, the holder's key `order` and a `newEntry` func that decodes and spells the insert value; `kinds` is dropped and `decodeEntry` reads the caption kinds directly (its error text is unchanged). The new field kinds are `KindNewPosition` (`"new-position"`) and `KindNewZoom` (`"new-zoom"`); the frame's centre cell is `{cols/2, rows/2}`.
NOTES (2026-10-20): a camera that is present but empty or null (`camera: {}`, `camera:`) is deleted and rewritten as `{positions|zooms: [...]}` after the keys that precede it, because yamledit's Replace cannot splice into a zero-width null node; `zoom.insert`'s default (clip start, 600ms) overlaps every-form.yaml's `camera.zooms[0]`, so it sits in `defaultRefusals` with a sample at 2000ms, while `position.insert`'s default round-trips.

Depends on item 8.

**What:**
**Goal:** `position.insert` (value `{"at": <moment>, "cell": {"x", "y"}}`, a point) and
`zoom.insert` (value `{"at": <moment>}`, written `to: 1.5`, `over: 600ms`) add an entry at the
owner's index (0 through the list's length), creating `camera:` and the list when absent;
`position.remove` and `zoom.remove` (null) remove the owner's entry, deleting an emptied list and
an emptied `camera:`; neither list has a move/reorder field; each changes only the list's text.
**Approach (assumed at the header base):** generalise `entryList` in `edit_list.go` from a
top-level key to a `yamledit.Path` (captions at `captions`, the camera lists at
`camera/positions` and `camera/zooms`); restore the zoom presets beside the list code.
**Regression guard.** `fields_test.go` joins Files: the insert/remove fields grow the per-owner lists `TestFields_DescribeEachOwnersFields` pins (a `defaultRefusals` entry when an insert's default overlaps every-form.yaml's positions).
The inserts take no `kind`, so neither uses `KindEntry`: each has its own `FieldKind` and Default — `position.insert` `{"at": clip start, "cell": frame centre}`, `zoom.insert` `{"at": clip start}` — and `insertField`/`newEntryDefault` stay for captions only.
`internal/storyboard/doc.go` joins Files: its package comment and `edit_list.go` map line name the position and zoom inserts and removes.
**Files:** internal/storyboard/edit_list.go, internal/storyboard/edit_list_test.go, internal/storyboard/edit.go, internal/storyboard/fields.go, internal/storyboard/fields_test.go, internal/storyboard/doc.go
**Read first:** internal/storyboard/edit_list.go — entryList, captionList, insertEntry, decodeEntry, entryFlow, removeEntry, isRemoveField; internal/yamledit/upsert.go — Upsert; internal/yamledit/sequence.go — InsertItem, DeleteItem; internal/yamledit/document.go — Delete;
internal/storyboard/fields.go — insertField, newEntryDefault, listEntryFields, KindEntry; internal/storyboard/fields_test.go — TestFields_DescribeEachOwnersFields, defaultRefusals;
internal/storyboard/edit_list_test.go — TestApply_InsertIntoAnAbsentOrEmptyList_WritesAFlowList, TestApply_RemoveTheOnlyEntry_DeletesTheListKey; internal/storyboard/edit.go — splicer.apply (isNullTaken)
**Tests:** insert into an absent `camera:`, an absent list and a present one; remove the only
entry of each list; an insert overlapping a position refused; out-of-range and mistyped values;
`TestFields_DescribeEachOwnersFields` lists the insert and remove fields per owner, and each
insert's Default round-trips through `Apply`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard ./internal/yamledit`
**Commit:** `feat(storyboard): add and remove camera positions and zooms`

## 10. Storyboard: the transition edit — ✅ DONE (2026-10-20)

NOTES (2026-10-20): `position.transition` is routed in `edit.go`'s `splicer.apply` (before `editPosition`, which lives in `edit_camera.go`, outside this item's Files); `setMoment`'s write half is extracted in `edit.go` as `splicer.writeMoment` so the transition's `at` moves share it.
NOTES (2026-10-20): Fields describes `position.transition` (new kind `KindTransition`, `"transition"`) only on a position with another starting before it, placed after `position.span`; its default is the transition as it stands, `{"start_ms": <previous end>, "end_ms": <own start>}`, which writes nothing. An edge left where it is writes nothing either. A cut right after a point is refused as an overlap, since both positions would start together.

Depends on item 9.

**What:**
**Goal:** `position.transition`, owner the position the transition leads into (N) and value
`{"start_ms": s, "end_ms": e}` (whole output ms, s ≤ e), moves that transition's edges in one
edit: P, the position before N by start, ends at s — a point P moves its `at` there, a held P
changes its `hold`; N starts at e keeping its end — a point N moves its `at`, a held N moves its
`at` and shortens or lengthens its `hold`. Each `at` keeps its beat and shifts its offset. No
other position changes. Refused: N is the first position; s > e; a resulting negative hold;
overlap. s = e is a cut.
**Approach (assumed at the header base):** new `internal/storyboard/edit_transition.go` reading
the resolved starts (`Position.Start()`) from the parsed board, splicing through item 8's
helpers, several splices in one `Apply` as `caption.shift` does.
**Regression guard.** `fields_test.go` joins Files for the same reason (`position.transition`).
**Files:** internal/storyboard/edit_transition.go, internal/storyboard/edit_transition_test.go, internal/storyboard/edit.go, internal/storyboard/fields.go, internal/storyboard/doc.go, internal/storyboard/fields_test.go
**Read first:** internal/storyboard/edit_caption.go — shiftCaption, shiftMoment, decodeShift; internal/storyboard/edit.go — Apply, splicer.apply, replaceOrInsert, momentFlow, decodeMilliseconds; internal/storyboard/clock.go — OutputClock.Resolve;
internal/storyboard/fields.go — Fields; internal/storyboard/fields_test.go — TestFields_DescribeEachOwnersFields, TestFields_EveryEditableField_RoundTripsThroughApply, defaultRefusals; internal/storyboard/doc.go — file map
**Tests:** drag a transition between two held positions; move its start edge onto a point P and
its end edge onto a held N; shrink to a cut; each refusal; only the edited values' text changes;
`TestFields_DescribeEachOwnersFields` lists `position.transition` on its owners.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard`
**Commit:** `feat(storyboard): edit a camera transition's edges`

## 11. Tune state reports positions and zooms — ✅ DONE (2026-10-20)

NOTES (2026-10-20): consequential edit — internal/tune/session.go: made necessary by hasOwner taking positions and zooms; the Apply doc comment named the removed "camera move" as an edit's owner, now "camera position, zoom".
NOTES (2026-10-20): PositionState embeds the kept PointState, so its target or cell is sent at the top level beside the timing (`target`/`cell`, omitempty) and PointState is live again; a new pointState helper spells a storyboard.Point. Both lists are sent as `[]`, never null, when the storyboard has no camera.
NOTES (2026-10-20): beyond the listed tests, TestSession_Refresh_InvalidOutsideWrite_ShowsEachProblemOnAPanel gained three cases (a position and a zoom problem on the shown list land on their owner; a zoom past the shown list lands on the clip), and TestSession_State_WithoutACamera_ReportsEmptyLists pins the empty lists.
NOTES (2026-10-20): no CHANGELOG entry: /api/state is the page's own API and the page shows no camera until items 14-15, whose entries describe it.

Depends on item 10.

**What:**
**Goal:** `GET /api/state` carries `positions` (`index`, `start_ms`, `end_ms`, `at`, `hold_ms`,
`ease`, and `target` or `cell`) and `zooms` (`index`, `start_ms`, `end_ms`, `at`, `over_ms`,
`ease`, `factor`); the session accepts edits owned by a position or zoom it holds; a storyboard
with positions loads, takes a position edit through `POST /api/edit`, undoes, and saves back
changing only the edited value.
**Approach (assumed at the header base):** `PositionState`/`ZoomState` in `state.go` built from
`board.Camera`, `hasOwner` cases for `OwnerPosition`/`OwnerZoom`; fields reach the page through
the existing `state.fields`.
**Files:** internal/tune/state.go, internal/tune/session_test.go, internal/tune/server_test.go, internal/tune/doc.go
**Read first:** internal/tune/state.go — State, Session.State, hasOwner, ownerOn, problemsOf, momentState, TargetState; internal/tune/session_test.go — newSessionFixtureOf, sessionStoryboard;
internal/tune/server_test.go — post, decodeResponse; internal/tune/edit.go — Edit; internal/tune/page_test.go — TestPage_EntryFields_AreEditFields
**Tests:** state reports both lists; handler applies position and zoom edits as the page sends
them; save round trip; a text target off screen shows its problem on that position.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune`
**Commit:** `feat(tune): report camera positions and zooms`

## 12. Compose and render: camera view, full frame and position aim — ✅ DONE (2026-10-20)

NOTES (2026-10-20): consequential edit — internal/compose/doc.go: made necessary by the full view (`ComposeFull`, the full-view Cell/Rect variants) in compose.go and `View`/`PositionCell` in camera.go; the file map now names them.
NOTES (2026-10-20): consequential edit — internal/render/doc.go: made necessary by the Pipeline's new `FullFrameAt`, `ViewAt` and `PositionCell`; the package comment now says what the pipeline answers of the full view.
NOTES (2026-10-20): API shape chosen here: `Compositor.ComposeFull(t, src)`, `Compositor.FullViewCell(p)` and `FullViewRect(b)` (no time: the full view's mapping never depends on it), `Compositor.ViewAt(t) View` and `Compositor.PositionCell(index) (col, row, ok)`; `View` is `{Crop FloatRect; Centre image.Point; Origin, CellSize FloatPoint}` in full-view pixels, the two float types exported beside it in camera.go. Pipeline mirrors each (`FullFrameAt`, `FullViewCell`, `FullViewRect`, `ViewAt` returning `(View, ok)`, `PositionCell`), reporting nothing on a partial pipeline.
NOTES (2026-10-20): the cell lookup Cell did inline is now `Layout.cellAt`, shared by Cell, FullViewCell, the view's centre and PositionCell; Cell's and Rect's bodies moved into `cellThrough`/`rectThrough` taking the crop, and `compose` now takes its crop, so Compose, ComposeDraft and ComposeFull differ only in crop and scaler. The camera keeps each position's aim in list order (`camera.aims`, filled in `newCamera`).
NOTES (2026-10-20): `make lint` still fails only on the pre-existing internal/tune/pagejs_test.go:1068 ineffassign noted under item 5.

Depends on item 7.

**What:** Recast at the regression check (2026-10-19).
**Goal:** a render `Pipeline` answers, for output time t: the full-view exact frame (composed
with the whole frame as the crop, cursor and captions drawn as usual, at the output size); the
camera view (the crop's rectangle in full-view output pixels, `centre` — the cell holding the
position track's pre-clamp value at t — and the grid's origin and cell size in full-view output
pixels); and, for position i, the cell its aim resolves to. Draft frames are unchanged.
**Approach (assumed at the header base):** compose gains `ComposeFull` and a view accessor that
reads the crop from `crop(t)` and `centre` from `camera.centreAt(t)` beside it, never from the
crop (at factor ≤ 1 `cropAround` gives the whole frame); the planner keeps each position's
resolved aim by list index (as the old `waypoints` map was kept); render's `Pipeline` passes them
through beside `FrameAt`.
**Regression guard.** The pipeline also maps cells and boxes through the full view (full-view variants of Cell and Rect beside the existing ones), so a pick made on the full view resolves the cells under the pointer.
The camera view's `centre` is the cell holding the position track's value at t BEFORE the per-frame clamp (during a hold, that position's aim; mid-glide, the glided point; before the first / after the last, that position's aim; no positions, the frame's centre); it replaces the earlier clamped-crop-centre wording, so a point placed there keeps where later zooms centre.
**Files:** internal/compose/compose.go, internal/compose/camera.go, internal/compose/compose_test.go, internal/compose/camera_test.go, internal/render/pipeline.go, internal/render/pipeline_test.go
**Read first:** internal/compose/compose.go — Compositor.crop, cropAround, Compositor.compose, Compositor.Cell, Compositor.Rect, Layout.cellCentre, Layout.grid, Output; internal/compose/camera.go — camera.centreAt, camera.factorAt, zoomPlanner.plan;
internal/render/pipeline.go — Pipeline.FrameAt, Pipeline.Cell, Pipeline.Rect; internal/compose/compose_test.go — newTestCompositor, parseBoard, directCompose; internal/render/pipeline_test.go — loadRenderTake, oracleFrames
**Tests:** full frame equals a factor-1 composition; crop rectangle at a zoomed, clamped moment;
`centre`, each case failing against a clamped-crop definition: inside an off-centre hold > 0 at
1x, the aim's cell, not the frame centre's; at a clamped, zoomed hold, the aim's cell, not the
crop centre's; mid-glide, the glided point's cell; before the first and after the last, that
position's aim cell; no positions, the frame's centre cell; aim of a cell and of a text position;
full-view Cell and Rect at a zoomed moment resolve the cell and box under a full-view pixel.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/compose ./internal/render`
**Commit:** `feat(render): expose the camera view, the full frame and position aims`

## 13. Tune: camera route and full-view frames — ✅ DONE (2026-10-20)

NOTES (2026-10-20): API shape chosen here: `Session.FullFrame(out)` (exact PNG of the full view) and `Session.Camera(out, position *int) (CameraView, error)` in frame.go, `CameraView{View compose.View; Aim *image.Point}` with nil Aim when no position is asked, so the view and the aim come from one good pipeline; `ErrNoPosition` (422 through statusOf) for an index the list lacks. `Camera` checks t against the schedule itself (`render.ErrOutsideClip`), since `Pipeline.ViewAt` does not. session.go needed no change.
NOTES (2026-10-20): `Session.Pick` gains a `FrameView` argument (`ViewCamera`, `ViewFull`) between the time and the corners; the pick body carries it as `"view": "full"` (absent for the camera's frame), anything else 400. `/api/camera` answers `aim` only with `?position=`; a position that is not a whole number of 0 or more is 400, an index past the list 422.
NOTES (2026-10-20): the newHandler doc comment named a "path" the session cannot give (the path route item 2 removed); its rewrite for the camera route drops it.
NOTES (2026-10-20): no CHANGELOG entry: /api/camera and view=full are the page's own API, which the page uses from items 14-18 on, whose entries describe the feature.
NOTES (2026-10-20): `make lint` still fails only on the pre-existing internal/tune/pagejs_test.go:1068 ineffassign noted under item 5.

Depends on items 11, 12.

**What:** Recast at the regression check (2026-10-19).
**Goal:** `GET /api/camera?t=<ms>` answers `{"centre": {"x","y"}, "crop": {"x","y","width",
"height"}, "grid": {"x","y","cell_width","cell_height"}}` and with `&position=<i>` also
`"aim": {"x","y"}`; `GET /api/frame?t=<ms>&view=full` answers the full-view exact frame;
bad queries answer 400, an unknown position 422.
**Approach (assumed at the header base):** session methods over `goodPipeline()` beside `Frame`;
route constants and handlers beside `routeFrame` in `server.go`.
**Regression guard.** `POST /api/pick` accepts the view as `view=full` (the frame route's spelling) and maps both the drag's cells and `Proposal.Rect` through the full view; its test fixture adds a `camera.zooms` entry covering the pick's t and asserts the cell and Rect differ from the crop mapping.
Answers: `view` is "" or "full", anything else 400 (frame and pick); `view=full` with `draft=1` is 400; `/api/camera` with t outside [0, length) is 422 (`render.ErrOutsideClip`) like `/api/frame`; an unknown position is 422 through a new sentinel in `statusOf` (item 2 deleted `ErrNoPath`).
**Files:** internal/tune/server.go, internal/tune/frame.go, internal/tune/session.go, internal/tune/server_test.go, internal/tune/frame_test.go, internal/tune/doc.go, internal/tune/pick.go, internal/tune/pick_test.go
**Read first:** internal/tune/server.go — newHandler, handleFrame, frameTimeOf, frameDraftOf, statusOf, pickRequest, handlePick, responseOf; internal/tune/frame.go — Session.Frame, Session.goodPipeline, encodeFrame; internal/tune/pick.go — Session.Pick, Proposal;
internal/tune/server_test.go — get, assertStatus, pickBody, TestHandler_Frame_BadDraft_IsRefused, TestHandler_Pick_ReturnsTheProposal;
internal/tune/pick_test.go — pickStoryboard, newPickSession, cellPixel
**Tests:** handler tests driving the exact query strings the page sends (`t`, `position`,
`view=full`); each answer above (unknown `view`, `view=full&draft=1`, t outside the clip on
`/api/camera`, an unknown position); a pick handler test with `view=full` under a zoom whose
cell and Rect differ from the crop mapping.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune`
**Commit:** `feat(tune): serve the camera view and full-view frames`

## 14. Tune page: zoom lane and inspector — ✅ DONE (2026-10-20)

NOTES (2026-10-20): the regression guard's split was needed (the zoom kind took app.js to 402 lines in place), so the entry-kind table (ENTRY_KINDS, one row per kind: its state list and its beat-select name) and the lookups entryValue, entryOfValue, isEntryKind, entryOf and entryOptions moved to the new split script internal/tune/page/kinds.js. It is loaded right after app.js (before controls.js, which destructures entryValue when it loads) and is listed in page_test.go's splitScripts. app.js's ownerKey, takeState's selection drop, renderBeatSelect, renderInspector and selectValue all read that one table. app.js is now 369 lines.
NOTES (2026-10-20): re-derived from the Read-first anchor TestPageJS_RefusedBarDrag_RedrawsInPlaceAndShowsTheRefusal, which item 1 deleted. Its shape (and its barLeft helper) comes back as TestPageJS_RefusedZoomDrag_RedrawsInPlaceAndShowsTheRefusal on a new zoomedStoryboard fixture.
NOTES (2026-10-20): consequential edit — internal/tune/doc.go: made necessary by the new page/kinds.js (file map entry) and by entries.js now holding the zoom panel too.
NOTES (2026-10-20): consequential edit — internal/tune/page/controls.js: made necessary by the zoom entry kind and kinds.js (comments only: load order, and owners can now be zooms as well as captions).
NOTES (2026-10-20): page_test.go also gains {"zoom", ZoomState{}} in entryReaders, so every zoom.<key> the page reads is checked against /api/state's keys. The zoom panel shows a read-only "plays" row and a Remove-only action row; zooms have no move buttons because render places zooms by their start times. listRow's remove button became removeButton, which both the caption and the zoom panels use.

Depends on item 11.

**What:**
**Goal:** the page has a **zoom** lane drawing one bar per `state.zooms` entry labelled with its
factor; its `+` sends one `zoom.insert` at the playhead; dragging a bar sends one `zoom.at` edit
keeping the beat, a refused drag redraws in place; selecting a bar opens a zoom panel editing
`at`, `over`, `to`, `ease` (four choices from `state.fields`) with a remove button; a zoom's
problem shows on its panel.
**Approach (assumed at the header base):** `lanes.js` lane entry and bars from `state.zooms`;
`entries.js` zoom panel through `controls.js` fields; `app.js` entry kind `zoom`; `timeline.js`
shift edit for zooms.
**Regression guard.** `page_test.go` joins Files; app.js stays under page_test.go's 400-line cap: if item 14's additions would pass it, first move the entry-kind table and lookup/selection helpers out of app.js into a new split script listed in `splitScripts` and `index.html`; items 15, 17 and 18 put their logic in lanes.js, entries.js, marker.js and frames.js, adding to app.js only wiring.
The zoom kind reaches every app.js site keyed by entry kind — `ownerKey` (else a refused zoom 0 edit keys as "0", the Clip frame's), `entryOf`, `renderBeatSelect` and `takeState`'s selection drop — all driven by one entry-kind list.
**Files:** internal/tune/page/lanes.js, internal/tune/page/entries.js, internal/tune/page/app.js, internal/tune/page/timeline.js, internal/tune/page/style.css, internal/tune/pagejs_test.go, internal/tune/page_test.go, internal/tune/page/index.html and the new split script (only when the split is needed)
**Read first:** internal/tune/page/lanes.js — adding, window.tuneLanes; internal/tune/page/timeline.js — shiftEdit, barNode, clipLaneRow; internal/tune/page/app.js — ownerKey, entryOf, renderBeatSelect, takeState; internal/tune/page/controls.js — choiceField, errorRows, fieldsOf, problemOwner;
internal/tune/pagejs_test.go — TestPageJS_EditProblem_ShowsOnItsOwningPanel, dragBar, entryValueOf, TestPageJS_RefusedBarDrag_RedrawsInPlaceAndShowsTheRefusal; internal/tune/page_test.go — TestPage_SplitScripts_StayBelowTheFileSizeGuide, registeredLanes
**Tests:** goja: add a zoom at the playhead; drag a zoom bar; four eases offered; remove; a zoom
problem on its panel and not the Clip frame's (restores the camera subtest of
`TestPageJS_EditProblem_ShowsOnItsOwningPanel`).
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPage'`
**Commit:** `feat(tune): add the zoom lane`

## 15. Tune page: position lane with transition bars and inspectors — ✅ DONE (2026-10-20)

NOTES (2026-10-20): re-derived from the assumption that the transitions are derived in lanes.js: the derivation (transitions, with byStart and aimText) lives in kinds.js, which loads before lanes.js, because the transition entry kind's lookups (entryOf, the selection drop in takeState, the beat select's options) need it as well; lanes.js reads it through window.tunePage. Each transition mirrors the storyboard's positionBefore: it runs from the end of the latest position starting strictly earlier (the first listed among ties) to its into-position's start.
NOTES (2026-10-20): ENTRY_KINDS rows now carry entries(state) in place of a state list name, so the transition kind can list derived entries; the beat select offers positions and transitions ahead of zooms and captions. The selection kind "transition" carries the into-position's index, and every transition-panel edit (position.transition, position.ease) is owned by that position, so its refusals and problems show there.
NOTES (2026-10-20): the position lane has no add button (item 17 adds `+`), so timeline.js's clipLaneRow draws an add button only for a lane that has add; barNode honours a bar's draggable: false (no window.tuneDrag attach, no "draggable" class) until item 16.
NOTES (2026-10-20): the transition panel's start field moves the whole transition keeping its length, and its length field keeps its start; both send {start_ms, end_ms}. A position's cell edits through controls.js's pixelsField ({x, y}); its text target sends the whole target from targetInputs/targetOf whenever text, nth or area changes.
NOTES (2026-10-20): EditProblem gains a "position" subtest (a refused hold past the next position's start shows on that position's panel and not on another position's).
NOTES (2026-10-20): consequential edit — internal/tune/page/controls.js: made necessary by the position panels (comments only: pixelsField now also edits a grid cell, and owners can be positions).
NOTES (2026-10-20): consequential edit — internal/tune/doc.go: made necessary by kinds.js and entries.js now holding positions and transitions (file map lines).
NOTES (2026-10-20): page_test.go's registeredLanes and entryReaders do not list the position lane or a position reader (page_test.go is outside this item's Files, and jsonKeys does not follow PositionState's embedded PointState); timeline.js is 468 lines, over the ~400-line guide it already passed before this item.

Depends on item 14.

**What:**
**Goal:** the page has a **position** lane: each position draws as a bar over its hold, or as a
point when its hold is 0; each gap between consecutive positions by start draws as a
**transition** bar in its own colour attached to both. A position or a transition can be
selected. The position panel edits `at`, `hold`, and its cell (`x`, `y`) or text target (`text`,
`nth`, `area`), with a remove button; the transition panel shows its start and length (editable
through `position.transition`) and its ease (four choices, edited as the into-position's
`position.ease`). Problems show on the owning position's panel.
**Approach (assumed at the header base):** transitions derived in `lanes.js` from
`state.positions` sorted by `start_ms`; selection kind `transition` carries the into-position's
index; panels in `entries.js`; colour `--lane-transition` in `style.css`.
**Regression guard.** `timeline.js` joins Files: `barNode` attaches a `shiftEdit` drag to every registered-lane bar, so position and transition bars carry a `draggable: false` bar flag `barNode` honours until item 16.
The position and transition kinds' table lives outside `app.js` (item 14's guard), adding to `app.js` only wiring, so it stays under the 400-line cap.
The transition panel ends with `errorRows` of its into-position's owner, so a refused `position.transition` edit (standing under `position:N`) shows there.
**Files:** internal/tune/page/lanes.js, internal/tune/page/entries.js, internal/tune/page/app.js, internal/tune/page/style.css, internal/tune/pagejs_test.go, internal/tune/page/timeline.js
**Read first:** internal/tune/page/timeline.js — barNode, shiftEdit, span, markSelection, entryKey; internal/tune/page/lanes.js — captionBars, adding; internal/tune/page/app.js — entryOf, takeState, renderInspector, selectEntry, ownerKey; internal/tune/page/entries.js — entryPanel, listRow;
internal/tune/page/controls.js — momentField, choiceField, targetInputs, targetRows, targetOf, errorRows; internal/tune/page/style.css — .block.caption; internal/tune/pagejs_test.go — dragBar, barLeft, selectValue
**Tests:** goja: bars, points and transitions drawn for positions given out of order; selecting
each; each panel edit sends one edit of the named field; a refused transition-length edit shows
on the transition panel; dragging a position or transition bar sends no edit.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPage'`
**Commit:** `feat(tune): add the position lane with transition bars`

## 16. Tune page: drag positions and transitions

Depends on item 15.

**What:**
**Goal:** on the position lane, dragging a position sends one `position.at` (beat kept);
dragging its end edge sends one `position.hold` and its start edge one `position.span` keeping
its end; a point draws an end grip, dragging it right opens it into a bar; dragging a transition
sends one `position.transition` with both edges shifted, resizing one of its edges one with that
edge moved; shrinking to 0 is allowed; a refused edit redraws the bar in place and shows the
refusal. Each is one undoable edit.
**Approach (assumed at the header base):** `timeline.js` bar and edge handlers over `drag.js`,
as the beat end drag does.
**Files:** internal/tune/page/timeline.js, internal/tune/page/lanes.js, internal/tune/page/drag.js, internal/tune/pagejs_test.go
**Read first:** internal/tune/page/timeline.js — barNode, handleNode, handleExtent, barExtent, shiftEdit; internal/tune/page/drag.js — attach, startDrag, endDrag, defersRedraw; internal/tune/page/app.js — edit, ownerKey;
internal/tune/pagejs_test.go — dragBar, dragBeatsHandle, editCount, barLeft, TestPageJS_RefusedBarDrag_RedrawsInPlaceAndShowsTheRefusal
**Tests:** goja: move, resize hold (both edges), open a point, drag a transition, resize each
transition edge, shrink to a cut, a refused edit snapping back.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPage'`
**Commit:** `feat(tune): drag positions and transitions`

## 17. Tune page: add a position on the camera's centre

Depends on items 13, 16.

**What:**
**Goal:** the position lane's `+` asks `GET /api/camera?t=<playhead ms>` and sends one
`position.insert` of a point at the playhead on the answer's `centre` cell, so adding one does
not move the camera.
**Approach (assumed at the header base):** the lane's `add` in `lanes.js` with an async fetch
through `app.js`'s request helper.
**Regression guard.** `timeline.js` joins Files: `addButton` awaits `add.edit` (`Promise.resolve`), selects the owner after, and sends a failed camera fetch to `page.reportStatus` like a refused edit. `request` is a closure in `app.js`, so lanes.js reaches it through a named member (e.g. `tune.camera(ms)` or `tunePage.request`), and app.js's contract header comment says so.
The playhead ms is rounded once (`Math.round`, as `adding` does for `offset_ms`) and that one ms is both the `t` query (`frameTimeOf` parses an integer) and the insert's `at`.
`+` uses item 12's pre-clamp `centre` as the route answers it. An insert inside a hold overlaps it and is refused, so the held-position case holds the LAST position with the playhead after its hold.
**Files:** internal/tune/page/lanes.js, internal/tune/page/app.js, internal/tune/pagejs_test.go, internal/tune/page/timeline.js
**Read first:** internal/tune/page/timeline.js — addButton, clipLaneRow, msAt; internal/tune/page/lanes.js — adding, window.tuneLanes; internal/tune/page/app.js — request, edit, window.tune, page (tunePage); internal/tune/page/transport.js — tick, seek, clampTime;
internal/tune/pagejs_test.go — loadPage, lastRequest, TestPageJS_RefusedLaneAdd_ShowsInTheStatusUntilAnAcceptedEdit
**Tests:** goja: `+` sends the insert with the centre the camera route answers; the LAST position
an off-centre held cell at 1x and the playhead after its hold: the inserted cell is that
position's aim cell, not the frame centre; a refused add shows in the status; `+` at a
fractional playhead asks an integer `t` and inserts at that same ms; a failed camera fetch shows
in the status.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPage'`
**Commit:** `feat(tune): add a camera position at the playhead`

## 18. Tune page: drag a position's cell on the preview

Depends on items 13, 16.
**Closes:** tune-position-marker

**What:** Recast at the regression check (2026-10-19).
**Goal:** selecting a position seeks the preview to its start and shows the full-view exact
frame (`view=full`) with the crop rectangle and a marker on the position's aim cell from
`GET /api/camera?t=&position=`; dragging the marker moves it cell by cell over any cell, clamped to
the grid; dropping sends one `position.cell` edit and the rectangle follows from a fresh camera
answer. Draft frames while playing or scrubbing are the normal view with the overlay hidden.
Selecting anything else returns to the normal view.
**Approach (assumed at the header base):** a new `internal/tune/page/marker.js` overlay beside
the pick box (its `<script>` and element in `index.html`), frame requests in `frames.js` carrying
`view=full` while a position is selected, scaled by `fit.js` like the pick box.
**Regression guard.** (keeps the ratified intent) frames.js's frame cache key includes the view, and every selection change that flips the view re-pumps the still frame (`settleFrames`), so the full view is fetched even when the normal exact frame at the position's start already shows; `picker.js` and `page_test.go` join Files, marker.js listed in `splitScripts`.
frames.js exposes the shown frame's view beside `frameTime` (e.g. `frameView()`): the overlay draws only while that view is full, and the pick sends `view=full` from it, not from the selection (a settle's draft-first frame is the normal view).
**Files:** internal/tune/page/marker.js, internal/tune/page/index.html, internal/tune/page/frames.js, internal/tune/page/fit.js, internal/tune/page/app.js, internal/tune/page/style.css, internal/tune/pagejs_test.go, internal/tune/page/picker.js, internal/tune/page_test.go
**Read first:** internal/tune/page/frames.js — frameKey, pumpStill, pumpMoving, settleFrames, request, frameTime; internal/tune/page/app.js — selectEntry, select, renderSelection, window.tune; internal/tune/page/picker.js — pick, naturalPerDisplayed, outputPoint;
internal/tune/page/path.js — wantedPath, requestPath, place, queueRender; internal/tune/page_test.go — splitScripts, TestPage_SplitScripts_StayBelowTheFileSizeGuide;
internal/tune/pagejs_test.go — exactFrame, draftFrame, framesAfter, holdFrames, pickAcross
**Tests:** goja: selecting asks the full view and draws rectangle and marker; a text target's
marker on its aim; dragging onto a blank cell and dropping sends one edit; selecting a zoom
returns to the normal view; a pick with the full view shown sends `view=full`; selecting a
position while the playhead already sits at its start fetches the full view; during a settle's
draft-first frame with a position selected, the overlay is hidden.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPage'`
**Commit:** `feat(tune): aim a position by dragging its cell on the preview`

## 19. Tune picker sets a position's text target

Depends on item 15.
**Closes:** tune-picker-position

**What:**
**Goal:** the picker's **Set as position target** button sends one `position.target` edit with
the proposed target to the selected position; it is disabled with a title saying to select a
position when none is selected; picking for action targets is unchanged; a pick proposal round
trips through the edit for a last-match target.
**Approach (assumed at the header base):** `picker.js` button wired to `tune.selectedEntry()`,
`index.html` element; the Go round-trip test in `pick_test.go`.
**Regression guard.** The button is enabled while the full view shows, so its pick goes with `view=full` (items 13 and 18) and maps through the full frame, not the zoomed crop; a Go test picks on the full view under a zoom.
`select`/`selectEntry` fire no listener (`tune.onState` fires only on new state), so the button re-evaluates its disabled state on document click/change as the deleted `path.js` `wire()` did, or by a `requestAnimationFrame` check as `timeline.js` `follow()` does; a goja test selects a position and finds the button enabled.
**Files:** internal/tune/page/picker.js, internal/tune/page/index.html, internal/tune/pagejs_test.go, internal/tune/pick_test.go
**Read first:** internal/tune/page/picker.js — showProposal, pickProposal, report, wire; internal/tune/page/index.html — #pick-result, .pick-actions; internal/tune/pick.go — Session.Pick, Proposal; internal/compose/compose.go — Compositor.Cell, Compositor.Rect;
internal/tune/pick_test.go — pickStoryboard, TestSession_LastMatchTarget_RoundTripsThroughTheProposalAndItsApply; internal/tune/pagejs_test.go — pickAcross; internal/tune/page/app.js — select, selectEntry, renderSelection
**Tests:** goja: the edit, the disabled button, select-then-enabled; Go: proposal applies as a
position target; a pick on the full view under a zoom.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune`
**Commit:** `feat(tune): set a position's text target from the picker`

## 20. Docs: tune, help text and package map name positions

Depends on items 18, 19.

**What:**
**Goal:** README's `photon tune` section, Commands table and Features bullet describe the
position and zoom lanes, transitions, the marker and **Set as position target**;
`photon tune --help` and `photon render --help` name positions, not moves; Note `package-layout`
names positions where it names pans; no non-test Go comment, README line, or Note names a pan
move or via path. The CHANGELOG entry (the item's sidecar) states that the camera is now
`camera.positions` and `camera.zooms`.
**Approach (assumed at the header base):** find every site with
``grep -rniE '\bpans?\b|`via`|via path|\bvia:|camera move' README.md cmd internal .capcom/knowledge --include='*.go' --include='*.md' | grep -v _test.go``.
**Regression guard.** The grep matches `via` only as the schema key or path, so README's Features "via ffmpeg" line stays. It binds over Files: every site it prints is rewritten, Files being a starting list (`internal/check/doc.go` "drags and pans", `internal/tune/pick.go` "a pan's camera.to", `internal/storyboard/edit_caption.go` "camera move").
The CHANGELOG sidecar entry explicitly supersedes the Unreleased bullets describing the pan lane and pan moves, via paths and the via-path overlay, the pick's "Set as pan target", camera-move problems on the camera move panel, and the `camera:` list; items 1–3 deliberately leave those bullets stale until then.
**Files:** README.md, cmd/photon/tune.go, cmd/photon/render.go, .capcom/knowledge/package-layout.md, internal/target/doc.go, internal/caption/doc.go, internal/check/doc.go, internal/tune/pick.go, internal/storyboard/edit_caption.go
**Read first:** README.md — photon tune section, Commands table, Features bullets; cmd/photon/tune.go — newTuneCommand; cmd/photon/render.go — newRenderCommand; cmd/photon/readme_test.go — TestREADMEDocumentsEveryField, TestREADMEDocumentsEveryCommand;
.capcom/knowledge/package-layout.md — internal/target row; internal/target/doc.go; internal/caption/doc.go; internal/check/doc.go
**Tests:** `cmd/photon` README tests.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./cmd/photon`
- `capcom check`
- the grep above prints nothing
**Commit:** `docs: describe the position lane, marker and picker`
