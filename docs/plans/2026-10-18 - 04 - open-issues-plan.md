# Open issues plan

**Goal:** End a timeline scrub on any pointer release, even after the axis is rebuilt mid-drag,
and measure the tune preview's draft frame latency on this host with a committed benchmark.
**Date:** 2026-10-18
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 125a8ee

**Regression check (2026-10-18, 125a8ee):**
- 1: guard folded (window pointer events in the DOM stub; axis track kept in a variable, no `querySelector`)
- 2: guard folded (benchmark builds its own 135×44 project and serves directly; `*testing.T` helpers and the 20×4 fixture take do not fit)

**Sources:**
- `.capcom/issues/tune-scrub-stuck-flag.md`
- `.capcom/issues/tune-preview-playback-speed.md`
- `.capcom/issues/tune-page-js-check.md`

**Ratified design calls:**
- **Scope:** fix the scrub flag and add a draft-frame benchmark; the manual checks stay with the
  user (Airic Lenz, 2026-10-18).

**Standing requirements:**
- skills: coding-standards
- Pi 5 host: single packages with `GOMEMLIMIT=2GiB`; never `./...` with `-race`; one test
  command at a time.

**Left to the user (not run by this plan):**
- `tune-preview-playback-speed`: play the 135×44 clip in `photon tune` on the Pi 5 and judge the
  fps by eye; close the ticket with item 2's logged number.
- `tune-page-js-check`: its parse criterion is met by `TestPage_Scripts_Parse`
  (`internal/tune/page_test.go`, goja); run the manual `photon tune` session it names, log it,
  close it.

**Out of scope:**
- Any change to draft or exact frame composition, caching or encoding.
- Page changes beyond the scrub's pointer handling.

## 1. End a scrub on any pointer release

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

## 2. Benchmark a draft frame request

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
