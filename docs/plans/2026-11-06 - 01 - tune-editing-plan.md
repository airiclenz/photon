# tune editing plan

**Goal:** `photon tune` edits the frame's `cols`, `rows` and `max_colors`, reorders captions by
dragging on the caption lane, and is covered by a headless browser test against a real tune
server.
**Date:** 2026-11-06
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 0d6027b
**Closes:** (per item)

**Sources:**
- `.capcom/issues/tune-frame-size-edit.md`, `.capcom/issues/tune-lane-reorder.md`,
  `.capcom/issues/tune-browser-tests.md`
- `.capcom/knowledge/package-layout.md`, `.capcom/knowledge/glossary.md`
- `README.md` (`## Commands` Tuning paragraph, `### Header`, `### Captions`)

**Ratified design calls** (user, 2026-11-06):
- **cols/rows:** editable and savable; on a size other than the take's, tune previews at the
  take's recorded size and shows a standing "re-record it" problem; Save is allowed while that is
  the only problem; `photon render` keeps refusing the mismatch.
- **max_colors preview:** tune quantizes its exact frames (`Frame`, `FullFrame`) to `max_colors`
  with a Go median-cut palette mapped without dithering; draft frames stay unquantized.
- **Lane reorder:** captions only. The caption lane gives each caption its own row in list order,
  last listed (drawn on top) in the top row; a vertical drag past a neighbour's row sends
  `caption.move`. Camera lanes are unchanged.
- **Browser tests:** chromedp driving system Chromium, behind a `browser` build tag, skipped
  when no Chromium is found; a separate ubuntu CI job runs them. No node.

**Standing requirements:**
- skills: coding-standards
- Pi host: run single packages with `GOMEMLIMIT=2GiB`; never `./...` with `-race`; never two
  test commands at once.

**Out of scope:** reordering camera positions or zooms; re-recording from tune; quantizing draft
frames; browser tests on macOS/Windows runners.

**Regression check (2026-11-06, 0d6027b):**
- 1: decision applied (stale tune tests updated) and guard folded (defaultRefusals samples, frame-field comment grep, doc.go)
- 2: guard folded (NewSession fills problems, exported `render.CheckFrame`, Save judged on error fields, bite edit, comment grep); header call corrected: only `photon render` refuses the mismatch
- 3: guard folded (deterministic palette, lookup-table mapping)
- 4: guard folded (tests compare against `render.Quantize`, "render composes" prose grep); its `-run 'Frame'` page-test clash is settled by item 1's decision
- 5: guard folded (premise false: framePanel adds count rows; `photon tune --help` reworded)
- 6: decision applied and guard folded (no size test covers timeline.js/lanes.js, plain style.top/height on direct track children, max(rows, 1))
- 7: decision applied and guard folded (reselect destination, refused-move setup, axis commit only for caption bars, README grep, clientY drag helper)
- 8: guard folded (new `## Development` heading in README, required-browser env var, SKIP acceptance)

## 1. storyboard: frame cols, rows and max_colors are edit fields — ✅ DONE (2026-11-06)

NOTES (2026-11-06): routing needed no editFrame change — Apply already sends every `frame.` field to editFrame, whose non-decimal branch decodes a count; only the Field* constants, editFields and comments changed in non-test code.
NOTES (2026-11-06): TestSession_State_ReadOnlyClipFields_AreStateKeys renamed TestSession_State_NoReadOnlyClipFields (still matched by `-run 'ReadOnlyClipFields'`) and now asserts no clip field is read-only; full `go test ./internal/tune` passes.
NOTES (2026-11-06): Field.IsReadOnly and clipFields' read-only branch are now never set for any storyboard field, kept because the tune page's read_only rendering (panels.js framePanel) still reads it; README line 172, `photon tune --help`, panels.js and internal/tune/state.go's Fields comment still describe cols/rows/max_colors as read-only — item 5 (tune page/docs) owns them.
NOTES (2026-11-06): retry — rewrapped the Edit doc comment (internal/storyboard/edit.go) and the [Apply] paragraph (internal/storyboard/doc.go) to 100 columns; the two remaining >100 lines in those files (doc.go:56, edit.go:188) predate this item and are untouched.

