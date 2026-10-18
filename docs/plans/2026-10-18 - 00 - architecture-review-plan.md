# Architecture review findings — implementation plan

**Goal:** Act on all seven findings of the 2026-10-18 architecture review: one frame pipeline in
render, screen lookup and beat matching owned by the take, located problems, one output clock,
values resolved once at Parse, yamledit query/upsert, and storyboard-owned typed edits.
**Date:** 2026-10-18
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** a2a5db5
**Closes:** (per item)

**Sources:**
- `docs/reviews/architecture-review-2026-10-18.html` (findings 1–7)
- `.capcom/issues/`: `frame-pipeline`, `take-screen-lookup`, `located-problems`, `output-clock`,
  `resolve-at-parse`, `yamledit-upsert`, `typed-storyboard-edits`, `tune-page-app-split`
- Notes `package-layout`, `coding-conventions`, `glossary`

**Ratified design calls (user, 2026-10-18):**
- **Scope:** all seven findings, in the order #1+#7, #3, #2, #6, #5, #4.
- **Beat order:** a take whose beats started in another order than the storyboard's is a hard error in `schedule.New` and `photon check` ("re-record it").
- **Edit home:** typed edits (`Apply`, `Fields`) live inside `internal/storyboard`, which imports `yamledit`.
- **Problems UI:** a camera or caption problem shows on its own panel only; the Clip frame panel shows clip-level problems; camera and caption options get the error mark.
- **Lint fonts:** `photon lint` always runs the font-bound tier (caption layout with the real fonts).
- **All-cut:** a storyboard whose every beat is cut is a Parse error.
- **Fields wire:** the editable-field description travels inside `/api/state`.
- **Error text:** CLI error texts stay byte-identical, except the beat-matcher pair item 9 unifies.
- **Inputs:** the inspector takes a target as text + nth + area inputs and a moment as a beat select + offset-ms input; the page builds typed JSON and holds no YAML.

**Writer's calls (2026-10-18, binding):** owner JSON is `{"kind": "clip"|"beat"|"camera"|"caption", "index": N}` (beat id for a beat, list index for camera/caption, 0 for clip), shared by problems and edits; `/api/edit` is page-internal and changes shape without versioning; storyboard colours resolve to a storyboard `RGB` type (no `image/color` in a domain package).

