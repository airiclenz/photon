# Tune polish — implementation plan

**Goal:** Close the ready `photon tune` issues: caption font check, a visible refused lane add,
draggable entry bars, a preview that fits a short window, editable frame font fields, the build
string on the page and in `photon version`, and a targeted camera zoom.
**Date:** 2026-10-18
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 251da89
**Closes:** (per item)

**Sources:**
- `.capcom/issues/`: `tune-caption-font-missing`, `tune-lane-add-refusal`,
  `tune-add-remove-entries`, `tune-drag-entry-bars`, `tune-fit-preview-height`,
  `tune-frame-font-edit`, `tune-show-version`, `camera-zoom-target`
- Notes `coding-conventions`, `package-layout`, `glossary`, `extension-model`

**Ratified design calls (user, 2026-10-18):**
- **Version output:** `photon version` and `--version` print the full build string; release smoke parses the version out of it.
- **Until drag:** a new `caption.shift` edit (delta ms) moves `at` and `until` together, anchors kept, one undo step.
- **Zoom aim:** a targeted camera zoom pans to its box centre over the zoom's own `over` with its `ease`.
- **Font bound:** storyboard validation bounds `frame.font_size` to the caption size range everywhere.

**Writer's calls (2026-10-18, binding):** `max_colors` stays read-only (the preview is drawn before
palette quantization); `cols`/`rows` stay read-only (they size the recorded terminal; a change
needs `photon record`); a camera zoom target is a `Target` (no `cell`); the build string is
`photon <version> (<7-char commit>[, dirty])`, `photon <version>` without VCS info; the page shows
it at the transport bar's right end; the fitted preview never shrinks below 120px tall.

**Standing requirements:**
- skills: coding-standards
- Tests on this host: single packages, `GOMEMLIMIT=2GiB`, never `./...` with `-race`, never two at once.
- `LLL` below means `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run --enable-only lll`.
- No committed file names the reference implementation (AGENTS.local.md forbidden names).
- Every script in `splitScripts` (`internal/tune/page_test.go`) stays below 400 lines.

**Out of scope:** `tune-page-js-check` (its manual `photon tune` session is the user's; it stays
open); editing `cols`, `rows`, `max_colors`; dragging a beat's own zoom bar; version or CHANGELOG
release headings.

**Regression check (2026-10-18, 251da89):**
- 2: guard folded (writer's decision: `window.tunePage.reportStatus`; supersedes timeline.js:2,
  app.js:28-29)
