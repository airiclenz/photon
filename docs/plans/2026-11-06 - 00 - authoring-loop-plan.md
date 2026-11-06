# Authoring loop plan

**Goal:** Shorten the edit-and-look loop. `photon tune` starts faster, drafts its full view, and
exports the GIF itself. `photon render --watch` re-renders on change, and long captions wrap.
**Date:** 2026-11-06
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 503cfce

**Regression check (2026-11-06, 503cfce):**
- 1: guard folded (decision: small largeTake for tests, benchmark-sized take for BenchmarkLoad only)
- 2: guard folded
- 3: guard folded
- 4: recast; supersedes CHANGELOG.md 0.2.0 "Playing or scrubbing shows the normal preview frames without the overlay" in part
- 5: guard folded
- 6: guard folded; supersedes `internal/tune/state.go` State doc "Version rises on every change" in part (export progress is the exception)
- 7: guard folded
- 8: guard folded
- 9: guard folded
- 10: guard folded
- 4 (re-check): guard folded (TestPageJS_SelectAZoomAfterAPosition_ReturnsToTheCamerasFrame want rewritten to draftFrame(start), exactFrame(start))

**Sources:**
- `.capcom/issues/tune-startup-time.md`, `tune-full-view-draft.md`, `tune-gif-export.md`,
  `render-watch.md`, `caption-text-wrapping.md`
- capcom Notes `coding-conventions`, `package-layout`, `glossary`, `extension-model`
- `README.md` (`## Commands`, `### Captions`)

**Ratified design calls** (user, 2026-11-06):
- **Item set:** these five issues. Left out: tune-frame-size-edit (cols×rows is the recorded
  pty size), npm/winget (outside accounts), cut-beat-camera, drag-action-options,
  caption-char-styling, tune-browser-tests, tune-lane-reorder.
- **GIF export:** needs a clean session (saved, no problems, not stale). It renders the file on
  disk, so the GIF is byte-identical to `photon render`.
- **Watch:** stdlib polling of size and mtime. No new dependency.
- **Wrap width:** automatic, to the frame width minus the edge margins. An optional caption
  `width:` (design pixels, box outer width) narrows it.

**Standing requirements:**
- skills: coding-standards
- Pi host: run single packages with `GOMEMLIMIT=2GiB`; never `./...` with `-race`; one test
  command at a time.
- A test that needs ffmpeg skips when ffmpeg is absent.
- A package past ~10 files keeps its `doc.go` file map current (`docmap_test.go`).

**Out of scope:**
- tune editing of the new caption `width` (YAML only)
- progress output on the `photon render` CLI
- serialising a tune export against a concurrent `photon render` from a terminal

## 1. Take load benchmark — ✅ DONE (2026-11-06)

NOTES (2026-11-06): baseline on the Pi 5 host, `GOMEMLIMIT=2GiB go test ./internal/take -bench BenchmarkLoad -run '^$' -benchtime 3x`: BenchmarkLoad-4 6074185242 ns/op, 721321069 B/op, 4300746 allocs/op (160×48 take, 600 snapshots)