**What:**
**Goal:** `storyboard.Apply` accepts `frame.cols`, `frame.rows` and `frame.max_colors`, splices
the value into `frame:` and refuses out-of-range values with the loader's own messages
(`want ≥1`, `want 2..256`); `Fields` reports all three as editable.
**Approach (assumed at the header base):** add three `Field*` constants beside the frame ones in
`internal/storyboard/edit.go`, list them in `editFields`, route them through
`(*splicer).editFrame` with `decodeCount`; range checks stay in `validateHeader` (Apply
re-validates). Reword the `Fields` doc comment and the `Edit` doc so no comment still calls them
read-only (grep `read-only\|read only` in `internal/storyboard`). A cols/rows shrink that leaves a
camera position's `cell` off the grid is refused by `Point.validate`, unchanged.
**Regression guard.** item 1 also updates the two tune tests it turns red — internal/tune/session_test.go TestSession_State_ReadOnlyClipFields_AreStateKeys becomes an assertion that no clip frame field is read-only (or is removed if nothing read-only remains to check), and internal/tune/pagejs_test.go TestPageJS_ClipFrame_EditsTheFontFields drops its read-only cols/rows/max_colors row assertion (item 5 adds the editable-input assertion); add both files to item 1's Files and `GOMEMLIMIT=2GiB go test ./internal/tune -run 'ReadOnlyClipFields|ClipFrame'` to its Acceptance; the page may show no cols/rows/max_colors rows between items 1 and 5, which is accepted inside one run.
Also: `defaultRefusals` (fields_test.go) gains samples valid for testdata/every-form.yaml (cols ≥41,
rows ≥13 — its cell {x: 40, y: 12} — max_colors 2..256), since their Default "0" is refused; the
comment rule is every comment in internal/storyboard that lists the frame's edit fields or calls a
frame field read-only — grep `frame\.fps\|read.only` (doc.go:15/26/47/61, edit.go, edit_beat.go).
**Files:** internal/storyboard/edit.go, internal/storyboard/edit_beat.go, internal/storyboard/fields.go, internal/storyboard/doc.go, internal/storyboard/edit_test.go, internal/storyboard/fields_test.go, internal/tune/session_test.go, internal/tune/pagejs_test.go
**Read first:** internal/storyboard/edit_beat.go — editFrame; internal/storyboard/edit.go — Field* consts, editFields; internal/storyboard/fields.go — clipFields; internal/storyboard/validate.go — validateHeader;
internal/storyboard/fields_test.go — defaultRefusals, TestFields_EveryEditableField_RoundTripsThroughApply; internal/tune/session_test.go — TestSession_State_ReadOnlyClipFields_AreStateKeys
**Tests:** table tests: each field set in range splices exactly one scalar; `frame.cols: 0`,
`frame.max_colors: 1` and `257` refused with the loader's message and source unchanged; a cols
shrink below a position cell refused; `Fields` marks none of the three read-only;
`defaultRefusals` gains `frame.cols`/`frame.rows`/`frame.max_colors` samples, and the
`TestFields_NameEveryEditField` order/kind rows name `frame.cols`, `frame.rows`, `frame.max_colors`
editable; the two tune tests updated as the guard says.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard`
- `go vet ./internal/storyboard`
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'ReadOnlyClipFields|ClipFrame'`
**Commit:** `feat(storyboard): make frame cols, rows and max_colors edit fields`

## 2. tune: a take recorded at another size previews at its size and stands as a problem