**Regression check (2026-10-18, a2a5db5):**
- 2: guard folded
- 3: guard folded
- 4: guard folded (`internal/tune/doc.go:19-20` rasterizer-reuse sentence superseded between items 4 and 5)
- 5: guard folded
- 6: guard folded
- 8: guard folded
- 9: recast
- 10: guard folded
- 11: guard folded
- 12: guard folded
- 13: guard folded
- 14: guard folded
- 15: guard folded
- 16: recast
- 17: recast
- 18: recast
- 19: recast
- 20: recast
- 21: guard folded
- 22: guard folded
- 23: guard folded
- 24: recast
- re-check round (regression-6, regression-7):
- 17: guard folded (setCaptionText, setMoment, setCaptionUntil keep Has + Replace)
- 18: recast (null refusal; `nth: last` wire and shared target speller in pick.go)
- 19: guard folded (null refusal; item 18's nth rule for camera.to; controls.js)
- 20: recast (null refusal for every caption field; `caption.until: null` clears it)
- 24: guard folded (TestPage_EntryFields_AreEditFields kept, reading storyboard's Fields)

**Standing requirements:**
- skills: coding-standards
- Every new non-test file is named in its package's `doc.go` file map (`testkit.CheckDocMap`).
- Forbidden names per `AGENTS.local.md` never land in a file.
- Tests run one package at a time with `GOMEMLIMIT=2GiB`; never `./...` with `-race`.

**Out of scope:**
- The open tune features `tune-caption-styling`, `tune-add-remove-entries`, `tune-edit-pan-via` (they now wait on `typed-storyboard-edits`).
- yamledit sequence-item insert/delete/move; `engine.BeatError`; schedule warning text scraping in the page.

## 1. Take answers "which screen shows at t" — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the method's parameter is named `at`, not `t` as the Goal's signature writes it, because `t` is already the receiver name of every `*Take` method in take.go; the signature is otherwise as written.

**What:**
**Goal:** `(*take.Take).ScreenAt(t time.Duration) int` returns the index of the last snapshot at or before t, the first snapshot when t precedes them all, and -1 for a take with no snapshots; `render.SnapshotAt` no longer exists.
**Approach (assumed at the header base):** move `render.SnapshotAt`'s `sort.Search` body into a `take` method; callers `render/encode.go` (`plan.writeFrames`), `tune/frame.go` (`pipeline.rasterAt`), `tune/pick.go` (`Session.Pick`) and `tune/frame_test.go` (`renderedFrame`) call `recorded.ScreenAt`. Move `TestSnapshotAt` to `take_test.go` as `TestTake_ScreenAt`.
**Files:** internal/take/take.go, internal/take/take_test.go, internal/render/encode.go, internal/render/render_test.go, internal/tune/frame.go, internal/tune/pick.go, internal/tune/frame_test.go
**Read first:** internal/render/encode.go — SnapshotAt, plan.writeFrames; internal/take/take.go — Take, Snapshot; internal/tune/frame.go — pipeline.rasterAt; internal/tune/pick.go — Session.Pick; internal/tune/frame_test.go — renderedFrame; internal/render/render_test.go — TestSnapshotAt; internal/take/take_test.go — TestTake_BeatSpans
**Tests:** `TestTake_ScreenAt` table: before the first, exactly at, between, after the last, empty take.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/take/`
- `GOMEMLIMIT=2GiB go test ./internal/render/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `! grep -rn 'SnapshotAt' internal/`
**Commit:** `refactor(take): own the screen-at-time lookup`

## 2. Take answers "which screens show during a stretch" — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the shared private helper is `Take.screensSince(start, stop)` (sort.Search based, no full walk); `ScreensDuring`/`ScreensThrough` differ only in the stop index, so no boolean flag. `compose.resolveShown` now takes the shown indices instead of since/until and walks them newest first with `slices.Backward`; no reversed-stretch guard was added in `panPoint` because schedule sections run in take span order, so since <= until there.

**What:**
**Goal:** `take` exposes `ScreensDuring(start, end)` (half-open `[start, end)`) and `ScreensThrough(start, end)` (closed `[start, end]`), each returning snapshot indices oldest first, where snapshot i is shown over `[At_i, At_{i+1})` and the last one from its `At` onward; `check` and `compose` call them and no longer walk snapshots; `compose`'s `+time.Nanosecond` workaround is gone.
**Approach (assumed at the header base):** one private walk in `take` behind both methods. `check.beatScreens` uses `ScreensDuring` for every span but the last and `ScreensThrough` for the last (today's `isLast` closed end). `compose.zoomPlanner.resolveShown` walks the returned indices newest first, trying the resolver as today; `zoomBox` keeps today's zero-length-beat behaviour by asking for `ScreensDuring(since, math.MaxInt64)`; `panPoint` calls `ScreensThrough` and drops `until + time.Nanosecond`. Error texts unchanged ("target /%s/: the take has no snapshot in %s").
**Regression guard.** `ScreensDuring` is defined by today's `check.beatScreens` predicate (`At_i < end && At_{i+1} > start`): a zero-length span between two snapshots yields the one screen on show, one exactly on a snapshot's `At` yields none, and the take table pins both. `shownAt`'s `clipSchedule.Length-time.Nanosecond` clamp (`compose/camera.go`) is not the walk workaround and stays.
**Files:** internal/take/take.go, internal/take/take_test.go, internal/check/check.go, internal/compose/zoom.go, internal/compose/camera.go
**Read first:** internal/check/check.go — beatScreens, Judge; internal/compose/zoom.go — zoomPlanner.resolveShown, zoomPlanner.zoomBox; internal/compose/camera.go — zoomPlanner.panPoint, shownAt; internal/take/take.go — Take.BeatSpans; internal/compose/compose_test.go — TestComposeZoomBoxWalksBackThroughTheBeat; internal/compose/camera_test.go — TestComposePanResolvesOnTheScreenAtItsEnd; internal/check/check_test.go — TestJudge_Seen; internal/compose/zoom_golden_test.go — TestComposeBeatZoomCropsMatchTheGolden
**Tests:** take table tests for both methods (stretch before the first snapshot, inside one, across several, ending exactly on a snapshot's `At`, zero length between snapshots (the one screen on show) and exactly on a snapshot's `At` (none)). Existing `TestJudge_Seen`, `TestComposeZoomBoxWalksBackThroughTheBeat`, `TestComposePanResolvesOnTheScreenAtItsEnd` and the zoom golden test pass unchanged.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/take/`
- `GOMEMLIMIT=2GiB go test ./internal/check/`
- `GOMEMLIMIT=2GiB go test ./internal/compose/`
- `! grep -nE 'until ?\+ ?time\.Nanosecond' internal/compose/camera.go internal/compose/zoom.go`
**Commit:** `refactor(take): own the screens-during-a-stretch lookup`

## 3. Render builds frames through one pipeline — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the Pipeline keeps no target-resolver field: Build builds the resolver from the config's areas and hands it to a new unexported `newCompositor` (which `NewCompositor` now wraps), so no field sits unread until item 4 needs one.
NOTES (2026-10-18): `ScreenAt(out)` returns `(take.Snapshot, error)` and, like `FrameAt`, returns the build's error on a partial pipeline; `Cell` reports ok=false, `Rect` the empty rectangle and `Size` the zero point there.
NOTES (2026-10-18): GIF bytes checked unchanged against the base commit: a zoomed, captioned clip rendered at HEAD and with this change gave the same sha256 (a603dff3…); the reuse-last raster now carries over from pass 1 into pass 2, and the doc.go and encode comments say so.

Depends on item 1.

**What:**
**Goal:** `internal/render` has a `Pipeline` built by `Build(loaded *clip.Clip, takePath string, recorded *take.Take, cache RasterCache) (*Pipeline, error)` that owns the frame-size check, the schedule, the rasterizer, the compositor and the target resolver, and answers `FrameAt(out)`, `ScreenAt(out)`, `Cell`, `Rect`, `Size` and `Schedule()`; `render.Run` encodes through it; the GIF bytes are unchanged.
**Approach (assumed at the header base):** new `internal/render/pipeline.go`. `RasterCache` is an interface `{Get(index int) (*image.RGBA, bool); Put(index int, frame *image.RGBA)}` with a reuse-last adapter in render (today's single `shown` slot in `plan.writeFrames`); tune's LRU becomes the second adapter in item 4. `ErrOutsideClip` and its "%s is not in [0, %s)" text move from tune into render (tune re-exports nothing; item 4 switches it). When a stage after `schedule.New` fails, Build returns the error AND a non-nil Pipeline whose `Schedule()` is set and whose frame methods return that error (tune shows the schedule on a compose error). `newPlan` keeps `loadTake` and the palette/out paths and calls `Build`; `writeFrames` loops `schedule.Frames(fps)` calling `FrameAt`.
**Regression guard.** the `FrameAt` parity test compares against an oracle built without the pipeline (`NewRasterizer` + `NewCompositor` + `recorded.ScreenAt`, the `tune/frame_test.go` `renderedFrame` recipe) or a digest of `writeFrames` output captured at the base commit, never against `writeFrames` itself. `newPlan` prints `Schedule().Warnings` whenever Build returns a non-nil Pipeline, before returning Build's error, and Build returns stage errors unwrapped (tune's `beatNamed` attributes by message prefix). Render gains its own `ErrOutsideClip` with the identical text "the time is outside the clip"; tune's stays until item 4 switches it.
**Files:** internal/render/pipeline.go, internal/render/pipeline_test.go, internal/render/render.go, internal/render/encode.go, internal/render/render_test.go, internal/render/doc.go
**Read first:** internal/render/render.go — newPlan, NewRasterizer, NewCompositor, Run, plan; internal/render/encode.go — plan.writeFrames, plan.encode; internal/schedule/schedule.go — Schedule.Frames, Schedule.Source; internal/tune/frame.go — ErrOutsideClip, Session.Frame; internal/tune/cache.go — rasterCache.get, rasterCache.put; internal/render/render_test.go — TestRunMatchesTheOnePassEncode, TestRunRefusesATakeOfAnotherFrame, loadRenderClip, saveRenderTake
**Tests:** `pipeline_test.go`: `FrameAt` for every scheduled frame equals the pipeline-free oracle or the base-commit digest (no ffmpeg); schedule warnings still print before a compose-stage error; outside-clip and frame-size refusals; the partial pipeline on a zoom target not on screen. `TestRunMatchesTheOnePassEncode` updated to the new plan fields and still passing where ffmpeg exists.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/render/`
- `go build ./... && go vet ./internal/tune/`
**Commit:** `refactor(render): build frames through one pipeline`

## 4. Tune asks the render pipeline for frames — ✅ DONE (2026-10-18)

NOTES (2026-10-18): tune's `pipeline` keeps `recorded` (the take its render pipeline was built on) and `rasters` (its cache) beside `board`, `loaded` and `frames *render.Pipeline`, so the cache-reuse rule reads the good pipeline's own take and cache without render exposing its take; `rasterizerFor` became `rasterCacheFor`, which hands `render.Build` the good pipeline's cache while the take and raster geometry stay, else a new one.
NOTES (2026-10-18): `rasterCache.get`/`put` became `Get`/`Put` so the LRU implements `render.RasterCache`; `doc.go` keeps the item-5-bound "reusing the rasterizer" sentence as the plan says, but its "a new rasterizer starts afresh" and the cache.go file-map line now say a new take or raster geometry starts the cache afresh.

Depends on item 3.

**What:**
**Goal:** `internal/tune` builds frames, screens, cells and rects only through `render.Pipeline`; it holds no frame-size check, no snapshot lookup and no `ErrOutsideClip` of its own; its frame-parity test compares `Session.Frame` with `render.Build(...).FrameAt` of the saved file.
**Approach (assumed at the header base):** tune's `pipeline` struct keeps `board` and `loaded` plus a `*render.Pipeline`; `Session.build` calls `render.Build` instead of `checkFrame`, `schedule.New`, `rasterizerFor`'s `NewRasterizer` and `NewCompositor`; `state.go` reads `Schedule()` (falling back to the last good one as today). `frame.go` and `pick.go` drop their `Source`/`rasterAt` boilerplate. `cache.go`'s LRU implements `render.RasterCache`; the reuse decision (same take pointer and raster geometry keeps the cache) stays in tune for now. Delete `checkFrame`, `rasterAt` and the `renderedFrame` chain in `frame_test.go`.
**Regression guard.** `server.go`, `pick_test.go`, `session_test.go` and `server_test.go` switch to `render.ErrOutsideClip` and `Pipeline.Rect`/`Size`, and the ZoomTargetNotOnScreen test compares the good `*render.Pipeline` pointer. `State` nil-checks the current `*render.Pipeline` (a take rewritten at another size fails Build before the schedule) and falls back to the good one's `Schedule()`. `Pipeline.ScreenAt` returns the `take.Snapshot` itself (or the Pipeline exposes its `*take.Take`), so `Pick` and the cache-reuse rule read the good pipeline's take, never `s.recorded`. Build reloading the terminal fonts on every edit is a cost carried to item 5: `internal/tune/doc.go:19-20` ("reusing the rasterizer while the take and the frame's raster geometry stay") is superseded between items 4 and 5, left as is, and holds again after item 5.
**Files:** internal/tune/session.go, internal/tune/frame.go, internal/tune/pick.go, internal/tune/state.go, internal/tune/cache.go, internal/tune/frame_test.go, internal/tune/doc.go, internal/tune/server.go, internal/tune/pick_test.go, internal/tune/session_test.go, internal/tune/server_test.go
**Read first:** internal/tune/session.go — Session.build, Session.rasterizerFor, rasterGeometry, checkFrame, pipeline, Session.rebuild, Session.refreshTake; internal/tune/state.go — Session.State, problemsOf, beatNamed; internal/tune/pick.go — Session.Pick; internal/tune/frame.go — Session.Frame, Session.goodPipeline; internal/tune/server.go — statusOf; internal/tune/session_test.go — TestSession_ZoomTargetNotOnScreen_KeepsTheLastGoodPipelineAndBlocksSave, TestSession_RewrittenTake_IsReadAgain; internal/tune/pick_test.go — cellPixel; internal/tune/server_test.go
**Tests:** the rewritten parity cases (duration, hold, cut, zoom ramp, rewritten take); `TestSession_ZoomTargetNotOnScreen_KeepsTheLastGoodPipelineAndBlocksSave`, `TestSession_RewrittenTake_IsReadAgain` and `TestNewSession_TakeOfAnotherFrame_IsRefused` pass.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `test "$(grep -rn 'recorded at %d×%d cells' internal/ | wc -l)" -eq 1`
- `! grep -nE 'ErrOutsideClip *=|recorded\.ScreenAt' internal/tune/*.go`
**Commit:** `refactor(tune): take frames from the render pipeline`

## 5. Render's surface shrinks and caption fonts load once — ✅ DONE (2026-10-18)

NOTES (2026-10-18): re-derived from tune's `Session.rasterizerFor`: at HEAD it is `Session.rasterCacheFor`, which reused only the raster cache (the rasterizer and the caption faces were rebuilt on every edit); the reuse rule now lives in render (`Pipeline.canReuseRasters`, `Pipeline.captionFacesFrom`) and tune passes `BuildOptions{Cache, Reuse: s.good.frames}`.
NOTES (2026-10-18): the raster-geometry key is render's `rasterGeometry(frame) raster.Geometry` (cols, rows, padding×scale, font_size×scale, line_height — what `raster.New` is given), shared with `newRasterizer`, in place of the frame with width/fps/max_colors zeroed; rasterizer reuse also requires the config's terminal `fonts` dir and theme to match.
NOTES (2026-10-18): caption faces are reused whenever the previous pipeline built in full over the same `caption_font` paths, independent of the take and raster geometry, so a font-size edit or a rewritten take loads no caption font either (the Goal's "once a session").
NOTES (2026-10-18): the tune test observes `Reuse` through a new unexported `Session.buildPipeline` field (set to `render.Build` in NewSession, wrapped by the test); tune's `pipeline.recorded` and `pipeline.rasters` fields were deleted, dead once the reuse rule moved to render.
NOTES (2026-10-18): consequential edit — internal/tune/doc.go: made necessary by the rebuild now reusing the rasterizer and caption faces through render (the old "reusing the rasterizer" sentence was false since item 4).
NOTES (2026-10-18): consequential edit — internal/tune/frame_test.go: made necessary by `render.Build` taking `BuildOptions`.
NOTES (2026-10-18): package-layout's tune row drops `compose` and `raster` from its imports (no non-test tune file imports them since item 4).

Depends on item 4.

**What:**
**Goal:** `internal/render` no longer exports `NewRasterizer` or `NewCompositor`; `Build` takes a previous pipeline to reuse and reuses its rasterizer, raster cache and caption faces when the take, the raster geometry and the config fonts match; a tune session loads caption fonts once, not per edit.
**Approach (assumed at the header base):** `Build`'s last parameter becomes `BuildOptions{Cache RasterCache; Reuse *Pipeline}`; tune's `rasterizerFor` reuse rule (take pointer equal and `rasterGeometry` equal, with Width, FPS and MaxColors zeroed) moves into render beside `Build`; `fonts.LoadCaption` runs only when there is nothing to reuse. `BeatReport` stays exported.
**Regression guard.** item 5 owns the Note `package-layout` rows for `render` and `tune` as they stand after items 4–5 (add .capcom/knowledge/package-layout.md to Files; `capcom check` in Acceptance). The exported `NewRasterizer`/`NewCompositor` definitions go, not only their qualified calls. Caption-font loads are counted by an unexported render counter asserted in `pipeline_test.go`; the tune test asserts only that each edit's Build is given the good `*render.Pipeline` as `Reuse`, and no exported render accessor is added.
**Files:** internal/render/pipeline.go, internal/render/pipeline_test.go, internal/render/render.go, internal/tune/session.go, internal/tune/session_test.go, internal/render/doc.go, .capcom/knowledge/package-layout.md
**Read first:** internal/render/render.go — NewRasterizer, NewCompositor, newPlan; internal/tune/session.go — Session.rasterizerFor, rasterGeometry, Session.build; internal/fonts/fonts.go — LoadCaption, CaptionSet; internal/caption/layout.go — newTextBlock; internal/tune/session_test.go — TestSession_ApplyAndState_RunInParallel; internal/tune/frame_test.go — TestSession_FrameAndApply_RunInParallel
**Tests:** a render test that a reused Build shares the previous rasterizer and caption faces and that a geometry change does not; a render test that ten reused Builds load caption fonts once (the unexported counter); a tune test that each edit's Build is given the good pipeline as `Reuse`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/render/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `! grep -rnE 'render\.(NewRasterizer|NewCompositor)' internal/ cmd/`
- `! grep -nE '^func (NewRasterizer|NewCompositor)\(' internal/render/*.go`
- `capcom check`
**Closes:** frame-pipeline
**Commit:** `refactor(render): build the pipeline once and reuse its fonts`

## 6. Storyboard errors carry a typed owner — ✅ DONE (2026-10-18)

NOTES (2026-10-18): consequential edit — internal/storyboard/storyboard.go: made necessary by located.go (the ValidationError doc comment now says a beat, camera or caption problem is a *LocatedError)
NOTES (2026-10-18): the located reporter is a `collector` type (`unlocated()` and `at(owner)`) beside `prefixed`; validateIDs, validateCamera and validateCaptions now take the collector instead of a reporter
NOTES (2026-10-18): TestLoadRejects_Camera/_Captions now stop (Fatalf, was Errorf) when the problem count is not one, so the new owner check can read the single problem

**What:**
**Goal:** `storyboard` exports `Owner{Kind OwnerKind; Index int}` (json `kind`, `index`; kinds `clip`, `beat`, `camera`, `caption`), `LocatedError{Owner; Err}` whose `Error()` renders today's prefix (`beat N: `, `camera[i]: `, `captions[i]: `) before `Err`, `AtBeat`/`AtCamera`/`AtCaption` constructors and `OwnerOf(err) Owner` (outermost located error via `errors.As`, else clip); every storyboard problem that names a beat, camera move or caption is a `LocatedError`; every message text is byte-identical.
**Approach (assumed at the header base):** new `internal/storyboard/located.go`. In `validate.go`, a reporter variant beside `prefixed` that wraps each problem in a `LocatedError` instead of concatenating the prefix, used by `Storyboard.validate` for beats and by `validateIDs`; same in `camera.go` `validateCamera` (holder `camera[%d]`) and `caption.go` `validateCaptions`. An overlap problem stays owned by the move it is reported on.
**Regression guard.** `LocatedError` has `Unwrap() error` returning `Err` (never `Unwrap() []error`, which tune's `problemsOf` would split as a join), so `errors.Is` reaches a wrapped sentinel such as `target.ErrNotOnScreen` through it.
**Files:** internal/storyboard/located.go, internal/storyboard/located_test.go, internal/storyboard/validate.go, internal/storyboard/camera.go, internal/storyboard/caption.go, internal/storyboard/storyboard_test.go, internal/storyboard/doc.go
**Read first:** internal/storyboard/validate.go — reporter, prefixed, Storyboard.validate, validateIDs; internal/storyboard/camera.go — validateCamera, placeMove; internal/storyboard/caption.go — validateCaptions, Caption.validate; internal/storyboard/storyboard.go — Parse, ValidationError; internal/tune/state.go — problemsOf, beatNamed, clipLevelPrefixes; internal/storyboard/storyboard_test.go — TestLoadReportsEveryProblem, TestLoadRejects_Camera, TestLoadRejects_Captions; internal/storyboard/doc.go — file map
**Tests:** `located_test.go`: `Error()` per kind, `OwnerOf` through `fmt.Errorf("compose: %w", …)` and through a joined error leaf; `errors.Is` reaches a sentinel through `AtCamera`; storyboard tests assert the owner of a beat, a camera and a caption problem; every existing text assertion unchanged.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run Lint`
**Commit:** `feat(storyboard): give problems a typed owner`

## 7. Clip, compose and caption errors carry their owner — ✅ DONE (2026-10-18)

NOTES (2026-10-18): clip's crossCheck builds a *storyboard.LocatedError directly from the namedArea's Owner (storyboard has no exported At(owner, err)); a clip-owned problem (top-level expect[i]) stays unwrapped, which OwnerOf already reports as the clip
NOTES (2026-10-18): the compose and caption tests now check the message start with HasPrefix (was Contains) so the stage prefix is asserted outermost

Depends on item 6.

**What:**
**Goal:** every clip cross-check, compose and caption error that names a beat, camera move or caption is a `storyboard.LocatedError` reachable by `storyboard.OwnerOf`, with the stage prefix (`compose: `, `caption: `) kept outermost and every message text byte-identical.
**Approach (assumed at the header base):** `clip/clip.go` `crossCheck`: `namedAreas`/`cameraAreas` carry an Owner (beat for `beat %d: …`, camera for `camera[%d]: pan…`, clip for top-level `expect[%d]`). `compose/camera.go` `planMoves`: `fmt.Errorf("compose: %w", storyboard.AtCamera(i, fmt.Errorf("pan: %w", err)))` (same for `zoom: at:`); `compose/zoom.go` `plan`: `AtBeat` under `compose: `; `caption/caption.go` `New`: `AtCaption` under `caption: `. `errors.Is(err, target.ErrNotOnScreen)` keeps working.
**Files:** internal/clip/clip.go, internal/clip/clip_test.go, internal/compose/camera.go, internal/compose/zoom.go, internal/compose/camera_test.go, internal/compose/compose_test.go, internal/caption/caption.go, internal/caption/layout_test.go
**Read first:** internal/clip/clip.go — crossCheck, namedAreas, cameraAreas, namedArea; internal/compose/camera.go — planMoves, planPan; internal/compose/zoom.go — zoomPlanner.plan; internal/caption/caption.go — New, place; internal/compose/camera_test.go — TestNewRefusesAPanTargetItsMoveNeverShows; internal/compose/compose_test.go — TestNewRefusesAZoomTargetTheBeatNeverShows; internal/caption/layout_test.go — TestNew_BoxLargerThanTheFrame_IsRefused; internal/clip/clip_test.go — TestWithStoryboard_UndeclaredCameraAreaFailsNamingIt, TestWithStoryboard_UndeclaredAreaFailsNamingIt
**Tests:** one owner assertion per producer: clip camera pan area, clip beat click area; compose pan not on screen (camera 0), beat zoom not on screen (beat 1); caption box larger than the frame (caption 0).
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/clip/`
- `GOMEMLIMIT=2GiB go test ./internal/compose/`
- `GOMEMLIMIT=2GiB go test ./internal/caption/`
**Commit:** `feat(compose): locate camera, beat and caption errors`

## 8. One output clock in the storyboard — ✅ DONE (2026-10-18)

NOTES (2026-10-18): beatZoomSpans reads each kept beat's start by its position in the storyboard (the clock's own per-beat entry) rather than through SectionStart by id, so a board with a duplicated beat id still gets the zoom spans and overlap texts it got before; SectionStart (first beat with the id) serves every by-id caller.
NOTES (2026-10-18): placeMove became a plain function taking the clock, Caption.validate takes the clock beside the board, and planPan and tune's movedState/clipMS take the clock, so validateCamera, validateCaptions, planMoves, cameraStates and captionStates build one clock each instead of one per move or caption.
NOTES (2026-10-18): the clock tests stay in moment_test.go (the file the plan lists) under TestOutputClock_* names; clockBoard gained a second beat 5 to cover the duplicated id.
NOTES (2026-10-18): the open issue caption-cut-beat-test still names Storyboard.ClipTime (moment.go:73) in its body; the function is now OutputClock.Resolve in clock.go.

**What:**
**Goal:** `storyboard` has an `OutputClock` (from `(*Storyboard).Clock()`) with `SectionStart(beat int) (time.Duration, error)`, `Resolve(Moment) (time.Duration, error)` and `Length() time.Duration`, built from the kept beats in storyboard order; output time is summed nowhere else in `storyboard`, `compose`, `caption` or `tune`; `ClipTime`, `Storyboard.Length` and `beatZoomSpans`' own sum are gone.
**Approach (assumed at the header base):** new `internal/storyboard/clock.go`; `Clock()` computes on demand so struct-literal boards work. Callers switch: `camera.go` `placeMove` and `beatZoomSpans` (uses `SectionStart`), `caption.go` `validateTiming`, `caption/caption.go` `span`, `compose/camera.go` `planMoves`/`planPan`, `tune/state.go` `clipMS`. Error texts of `ClipTime` (cut beat, unknown beat) are kept by `Resolve`.
**Regression guard.** add internal/schedule/schedule_test.go to Files — item 8 makes TestNew_AgreesWithTheStoryboardClipClock compile against Clock(), item 9 then extends it. `internal/storyboard/doc.go` maps `clock.go` and its `moment.go` line stops naming `ClipTime`/`Length`. `SectionStart` of a cut beat returns where it sits (the sum of the kept beats before it) with a nil error; only `Resolve` refuses a cut or unknown beat, with today's texts. The clock resolves the first beat with a given id and sums kept durations unfiltered, as `ClipTime` does, since validation runs it on invalid boards.
**Files:** internal/storyboard/clock.go, internal/storyboard/moment.go, internal/storyboard/moment_test.go, internal/storyboard/camera.go, internal/storyboard/caption.go, internal/caption/caption.go, internal/compose/camera.go, internal/tune/state.go, internal/schedule/schedule_test.go, internal/storyboard/doc.go
**Read first:** internal/storyboard/moment.go — Storyboard.ClipTime, Storyboard.Length, Moment; internal/storyboard/camera.go — beatZoomSpans, placeMove, validateCamera; internal/storyboard/caption.go — Caption.validateTiming; internal/schedule/schedule_test.go — TestNew_AgreesWithTheStoryboardClipClock, TestNew_SpansFromTake; internal/caption/caption.go — span; internal/compose/camera.go — planMoves, planPan; internal/tune/state.go — clipMS, movedState, captionStates; internal/storyboard/moment_test.go — clockBoard, TestStoryboard_ClipTime_Refuses
**Tests:** a clock table test (cut beats skipped, offsets, negative offset, unknown and cut beat errors from `Resolve`, a cut beat's `SectionStart`, a duplicated id resolving to its first beat); existing storyboard, caption, compose and tune tests pass.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/caption/`
- `GOMEMLIMIT=2GiB go test ./internal/compose/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `GOMEMLIMIT=2GiB go test ./internal/schedule/`
- `! grep -rn 'ClipTime' internal/`
- `! grep -n 'func (s \*Storyboard) Length' internal/storyboard/*.go`
**Commit:** `refactor(storyboard): one output clock`

## 9. Take matches beats once; a reordered take is refused — ✅ DONE (2026-10-18)

NOTES (2026-10-18): deviation — internal/storyboard/storyboard.go (not in **Files:**) gains `(*Storyboard).BeatIDs`, the storyboard's beat ids in order with cut beats, because schedule and check both hand it to `MatchBeats`; one owner instead of a duplicated `beatIDs` helper in each package.
NOTES (2026-10-18): consequential edit — internal/take/doc.go: made necessary by take.go gaining `Take.MatchBeats` and `BeatError` (file-map line).
NOTES (2026-10-18): consequential edit — internal/schedule/doc.go: made necessary by schedule.New now refusing a mismatched or reordered take (package comment).
NOTES (2026-10-18): schedule's `sectionsOf` drops its own never-started / undeclared checks (now `MatchBeats`' job) and keeps `checkTiming`; `New` also takes `Length` from `board.Clock().Length()`, not only each `OutStart` from `SectionStart`.
NOTES (2026-10-18): `TestNew_AgreesWithTheStoryboardClipClock` now compares every section (cut ones included) against `SectionStart`; the out-of-order rejection is its own test, `TestNew_RefusesATakeOutOfStoryboardOrder`, plus a case in `TestNew_RejectsBadSpans`.

Depends on items 6 and 8.

**What:** Recast at the regression check (2026-10-18).
**Goal:** `(*take.Take).MatchBeats(ids []int) error` (ids in storyboard order, cut beats included) joins one error per beat the take never started, per started beat not in ids, and per beat started twice, then — when the sets agree — refuses a take whose start order differs from ids; `schedule.New` and `check` use it and `check.matchBeats` is gone; `schedule.New` takes every section's `OutStart` from `board.Clock().SectionStart` and so agrees with the clock by construction.
**Approach (assumed at the header base):** `take` errors are a typed `take.BeatError{Beat int; Problem string}` rendering `beat N: <problem>`; texts are check's: `the take never started it`, `the take started it but the storyboard has no such beat`, `started twice in the take`; order: `the take started its beats in the order 2, 1; the storyboard runs 1, 2 — re-record it` (no beat owner). `schedule` wraps as `schedule: %w` and turns a `BeatError` whose beat is on the board into `storyboard.AtBeat` with the same text. `sectionsOf` keeps its `checkTiming` call (item 14 removes it).
**Regression guard.** schedule wraps each MatchBeats leaf on its own (`schedule: ` around the located or unlocated leaf) and joins the wrapped leaves with errors.Join, so tune's problemsOf splits them one Problem per beat; each line's text is unchanged. `SectionStart` of a cut beat gives its sitting start with no error (item 8's guard), so cut sections keep today's `OutStart`. `schedule.New` and `check.beatScreens` call `MatchBeats` before `BeatSpans`, so the unified `beat N: started twice in the take` leaf fires; `take_test.go` "a beat started twice" still pins `BeatSpans`' own text, left as is.
**Files:** internal/take/take.go, internal/take/take_test.go, internal/schedule/schedule.go, internal/schedule/schedule_test.go, internal/check/check.go, internal/check/check_test.go
**Read first:** internal/take/take.go — Take.BeatSpans, BeatSpan, Take.end; internal/schedule/schedule.go — New, sectionsOf, checkTiming; internal/check/check.go — beatScreens, matchBeats, Judge;
internal/schedule/schedule_test.go — recording, TestNew_RejectsBadSpans, TestNew_SpansFromTake, TestNew_AgreesWithTheStoryboardClipClock; internal/check/check_test.go — TestJudge_BeatsMustAgree, fixtureBoard, fixtureTake;
internal/tune/state.go — problemsOf, beatNamed, beatMention; internal/tune/session.go — Session.rebuild, Session.build; internal/rig/record.go — Record (check.Judge after recording)
**Tests:** take table tests for each mismatch and the order refusal; `TestNew_RejectsBadSpans` and `TestJudge_BeatsMustAgree` updated to the unified texts plus an out-of-order case each; `TestNew_AgreesWithTheStoryboardClipClock` becomes the out-of-order rejection plus an in-order agreement check; a schedule test that two mismatched beats give two joined leaves, each `schedule: `-prefixed.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/take/`
- `GOMEMLIMIT=2GiB go test ./internal/schedule/`
- `GOMEMLIMIT=2GiB go test ./internal/check/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `! grep -n 'func matchBeats' internal/check/check.go`
**Closes:** output-clock; take-screen-lookup
**Commit:** `feat(take): refuse a take whose beats ran out of order`

## 10. Split the tune page's app.js — ✅ DONE (2026-10-18)

NOTES (2026-10-18): re-derived from the assumption that app.js keeps the frame clock and the transport and still fits below 400 lines: with the session API, selection, wiring and window.tune it ran ~500, so the frame clock, transport, playback drawing and keys (frameCount…showFrame, seek/play/pause/tick, jumpSection, renderPlay/renderTime, onKey/onKeyUp) moved to a fifth script, page/transport.js; app.js keeps the contract (time, frameTime, seek, pause delegate to it) and ends at 395 lines.
NOTES (2026-10-18): the split scripts load after app.js and share state through one internal namespace, window.tunePage (state(), refusal(), applyEdit, view, frame, helpers; each split script Object.assigns its own functions); window.tune is unchanged and wire() now runs on DOMContentLoaded, so no /api/state answer is handled before every script has run. renderInspector, renderBeatSelect and the selection stay in app.js (they own selectedBeat/selectedEntry) and call page.beatPanel/framePanel/entryPanel/errorMark.
NOTES (2026-10-18): page_test.go: pageFileNames and entryFieldSenders were replaced by a pageScripts(t) glob over page/*.js (plus pageAssetNames for style.css), which the embed, index-loads, EntryFields and EntryReads tests all read; TestPage_App_ReadOnlyFrameFields_AreStateKeys keeps its name and now reads panels.js; new tests TestPage_SplitScripts_StayBelowTheFileSizeGuide (all five split scripts < 400 lines) and TestPage_App_WiresOnceEverySplitScriptHasRun (index loads app.js first; app.js wires on DOMContentLoaded).
NOTES (2026-10-18): doc.go's file map now names every page asset, as issue tune-page-app-split's acceptance asks. A throwaway headless-chromium run against a camera+captions session fixture showed the clip frame, beat, camera and caption panels, a refused and an accepted inspector edit, seek, frameTime and arrow-key stepping all working (test file deleted afterwards).

**What:**
**Goal:** `internal/tune/page/app.js` and every script split from it are under 400 lines; the `window.tune` contract is unchanged; the page behaves as before.
**Approach (assumed at the header base):** split into `app.js` (the `window.tune` contract, session API, frame clock, transport, wiring), `controls.js` (field controls, `errorRows`, refusal rendering), `panels.js` (beat, frame and zoom panels) and `entries.js` (camera-move and caption panels), loaded from `index.html` in that dependency order the way `lanes.js` is. `page_test.go` greps read the file that now holds each construct.
**Regression guard.** the split covers `app.js` and the scripts it splits into; `timeline.js` (460 lines) is out of scope, as issue `tune-page-app-split` says it needs no edits. No `/api/state` answer is handled before every split script has run: `wire()` starts on `DOMContentLoaded` (or the split scripts load before `app.js`), and they reach app's closure state (`current`, `refusal`, `applyEdit`, `view`) through one internal namespace, never by widening `window.tune`. page_test.go's per-file grep lists (pageFileNames, entryFieldSenders, the EntryReads file list) cover every internal/tune/page/*.js by glob, so later items' checks reach the split files.
**Files:** internal/tune/page/app.js, internal/tune/page/controls.js, internal/tune/page/panels.js, internal/tune/page/entries.js, internal/tune/page/index.html, internal/tune/page_test.go, internal/tune/doc.go
**Read first:** internal/tune/page/app.js — wire, poll, takeState, renderInspector, errorRows, window.tune; internal/tune/page_test.go — pageFileNames, entryFieldSenders, TestPage_EntryReads_AreStateKeys, TestPage_App_ReadOnlyFrameFields_AreStateKeys, TestPage_IndexReferences_AreServed; internal/tune/page/index.html — script tags; internal/tune/page/timeline.js — tune, registeredLanes; internal/tune/page/lanes.js — window.tuneLanes; internal/tune/server.go — pageFiles
**Tests:** the page tests pass reading the new files; a page test that `app.js`, `controls.js`, `panels.js` and `entries.js` are below 400 lines; the per-file grep lists read every `page/*.js` by glob.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `test -z "$(for f in internal/tune/page/app.js internal/tune/page/controls.js internal/tune/page/panels.js internal/tune/page/entries.js; do [ "$(wc -l < "$f")" -lt 400 ] || echo "$f"; done)"`
**Closes:** tune-page-app-split
**Commit:** `refactor(tune): split the page script below the file-size guide`

## 11. Tune attributes problems by owner — ✅ DONE (2026-10-18)

NOTES (2026-10-18): re-derived from the assumption that errorRows/errorMark and the beat select's options live together in app.js: item 10 moved errorRows and errorMark into controls.js, while the camera and caption options of the beat select stay in app.js (renderBeatSelect), so app.js gets the error mark on those options; panels.js and entries.js needed no change, since their errorRows(beat.id / CLIP / {kind, index}) calls keep the page's owner spelling and controls.js maps it onto a problem's owner.
NOTES (2026-10-18): errorMark now takes the same owner errorRows does (beat id, CLIP, or a {kind, index} entry), not only a beat id; both filter through one problemsOn helper.
NOTES (2026-10-18): TestSession_Refresh_CameraProblemNamingABeat_IsOnTheClip renamed to TestSession_Refresh_CameraProblemNamingABeat_IsOnTheMove, since it now asserts owner camera 0 and its old name would be false.
NOTES (2026-10-18): the new pan-target and caption-box session tests write the bad storyboard and Refresh (as the camera test does), since NewSession refuses a storyboard whose first build fails.

Depends on items 7, 9 and 10.

**What:**
**Goal:** tune's `Problem` is `{owner: {kind, index}, message}`, its owner taken from `storyboard.OwnerOf` per joined leaf (a beat owner whose id is not on the board becomes clip); the page shows each problem on its owner's panel only, the Clip frame panel shows clip-level problems, and camera and caption options carry the error mark; no regex, prefix list or `startsWith` attributes a problem. Fixes the misattribution of `compose: camera[i]:` and `caption: captions[i]:` errors.
**Approach (assumed at the header base):** `state.go` `problemsOf` splits `Unwrap() []error` as today, then calls `OwnerOf` per leaf; delete `beatMention`, `clipLevelPrefixes`, `beatNamed`. Page: `errorRows` and `errorMark` compare `problem.owner.kind`/`index`; delete `ENTRY_ERROR_PREFIXES` and `entryErrorPrefix`. `page_test.go` `entryReaders` gains `problem` (Problem) and `owner` (storyboard.Owner).
**Regression guard.** the `problem`/`owner` entry-reader checks reach `controls.js`, `panels.js` and `entries.js` through item 10's glob over every `page/*.js`. `TestSession_ZoomTargetNotOnScreen_KeepsTheLastGoodPipelineAndBlocksSave` and `TestSession_RewrittenTake_IsReadAgain` read `Problem.Beat` today and are rewritten to the owner.
**Files:** internal/tune/state.go, internal/tune/session_test.go, internal/tune/page/controls.js, internal/tune/page/entries.js, internal/tune/page/panels.js, internal/tune/page_test.go, internal/tune/doc.go
**Read first:** internal/tune/state.go — Problem, problemsOf, beatNamed, clipLevelPrefixes, beatMention; internal/tune/session.go — rebuild, build, Save; internal/tune/page/app.js — errorRows, errorMark, entryErrorPrefix, ENTRY_ERROR_PREFIXES, isEntry; internal/tune/page_test.go — entryReaders, jsonKeys, TestPage_EntryReads_AreStateKeys; internal/tune/session_test.go — TestSession_Refresh_CameraProblemNamingABeat_IsOnTheClip, TestSession_ZoomTargetNotOnScreen_KeepsTheLastGoodPipelineAndBlocksSave, TestSession_RewrittenTake_IsReadAgain, newSessionFixture
**Tests:** rewrite `TestSession_Refresh_CameraProblemNamingABeat_IsOnTheClip` to expect owner camera 0, `TestSession_ZoomTargetNotOnScreen_KeepsTheLastGoodPipelineAndBlocksSave` owner beat 2 and `TestSession_RewrittenTake_IsReadAgain` owner clip; new session tests: a pan target not on screen lands on camera 0, an oversized caption box on caption 0, a schedule "never started" on its beat.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `! grep -rnE 'beatNamed|clipLevelPrefixes|ENTRY_ERROR_PREFIXES|entryErrorPrefix' internal/tune/`
**Closes:** located-problems
**Commit:** `fix(tune): show compose and caption problems on their own entry`

## 12. Parse resolves caption and move values — ✅ DONE (2026-10-18)

NOTES (2026-10-18): a left-out caption colour resolves to its default inside validation (`cmp.Or` with the default spelling before `validateColor` parses it) rather than in `applyDefaults`, which is unchanged; the result is the same — `TextColor()`/`BoxColor()` carry the default — and `validateColor` keeps reporting only a set, misspelled colour.
NOTES (2026-10-18): the all-cut refusal is reported and validation carries on, so a board that also has captions or camera moves additionally reports their times as outside the 0s clip (the report-everything-at-once rule).

Depends on item 8.

**What:**
**Goal:** after `storyboard.Parse`, every caption reports its resolved output `Start()` and `Stop()` and its `TextColor()` and `BoxColor()` as `storyboard.RGB`, and every zoom and pan move its resolved `Start()`; a storyboard whose every beat is cut is a Parse problem (`beats: every beat is cut`); authored fields (`Moment`, colour strings) stay for tune.
**Approach (assumed at the header base):** `validateTiming` and `placeMove` store what they resolve (unexported fields) instead of discarding it; `validateColor` returns the parsed `RGB`; defaults from `applyDefaults` are resolved too. Accessors are value methods; a struct-literal board reads zero values (documented).
**Regression guard.** `TestLoadEveryForm` compares `board.Camera` and `board.Captions` with `reflect.DeepEqual`; its `wantCamera`/`wantCaptions` carry the resolved values (same package), or it compares authored fields only.
**Files:** internal/storyboard/caption.go, internal/storyboard/camera.go, internal/storyboard/validate.go, internal/storyboard/storyboard.go, internal/storyboard/storyboard_test.go, internal/storyboard/doc.go
**Read first:** internal/storyboard/caption.go — Caption.validate, validateTiming, validateColor, applyDefaults, validateCaptions; internal/storyboard/camera.go — validateCamera, placeMove, ZoomMove, PanMove; internal/storyboard/storyboard.go — Parse, applyDefaults; internal/storyboard/validate.go — Storyboard.validate; internal/storyboard/storyboard_test.go — TestLoadEveryForm, TestParseMatchesLoad, TestLoadRejects
**Tests:** `TestLoadEveryForm` updated to the resolved fields; resolved start/stop for `for` and `until` captions, beat-anchored moments, default and explicit colours; the all-cut refusal; `cmd/photon` lint tests pass.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run Lint`
**Commit:** `feat(storyboard): resolve caption and move values at parse`

## 13. Caption and compose read resolved values — ✅ DONE (2026-10-18)

NOTES (2026-10-18): consequential edit — internal/compose/compose.go: made necessary by dropping planMoves' timing error; `New`'s doc no longer lists "a camera move whose start is not on the clip's clock" and states the board must come from `storyboard.Parse`.
NOTES (2026-10-18): internal/caption/caption_test.go needed no change: its boards were already built through `storyboard.Parse`, and no caption or compose test exercised the deleted timing, colour or `want a to or a via` error paths, so none was removed.
NOTES (2026-10-18): the compose tests build their captioned and camera-moved boards from YAML through a new `parseBoard` helper (`boardHeader` of the 49 × 14 test frame); `boxCaption` and `cameraTake` now take YAML text, and the struct-literal helpers `at`, `ptr`, `textPoint` and `cellPoint` went with their last callers.

Depends on item 12.

**What:**
**Goal:** `caption.New` and compose's move planning read the storyboard's resolved starts, stops and colours and have no timing or colour error returns; `caption` has no `span` or `parseColor`.
**Approach (assumed at the header base):** `caption/caption.go` `place` uses `Start()/Stop()/TextColor()/BoxColor()` (converted to `color.RGBA` in caption); its only errors are layout ones (`boxRect`, faces, size). `compose/camera.go` `planMoves`/`planPan` take `Start()` and drop their `at:` and `want a to or a via` branches. Tests that build struct-literal boards with captions or camera moves (`camera_test.go`, `compose_test.go`, `cursor_test.go`, `caption_test.go`) build them through `storyboard.Parse` of YAML text instead.
**Regression guard.** `internal/caption/doc.go` says [New] resolves each caption's start and end and maps `caption.go` to "each caption's span on the clock … and its colours"; every caption comment saying it resolves times or parses colours is reworded (`grep -n 'resolve\|span\|colour' internal/caption/*.go`).
**Files:** internal/caption/caption.go, internal/caption/caption_test.go, internal/compose/camera.go, internal/compose/camera_test.go, internal/compose/compose_test.go, internal/compose/cursor_test.go, internal/caption/doc.go
**Read first:** internal/caption/caption.go — New, place, span, parseColor; internal/compose/camera.go — planMoves, planPan; internal/compose/compose.go — New; internal/compose/camera_test.go — cameraTake, TestNewRefusesAPanTargetItsMoveNeverShows; internal/compose/compose_test.go — boxCaption, zoomClickTake, newTestCompositor; internal/compose/cursor_test.go — TestComposeDrawsACaptionOverTheCursor; internal/caption/caption_test.go — parseBoard, boardHeader; internal/caption/doc.go
**Tests:** existing caption and compose behaviour tests pass on parsed boards; tests of deleted error paths are removed.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/caption/`
- `GOMEMLIMIT=2GiB go test ./internal/compose/`
- `! grep -nE 'func (span|parseColor)\(' internal/caption/*.go`
**Commit:** `refactor(caption): read resolved times and colours`

## 14. Schedule and tune stop re-checking validated values — ✅ DONE (2026-10-18)

NOTES (2026-10-18): internal/storyboard/storyboard_test.go needed no change: `TestLoadRejects` already covers a zero duration (`duration missing`) and a hold not under its duration (`hold not under duration`), and the all-cut refusal (`beats: every beat is cut`) has its own test from item 12, so the three removed `TestNew_RejectsBadSpans` cases were dropped without a storyboard copy.
NOTES (2026-10-18): tune's `movedState` now takes the move's resolved start (`zoom.Start()`/`pan.Start()`) in place of the output clock; a caption's StopMS is `Stop()` for both `for` and `until` captions, so the until branch only fills the authored `Until` field.

Depends on items 9 and 12.

**What:**
**Goal:** `schedule` has no `checkTiming` and no all-cut error (its doc states the board comes from `storyboard.Parse`); tune's `clipMS` has no error branch and reads resolved starts.
**Approach (assumed at the header base):** delete `checkTiming` and the `every beat is cut` branch from `schedule.New`/`sectionsOf`; their `TestNew_RejectsBadSpans` cases move to storyboard tests where storyboard does not already cover them. `tune/state.go` camera and caption states read `Start()`/`Stop()`; `clipMS` goes.
**Regression guard.** `internal/schedule/doc.go` says "a schedule whose every beat is cut is an error"; every schedule comment naming the all-cut error or the duration/hold checks is reworded (`grep -n 'every beat is cut\|positive duration\|hold in' internal/schedule/*.go`, `New`'s doc included).
**Files:** internal/schedule/schedule.go, internal/schedule/schedule_test.go, internal/storyboard/storyboard_test.go, internal/tune/state.go, internal/schedule/doc.go
**Read first:** internal/schedule/schedule.go — New, sectionsOf, checkTiming; internal/schedule/schedule_test.go — TestNew_RejectsBadSpans, recording; internal/schedule/doc.go; internal/tune/state.go — clipMS, movedState, cameraStates, captionStates; internal/storyboard/storyboard_test.go — TestLoadRejects
**Tests:** schedule tests minus the removed cases; storyboard tests cover zero duration, hold not under duration and all cut.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/schedule/`
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `! grep -rn 'checkTiming\|clipMS' internal/`
**Commit:** `refactor(schedule): trust the parsed storyboard's timing`

## 15. photon lint runs the font-bound tier — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the size/unit maths is `compose.Output(layout, width) (size, unit, err)`, called by `compose.New`; the Compositor keeps the unit as a field and `Compositor.unit()` is gone (compose_test reads the field). render's cell layout moved into a `cellLayout` helper that `newCompositor` and `CheckCaptions` share.
NOTES (2026-10-18): `CheckCaptions` returns nil without loading any font when the storyboard has no captions, and files caption.New's located problem as a one-problem `storyboard.ValidationError` under the storyboard path, so lint prints it as `<path>: 1 problem(s)` / `  captions[N]: ...` like Parse's problems; caption.New stops at the first caption that does not fit, so only that one is reported.
NOTES (2026-10-18): consequential edit — internal/compose/compose_test.go: made necessary by removing `Compositor.unit()` in favour of the `unit` field
NOTES (2026-10-18): consequential edit — internal/compose/doc.go: made necessary by the new exported `compose.Output` (file map and caption paragraph)
NOTES (2026-10-18): consequential edit — internal/render/render.go: made necessary by `CheckCaptions` sharing newCompositor's cell layout (`cellLayout`)
NOTES (2026-10-18): README also updated in the `photon lint` row of the commands table; `photon lint --help` long text names the caption-box check.

Depends on items 5 and 13.

**What:**
**Goal:** `photon lint` lays the captions out with the clip's real fonts and reports a caption box larger than the frame (owner caption, today's text) with exit code 1; the three tiers are named in the package docs: static (`storyboard.Parse`), font-bound (`render.CheckCaptions`), take-bound (`compose.New` targets).
**Approach (assumed at the header base):** `render.CheckCaptions(loaded *clip.Clip) error` builds the rasterizer geometry and caption faces as `Build` does and calls `caption.New` with the shipped frame size and unit; the size/unit maths moves out of `compose.New`/`Compositor.unit` into one exported compose helper both call. `cmd/photon/lint.go` runs it after `clip.Load` succeeds and prints its problems in the same form.
**Regression guard.** item 15 updates the Note `package-layout` `render` row if render now imports `caption` (add the Note to Files). The lint fixture is a new `cmd/photon/testdata/invalid/caption-box.yaml`, run with `--config` `validConfig`, whose caption keeps `font.size` ≤72 (above 72 is already a Parse problem) with a text long enough (about 40 characters at frame width 600) for its box to exceed the frame; the test asserts `larger than the`. `README.md` names the box-larger-than-frame check in the lint list of the Captions section and beside its `photon render` sentence.
**Files:** internal/render/check.go, internal/render/check_test.go, internal/render/doc.go, internal/compose/compose.go, cmd/photon/lint.go, cmd/photon/lint_test.go, internal/storyboard/doc.go, cmd/photon/testdata/invalid/caption-box.yaml, README.md, .capcom/knowledge/package-layout.md
**Read first:** cmd/photon/lint.go — newLintCommand; internal/compose/compose.go — New, Compositor.unit, Compositor.drawCursor; internal/caption/caption.go — New, place; internal/caption/layout.go — boxRect; internal/render/render.go — NewRasterizer, NewCompositor; cmd/photon/lint_test.go — TestLint, runLint, TestLint_ValidClipLintsCleanWithoutAHomeDir; README.md — Captions section
**Tests:** lint test: `caption-box.yaml` (font.size ≤72, a long text) fails with `captions[0]:`, `larger than the` and exit 1; the clean every-form and minimal boards still pass; render test for `CheckCaptions`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/render/`
- `GOMEMLIMIT=2GiB go test ./internal/compose/`
- `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run Lint`
- `grep -n 'font-bound' internal/storyboard/doc.go internal/render/doc.go`
**Closes:** resolve-at-parse
**Commit:** `feat(lint): check caption layout with the real fonts`

## 16. yamledit answers queries and upserts — ✅ DONE (2026-10-18)

NOTES (2026-10-18): Upsert refuses a mapping path that names a sequence (`Upsert(beats, "0", …)`) with ErrKind rather than Setting the sequence item, and DeleteIfPresent treats only ErrNotFound as absent — a path stepping into a scalar keeps Delete's ErrKind.

**What:** Recast at the regression check (2026-10-18).
**Goal:** `yamledit.Document` has `Has(path) bool`, `Scalar(path) (string, bool)` (the scalar's decoded value, no quotes), `Upsert(mapping Path, key, value string, order []string) error` and `DeleteIfPresent(path) error`, working on block, flow and sequence-item mappings.
**Approach (assumed at the header base):** new `internal/yamledit/query.go` (on the existing `locate`/`child`) and `upsert.go`. Upsert: a present scalar is `Set` (keeping its refusals), a present collection is `Replace`d; an absent key is `Insert`ed after the present keys that precede it in `order` (the mapping's full key order), and appended at the mapping's end when `order` does not name it. DeleteIfPresent returns nil for an absent key.
**Regression guard.** `Scalar(path)` returns the scalar's decoded value (no quotes), not its source text.
**Files:** internal/yamledit/query.go, internal/yamledit/upsert.go, internal/yamledit/upsert_test.go, internal/yamledit/doc.go
**Read first:** internal/yamledit/document.go — Document.locate, child, Insert, lastPresent, Set, Delete, apply; internal/yamledit/replace.go — Replace, replaceSpan, blockValueSpan; internal/yamledit/scan.go — scalarSpan; internal/yamledit/document_test.go; internal/yamledit/doc.go — file map
**Tests:** table tests over block, flow and `- id:` sequence-item mappings: upsert present/absent/unordered key, delete present/absent, `Scalar` of plain and quoted text (both decoded, no quotes), `Has` of a sequence index; every other byte kept.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/yamledit/`
**Commit:** `feat(yamledit): add query, upsert and delete-if-present`

## 17. Tune edits through yamledit's own tree — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the whole-node replacers (setMoment, setCaptionUntil, setCaptionText) share a new splicer.replaceOrInsert in edit.go: Has + Replace when the key is present, and for the absent case Upsert (exactly Insert after the order's preceding keys) in place of a literal Insert, so no tune-side copy of keysBefore survives; setCaptionText now passes captionKeyOrder where it passed nil (text is first in that order, so the insert point is unchanged).
NOTES (2026-10-18): replaceTarget now replaces a target-only zoom whole with the flow `{target: …}` for a flow zoom too (previously delete + insert of its target, same result), and a target-only block zoom is Replaced in its place rather than deleted and re-inserted in beat key order; edit_test.go passes unchanged. internal/tune/doc.go needed no change (it never described the node tree).

Depends on item 16.

**What:** Recast at the regression check (2026-10-18).
**Goal:** `internal/tune` keeps no `yaml.Node` tree for editing: no `splicer.root`, `lookup`, `childNode`, `writeScalar`, `deleteKey` or `keysBefore`, and `edit.go` no longer parses the source a third time; every case in `edit_test.go` passes unchanged.
**Approach (assumed at the header base):** helpers call `Upsert`/`DeleteIfPresent`/`Has`; `writeDuration` reads the old value with `Scalar` to keep its unit; `replaceTarget` keeps the one branch the guard names. Multi-step edits (`setCaptionFor`/`setCaptionUntil`) read presence from the document after each splice.
**Regression guard.** replaceTarget keeps one branch — when the zoom holds only its target (Has of zoom.factor, zoom.in and zoom.out all false) Replace the whole `zoom` with the flow form `{target: …}` (zoomOnto), otherwise Replace `zoom.target` in place; writeDuration passes formatDuration the decoded scalar value from Scalar; widen the Acceptance grep to `! grep -nE 'yaml\.Node|childNode|keysBefore|writeScalar|deleteKey|\.lookup\(' internal/tune/edit*.go`. The whole-node replacers setCaptionText, setMoment and setCaptionUntil keep `Has` + `Replace` when the key is present, since Upsert's `Set` refuses a block scalar (scan.go scalarSpan) and `text: |` must still be replaced; Upsert serves only writeScalar/writeDuration's cases. Insert stays for the absent case, with captionKeyOrder/order.
**Files:** internal/tune/edit.go, internal/tune/edit_camera.go, internal/tune/edit_caption.go, internal/tune/doc.go
**Read first:** internal/tune/edit.go — apply, splicer, writeDuration, writeScalar, deleteKey, replaceTarget, addOrRemoveZoom, lookup, keysBefore; internal/tune/edit_caption.go — setCaptionText, setCaptionFor, setCaptionUntil, setPlacement;
internal/tune/edit_camera.go — setMoment, setEase, setZoomTo, setPanTo; internal/tune/value.go — formatDuration; internal/tune/edit_test.go — TestApply_Edit_ChangesOnlyTheEditedText, TestApply_CaptionEdit_ChangesOnlyTheEditedText, captionEditBoard;
internal/tune/testdata/board.yaml — beats 2, 4, 6; internal/yamledit/scan.go — scalarSpan
**Tests:** `edit_test.go` unchanged and passing.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `git diff --quiet HEAD -- internal/tune/edit_test.go`
- `! grep -nE 'yaml\.Node|childNode|keysBefore|writeScalar|deleteKey|\.lookup\(' internal/tune/edit*.go`
**Closes:** yamledit-upsert
**Commit:** `refactor(tune): edit through yamledit's own tree`

## 18. Typed edit wire for beats and the frame

Depends on items 11 and 17.

**What:** Recast at the regression check (2026-10-18).
**Goal:** `POST /api/edit` takes `{"owner": {kind, index}, "field", "value"}` with `value` raw JSON; beat and frame fields take typed values — `duration`, `hold`, `zoom.in`, `zoom.out` integer milliseconds, `frame.width`, `frame.fps` integers, `cut` a bool, `zoom.factor` a number (0 = auto), `zoom` and `zoom.target` a target `{text, nth?, area?}` (`zoom: null` removes the zoom); camera and caption values are still JSON strings holding today's text.
**Approach (assumed at the header base):** tune's `Edit` becomes `{Owner storyboard.Owner; Field string; Value json.RawMessage}`; `apply` decodes per field; the beat/frame branches of `value.go` become JSON decoders. Page: every sender builds the owner object; `timeline.js` drags and `panels.js` send typed beat/frame values; `picker.js` sends the pick response's typed `target` for `zoom` and `zoom.target`.
**Regression guard.** per the header's **Inputs** call, item 18 replaces the free-text zoom and zoom.target inputs with structured target inputs (picks fill them), and nth uses the JSON encoding the pick response's typed target already uses. Tune tests that build `Edit` literals (`frame_test.go`, `pick_test.go`, `session_test.go`) move to the new shape: `pick_test.go`'s zoom-target apply sends the proposal's typed target JSON, `session_test.go`'s camera.at apply a JSON string. Every page sender, `applyEdit` and `ownerKey` included (`app.js`, `controls.js`, `entries.js`), uses `{owner, field, value}`, since `decodeBody` refuses unknown fields. A JSON null or absent `value` is refused for every field except `zoom` (null removes the zoom) and `caption.until` (item 20: null clears it), with a test of a null duration refused; on the wire — edits and pick responses alike — a target's `nth` is an integer ≥1 or the string "last", absent meaning first; the Go side's target YAML speller writes `nth: last` for the last match, and pick.go's targetFlow uses the same speller so it no longer drops it (test: a picked last-match target round-trips).
**Files:** internal/tune/server.go, internal/tune/edit.go, internal/tune/value.go, internal/tune/edit_test.go, internal/tune/server_test.go, internal/tune/page/timeline.js, internal/tune/page/panels.js, internal/tune/page/picker.js, internal/tune/frame_test.go, internal/tune/pick_test.go, internal/tune/session_test.go, internal/tune/page/app.js, internal/tune/page/controls.js, internal/tune/page/entries.js, internal/tune/pick.go
**Read first:** internal/tune/edit.go — Edit, editName, splicer.apply, editFrame, editBeat, addOrRemoveZoom, replaceTarget; internal/tune/server.go — handleEdit, decodeBody, pickTarget, responseOf; internal/tune/pick.go — targetFlow, snippetsOf;
internal/tune/page/app.js — edit, applyEdit, ownerKey, zoomPanel, factorField, millisecondsField, countField, cutField; internal/tune/page/picker.js — setAsZoomTarget, setAsPanTarget; internal/tune/page/timeline.js — endDrag;
internal/tune/edit_test.go — TestApply_Edit_ChangesOnlyTheEditedText, TestApply_WireSpellings; internal/storyboard/beat.go — Nth, NthLast, Nth.UnmarshalYAML
**Tests:** edit_test cases for beat/frame fields on typed values; refusals for a wrong JSON type; a null `duration` value refused; a zoom target with `"nth": "last"` written as `nth: last`; a pick_test that a picked last-match target round-trips through the proposal and its apply; a server test posting the exact JSON the page sends for a duration drag and a picked zoom target.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Commit:** `feat(tune): send typed beat and frame edits`

## 19. Typed edit wire for camera moves

Depends on item 18.

**What:** Recast at the regression check (2026-10-18).
**Goal:** camera fields take typed values: `camera.at` a moment `{beat, offset_ms}` (beat 0 = clip time), `camera.over` integer milliseconds, `camera.ease` an enum string, `camera.to` a target `{text, nth?, area?}` or a point `{cell: {x, y}}` on a pan, and a number (the factor) on a zoom move.
**Approach (assumed at the header base):** `edit_camera.go` decodes per field and spells the YAML itself; `value.go` loses the camera string parsers. `entries.js` camera panel and `picker.js` (`camera.to`) send typed values.
**Regression guard.** camera.at gets the structured moment input and camera.to the structured target input (picks fill it), per the Inputs call. `camera.at` decodes its own typed moment; `setMoment`'s string path and `parseMoment` stay for `caption.at`/`caption.until` until item 20. `session_test.go`'s camera.at apply sends `{"beat":1,"offset_ms":150}`. The null refusal of item 18 applies to every camera field; `camera.at: null` is refused. `camera.to` takes item 18's nth rule ("last" written `nth: last`), and the edit_test case "a flow pan cell becomes a target" keeps `nth: last` on a typed value.
**Files:** internal/tune/edit_camera.go, internal/tune/value.go, internal/tune/edit_test.go, internal/tune/server_test.go, internal/tune/page/entries.js, internal/tune/page/picker.js, internal/tune/session_test.go, internal/tune/page/controls.js
**Read first:** internal/tune/edit_camera.go — editCamera, setMoment, setZoomTo, setPanTo; internal/tune/value.go — parseMoment, parsePoint, parseDecimal; internal/tune/page/app.js — cameraPanel, aimRow, textField, momentText; internal/tune/page/picker.js — setAsPanTarget, selectedPan;
internal/tune/edit_test.go — cameraEdit, TestApply_CameraEdit_ChangesOnlyTheEditedText, TestApply_CameraWireSpellings; internal/tune/session_test.go — TestSession_CameraEdit_MovesTheMoveAndUndoes;
internal/tune/state.go — MomentState, CellState, TargetState; internal/storyboard/moment.go — Moment
**Tests:** edit_test camera cases on typed values; a `camera.at: null` refusal case; the flow-pan-cell case with a typed `"nth": "last"` target written `nth: last`; a server test posting the picker's exact `camera.to` JSON.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Commit:** `feat(tune): send typed camera edits`

## 20. Typed edit wire for captions

Depends on item 19.

**What:** Recast at the regression check (2026-10-18).
**Goal:** caption fields take typed values (`caption.at` and `caption.until` moments, `caption.until: null` clears it, `caption.for` integer milliseconds, `caption.text` a string, `caption.place`/`caption.align` enum strings); tune parses no YAML text from the page and the page spells no YAML.
**Approach (assumed at the header base):** `edit_caption.go` decodes per field; delete `value.go`'s `parseMoment`, `parseTarget`, `parsePoint`, `parseFlowMapping`, `oneLineNode` and the JS `momentText`, `flowTarget`, `quoted`.
**Regression guard.** caption.at and caption.until get the structured moment input, per the Inputs call. The null refusal of item 18 applies to every caption field except `caption.until` (null clears it, per item 18); a `caption.at: null` edit is refused at decode with the field's `want …` error, and an edit_test case pins it beside the `caption.until: null` one.
**Files:** internal/tune/edit_caption.go, internal/tune/value.go, internal/tune/edit_test.go, internal/tune/page/entries.js, internal/tune/page/controls.js, internal/tune/page_test.go
**Read first:** internal/tune/edit_caption.go — editCaption, setCaptionUntil, setCaptionFor, setCaptionText, setPlacement; internal/tune/value.go — parseMoment, oneLineNode, parseFlowMapping, formatDuration;
internal/tune/edit_test.go — captionEdit, captionEditBoard, TestApply_CaptionWireSpellings, TestApply_CaptionEditFailingValidation_IsRefused, TestSession_CaptionEdit_SwapsTheEndAndUndoes;
internal/tune/page/app.js — captionPanel, captionForField, captionUntilField, momentText, flowTarget, quoted, MOMENT_PLACEHOLDER; internal/storyboard/caption.go — Caption.validateTiming
**Tests:** edit_test caption cases on typed values; a `caption.until: null` clear case; a `caption.at: null` refusal case.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `! grep -rnE 'momentText|flowTarget|parseFlowMapping|oneLineNode' internal/tune/`
- `! grep -rnE 'MOMENT_PLACEHOLDER|TARGET_PLACEHOLDER|\bquoted\(' internal/tune/page/`
- `! grep -n 'yaml\.' internal/tune/value.go internal/tune/edit_caption.go`
**Commit:** `feat(tune): send typed caption edits`

## 21. Storyboard applies typed beat and frame edits

Depends on item 20.

**What:**
**Goal:** `storyboard.Apply(src []byte, path string, edit Edit) ([]byte, *Storyboard, error)` with `Edit{Owner; Field string; Value json.RawMessage}` applies every beat and frame field, splicing through `yamledit` with key order read from the storyboard's yaml struct tags; tune routes beat and frame edits to it and holds no beat/frame key-order table or field rule.
**Approach (assumed at the header base):** new `internal/storyboard/edit.go` (Edit, Apply, a private `keyOrder(reflect.Type)` over yaml tags skipping `-` and inline) and `edit_beat.go`, moving tune's `editBeat`, `editFrame`, `setRamp`, `setCut`, `setFactor`, `addOrRemoveZoom`, `replaceTarget`, `writeDuration` and their tests; `beatKeyOrder`, `zoomKeyOrder`, `frameKeyOrder` go. Update Note `package-layout`: storyboard imports `keys`, `yamledit`; tune's row.
**Regression guard.** `Apply` takes the path and passes it to both `Parse` calls, as tune's `apply(src, path, edit)` does, so a refusal keeps its file prefix and the board its `Path`. `keyOrder(Beat)` is pinned as today's `beatKeyOrder` plus `expect` (insert positions unchanged, `expect` follows `zoom`); zoom and frame orders match exactly. `testdata/board.yaml` moves by `git mv` from `internal/tune/testdata/`, whose only reader is the moving beat/frame cases. The storyboard `doc.go` sentence "It imports only keys" is rewritten, and the Note's `yamledit` row moves above `storyboard`'s, since dependencies point down the table.
**Files:** internal/storyboard/edit.go, internal/storyboard/edit_beat.go, internal/storyboard/edit_test.go, internal/storyboard/doc.go, internal/tune/edit.go, internal/tune/edit_test.go, internal/tune/doc.go, .capcom/knowledge/package-layout.md, internal/storyboard/testdata/board.yaml, internal/tune/testdata/board.yaml
**Read first:** internal/tune/edit.go — apply, splicer.apply, editBeat, editFrame, writeDuration, setRamp, setCut, setFactor, addOrRemoveZoom, beatKeyOrder; internal/storyboard/storyboard.go — Parse, ValidationError; internal/storyboard/beat.go — Beat, Zoom, DefaultZoomRamp; internal/tune/session.go — Session.Apply; internal/tune/edit_test.go — TestApply_Edit_ChangesOnlyTheEditedText, TestApply_InvalidResult_IsRefusedWithTheValidationError, readBoard, boardPath; .capcom/knowledge/package-layout.md — package table; internal/storyboard/doc.go — package comment
**Tests:** moved beat/frame cases pass in storyboard; a test that `keyOrder` reproduces today's zoom and frame orders and today's beat order plus `expect`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `capcom check`
- `! grep -rnE 'beatKeyOrder|zoomKeyOrder|frameKeyOrder' internal/tune/`
**Commit:** `refactor(storyboard): own typed beat and frame edits`

## 22. Storyboard applies typed camera and caption edits

Depends on item 21.

**What:**
**Goal:** `storyboard.Apply` applies every camera and caption field; `internal/tune` holds no edit engine (no `edit_camera.go`, `edit_caption.go`, `value.go`, key-order table or field constant beyond decoding the wire) and only forwards `Edit`s to `storyboard.Apply`.
**Approach (assumed at the header base):** move `edit_camera.go` and `edit_caption.go` into `internal/storyboard/edit_camera.go` and `edit_caption.go` with their tests; `zoomMoveKeyOrder`, `panMoveKeyOrder`, `captionKeyOrder` come from `keyOrder`; `editFields` moves with them.
**Regression guard.** add internal/tune/session_test.go, internal/tune/frame_test.go and internal/tune/pick_test.go to Files — tune tests that build Edits use the field names storyboard exports. `TestPage_EntryFields_AreEditFields` reads storyboard's exported field list in place of `editFields` until item 24 deletes it. The moved camera and caption tests carry their own copy of the `sessionStoryboard`/`cameraCaptionsStoryboard` base text in `storyboard/edit_test.go`; the deleted `tune/value.go`, `edit_camera.go` and `edit_caption.go` are listed in Files.
**Files:** internal/storyboard/edit_camera.go, internal/storyboard/edit_caption.go, internal/storyboard/edit_test.go, internal/storyboard/doc.go, internal/tune/edit.go, internal/tune/edit_test.go, internal/tune/doc.go, internal/tune/page_test.go, internal/tune/session_test.go, internal/tune/frame_test.go, internal/tune/pick_test.go, internal/tune/value.go, internal/tune/edit_camera.go, internal/tune/edit_caption.go
**Read first:** internal/tune/edit_camera.go — editCamera, setMoment, setPanTo, zoomMoveKeyOrder, panMoveKeyOrder; internal/tune/edit_caption.go — editCaption, captionKeyOrder, placeSpellings, alignSpellings; internal/tune/edit.go — editFields, editName, isCameraField, isCaptionField; internal/tune/page_test.go — TestPage_EntryFields_AreEditFields; internal/tune/edit_test.go — cameraEditBoard, captionEditBoard, TestSession_CaptionEdit_SwapsTheEndAndUndoes; internal/tune/session_test.go — sessionStoryboard, cameraCaptionsStoryboard; internal/storyboard/camera.go — ZoomMove, PanMove
**Tests:** moved camera and caption cases pass in storyboard; tune keeps its session-level edit tests.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `test ! -e internal/tune/value.go && test ! -e internal/tune/edit_camera.go && test ! -e internal/tune/edit_caption.go`
**Commit:** `refactor(storyboard): own typed camera and caption edits`

## 23. Storyboard describes its editable fields

Depends on item 22.

**What:**
**Goal:** `storyboard.Fields(board *Storyboard) []Field` lists every editable and read-only field per owner — `Field{Owner; Name; Kind (duration|count|decimal|bool|enum|text|moment|target|point); Choices []string; Default json.RawMessage; ReadOnly bool}` — from its struct tags, `applyDefaults` and enums; `/api/state` carries it as `fields`.
**Approach (assumed at the header base):** new `internal/storyboard/fields.go`; tune `State` gains `Fields []storyboard.Field` (`json:"fields"`); `readOnlyFrameFields` stays in tune until item 24 deletes it with its test.
**Regression guard.** read-only clip Field names are the State json keys (cols, rows, …), and a Go test asserts every ReadOnly clip Field name is a State json key (the guard TestPage_App_ReadOnlyFrameFields_AreStateKeys gave). `Field`'s json tags are `owner`, `name`, `kind`, `choices`, `default`, `read_only`. The round trip applies a valid sample per kind and owner where the Default is refused and lists the expected refusals on `every-form.yaml`: `cut` false on cut beat 4, `duration`/`frame.width`/`caption.for` 0, `zoom: null` on zoomless beats 1 and 4, `camera.to` on the via pan `camera[4]`.
**Files:** internal/storyboard/fields.go, internal/storyboard/fields_test.go, internal/storyboard/doc.go, internal/tune/state.go, internal/tune/session_test.go
**Read first:** internal/storyboard/caption.go — Caption.applyDefaults, Place, Align, Caption.validateTiming; internal/storyboard/camera.go — Ease, PanMove; internal/storyboard/beat.go — Zoom, DefaultZoomRamp; internal/storyboard/validate.go — Beat.validate; internal/storyboard/testdata/every-form.yaml; internal/tune/state.go — State, Session.State; internal/tune/page_test.go — readOnlyFrameFields, TestPage_App_ReadOnlyFrameFields_AreStateKeys, jsonKeys
**Tests:** a round-trip test over `testdata/every-form.yaml`: for every non-read-only Field, `Apply` of its Default (or, where the Default is refused as listed, a valid sample of its kind and owner) yields a source that `Parse` accepts; a tune test that every ReadOnly clip Field name is a `State` json key; every `Edit` field name `Apply` accepts appears in `Fields`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Commit:** `feat(storyboard): describe the editable fields`

## 24. The tune page renders fields from the description

Depends on item 23.

**What:** Recast at the regression check (2026-10-18).
**Goal:** the page reads enum choices, defaults and read-only flags from `state.fields` and holds no enum list or default value of a storyboard field.
**Approach (assumed at the header base):** delete `EASES`, `PLACES`, `ALIGNS`, `READ_ONLY_FRAME_FIELDS` and their uses in `controls.js`/`panels.js`/`entries.js`; delete `TestPage_App_ReadOnlyFrameFields_AreStateKeys` (with `readOnlyFrameFields`); `TestPage_EntryReads_AreStateKeys` gains `field` (storyboard.Field).
**Regression guard.** keep DEFAULT_FACTOR (it is the page's manual-factor seed, not a storyboard default) — drop it from the delete list, the Goal and the Acceptance grep; the EntryReads reader regex excludes a preceding "." so a chain like `change.field.split` is not read as key "split". `framePanel` prints `current[field.name]` for each ReadOnly clip Field, valid by item 23's rule that read-only clip Field names are `State` json keys. Keep `TestPage_EntryFields_AreEditFields`, reading the field names from storyboard's `Fields` instead of `editFields`: it stays the one check that every `camera.*`/`caption.*` literal the page sends is an editable Field and that every entry Field is sent, which item 23's round trip (Fields against Apply) does not cover.
**Files:** internal/tune/page/controls.js, internal/tune/page/panels.js, internal/tune/page/entries.js, internal/tune/page_test.go
**Read first:** internal/tune/page_test.go — TestPage_EntryReads_AreStateKeys, entryReaders, jsonKeys, TestPage_EntryFields_AreEditFields, entryFieldName, TestPage_App_ReadOnlyFrameFields_AreStateKeys;
internal/tune/page/app.js — EASES, PLACES, ALIGNS, READ_ONLY_FRAME_FIELDS, framePanel, choiceField, cameraPanel, captionPanel, factorField, ownerKey; internal/tune/state.go — State, CameraState
**Tests:** remaining page tests pass; `TestPage_EntryFields_AreEditFields` passes against the names in `storyboard.Fields`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `! grep -rnE 'EASES|PLACES|ALIGNS|READ_ONLY_FRAME_FIELDS' internal/tune/page/`
**Closes:** typed-storyboard-edits
**Commit:** `refactor(tune): render the inspector from the field description`
