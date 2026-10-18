# Open issues plan

**Goal:** Fix the stuck scrub flag and the zoom target bleed, benchmark a draft frame, and make
the pan track the camera's only aim by removing zoom targets and beat zoom.
**Date:** 2026-10-18
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** fa98f74

**Regression check (2026-10-18, fa98f74):**
- 1: guard folded at 125a8ee (window pointer events in the DOM stub; axis track kept in a variable, no `querySelector`)
- 2: guard folded at 125a8ee (benchmark builds its own 135×44 project and serves directly; `*testing.T` helpers and the 20×4 fixture take do not fit)
- 3: guard folded (1× and alpha-at-2× rows rewritten; test (b) asserts the clamped start; stale-start comment rule, `doc.go` added)
- 4: guard folded (`camera.target` exempted in `TestPage_EntryFields_AreEditFields` until item 8; `SNIPPET_ZOOM` kept; page grep in Acceptance)
- 5: guard folded (writer's decision: `doc.go`, pan-target test rewrites; camera fixture kept out of `sessionStoryboard`; named tests; refusal on a zoom-track overlap; grep in Acceptance)
- 6: guard folded (fixture via `parseBoard` with a cell pan; `TestZoomRampsShrinkToFitTheSection` deleted here; Files corrected)
- 7: guard folded (writer's decision: tune tests in Acceptance; drag texts kept for the cursor tests; auto-fit test, golden test file and beat-zoom helpers deleted; Goal grep)
- 8: guard folded (writer's decision: `page_test.go` exemption removed; dead target helpers dropped; comment rule, `doc.go`; named `fields_test.go` rows; grep in Acceptance)
- 9: guard folded (writer's decision: compose/render/tune tests in Acceptance; presence-only `target` key; `fields_test.go` camera[5] row; comment rule)
- 10: guard folded (writer's decision: compose/render/tune tests in Acceptance, in place of the report's `go vet ./...`; owner `beat <id>`; presence-only `zoom` key; `edit_test.go`, `clip_test.go`, `validBase` rows; comment rule)
- 11: guard folded (writer's decision: CHANGELOG sidecar names every Unreleased zoom bullet; removed-form grep; glossary Beat and Target; zoom rule over docs and help)
- 8: rejected — TestKeyOrder's "a zoom" row (edit_test.go:406) goes red at item 8; `type Zoom` stays until item 10, whose guard drops the row

**Sources:** `.capcom/issues/` — `tune-scrub-stuck-flag.md`, `tune-preview-playback-speed.md`,
`tune-page-js-check.md`, `zoom-target-bleed.md`, `zoom-target-removal.md`, `beat-zoom-removal.md`

**Ratified design calls (Airic Lenz, 2026-10-18):**
- **Scope:** manual checks of tune-preview-playback-speed and tune-page-js-check stay with the user.
- **Pan start:** a pan path starts from the crop centre visible at its start (frame centre at 1×, else the clamped centre); a pan at 1× still sets where the next zoom aims.
- **Zoom target:** removed; `target:` on a camera zoom is a load error under `camera[i]` naming a pan move as the replacement.
- **Beat zoom:** removed (breaking v0.1.0); `zoom:` on a beat is a load error under the beat (`beat <id>`) naming camera zoom and pan moves; CHANGELOG records the break.

**Standing requirements:**
- skills: coding-standards
- Pi 5 host: single packages with `GOMEMLIMIT=2GiB`; never `./...` with `-race`; one test command at a time.

**Left to the user (not run by this plan):**
- `tune-preview-playback-speed`: judge the fps by eye on the 135×44 clip; close with item 2's number.
- `tune-page-js-check`: parse criterion met by `TestPage_Scripts_Parse`; run its manual session, log, close.

**Out of scope:**
- Draft/exact frame caching or encoding; any auto-migration of old storyboards.

## 1. End a scrub on any pointer release — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the DOM stub now bubbles body → document → window (bubbleParent in testdata/dom.js), so the existing scrub test's move/up dispatched on the track are heard on the window unchanged; scrubAxis itself was not modified.
NOTES (2026-10-18): no CHANGELOG entry: the stuck flag came with the draft-frame preview (68a294f), which is still unreleased under [Unreleased], so no shipped version had the defect.

**What:**
Fix for a defect from `68a294f` (feat(tune): show draft frames while the preview moves), filed
as `tune-scrub-stuck-flag`: a scrub ends only through `pointerup`/`pointercancel` on the axis
track, so when the timeline is rebuilt mid-drag and the pointer is released off the new track,
both `isScrubbing` flags (timeline.js and transport.js) stay set and the preview never settles.

**Goal:** a scrub ends on a pointer release or cancel anywhere in the window, also after the
axis track was rebuilt during the drag: both scrub flags clear and the exact frame at the
playhead is requested.

**Approach (assumed at the header base):**
- `internal/tune/page/timeline.js`: `startScrub` registers `pointermove`, `pointerup` and
  `pointercancel` listeners on `window`; `endScrub` removes them. `axisRow`'s track keeps only
  its `pointerdown` listener.
- `moveScrub` measures `clientX` against the axis track currently in the timeline (looked up
  at each move, since a rebuild replaces the node), not the node the scrub started on.
- `endScrub` stays guarded by `isScrubbing`, so a release seen twice settles once.
- Update the file's header comment where it describes how a scrub ends.

**Regression guard.** The DOM stub's bubbling stops at `body` (`testdata/dom.js` walks `parentNode`),
so window listeners never hear `scrubAxis`'s track events: make the stub bubble body → document →
window as a browser does, or make `scrubAxis` send move/up/cancel through `testDOM.dispatch("window", …)`.
The stub has no `querySelector`: keep the current axis track in a variable `axisRow` sets (or use `querySelectorAll(".axis")[0]`).

**Files:** `internal/tune/page/timeline.js`, `internal/tune/pagejs_test.go`, `internal/tune/testdata/dom.js`
**Read first:** `internal/tune/page/timeline.js` — axisRow, startScrub, endScrub; `internal/tune/page/transport.js` — beginScrub;
`internal/tune/pagejs_test.go` — scrubAxis, TestPageJS_Scrub_RequestsDraftsAndItsReleaseTheExactFrame;
`internal/tune/testdata/dom.js` — EventTarget.dispatchEvent, testDOM.dispatch

**Tests:** in `internal/tune/pagejs_test.go`, a goja page test that presses on the axis
(`scrubAxis`), rebuilds the timeline so the axis track is a new node, dispatches `pointerup` on
the window (not on the track), then asserts the exact frame at the release time is requested and
a later play/pause cycle is not held on draft frames. Existing scrub tests stay green, among them
`TestPageJS_Scrub_RequestsDraftsAndItsReleaseTheExactFrame` with its move/up now heard on the window.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPageJS_Scrub|TestPage_' -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/tune -count=1`

**Closes:** tune-scrub-stuck-flag

**Commit:** `fix(tune): end a scrub on any pointer release`

## 2. Benchmark a draft frame request — ✅ DONE (2026-10-18)

NOTES (2026-10-18): benchmark is self-contained per the regression guard (own photon.yaml, 135×44 board, 64-snapshot take in b.TempDir, clip.Load + NewSession + newHandler, ServeHTTP directly); the shared *testing.T helpers are untouched.
NOTES (2026-10-18): measured ~148 ms/op on the Pi 5 (147.7 ms at -benchtime 20x, 149.1 ms at 64x, 24 MB/op) — over the ticket's 100 ms criterion; logged as measured on tune-preview-playback-speed, no tuning, ticket stays open.

**What:**
`tune-preview-playback-speed` is left open only on its on-host acceptance; this item measures
its third criterion ("a draft frame request on the clip above answers in well under 100 ms on
the Pi 5 host"). It does not close the ticket.

**Goal:** `internal/tune` carries a benchmark of `GET /api/frame?t=<ms>&draft=1` on a take of
the ticket's geometry (135×44 terminal, `font_size: 30`, `scale: 2`, `width: 1250`), and its
measured time per request on this host is logged on `tune-preview-playback-speed`.

**Approach (assumed at the header base):**
- A `BenchmarkHandler_DraftFrame` in `internal/tune/server_test.go`, built on the session fixture
  helpers (`newSessionFixtureOf`) with a storyboard and take of that geometry, many distinct
  snapshots generated in the test.
- Each iteration requests a time whose snapshot is not in the draft raster cache (cycle through
  more distinct snapshots than the cache holds), so it measures the cold path a playing preview
  pays; fixture setup sits outside the timer (`b.ResetTimer`).
- Run it on this host and log the result:
  `capcom log tune-preview-playback-speed "draft frame benchmark (135x44, Pi 5): <ns/op> …"`.
- A result over 100 ms is logged as measured; no tuning in this item.

**Regression guard.** `newSessionFixtureOf`, `get`/`serveRequest`/`frameOf` take `*testing.T`, and the fixture's 20×4 take is refused by
`checkFrame` on a 135×44 board: the benchmark writes its own `photon.yaml` (`sessionConfig`), 135×44 board and many-snapshot take into
`b.TempDir()`, runs `clip.Load` + `NewSession` + `newHandler` itself and serves with `httptest.NewRequest`/`ServeHTTP` directly
(or widens those helpers to `testing.TB` and lets the fixture take a take writer).

**Files:** `internal/tune/server_test.go`, `.capcom/issues/tune-preview-playback-speed.md`
**Read first:** `internal/tune/server_test.go` — serveRequest; `internal/tune/session_test.go` — newSessionFixtureOf, saveSessionTake, lineSnapshot, sessionConfig;
`internal/tune/server.go` — handleFrame; `internal/tune/cache.go` — draftCacheBytes; `internal/render/pipeline.go` — checkFrame

**Tests:** the benchmark itself; `go test` without `-bench` must not run it.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run '^$' -bench BenchmarkHandler_DraftFrame -benchtime 20x -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/tune -count=1`
- `capcom show tune-preview-playback-speed` shows the logged benchmark line.

**Commit:** `test(tune): benchmark a draft frame request`

## 3. Start a pan from the centre the crop shows — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the shared clamp is `cropAround(size, factor, centre)` in compose.go; `(*Compositor).crop` now always evaluates `centreAt`, also at a factor of 1, where it used to skip it.
NOTES (2026-10-18): no CHANGELOG entry: camera pan moves (the only pans with a laid path; beat-zoom aims are instant) are still unreleased under [Unreleased], so no shipped version had the defect.
NOTES (2026-10-18): rewrapped the reflowed camera paragraph of internal/compose/doc.go (from "every frame scales" to its end) so every line is at most 100 columns.

**What:**
Fix for `zoom-target-bleed`, a defect in `newCamera`'s path laying since camera pans landed: each
pan path starts at the previous pan's `To` even when a zoom-out to 1× hid it, so a later zoom-in
first heads for that stale point (zoom 0 in on A, zoom 1 out to 1×, zoom 2 in on B heads to A
first).

**Regression guard.** In `camera_test.go` rewrite "a pan starts where the previous one ended" as
"a pan at 1× starts from the frame centre", and build `TestComposePlansOverlappingCameraMovesAcrossBeats`'s
via `curve` and its rows from alpha clamped at 2× (250,150); neither is a deferral. Test (b) asserts
`centreAt(pan.Start)` equals the clamped centre (the crop-equality check passes before the fix).
Rule: every comment saying a pan path starts where the track holds it or the previous pan ended —
`grep -rn -i "track holds\|previous pan\|where the pan track" internal/compose`.

**Goal:** every pan-track path starts from the crop centre visible at its start: the frame
centre when the factor there is ≤ 1, otherwise the held centre clamped as the crop clamps it.

**Approach (assumed at the header base):**
- `internal/compose/camera.go` `newCamera`: before laying each segment's `newPath`, replace
  `held` with the visible centre at `pan.Start`, using the zoom track's factor there
  (`trackAt` over the sorted zooms, as `factorAt` does).
- One clamp, shared: extract the centre/size clamp from `(*Compositor).crop`
  (`internal/compose/compose.go`) into a function both `crop` and `newCamera` call; no copy.
- Unchanged: a pan at 1× still ends at its `To`, so the next zoom aims there (README behaviour).

**Files:** `internal/compose/camera.go`, `internal/compose/compose.go`, `internal/compose/camera_test.go`,
`internal/compose/doc.go`
**Read first:** `internal/compose/camera.go` — newCamera, factorAt, centreAt; `internal/compose/compose.go` — (*Compositor).crop;
`internal/compose/camera_test.go` — TestCameraPanFollowsItsPath, TestComposePlansOverlappingCameraMovesAcrossBeats, cameraLayout;
`internal/compose/doc.go` — package comment

**Tests:** in `camera_test.go`: (a) zoom in + pan to A, zoom out to 1×, zoom in + pan to B over
the same stretch: `centreAt` samples through the last move run from the frame centre towards B,
none nearer A than the previous sample; (b) a pan starting while the crop is clamped against an
edge: `centreAt(pan.Start)` equals the clamped centre. Both fail before the fix. The 1× and
alpha-at-2× rows the guard names are rewritten.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/render -count=1`

**Closes:** zoom-target-bleed

**Commit:** `fix(compose): start a pan from the centre the crop shows`

## 4. Drop zoom aiming from the tune page

**What:**
First step of `zoom-target-removal` and `beat-zoom-removal` (ratified header calls): the page
stops offering any zoom target or beat zoom. The server still accepts the edits until item 8.

**Regression guard.** `TestPage_EntryFields_AreEditFields` exempts `camera.target` from "the page
sends no … edit", with a comment that item 8 removes the exemption (fields.go lists it editable
until then). `SNIPPET_ZOOM` stays — the server still sends the "zoom" snippet that fills
`#pick-target`; only picker.js's header comment drops its "zoom, camera.target" wording.

**Goal:** `internal/tune/page/` has no zoom lane, zoom panel, zoom-target rows or "Set as zoom
target" button; pans keep "Set as pan target" and their inspector rows.

**Approach (assumed at the header base):**
- `entries.js`: drop `zoomTargetRows` and its call in `aimRows`.
- `picker.js`: drop `setAsZoomTarget` (both branches), `setCameraZoomTarget`,
  `FIELD_CAMERA_TARGET`, `FIELD_ZOOM*`; update its header comment.
- `panels.js`: drop `zoomPanel`, `factorField` and their callers.
- `timeline.js`: drop the "zoom" lane (`drawZoom`, `zoomHandles`, `zoomRamps`).
- `index.html`: drop `#pick-zoom`; `style.css`: drop `.block.zoomed*`.
- Every comment naming the removed pieces goes too: `grep -n -i "zoom" internal/tune/page/*`
  and keep only camera-zoom-move wording.

**Files:** `internal/tune/page/entries.js`, `internal/tune/page/picker.js`,
`internal/tune/page/panels.js`, `internal/tune/page/timeline.js`, `internal/tune/page/index.html`,
`internal/tune/page/style.css`, `internal/tune/pagejs_test.go`, `internal/tune/page_test.go`
**Read first:** `internal/tune/page/picker.js` — setAsZoomTarget, setCameraZoomTarget; `internal/tune/page/entries.js` — aimRows, zoomTargetRows;
`internal/tune/page/panels.js` — zoomPanel; `internal/tune/page/timeline.js` — drawZoom; `internal/tune/page_test.go` — TestPage_EntryFields_AreEditFields;
`internal/tune/pagejs_test.go` — TestPageJS_CameraZoomTarget_AddAndRemoveInTheInspector

**Tests:** delete the page tests of the removed controls
(`TestPageJS_SetAsZoomTarget_*`, `TestPageJS_CameraZoomTarget_AddAndRemoveInTheInspector`, zoom
lane/panel tests); add one asserting the page has no `#pick-zoom` and a selected camera zoom's
panel shows no target row; exempt `camera.target` in `TestPage_EntryFields_AreEditFields`.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPage' -count=1`
- `grep -nE '"zoom\.|pick-zoom|drawZoom|zoomPanel|zoomTargetRows|camera\.target' internal/tune/page/*` prints nothing

**Commit:** `feat(tune)!: drop zoom targets and beat zoom from the page`

## 5. Drop zoom aiming from the tune state

**What:**
Continues item 4. Depends on item 4.

**Regression guard.** add internal/tune/doc.go to Files and drop the beat zoom, zoom factor auto and zoom YAML from its [Edit] sentence; replace every internal/tune test use of beat-zoom or zoom-target edits and YAML (grep -n 'FieldZoom\|FieldCameraTarget\|zoom:' internal/tune), including the build-error-forcing FieldZoomTarget edits in session_test.go, with pan-target equivalents, so items 7–10 leave internal/tune green.
Also: `sessionStoryboard` only loses beat 2's `zoom:` and holds no `camera:` (fixtures append one);
the camera zoom + pan over beat 2 go in a separate fixture. Named: `TestSession_State_ReportsTheSchedule`,
`TestSession_Pick_ProposedTargetIsAZoomTargetEdit`, `TestSession_LastMatchTarget_RoundTrips…` (drop
`pickStoryboard`'s beat zoom), `TestSession_State_CaptionBoxes_AbsentWithoutAFullPipeline` and
`TestSession_EveryEditsBuild_ReusesTheGoodPipeline` (pan target "nowhere"); the two outside writes of
`TestSession_Refresh_InvalidOutsideWrite_…` turn invalid another way (e.g. zoom `to: 9`, keeping the
`camera[0]:` prefix); frame_test's "zoom ramp" row becomes a `camera.over` edit on a camera-zoom fixture.
`TestPageJS_RefusedBarDrag_…` gets a fixture with a second zoom in the release stretch and pins
"overlaps" and "on the zoom track". Comment rule: `grep -n -i "zoom" internal/tune/*.go cmd/photon/tune.go`.

**Goal:** `internal/tune` Go code reports no zoom target and no beat zoom: no `ZoomState`, no
`BeatState.Zoom`, no zoom `Target` on `CameraState`; its test fixtures hold no beat `zoom:` and no
zoom `target:`.

**Approach (assumed at the header base):**
- `state.go`: drop `ZoomState`, `BeatState.Zoom` and the zoom fill of `CameraState.Target`
  (keep `TargetState` — pans use it).
- `session_test.go` `sessionStoryboard`: replace beat 2's `zoom:` with a camera zoom plus a pan
  over the same stretch; rewrite tests that relied on it (`TestSession_ZoomTargetNotOnScreen_…`
  becomes a pan target not on screen; `TestPageJS_RefusedBarDrag_…` relies on a camera-move
  overlap refusal instead of the beat zoom's).
- Delete `TestSession_State_ReportsAZoomsTarget`,
  `TestHandler_CameraTargetAsThePageSendsIt_AimsAndUnaimsAZoom` and the zoom rows of
  `TestHandler_EditsAsThePageSendsThem_AreApplied`; trim the zoom-target half of
  `TestSession_PickProposal_AppliesAsAZoomTargetAndAPanDestination`.
- `pick.go`: drop the `snippetZoom` comment; `cmd/photon/tune.go`: drop zoom from help text.

**Files:** `internal/tune/state.go`, `internal/tune/pick.go`, `internal/tune/session_test.go`,
`internal/tune/server_test.go`, `internal/tune/pick_test.go`, `internal/tune/frame_test.go`,
`internal/tune/pagejs_test.go`, `cmd/photon/tune.go`, `internal/tune/doc.go`
**Read first:** `internal/tune/state.go` — BeatState, ZoomState, cameraStates; `internal/tune/session_test.go` — sessionStoryboard, cameraCaptionsStoryboard;
`internal/tune/pick_test.go` — pickStoryboard; `internal/tune/pagejs_test.go` — TestPageJS_RefusedBarDrag_RedrawsInPlaceAndShowsTheRefusal;
`internal/tune/doc.go`

**Tests:** the rewritten tests above and those the guard names; the package stays green.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -count=1`
- `GOMEMLIMIT=2GiB go test ./cmd/photon -count=1`
- `grep -rnE 'ZoomState|^ +zoom: |zoom: \{to: [^}]*target:' internal/tune` prints nothing

**Commit:** `refactor(tune)!: drop zoom targets and beat zoom from the state`

## 6. Move zoom tests onto camera moves

**What:**
Test-only preparation for item 7: the compose and render tests that build a beat zoom keep their
coverage on camera zoom and pan moves.

**Regression guard.** No `zoom_test.go` or `draft_test.go` exists: the fixtures and Draft tests live in
`compose_test.go`. `zoomClickTake` takes `t` and builds via `parseBoard` (struct-literal moves all
start at 0): zoom to 2 at `{beat: 2}` over 1s, zoom to 1 at 5s over 1s (the 5.8s mid-ramp sample
needs the pull), and a pan to the box centre's `cell:` that ends by 2s, e.g. at 1s over 1s (a `text:`
pan does not resolve on the blank snapshots). Delete `TestZoomRampsShrinkToFitTheSection` here, not
in item 7 (it sets `Beats[1].Zoom`, nil once rebased). `TestRun_ScheduleWarnings_PrintBeforeAComposeError`
also reads `lostZoomStoryboard` and stays green on `errors.Is`.

**Approach (assumed at the header base):**
- `internal/compose`: rebase the `zoomClickTake` fixture (and `draftBoard`) on a camera zoom plus
  a pan to the same target; the tests built on it (crop inside the frame, zoomed cursor and
  ring, captions in place, `Compositor` Cell/Rect, direct path, the Draft tests) keep their
  assertions.
- `internal/render/pipeline_test.go`: `lostZoomStoryboard` and
  `TestBuild_AZoomTargetNotOnScreen_LeavesAPartialPipeline` use a pan target not on screen.
- Leave the beat-only tests for item 7 to delete.

**Files:** `internal/compose/compose_test.go`, `internal/render/pipeline_test.go`
**Read first:** `internal/compose/compose_test.go` — zoomClickTake, draftBoard, parseBoard, TestCompositorCell, TestComposeZoomCropStaysInsideTheFrame;
`internal/storyboard/camera.go` — ZoomMove.Start; `internal/render/pipeline_test.go` — lostZoomStoryboard,
TestBuild_AZoomTargetNotOnScreen_LeavesAPartialPipeline

**Tests:** the moved tests themselves; `TestZoomRampsShrinkToFitTheSection` deleted.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/render -count=1`

**Commit:** `test(compose): build zoom fixtures from camera moves`

## 7. Drop beat zoom and zoom aims from compose

**What:**
Part of `zoom-target-removal` and `beat-zoom-removal`. Depends on items 3 and 6.

**Regression guard.** add `GOMEMLIMIT=2GiB go test ./internal/tune -count=1` to Acceptance.
Also: keep `dragFromText`/`dragToText` (move them into `cursor_test.go`, whose `dragAt` reads them);
delete `TestComposeAutoFitFactor`, `zoom_golden_test.go` whole and `beatZoomAim`/`beatZoomRamps`
(CI's `unused` lint reds on orphans); `TestZoomRampsShrinkToFitTheSection` is already gone (item 6).
The Goal is checked by the Acceptance grep.

**Goal:** `internal/compose` plans no beat-zoom section and no zoom aim: the pan track holds only
pan moves, the zoom track only camera zoom moves.

**Approach (assumed at the header base):**
- `zoom.go`: drop `planSection`, `zoomBox`, `loggedBox`, `dragEndIs`, `autoFitFactor`, the
  auto-fit and drag constants and the beat loop in `zoomPlanner.plan`; keep `resolveShown`,
  `easeInOut`, `clamp01` and `zoomPlanner` (move them to the file that uses them most).
- `camera.go`: drop `planZoomAim` and the `zoom.Target` branch of `planMoves`.
- Docs: `compose.go` `New` doc, `doc.go` (its file map is pinned by `docmap_test.go`),
  `internal/render/render.go`, `doc.go`, `pipeline.go` comments. Rule: every comment naming a
  beat's zoom or a zoom's aim: `grep -rn -i "beat.*zoom\|zoom.*aim\|zoom.*target" internal/compose internal/render`.
- Delete the beat-only tests: `TestComposeBeatZoomCropsMatchTheGolden` with
  `testdata/beat-zoom-crops.golden`, ZoomRampsShrinkToFitTheSection,
  ZoomResolvesItsTargetOnTheSnapshot, ZoomBoxWalksBackThroughTheBeat,
  NewRefusesAZoomTargetTheBeatNeverShows, `TestComposeZoomAimsAtADragsLoggedBox`,
  `TestComposeTargetedZoomAimsAtItsTarget`, `TestNewRefusesAZoomTargetItsMoveNeverShows`.

**Files:** `internal/compose/zoom.go`, `internal/compose/camera.go`, `internal/compose/compose.go`,
`internal/compose/doc.go`, `internal/compose/*_test.go`, `internal/compose/testdata/beat-zoom-crops.golden`,
`internal/render/render.go`, `internal/render/doc.go`, `internal/render/pipeline.go`
**Read first:** `internal/compose/zoom.go` — zoomPlanner.plan, dragFromText, autoFitFactor; `internal/compose/camera.go` — planMoves, planZoomAim;
`internal/compose/cursor_test.go` — dragAt; `internal/compose/compose_test.go` — TestComposeAutoFitFactor, beatZoomAim

**Tests:** remaining compose/render/tune tests green; `TestComposeUntargetedZoomLeavesThePanTrack` kept;
the guard's deletions.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/render -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/tune -count=1`
- `grep -n "planSection\|planZoomAim\|beat\.Zoom" internal/compose/*.go` prints nothing

**Commit:** `refactor(compose)!: plan no beat zoom and no zoom aim`

## 8. Drop zoom-target and beat-zoom edits

**What:**
Part of `zoom-target-removal` and `beat-zoom-removal`. Depends on items 4 and 5.

**Regression guard.** add internal/tune/page_test.go to Files and handle it as regression-3.md's note on item 4's exemption says
(remove the `camera.target` exemption item 4 put in `TestPage_EntryFields_AreEditFields`).
Also drop `decodeTarget`, `keyTarget`, `keyFactor` and `defaultTarget` (no caller remains; CI's
`unused` lint reds); keep `keyZoom` and `Target.YAMLFlow`. Comment rule, `doc.go` included:
`grep -n -i "zoom\.\(target\|factor\|in\|out\)\|camera\.target\|or zoom\b\|zoom and its" internal/storyboard/*.go`.

**Goal:** `storyboard.Apply` offers no edit of a zoom's target or a beat's zoom, and
`storyboard` field descriptions list neither.

**Approach (assumed at the header base):**
- `edit_camera.go`: drop `FieldCameraTarget`, its case, `setZoomTarget` (keep `zoomMoveKeyOrder`).
- `edit.go`: drop `FieldZoom`, `FieldZoomTarget`, `Factor`, `In`, `Out` from the field list,
  `isNullTaken`, `hasZoom`.
- `edit_beat.go`: drop the zoom cases, `setRamp`, `setFactor`, `addOrRemoveZoom`,
  `replaceTarget`, `zoomOnto`, `decodeFactor`, `zoomKeyOrder`; keep `keyZoom`, `keyTarget`,
  `decodeTarget`, `Target.YAMLFlow` while pans and picks use them.
- `fields.go`: drop camera.target for zooms and the zoom part of `beatFields`, `defaultTarget`
  if unused.

**Files:** `internal/storyboard/edit_camera.go`, `internal/storyboard/edit.go`,
`internal/storyboard/edit_beat.go`, `internal/storyboard/fields.go`,
`internal/storyboard/edit_test.go`, `internal/storyboard/fields_test.go`,
`internal/storyboard/edit_list_test.go`, `internal/storyboard/doc.go`, `internal/tune/page_test.go`
**Read first:** `internal/storyboard/edit.go` — Edit, Apply; `internal/storyboard/edit_beat.go` — editBeat, decodeTarget, Target.YAMLFlow;
`internal/storyboard/edit_camera.go` — setZoomTarget; `internal/storyboard/fields.go` — beatFields, cameraFields

**Tests:** delete `TestApply_CameraTarget_*` and the zoom rows of the `TestApply_*`, WireSpellings
and CameraEdit tables; `zoomTargetBoard` and its test; `fields_test.go`'s beat `FieldZoom`/`FieldZoomTarget`
rows of `defaultRefusals` and the zoom rows of `TestFields_DescribeEachOwnersFields`.
`InsertOverlappingAMove` already overlaps camera[1]'s zoom and stays as is.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/tune -count=1`
- `grep -n "FieldZoom\|FieldCameraTarget\|decodeTarget\|keyTarget\|keyFactor" internal/storyboard/*.go` prints nothing

**Commit:** `refactor(storyboard)!: drop zoom-target and beat-zoom edits`

## 9. Refuse a zoom target on load

**What:**
Closes `zoom-target-removal`. Depends on items 5, 7 and 8.

**Regression guard.** Acceptance also runs `GOMEMLIMIT=2GiB go test ./internal/compose -count=1`, `GOMEMLIMIT=2GiB go test ./internal/render -count=1` and `GOMEMLIMIT=2GiB go test ./internal/tune -count=1`, one command at a time.
Also: `Parse`'s strict decode refuses `target` before validation, so give `ZoomMove` an exported
presence-only field tagged `yaml:"target"` (a type with its own `UnmarshalYAML`), reported from
`validateForm` via `moveReport` — not `ZoomMove.UnmarshalYAML` via `node.Decode`, which drops
`KnownFields` (the "unknown move field" row reds); `TestKeyOrder`'s zoom-move row keeps "target".
In `fields_test.go` drop the `{moveOwner(5), FieldCameraAt}` refusal row (camera[5]'s default then
fits) and the camera[5] clause of the `defaultRefusals` comment. Comment rule:
`grep -rn -i "targeted zoom\|zoom.*target\|zoom's target" internal/storyboard internal/clip`.

**Goal:** `storyboard.ZoomMove` has no `Target`; loading a camera zoom with `target:` fails under
`camera[i]` with `zoom: target: removed; aim with a pan move over the same stretch`.

**Approach (assumed at the header base):**
- `storyboard/camera.go`: drop `ZoomMove.Target` and its doc, the `tracks()` targeted case and
  the target branch of `ZoomMove.validate`; refuse the key with the message above (the strict
  decoder alone would give a generic unknown-field error).
- `storyboard/doc.go`, `internal/clip/clip.go` `cameraAreas` zoom-target branch.
- `internal/storyboard/testdata/every-form.yaml`: drop the zoom's `target:`.

**Files:** `internal/storyboard/camera.go`, `internal/storyboard/doc.go`,
`internal/storyboard/storyboard_test.go`, `internal/storyboard/testdata/every-form.yaml`,
`internal/clip/clip.go`, `internal/clip/clip_test.go`, `internal/storyboard/fields_test.go`,
`internal/storyboard/edit.go`
**Read first:** `internal/storyboard/camera.go` — ZoomMove, CameraMove.tracks, CameraMove.validateForm, ZoomMove.validate;
`internal/storyboard/storyboard.go` — Parse; `internal/storyboard/storyboard_test.go` — TestLoadRejects_Camera;
`internal/storyboard/fields_test.go` — defaultRefusals; `internal/clip/clip.go` — cameraAreas

**Tests:** a `TestLoadRejects_Camera` row loading a zoom with `target:` and checking the exact
message under `camera[i]`; drop the old target rows and the clip_test zoom-target case; drop the
`fields_test.go` camera[5] `FieldCameraAt` refusal row.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard ./internal/clip -count=1`
- `go build ./...`
- `GOMEMLIMIT=2GiB go test ./internal/compose -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/render -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/tune -count=1`

**Closes:** zoom-target-removal

**Commit:** `feat(storyboard)!: refuse a target on a camera zoom`

## 10. Refuse a beat zoom on load

**What:**
Closes `beat-zoom-removal` with item 11. Depends on items 5, 7, 8 and 9.

**Regression guard.** same three extra Acceptance commands as item 9.
Also: beat problems file by id, so the error reads under `beat <id>` (`beat 1: zoom: removed; use a
camera zoom move and a pan move`) — the Goal's `beats[i]` spelled as the beat owner — from
`Beat.validate`'s reporter via an exported presence-only field tagged `yaml:"zoom"`, not
`Beat.UnmarshalYAML` (`node.Decode` drops `KnownFields`; the unknown-field rows red). In `edit_test.go`
drop `editBaseBoard`'s zoom line and its comment, TestKeyOrder's "a zoom" row, and "zoom" from its
beat row if no `zoom` key field remains. In `clip_test.go` drop the `beat 1: zoom.target.area` want
line; fix `undeclared/demo.yaml`'s "sidebar twice" header. Move `nth: 1` onto `validBase`'s click
target (keeps the three nth decode rows); recast `TestParseMatchesLoad`'s "invalid zoom" row on
another field. Comment rule: `grep -rn -i "beat's zoom\|\[Zoom\]\|zoom ramp\|zoom section\|Cut, Zoom" internal/storyboard internal/clip`.

**Goal:** `storyboard.Beat` has no `Zoom` and `type Zoom` is gone; loading a beat with `zoom:`
fails under `beats[i]` with `zoom: removed; use a camera zoom move and a pan move`.

**Approach (assumed at the header base):**
- `beat.go`: drop `Beat.Zoom`, `type Zoom`, `DefaultZoomRamp`; `validate.go`: drop
  `Zoom.validate` (keep the min/max factor bounds `ZoomMove` uses); `storyboard.go`: drop the
  ramp defaults; `camera.go`: drop `beatZoomSpans` and the beat-overlap loop of `validateCamera`.
- `doc.go` and `internal/clip/clip.go` `namedAreas`.
- Testdata: drop beat `zoom:` from `internal/storyboard/testdata/{board,every-form}.yaml`,
  `cmd/photon/testdata/valid/every-form.yaml`, `internal/clip/testdata/{valid,undeclared}/demo.yaml`.

**Files:** `internal/storyboard/beat.go`, `internal/storyboard/validate.go`,
`internal/storyboard/storyboard.go`, `internal/storyboard/camera.go`, `internal/storyboard/doc.go`,
`internal/storyboard/storyboard_test.go`, `internal/storyboard/testdata/*.yaml`,
`internal/clip/clip.go`, `internal/clip/testdata/*/demo.yaml`, `cmd/photon/testdata/valid/every-form.yaml`,
`internal/storyboard/edit_test.go`, `internal/storyboard/fields_test.go`, `internal/clip/clip_test.go`
**Read first:** `internal/storyboard/beat.go` — Beat, Zoom, DefaultZoomRamp; `internal/storyboard/validate.go` — Beat.validate;
`internal/storyboard/camera.go` — beatZoomSpans; `internal/storyboard/edit_test.go` — editBaseBoard;
`internal/storyboard/storyboard_test.go` — validBase, TestParseMatchesLoad

**Tests:** a `TestLoadRejects` row loading a beat with `zoom:` and checking the exact message
under `beat <id>`; drop the old beat-zoom rows (`TestLoadRejects`, `TestLoadRejects_Camera`) and
`TestLoadEveryForm`'s beat-zoom asserts; the `edit_test.go`, `clip_test.go` and `validBase` changes
the guard names.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard ./internal/clip ./cmd/photon -count=1`
- `go build ./...`
- `GOMEMLIMIT=2GiB go test ./internal/compose -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/render -count=1`
- `GOMEMLIMIT=2GiB go test ./internal/tune -count=1`

**Commit:** `feat(storyboard)!: refuse a beat zoom`

## 11. Document aiming through pans only

**What:**
Docs for `zoom-target-removal` and `beat-zoom-removal`. Depends on items 9 and 10. The CHANGELOG
sidecar records the breaking removal of beat zoom and that a zoom no longer takes `target`, and
names the stale Unreleased bullets (zoom `target`, beat zoom as camera-move shorthand, Set as
zoom target) to drop.

**Regression guard.** The CHANGELOG sidecar names every Unreleased bullet that describes a beat zoom or a zoom target (rule: `grep -n -i zoom CHANGELOG.md` within the Unreleased section), not only the three listed.
Also: the Acceptance grep matches only removed forms (README's `camera` header row describes the
kept schema and matches `zoom.*target`). Glossary: the Beat and Target entries (there is no zoom
entry). Rule: `grep -rn -i "zoom" README.md .capcom/knowledge internal/*/doc.go cmd/photon/*.go`,
keeping camera-zoom-move wording only; the README TOC anchor follows any heading rename.

**Goal:** README, glossary and command help describe zooming with camera zoom moves and aiming
with pans only; none describes a beat zoom or a zoom target.

**Approach (assumed at the header base):**
- README: rewrite the "Sections, zoom and cursor" section and every line naming a beat `zoom:`
  or a zoom `target`: `grep -n -i "zoom" README.md`.
- `.capcom/knowledge/glossary.md`: the zoom entry; `cmd/photon/render.go` help text.

**Files:** `README.md`, `.capcom/knowledge/glossary.md`, `cmd/photon/render.go`,
`.capcom/knowledge/package-layout.md`, `internal/target/doc.go`, `internal/check/doc.go`
**Read first:** `README.md` — Sections, zoom and cursor, Camera, Header table camera row; `.capcom/knowledge/glossary.md` — Beat, Target;
`cmd/photon/render.go` — newRenderCommand; `cmd/photon/readme_test.go` — TestREADMEDocumentsEveryField;
`.capcom/knowledge/package-layout.md` — internal/target row

**Tests:** `GOMEMLIMIT=2GiB go test ./cmd/photon -count=1` (help text pins, if any).

**Acceptance:**
- `grep -n -i "beat's .zoom\|zoom\.target\|zoom's .target\|targeted zoom\|zoom\.factor\|zoom\.in\|zoom target" README.md .capcom/knowledge/glossary.md` prints nothing
- `capcom check`

**Closes:** beat-zoom-removal

**Commit:** `docs: aim through pans only`
