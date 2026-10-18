# Tune playback — implementation plan

**Goal:** Make `photon tune`'s preview play at the clip's pace: draft frames, rasterized at
scale 1 (padding and font size unscaled) and scaled with a fast interpolator, while playing or
scrubbing; the exact render frame once the playhead settles; frames fetched ahead of the playhead
while playing.
**Date:** 2026-10-18
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** db16493
**Closes:** tune-preview-playback-speed

**Sources:**
- `.capcom/issues/tune-preview-playback-speed.md` (measurements: compose ~700 ms of CatmullRom
  downscale, raster ~100 ms and 66 MB, PNG ~26 ms, one request in flight)
- Notes `coding-conventions`, `package-layout`, `glossary`

**Ratified design calls (user, 2026-10-18):**
- **Playback fidelity:** while playing (and scrubbing) the page shows draft frames; paused,
  stepped or released it settles on the exact frame `photon render` composes.
- **Prefetch:** while playing the page keeps up to 4 frame requests in flight ahead of the
  playhead, with a small client cache keyed by state version, quality and time.

**Writer's calls (2026-10-18, binding):** a draft rasterizes at scale 1 (padding and font
size unscaled; the code's `designScale` is 2, so never "the design scale") and scales with
`draw.ApproxBiLinear`; a draft frame has the exact frame's pixel size and draws the cursor and
captions as the exact one does; `photon render` never draws drafts; the draft raster cache is
256 MiB; the route is `GET /api/frame?t=<ms>&draft=1`.

**Regression check (2026-10-18, db16493):**
- 1: guard folded (decision 1; Compose byte pin; uniform-source comparison)
- 2: guard folded (decision 2; draft-first reuse precedence; nil DraftCache; render/doc.go)
- 3: guard folded (decision 3; README and tune/doc.go wording; draft bytes differ)
- 4: guard folded (decision 4; settle ordering; paused edit exact only; README; script comments)
- 5: guard folded (held fetch answers in dom.js; no AbortController; cache owns URLs; pause settle)

**Standing requirements:**
- skills: coding-standards
- Tests on this host: single packages, `GOMEMLIMIT=2GiB`, never `./...` with `-race`, never two at once.
- No committed file names the reference implementation (AGENTS.local.md forbidden names).
- Every script in `splitScripts` (`internal/tune/page_test.go`) stays below 400 lines.
- A new file in a package gets its line in that package's `doc.go` file map (`TestDocMap`).