**What:**
**Goal:** `go test ./internal/take -bench BenchmarkLoad -run '^$'` measures `take.Load` on a
large take. A shared test helper builds that take for later items' equivalence tests.
**Approach (assumed at the header base):** A new `internal/take/file_bench_test.go`. Its
helper `largeTake(cols, rows, snapshots int) *Take` builds a 160×48 take with 600 snapshots.
Cells mix palette, RGB and default colours, attributes and wide graphemes. It also has events
spread over the take. The benchmark `Save`s the take once to `b.TempDir()` and times `Load` in
a `b.Loop()` with `b.ReportAllocs()`. Record the baseline `ns/op` as a dated NOTES line under
this item.
**Regression guard.** Only BenchmarkLoad uses the 160×48×600 take; TestLargeTakeRoundTrips and every later equivalence/race test use a small largeTake (about 40×12 with 20 snapshots) so tests stay fast under -race on the Pi.
**Files:** internal/take/file_bench_test.go
**Read first:** internal/take/file.go — Save, Load, Write; internal/take/file_test.go — sampleTake,
encodeTake; internal/take/take.go — Take, Snapshot; internal/tune/server_test.go — draftBenchTake
**Tests:** the benchmark itself, plus `TestLargeTakeRoundTrips` (Save then Load of the small
`largeTake` gives a `reflect.DeepEqual` take).
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/take -run TestLargeTakeRoundTrips`
- `GOMEMLIMIT=2GiB go test ./internal/take -bench BenchmarkLoad -run '^$' -benchtime 3x`
**Commit:** `test(take): benchmark loading a large take`

## 2. Decode take lines in parallel

**What:**
**Goal:** `take.Read` returns a take `reflect.DeepEqual` to the sequential decoder's on every
stream `Write` produces. Every existing error case keeps its sentinel and its `line N` wording.
`BenchmarkLoad` runs at most half the item-1 baseline `ns/op` on a host with ≥4 CPUs. Record
the after figure as a NOTES line.
**Approach (assumed at the header base):**
- Change `Read`/`readLines` in `internal/take/file.go`.
- Read the gunzipped stream line by line with `bufio.Reader.ReadBytes('\n')`; `json.Encoder`
  ends each value with one newline. The first line goes through `readHeader`'s checks; adapt it
  to take the line's bytes.
- Decode the later lines on `min(runtime.GOMAXPROCS(0), 4)` workers. Each worker decodes one
  line with a strict `json.Decoder` (`DisallowUnknownFields`) into `line`.
- Results are kept in line order. Lines in flight are capped (≤ 4× workers) to bound memory on
  the Pi.
- The lowest-numbered failing line's error is returned: `"%w: line %d: %w"`, `ErrFormat`.
- A truncated gzip stream still fails.
- Skip only whitespace-only trailing input.
- Keep the old sequential `readLines` as a test-only reference decoder in `file_test.go`.
**Regression guard.** Equivalence and -race tests use the small largeTake from item 1, never the benchmark-sized take.
On any error Read stops the reader and drains or cancels the workers before it returns (tune's
`refreshTake` reloads half-written takes). A line holding anything after its one JSON value
(`dec.More` / trailing bytes) is `ErrFormat` with its line number, never silently dropped.
**Files:** internal/take/file.go, internal/take/file_test.go
**Read first:** internal/take/file.go — Read, readHeader, readLines, ErrFormat;
internal/take/file_test.go — TestLoad_RefusesWhatItCannotRead, gzipLines, encodeTake;
internal/tune/session.go — refreshTake
**Tests:** `TestReadMatchesSequentialDecoder` covers the small `largeTake`, the existing
fixtures, and an empty take. New cases in `TestLoad_RefusesWhatItCannotRead`: corrupt line N,
two-field line, unknown field, trailing bytes after a value — each also asserting its `line N`
text (and `neither one snapshot nor one event` for `{}` and the two-field line); truncation stays
green. A corrupt and a truncated stream leave `runtime.NumGoroutine` where it was. A new test
makes line 7 and line 3 both bad and checks the error names line 3.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/take`
- `GOMEMLIMIT=2GiB go test -race ./internal/take -run 'Read|Load'`
- `GOMEMLIMIT=2GiB go test ./internal/take -bench BenchmarkLoad -run '^$' -benchtime 3x`
**Closes:** tune-startup-time
Depends on item 1.
**Commit:** `perf(take): decode take lines in parallel`

## 3. Full-view draft frames on the server

**What:**
**Goal:** `GET /api/frame?view=full&draft=1&t=<ms>` returns 200 with a PNG at the full view's
pixel size. The PNG is drawn from the draft raster with the draft scaler. A pipeline without a
draft rasterizer falls back to the exact full frame.
**Approach (assumed at the header base):**
- `internal/compose/compose.go`: add `Compositor.ComposeFullDraft` (`fullCrop()` with
  `draw.ApproxBiLinear`), beside `ComposeFull`/`ComposeDraft`, through the shared `compose`.
