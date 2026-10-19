# Draft frame speed — implementation plan

**Goal:** Cut a cold `GET /api/frame?draft=1` on the 135×44 benchmark clip from ~153 ms to well
under 100 ms on the Pi 5 host. Draft frames rasterize at the shipped frame's size, and the server
encodes them as uncompressed PNG.
**Date:** 2026-10-19
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 6ce6367

**Sources:**
- `.capcom/issues/tune-preview-playback-speed.md` (acceptance; log of the 148 ms measurement)
- `docs/plans/archived/2026-10-18 - 03 - tune-playback-plan.md` (the draft path this plan speeds up)
- Notes `coding-conventions`, `package-layout`, `glossary`

**Profile at base (BenchmarkHandler_DraftFrame, 40×, Pi 5):** PNG BestSpeed encode ~83 ms,
ApproxBiLinear scale ~42 ms (its cost follows the destination's pixels, not the source's),
scale-1 raster ~39 ms. On a 1250×826 frame, `png.NoCompression` takes ~10 ms (3.1 MB, filters
skipped) and JPEG q90 ~52 ms.

**Ratified design calls (user, 2026-10-19):**
- **Draft raster:** a draft is rasterized at the shipped frame's size. Its font size and padding
  are the exact raster's, scaled by frame.width over the exact raster's width, and never larger.
  It replaces the scale-1 draft raster. Drafts shown during a camera zoom upscale from the
  shipped-size raster and are softer than today's (user accepted, 2026-10-19).
- **Client cache:** the page's draft cache keeps its 48 entries (~150 MB of uncompressed blobs).
- **Issue:** this plan does not close `tune-preview-playback-speed`. Item 3 logs the numbers,
  and the user checks playback by eye and closes the issue.

**Writer's calls (2026-10-19, binding):** draft frames are PNGs encoded at `png.NoCompression`,
and exact frames keep `png.BestSpeed`. The draft compose keeps `draw.ApproxBiLinear`. The draft
raster cache keeps its 256 MiB bound. The route and the page's fetch logic are unchanged.

**Standing requirements:**
- skills: coding-standards
- Tests on this host: single packages, `GOMEMLIMIT=2GiB`, never `./...` with `-race`, never two at once.
- No committed file names the reference implementation (AGENTS.local.md forbidden names).
- A new file in a package gets its line in that package's `doc.go` file map (`TestDocMap`).

**Out of scope:** the exact frame path (`photon render`'s output stays byte-identical); the
take's load time; the page's prefetch depth and cache size; any wire format other than PNG.

**Regression check (2026-10-19, 6ce6367):**
- 1: guard folded (width bound, shared-stage test, shared-cache reuse, widened prose grep,
  take-free geometry test); writer's decision applied (tune acceptance, zoom softness, warm
  benchmark); supersedes the "never the reverse" draft-cache rule.
- 2: guard folded (stored-block check, pixel comparison, CHANGELOG amend); supersedes the
  `CHANGELOG.md` Unreleased scale-1 draft entry.
- 3: guard folded (storyboard length from the snapshot count, exactly 4 goroutines, no
  `b.Fatalf` off the benchmark goroutine).

## 1. Rasterize draft frames at the shipped size — ✅ DONE (2026-10-19)

NOTES (2026-10-19): `draftGeometry(loaded, faces, exact)` replaces `draftScale`/`draftGeometry(frame)`; it builds trial rasterizers to measure the rounded cell width, and in a frame too narrow for any font size (after `draftFitAttempts` = 8 shrinks) falls back to the raster geometry, so the draft stage is then the exact stage rather than failing the build.

NOTES (2026-10-19): `draftGeometryFrom` reuses the previous pipeline's draft geometry when the exact rasterizer was taken over and frame.width is unchanged, so a full-reuse rebuild still loads no terminal font; `drawsAlike` lost its geometry parameter (each caller compares its own geometry).

NOTES (2026-10-19): `TestBuild_Reuse_KeepsTheDraftRastersWhileTheDraftGeometryHolds` adapted: "scale 2 to 1" now rebuilds the draft stage (it becomes the exact one), "scale 2 to 3" still keeps it, and "the same clip" / "another width" cases were added; it now asserts the kept rasterizer and cache rather than put counts alone.