**Out of scope:** the take's load time at `photon tune` start (JSON decode of the take);
speeding up the exact compose (`photon render`'s output stays byte-identical); changing the
frame PNG encoding; `tune-page-js-check`.

## 1. Compose a draft frame — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the regression guard's wording fix ("scale 1", never "the design scale") was already in the plan's Goal, Writer's calls and item text at dispatch; no code or doc wording needed it, and the plan file was left untouched as the run protocol requires.

**What:**
**Goal:** `compose.Compositor` has a `ComposeDraft(t, src)` method that builds the frame at `t`
from a source raster of any size, scaled with `draw.ApproxBiLinear`,
returning an image of exactly `Size()` with the cursor and captions drawn as `Compose` draws them;
`Compose`'s output is unchanged.
**Approach (assumed at the header base):** `Compose` and `ComposeDraft` share one private body
parameterised by the interpolator and the source-to-layout ratio
(`src.Bounds().Dx()/layout.Size.X`, `Dy()/layout.Size.Y`): the camera crop from `crop(t)` is
computed in layout pixels and mapped onto `src` by that ratio before the scale or transform; the
cursor (`drawCursor`) and captions draw in output space as now. Extends `compose.go`; no new file.
**Regression guard.** The draft raster is drawn at scale 1, never called "the design scale" (the
code's designScale is 2, compose/cursor.go and caption/layout.go); fix the wording in this item, the
plan Goal and the Writer's calls line. A scale-1 raster is not an exact 1/frame.scale of the exact
raster (cell width rounds per scale), so ComposeDraft maps the camera crop onto src by the per-axis
ratio src.Bounds() to layout.Size and accepts that draft text may sit a pixel or so off the exact
frame's; tests compare only output-size, cursor and caption pixels, never terminal-text placement
against Compose. A compose_test pins `Compose`'s bytes on `stripedFrame` against an in-test copy of
today's direct path (CatmullRom `Scale` at factor 1, `Transform` with today's `Aff3` in a zoom) and
fails on any pixel change. The cursor and caption comparison uses a uniform source (one colour at
both sizes, as `solidFrame`), since both blend over the scaled base.
**Files:** internal/compose/compose.go, internal/compose/compose_test.go
**Read first:** internal/compose/compose.go — Compositor.Compose, Compositor.crop, Compositor.drawCursor;
internal/compose/cursor.go — paintAnnulus, designScale; internal/compose/compose_test.go — solidFrame,
stripedFrame, assertSameRegion
**Tests:** `ComposeDraft` on a source at the layout's size equals a frame of `Size()`; on a
uniform-colour source at a smaller size (not assumed an exact half) at factor 1 and inside a zoom,
the output size and the cursor and caption pixels match `Compose`'s on the same colour at the same
`t`; `Compose`'s bytes equal the in-test direct path at factor 1 and in a zoom; the existing compose
goldens still pass.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/compose`
**Commit:** `feat(compose): compose a draft frame from a smaller raster`

## 2. Draw draft frames from the render pipeline — ✅ DONE (2026-10-18)

NOTES (2026-10-18): when a build makes both a draft and an exact rasterizer afresh (frame.scale other than 1, nothing reused), each loads the terminal fonts once (two `fonts.Load` calls); sharing one parsed font set between them is a possible later saving, not done here.
NOTES (2026-10-18): internal/render/pipeline_test.go grows to ~580 lines with the six new tests, past the ~400-line guideline the plan's Files list kept them in.

Depends on item 1.

**What:**
**Goal:** `render.Pipeline` has `DraftAt(out)`: the frame at `out` drawn from the snapshot
rasterized at scale 1 (padding and font size unscaled) through `ComposeDraft`, kept in
`BuildOptions.DraftCache`; with a nil `DraftCache`, `DraftAt` returns `FrameAt`'s frame. A later
`Build` with `Reuse` takes over the draft rasterizer and cache while the take, the draft geometry,
the terminal fonts and the theme match — a change of `frame.scale` alone keeps them.
**Approach (assumed at the header base):** a `draftGeometry(frame)` beside `rasterGeometry`
(`rasterGeometry` at scale 1: padding and font size unscaled); `buildFrames` builds or reuses the
draft rasterizer next to the exact one (at `frame.Scale == 1` the exact stage may reuse the draft
rasterizer and cache, never the reverse); `DraftAt` shares
`screenIndexAt` and the cache get/put with `FrameAt`. `render.Run` passes no draft cache.
Extends `pipeline.go` and `render.go`; no new file.
**Regression guard.** Same terminology fix ("scale 1", not "design scale"; rename any draftGeometry
wording accordingly). Tests must not assume the draft raster is an exact half of the exact one.
Reuse precedence: `Build` takes over `Reuse`'s draft rasterizer and cache first (a 2→1 edit keeps
them though `canReuseRasters` fails); at Scale 1 the exact stage may reuse them, never the reverse,
so the draft cache never inherits the 1 GiB exact cache. With a nil `DraftCache`, `Build` builds no
draft rasterizer (no second `fonts.Load`) and `DraftAt` is `FrameAt`. Every sentence in
`render/doc.go` and the `Pipeline`/`BuildOptions` comments naming the raster cache or the reuse rules
also names the draft cache (`grep -n 'cache\|reuse'`).
**Files:** internal/render/pipeline.go, internal/render/render.go, internal/render/pipeline_test.go,
internal/render/doc.go
**Read first:** internal/render/pipeline.go — BuildOptions, buildFrames, canReuseRasters, FrameAt;
internal/render/render.go — rasterGeometry, newRasterizer; internal/render/pipeline_test.go —
TestBuild_Reuse_SharesTheRastersOnlyWhileTheyHold, oracleFrames
**Tests:** `DraftAt` returns a frame of `Size()` for every scheduled frame of a fixture clip;
outside the clip it fails with `ErrOutsideClip`; a nil `DraftCache` gives `FrameAt`'s bytes; a
rebuild changing only `frame.scale` reuses the draft cache (a counting cache sees no new put for a
snapshot already drawn), including a 2→1 edit; a nil-`DraftCache` build holds no draft rasterizer;
`TestPipeline_FrameAt_EveryScheduledFrame_EqualsThePipelineFreeOracle`
still passes.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render`
**Commit:** `feat(render): draw draft frames at scale 1`

## 3. Serve draft frames from photon tune — ✅ DONE (2026-10-18)

NOTES (2026-10-18): consequential edit — internal/tune/session.go: `build`'s doc comment now names the draft rasterizer and draft raster cache that carry over, made necessary by passing DraftCache in render.BuildOptions
NOTES (2026-10-18): the Unreleased `photon tune` entry in CHANGELOG.md (line 42) still says the page previews "frame for frame as `photon render` composes it"; the closeout may want to reword it to match the README now that playback shows draft frames

Depends on item 2.

**What:**
**Goal:** `GET /api/frame?t=<ms>&draft=1` answers the PNG of `Session.DraftFrame(out)`, drawn from
the last good pipeline's `DraftAt`; `draft` absent or `0` answers the exact frame as before; any
other `draft` value answers 400. Each session pipeline has a 256 MiB draft raster cache carried
across rebuilds as the exact one is. `photon tune --help` says playback and scrubbing show draft
frames and the preview settles on the exact frame when stopped. The `glossary` Note defines
"draft frame".
**Approach (assumed at the header base):** `Session.build` passes `DraftCache:
newRasterCache(draftCacheBytes)` in `render.BuildOptions`; `DraftFrame` shares `Frame`'s PNG
encode through one helper in `frame.go`; `handleFrame` reads the `draft` query parameter beside
`frameTimeOf`. The help text is the `Long` string in `cmd/photon/tune.go`.
**Regression guard.** Same terminology fix in the help text and glossary wording ("draft frame:
drawn from a scale-1 raster with a fast scaler"). Every sentence in README.md's Tuning paragraph,
`cmd/photon/tune.go` and `internal/tune/doc.go` claiming the preview is render's frame
(`grep -n 'render. composes\|frame for frame'`) says draft while playing or scrubbing, exact once
settled; this supersedes README.md:165-166 ("previews the take frame for frame as `photon render`
composes it"). `internal/tune/doc.go`'s `Session.Frame`/1 GiB cache sentences and the frame.go and
cache.go file-map lines also name `DraftFrame` and its 256 MiB draft cache.
**Files:** internal/tune/frame.go, internal/tune/cache.go, internal/tune/session.go,
internal/tune/server.go, internal/tune/frame_test.go, internal/tune/server_test.go,
cmd/photon/tune.go, .capcom/knowledge/glossary.md, README.md, internal/tune/doc.go
**Read first:** internal/tune/session.go — Session.build; internal/tune/frame.go — Session.Frame,
Session.goodPipeline; internal/tune/cache.go — newRasterCache, rasterCacheBytes; internal/tune/server.go
— handleFrame, frameTimeOf; internal/tune/server_test.go — TestHandler_Frame_IsTheSessionsFrameAsPNG
**Tests:** `DraftFrame` answers a PNG of the exact frame's size whose bytes differ from the exact
frame's on the scale-2 session fixture; outside the clip it is refused
as `Frame` is; the handler with `draft=1` answers `image/png` of that size, with `draft=x` 400;
`TestHandler_Frame_IsTheSessionsFrameAsPNG` still passes.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'Frame|Handler'`;
`GOMEMLIMIT=2GiB go test ./cmd/photon`; `capcom check`
**Commit:** `feat(tune): serve draft frames`

## 4. Show draft frames while playing or scrubbing — ✅ DONE (2026-10-18)

NOTES (2026-10-18): pause() settles only when it stops a play (and never during a scrub), so the pause() calls in drag.js and picker.js while already paused send no request of their own. A seek while paused and not scrubbing (Shift+arrow, a section jump, window.tune.seek) counts as a jump: it settles with a draft first.
NOTES (2026-10-18): settleFrames sends its request from a microtask, after the caller's turn, so stepFrames' pause() followed by seek() asks only at the new playhead; a draft-first flag is used up by the next settled request, so a new state while paused asks for the exact frame only. index.html loads frames.js after fit.js (still after app.js and before transport.js). README.md's Tuning sentence already matched this behaviour and was left unchanged.

Depends on item 3.

**What:**
**Goal:** The page asks for `&draft=1` frames while playing and while a scrub's pointer is down,
and for the exact frame once the playhead settles (paused, stepped, jumped, scrub released, play
ended); settling first shows a draft unless the exact frame is already at hand, then replaces it
with the exact one. An exact frame shown is never replaced by a draft of the same version and
time. Frame loading lives in a new `internal/tune/page/frames.js`.
**Approach (assumed at the header base):** move `pumpFrames`, `loadFrame` and `showFrame` from
`transport.js` into `frames.js` (loaded after `app.js`, before `transport.js`, in `index.html`),
adding `frames.js` to `splitScripts` and the `doc.go` file map. The wanted frame key becomes
`version:quality:ms`. `timeline.js`'s `startScrub`/`endScrub` tell transport a scrub began or
ended through `window.tunePage`; `transport.js` keeps the clock and the keys.
**Regression guard.** frames.js loads before transport.js, so it reaches the clock lazily through
window.tunePage; transport.js exports what frames.js needs (playhead, frameCount, frameIndexAt,
frameStartMs, playing and scrubbing flags) and stops exporting pumpFrames and frameTime, which
frames.js now owns, so transport's Object.assign does not overwrite them. A settle reads the
playhead and the play/scrub flags when its request goes out, after the caller's seek (`stepFrames`,
drag.js and picker.js call `pause()` before seeking); `startScrub` signals "scrub began" before
`tune.pause()`, and `pause()` during a scrub does not settle. A new state while paused and not
scrubbing asks for the exact frame only, no draft. README.md's Tuning sentence (amended in item 3,
superseding README.md:165-166) is checked against this behaviour. Every comment listing the page
scripts or what a script takes from or adds to window.tunePage
(`grep -n 'transport.js\|tunePage' internal/tune/page/*.js internal/tune/doc.go`) is updated.
**Files:** internal/tune/page/frames.js, internal/tune/page/transport.js,
internal/tune/page/timeline.js, internal/tune/page/index.html, internal/tune/page_test.go,
internal/tune/pagejs_test.go, internal/tune/doc.go, internal/tune/page/app.js, README.md
**Read first:** internal/tune/page/transport.js — pumpFrames, loadFrame, showFrame, pause,
Object.assign(page…); internal/tune/page/timeline.js — startScrub, endScrub; internal/tune/page/app.js
— takeState
**Tests:** goja page tests: playing requests `draft=1` frames; pausing requests the exact frame
at the playhead; a frame step requests a draft then the exact frame; a scrub drag requests drafts
and its release the exact frame; a step while playing requests no frame at the stale playhead; an
edit while paused requests the exact frame only; `TestPageJS_LoadsAndWires` and
`TestPageJS_CaptionColour_SendsTheEditAndReloadsTheFrame` still pass.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'PageJS|Page'`
**Commit:** `feat(tune): show draft frames while the preview moves`

## 5. Fetch frames ahead of the playhead

Depends on item 4.

**What:**
**Goal:** While playing, the page keeps up to 4 frame requests in flight for the next frame
times on the clip's fps grid ahead of the playhead, holds answered frames in a client cache of at
most 48 entries keyed `version:quality:ms` (object URLs revoked on eviction and on a state
version change), shows a cached frame without a request, and drops requests and entries the
playhead has passed. Paused, at most one request is in flight.
**Approach (assumed at the header base):** extends `frames.js` from item 4; the fps, length and
version come from `page.state()`; the shown `<img>` URL is never revoked while shown.
**Regression guard.** `testdata/dom.js` gains a hold and release of fetch answers
(`testDOM.holdFetches`/`releaseFetches`); its fetch answers at once today. A passed request is
dropped by ignoring its answer on arrival (and revoking its URL), never by `AbortController`, which
the test DOM lacks. The cache owns the object URLs it holds: `showFrame` revokes the previously shown
URL only when no cache entry holds it. On pause the settle request goes out at once, passed in-flight
answers are ignored, and "at most one" counts only requests sent while paused.
**Files:** internal/tune/page/frames.js, internal/tune/pagejs_test.go, internal/tune/testdata/dom.js
**Read first:** internal/tune/page/transport.js — showFrame, frameIndexAt, frameStartMs, tick;
internal/tune/testdata/dom.js — fetch, URL.createObjectURL/revokeObjectURL, HTMLImageElement src;
internal/tune/pagejs_test.go — serve
**Tests:** goja page tests, holding answers with `testDOM.holdFetches`: on play, 4 distinct
`draft=1` times ahead of the playhead are requested before any answers; a cached frame reached by the clock is shown
with no new request; after an edit bumps the version, no frame of the old version is shown; a
paused page has at most one frame request open of those sent while paused; stepping back while
paused to a cached frame shows it without a decode error.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune -run 'PageJS|Page'`
**Commit:** `feat(tune): fetch preview frames ahead while playing`