- `internal/render/pipeline.go`: add `Pipeline.FullDraftAt`, mirroring `DraftAt` but cropped
  like `FullFrameAt`, using `draftRasterizer`/`draftCache`.
- `internal/tune/frame.go`: add `Session.FullDraftFrame`, encoded like `DraftFrame`.
- `internal/tune/server.go`: `handleFrame` routes `view=full&draft=1` to it and drops the
  "the full view has no draft frame" refusal.
**Regression guard.** Bounds alone cannot see the crop (`compose` always returns `c.out`): the
compose test, modelled on `TestComposeFullEqualsAFactorOneComposition`, checks content. The
"full view's draft" row leaves the 400-only `TestHandler_Frame_BadView_IsRefused` table. Run
compose and render as separate `go test` commands (one package each, per AGENTS.md).
**Files:** internal/compose/compose.go, internal/compose/compose_test.go,
internal/render/pipeline.go, internal/render/pipeline_draft_test.go, internal/tune/frame.go,
internal/tune/frame_test.go, internal/tune/server.go, internal/tune/server_test.go
**Read first:** internal/tune/server.go — handleFrame, frameDraftOf; internal/render/pipeline.go —
DraftAt, FullFrameAt; internal/compose/compose.go — ComposeDraft, ComposeFull;
internal/tune/server_test.go — TestHandler_Frame_BadView_IsRefused; internal/compose/compose_test.go — TestComposeFullEqualsAFactorOneComposition
**Tests:**
- Remove the "full view's draft" row from `TestHandler_Frame_BadView_IsRefused`; add
  `TestHandler_Frame_FullViewDraft` on `cameraStoryboard`: 200, PNG bounds equal the full
  frame's, bytes equal `session.FullDraftFrame(t)`.
- The pipeline test compares `FullDraftAt` and `FullFrameAt` bounds; with no DraftCache,
  `FullDraftAt` gives the same bytes as `FullFrameAt`.
- The compose test: a zoomed `ComposeFullDraft(3s, src)` equals an unzoomed `ComposeDraft`'s
  bytes and differs from the zoomed `ComposeDraft`'s.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose -run 'Full|Draft'`