NOTES (2026-10-19): no CHANGELOG entry from this item — the Unreleased scale-1 draft entry, and the scale-1 wording in README.md, internal/tune/frame.go, internal/tune/doc.go and the glossary Note, belong to item 2, which rewrites them.

**What:**
**Goal:** a pipeline built with a draft cache draws a draft frame from a raster whose width is at
most frame.width, and as close to it as the scaled font allows. `Pipeline.DraftAt` keeps the
exact frame's pixel size, cursor and captions. When the scaled geometry equals the exact one
(the exact raster is no wider than frame.width), the draft and exact stages share one rasterizer
and cache. Draft rasters survive a rebuild exactly when the draft geometry, terminal fonts, theme
and take are unchanged.
**Approach (assumed at the header base):** in `internal/render`:
- Replace `draftScale` and `draftGeometry(frame)` with a draft geometry derived from the exact
  rasterizer: `rasterGeometry(frame)` with FontSize multiplied and Padding multiplied-and-rounded
  by `min(1, frame.Width / exact.Size().X)`.
- `buildFrames` builds or reuses the exact stage first, then the draft stage from it. The
  scale-1 sharing in `rastersFrom` becomes "share when the draft geometry equals
  `rasterGeometry`". The `Pipeline` keeps the draft geometry it built at, and
  `canReuseDraftRasters` compares that, not `draftGeometry(frame)`.
- `compose.ComposeDraft` is unchanged: `compose` already maps the crop onto a source of any size
  by its size against the layout.

**Regression guard.**
- Width bound: the raster's cell width is `round(advance)` (`internal/raster/glyph.go`), so
  multiply-and-round overshoots (renderStoryboard: 170 px > 160). After building at k, rebuild
  with FontSize scaled by frame.width / Size().X until Size().X ≤ frame.width.
- Share direction: at k=1 the draft stage is the exact stage (its rasterizer and cache);
  otherwise the draft gets a rasterizer of its own over `options.DraftCache`. This supersedes
  the "never the reverse … the draft cache never inherits the 1 GiB exact cache" rule
  (`docs/plans/archived/2026-10-18 - 03 - tune-playback-plan.md`, `internal/render/doc.go`,
  the `BuildOptions` doc in `pipeline.go`), whose sentences are rewritten to the new rule.
- Shared-cache reuse: `canReuseDraftRasters` refuses a previous whose draftRasterizer is its
  rasterizer unless the new pipeline shares that same reused exact stage too, so a rebuild never
  binds the old 1 GiB exact cache as the draft cache beside a new exact cache.
- Prose: every sentence on scale-only reuse or the share direction states the new rule,
  including "a change of frame.scale alone keeps them" and the `scale\n1` split in `render.go`.
- The benchmark-geometry test builds no take (the take helper fixes Rows: 4): it uses
  `loadRenderClipOf` on a 135×44 board, `newRasterizer`, the new draft-geometry function and
  `raster.New`.
- Acceptance also runs `GOMEMLIMIT=2GiB go test ./internal/tune` (the tune session tests build
  pipelines with a draft cache). Between items 1 and 3 BenchmarkHandler_DraftFrame may run warm;
  item 3 restores the cold path, so this is expected.
**Files:** `internal/render/render.go`, `internal/render/pipeline.go`, `internal/render/doc.go`,
`internal/render/pipeline_draft_test.go`, `internal/compose/compose.go`
**Read first:** internal/render/pipeline.go — buildFrames, draftRastersFrom, rastersFrom, canReuseDraftRasters;
internal/render/render.go — rasterGeometry, draftGeometry; internal/raster/raster.go — Size;
internal/render/pipeline_draft_test.go — TestBuild_Reuse_TheDraftStageNeverTakesTheExactRasters
**Tests:** in `pipeline_draft_test.go`:
- On the benchmark geometry (135×44, font_size 30, scale 2, width 1250), built without a take,
  the draft raster width is ≤ 1250 and ≥ 1200.