**What:**
**Goal:** a tune session whose storyboard frame size differs from the take's opens and keeps
previewing (frames drawn at the take's recorded cols×rows), reports one clip problem naming both
sizes and telling the user to re-record with `photon record`, and `Save` succeeds while that is
the only standing problem; `photon render` still refuses the same mismatch.
**Approach (assumed at the header base):** in `internal/render/pipeline.go` export a sentinel
`ErrTakeSize` that `checkFrame` wraps (message text unchanged). In `internal/tune/session.go`,
`build` compares the parsed board's `Frame.Cols/Rows` with `s.recorded`; on a mismatch it builds
the pipeline on a copy of the clip whose storyboard frame carries the take's size and keeps the
mismatch as a separate session field (like `takeErr`), not as the build error, so `good` still
advances. `rebuild` joins it into `problems`; `Save` ignores problems that are only this one
(`errors.Is(…, render.ErrTakeSize)`), and `validate` must not refuse it. `NewSession` no longer
fails on the mismatch; update its doc and the `State`/doc.go comments on `Cols`/`Rows`.
**Regression guard.** `NewSession` fills `s.problems` after its first build as `rebuild` does (one
shared helper joining build error, `takeErr` and the size error). Export `render.CheckFrame`
(wraps `ErrTakeSize`, text unchanged — pipeline_test.go pins `err.Error()`) for `build` to call;
`Save` refuses when any error other than the size field stands, judged from the error fields, not
`Problem` text. Comment rule: every comment in internal/tune saying a take of another size builds
no pipeline or is refused — grep `another.*size\|other than the storyboard`.
**Files:** internal/render/pipeline.go, internal/render/pipeline_test.go, internal/tune/session.go, internal/tune/state.go, internal/tune/doc.go, internal/tune/session_test.go
**Read first:** internal/tune/session.go — build, rebuild, NewSession, Save; internal/render/pipeline.go — checkFrame; internal/tune/state.go — State;
internal/tune/export.go — Export (gates on s.problems, renders s.good.loaded: keep built.loaded the real clip, pass the copy only to buildPipeline);
internal/tune/session_test.go — TestNewSession_TakeOfAnotherFrame_IsRefused
**Tests:** `errors.Is(err, render.ErrTakeSize)` from `Build` on a mismatch; session: apply
`frame.cols` change → one problem, `Frame` still served at the take's size, `Save` writes the new
cols, and a following beat duration edit changes `State().LengthMS`/`Sections` (the pipeline still
builds); `NewSession` on a mismatched pair opens with the problem
(`TestNewSession_TakeOfAnotherFrame_IsRefused` rewritten into it); another standing problem
alongside it still blocks `Save`; undo clears the problem.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/render -run 'Build|Frame'`
- `GOMEMLIMIT=2GiB go test ./internal/tune -run Session`
**Commit:** `feat(tune): keep previewing at the take's size when the frame size changes`
Depends on item 1.

## 3. render: quantize a frame to max_colors for preview

**What:**
**Goal:** `render.Quantize(img *image.RGBA, maxColors int) *image.RGBA` returns an image of the
same bounds holding at most `maxColors` distinct colours, built by median cut over the image's
colours and mapped to the nearest palette colour without dithering; an image already holding
≤ `maxColors` colours comes back pixel-identical.
**Approach (assumed at the header base):** a new file `internal/render/quantize.go`; standard
library only; colour counting first so the fast path returns early. It mirrors the palette pass's
`ditherMode = "none"` in encode.go; name that link in the doc comment. Add the file to the
package doc's file map if `docmap_test.go` requires it.
**Regression guard.** The output is deterministic: colours are ordered by a total key before the
median cut, so two calls on one image give equal `Pix` (the tune handler tests compare frame bytes).
Map each distinct colour to its palette entry once (a lookup table), then remap pixels by lookup;
never search the palette per pixel.
**Files:** internal/render/quantize.go, internal/render/quantize_test.go, internal/render/doc.go
**Read first:** internal/render/encode.go — ditherMode, paletteArgs; internal/render/doc.go — file map; internal/testkit/docmap.go — CheckDocMap;
internal/render/docmap_test.go — TestDocMap; internal/compose/compose.go — Compositor.compose (returns a fresh image)
**Tests:** distinct-colour count ≤ max for 2, 16, 256 on a gradient image; identity on a
4-colour image with max 16; bounds preserved; no alpha change on opaque input; `Quantize` twice on
an image of more than max colours gives equal `Pix`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/render -run 'Quantize|DocMap'`
**Commit:** `feat(render): quantize a frame to max_colors for preview`

## 4. tune: exact frames are quantized to the frame's max_colors

**What:**
**Goal:** `Session.Frame` and `Session.FullFrame` PNGs hold at most the storyboard's
`frame.max_colors` colours, so a `frame.max_colors` edit changes the next exact frame without a
restart; `DraftFrame` and `FullDraftFrame` are unquantized.
**Approach (assumed at the header base):** in `internal/tune/frame.go` wrap the exact frame
functions passed to `encodeFrame` so they run `render.Quantize` with the good pipeline's board
`Frame.MaxColors`. Update the `Frame`/`FullFrame` doc comments that say "before the GIF's palette
quantization".
**Regression guard.** `renderedFrame` and the FullFrame test compare against
`render.Quantize(built frame, loaded board's Frame.MaxColors)`. Reword every site equating the tune
preview's exact frame with the frame render composes — grep
``render\` composes\|render composes\|FullFrame\]'s frame`` in internal/tune, cmd/photon, README.md, .capcom/knowledge.
**Files:** internal/tune/frame.go, internal/tune/frame_test.go, internal/tune/doc.go, internal/tune/page/frames.js, cmd/photon/tune.go, README.md, .capcom/knowledge/glossary.md
**Read first:** internal/tune/frame.go — Session.Frame, Session.FullFrame, Session.FullDraftFrame, encodeFrame; internal/tune/frame_test.go — renderedFrame, assertSameFrame;
internal/tune/server_test.go — TestHandler_Frame_IsTheSessionsFrameAsPNG; internal/tune/doc.go — package comment
**Tests:** apply `frame.max_colors: 2` → decoded `Frame` PNG has ≤ 2 colours; the draft frame at
the same time has more; undo restores the count; `TestSession_Frame_AfterEditMatchesTheRenderOfTheSavedFile`,
`TestSession_FullFrame_IsTheWholeFrameWhileZoomed` and `TestSession_Frame_RewrittenTake_ShowsTheNewTake`
compare against the quantized render.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'Frame'`
**Commit:** `feat(tune): show max_colors in the paused preview`
Depends on items 1 and 3.

## 5. tune page: cols, rows and max_colors edit in the clip panel; docs

**What:**
**Goal:** the page's clip panel shows `cols`, `rows` and `max_colors` as editable inputs that
send `frame.cols`, `frame.rows`, `frame.max_colors` edits; a refused value shows the loader's
message; README's Tuning paragraph and `### Header` table describe the editing and the
re-record problem; the issue's Acceptance records the cols/rows call.
**Approach (assumed at the header base):** `internal/tune/page/panels.js` renders clip fields
from `fieldsOf(CLIP)` (read-only ones via `readOnlyRow`), so item 1 may already make them inputs:
verify and fix only what is missing. README: replace "`cols`, `rows` and `max_colors` show
read-only" with the new behaviour. In `.capcom/issues/tune-frame-size-edit.md` amend the preview
bullet to "cols/rows changes preview at the take's size with a re-record problem (ratified
2026-11-06)"; run `capcom check`.
**Regression guard.** The premise is false: `framePanel` hard-codes its inputs and lists only
`read_only` fields, so after item 1 the three rows are gone. Add `countField` rows "cols", "rows"
(`MIN_ONE`) and "max_colors" (min 2) sending `frame.cols`/`frame.rows`/`frame.max_colors`, drop the
now-empty read-only mapping, keep those labels for `hasField`; reword `photon tune --help`'s Long
text and the `newTuneCommand` comment (cmd/photon/tune.go) alongside README.
**Files:** internal/tune/page/panels.js, internal/tune/pagejs_test.go, cmd/photon/tune.go, README.md, .capcom/issues/tune-frame-size-edit.md
**Read first:** internal/tune/page/panels.js — framePanel; internal/tune/page/controls.js — countField; internal/tune/pagejs_test.go — TestPageJS_ClipFrame_EditsTheFontFields, assertEdit, assertStatusFailure;
cmd/photon/tune.go — newTuneCommand; README.md — Tuning paragraph, Header table
**Tests:** pagejs: setting the cols field sends `frame.cols` with the typed number
(`assertEdit`); `max_colors: 1` shows the refusal message in status;
`TestPageJS_ClipFrame_EditsTheFontFields` asserts the cols, rows and max_colors inputs.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'Page'`
- `capcom check`
**Closes:** tune-frame-size-edit
**Commit:** `feat(tune): edit cols, rows and max_colors on the page`
Depends on items 2 and 4.

## 6. tune page: one caption row per caption in list order

**What:**
**Goal:** the caption lane draws each caption in its own row, rows in list order with the last
listed caption in the top row; horizontal dragging still shifts a caption in time.
**Approach (assumed at the header base):** `captionBars` in `internal/tune/page/lanes.js`
returns bars with a row index; `clipLaneRow` in `timeline.js` sizes the caption lane to its row
count and places bars by row (CSS in style.css). Keep every script under `maxScriptLines`
(page_test.go): move code to a new script rather than growing past it, registering it in
index.html and doc.go's page map.
**Regression guard.** caption row assignment lives in lanes.js and is exposed on `window.tuneLanes` as a row-of-index function and its inverse (index-at-row) for item 7; timeline.js changes stay minimal (timeline.js is already over the 400-line guide and outside splitScripts — do not grow it by more than a few lines; new logic goes in lanes.js or a new script)
No size test covers timeline.js or lanes.js; a split-out script joins `splitScripts`, loads in
index.html after app.js and before timeline.js, and is mapped in doc.go. Place rows with plain
`style.top`/`style.height` on bars that stay direct children of the caption lane's one
`.lane-track` (dom.js has no `setProperty`); size the caption track to max(rows, 1) rows.
**Files:** internal/tune/page/lanes.js, internal/tune/page/timeline.js, internal/tune/page/style.css, internal/tune/pagejs_test.go (and, if a script is split out, it, internal/tune/page/index.html, internal/tune/doc.go, internal/tune/page_test.go)
**Read first:** internal/tune/page/lanes.js — captionBars, window.tuneLanes; internal/tune/page/timeline.js — clipLaneRow; internal/tune/page/style.css — .lane-track; internal/tune/pagejs_test.go — dragBar;
internal/tune/page_test.go — splitScripts, TestPage_EntryReads_AreStateKeys (a `caption.row` read fails it); internal/tune/testdata/dom.js — Element style
**Tests:** pagejs: three captions → three rows, last listed on top; a horizontal drag still
sends `caption.shift`; with no captions the caption lane is one row tall; `window.tuneLanes`
row-of-index and index-at-row invert each other.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'Page'`
**Commit:** `feat(tune): stack the caption lane one row per caption`

## 7. tune page: drag a caption up or down its lane to reorder it

**What:**
**Goal:** a vertical drag of a caption bar past a neighbouring row's midpoint sends one
`caption.move` with the destination list index on release; a drag commits to horizontal (shift)
or vertical (reorder) by the dominant axis of its first 6 px; a refused move shows its message
via `page.reportStatus` and leaves the storyboard unchanged; README's Tuning paragraph and
`### Captions` say captions reorder by dragging.
**Approach (assumed at the header base):** extend `window.tuneDrag.attach` in `drag.js` with an
optional vertical handler used only by caption bars (`attachShift` in timeline.js); row → list
index is the inverse of item 6's mapping. The Move up/down buttons stay.
**Regression guard.** the row→index inverse comes from item 6's `window.tuneLanes` function, never from reading DOM style; new drag logic goes in drag.js, and timeline.js grows by a few lines at most
The 6 px axis commit applies only to drags that carry the vertical handler; beat handles, grips and
zoom/position bars keep today's behaviour. An accepted move calls
`selectEntry({kind: "caption", index: destination})` as `moveButton` does.
**Files:** internal/tune/page/drag.js, internal/tune/page/timeline.js, internal/tune/pagejs_test.go, README.md
**Read first:** internal/tune/page/drag.js — attach, startDrag, moveDrag; internal/tune/page/timeline.js — attachShift; internal/tune/page/entries.js — moveButton;
internal/storyboard/edit_list.go — moveEntry; internal/tune/pagejs_test.go — dragBar, TestPageJS_RemoveAndMoveACaption
**Tests:** pagejs: dragging the top caption down one row sends `caption.move` to the right
index and selects the moved caption at its destination; a 3 px vertical wobble on a horizontal
drag sends only `caption.shift`; a refused move reports status and no state change — after
loadPage remove a caption in Go with `session.Apply` (the poll never runs on its own), drag to the
now-missing index, and compare `session.State()` before and after; a new drag helper fires both
`clientX` and `clientY`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'Page'`
- `grep -ci 'reorder' README.md` prints at least 2 (Tuning paragraph and `### Captions`)
**Closes:** tune-lane-reorder
**Commit:** `feat(tune): reorder captions by dragging on their lane`
Depends on item 6.

## 8. tune: headless browser test against a real tune server

**What:**
**Goal:** `go test -tags browser ./internal/tune -run Browser` starts the real tune HTTP server
on a session fixture, drives the page in headless Chromium through load, play, scrub and one edit
followed by Save, and checks the storyboard on disk changed; it skips when no Chromium is found;
a CI job runs it on ubuntu; README's development section names the command.
**Approach (assumed at the header base):** add `github.com/chromedp/chromedp` to go.mod. New
file `internal/tune/browser_test.go` with `//go:build browser`, reusing `newSessionFixture` and
`serve` on a `127.0.0.1:0` listener. Chromium flags for the Pi: headless, `--disable-gpu`,
`--disable-dev-shm-usage`, small window. In `.github/workflows/ci.yml` add a `browser` job on
`ubuntu-latest` (actions pinned by SHA like the `check` job) running the command.
**Regression guard.** README has no development section: add a `## Development` heading to
README.md naming the command. The test resolves the browser the way chromedp's allocator does,
with an exec-path override env var; the CI job sets a second env var that turns the no-Chromium
skip into a failure.
**Files:** go.mod, go.sum, internal/tune/browser_test.go, .github/workflows/ci.yml, README.md
**Read first:** internal/tune/run.go — serve; internal/tune/session_test.go — newSessionFixture; internal/tune/server.go — newHandler, guard;
internal/tune/page/app.js — poll; .github/workflows/ci.yml — check job; Makefile — test; go.mod — require
**Tests:** the browser test itself.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test -tags browser ./internal/tune -run Browser -v` (passes here with `/usr/bin/chromium`)
- the same command with the exec-path override pointing at a missing file prints `--- SKIP`
- `go vet -tags browser ./internal/tune`
- `grep -n '^## Development' README.md`
**Closes:** tune-browser-tests
**Commit:** `test(tune): drive the page in headless Chromium`
