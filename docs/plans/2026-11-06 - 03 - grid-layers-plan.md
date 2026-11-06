# Grid layers plan (browser capture, part A)

**Goal:** Land the two-layer grid model a browser source will plug into, with no change to what a
terminal take records, checks or renders: the decision Note, a text layer and an image layer on
every snapshot, cell geometry in the take, compact pixel frames, the engine's `Source` seam, and
the browser launch, area and selector keys in config and storyboard.
**Date:** 2026-11-06
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** d1ba209

**Sources:**
- `.capcom/issues/browser-capture.md` and its children under `.capcom/issues/browser-capture/`
- Notes `extension-model`, `package-layout`, `glossary`, `coding-conventions`

**Ratified design calls (user, 2026-11-06):**
- **Packaging:** two plans; this one is A. Plan B (capture source, render/tune, film-tune-page) is written after A lands.
- **Selectors after recording:** the source records, with each snapshot, the boxes of every selector the clip names (storyboard targets + config areas); check, render and tune resolve selectors from those; tune proposes text targets plus selectors the clip already names.
- **Frame storage:** inside the take file; take format v2 when a take has an image layer; PNG key frames plus PNG patches of the changed rectangle; key frame every 2 s or when the patch covers more than half the frame.
- **Size budget:** a one-minute 1280×800, 24 fps browser take is ≤ 25 MB.
- **Viewport:** storyboard `frame.viewport: {width, height}` and `frame.device_scale`; config launch holds only `url` (+ optional `browser` path).
- **Grid unit:** one browser cell is one CSS pixel; the take records cell geometry W = H = device scale.
- **Engine seam:** `engine.Terminal` becomes `engine.Source` with `Type(text)`, `Key(name)`, `Pointer(event)`; the pty adapter encodes bytes and SGR; `Current`, `Match`, `Done`, `Event` stay.
- **Launch keys:** `launch.url` is the alternative to `launch.command` (exactly one set); `${VAR}` in url expands from the launch env; optional `launch.browser` overrides Chromium discovery; `strip` is refused with url.
- **Text areas:** top/bottom anchors read as y-bands over text runs; `offset` refused on a url-launch clip; `areas.<name>.selector` is an area from a recorded selector box.
- **Selector lint:** shape only (non-empty, balanced brackets/quotes/parens, url-launch clip only).
- **Engine item placement:** the `Source` seam is in this plan; `browser-capture-source` stays open.
- **Image code:** `take` stays free of image code (Note `package-layout`); a new `internal/pixels` owns encoding, decoding and seeking the image layer.
- **Caption size:** on a url-launch clip a caption's font.size is in CSS px (scaled by device_scale × scale), default 16, bounds [6, 72] (user, 2026-11-06).

**Standing requirements:**
- skills: coding-standards
- Pi 5 host: single packages with `GOMEMLIMIT=2GiB`; never `./...` with `-race`; one test command at a time.
- Every new non-test `.go` file is named in its package's `doc.go` (testkit `CheckDocMap`).
- An item that changes a package's imports or owns a new package updates that row of Note `package-layout`.
- No AI attribution; no forbidden names (AGENTS.local.md).

**Out of scope:** the Chromium source, URL expansion at record time, rendering or tuning a browser take, the example clip (plan B); any version bump.

**Regression check (2026-11-06, d1ba209):**
- 1: guard folded
- 2: guard folded
- 3: recast
- 4: guard folded
- 5: guard folded
- 6: guard folded
- 7: guard folded
- 8: guard folded
- 9: guard folded; yields to Note `package-layout` ("nothing imports upward")
- 10: recast
- 11: recast
- 12: guard folded
- 13: recast; supersedes the `CheckCaptions` doc comment (internal/render/check.go:12-17)
- 3 (re-check): guard folded
- 10 (re-check): guard folded
- 11 (re-check): decision applied (Files line corrected); guard folded (clipFields describes viewport and device_scale; Target.validate wording)
- 13 (re-check): recast (url-clip geometry in one internal/render helper)

## 1. Record the grid-surfaces decision — ✅ DONE (2026-11-06)

**What:**
**Goal:** a decision Note `grid-surfaces` states the two-layer grid model and every ratified call in this header; Notes `extension-model`, `package-layout` and `glossary` agree with it; `capcom check` is clean.
**Approach (assumed at the header base):** `capcom know add grid-surfaces` (labels `[decision, architecture]`), body: decision (2026-11-06), why, the model (snapshot = text layer + image layer on one grid; terminal derives both from cells; browser stores screenshot frames and layout text), capture sources (pty terminal; headless Chromium driven over the DevTools Protocol through chromedp), the grid unit and cell geometry, frame storage and the 25 MB budget, recorded selector boxes, edits a pixel take refuses (`font_size`, `line_height`, `cols`, `rows`, `padding`), the engine's `Source` seam, package ownership (layers and geometry in `take`, image codec in `pixels`, the browser source in a planned `internal/browser` importing `take`, `pixels`, `keys`). `capcom know update` on `extension-model`: a browser is a launch kind (`launch.url`), areas found by anchors or selector. `package-layout`: prose for the ownership above and the `Source` interface rule (`engine` never imports `terminal` or `browser`); rows change only in the items that change imports. `glossary`: Snapshot (two layers), Cell (CSS pixel on a browser take), Area (anchors or selector), new terms Text layer, Image layer, Text run, Capture source.
**Regression guard.** `capcom know add` of a top-level Note rewrites the managed CAPCOM block in AGENTS.md (lines 30-83, "Knowledge tree, top level"): commit the refreshed AGENTS.md with the Notes. The command form is `capcom know add --id grid-surfaces --title ... --label decision,architecture --body-file -`.
**Files:** .capcom/knowledge/grid-surfaces.md, .capcom/knowledge/extension-model.md, .capcom/knowledge/package-layout.md, .capcom/knowledge/glossary.md, AGENTS.md
**Read first:** .capcom/knowledge/package-layout.md — table rows `take`, `engine`, Rules paragraph; .capcom/knowledge/extension-model.md — "How a project plugs in" table (areas row);
.capcom/knowledge/glossary.md — Snapshot, Cell, Area; AGENTS.md — CAPCOM block
**Closes:** grid-surfaces-decision
**Tests:** none (docs).
**Acceptance:**
- `capcom know show grid-surfaces` prints the Note; `capcom check` reports no problems.
- `grep -c "grid-surfaces" .capcom/knowledge/extension-model.md .capcom/knowledge/package-layout.md` is ≥ 1 for each.
**Commit:** `docs(knowledge): record the grid-surfaces decision`