- On renderStoryboard (width 160, scale 2), the draft raster's Size().X is ≤ 160; the test
  fails with plain multiply-and-round.
- Replace `TestBuild_Reuse_TheDraftStageNeverTakesTheExactRasters` with the new rule's test: at
  k=1 (scale 1, width ≥ the raster width) the draft is the exact stage, rasterizer and cache;
  otherwise it gets a rasterizer of its own over `options.DraftCache`.
- With `countingCache`: a rebuild from a shared previous (k=1) to a pipeline whose draft
  geometry holds but whose exact geometry changes does not take the old exact cache as its
  draft cache.
- Draft rasters are kept across a rebuild with the same draft geometry, and rebuilt when
  frame.width changes.
- Adapt the existing scale-only-change reuse test to the new rule.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render ./internal/compose`;
`GOMEMLIMIT=2GiB go test ./internal/tune`;
`go vet ./internal/render ./internal/compose`
Every comment in `internal/render` and `internal/compose` that says drafts are drawn "at scale 1"
or from a "scale-1 raster", or states scale-only reuse or the old share direction, states the
new rule:
`grep -rn -i "scale 1\|scale-1\|scale alone\|never the reverse\|never takes" internal/render internal/compose`
lists no outdated sentence.
**Commit:** `perf(render): rasterize draft frames at the shipped size`

## 2. Encode draft frames uncompressed — ✅ DONE (2026-10-19)

NOTES (2026-10-19): CHANGELOG.md's Unreleased draft-frame line is amended in place, as the item's regression guard requires: it now says drafts are drawn from a raster at the shipped size, sent as an uncompressed PNG, with the draft rasters in a 256 MiB cache. This covers items 1 and 2, so this sidecar has no CHANGELOG section and the closeout adds no second, contradicting line.

NOTES (2026-10-19): the new test is `TestHandler_Frame_Draft_IsAnUncompressedPNG` (helpers `idatData`, `zlibHeaderBytes` in server_test.go; `decodedFrame` in frame_test.go). It checks that the first deflate block is stored (BTYPE 00) and that the exact frame is smaller than its draft. With the draft switched back to BestSpeed, it fails on both checks.


**What:**
**Goal:** `Session.DraftFrame` answers with a PNG encoded at `png.NoCompression`, and
`Session.Frame` with a PNG encoded at `png.BestSpeed`. Every user-facing sentence about draft
frames describes a raster at the shipped size and drops the scale-1 raster.
**Approach (assumed at the header base):** in `internal/tune/frame.go`, `encodeFrame` takes the
compression level from its caller: `DraftFrame` passes `png.NoCompression` and `Frame` passes
`png.BestSpeed`. Rewrite the draft wording ("scale-1 raster", "drawn at scale 1") in
`internal/tune/frame.go`, `internal/tune/doc.go`, `internal/tune/cache.go`,
`internal/tune/page/frames.js`, `cmd/photon/tune.go` (the `tune` help text), `README.md` (the
Tuning paragraph) and `.capcom/knowledge/glossary.md` (Draft frame) as "drawn from a raster at
the shipped size with a fast scaler".
Depends on item 1.

**Regression guard.**
- Go writes zlib level bits 0 for both levels 0 and 1, so the level bits cannot tell
  NoCompression from BestSpeed: the test checks that the first deflate block after the 2-byte
  zlib header has BTYPE 00 (stored), or the summed IDAT length ≥ width×height×3 alone.
- `TestSession_DraftFrame_IsTheExactFramesSizeWithOtherPixels` and
  `TestHandler_Frame_Draft_IsADraftOfTheExactFramesSize` `png.Decode` both frames and compare
  their Pix, not the encoded bytes.
- `CHANGELOG.md`'s Unreleased draft-frame entry ("drawn from a scale-1 raster … keeps the
  scale-1 rasters in a 256 MiB cache") is superseded: the item amends that line in place, never
  adds a second, contradicting one.
**Files:** `internal/tune/frame.go`, `internal/tune/doc.go`, `internal/tune/cache.go`,
`internal/tune/page/frames.js`, `internal/tune/server_test.go`, `internal/tune/frame_test.go`,
`cmd/photon/tune.go`, `README.md`, `.capcom/knowledge/glossary.md`, `CHANGELOG.md`
**Read first:** internal/tune/frame.go — encodeFrame, Session.DraftFrame; internal/tune/frame_test.go — TestSession_DraftFrame_IsTheExactFramesSizeWithOtherPixels;
internal/tune/server_test.go — TestHandler_Frame_Draft_IsADraftOfTheExactFramesSize, frameOf; internal/tune/cache.go — draftCacheBytes;
cmd/photon/tune.go — newTuneCommand; CHANGELOG.md — Unreleased draft-frame entry
**Tests:** in `internal/tune/server_test.go`, a draft frame response decodes as a PNG of the exact
frame's size whose IDAT data uses stored deflate blocks (the first deflate block after the
2-byte zlib header has BTYPE 00, or the summed IDAT length ≥ width×height×3), and an exact
frame response is smaller than its draft. The two "draft differs from exact" tests (in
`frame_test.go` and `server_test.go`) compare decoded Pix.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune ./cmd/photon`; `go vet ./internal/tune ./cmd/photon`;
`grep -rn -i "scale-1\|scale 1" README.md CHANGELOG.md cmd/photon internal/tune .capcom/knowledge` lists no draft sentence;
`capcom check`
**Commit:** `perf(tune): encode draft frames uncompressed`