- `GOMEMLIMIT=2GiB go test ./internal/render -run 'Full|Draft'`
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'Frame|Draft'`
**Commit:** `feat(tune): serve draft frames for the full view`

## 4. Full-view draft frames on the page

**What:** Recast at the regression check (2026-11-06).
**Goal:** While playing or scrubbing with the full view shown, the page requests
`view=full&draft=1` frames at the clip's pace. Once stopped it settles on the exact full
frame, as the camera view does.
**Approach (assumed at the header base):** In `internal/tune/page/frames.js`, the moving and
draft-first paths (`pumpMoving`, `prefetch`, the draft request in `pumpStill`) hard-code
`VIEW_CAMERA`. They use the still view (`stillView()`, from marker.js `wantsFullView`)
instead; `frameKey` already keys on view. In the README `**Tuning**` paragraph, say drafts
cover both views.
**Regression guard.** The full view stays shown while playing or scrubbing and is fed draft frames, but the camera overlay (crop and marker) is drawn only on exact frames: frames.js exports the shown frame's quality (e.g. frameQuality) and marker.js refresh/shownCamera require an exact frame, which also removes the per-draft GET /api/camera and the overlay blinking; add marker.js and cmd/photon/tune.go to Files. On the moving and draft-first paths every frameKey/cached/isAtHand call passes stillView() (find with grep -n 'frameKey(\|cached(\|isAtHand(' internal/tune/page/frames.js; isAtHand gains a view parameter). Tests build the expected path as draftFrame(ms)+viewFullQuery, the order fetchFrame sends; TestPageJS_SettleWithAPositionSelected_HidesTheOverlayOnTheDraftFirst keeps its no-overlay assertion and only its view assertion is rewritten. Prose rule: every sentence saying the full view shows only while paused or that drafts are always the camera's is updated (grep -n -i 'paused\|full view\|no draft' README.md cmd/photon/tune.go internal/tune/page/*.js); the overlay-only-while-stopped wording stays true. Supersedes CHANGELOG.md 0.2.0 line "Playing or scrubbing shows the normal preview frames without the overlay" in part: playing now shows full-view drafts, still without the overlay — the item's CHANGELOG sidecar entry says so.
Leaving the full view now drafts the camera's frame first (selectPosition's draft-first fetches the full view's draft, so the camera draft is no longer cached): TestPageJS_SelectAZoomAfterAPosition_ReturnsToTheCamerasFrame has its want rewritten to draftFrame(start), exactFrame(start), beside the SettleWithAPositionSelected rewrite.
**Files:** internal/tune/page/frames.js, internal/tune/page/marker.js, internal/tune/pagejs_test.go,
README.md, cmd/photon/tune.go
**Read first:** internal/tune/page/frames.js — pumpStill, pumpMoving, prefetch, isAtHand, stillView;
internal/tune/page/marker.js — refresh, shownCamera; internal/tune/pagejs_test.go —
TestPageJS_SelectAZoomAfterAPosition_ReturnsToTheCamerasFrame
**Tests:** A pagejs test plays with the full view on and asserts the requests are
`draftFrame(ms)+viewFullQuery`, with no overlay and no GET /api/camera per draft. After stop, it
asserts one exact `view=full` request without `draft`, with the overlay drawn.
`TestPageJS_SettleWithAPositionSelected_HidesTheOverlayOnTheDraftFirst` has only its view
assertion rewritten; beside it, `TestPageJS_SelectAZoomAfterAPosition_ReturnsToTheCamerasFrame`
has its want rewritten to `draftFrame(start), exactFrame(start)`. The other camera-view tests
stay green.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'PageJS|Page'`
**Closes:** tune-full-view-draft
Depends on item 3.
**Commit:** `feat(tune): draft the full view while playing or scrubbing`

## 5. Render progress hook