- 3: guard folded
- 4: guard folded (writer's decision: the unsent page-fields list in `page_test.go`)
- 5: guard folded (writer's decision); yields to lanes.js:9-10
- 6: guard folded
- 7: guard folded (writer's decision: items 7 and 8 land in one run)
- 9: guard folded
- 10: recast (writer's decision); supersedes the archived release-tooling plan's bare-version
  call and smoke.go:30-31, 435-436
- 10 (re-check): Goal rewritten to the recast guard (writer's decision: `, dirty` only warns);
  FAIL-wording guard folded
- 11: guard folded
- 12: guard folded; narrows camera.go:9-10 and README.md:439, 481
- 13: guard folded (writer's decision)
- 15: guard folded (writer's decision)

## 1. Confirm the caption font controls with a page test — ✅ DONE (2026-10-18)

NOTES (2026-10-18): added a small `hasField` page-harness helper in internal/tune/pagejs_test.go to assert the "font size" and "weight" rows show before they are edited

**What:** Test-only. A goja page test in `internal/tune/pagejs_test.go` selects caption 0 of
`cameraCaptionsStoryboard`, asserts the inspector shows the "font size" and "weight" controls
(`entries.js` `styleRows`), changes each through `setField` / `chooseField`, and asserts the
`caption.font.size` / `caption.font.weight` edit sent with `assertEdit` and a frame reload. The
rows exist on `main` (added in `a02b0e5`, after `v0.1.0`), so the session's report came from an
older build; the test is the confirmation the issue asks for.
**Files:** `internal/tune/pagejs_test.go`
**Read first:** internal/tune/pagejs_test.go — TestPageJS_CaptionColour_SendsTheEditAndReloadsTheFrame,
setField, chooseField, assertEdit; internal/tune/page/entries.js — styleRows;
internal/tune/page/controls.js — decimalField, choiceField; internal/tune/session_test.go — cameraCaptionsStoryboard
**Closes:** tune-caption-font-missing
**Tests:** `TestPageJS_CaptionFont_ShowsSizeAndWeight_AndSendsTheEdits`
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'TestPageJS_CaptionFont'`
**Commit:** `test(tune): cover the caption font size and weight controls`

## 2. Show a refused lane add on the page — ✅ DONE (2026-10-18)

**What:** Fix: a lane add's refusal is swallowed by `timeline.js` `addButton`, so it shows only
when the named entry is in the inspector.
**Regression guard.** timeline.js reaches the exported reporter as `window.tunePage.reportStatus`
(set by app.js before timeline.js runs); item 5 reuses that handle. This supersedes timeline.js:2
("builds on app.js's window.tune alone") and app.js:28-29 ("only picker.js touches it"): reword
both and `addButton`'s "only caught here" comment (`grep -n 'tunePage\|window.tune alone\|only
caught here' internal/tune/page/*.js`). app.js `edit`'s success calls `page.reportStatus("", false)`,
so the next accepted edit clears the refusal (and a standing Saved/Undone).
**Goal:** A refused add from any lane's add button shows its refusal message in the transport
bar's `#status` (failure styling), whatever the inspector holds; a later successful edit or
command clears or replaces it.
**Approach (assumed at the header base):** app.js's private `report(message, isFailure)` moves to
`transport.js` (which owns the bar) and is exported on `window.tunePage` as `reportStatus`;
app.js's `post` calls it there (app.js may not grow: it sits at the 399-line cap). `addButton`'s
rejection handler calls `page.reportStatus(error.message, true)` instead of `() => {}`.
**Files:** `internal/tune/page/app.js`, `internal/tune/page/transport.js`,
`internal/tune/page/timeline.js`, `internal/tune/pagejs_test.go`
**Read first:** internal/tune/page/app.js — report, post, edit, page (window.tunePage);
internal/tune/page/timeline.js — addButton; internal/tune/page/transport.js — the IIFE's
Object.assign(page, …); internal/tune/pagejs_test.go — TestPageJS_AddAZoomAndAPanAtThePlayhead_ThenMoveUp, text
**Closes:** tune-lane-add-refusal; tune-add-remove-entries
**Tests:** a page test adds a second zoom at a playhead inside an existing zoom (an overlap),
asserts `/api/edit` answered a refusal and `text("status")` holds the message naming both moves,
with an unrelated beat selected, then makes an accepted edit and asserts `text("status")` no
longer holds the refusal; `TestPageJS_LoadsAndWires` still passes.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'TestPage'`
**Commit:** `fix(tune): show a refused lane add in the status bar`

## 3. Move the timeline's pointer drag into drag.js — ✅ DONE (2026-10-18)

NOTES (2026-10-18): window.tuneDrag is {SNAP_MS, attach(drag), defersRedraw()}; a drag descriptor is {node, track, value, min, max, axis: {msAt, percentOf}, valueAt(ms), extent(value) -> {edge, from, to, label}, select(), edit(value), redraw()}, so drag.js snaps and clamps the value while the timeline keeps the clock mapping; timeline.js's render asks defersRedraw() instead of reading its own drag state.
NOTES (2026-10-18): timeline.js's file-top comment reworded (window.tune's edit is now used there for a lane's add; a drag's edit goes through window.tuneDrag), as the item's own change made the old wording false.

**What:** Refactor, no behaviour change, so item 5 has room.
**Regression guard.** `SNAP_MS` stays readable from timeline.js (`beatHandles`, `holdHandles`,
`zoomHandles` bound by it): drag.js exports it as `tuneDrag.SNAP_MS`. The drag descriptor also
carries what a drag selects when it starts (today `tune.select(handle.beat)`) and the redraw it
calls on refusal, cancel or a pending render (timeline's `render`). `internal/tune/doc.go`'s file
map gains a `page/drag.js` line, and the `splitScripts` comment in `page_test.go` is reworded.
**Goal:** A new script `internal/tune/page/drag.js`, loaded after `lanes.js` and before
`timeline.js` and listed in `splitScripts`, owns the timeline's pointer-drag machinery (pointer
capture, the `.drag-ghost`, 50ms snapping, the deferred redraw while a drag runs, one edit at
the end, redraw from state on refusal); beat handle drags run through it unchanged.
**Approach (assumed at the header base):** `timeline.js` `startDrag`, `moveDrag`, `endDrag`,
`cancelDrag`, `finishDrag`, `showDrag`, `SNAP_MS` and the `drag` / `isRedrawPending` state move
behind `window.tuneDrag` (the `window.tuneLanes` pattern): a drag is described by its node,
track, value, bounds, how a value is drawn, and the edit it sends. `timeline.js` keeps
rendering and asks `tuneDrag` whether a redraw is pending.
**Files:** `internal/tune/page/drag.js`, `internal/tune/page/timeline.js`,
`internal/tune/page/index.html`, `internal/tune/page_test.go`, `internal/tune/pagejs_test.go`,
`internal/tune/doc.go`
**Read first:** internal/tune/page/timeline.js — startDrag, endDrag, finishDrag, beatHandles;
internal/tune/page_test.go — splitScripts; internal/tune/testdata/dom.js — getBoundingClientRect,
testDOM.dispatch; internal/tune/doc.go — file map
**Tests:** a new page test drags a beat's duration handle (no such test exists) and asserts one
edit `{"owner":{"kind":"beat","index":1},"field":"duration","value":N}` snapped to 50ms (it sets
the beats lane `.lane-track`'s layout and dispatches pointer events on `.handle` through eval,
since `testDOM.dispatch` finds elements only by id); `TestPage_IndexReferences_AreServed` and the
`splitScripts` size test cover `drag.js`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'TestPage'`
**Commit:** `refactor(tune): move the timeline's pointer drag into drag.js`

## 4. Add a caption.shift edit — ✅ DONE (2026-10-18)

NOTES (2026-10-18): caption.shift sits after caption.until in editFields and in captionFields (and so in the pinned textOwner lists); no CHANGELOG entry — the page does not send the edit until item 5, which carries the user-facing change.
NOTES (2026-10-18): TestPage_EntryFields_AreEditFields also fails when the page sends a field unsentPageFields still lists, so item 5 must drop caption.shift from the list when entries/drag start sending it.

**What:** `storyboard.Apply` gains the field `caption.shift` (value: whole ms, may be negative):
it adds the delta to the caption's `at` offset and, when present, its `until` offset, keeping
each moment's beat anchor, as one edit. `for` captions change only `at`. Writes follow the
existing caption moment splicing (`edit_caption*.go`); a zero delta leaves the source as written;
the result is re-validated like any edit. Listed in `editFields`, the `Edit` doc comment and
`internal/storyboard/doc.go`; it is a pseudo-field like `caption.move` (not a state field).
**Regression guard.** Item 4 adds `caption.shift` to a named list in internal/tune/page_test.go of
edit fields the page does not send yet (e.g. `unsentPageFields`), consulted by
TestPage_EntryFields_AreEditFields, so the full internal/tune run stays green at item 4's commit;
Acceptance runs `GOMEMLIMIT=2GiB go test ./internal/tune/` without -skip; page_test.go joins Files.
`captionFields` describes `caption.shift` for every caption as `KindCount` (a whole number that
may be negative; `KindDuration`'s `decodeMilliseconds` refuses negatives) with Default `"0"`, an
accepted no-op in `TestFields_EveryEditableField_RoundTripsThroughApply`; the pinned
`textOwner(0)` / `textOwner(2)` lists in `TestFields_DescribeEachOwnersFields` gain it.
**Goal:** `caption.shift` on an `until` caption moves both moments by the delta, comments and
layout kept; an invalid result (e.g. a negative clip time) is refused with the validation error.
**Files:** `internal/storyboard/edit.go`, `internal/storyboard/edit_caption.go`,
`internal/storyboard/fields.go`, `internal/storyboard/doc.go`,
`internal/storyboard/edit_test.go`, `internal/storyboard/fields_test.go`, `internal/tune/page_test.go`
**Read first:** internal/storyboard/edit_caption.go — editCaption, setMoment, setCaptionUntil;
internal/storyboard/edit.go — editFields, decodeMoment; internal/storyboard/fields.go —
captionFields, listEntryFields; internal/storyboard/fields_test.go — TestFields_DescribeEachOwnersFields
**Tests:** edit tests on block and flow captions (for and until), a negative delta, a refused
shift; field-list parity tests, with `caption.shift` in the pinned caption field lists;
`caption.shift` on the unsent page-fields list.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/`;
`GOMEMLIMIT=2GiB go test ./internal/tune/`; `LLL ./internal/storyboard/...`
**Commit:** `feat(storyboard): shift a caption's start and end together`

## 5. Drag camera moves and captions on the timeline — ✅ DONE (2026-10-18)

NOTES (2026-10-18): drag.js's contract gains two parts: valueAt(ms, grabbedMs), so that a bar moves by how far the pointer travels from where it grabbed the bar (a handle ignores grabbedMs and still jumps to the pointer), and refused(message), which runs after the redraw when an edit is refused. A bar passes page.reportStatus; a handle passes a no-op, because its refusal already shows on the beat the drag selected.
NOTES (2026-10-18): a bar drag shifts by a delta snapped to 50ms (so the camera.at offset or caption.shift value moves in 50ms steps), bounded so the whole bar stays on the clip.
NOTES (2026-10-18): unsentPageFields in page_test.go is now empty (`var unsentPageFields []string`). The list and its check stay in place for fields added later.
NOTES (2026-10-18): consequential edit — internal/tune/page/app.js: made necessary by timeline.js now reporting a refused bar drag through reportStatus (header comment)
NOTES (2026-10-18): consequential edit — internal/tune/page/transport.js: made necessary by timeline.js now reporting a refused bar drag through reportStatus (header comment)
NOTES (2026-10-18): timeline.js grows from 437 to 482 lines. It is not in splitScripts, so the 400-line page test does not cover it, but it is further past the ~400-line guide.

**What:** Depends on items 2, 3 and 4.
**Regression guard.** Item 5 removes `caption.shift` from that list (page_test.go joins Files);
timeline.js reaches the status reporter as `window.tunePage.reportStatus`, the same handle item 2
exports. drag.js joins Files (the on-start select and the refusal report run through it). The
overlap case drags the zoom move into beat 2's zoom section (refused as "overlaps … section …,
which holds both tracks"), dispatching on bars through eval after setting the lane track's
layout. The item yields to lanes.js:9-10 ("Nothing here touches the page or computes timing"):
the drag's edit builder (`at.offset_ms + delta`) does not land in lanes.js.
**Goal:** Dragging a bar in the "camera zoom", "pan" or "captions" lane shifts the entry's start
and keeps its length, snapping to 50ms, and sends one edit at the end: `camera.at` with
`{beat: at.beat, offset_ms: at.offset_ms + delta}` for a camera move, `caption.shift` with the
delta for a caption. A click without movement still selects. A refused drag redraws the bar where
it was and shows the refusal through `page.reportStatus` (item 2); the drag selects its entry.
**Approach (assumed at the header base):** `timeline.js` `barNode` wires pointer events through
`window.tuneDrag` (item 3); bars carry their entry, so the moment is read from `state.camera[i].at`.
**Files:** `internal/tune/page/timeline.js`, `internal/tune/page/lanes.js`,
`internal/tune/page/style.css`, `internal/tune/pagejs_test.go`, `internal/tune/page/drag.js`,
`internal/tune/page_test.go`
**Read first:** internal/tune/page/timeline.js — barNode, clipLaneRow, endDrag;
internal/tune/page/lanes.js — cameraBars, captionBars; internal/tune/state.go — CameraState.At,
CaptionState.At; internal/storyboard/camera.go — validateCamera
**Closes:** tune-drag-entry-bars
**Tests:** page tests drag a pan bar (asserts the `camera.at` edit), drag an `until` caption bar
(asserts `caption.shift`), and drag the zoom move into beat 2's zoom section (asserts the bar
redraws in place and `#status` shows the refusal); `caption.shift` is off the unsent
page-fields list.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Commit:** `feat(tune): drag camera moves and captions on the timeline`

## 6. Fit the preview to a short window — ✅ DONE (2026-10-18)

NOTES (2026-10-18): fit.js re-fits on frame load and window resize, plus a ResizeObserver on the preview column, transport bar and timeline (queued to the next animation frame) in place of a page `load` listener, so a timeline redrawn by a new state or a pick result shown under the frame also re-fits; it measures the page's top from `document.body.getBoundingClientRect()` so a scrolled page does not skew the image's top.
NOTES (2026-10-18): index.html also gives `section.preview` an id (`preview`), and style.css sets `.preview { align-self: start }`, so the preview column's height is its own content and never the taller inspector's row; fit.js reads the column's width and what lies under the frame from it. The timeline section's id is `timeline-panel` (`timeline` is taken by its rows).
NOTES (2026-10-18): the DOM stub's ResizeObserver now records its observed elements and `testDOM.resized(id)` calls them back; `testDOM.dispatch("window", ...)` fires on the window; `window.innerHeight` defaults to 768.

**What:** Page-only; no Go change outside tests.
**Regression guard.** `nav.transport` and `section.timeline` get ids in index.html, and fit.js
measures only by `getElementById`, `getBoundingClientRect` and `window.innerHeight` (the stub has
no `querySelector`, `offsetHeight` or `documentElement`). maxHeight = innerHeight − (image top +
preview-column siblings below it + transport + timeline + gaps), never document height; a taller
inspector (it shares grid row 1) is out of reach. fit.js registers its `resize` listener before
picker.js's. Every comment enumerating the page scripts names fit.js (`grep -n 'transport.js'
internal/tune`).
**Goal:** When the window is too short for the width-fitted preview plus the transport bar and
timeline, the frame image shrinks (aspect kept, never below 120px tall) so the timeline stays
on screen; a resize re-fits; a tall window shows the width-fitted size; the pick box and via
path stay aligned at every size.
**Approach (assumed at the header base):** a new script `internal/tune/page/fit.js` (after
`app.js`, in `splitScripts`) sets the frame image's `style.maxHeight` from `window.innerHeight`
minus the height everything else on the page takes, on load, frame `load` and window
`resize`. `picker.js` `drawBox` and `path.js` `place` already follow the image's rendered box;
they also redraw on a `ResizeObserver` of the image.
**Files:** `internal/tune/page/fit.js`, `internal/tune/page/index.html`,
`internal/tune/page/style.css`, `internal/tune/page/picker.js`, `internal/tune/page/path.js`,
`internal/tune/page_test.go`, `internal/tune/pagejs_test.go`, `internal/tune/testdata/dom.js`,
`internal/tune/doc.go`, `internal/tune/page/app.js`
**Read first:** internal/tune/page/picker.js — drawBox, wire; internal/tune/page/path.js — place;
internal/tune/testdata/dom.js — ResizeObserver, Element.getBoundingClientRect;
internal/tune/pagejs_test.go — loadPage; internal/tune/page/style.css — .workspace, .inspector
**Closes:** tune-fit-preview-height
**Tests:** the DOM stub gains `window.innerHeight`; page tests set layouts, fire `resize`, and
assert the image's max height for a short window, the 120px floor, and none for a tall one; a
test arms a pick proposal, fires `resize` on a short window, and asserts `#pick-box` and
`#via-path` styles match the shrunk image's box.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'TestPage'`
**Commit:** `feat(tune): shrink the preview to fit a short window`

## 7. Make the frame's render-only fields edit fields — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the shared size constants already existed (`minCaptionSize`/`maxCaptionSize` in caption.go); validateHeader reuses them and caption.go only gains a comment saying they bound frame.font_size too.
NOTES (2026-10-18): frame-field mentions left for item 8 as page-side sites: README.md:167 (tune inspector), cmd/photon/tune.go:27 help text, internal/tune/page/panels.js:44, internal/tune/doc.go:5, internal/tune/state.go:32.

**What:** Storyboard half of `tune-frame-font-edit`; the panel is item 8.
**Regression guard.** The Clip frame panel lists only read-only clip fields until item 8 adds
inputs for the newly editable ones; that gap between the two commits is accepted because the run
fast-forwards both together — item 8 must land in the same run. `validateHeader` keeps
`frame.font_size: want >0, got 0` for ≤ 0 (TestLoadRejects "frame font size") and adds
`frame.font_size: want in [6, 72], got %g` after it; the caption-side `got 80 from
frame.font_size` report (TestLoad_CaptionSizeFromTheFrame) stays. `defaultRefusals` gains exactly
three entries, frame.font_size, frame.line_height and frame.scale (valid samples); frame.padding
takes its 0 default. Every comment naming the frame fields an Edit takes lists all six (``grep -rn
'frame.width and frame.fps\|frame.width or frame.fps\|shows but does not edit' internal``);
internal/tune's sites are item 8's.
**Goal:** `frame.font_size` and `frame.line_height` (decimals above 0) and `frame.padding`
(whole, ≥ 0) and `frame.scale` (whole, ≥ 1) are edit fields applied through `storyboard.Apply`
with comments and layout kept; validation refuses a `frame.font_size` outside the caption size
range (6..72) on load, lint and edit alike; `cols`, `rows` and `max_colors` stay read-only.
**Approach (assumed at the header base):** add the four names to `editFields`; `ownerKindOf`
routes every `frame.` field to `OwnerClip`; `editFrame` (`edit_beat.go`) gains a decimal branch
writing with `formatDecimal`. `validateHeader` checks the font size against one pair of size
constants shared with `CaptionFont.validate`. The `Fields` doc comment says why cols/rows and
max_colors are read-only. README's `frame.font_size` row states the range.
**Files:** `internal/storyboard/edit.go`, `internal/storyboard/edit_beat.go`,
`internal/storyboard/validate.go`, `internal/storyboard/caption.go`,
`internal/storyboard/fields.go`, `internal/storyboard/edit_test.go`,
`internal/storyboard/fields_test.go`, `internal/storyboard/storyboard_test.go`, `README.md`,
`internal/storyboard/doc.go`
**Read first:** internal/storyboard/edit_beat.go — editFrame; internal/storyboard/edit.go —
editFields, ownerKindOf; internal/storyboard/validate.go — validateHeader; internal/storyboard/caption.go —
CaptionFont.validate; internal/storyboard/fields.go — clipFields; internal/storyboard/fields_test.go — defaultRefusals; internal/storyboard/storyboard_test.go — TestLoadRejects
**Tests:** edit cases per new field on block and flow frames; refusals (font size 5 and 73,
padding −1, scale 0), with a TestLoadRejects case for the range wording beside the kept
`want >0` case; three `defaultRefusals` entries (font_size, line_height, scale);
`TestFields_DescribeEachOwnersFields` updated.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/`; `LLL ./internal/storyboard/...`
**Commit:** `feat(storyboard): edit the frame's font size, line height, padding and scale`

## 8. Edit the frame font fields in the Clip frame panel — ✅ DONE (2026-10-18)

NOTES (2026-10-18): `countField` now takes its `min` as a fourth argument (padding takes 0, width, fps and scale take 1) instead of hardcoding "1"; `internal/tune/doc.go`'s Edit paragraph was rewrapped after naming the four new frame fields.

**What:** Depends on item 7.
**Goal:** The tune inspector's Clip frame panel shows `font_size`, `line_height`, `padding` and
`scale` as inputs alongside `width` and `fps`, each sending its `frame.` edit and re-rendering
the preview; `cols`, `rows` and `max_colors` show read-only.
**Approach (assumed at the header base):** `panels.js` `framePanel` adds `decimalField` rows
(step 0.5 for font size, 0.05 for line height) and count rows (padding min 0, scale min 1).
`state.go`'s "shows but does not edit" comment, `cmd/photon/tune.go`'s `Long`,
`internal/tune/doc.go` and README's Tuning paragraph name the new editable fields.
**Files:** `internal/tune/page/panels.js`, `internal/tune/page/controls.js`,
`internal/tune/state.go`, `internal/tune/doc.go`, `internal/tune/session_test.go`,
`internal/tune/pagejs_test.go`, `cmd/photon/tune.go`, `README.md`
**Read first:** internal/tune/page/panels.js — framePanel; internal/tune/page/controls.js —
countField (hardcodes min "1"), decimalField; internal/tune/state.go — State; internal/tune/session_test.go —
TestSession_State_ReportsTheReadOnlyFrame; internal/tune/pagejs_test.go — setField, assertEdit; internal/render/pipeline.go — canReuseRasters
**Closes:** tune-frame-font-edit
**Tests:** a page test sets each field and asserts its edit; a session test applies a
`frame.font_size` edit and asserts the frame changes; `TestSession_State_ReportsTheReadOnlyFrame`
renamed and updated.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/ ./cmd/photon/`; `LLL ./internal/tune/... ./cmd/photon/...`
**Commit:** `feat(tune): edit the frame's font fields in the Clip frame panel`

## 9. Print the build string from photon version — ✅ DONE (2026-10-18)

NOTES (2026-10-18): `TestBuild` is the table (clean, dirty, no VCS settings, nil info) over the unexported `build(version, info)`; `TestBuildStartsWithVersion` also checks `Build()` keeps the VERSION semver. `docs/releasing.md:94` ("against the bare version") and smoke's bare-version comparison are left to item 10.

**What:** CLI half of `tune-show-version`; the page is item 11.
**Regression guard.** Every comment, help text and doc saying `photon version` prints the bare
version or "such as 1.0.0" names the build string instead, found with ``grep -rnE 'bare
version|such as `?1\.0\.0|prints the version' version.go cmd/photon README.md .capcom/knowledge``;
`cmd/photon/doc.go` joins Files.
**Goal:** `photon version` and `photon --version` print `photon <VERSION> (<7-char
vcs.revision>[, dirty])` from Go's build info, or `photon <VERSION>` when the binary carries no
VCS info; `Version()` stays the bare semver.
**Approach (assumed at the header base):** the root package gains `Build()`, a thin wrapper
over a pure helper taking the version and a `*debug.BuildInfo` (table-tested: clean, dirty, no
VCS settings, nil info). `cmd/photon/version.go` and `root.go`'s `Version:` use `Build()`. Note
`package-layout` and Note `extension-model` (via `capcom know`) say the root package holds
`Version()` and `Build()`; README's `photon version` row shows the new form.
**Files:** `version.go`, `version_test.go`, `cmd/photon/version.go`, `cmd/photon/root.go`,
`cmd/photon/version_test.go`, `README.md`, `.capcom/knowledge/package-layout.md`,
`.capcom/knowledge/extension-model.md`, `cmd/photon/doc.go`
**Read first:** version.go — Version, versionFile; cmd/photon/version.go — newVersionCommand;
cmd/photon/root.go — versionTemplate, newRootCommand; cmd/photon/version_test.go — TestVersion;
cmd/photon/readme_test.go — TestREADMEDocumentsEveryCommand; .capcom/knowledge/package-layout.md
**Tests:** `TestBuild*` table; `cmd/photon` `TestVersion` expects `photon.Build()+"\n"` for both.
**Acceptance:** `GOMEMLIMIT=2GiB go test . ./cmd/photon/`; `LLL . ./cmd/photon/...`; `capcom check`
**Commit:** `feat: print the commit and dirty state in photon version`

## 10. Read the version out of the build string in release smoke — ✅ DONE (2026-10-18)

NOTES (2026-10-18): `checkReportsVersion` parses the line with a new `parseVersionLine` (three forms; anything else FAILs quoting the raw line, "which is not a photon version line") and reads the binary's own `vcs.revision` through a small `binaryRevision` helper over `r.options.readBuildInfo`. A wrong commit FAILs as `<label> reports commit <c>, but its vcs.revision is <rev>`, and `, dirty` warns `<label> was built from a modified tree`. PASS wording is still `<label> reports "<version>"`.
NOTES (2026-10-18): the smoke fake gains `printedLine`, which builds `photon <v> (<rev[:7]>[, dirty])` from fake.revision/fake.modified, plus `binaryLine`/`brewLine` overrides for a raw line. The brew case expecting "photon 0.24.11" to fail now passes. New cases cover the three forms, a wrong bare version, a wrong commit (host and brew), an unparsable line and the dirty warning; `TestParseVersionLine` is the parser table. smoke.go was already over the ~400-line file guideline (603 lines before, about 680 now), and the parser stayed in it because the item's Files list names no new file.

**What:** Recast at the regression check (2026-10-18). Depends on item 9; keeps releases passing after it.
**Regression guard.** Writer's decision, keeping the user's ratified "full build string" call
(which supersedes the archived release-tooling plan's "photon version keeps printing the bare
version" and the smoke.go comments saying so, smoke.go:30-31 and 435-436 — the item updates those
comments): smoke accepts three line forms from `<binary> version` (host and brew paths alike) —
`photon X.Y.Z (<commit>[, dirty])`, `photon X.Y.Z`, and the bare `X.Y.Z` that releases cut before
item 9 print; the version must equal the bare release in every form. The commit, when printed,
must be a prefix of the vcs.revision read by r.options.readBuildInfo from the same binary; when
the build info is unreadable or has no vcs.revision, the commit is not compared (checkBuildStamps
already reports that). `, dirty` stays a warning, never a failure, matching smoke's existing
warn-only rule for a modified tree. Files add cmd/release/testdata/photon-0.0.0.rb (its comment
line changes exactly as the template's). A wrong-version FAIL names the parsed version, not the raw
line smoke.go:444-445 quotes today, so the pins `reports "0.24.10", not "0.24.11"` and its brew
twin (smoke_test.go:377, 402) stay unchanged; a line that does not parse is quoted raw.
**Goal:** `cmd/release` smoke accepts a binary whose `photon version` prints
`photon X.Y.Z (<commit>[, dirty])`, `photon X.Y.Z` or the bare `X.Y.Z`; it fails a version other
than the bare release or a printed commit that is not a prefix of the binary's own `vcs.revision`,
and reports `, dirty` as a warning, never a failure.
**Approach (assumed at the header base):** `smoke.go` `checkReportsVersion` parses the line
(standard library only); the Homebrew formula template's `test do` comment and
`docs/releasing.md`'s smoke bullet describe the new output.
**Files:** `cmd/release/smoke.go`, `cmd/release/smoke_test.go`,
`cmd/release/templates/photon.rb.tmpl`, `docs/releasing.md`, `cmd/release/testdata/photon-0.0.0.rb`
**Read first:** cmd/release/smoke.go — checkReportsVersion, buildSetting, smokeRun.warn;
cmd/release/homebrew.go — checkHomebrew; cmd/release/smoke_test.go — smokeFake.respondOn,
smokeFake.readBuildInfo, TestSmokeChecks; cmd/release/templates/photon.rb.tmpl — test do
**Tests:** the fake binary builds its printed line from fake.revision/fake.modified; the case
expecting "photon 0.24.11" to fail is inverted; PASS wording keeps naming the parsed version;
cases cover the three forms, a wrong version, a wrong commit, a dirty warning; the wrong-version
FAIL pins (smoke_test.go:377, 402) stay; "binaries built from a modified tree" (fake.modified
"true") warns and keeps 0 failed.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/release/`; `LLL ./cmd/release/...`
**Commit:** `fix(release): read the version out of photon version's build string`

## 11. Show the build string on the tune page

**What:** Depends on items 2 and 9.
**Regression guard.** `Session` gains an unexported build field that `Run` sets after `NewSession`,
whose signature stays (session_test.go, pick_test.go call it); `build` is asserted through `Run` in
`TestRun_PortZero_ListensOnLoopbackAndStopsOnCancel` with `Options{Build: ...}`, and the page test
sets the fixture session's field directly. transport.js looks up `#build` itself
(`document.getElementById`) and fills it from `renderTime` / `onState`, leaving app.js untouched.
**Goal:** `/api/state` carries `build` (the string `photon version` prints) and the tune page
shows it, muted, at the transport bar's right end.
**Approach (assumed at the header base):** `tune.Options` gains `Build`, set by
`cmd/photon/tune.go` from `photon.Build()`; `State` gains `Build string \`json:"build"\``
(`version` is taken by the state counter); `index.html` gains `<span id="build">`, filled by
`transport.js`.
**Files:** `internal/tune/run.go`, `internal/tune/session.go`, `internal/tune/state.go`,
`internal/tune/page/index.html`, `internal/tune/page/transport.js`,
`internal/tune/page/style.css`, `internal/tune/server_test.go`, `internal/tune/pagejs_test.go`,
`cmd/photon/tune.go`
**Read first:** internal/tune/run.go — Options, Run; internal/tune/state.go — State;
internal/tune/session.go — Session, NewSession; internal/tune/server_test.go — TestRun_PortZero_ListensOnLoopbackAndStopsOnCancel;
internal/tune/session_test.go — newSessionFixtureOf; internal/tune/page/transport.js — renderTime
**Closes:** tune-show-version
**Tests:** `TestRun_PortZero_ListensOnLoopbackAndStopsOnCancel` asserts `build` through `Run`; a
page test sets the fixture session's build and asserts `#build` text.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/ ./cmd/photon/`; `LLL ./internal/tune/... ./cmd/photon/...`
**Commit:** `feat(tune): show photon's build on the page`

## 12. Give a camera zoom move an optional target

**What:** Model and validation half of `camera-zoom-target`.
**Regression guard.** `TestKeyOrder_StructTags_GiveTheStoryboardKeyOrder`'s `ZoomMove` pin gains
`target`, and the item states where it sits (`zoomMoveKeyOrder` takes Upsert placement from it).
every-form.yaml's targeted zoom is appended last in the free gap (e.g. `at: 1800ms, over: 200ms`),
its header comment updated, with `defaultRefusals` for `moveOwner(5)` camera.over, camera.to and
camera.at. This narrows camera.go:9-10 and README.md:439, 481 ("a zoom and a pan may overlap"):
reword every such site (`grep -rnE 'may overlap|independent|zoom and a pan' README.md
internal/storyboard internal/clip`).
**Goal:** A camera zoom move decodes an optional `target` (a `Target`: text, `nth`, `area`);
validation checks it like a beat zoom's target, checks its area against the config, and refuses
a targeted zoom whose span overlaps a pan move (or another targeted zoom) on the pan track with
a message naming both moves; an untargeted zoom validates as it does today.
**Approach (assumed at the header base):** `ZoomMove` gains `Target *Target \`yaml:"target"\``;
`ZoomMove.validate` runs `Target.validate` under `zoom.target.`; `validateCamera` places a
targeted zoom's span on `held[trackPan]` too, using the same-track overlap message.
`internal/clip/clip.go` `cameraAreas` lists `zoom.target.area`. README's Camera section
documents the key and the overlap rule.
**Files:** `internal/storyboard/camera.go`, `internal/storyboard/storyboard_test.go`,
`internal/storyboard/testdata/every-form.yaml`, `internal/clip/clip.go`,
`internal/clip/clip_test.go`, `README.md`, `internal/storyboard/edit_test.go`,
`internal/storyboard/fields_test.go`
**Read first:** internal/storyboard/camera.go — validateCamera, ZoomMove.validate, placeMove;
internal/storyboard/validate.go — Target.validate; internal/storyboard/edit_test.go — TestKeyOrder_StructTags_GiveTheStoryboardKeyOrder;
internal/storyboard/fields_test.go — defaultRefusals; internal/storyboard/storyboard_test.go — wantCamera; internal/clip/clip.go — cameraAreas
**Tests:** decode cases; refusals for a bad target, an overlap with a pan in either order, an
undeclared area; the `ZoomMove` key-order pin and the new camera[5]'s `defaultRefusals`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/ ./internal/clip/`; `LLL ./internal/storyboard/... ./internal/clip/...`
**Commit:** `feat(storyboard): give a camera zoom move a target`

## 13. Edit a camera zoom's target

**What:** Depends on item 12.
**Regression guard.** Item 13 adds `camera.target` to the same unsent-fields list in
internal/tune/page_test.go (joins Files) and its Acceptance runs `GOMEMLIMIT=2GiB go test
./internal/tune/` without -skip.
**Goal:** `camera.target` sets (a target value) or removes (null) a camera zoom's target
through `storyboard.Apply`, comments and layout kept, and is refused on a pan; `Fields` lists
it for zoom moves (`KindTarget`, default null).
**Approach (assumed at the header base):** `editCamera` gains a `FieldCameraTarget` case ahead
of its `default:` branch, using `decodeTarget`, `Target.YAMLFlow` and the `Upsert` / `Replace` /
`DeleteIfPresent` pattern of the beat zoom target (`edit_beat.go`); the field joins
`editFields` and `isNullTaken`; doc comments and `internal/storyboard/doc.go` name it.
**Files:** `internal/storyboard/edit_camera.go`, `internal/storyboard/edit.go`,
`internal/storyboard/fields.go`, `internal/storyboard/doc.go`,
`internal/storyboard/edit_test.go`, `internal/storyboard/fields_test.go`, `internal/tune/page_test.go`
**Read first:** internal/storyboard/edit_camera.go — editCamera, zoomMoveKeyOrder;
internal/storyboard/edit_beat.go — replaceTarget, decodeTarget, Target.YAMLFlow; internal/storyboard/edit.go —
editFields, splicer.apply (isNullTaken); internal/storyboard/fields.go — cameraFields
**Tests:** set, replace, remove on block and flow lists; refused on a pan; refused overlap;
`camera.target` on the unsent page-fields list.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/`;
`GOMEMLIMIT=2GiB go test ./internal/tune/`; `LLL ./internal/storyboard/...`
**Commit:** `feat(storyboard): edit a camera zoom's target`

## 14. Aim a targeted camera zoom in compose

**What:** Depends on item 12.
**Goal:** A targeted camera zoom pans the camera to its target box's centre over the zoom's
own `over` with its `ease`, the target resolved on the screens shown through the move's end the
way a pan's `to` is; a target never shown fails composing naming the move; an untargeted zoom
composes as today.
**Approach (assumed at the header base):** `internal/compose/camera.go` `planMoves`'s zoom case
also appends a `move[pointF]` to the pan track, resolving with `resolveShown` over
`ScreensThrough(shownAt(start), shownAt(start+Over))`, wrapped as
`storyboard.AtCamera(index, fmt.Errorf("zoom: target: %w", err))`. `compose/doc.go` and
`render.go` `newCompositor`'s doc mention the zoom target.
**Files:** `internal/compose/camera.go`, `internal/compose/camera_test.go`,
`internal/compose/doc.go`, `internal/render/render.go`
**Read first:** internal/compose/camera.go — zoomPlanner.planMoves, planPan, shownAt;
internal/compose/zoom.go — resolveShown; internal/compose/camera_test.go — cameraTake,
TestNewRefusesAPanTargetItsMoveNeverShows; internal/compose/doc.go — package doc; internal/render/render.go — newCompositor
**Tests:** a targeted zoom's frames centre the box; a missing target is refused naming the move.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/compose/ ./internal/render/`; `LLL ./internal/compose/... ./internal/render/...`
**Commit:** `feat(compose): aim a targeted camera zoom at its target`

## 15. Set a camera zoom's target in photon tune

**What:** Depends on items 13 and 14.
**Regression guard.** Item 15 removes `camera.target` from that list and deletes the list if it
is then empty (page_test.go joins Files).
**Goal:** Tune's state carries a camera zoom's target; the camera zoom inspector shows the
target rows (as a pan's `to` does) with add and remove; "Set as zoom target" with a camera zoom
selected sends `camera.target` with the pick and reports "Zoom target set on camera N"; with
nothing zoomable selected it reports "Select a beat or a camera zoom first."
**Approach (assumed at the header base):** `state.go` `cameraStates` fills `Target` for zooms
too; `entries.js` `aimRows` adds `targetRows` wired to `camera.target`; `picker.js`
`setAsZoomTarget` gains a camera branch modelled on `setAsPanTarget`.
**Files:** `internal/tune/state.go`, `internal/tune/session_test.go`,
`internal/tune/page/entries.js`, `internal/tune/page/picker.js`, `internal/tune/pagejs_test.go`,
`internal/tune/server_test.go`, `README.md`, `internal/tune/page_test.go`
**Read first:** internal/tune/page/picker.js — setAsZoomTarget, setAsPanTarget;
internal/tune/page/entries.js — cameraPanel, aimRows; internal/tune/state.go — cameraStates, CameraState;
internal/tune/pagejs_test.go — TestPageJS_SetAsPanTarget_SendsTheProposal; internal/tune/session_test.go — cameraCaptionsStoryboard
**Closes:** camera-zoom-target
**Tests:** a page test picks across a target with a camera zoom selected (a fixture whose zoom
overlaps no pan) and asserts the `camera.target` edit; the no-selection message; state test;
`camera.target` off the unsent page-fields list (deleted when empty).
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/`; `LLL ./internal/tune/...`
**Commit:** `feat(tune): set a camera zoom's target from a pick`