## 3. Benchmark the draft path cold and in parallel, and log the numbers

**What:** test-only, plus a log line on the issue. `BenchmarkHandler_DraftFrame` must stay on the
cold path: with rasters at the shipped size, the 256 MiB draft cache holds about 64 of them. So
`draftBenchSnapshots` grows until every request misses the cache (at least twice the cache's
capacity at this geometry, worked out from the draft raster's size, not hard-coded from the
estimate), and the comments above it give the new capacity. Add `BenchmarkHandler_DraftFramePlaying`.
It serves cold draft frames from 4 goroutines at once, the 4 requests the page keeps in flight
(exactly 4 goroutines over a shared atomic counter), and reports
`frames/s` with `b.ReportMetric`.
Run both on the Pi 5 host and log the results with
`capcom log tune-preview-playback-speed "<ms/op, frames/s, date, commit>"`.
Depends on items 1 and 2.

**Regression guard.**
- `draftBenchStoryboard` is built from the snapshot count (beat duration ≥
  draftBenchSnapshots×draftBenchStep), so the section fits and plays 1× as its comment says;
  a fixed `6500ms` makes requests past 6.5 s a 422. The raster size may be measured first with
  `render.Build` and a probe rasterCache as DraftCache: one DraftAt, then read its used bytes.
- `b.SetParallelism` with `b.RunParallel` starts parallelism×GOMAXPROCS goroutines (16 on the
  Pi 5), so the playing benchmark runs exactly 4 (or SetParallelism(1) only when GOMAXPROCS is
  4). Its goroutines report failures with `b.Errorf` and return, or collect them, and the
  benchmark goroutine fails; none calls `b.Fatalf` (as `serveDraftFrame` does).
**Files:** `internal/tune/server_test.go`, `.capcom/issues/tune-preview-playback-speed.md`
**Read first:** internal/tune/server_test.go — BenchmarkHandler_DraftFrame, draftBenchStoryboard, draftBenchSnapshots, serveDraftFrame;
internal/schedule/schedule.go — Section.fits; internal/tune/cache.go — rasterCache.Put, draftCacheBytes;
internal/tune/session.go — NewSession
**Tests:** the two benchmarks.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run '^$' -bench 'BenchmarkHandler_DraftFrame$' -benchtime 20x`
  reports under 70 ms/op.
- `GOMEMLIMIT=2GiB go test ./internal/tune -run '^$' -bench BenchmarkHandler_DraftFramePlaying -benchtime 80x`
  reports at least 24 frames/s.
- `capcom show tune-preview-playback-speed` shows the logged line, and the issue stays open.
**Commit:** `test(tune): benchmark draft frames cold and while playing`