**What:**
**Goal:** `render.Options` has `Progress func(done, total int)`. When it is set, `render.Run`
calls it after each frame of both ffmpeg passes, with `total` = 2 × frame count and `done`
rising to `total`. When it is nil, behaviour and output stay unchanged.
**Approach (assumed at the header base):** Add the field to `Options` in
`internal/render/render.go`. Thread it into `plan.encode` → `writeFrames` (`encode.go`). The
pass index offsets `done`. `cmd/photon/render.go` leaves it nil.
**Regression guard.** `TestRunMatchesTheOnePassEncode` calls `reference.encode(ctx,
commandLine)` directly (render_test.go:422): carry the callback and the pass offset on `plan`
(set in `newPlan`/`Run`) so `encode`/`writeFrames` keep their signatures, or update that call
site in the same commit.
**Files:** internal/render/render.go, internal/render/encode.go, internal/render/render_test.go
**Read first:** internal/render/render.go — Options, Run, newPlan, plan; internal/render/encode.go —
plan.encode, plan.writeFrames; internal/render/render_test.go — TestRunMatchesTheOnePassEncode;
internal/schedule/schedule.go — Schedule.Frames
**Tests:** `TestRunReportsProgress` (ffmpeg-gated) checks `done` is monotonic, the last call
is `(total, total)`, and `total` = 2 × frames. An existing render test passes `Progress: nil`
unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render -run 'Run'`
**Commit:** `feat(render): report frame progress through Options.Progress`

## 6. Export job in the tune session

**What:**
**Goal:** `POST /api/export` starts one background render of the saved clip to its configured
output and returns 202. It returns 409 when the session is dirty, has problems, is stale, or
an export is already running. `/api/state` carries `export: {running, done, total, out,
error}`. A finished export reports `out`; a failed one reports `error`. Neither touches the
history or the unsaved bytes.
**Approach (assumed at the header base):**
- New `internal/tune/export.go` owns the job: one mutex-guarded record, sentinels
  `ErrUnsaved`/`ErrExporting` mapped in `statusOf`.
- The job runs `render.Run` on the session's last good clip (`s.good.loaded`) with
  `render.Options{Take: takePath, Progress: …}`. stdout/stderr go to buffers, and stderr becomes
  `error` on failure.
- It runs under `tune.Run`'s context, so stopping tune kills ffmpeg.
- Being async avoids `writeTimeout`.
- `State` (`state.go`) gains `Export`. Route it in `newHandler` behind `guard`.
- Add `export.go` to `doc.go`'s map.
**Regression guard.** Export progress must not bump State's version (frames.js syncVersion empties the frame cache on a version change); the 202 response body of POST /api/export is a State, as post() in app.js expects.
Supersedes in part State's doc in `state.go` ("Version rises on every change"): add the export
exception there. `error` is the `err.Error()` `render.Run` returns (stderr is warnings only).
Problems refuse with a sentinel `statusOf` maps to 409, not `ErrInvalid` (422); extend
`newHandler`'s status doc. Under `s.mu`, re-read `s.path` against `s.disk` before starting, as
`Save` does, refusing with `ErrStale`. The context lives on the Session, set in `Run` beside
`photonBuild`, defaulting to `context.Background()` in `NewSession` (no `newHandler` change). A
render seam on Session (like `buildPipeline`) lets tests hold `running` and wait for the job.
**Files:** internal/tune/export.go, internal/tune/export_test.go, internal/tune/session.go,
internal/tune/server.go, internal/tune/state.go, internal/tune/run.go, internal/tune/doc.go
**Read first:** internal/tune/session.go — NewSession, Save, buildPipeline; internal/tune/server.go —
newHandler, stateAfter, statusOf; internal/tune/run.go — Run; internal/tune/state.go — State
**Tests:**
- export_test (ffmpeg-gated): export, poll state until `running` is false, then compare the
  GIF bytes with `render.Run` on `clip.Load` of the same files to a second path.
- The 202 body decodes as a State; a progress tick leaves `version` unchanged.
- 409 for dirty; for an invalid board built clean (take made unreadable, then Refresh); for a
  storyboard rewritten on disk but not yet refreshed; and for a second POST while the render
  seam holds the job running (released and awaited before the test returns).
- A failing render (missing take) reports `error` = the returned error's text and leaves the
  session dirty-state unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'Export|State|Handler'`
Depends on item 5.
**Commit:** `feat(tune): render the GIF from the session on request`

## 7. Export button on the tune page

**What:**
**Goal:** The page has an Export GIF button beside Save. It is disabled while the session is
dirty, has errors, is stale, or an export runs. While running it shows `Exporting N%`. On
finish the status line reports the written path or the error, and edits stay as they were.
**Approach (assumed at the header base):**
- Copy the `#save` pattern: `index.html` button `#export`, `API.export` and `post(path, done)`
  in `app.js`, and `reportStatus` for the result.