## 2. Give a snapshot a text layer — ✅ DONE (2026-11-06)

**What:**
**Goal:** `take.Snapshot.Text()` returns the snapshot's text layer as `[]take.TextRun` in reading order; a cell snapshot derives one run per row exactly as the target package's row reading does today; a snapshot without cells returns its stored runs; snapshots also carry recorded selector boxes.
**Approach (assumed at the header base):** in `internal/take` add `TextRun{Text string; Box Box; Edges []int}` where `Edges[i]` is the x of byte i and `len(Edges) == len(Text)+1`, with a method returning the Box of byte span [a, b). Move the derivation from `target.rowsOf`/`screenRow` (trailing blanks trimmed, wide-grapheme trailers skipped, empty grapheme as a space) into take; a terminal run's Box is `{0, y, cols, 1}` and its Edges are cell columns. Add `Snapshot.Runs []TextRun` (`json:"runs,omitempty"`) and `Snapshot.Selectors map[string][]Box` (`json:"sel,omitempty"`). `Text()` derives from Cells when Cells is non-empty, else returns Runs. Add `Snapshot.Bounds() Box` (grid size: from Cells, else the union of runs). No format version change: the new fields are absent from every terminal take. New file `internal/take/text.go`, named in `doc.go`.
**Regression guard.** `Snapshot.Bounds()` on a cell snapshot is {0, 0, longest row's length, len(Cells)} so ragged test fixtures stay safe; on a run-only snapshot it is the union of its runs and is used only for clamping, never for area containment. Copy, not move: take gains the derivation while `target.rowsOf`/`screenRow` stay unchanged until item 3 deletes them, so internal/target still builds at this commit.
**Files:** internal/take/text.go, internal/take/text_test.go, internal/take/take.go, internal/take/doc.go
**Read first:** internal/target/resolver.go — rowsOf, screenRow, blankGrapheme; internal/take/take.go — Snapshot, Cursor; internal/take/event.go — Box;
internal/take/file_test.go — sampleTake, TestFile_SaveLoadRoundTrip; internal/take/doc.go — file map
**Tests:** table tests: wide graphemes, combining marks, trailing blanks, blank rows; a hand-built run snapshot; JSON round trip of Runs and Selectors; a v1 take file still reads with neither field; `Bounds()` on ragged rows (longest row's length) and on a run-only snapshot (union of runs).
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/take/`
**Commit:** `feat(take): give a snapshot a text layer`

## 3. Resolve targets and areas on the text layer — ✅ DONE (2026-11-06)

NOTES (2026-11-06): anchor error messages keep the word "row" ("top row 7 is below bottom row 5", "row 0 offset by -1") so existing resolver test expectations stay unchanged; the number is a line index, which on a terminal is the row.
NOTES (2026-11-06): an anchor matches against a line's text (its runs joined by one space), while a target regex matches within one run at a time and never spans runs.

**What:** Recast at the regression check (2026-11-06).
**Goal:** `internal/target` reads only `Snapshot.Text()` (no `.Cells`); an area resolves to a `take.Box`; a target's box comes from its run's Edges; every existing resolver test passes unchanged in expectation; a target resolves on hand-built runs that sit off character cells.
**Approach (assumed at the header base):** `rowsOf`/`screenRow` give way to the take text layer. Group runs into lines (runs sharing a top edge, x order); anchors pick a line, `above`/`below`/`offset` count lines (terminal: one line per row, so unchanged). An area is the Box spanning the full grid width from its top line's top to its bottom line's bottom. `Resolver.Rows` is replaced by `Resolver.Area(screen, name) (take.Box, error)`. `Target` matches the regex within each run whose Box lies inside the area box, in reading order, box from `TextRun` span. `Text` joins a line's runs with one space and lines with newlines (terminal: identical to today). Keep `ErrNotOnScreen` wrapping as is.
**Regression guard.** An anchor area contains a match by y-band only (the match's box lies vertically within the area's top..bottom, any x); a selector area contains a match by full 2-D box containment; `Resolver.Rows` is removed outright and replaced by `Resolver.Area`, so item 4 makes no edit in internal/target. Because `Rows` goes here, its one caller `tune.proposer.areasHolding` (internal/tune/pick.go:235) switches to `Resolver.Area` in this commit, holding a pick by the y-band rule of item 4's guard, so internal/tune builds at every commit.
An empty run is a line: on a cell snapshot there is one line per row, blank rows included, so an `offset` across a blank row and `Text` over "a\n\nb" are unchanged (today's `rowsOf` keeps every row).
**Files:** internal/target/resolver.go, internal/target/anchor.go, internal/target/resolver_test.go, internal/target/doc.go, internal/tune/pick.go
**Read first:** internal/target/resolver.go — Resolver.Target, Resolver.Rows, Resolver.Text, rowsOf; internal/target/anchor.go — area.rows, compileArea;
internal/target/resolver_test.go — snapshotOf; internal/tune/pick.go — proposer.areasHolding
**Tests:** existing resolver tests unchanged except `Rows` → `Area`; new: target and area on runs at fractional-cell CSS-pixel positions, two runs on one line, `nth: last` across runs, a run on a ragged row longer than row 0 found in its anchor area, an anchor `offset` across a blank row and `Text` over an area holding one (a `snapshotOf` with a "" line); existing tune pick tests pass unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/target/`, then `GOMEMLIMIT=2GiB go test ./internal/tune/`, and `grep -n "\.Cells" internal/target/*.go | grep -v _test` prints nothing.
**Commit:** `refactor(target): resolve targets and areas on the text layer`

## 4. Move the resolver's readers onto the text layer — ✅ DONE (2026-11-06)

NOTES (2026-11-06): re-derived from "tune proposer.areasHolding switches to Resolver.Area" and "check/compose need edits": item 3 already moved areasHolding to Resolver.Area with the y-band rule and box.H-then-name order, and check.judgement.seen, engine.screenSatisfies and compose's camera already compile against the resolver's API, so internal/check/check.go and internal/compose/camera.go are unchanged and absent from FILES.
NOTES (2026-11-06): pickedCells applies the guard's "a byte is picked when its span [Edges[i], Edges[i+1]) intersects [first, last]" per grapheme, not per byte: the bytes of a multi-byte grapheme share one edge, so a byte's own span is empty; a grapheme's span runs from its edge to the next byte's edge on another column (pickedSpan/graphemeEnd), and the whole grapheme is picked or not.
NOTES (2026-11-06): pickedCells reads the first run, in reading order, whose box holds the picked row and shows a non-blank grapheme within the pick; a pick across two runs of one browser line yields only the first run's text, since a target never spans runs (terminal: one run per row, unchanged).
NOTES (2026-11-06): clampToScreen clamps into Snapshot.Bounds() (longest row by row count, offset by the bounds' origin), where it used the first row's width; terminal grids are rectangular, so behaviour is unchanged.
NOTES (2026-11-06): the new wide-grapheme test drives pickedCells then proposer.propose on a hand-built cell snapshot (TestPickedCells_WideGrapheme_IsProposedWhole) rather than Session.Pick, because the pick session fixture's take holds only width-1 cells.

**What:**
**Goal:** outside `take`, `raster` and `terminal`, no non-test code reads `Snapshot.Cells`; check, the engine, tune's picker and compose's camera work through the resolver's new API and the text layer; terminal behaviour is unchanged.
**Approach (assumed at the header base):** `engine.clampToScreen` clamps to `Snapshot.Bounds()`; `tune.pickedCells`/`isBlank` read the text runs intersecting the picked box; tune `proposer.areasHolding` uses `Resolver.Area` (an area holds a pick when its box contains it); `check.judgement.seen` and `engine.screenSatisfies` keep `Resolver.Text`; `compose` camera's `Resolver.Target` call needs no change beyond compiling. Depends on item 3.
**Regression guard.** An area holds a pick by y-band, `area.Y <= pick.Y && pick.Y+pick.H <= area.Y+area.H` (today's `top <= picked.Y < end`), sorted by `box.H` then name; item 3 already switches `areasHolding` to this rule, so it is kept here. In `pickedCells`, a byte is picked when its span [Edges[i], Edges[i+1]) intersects [first, last], so a wide grapheme picked on either of its cells is proposed whole.
**Files:** internal/engine/drag.go, internal/tune/pick.go, internal/tune/pick_test.go, internal/check/check.go, internal/compose/camera.go
**Read first:** internal/tune/pick.go — pickedCells, isBlank, proposer.areasHolding; internal/tune/session_test.go — lineSnapshot;
internal/tune/pick_test.go — TestSession_Pick_ProposesAResolverCheckedTarget; internal/engine/drag.go — clampToScreen; internal/check/check.go — judgement.seen;
internal/compose/camera.go — target call via p.resolver.Target
**Closes:** text-layer-resolver
**Tests:** existing engine, tune pick, check, compose, render, rig and cmd/photon tests pass unchanged; new tune pick test on a wide grapheme (a pick starting on its trailing cell, and one ending on its first cell) proposes the whole grapheme.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/engine/`, then `./internal/tune/`, then `./internal/check/`, then `./internal/compose/`, then `./internal/render/`, then `./internal/rig/`, then `./cmd/photon/` (one at a time).
- `grep -rn "\.Cells" --include=*.go internal | grep -v _test | grep -v "internal/\(take\|raster\|terminal\)/"` prints nothing.
**Commit:** `refactor: read the text layer instead of cells outside take, raster and terminal`

## 5. Put cell geometry in the take — ✅ DONE (2026-11-06)

NOTES (2026-11-06): internal/compose/camera_test.go needed no edit — `cameraLayout` is `Layout{Size: testLayout.Size}` and sets no cell size, so it compiles and passes unchanged.
NOTES (2026-11-06): `render.cellLayout` takes `recorded *take.Take` with nil read as a take holding no cell size; `CheckCaptions` (no take) passes nil and so keeps the rasterizer's metrics.
NOTES (2026-11-06): the new test is `TestCompositorRect_OnePixelCells_IsTheBoxsPixelRectangle` in compose_test.go (1×1 cells, no padding, shipped at the source width: box {120,40,300,80} maps to Rect(120,40,420,120)).

**What:**
**Goal:** compose places boxes from a `take.CellSize{W, H float64}` in its Layout; render fills it from the take's recorded geometry when set, else from the rasterizer's metrics; terminal renders are byte-identical; a camera box on a 1×1 grid maps to the matching pixel rectangle.
**Approach (assumed at the header base):** add `take.CellSize` and `Take.Cell CellSize` (zero for a terminal take). `compose.Layout`'s `CellWidth`/`LineHeight` ints become `Cell take.CellSize`; `boxRect`, the cell centre, `cellAt`, the grid size and the caption baseline use it (integer-valued floats keep terminal results exact). `render.cellLayout` takes the take: `recorded.Cell` when non-zero, else `rasterizer.CellWidth()`/`LineHeight()`. Update `render.CheckCaptions`, `newCompositor`, and the test layouts `testLayout`, `cameraLayout` and the literal in compose_test.go. The header field is persisted by item 6.
**Regression guard.** compose and render have no golden or hash test (no testdata dir; pipeline_test.go compares against an in-tree oracle): the Goal's "terminal renders are byte-identical" means every existing compose, render and tune test passes unchanged. No caption code reads `Layout`: the Approach's "caption baseline" is `Layout.home` and `Compositor.ViewAt`'s cell end. `Output` keeps its guard as `Cell.W <= 0 || Cell.H <= 0`, so compose_test.go "no cell size" still reads "invalid cell layout".
**Files:** internal/take/take.go, internal/compose/compose.go, internal/compose/camera.go, internal/compose/compose_test.go, internal/compose/camera_test.go, internal/render/render.go, internal/render/check.go
**Read first:** internal/compose/compose.go — Layout, boxRect, home, Output; internal/compose/camera.go — Compositor.ViewAt;
internal/render/render.go — cellLayout, newCompositor; internal/render/check.go — CheckCaptions
**Closes:** take-cell-geometry
**Tests:** new compose test: Layout with `Cell{1,1}` places a camera box at the exact pixel rectangle; every existing compose, render and tune test passes unchanged (compose_test.go "no cell size" still reads "invalid cell layout").
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/compose/`, then `GOMEMLIMIT=2GiB go test ./internal/render/`, then `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Commit:** `refactor(compose): read cell geometry from the take's grid`

## 6. Take format v2 for an image layer — ✅ DONE (2026-11-06)

NOTES (2026-11-06): retry kept the previous attempt's internal/take/take.go edit (ImageEntry, Snapshot.Image `img`, json tags w/h on CellSize), which serves this item.
NOTES (2026-11-06): the v1 refusal lives in a new checkLineVersion helper that decodeLine and the test-only reference decoder (readSequentially) both call; readHeader now also returns the file's version. A v1 line refuses a non-null `img` or `runs` (a JSON null reads as absent); Write triggers v2 on a non-empty Runs, matching its omitempty.

**What:**
**Goal:** a take with an image layer or cell geometry writes format version 2 and reads back field for field; a take without them still writes version 1 byte-identically; every version-1 file reads; a v1 file carrying a v2-only field is refused.
**Approach (assumed at the header base):** in `internal/take` add `ImageEntry{Key bool; Rect Box; PNG []byte}` (encoded bytes only; take imports no image package) and `Snapshot.Image *ImageEntry` (`json:"img,omitempty"`); header gains `cell` (`Take.Cell`, omitted when zero). `FormatVersion` becomes 2; `Write` picks 1 when no snapshot has Image or Runs and Cell is zero, else 2; `readHeader` accepts 1..2 and the line decoder refuses `img`/`runs` on a v1 take with `ErrFormat`. Keep `decodeStrict` and the parallel line decode.
**Regression guard.** Depends on items 2 and 5 (it persists `Snapshot.Runs`/`Selectors` and `Take.Cell`). The v2-only fields are exactly `Write`'s v2 triggers — snapshot `img`, snapshot `runs`, header `cell` — each refused on a v1 file with `ErrFormat`; `sel` is v1-legal. The v1 write check compares decompressed lines (`takeLines`) to a pre-change fixture, never gzip bytes (compress/flate output is not stable across toolchains). The `ErrNewerVersion` cases "newer version" and "newer version with a new field" in `TestLoad_RefusesWhatItCannotRead` move to version 3.
**Files:** internal/take/file.go, internal/take/take.go, internal/take/file_test.go
**Read first:** internal/take/file.go — FormatVersion, header, Write, readHeader, decodeStrict; internal/take/file_test.go — TestLoad_RefusesWhatItCannotRead, takeLines;
internal/take/file_bench_test.go — BenchmarkLoad
**Depends on:** items 2 and 5
**Tests:** v1 terminal take decompressed lines unchanged against a pre-change fixture; v2 round trip with Runs, Selectors, Image, Cell; `img`, `runs` and header `cell` each refused on v1, `sel` read on v1; the two newer-version cases moved to version 3; `BenchmarkLoad` still builds.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/take/` and `GOMEMLIMIT=2GiB go test -run XXX -bench BenchmarkLoad -benchtime 1x ./internal/take/`
**Commit:** `feat(take): format v2 carries an image layer and cell geometry`

## 7. Encode, decode and seek the image layer — ✅ DONE (2026-11-06)

NOTES (2026-11-06): beyond the plan's three key-frame triggers, a change of frame size also starts a key frame; a snapshot with no entry decodes as the frame before it, so a caller that keeps such a snapshot still reads correctly.
NOTES (2026-11-06): each PNG holds the frame's RGBA bytes verbatim (as NRGBA), so even translucent frames round-trip bit for bit; an opaque frame, as every capture is, is written as plain RGB. Decoder.Frame returns its own buffer, overwritten by the next call.
NOTES (2026-11-06): the plan's synthetic page measures 4.8 MB/min (288 entries, 36 key frames); adding a spinner that turns every frame far from the typing box measured 30.3 MB/min, because one bounding rectangle over two distant changes exceeds half the frame and every such frame becomes a key frame.

**What:**
**Goal:** a new package `internal/pixels` turns a sequence of RGBA frames into take `ImageEntry` values and back losslessly at any snapshot index; a one-minute 1280×800, 24 fps synthetic take written with `take.Save` is ≤ 25 MB.
**Approach (assumed at the header base):** `pixels.Encoder` (holds the previous frame): a key frame at the first frame, every 2 s on the take clock, or when the changed bounding rectangle exceeds half the frame; otherwise a PNG patch of the changed rectangle; an unchanged frame yields no entry (the caller drops the snapshot). `pixels.Decoder` over a `*take.Take`: `Frame(index) (*image.RGBA, error)` seeks back to the last key frame and applies patches, caching the last decoded index so sequential reads cost one patch. Imports `take` and the standard image packages only. New package with `doc.go` and `docmap_test.go`; add its row to Note `package-layout`.
**Regression guard.** the size test generates each frame on the fly and keeps only the encoder's previous frame and the written entries, never a slice of all frames, because it runs in the normal CI suite (`go test ./...` without -short) on three OSes.
**Files:** internal/pixels/doc.go, internal/pixels/docmap_test.go, internal/pixels/pixels.go, internal/pixels/pixels_test.go, .capcom/knowledge/package-layout.md
**Read first:** internal/take/file.go — Save, Write; internal/take/take.go — Snapshot, Take.Snapshots; internal/take/docmap_test.go — TestDocMap (testkit.CheckDocMap);
internal/take/file_bench_test.go — largeTake (synthetic-take pattern); .capcom/knowledge/package-layout.md — table (pixels row below take)
**Closes:** pixel-take-storage
**Depends on:** item 6
**Tests:** round trip equality at every index, random-access order; key frame cadence; the size test draws a synthetic page (typed text growing, a scroll every 5 s, a panel swap) for 1440 frames, saves to `t.TempDir()`, asserts ≤ 25 MB; skip it under `testing.Short()`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/pixels/`
**Commit:** `feat(pixels): store a browser take's frames as key frames and patches`

## 8. Draw from the image layer — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the image branch is chosen per take (any snapshot carries an image entry), not per `Snapshots[index].Image`: the decoder shows the previous frame for a snapshot without an entry, so a per-snapshot check would rasterize such a snapshot's absent cells instead.
NOTES (2026-11-06): `rasterAt` became a `Pipeline` method returning an error, so a malformed image layer fails `FrameAt`, `FullFrameAt`, `DraftAt` and `FullDraftAt` (wrapping `pixels.ErrMalformed`); the decoder (behind a mutex, each call handed a fresh copy) carries over on `BuildOptions.Reuse` while the take matches.
NOTES (2026-11-06): `oracleFrames` now delegates to a new `oracleFramesOf` taking the snapshot drawer, so the image oracle shares its schedule and compositor setup; its behaviour is unchanged.
NOTES (2026-11-06): consequential edit — internal/render/doc.go: made necessary by render now importing pixels (the package comment lists its imports).

**What:**
**Goal:** the render pipeline draws a snapshot from its image layer when it has one and from its cells through raster otherwise; tune's frames follow through the pipeline; terminal renders and tune frames are byte-identical.
**Approach (assumed at the header base):** in `internal/render/pipeline.go` `rasterAt` checks `recorded.Snapshots[index].Image`: when set, the frame comes from a `pixels.Decoder` built once per pipeline (kept where the raster cache lives) instead of `rasterizer.Frame`; otherwise unchanged. Draft frames scale whichever image results. Add `pixels` to render's row in Note `package-layout`. Depends on item 7.
**Regression guard.** tune draws frames outside the session lock on concurrent goroutines and `Pipeline` is safe for concurrent use (pipeline.go:55): guard the pipeline's `pixels.Decoder` with a mutex and hand each call a fresh `*image.RGBA` copy, never the decoder's working buffer, so the raster cache keeps no image a later patch mutates. `FrameAt` scales and overdraws, so test `rasterAt` against the encoder's input frames and `FrameAt` against an `oracleFrames`-style oracle composing the decoded image; "byte-identical" means existing render and tune tests pass unchanged.
**Files:** internal/render/pipeline.go, internal/render/pipeline_test.go, .capcom/knowledge/package-layout.md
**Read first:** internal/render/pipeline.go — rasterAt, Pipeline, FrameAt, canReuseRasters; internal/tune/frame.go — Session.Frame; internal/tune/cache.go — rasterCache;
internal/render/pipeline_test.go — oracleFrames, TestPipeline_FrameAt_EveryScheduledFrame_EqualsThePipelineFreeOracle
**Closes:** snapshot-two-layers
**Tests:** over a hand-built v2 take with two image snapshots, `rasterAt` equals the encoder's input frame at each and `FrameAt` equals an oracle composing the decoded image; a concurrent `FrameAt` test on an image take; existing render and tune tests pass unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render/`, then `GOMEMLIMIT=2GiB go test ./internal/tune/`, then `GOMEMLIMIT=2GiB go test -race -run <the concurrent FrameAt test> ./internal/render/` (render alone, never `./...`)
**Commit:** `feat(render): draw a snapshot from its image layer when it has one`

## 9. Generalize the engine's Terminal into a Source — ✅ DONE (2026-11-06)

NOTES (2026-11-06): keys gains `PointerKind` (PointerPress, PointerMove, PointerRelease), `Pointer{Kind, X, Y, Mouse}` and `Pointer.Report()`, the one switch from a pointer event to its SGR report (MousePress/MouseMotion/MouseRelease), shared by the pty terminal and the engine's test fake; keys_test.go gains two tests for it.
NOTES (2026-11-06): the engine's field `term` is renamed `source` and `New`'s parameter with it, to follow the interface's rename; `fakeTerminal` keeps its name and now implements Type/Key/Pointer, encoding as the pty does, so every byte expectation is unchanged.
NOTES (2026-11-06): internal/rig/record.go needs no edit: `*terminal.Terminal` satisfies `engine.Source` as passed in `performBeats`; `Terminal.Send` stays exported for the end keys (`Quit`), the reply pump and terminal's own tests.
NOTES (2026-11-06): package-layout's closing "Grid surfaces" paragraph now says the interface became `Source` (past tense), alongside the keys, terminal and engine row updates and the interface rule.

**What:**
**Goal:** the engine drives an `engine.Source` with `Type(text string) error`, `Key(name string) error`, `Pointer(engine.PointerEvent) error`, `Current`, `Match`, `Done`, `Event`; the engine builds no input bytes; the pty terminal implements it; every recorded byte and take event is unchanged.
**Approach (assumed at the header base):** `PointerEvent{Kind (press, move, release); X, Y int; Button keys.Button; Mods keys.Mods}` in grid cells. `Engine.typeText` keeps its humanize pacing and calls `Type` per chunk; `pressKey` calls `Key`; `sendClick`/`sendDrag` emit pointer events. `*terminal.Terminal` gains the three methods, encoding with `keys.Lookup`, `keys.MousePress`, `keys.MouseMotion`, `keys.MouseRelease`; `Send` stays for rig's end keys if used there. Update `fakeTerminal` in engine_test.go (record events, encode as the pty does where tests assert bytes), the Windows stubs in terminal_windows.go, and `rig.recorder.performBeats`. Update rows `engine` and `terminal` in Note `package-layout` (and the interface rule's name).
**Regression guard.** Define the event in `keys` (`keys.Pointer{Kind, X, Y, Mouse}`) with `type PointerEvent = keys.Pointer` in engine; `terminal` imports `keys`, never `engine` (engine's fixture_test.go imports terminal, so that would be a test import cycle); the Note's terminal row gains `keys` — the item yields to Note `package-layout` ("nothing imports upward"). The engine keeps the `keys.Lookup` known-name check before calling `Key`, so `unknown key name "hyper"` with zero sends holds for every Source. Update every comment naming `Terminal`, `Send` or SGR as the engine's write path (`grep -n "Terminal\|Send\|SGR" internal/engine/*.go internal/terminal/doc.go internal/terminal/terminal.go`).
**Files:** internal/keys/keys.go, internal/engine/doc.go, internal/engine/engine.go, internal/engine/action.go, internal/engine/drag.go, internal/engine/engine_test.go, internal/terminal/doc.go, internal/terminal/terminal.go, internal/terminal/terminal_windows.go, internal/rig/record.go, .capcom/knowledge/package-layout.md
**Read first:** internal/engine/engine.go — Terminal; internal/engine/action.go — pressKey, sendClick; internal/engine/engine_test.go — fakeTerminal, TestEngine_Run_Failures;
internal/engine/fixture_test.go — TestEngine_Run_FixtureTUI; internal/keys/keys.go — Lookup, MousePress
**Tests:** existing engine (incl. fixture_test.go), drag, typing, terminal and rig tests pass with unchanged byte expectations; `TestEngine_Run_Failures` keeps `unknown key name "hyper"` with zero sends.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/keys/`, then `./internal/engine/`, then `./internal/terminal/`, then `./internal/rig/`; `GOOS=windows go vet ./internal/terminal/ ./internal/engine/`
**Commit:** `refactor(engine): drive a Source with typed input instead of bytes`

## 10. Config: browser launch and selector areas

**What:** Recast at the regression check (2026-11-06).
**Goal:** `photon.yaml` accepts `launch.url` and `launch.browser`, and `areas.<name>.selector`; strict decode and validation report `launch.url`, `launch.browser`, `areas.<name>.selector` by key; exactly one of `launch.command`/`launch.url`; `strip` with `url` refused; a selector area has no anchors and vice versa; `offset` in any anchor is refused when `launch.url` is set.
**Approach (assumed at the header base):** extend `config.Launch` (`URL string yaml:"url"`, `Browser string yaml:"browser"`) and `Launch.validate`; url must parse as http(s) after replacing `${NAME}` references with a placeholder (`NAME` an env-name shape); `Config.IsBrowser()` reports a url launch. Extend `config.Area` with `Selector string` and `Area.validate`/`validateAreas` (selector shape: non-empty, balanced `[]`, `()`, quotes); `Anchor.validate` gets the browser flag through the reporter path. Keep the `prefixed` key naming.
**Regression guard.** write the selector shape check in `internal/storyboard` from the start (an exported func such as `storyboard.CheckSelector`) and call it from config — add `.capcom/knowledge/package-layout.md` to Files and record `storyboard` in config's imports row; item 10 documents its own keys (`launch.url`, `launch.browser`, `areas.<name>.selector`) under README `## photon.yaml reference` so `TestREADMEDocumentsEveryField` stays green — add README.md to Files and `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run TestREADMEDocumentsEveryField` to Acceptance; until plan B, rig refuses a url-launch config before launching (record and setup) with an error naming `launch.url` and saying browser capture is not available yet — add internal/rig/record.go and a rig test to Files/Tests.
With neither key set the problem stays `launch.command: missing` (pinned in config_test.go and cmd/photon/lint_test.go); "want exactly one of command or url" only when both are set. `areas.<name>.selector` is refused when `launch.url` is unset. A selector problem reads `areas.<name>: selector: …`.
The setup refusal lands in `rig.Setup` (internal/rig/setup.go, its doc comment too), tested beside `TestSetup_*` in record_test.go. The `${NAME}` placeholder is a digit string (e.g. "1"), so a port-position url such as `http://127.0.0.1:${PHOTON_PORT_WEB}/` parses. The `storyboard.CheckSelector` tests sit in internal/storyboard/storyboard_test.go (validate.go has no test file of its own).
**Files:** internal/config/config.go, internal/config/area.go, internal/config/config_test.go, internal/config/area_test.go, internal/storyboard/validate.go, internal/storyboard/storyboard_test.go, internal/rig/record.go, internal/rig/setup.go, internal/rig/record_test.go, README.md, .capcom/knowledge/package-layout.md
**Read first:** internal/config/config.go — Launch, Launch.validate; internal/config/area.go — Area, validateAreas; internal/config/config_test.go — TestLoadRejects;
internal/rig/record.go — Record; internal/rig/setup.go — Setup; cmd/photon/readme_test.go — TestREADMEDocumentsEveryField
**Tests:** table tests for each accepted and refused shape, with the exact key path in the message: `launch.command: missing` with neither key, the exactly-one message with both, `areas.<name>: selector: …` for a bad or command-launch selector; a port-position `${PHOTON_PORT_<NAME>}` url accepted; `storyboard.CheckSelector` shapes; rig refuses a url-launch config before launching, for record and for setup, naming `launch.url`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/config/`, then `./internal/storyboard/`, then `./internal/rig/`, then `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run Lint`, then `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run TestREADMEDocumentsEveryField`
**Commit:** `feat(config): a browser launch by url and areas by selector`

## 11. Storyboard: viewport, device scale and selector targets

**What:** Recast at the regression check (2026-11-06).
**Goal:** a storyboard accepts `frame.viewport: {width, height}`, `frame.device_scale` and a target with `selector` instead of `text` (exactly one); `clip.Load` cross-checks a url-launch clip: viewport required; `frame.cols`, `rows`, `font_size`, `line_height`, `padding` refused; and refuses viewport, device_scale and selector targets on a command-launch clip, each error naming its key.
**Approach (assumed at the header base):** `storyboard.Frame` gains `Viewport` (`Width`, `Height`) and `DeviceScale float64` (default 1, range 1..4); `storyboard.Target` gains `Selector string` (yaml and json tags); `Storyboard.validateHeader` and `Target.validate` check shapes (selector shape check shared with item 10's rule: put it in `storyboard`, called from `config` — storyboard sits below config). `clip.crossCheck` adds the launch-kind checks beside `namedAreas`. Depends on item 10.
**Regression guard.** apply all five of the reviewer's G guards for item 11 (DeviceScale/Viewport zero means unset, default 1 read through an accessor, crossCheck tests `!= 0`; with frame.viewport set validateHeader skips the grid minimums and refuses cols/rows/font_size/line_height/padding, keeping today's messages when viewport is absent; camera cells bounded by the viewport; selector counted as the target form in DragEnd and Point exactly-one checks keeping today's wording when selector is empty; `json:"selector,omitempty"` and both key-order pins in edit_test.go updated; Files gain camera.go, caption.go, edit_test.go and Acceptance gains the cmd/photon Lint run); clip.crossCheck: url launch ⇔ frame.viewport set. Caption size (Ratified design call — add to the header list as `**Caption size:** on a url-launch clip a caption's font.size is in CSS px (scaled by device_scale × scale), default 16, bounds [6, 72] (user, 2026-11-06).`): when frame.viewport is set a caption's default size is 16 instead of frame.font_size. Item 11 documents its own keys (`frame.viewport`, `frame.device_scale`, a target's `selector`) under README `## Storyboard reference` and adds README.md to Files and `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run TestREADMEDocumentsEveryField` to Acceptance.
`frame.viewport` and `frame.device_scale` are described in clipFields (internal/storyboard/fields.go, which tune reads) as the existing frame keys are, editable and listed in `editFields`, so fields_test and tune's TestSession_State_NoReadOnlyClipFields stay green.
`Target.validate` keeps today's "text: missing" wording when neither text nor selector is set, and reports a target setting both as one problem naming `selector`.
**Files:** internal/storyboard/storyboard.go, internal/storyboard/beat.go, internal/storyboard/validate.go, internal/storyboard/storyboard_test.go, internal/storyboard/camera.go, internal/storyboard/caption.go, internal/storyboard/fields.go, internal/storyboard/fields_test.go, internal/storyboard/edit.go, internal/storyboard/edit_test.go, internal/clip/clip.go, internal/clip/clip_test.go, README.md
**Read first:** internal/storyboard/storyboard.go — Frame, applyDefaults; internal/storyboard/validate.go — validateHeader, Target.validate, DragEnd.validate; internal/storyboard/camera.go — Point.validate;
internal/storyboard/fields.go — clipFields; internal/clip/clip.go — crossCheck
**Tests:** storyboard decode/validate tables; clip cross-check tables for both launch kinds; every existing command-launch storyboard loads with today's Frame (`TestLoadEveryForm`) and today's messages; a url storyboard without grid keys loads; a selector `to:` and a selector camera position are accepted; a caption under `frame.viewport` defaults to size 16; the Target JSON and key-order pins in edit_test.go updated; `frame.viewport` and `frame.device_scale` in TestFields_DescribeEachOwnersFields as editable clip fields; `click.target.text: missing` and `drag.from.text: missing` unchanged with neither key, and one problem naming `selector` with both.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/`, then `./internal/clip/`, then `./internal/config/`, then `GOMEMLIMIT=2GiB go test ./internal/tune/ -run TestSession_State_NoReadOnlyClipFields`, then `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run Lint`, then `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run TestREADMEDocumentsEveryField`
**Commit:** `feat(storyboard): a browser viewport and selector targets`

## 12. Resolve selector targets and areas from recorded boxes

**What:**
**Goal:** the resolver resolves a selector target from `Snapshot.Selectors[selector]` (reading order, `nth`, inside its area) and a selector area to that selector's first box; a selector absent from the snapshot is `ErrNotOnScreen`; `clip.Clip.Selectors()` lists every selector the clip names (targets in beats, camera positions, expects, and config areas), sorted and unique, for the source to record.
**Approach (assumed at the header base):** `target.New` compiles selector areas beside anchor areas; `Resolver.Target` branches on `target.Selector`; `Resolver.Text` on a selector area joins the runs inside its box. Depends on items 3 and 11.
**Regression guard.** `storyboard.Expect` ({Seen, Area, Run}) carries no target, so "expects" is dropped from the Goal: `Selectors()` lists the real target sites — `click.target`, `drag.from`, `drag.via[i]`, `drag.to` (`ClickAction`, `DragAction`, `DragEnd`) and camera positions (`Point`) — plus config `areas.<name>.selector`; an expect reaches a selector only through a selector area.
**Files:** internal/target/resolver.go, internal/target/anchor.go, internal/target/resolver_test.go, internal/clip/clip.go, internal/clip/clip_test.go
**Read first:** internal/target/resolver.go — New, Resolver.Target, Resolver.Text; internal/target/anchor.go — compileArea; internal/clip/clip.go — namedAreas, crossCheck;
internal/storyboard/beat.go — DragEnd, Expect
**Tests:** selector target with `nth`, inside a selector area and an anchor area; missing selector; `Selectors()` over a storyboard naming selectors at each target site (click target, drag from, via and to, camera position) and in a config area.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/target/`, then `GOMEMLIMIT=2GiB go test ./internal/clip/`
**Commit:** `feat(target): resolve selectors from the boxes a take recorded`

## 13. Lint a browser clip and document the keys

**What:** Recast at the regression check (2026-11-06).
**Goal:** `photon lint` passes a valid url-launch clip with captions without starting a browser and reports each bad browser key; `render.CheckCaptions` on a url clip sizes captions per the header's caption-size call (CSS px scaled by device_scale × scale, default 16, bounds [6, 72]); the README's `## Targets and areas` describes selector targets, selector areas, anchor bands on a browser text layer and captions in CSS px, and the README describes lint's behaviour on a browser clip. The keys items 10 and 11 own are documented there, not here.
**Approach (assumed at the header base):** `render.CheckCaptions` builds its layout for a url clip from `viewport × device_scale × scale` with `Cell{device_scale, device_scale}` instead of the rasterizer's metrics. README section `## Targets and areas`: add the prose, note that recording a browser clip arrives with the capture source. Depends on items 5, 11.
**Regression guard.** item 13 no longer documents the keys items 10 and 11 own; it writes the `## Targets and areas` prose (selector targets, selector areas, anchor bands on a browser text layer, captions in CSS px) and the lint behaviour; `render.CheckCaptions` on a url clip sizes captions per the caption-size call — restate its Goal accordingly.
The url branch of `CheckCaptions` builds no rasterizer and loads no terminal faces (a test asserts `terminalLoads` unchanged); a url test clip loads through a `loadRenderClipOf` variant taking a config. This supersedes the rasterizer-only description in `CheckCaptions`' doc comment (internal/render/check.go:12-17): update it with the url branch.
`compose.Layout.Scale` stays the int output multiplier (`frame.scale`); device scale lives only in `Layout.Cell` (`take.CellSize{device_scale, device_scale}`); a url clip's frame size is `round(viewport.width × device_scale) × scale` by `round(viewport.height × device_scale) × scale` and caption sizes scale by `device_scale × scale`; item 13 owns this geometry in one helper in internal/render that plan B's render reuses, never restated per caller.
**Files:** internal/render/check.go, internal/render/check_test.go, internal/render/render_test.go, cmd/photon/lint_test.go, cmd/photon/testdata/browser/ (a url-launch photon.yaml and storyboard), README.md
**Read first:** internal/render/check.go — CheckCaptions; internal/render/render.go — newRasterizer, cellLayout, terminalLoads; internal/compose/compose.go — Layout, Output;
internal/render/render_test.go — loadRenderClipOf; internal/render/pipeline_draft_test.go — TestBuild_BothRasterizers_LoadTheTerminalFontsOnce
**Closes:** browser-launch-config
**Tests:** lint on the testdata url-launch clip passes; a clip with `frame.font_size` under `launch.url` fails naming `frame.font_size`; `CheckCaptions` on a url clip with a caption without `font.size` lays it out at 16 CSS px and leaves `terminalLoads` unchanged; the url-geometry helper's frame size and caption scale at device_scale 1, 2 and 1.5 (rounded size, `Layout.Scale` = `frame.scale`, `Cell` = device_scale).
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render/ -run CheckCaptions`, then `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run 'Lint|README'`
**Commit:** `feat(lint): check a browser clip and document its keys`