- Progress comes from `state.export` on the existing one-second `poll`.
- Give `#export` `class="primary"`, as `#save` has (`button.primary`); no `style.css` change.
- Update the README `**Tuning**` paragraph and `cmd/photon/tune.go`'s Long text.
**Regression guard.** The page reads state.export on every poll outside poll's stateKey/version gate, so progress updates without invalidating the frame cache.
app.js is at 390 of `maxScriptLines` 400: put the export UI in a new split script (in
`splitScripts`, loaded after app.js in index.html, in doc.go's file map) or in transport.js.
Button tests run through item 6's render seam and an in-package export record, never a real render.
**Files:** internal/tune/page/index.html, internal/tune/page/app.js, a new split script under
internal/tune/page/ (or internal/tune/page/transport.js), internal/tune/doc.go,
internal/tune/pagejs_test.go, internal/tune/page_test.go, README.md, cmd/photon/tune.go
**Read first:** internal/tune/page/app.js — post, poll, stateKey, takeState; internal/tune/page_test.go —
splitScripts, TestPage_SplitScripts_StayBelowTheFileSizeGuide; internal/tune/page/transport.js —
reportStatus; internal/tune/page/frames.js — syncVersion
**Tests:** A pagejs test checks the button is disabled when dirty and posts `/api/export`
when clean. With `done=1,total=2` set on the export record (version unchanged) it shows
`Exporting 50%`, and it reports `out` on the running→false transition. page_test checks
`#export` exists and the split scripts stay below the size guide.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'Page'`
- `go build ./cmd/photon`
**Closes:** tune-gif-export
Depends on item 6.
**Commit:** `feat(tune): export the GIF from the page`

## 8. photon render --watch

**What:**
**Goal:** `photon render --watch <storyboard> [<take>]` renders once. After that it renders
again each time the storyboard, config, take or a caption font file changes. A failed load or
render prints `photon: <err>` to stderr and keeps watching. Ctrl+C or SIGTERM stops it with
exit 0.
**Approach (assumed at the header base):**
- New `internal/render/watch.go`:
  `Watch(ctx, storyboardPath string, load func() (*clip.Clip, error), options Options, stdout,
  stderr io.Writer) error`.
- It polls `{size, modTime}` stamps every `watchInterval` (500ms) and re-renders once the
  stamps are unchanged across two consecutive polls.
- Each round re-runs `load`, so a config edit can move the work dir and take. Re-derive the
  watched set from the loaded clip: `Storyboard.Path`, `Config.Path`, the take
  (`options.Take` or `loaded.Take()`), and the config's caption font files.
- After a failed load, keep the last set plus `storyboardPath`.
- Own stamp helper; tune's `fileStamp` stays (incidental similarity).
- `cmd/photon/render.go`: add a `--watch` flag. Copy `signal.NotifyContext` from
  `cmd/photon/tune.go`. Pass `clip.Load` as a closure.
- Add `watch.go` to render's `doc.go` map. Document the flag in README `## Commands`.
**Regression guard.** After a failed load with no earlier set, also watch the `--config` path,
else `config.Find`'s result from the storyboard's dir (or that dir's photon.yaml). Wrap the
context in `signal.NotifyContext` only on the `--watch` branch. Name the `--watch` exit rule in
render.go's Long "Exit status" text and README's exit-code rows.
**Files:** internal/render/watch.go, internal/render/watch_test.go, internal/render/doc.go,
cmd/photon/render.go, cmd/photon/render_test.go, README.md
**Read first:** cmd/photon/render.go — newRenderCommand; cmd/photon/tune.go — signal.NotifyContext;
internal/render/render.go — Run, Options; internal/clip/clip.go — Load; internal/config/config.go —
Config.Path, CaptionFont; cmd/photon/render_test.go — TestRender
**Tests:** watch_test with `DryRun: true`, so no ffmpeg:
- an initial render, then a table over storyboard, config and take rewrites (size-changing
  writes), each asserting a further pair of ffmpeg command lines on stdout;
- a config fixed after a first failed load renders again;
- a broken storyboard prints an error, and the next fix renders again;
- context cancel returns nil.
A cmd test checks the `--watch` flag is registered.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/render -run Watch`
- `GOMEMLIMIT=2GiB go test ./cmd/photon -run Render`
**Closes:** render-watch
**Commit:** `feat(render): re-render on change with --watch`

## 9. Caption width field

**What:**
**Goal:** A caption accepts an optional `width:` (int, design pixels, the box's outer width).
0 or absent means automatic. A negative value is refused on load with a located error. The
field round-trips through `storyboard.Apply` splices without changing other keys.
**Approach (assumed at the header base):** Add `Width int \`yaml:"width"\`` to
`storyboard.Caption` in `internal/storyboard/caption.go`. Check it in `Caption.validate` (≥ 0).
**Regression guard.** Place `Width` after `Fade` and append "width" after "fade" in
`TestKeyOrder_StructTags_GiveTheStoryboardKeyOrder`'s pinned Caption list, so `captionKeyOrder`
for existing inserts is unchanged. The refusal cases live in `TestLoadRejects_Captions`
(storyboard_test.go); there is no caption_test.go.
**Files:** internal/storyboard/caption.go, internal/storyboard/storyboard_test.go,
internal/storyboard/edit_test.go
**Read first:** internal/storyboard/caption.go — Caption, Caption.validate; internal/storyboard/storyboard_test.go —
TestLoadRejects_Captions, captionBase; internal/storyboard/edit_test.go — TestKeyOrder_StructTags_GiveTheStoryboardKeyOrder,
TestApply_CaptionEdit_ChangesOnlyTheEditedText; internal/storyboard/edit_caption.go — captionKeyOrder; internal/storyboard/edit.go — keyOrder
**Tests:** In `TestLoadRejects_Captions`, a -1 case wanting "captions[N]: width: want ≥0, got
-1"; accepted cases for absent, 0 and 300 beside it. The pinned key-order list gains "width".
An existing caption edit test still passes on a board with `width` set.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard`
**Commit:** `feat(storyboard): accept a caption width`

## 10. Wrap caption text

**What:**
**Goal:**
- Each `\n`-separated line of caption text wraps greedily at spaces. The text fits within
  `min(width or ∞, frame width − 2·edgeMargin) − 2·boxPaddingX` design pixels, scaled by unit.
- A word longer than that stays alone on its line, and the box-larger-than-frame error still
  applies.
- Placement and alignment apply to the wrapped block.
- A caption whose lines all fit renders pixel-identically to the base.
- A golden PNG covers a wrapped caption.
**Approach (assumed at the header base):**
- `internal/caption/layout.go` `newTextBlock` gains a max text width and splits each hard line
  into wrapped lines with `font.MeasureString`.
- `place` in `caption.go` computes the width from `out`, `unit` and `caption.Width`.
- Golden test: new `internal/caption/golden_test.go` copies the raster pattern (`-update`
  flag, `writeGolden`/`readGolden`/`assertMatchesGolden`, a tolerance). It uses the embedded
  faces and `internal/caption/testdata/wrapped.png`.
- README `### Captions`: drop "Lines are not wrapped"; document wrapping and `width`.
**Regression guard.** The item's CHANGELOG sidecar entry states that a caption whose text is wider than the frame minus the edge margins (but fit the frame) now wraps, changing its shipped GIF, and that multi-word captions refused as too wide may now pass lint.
A hard line that fits is kept byte-for-byte; only an overlong line is split, at single spaces.
`cmd/photon/testdata/invalid/caption-box.yaml`'s text becomes one unbreakable word still wider
than the frame (header comment fixed). Every comment or fixture calling a multi-word caption
"too wide"/"wider than the frame" is updated or made one word (`grep -rn "too wide\|wider than"
internal cmd --include=*_test.go --include=*.yaml`), e.g. `tooWideText` in tune's session_test.go.
**Files:** internal/caption/layout.go, internal/caption/caption.go,
internal/caption/layout_test.go, internal/caption/golden_test.go,
internal/caption/testdata/wrapped.png, README.md, cmd/photon/testdata/invalid/caption-box.yaml,
internal/tune/session_test.go
**Read first:** internal/caption/layout.go — newTextBlock, textBlock, boxRect; internal/caption/caption.go —
place; internal/caption/layout_test.go — TestNew_BoxLargerThanTheFrame_IsRefused;
internal/raster/raster_test.go — writeGolden, assertMatchesGolden; cmd/photon/lint_test.go — TestLint
**Tests:**
- Layout table: short text gives 1 line; long text gives N lines, each ≤ max; `width: 200`
  narrows; an overlong word stays alone; a hard `\n` is kept; a fitting double-spaced or
  indented line comes back unchanged.
- The golden test.
- The existing caption and `render` `TestCheckCaptions_*` tests stay green, and `TestLint`'s
  "a caption box larger than the frame" case stays green on the one-word fixture.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/caption`
- `GOMEMLIMIT=2GiB go test ./internal/render -run Caption`
- `GOMEMLIMIT=2GiB go test ./cmd/photon -run TestLint`
**Closes:** caption-text-wrapping
Depends on item 9.
**Commit:** `feat(caption): wrap long caption text at word boundaries`
