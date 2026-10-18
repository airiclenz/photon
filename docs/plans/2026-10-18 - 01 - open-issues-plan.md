# Open issues — implementation plan

**Goal:** Close the ready capcom issues: three small test/fix gaps, one wire codec, scripted checks
of the tune page, add/remove/reorder and styling of camera moves and captions, `via` pan editing
in `photon tune`, and an enforced 100-column wrap.
**Date:** 2026-10-18
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 59de46c
**Closes:** (per item)

**Sources:**
- `.capcom/issues/`: `caption-cut-beat-test`, `caption-font-directory`,
  `pipeline-reuse-invalidation-test`, `target-wire-one-codec`, `tune-page-js-check`,
  `tune-pan-target-test`, `tune-add-remove-entries`, `tune-caption-styling`, `tune-edit-pan-via`,
  `wrap-at-100-columns`
- Notes `coding-conventions`, `package-layout`, `glossary`

**Ratified design calls (user, 2026-10-18):**
- **JS engine:** page scripts run in tests under goja, a test-only Go dependency, against a DOM stub.
- **Wrap:** `lll` at 100 columns (tab width 4) for non-test Go files; `_test.go` exempt by rule.
- **List edits:** pseudo-fields `camera.insert|remove|move`, `caption.insert|remove|move`; `Edit` keeps its shape.
- **Reorder UI:** Move up / Move down buttons in the camera and caption inspectors.
- **New entries:** zoom to 1.5 over 600ms; pan to the frame's centre cell over 600ms; caption "Caption" for 2s; `at` the playhead's clip time; placed in start-time order.
- **Empty list:** removing the last entry deletes the `camera:` / `captions:` key.
- **to → via:** seeds `via: [centre cell, old to]`; via → to keeps the last waypoint.
- **Position seed:** switching a caption to `position` seeds the box's current laid-out top-left; switching back drops `position` (bottom/middle defaults).
- **Via path:** drawn as an SVG polyline over the frame preview while the via pan is selected.

**Writer's calls (2026-10-18, binding):** `tune-page-js-check` stays open after this plan (its
manual `photon tune` session is the user's); a colour equal to its default compares
case-insensitively.

**Standing requirements:**
- skills: coding-standards
- Tests on this host: single packages, `GOMEMLIMIT=2GiB`, never `./...` with `-race`, never two at once.
- `LLL` below means `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run --enable-only lll`.
- No committed file names the reference implementation (AGENTS.local.md forbidden names).

**Out of scope:** drag-to-reorder on lanes; editing beat zooms; any change to the CLI surface;
version or CHANGELOG release headings.

**Regression check (2026-10-18, 59de46c):**
- 1: guard folded
- 3: guard folded
- 4: guard folded
- 5: guard folded
- 6: guard folded
- 7: guard folded
- 8: guard folded (writer's decision: `pendingPageFields`)
- 9: guard folded (writer's decision)
- 10: guard folded (writer's decision)
- 11: guard folded (writer's decision)
- 12: guard folded
- 13: guard folded (writer's decision: `entryFieldName` widened here)
- 14: guard folded (writer's decision); supersedes the `setPanTo` doc comment in
  `internal/storyboard/edit_camera.go`
- 15: guard folded; supersedes the `CameraState` doc comment in `internal/tune/state.go`
- 15: rejected — run Acceptance as `go test ./internal/tune/ -skip TestPage_EntryFields_AreEditFields`;
  item 14's decision lists its via fields in `pendingPageFields`, so the full tune run stays green
- 16: guard folded (writer's decision); supersedes the `window.tunePage` header comments in
  `internal/tune/page/app.js` and `internal/tune/page/picker.js`
- 17: guard folded
- 18: guard folded
- 19: guard folded
- 20: guard folded (writer's decision)
- 21: guard folded
- 23: guard folded (writer's decision)

## 1. Test a caption anchored to a cut beat — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the variant base is a package-level `var captionCutBase` (validBase with beat 2 `cut: true` plus a kept beat 3 of 2s, one caption `at: {beat: 1}, until: {beat: 3}`); TestLoadRejects_Captions names its row type `rejectCase`, adds `cutBeatCases`, and runs both tables through the one subtest body via an embedded `basedCase{base, rejectCase}`, checking both bases parse clean.


**What:** Test-only. `TestLoadRejects_Captions` (`internal/storyboard/storyboard_test.go`) gains two
cases over a base whose beat 2 is `cut: true`: a caption `at: {beat: 2}` expecting
`captions[0]: at: beat 2 is cut, so it has no clip time`, and one `until: {beat: 2}` expecting
`captions[0]: until: beat 2 is cut, so it has no clip time`. The refusal comes from
`OutputClock.Resolve` via `Caption.validateTiming`; add a cut-beat variant of `captionBase` (or a
per-case base) rather than changing `captionBase` for every case.
**Regression guard.** Cutting beat 2 of `captionBase` shrinks its 5s clip to 3s and breaks its other
anchors, so build the variant so only the swapped fragment names the cut beat and the clip keeps
5s: e.g. `validBase` with beat 2 `cut: true` plus a kept beat 3 `duration: 2s`, captions anchored
`until: {beat: 3}`; each case swaps one anchor to `beat: 2`; assert the variant base parses clean.
**Files:** internal/storyboard/storyboard_test.go
**Read first:** internal/storyboard/storyboard_test.go — TestLoadRejects_Captions, captionBase, validBase;
internal/storyboard/caption.go — Caption.validateTiming; internal/storyboard/clock.go —
Storyboard.Clock, OutputClock.Resolve, OutputClock.Length
**Tests:** the two new cases; the cut-beat variant base parses clean.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/ -run TestLoadRejects_Captions -v`
**Closes:** caption-cut-beat-test
**Commit:** `test(storyboard): cover a caption anchored to a cut beat`

## 2. Refuse a directory named as a caption font — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the open/stat/close check moved into a small `readableFile(path) error` helper beside `CaptionFont.validateFiles`, which wraps its error as `caption_font.<name>: %w`; the directory problem reads `caption_font.<name>: <path> is a directory`; no sentinel, as no caller tells it apart
NOTES (2026-10-18): consequential edit — README.md: made necessary by the directory check; the `caption_font` row now says "Each must be a readable file, not a directory."


**What:** Fix: `config.Load` accepts a directory as `caption_font.regular`/`.bold` because
`os.Open` succeeds on one.
**Goal:** `config.Load` reports a `caption_font.regular` or `caption_font.bold` that names a
directory as a `*ValidationError` problem prefixed `caption_font.<name>: ` and naming the path.
**Approach (assumed at the header base):** in `CaptionFont.validateFiles`, after the open, `Stat`
the file and report a directory as `caption_font.<name>: <path> is a directory`; the error is a
named sentinel or a wrapped one callers can tell apart only if a caller needs to (none does now).
**Files:** internal/config/config.go, internal/config/config_test.go
**Read first:** internal/config/config.go — CaptionFont.validateFiles, Load, resolvePaths;
internal/config/config_test.go — TestLoadRejectsAnUnreadableCaptionFont, captionFontConfig,
writeConfig, writeFontFile; README.md — the `caption_font.regular`, `caption_font.bold` table row ("Each must be readable")
**Tests:** a table test beside `TestLoadRejectsAnUnreadableCaptionFont`: a directory as `regular`,
and one as `bold`, each one problem naming the field and the path.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/config/`
**Closes:** caption-font-directory
**Commit:** `fix(config): refuse a directory named as a caption font`

## 3. Test that a changed fonts dir, theme or caption font is not reused — ✅ DONE (2026-10-18)

**What:** Test-only. In `internal/render/pipeline_test.go`, extend
`TestBuild_Reuse_SharesTheRastersOnlyWhileTheyHold` (or add a sibling table) with cases that build
over a previous pipeline after changing `loaded.Config.Fonts`, after changing one theme colour
(`Theme.FG`, and one `Theme.ANSI` entry), asserting a fresh rasterizer and cache
(`built.rasterizer != previous.rasterizer`, `built.cache` is the passed cache). A second test builds
over a previous pipeline with a changed `Config.CaptionFont` and asserts the caption faces are
loaded afresh (`captionLoads` delta 1, and the face differs from the previous one). Font files the
test points at are copies of the embedded faces written into `t.TempDir()`; mutate the loaded
`*config.Config` rather than adding a config-writing helper unless the loaded copy is shared.
**Regression guard.** The caption-font test does not call `t.Parallel()` (`captionLoads` is
process-wide), as `TestBuild_TenReusedBuilds_LoadCaptionFontsOnce` does. `fonts.embedded` is
unexported: copy all six terminal faces (`fonts.Load` takes none from the embed when a dir is set)
and the two caption faces from `../fonts/*.ttf`.
**Files:** internal/render/pipeline_test.go, internal/render/render_test.go
**Read first:** internal/render/pipeline_test.go — TestBuild_Reuse_SharesTheRastersOnlyWhileTheyHold,
TestBuild_TenReusedBuilds_LoadCaptionFontsOnce; internal/render/pipeline.go — Pipeline.canReuseRasters, Pipeline.captionFacesFrom;
internal/render/render.go — captionLoads, loadCaptionFaces; internal/render/render_test.go — loadRenderClipOf; internal/fonts/fonts.go — Load
**Tests:** the new cases; existing reuse tests pass unchanged.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/render/ -run 'TestBuild_'`
**Closes:** pipeline-reuse-invalidation-test
**Commit:** `test(render): rebuild rasters and caption faces when fonts or theme change`

## 4. One wire codec for a target's nth — ✅ DONE (2026-10-18)

NOTES (2026-10-18): took the plan's second option. The exported wire struct is `storyboard.Target` itself, which only carries Text, Nth and Area, with `json:"text"`, `json:"nth,omitempty"` and `json:"area,omitempty"` tags. `storyboard.Nth` gains MarshalJSON and UnmarshalJSON with the existing error text kept. storyboard's private `wireTarget` and `wireNth` are deleted: `decodeTarget` decodes into Target and `wirePoint` embeds Target. tune's `pickTarget`, `wireNth` and `nthLast` are deleted: `pickResponse.Target` is a `storyboard.Target`, and `pickTargetOf` still turns nth 1 into the zero nth, which the wire leaves out.

NOTES (2026-10-18): the new tune table test (TestSession_PickProposal_AppliesAsAZoomTargetAndAPanDestination) applies each proposal through `storyboard.Apply` and uses a separate pan-only board (`pickPanStoryboard`), because validation refuses a camera pan that overlaps beat 1's zoom section. Real `Session.Pick` produces three rows: first match, first match in an area, and nth 2 in an area. The other rows are built by hand, as the guard requires: nth 2 with no area, last, last in an area, and an nth of 1 that must be folded.

**What:**
**Regression guard.** The exported wire struct carries `json:"nth,omitempty"` and
`json:"area,omitempty"` (decode unaffected); `pickTargetOf` keeps folding nth 1 to the omitted first
match. The proposer never proposes `last`: the `last` rows (and any shape `newPickSession`'s screen
cannot yield) build the Proposal by hand through `responseOf`, as
`TestSession_LastMatchTarget_RoundTripsThroughTheProposalAndItsApply` does.
**Goal:** exactly one type encodes and decodes a target's nth on the JSON wire and it lives in
`storyboard`; tune's pick proposal marshals through it; tune has no `wireNth`, `nthLast` or nth
`UnmarshalJSON`; a pick-proposed target round-trips through `storyboard.Apply` for first match,
nth 2, last, each with and without area.
**Approach (assumed at the header base):** give `storyboard.Nth` `MarshalJSON`/`UnmarshalJSON`
(int from 1, or `"last"`; the existing error text kept) and replace storyboard's private `wireNth`
with it inside `wireTarget`. Tune's `pickTarget` becomes the storyboard wire type (export the target
wire struct, or JSON tags with `omitempty` on `storyboard.Target` if it carries nothing else),
and `pickTargetOf`/`responseOf` produce it. Producers and consumers: `pickTargetOf`, `responseOf`,
`pickResponse.Target` (tune); `decodeTarget`, `decodePoint`/`wirePoint` (storyboard); tests in
`server_test.go` and `pick_test.go` that decode `pickResponse`.
**Files:** internal/storyboard/beat.go, internal/storyboard/edit_beat.go,
internal/storyboard/edit_camera.go, internal/storyboard/edit_test.go, internal/tune/server.go,
internal/tune/pick.go, internal/tune/pick_test.go, internal/tune/server_test.go
**Read first:** internal/storyboard/edit_beat.go — wireTarget, wireNth, decodeTarget; internal/storyboard/beat.go — Nth;
internal/tune/server.go — pickTargetOf, responseOf; internal/tune/pick.go — proposer.propose;
internal/tune/pick_test.go — TestSession_LastMatchTarget_RoundTripsThroughTheProposalAndItsApply
**Tests:** storyboard: `Nth` JSON round-trip table (1, 2, last, 0 and "first" refused). tune: a
table test that takes each proposal from `/api/pick` (or `Session.Pick`) and applies it as
`zoom.target` and as a pan's `camera.to` through `storyboard.Apply`, asserting the written target.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `! grep -nE 'wireNth|nthLast\b' internal/tune/*.go`
**Closes:** target-wire-one-codec
**Commit:** `refactor(storyboard): own the target nth wire codec`

## 5. Parse every tune page script with a JavaScript engine — ✅ DONE (2026-10-18)

NOTES (2026-10-18): goja pinned at pseudo-version v0.0.0-20261017142635-e6862644b38f (no tagged releases); its go.mod raises golang.org/x/text v0.42.0 → v0.43.0 and golang.org/x/sync v0.23.0 → v0.24.0, both linked by the shipped binary (`go version -m` confirms; goja itself is not linked) — the guard named only x/text; `go build ./cmd/photon` passes.

NOTES (2026-10-18): the issue `tune-page-js-check` stays open (writer's call); its `capcom log` of this item's outcome is left to the run's closeout, since the implementer never comments on a ticket.

**What:** Partly addresses `tune-page-js-check` (the manual session stays the user's; log this
item's outcome on that issue with `capcom log`).
**Regression guard.** goja has no tagged releases: pin its pseudo-version, not "its latest
release". Its go.mod raises `golang.org/x/text` v0.42.0 → v0.43.0, which the shipped binary links
(`internal/fonts` → `x/image/font/sfnt`): name that bump in the item's commit and build the binary,
or pin a goja pseudo-version whose go.mod needs x/text ≤ v0.42.0.
**Goal:** a Go test in `internal/tune` compiles every embedded `page/*.js` with goja and fails
naming the file and the parse error; goja is a test-only dependency pinned in `go.mod` with its
reason stated in the test file.
**Approach (assumed at the header base):** `go get github.com/dop251/goja` at its latest release;
`TestPage_Scripts_Parse` walks `pageScripts(t)` and calls `goja.Compile(name, source, true)`. If
goja cannot parse a construct the page uses (async/await, destructuring, spread), stop and report
it — never rewrite page code to suit the engine.
**Files:** go.mod, go.sum, internal/tune/page_test.go
**Read first:** internal/tune/page_test.go — pageScripts, readPageFile, TestPage_Scripts_SpellNoYAML;
internal/tune/server.go — pageFiles, pageDir; go.mod — require blocks
**Tests:** the new test; a temporary syntax error in one script makes it fail (verify, then revert).
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run TestPage_ -v`
- `go build ./cmd/photon`
**Commit:** `test(tune): parse every page script with goja`

## 6. Drive the page in a DOM stub; test "Set as pan target" — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the harness declares index.html's elements by id with their tags (regex over index.html) before the scripts run, so #frame is an img and the inputs/selects are their own classes; any other id is still made on demand as a div. The response's JSON/blob shaping lives in dom.js's fetch over a Go testHost.serve bridge (which calls serveRequest and decodes a PNG's size), and the harness also fails on console.error/warn and on unhandled promise rejections.

NOTES (2026-10-18): internal/tune/testdata/dom.js is 572 lines, past the ~400-line file guide; the plan names it as the one stub file, so it was not split.

NOTES (2026-10-18): mutation-checked: changing setAsPanTarget's owner index or its report text in picker.js fails TestPageJS_SetAsPanTarget_SendsTheProposal (both restored).

**What:** Depends on item 5.
**Regression guard.** The stub also gives what the scripts call while loading: `querySelectorAll`
(iterable), `replaceChildren`, `remove`, `setAttribute`, `dataset`, `getBoundingClientRect`,
`setPointerCapture`, `clientWidth`, `offsetLeft`/`Top`, an img `onload` fired when `src` is set
(`naturalWidth` = the PNG's width), `response.blob`, `performance.now`, `console`. `fetch` bridges
through `serveRequest(t, handler, testAddr, method, path, <its Content-Type>, body)` (else 403/415).
**Goal:** a test harness in `internal/tune` loads every page script in `index.html` order into
goja over a minimal DOM stub whose `fetch` is answered by the real tune handler, fires
`DOMContentLoaded` without a thrown error, and can dispatch events on elements by id; a test drives
a pick to a proposal, selects a pan move, clicks `#pick-pan`, and asserts the `/api/edit` body
(`{owner:{kind:"camera",index:i}, field:"camera.to", value:<the proposal's target>}`) and the
report text the page shows.
**Approach (assumed at the header base):** harness in `internal/tune/pagejs_test.go` plus the stub
in `internal/tune/testdata/dom.js` (elements created on demand by id, `addEventListener`/dispatch,
`classList`, `style`, `append`, `textContent`, `hidden`, numeric layout props for `naturalPerDisplayed`;
`requestAnimationFrame`, `ResizeObserver`, `URL.createObjectURL`, `navigator.clipboard`,
`setTimeout`/`setInterval` as manual queues). `fetch` bridges to `newHandler(session, testAddr)`
through `httptest` and resolves a promise; the harness records every request. The pick is driven
through the stage's pointer events with layout props set so output pixels equal displayed ones.
Session from `cameraCaptionsStoryboard` (a `to` pan exists there).
**Files:** internal/tune/pagejs_test.go, internal/tune/testdata/dom.js
**Read first:** internal/tune/page/picker.js — wire, setAsPanTarget, naturalPerDisplayed; internal/tune/page/transport.js — loadFrame;
internal/tune/page/app.js — request; internal/tune/server.go — guard; internal/tune/server_test.go — serveRequest, testAddr
**Tests:** `TestPageJS_LoadsAndWires`; `TestPageJS_SetAsPanTarget_SendsTheProposal`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'TestPageJS_' -v`
**Closes:** tune-pan-target-test
**Commit:** `test(tune): drive the page under goja and set a pick as the pan target`

## 7. Insert, delete and move a sequence item in yamledit — ✅ DONE (2026-10-18)

NOTES (2026-10-18): API is `InsertItem(sequence Path, index int, value string)`, `DeleteItem(sequence Path, index int)`, `MoveItem(sequence Path, from, to int)`; deleting a sequence's only item goes through `Replace(sequence, "[]")`, so the only item of a block sequence at the document's root is refused with ErrUnsupported, as Replace refuses a root block node.

NOTES (2026-10-18): a block item's span is its dash line through `itemEnd`, so comment lines between items (and a comment above the first dash) stay where they stand on a move or delete; an item whose dash has text before it on its line (the inner first item of `- - a`) is refused with ErrUnsupported.

**What:**
**Regression guard.** Move cuts and re-splices the item's whole span verbatim (dash line through
`itemEnd`, block scalars and inner comments included), never remove + one-line insert, which
`checkOneLine` refuses for a multi-line item such as every-form.yaml's via pan or block caption.
**Goal:** `yamledit` can insert a one-line item into a block or flow sequence at an index (0..len),
delete the item at an index, and move an item to another index, in both block and flow sequences,
leaving every other byte as written (comments on untouched items kept); each is covered by
table tests on block and flow sequences, nested ones included (a `via:` inside a list item).
**Approach (assumed at the header base):** extends `internal/yamledit` (no new package): new
methods on the document beside `Insert`/`Delete`/`Replace`, in a new file `sequence.go`, using the
existing `locate`/scan machinery and `Path` index steps; a block item is written `- <value>` at the
sequence's indent, a flow item `, `-joined. Deleting the last item of a sequence leaves `[]` —
the caller decides whether to delete the key. Errors reuse `ErrNotFound`/`ErrKind`/`ErrUnsupported`.
Update `doc.go`'s map if the package has one.
**Files:** internal/yamledit/sequence.go, internal/yamledit/sequence_test.go, internal/yamledit/doc.go
**Read first:** internal/yamledit/document.go — locate, apply, splice, checkOneLine; internal/yamledit/scan.go — itemEnd, blockEnd;
internal/yamledit/replace.go — blockItemSpan; internal/yamledit/doc.go — file map
**Tests:** insert at 0/middle/end, delete first/middle/last/only, move up/down/same, for block and
flow; a commented block list keeps its comments; the move and delete tests include a multi-line
block-mapping item and a `|` scalar item.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/yamledit/`
**Commit:** `feat(yamledit): insert, delete and move sequence items`

## 8. Storyboard edits that add, remove and reorder camera moves and captions — ✅ DONE (2026-10-18)

NOTES (2026-10-18): Fields describes each list's insert once, owned by index 0 (present even when the list is empty), and a remove and move on every entry (owner = that entry; move's Default is its own index, remove's null); two new FieldKinds back them: `entry` (insert, Choices zoom/pan or caption, Default `{"kind":<first>,"at":clip start}`) and `removal` (null); a move is `count`.

NOTES (2026-10-18): every-form.yaml has no 600ms gap on either camera track, so `{camera[0], camera.insert}` is listed in defaultRefusals with a nil sample (the Default is refused as an overlap); successful inserts are covered in edit_list_test.go on fixtures with room.

NOTES (2026-10-18): a new entry starting at the clip's first frame is written without an `at` key, as setMoment already does for an absent at; an insert into a list the storyboard lacks or holds as `[]` writes the whole key through Upsert as a one-item flow list.

NOTES (2026-10-18): no CHANGELOG entry — the page sends none of these edits until item 9, which carries the user-visible entry.

**What:** Depends on item 7.
**Regression guard.** Item 8 creates a `pendingPageFields` set in internal/tune/page_test.go naming its six new fields; the set exempts its names only from the "the page sends no X edit" half of TestPage_EntryFields_AreEditFields, never from the "sends a field the storyboard does not describe" half; add internal/tune/page_test.go to Files and `GOMEMLIMIT=2GiB go test ./internal/tune/ -run TestPage_EntryFields_AreEditFields` to Acceptance.
The list edits route before `editCamera`/`editCaption`'s bounds check and `board.Camera[index]`
read: insert takes 0..len, remove/move 0..len-1. Each new field's Kind and Default pass
`TestFields_EveryEditableField_RoundTripsThroughApply` on every owner of every-form.yaml, or the
case (insert, overlap) is listed in `defaultRefusals` with a valid sample.
**Goal:** `storyboard.Apply` accepts `camera.insert`, `camera.remove`, `camera.move`,
`caption.insert`, `caption.remove`, `caption.move`; each splices only its list's text, and the
result is refused when it fails validation (an insert overlapping another move on its track is
refused naming both moves).
**Approach (assumed at the header base):** new `internal/storyboard/edit_list.go`, routed from the
`editCamera`/`editCaption` switches. `insert`: owner index is the new entry's index (0..len); value
`{"kind":"zoom"|"pan"|"caption","at":<wire moment>}`; the entry is written from named preset
constants (zoom `to: 1.5`, `over: 600ms`; pan `to: {cell: {x: cols/2, y: rows/2}}`, `over: 600ms`;
caption `text: "Caption"`, `for: 2s`) as one flow line; an absent list key is created by `Upsert`
at its `keyOrder` place as a flow list. `remove`: value `null` (allowed for these fields); removing
the last item deletes the list key. `move`: value is the destination index. Add each field to
`editFields` and to `Fields()` (owner per list; a kind the page test accepts).
**Files:** internal/storyboard/edit_list.go, internal/storyboard/edit.go,
internal/storyboard/edit_camera.go, internal/storyboard/edit_caption.go,
internal/storyboard/fields.go, internal/storyboard/edit_list_test.go,
internal/storyboard/fields_test.go, internal/storyboard/doc.go, internal/tune/page_test.go
**Read first:** internal/storyboard/edit.go — splicer.apply, editFields, keyOrder; internal/storyboard/edit_camera.go — editCamera;
internal/storyboard/edit_caption.go — editCaption; internal/storyboard/fields.go — Fields;
internal/storyboard/fields_test.go — defaultRefusals, TestFields_EveryEditableField_RoundTripsThroughApply
**Tests:** each edit on a block and a flow `camera:`/`captions:` list; insert into an absent list;
remove the only entry deletes the key; overlap refused naming both; index out of range refused.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run TestPage_EntryFields_AreEditFields`
**Commit:** `feat(storyboard): add, remove and reorder camera moves and captions`

## 9. Page: add, remove and reorder camera moves and captions — ✅ DONE (2026-10-18)

NOTES (2026-10-18): consequential edit — README.md: made necessary by the page now adding, removing and reordering camera moves and captions (the tuning paragraph lists what the page edits).

NOTES (2026-10-18): each registered lane in lanes.js carries `add: {title, edit(state, ms)}`; timeline.js's `laneElement` takes trailing controls and `clipLaneRow` puts the lane's `+` button in the lane-name cell; `--lane-name-width` grew from 84px to 104px so "camera zoom" and its button fit.

NOTES (2026-10-18): added a third test, `TestPageJS_MoveButtons_DisabledAtTheListEnds` (Move up disabled on a list's first entry, Move down on its last), beside the two the plan names.

NOTES (2026-10-18): timeline.js grows from 461 to 482 lines; it is not one of `splitScripts` and was already past the ~400-line guide before this item.

NOTES (2026-10-18): added `TestPageJS_AddAZoomAndAPanAtThePlayhead_ThenMoveUp` on a new fixture, `roomyCameraStoryboard` (no beat-level zoom, room for a 600ms move at 500ms), because cameraCaptionsStoryboard refuses every zoom or pan add; it clicks "Add a zoom at the playhead", "Add a pan at the playhead" and "Move up" and asserts each /api/edit body, its 200 and the selection.

**What:** Depends on items 6 and 8.
**Regression guard.** Removes item 8's fields from `pendingPageFields`. Add sends
`at: {beat: 0, offset_ms: <playhead>}` (beat 0 is clip time, controls.js `CLIP_TIME_BEAT`;
`decodeMoment` refuses a moment without a beat). Each of the six names appears as one whole string
literal in a page script (never built as `list + ".insert"`). entries.js and timeline.js chain the
selection as `window.tune.edit(change).then(() => window.tune.selectEntry(...) /
window.tune.select(null), () => {})`; app.js stays untouched (397 of 400 lines).
**Goal:** the tune page offers, for camera moves and captions, add at the playhead (zoom, pan,
caption), Remove, Move up and Move down; each sends the item 8 edit and the selection follows the
entry (the new one after add, the moved one after a move, none after remove); `TestPageJS_` tests
drive each button and assert the `/api/edit` body.
**Approach (assumed at the header base):** each of the lanes "camera zoom", "pan" and "captions"
gets an add button in its lane-name cell (lanes.js data + timeline.js `clipLaneRow`) sending
`<list>.insert` with `owner.index` = the count of entries whose start is at or before the playhead
and `at` = `{offset_ms: playhead}` in clip time; Remove / Move up / Move down rows at the end of
`cameraPanel` and `captionPanel` (entries.js) follow `zoomPanel`'s button pattern. Keep app.js
under `maxScriptLines`; new code goes in entries.js / lanes.js. The page test's field allow-list
(`TestPage_EntryFields_AreEditFields`) must accept the new fields.
**Files:** internal/tune/page/entries.js, internal/tune/page/lanes.js,
internal/tune/page/timeline.js, internal/tune/page/style.css, internal/tune/page_test.go,
internal/tune/pagejs_test.go
**Read first:** internal/tune/page/entries.js — cameraPanel, captionPanel; internal/tune/page/panels.js — zoomPanel;
internal/tune/page/timeline.js — clipLaneRow; internal/tune/page/lanes.js — window.tuneLanes; internal/tune/page/app.js — edit, selectEntry;
internal/storyboard/edit.go — decodeMoment
**Tests:** `TestPageJS_AddCaptionAtThePlayhead`, `TestPageJS_RemoveAndMoveACameraMove`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'TestPage' -v`
**Closes:** tune-add-remove-entries
**Commit:** `feat(tune): add, remove and reorder camera moves and captions`

## 10. Storyboard edits for a caption's font, colours, box and fade — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the new kinds are `KindColor` ("color") and `KindFraction` ("fraction"); font.size is a `KindDecimal` and font.weight a `KindEnum` (regular, bold) whose choices `weightChoices` the edit also checks against.

NOTES (2026-10-18): caption.font.size 0 or below is refused at decode ("want a size above 0"), since a written `size: 0` would mean frame.font_size; sizes and opacities outside their bounds are left to validation, which refuses them.

NOTES (2026-10-18): an empty `font:` (a null) counts as an absent mapping: a non-default value deletes and reinserts it as a one-key flow mapping (yamledit's Replace cannot splice over a value with no text), and a default deletes it.

NOTES (2026-10-18): setPlacement's spelling check is extracted as decodeSpelling in edit_caption.go so the weight edit reuses it; editCaption's last case is now an explicit caption.align, with a default that errors on an unknown caption field.

**What:**
**Regression guard.** Adds its new caption style fields to `pendingPageFields` (internal/tune/page_test.go in Files; the same tune test command in Acceptance).
`TestApply_CaptionEditOnWhatIsNotThere_IsRefused`'s unknown-field sample `caption.color` becomes one
that stays unknown (e.g. `caption.colour`). A value equal to its default goes to `DeleteIfPresent`
(then the emptied mapping is dropped); only a non-default goes through `writeDuration`, which
rewrites a present key. Colours are written through `yamledit.Quote` (a bare `#` is a YAML comment).
**Goal:** `storyboard.Apply` accepts `caption.font.size`, `caption.font.weight`, `caption.color`,
`caption.box.color`, `caption.box.opacity`, `caption.fade.in`, `caption.fade.out` on block and
flow captions; a value equal to its default removes the key (and an emptied `font:`/`box:`/`fade:`
mapping is removed with it); an absent nested mapping is created; an invalid result is refused.
`Fields()` lists each with its default (`font.size`'s is the board's `frame.font_size`).
**Approach (assumed at the header base):** new `internal/storyboard/edit_caption_style.go` routed
from `editCaption` (whose last case is a `default:` — make every case explicit). Defaults are the
ones `applyDefaults` uses (weight regular, `#ffffff`, `#000000`, opacity 0.6, fades 0); colours
compare case-insensitively. Durations through `writeDuration`; nested keys at `keyOrder` place.
New field kinds the page needs: `color` and `fraction` (or reuse `decimal` with bounds) — add to
`Field.Kind`'s set.
**Files:** internal/storyboard/edit_caption_style.go, internal/storyboard/edit_caption.go,
internal/storyboard/edit.go, internal/storyboard/fields.go,
internal/storyboard/edit_caption_style_test.go, internal/storyboard/fields_test.go,
internal/storyboard/doc.go, internal/storyboard/edit_test.go, internal/tune/page_test.go
**Read first:** internal/storyboard/edit_caption.go — editCaption, captionKeyOrder; internal/storyboard/edit.go — writeDuration;
internal/storyboard/caption.go — applyDefaults; internal/storyboard/fields.go — captionFields; internal/storyboard/fields_test.go — defaultRefusals;
internal/storyboard/edit_test.go — TestApply_CaptionEditOnWhatIsNotThere_IsRefused; internal/yamledit/upsert.go — DeleteIfPresent
**Tests:** each field set/changed/reset-to-default on a block and a flow caption; size 80 and
opacity 1.5 refused; fade longer than the caption refused.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run TestPage_EntryFields_AreEditFields`
**Commit:** `feat(storyboard): edit a caption's font, colours, box and fade`

## 11. Storyboard edits for a caption's offset and position — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the new kind is `KindPixels` ("pixels"), typed `{"x": 8, "y": -4}`: whole numbers, both keys required, any other key refused. Fields() lists caption.offset (default `{"x":0,"y":0}`) and caption.position (default null) after caption.align, in the caption's key order.

NOTES (2026-10-18): setPlacement (edit_caption.go) now deletes a position for caption.place and caption.align, the default spelling too, so `bottom` or `middle` on a positioned caption puts it back at bottom middle. An offset or position is written as a `{x: 8, y: -4}` flow mapping over the whole node; a key holding no text (an empty `offset:`) is deleted and inserted again, the same way item 10 handles an empty `font:`.

NOTES (2026-10-18): edit_test.go gains a null caption.offset case in TestApply_CaptionNull_IsRefusedButForUntil; the Apply doc paragraph is rewrapped to 100 columns where the new sentences landed.

NOTES (2026-10-18): consequential edit — internal/storyboard/doc.go: made necessary by caption.offset and caption.position joining edit_caption_style.go, which the doc map's line lists the edits of

**What:** Depends on item 10.
**Regression guard.** Adds `caption.offset` and `caption.position` to `pendingPageFields` (same Files and Acceptance additions).
`FieldCaptionPosition` joins `apply`'s null exemption beside zoom and `caption.until` (edit.go) and
the `Edit` doc comment's null sentence.
**Goal:** `storyboard.Apply` accepts `caption.offset` (`{x,y}`, `{0,0}` removes the key) and
`caption.position` (`{x,y}` writes `position` and deletes `place`, `align`, `offset`; `null`
deletes `position`); `caption.place`, `caption.align` and `caption.offset` on a positioned caption
delete `position` in the same edit; each result validates.
**Approach (assumed at the header base):** in `edit_caption_style.go`, one splice sequence per
edit so validation (`validatePlacement`) sees only the final source. A new `pixels` field kind.
**Files:** internal/storyboard/edit_caption_style.go, internal/storyboard/edit_caption.go,
internal/storyboard/edit.go, internal/storyboard/fields.go,
internal/storyboard/edit_caption_style_test.go, internal/storyboard/fields_test.go,
internal/tune/page_test.go
**Read first:** internal/storyboard/edit.go — apply; internal/storyboard/edit_caption.go — setPlacement;
internal/storyboard/caption.go — validatePlacement; internal/storyboard/fields_test.go — defaultRefusals, TestFields_EveryEditableField_RoundTripsThroughApply;
internal/storyboard/edit_test.go — TestApply_CaptionNull_IsRefusedButForUntil; internal/storyboard/testdata/every-form.yaml — captions[1] (positioned)
**Tests:** switch place/align/offset → position and back, block and flow captions.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run TestPage_EntryFields_AreEditFields`
**Commit:** `feat(storyboard): edit a caption's offset and position`

## 12. Tune state carries a caption's style and laid-out box — ✅ DONE (2026-10-18)

NOTES (2026-10-18): new `/api/state` caption keys: `offset {x,y}`, `position {x,y}` (omitted when unset), `font_size`, `font_weight`, `color`, `box_color`, `box_opacity`, `fade_in_ms`, `fade_out_ms` (defaults filled as Parse resolves them, colours as #rrggbb), `box {x,y,w,h}` in shipped pixels and `box_position {x,y}`, its top-left in design pixels (box min / unit, rounded); both box keys are omitted unless `s.current.frames` built in full.

NOTES (2026-10-18): accessors added: `caption.Layer.Boxes`, `compose.Compositor.CaptionBoxes` and `Unit`, `render.Pipeline.CaptionBoxes` and `Unit` (none / 0 on a partial pipeline, added to the Pipeline doc's partial-pipeline list). internal/tune/session.go needed no change: `State()` lives in state.go, which now passes `s.current.frames` to `captionStates`.

**What:** Depends on item 11.
**Regression guard.** The box is in shipped pixels, position/offset in design pixels: `CaptionState`
also carries the box's top-left in design pixels (box min / unit, rounded), or the unit. Boxes come
only from `s.current.frames` when it built in full (compositor non-nil), else they are omitted. The
accessors reach the unexported layer and boxes through internal/compose and internal/caption.
**Goal:** `/api/state`'s caption entries carry font size and weight, colour, box colour and
opacity, fade in/out, offset, position, and the caption's laid-out box `{x,y,w,h}` in output
pixels as the render pipeline draws it (absent when the pipeline is not built).
**Approach (assumed at the header base):** extend `CaptionState` (`internal/tune/state.go`); the
box comes from the last good pipeline's caption layout — add the narrowest accessor to
`render.Pipeline` (and `caption` if needed) rather than laying captions out again in tune.
**Files:** internal/tune/state.go, internal/tune/session.go, internal/tune/session_test.go,
internal/render/pipeline.go, internal/render/pipeline_test.go, internal/compose/compose.go,
internal/compose/compose_test.go, internal/caption/caption.go, internal/caption/caption_test.go
**Read first:** internal/tune/state.go — CaptionState, captionStates; internal/tune/session.go — Session (current, good);
internal/render/pipeline.go — Pipeline; internal/compose/compose.go — Output; internal/caption/caption.go — Layer;
internal/caption/layout.go — boxRect; internal/tune/session_test.go — TestSession_State_ReportsCameraMovesAndCaptions
**Tests:** a session test reads the new keys for a styled caption and a box that matches the
pipeline's layout; `TestSession_State_ReportsCameraMovesAndCaptions` extends `wantCaptions` with the
style keys and checks the pipeline-dependent box separately; `TestPage_EntryReads_AreStateKeys`
still passes.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/render/ -run 'TestBuild_|TestPipeline'`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Commit:** `feat(tune): report a caption's style and laid-out box`

## 13. Page: caption style controls — ✅ DONE (2026-10-18)

NOTES (2026-10-18): internal/tune/testdata/dom.js is unchanged: the DOM stub already fires an img's onload when its src is set (item 6), so loadFrame's reload after an edit runs under the tests as it stands.

NOTES (2026-10-18): the offset row, like place and align, shows only while the caption has no position, since a position replaces all three; TestPageJS_CaptionPosition_SeedsTheLaidOutCorner also unchecks the box and asserts the null edit.

**What:** Depends on items 6 and 12.
**Regression guard.** Widens `entryFieldName` in internal/tune/page_test.go to admit dotted field names (caption.font.size, caption.box.opacity, camera.via.insert) and removes items 10 and 11's fields from `pendingPageFields`.
Every sent field is spelled as one whole string literal. dom.js fires an img's `onload` when `src`
is set (`loadFrame` waits on it). "Explicit position" seeds from item 12's design-pixel corner, and
its checkbox is disabled while the caption has no box.
**Goal:** the caption inspector shows a number for font size, a select for weight, colour pickers
for text and box colour, a 0–1 slider for box opacity, millisecond inputs for fade in/out, x/y
numbers for offset, and an "explicit position" checkbox with x/y numbers (checking it sends
`caption.position` seeded with the laid-out box's top-left; unchecking sends `null`); each sends
its item 10/11 edit and the preview frame reloads after an accepted edit.
**Approach (assumed at the header base):** colour (`<input type=color>`) and slider controls in
controls.js beside `choiceField`; rows in entries.js `captionPanel` (place/align rows shown only
without a position). Keep each split script under `maxScriptLines`; a new script goes into
`index.html`, `splitScripts` and the page tests' load order.
**Files:** internal/tune/page/controls.js, internal/tune/page/entries.js,
internal/tune/page/style.css, internal/tune/page_test.go, internal/tune/pagejs_test.go,
internal/tune/testdata/dom.js
**Read first:** internal/tune/page/entries.js — captionPanel, captionForField; internal/tune/page/controls.js — choiceField;
internal/tune/page_test.go — entryFieldName, TestPage_EntryFields_AreEditFields; internal/tune/page/transport.js — loadFrame, showFrame;
internal/tune/page/app.js — applyEdit
**Tests:** `TestPageJS_CaptionColour_SendsTheEditAndReloadsTheFrame` (asserts the edit body and a
following `/api/frame` request); `TestPageJS_CaptionPosition_SeedsTheLaidOutCorner`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'TestPage' -v`
**Closes:** tune-caption-styling
**Commit:** `feat(tune): edit a caption's font, colours, box, fade and position`

## 14. Storyboard edits for a pan's via waypoints — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the shared centre-cell helper is `centreCell(frame) Point` beside a new `pointFlow(Point)` in edit_camera.go (decodePoint now spells through pointFlow); item 8's `entryFlow` in internal/storyboard/edit_list.go, which the plan's Files did not list, now uses both, as the plan's "one shared helper" asks; its output is unchanged.

NOTES (2026-10-18): Fields gives every pan a `camera.path` (KindEnum, choices to/via, defaulting to the path the pan follows, so applying it writes nothing) and a via pan `camera.via` and `camera.via.insert` (new KindWaypoint, "waypoint") and `camera.via.remove` (new KindWaypointRemoval, "waypoint_removal"), each defaulting to null; a via pan still lists `camera.to`, refused as before. fields_test.go adds them to defaultRefusals (gofmt realigned that map's keys) and to the owner/field tables, a straight pan's (camera[2]) owner row among them.

NOTES (2026-10-18): setPanTo's refusal now reads "the pan follows a via path: set its waypoints with camera.via, or make it a straight pan with camera.path"; a waypoint edit on a straight pan or a zoom, and a waypoint index out of range, are refused before splicing; removing to one waypoint is refused by validation (`pan.via: want at least 2 points, got 1`).

**What:** Depends on items 4, 7 and 8.
**Regression guard.** Adds its four via fields to `pendingPageFields` (same Files and Acceptance additions).
edit_test.go's unknown-field case `camera.via` moves to a still-unknown field (e.g. `camera.speed`).
`setPanTo`'s refusal points at camera.via/camera.path, keeping the "the pan follows a via path" prefix
edit_test.go pins; this supersedes its doc comment ("waypoints are edited in the storyboard itself").
**Goal:** `storyboard.Apply` accepts `camera.via` (value `{"index":i,"point":<wire point>}` sets
waypoint i), `camera.via.insert` (`{"index":i,"point":...}`), `camera.via.remove` (`{"index":i}`),
and `camera.path` (`"to"` or `"via"`): to → via writes `via: [<centre cell>, <old to>]` and deletes
`to`; via → to writes `to: <last waypoint>` and deletes `via`. Removing below two waypoints is
refused by validation; `camera.to` on a via pan stays refused.
**Approach (assumed at the header base):** new `internal/storyboard/edit_via.go` routed from
`editCamera`; points through `decodePoint`; sequence splices through item 7's yamledit methods;
the centre cell is `{x: cols/2, y: rows/2}` from the board's frame, the same rule as item 8's pan
preset (one shared helper).
**Files:** internal/storyboard/edit_via.go, internal/storyboard/edit_camera.go,
internal/storyboard/edit.go, internal/storyboard/fields.go, internal/storyboard/edit_via_test.go,
internal/storyboard/fields_test.go, internal/storyboard/doc.go, internal/storyboard/edit_test.go,
internal/tune/page_test.go
**Read first:** internal/storyboard/edit_camera.go — editCamera, setPanTo, decodePoint; internal/storyboard/camera.go — PanMove.validate, minViaPoints;
internal/storyboard/edit_test.go — TestApply_CameraEditOnWhatIsNotThere_IsRefused; internal/storyboard/fields.go — cameraFields;
internal/storyboard/fields_test.go — defaultRefusals (camera[4] via pan)
**Tests:** each edit on block and flow `via:` lists, with target and cell points; remove to one
waypoint refused; switch both ways and undo by re-applying the inverse.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run TestPage_EntryFields_AreEditFields`
**Commit:** `feat(storyboard): edit a pan's via waypoints`

## 15. Tune state carries a via pan's waypoints and its path in frame pixels — ✅ DONE (2026-10-18)

NOTES (2026-10-18): re-derived from the assumption that the camera is assembled in camera.go/compose.go: `zoomPlanner.plan`, which calls `planMoves` and `newCamera`, lives in internal/compose/zoom.go, so it now takes `planMoves`' new `waypoints` result (each via pan's resolved points, keyed by its storyboard camera index) and sets it on the camera; internal/compose/path.go needed no change.

NOTES (2026-10-18): consequential edit — internal/compose/doc.go: made necessary by the new `Compositor.Path` and the camera's per-index via points (file map lines for compose.go and camera.go).

NOTES (2026-10-18): wire: camera entries gain `via: [{cell: {x,y}} | {target: {text,nth,area}}]` (omitted unless a via pan; `via_count` kept); `GET /api/path?camera=<i>&t=<ms>` answers `{"points": [{"x","y"}, ...]}`, one per via point in order (the last the destination), whole output pixels through the crop at t, unclipped; 422 (`ErrNoPath`) for a zoom, a pan to or an index out of range, 400 for a non-integer camera or t. Accessor chain: `compose.Compositor.Path` -> `render.Pipeline.Path` (false on a partial pipeline) -> `tune.Session.Path` (last good pipeline, like Frame). Out-of-clip t is not refused: the crop is defined at any t.

NOTES (2026-10-18): `pointState` in state.go now spells both a pan's to and its via points (the to branch refactored onto it, same output). The t-parse of handleFrame moved into a shared `frameTimeOf` (same 400 message). internal/tune/session.go is now 415 lines, past the ~400 guideline.

**What:** Depends on item 14.
**Regression guard.** Waypoints are reported as `via: [{target?: TargetState, cell?: CellState}]`
(the `to` spelling). Resolution is compose's camera.go (`planPan`/`panPoint`), whose pans drop the
storyboard index and sort by start: the accessor keys pans by camera index and maps through the crop
at t (`toOutput`). The handler test pins t=0; points outside the crop come back unclipped. Every
route/422 comment is updated (`grep -n '422\|frames as PNG' internal/tune/*.go`); this supersedes
state.go's `CameraState` comment (via points shown "but not edited"), rewritten with the new field.
**Goal:** `/api/state`'s camera entries for a via pan carry each waypoint as its wire point; a
`GET /api/path?camera=<i>&t=<ms>` answers the pan's waypoint centres in output pixels of the frame
at `t` (422 for a non-via move or an index out of range).
**Approach (assumed at the header base):** extend `CameraState` (`internal/tune/state.go`) beside
`ViaCount`; the path reuses compose's via-path resolution (`internal/compose/path.go`) through the
render pipeline — export the narrowest accessor rather than resolving targets again in tune.
**Files:** internal/tune/state.go, internal/tune/server.go, internal/tune/session.go,
internal/tune/server_test.go, internal/render/pipeline.go, internal/compose/path.go,
internal/tune/session_test.go, internal/compose/camera.go, internal/compose/compose.go,
internal/compose/camera_test.go, internal/tune/doc.go
**Read first:** internal/compose/camera.go — newCamera, planMoves, planPan, panPoint; internal/compose/compose.go — toOutput;
internal/tune/state.go — CameraState; internal/tune/server.go — newHandler;
internal/tune/session_test.go — TestSession_State_ReportsCameraMovesAndCaptions
**Tests:** a handler test for `/api/path` on `cameraCaptionsStoryboard`'s via pan (points inside
the frame, one per waypoint) and its 422s; state keys asserted; `wantCamera`'s Index 1 entry gains
its two waypoints; a compose test that a zoom before the via pan does not shift the index.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Commit:** `feat(tune): report a via pan's waypoints and path`

## 16. Page: edit a via pan and draw its path — ✅ DONE (2026-10-18)

NOTES (2026-10-18): the path overlay lives in a new script, internal/tune/page/path.js (loaded after picker.js, built on window.tune alone), joined to index.html and splitScripts; it redraws on frame load, resize, every new state and after any click or change on the page body, since window.tune announces no selection change and app.js code stays untouched.

NOTES (2026-10-18): the overlay is shown and hidden by a `drawn` class, not `hidden`, because an `<svg>` element has no `hidden` property; its viewBox is the `<img>`'s natural size, so /api/path's output-frame pixels need no scaling.

NOTES (2026-10-18): picker.js adds `pickProposal()` and `reportPick(message, isFailure)` to window.tunePage; a waypoint's Set from pick with no proposal standing sends nothing and reports "Pick a target first…" in the pick status. A target waypoint shows read-only above its (empty) cell inputs; waypoint buttons carry titles naming their waypoint ("Set waypoint 0 from the pick", "Insert a copy of waypoint 0 after it", "Remove waypoint 0").

NOTES (2026-10-18): pendingPageFields and its use in TestPage_EntryFields_AreEditFields are deleted (the set would have been empty).

NOTES (2026-10-18): consequential edit — internal/tune/doc.go: made necessary by the new page/path.js (the package doc's file map).

NOTES (2026-10-18): consequential edit — README.md: made necessary by the inspector now editing a pan's via path ("A pan's `via` … change in the YAML" became false); the same sentence's caption half, already stale since item 13, is folded into the rewrite.

**What:** Depends on items 6, 13 and 15.
**Regression guard.** Also depends on item 13; removes item 14's fields from `pendingPageFields` and deletes the set, which must then be empty.
picker.js exposes the proposal on `window.tunePage` (e.g. `page.pickProposal()`), read by entries.js
at click time; app.js code untouched. This supersedes the header comments app.js:24-26 and
picker.js:3 (`grep -n 'tunePage\|window.tune alone' internal/tune/page/*.js`), both rewritten. The
polyline is a static `<svg id="via-path"><polyline id="via-line">` in index.html under `#stage`.
"Insert after" waypoint i inserts a copy of waypoint i.
**Goal:** for a selected via pan, the inspector lists each waypoint with "Set from pick" (sends
`camera.via` with the standing proposal's target), cell x/y inputs, Insert after / Remove buttons,
and a to/via select (`camera.path`); a `to` pan shows the same select; an SVG polyline over the
frame preview draws the path from `/api/path` while the via pan is selected and the frame time
matches.
**Approach (assumed at the header base):** inspector rows in `aimRows` (entries.js); the pick
proposal is read through picker.js the way `setAsPanTarget` reads it; the overlay lives in
`#stage` and is positioned like `drawBox`. A new script, if any, joins `index.html`,
`splitScripts` and the page tests.
**Files:** internal/tune/page/entries.js, internal/tune/page/picker.js,
internal/tune/page/index.html, internal/tune/page/style.css, internal/tune/page_test.go,
internal/tune/pagejs_test.go, internal/tune/page/app.js (header comment only)
**Read first:** internal/tune/page/entries.js — aimRows; internal/tune/page/picker.js — setAsPanTarget, drawBox;
internal/tune/page/app.js — window.tune, window.tunePage; internal/tune/page_test.go — TestPage_SplitScripts_StayBelowTheFileSizeGuide;
internal/tune/page/index.html — #stage; internal/tune/page/controls.js — targetInputs
**Tests:** `TestPageJS_ViaWaypoint_SetFromPick`; `TestPageJS_ViaPath_IsDrawnForTheSelectedPan`;
`TestPageJS_ViaWaypoint_InsertAndRemove`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'TestPage' -v`
**Closes:** tune-edit-pan-via
**Commit:** `feat(tune): edit a pan's via waypoints and draw its path`

## 17. Enforce the 100-column wrap for non-test Go files — ✅ DONE (2026-10-18)

NOTES (2026-10-18): `make lint` and CI's golangci-lint step now fail on existing `lll` findings (168 in cmd/release alone) until items 18–23 land; closeout must not fast-forward main while 17 is done and 23 is not.
NOTES (2026-10-18): Makefile's header comment was rewrapped to stay within 100 columns after naming lll; ci.yml carries no linter-set wording, so it is unchanged.

**What:**
**Regression guard.** Closeout never fast-forwards main while item 17 is done and item 23 is not
(main would fail CI's golangci-lint step and the release gate). Every comment naming the linter set
is updated (`grep -n 'standard' Makefile .golangci.yml .github/workflows/ci.yml`). keys passes even
without lll, so Acceptance also checks a known finding and the config and Note text.
**Goal:** `.golangci.yml` enables `lll` with `line-length: 100` and `tab-width: 4`, excludes
`_test\.go` from it by an exclusion rule, and says why in a comment; Note `coding-conventions`'
Formatting bullet states the enforced wrap and the test exemption; the stray blank line before
`want` in `TestSession_PanTargetNotOnScreen_IsOnTheMove` (`internal/tune/session_test.go`) is gone.
**Approach (assumed at the header base):** `linters.enable: [lll]`, `linters.settings.lll`,
`linters.exclusions.rules: [{path: _test\.go, linters: [lll]}]`; edit
`.capcom/knowledge/coding-conventions.md`, then `capcom check`. Existing violations are fixed by
items 18–23; `make lint` fails on `lll` alone until item 23 lands.
**Files:** .golangci.yml, .capcom/knowledge/coding-conventions.md, internal/tune/session_test.go,
Makefile
**Read first:** .golangci.yml — linters.settings, formatters; Makefile — GOLANGCI_LINT_VERSION, lint;
.github/workflows/ci.yml — golangci-lint step; .capcom/knowledge/coding-conventions.md — Formatting bullet;
internal/tune/session_test.go — TestSession_PanTargetNotOnScreen_IsOnTheMove; golangci-lint v2.13.2 pkg/config/loader.go — handleEnableOnlyOption
**Tests:** none beyond the commands.
**Acceptance:**
- `capcom check`
- `LLL ./internal/keys/...` reports no issue (a package already within 100 columns)
- `LLL ./cmd/release/...` reports cmd/release/publish.go:93 and no `_test.go` path
- `grep -n 'line-length: 100' .golangci.yml` and `grep -n 'tab-width: 4' .golangci.yml` match
- `capcom know show coding-conventions | grep -n 100` matches
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run TestSession_PanTargetNotOnScreen`
**Commit:** `chore(lint): enforce the 100-column wrap outside tests`

## 18. Wrap cmd/release to 100 columns

**What:** Depends on item 17. Rewrap every `lll` finding in `cmd/release` (code, strings split with
`+`, comments): no behaviour change, error and help texts byte-identical.
**Regression guard.** `LLL` runs lll alone (`--enable-only` clears gofmt and every other
linter), so a rewrapped doc-map continuation or a moved `//nolint` passes it yet fails `make lint`
and CI: Acceptance also runs the full config on the package(s).
**Files:** cmd/release/*.go (non-test files only)
**Read first:** cmd/release/doc.go — package doc map; cmd/release/publish.go — newPublishCommand Long;
cmd/release/status.go — newStatusCommand Long; cmd/release/smoke.go — newSmokeCommand Long; cmd/release/prepare.go — newPrepareCommand;
cmd/release/dist.go — newDistCommand; internal/testkit/docmap.go — CheckDocMap, names
**Tests:** existing.
**Acceptance:**
- `LLL ./cmd/release/...` reports no issue
- `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run ./cmd/release/...` reports no issue
- `GOMEMLIMIT=2GiB go test ./cmd/release/`
**Commit:** `style(release): wrap at 100 columns`

## 19. Wrap cmd/photon and internal/{caption,check,clip,compose,config,engine} to 100 columns

**What:** Depends on item 17. As item 18, for these packages. Doc-map lines in `doc.go` keep the
shape the doc-map structural test parses.
**Regression guard.** `LLL` runs lll alone (`--enable-only` clears gofmt and every other
linter), so a rewrapped doc-map continuation or a moved `//nolint` passes it yet fails `make lint`
and CI: Acceptance also runs the full config on the package(s).
`//nolint:errcheck` stays directly above the `fmt.Fprintf` in cmd/photon/main.go (an own-line
nolint covers only the next line's node): shorten its reason or move the explanation above it.
**Files:** non-test .go files in cmd/photon, internal/caption, internal/check, internal/clip,
internal/compose, internal/config, internal/engine
**Read first:** cmd/photon/main.go — execute; cmd/photon/root.go — newRootCommand, versionTemplate; cmd/photon/tune.go — newTuneCommand;
cmd/photon/readme_test.go — TestREADMEDocumentsEveryCommand; internal/caption/caption.go — New;
internal/check/check.go — judgement.expects; internal/testkit/docmap.go — names
**Tests:** existing.
**Acceptance:**
- `LLL ./cmd/photon/... ./internal/caption/... ./internal/check/... ./internal/clip/... ./internal/compose/... ./internal/config/... ./internal/engine/...` reports no issue
- `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run ./cmd/photon/... ./internal/caption/... ./internal/check/... ./internal/clip/... ./internal/compose/... ./internal/config/... ./internal/engine/...` reports no issue
- `GOMEMLIMIT=2GiB go test` on each of those packages, one at a time
**Commit:** `style: wrap cmd/photon and the caption-to-engine packages at 100 columns`

## 20. Wrap internal/{raster,render,rig,schedule,service} to 100 columns

**What:** Depends on item 17. As item 19, for these packages.
**Regression guard.** Acceptance also runs `GOOS=windows LLL ./internal/service/...` (procgroup_windows.go is Windows-only).
`LLL` runs lll alone (`--enable-only` clears gofmt and every other linter), so Acceptance also runs
the full config on the packages (render/service doc maps, encode.go's `//nolint:gosec` lines).
**Files:** non-test .go files in internal/raster, internal/render, internal/rig,
internal/schedule, internal/service
**Read first:** internal/service/procgroup_windows.go — joinGroup; internal/service/service.go — launch; internal/service/doc.go — doc map;
internal/render/encode.go — plan.encode (nolint:gosec); internal/render/pipeline.go — Build; internal/schedule/schedule.go — Section;
internal/raster/raster.go — palette
**Tests:** existing.
**Acceptance:**
- `LLL ./internal/raster/... ./internal/render/... ./internal/rig/... ./internal/schedule/... ./internal/service/...` reports no issue
- `GOOS=windows LLL ./internal/service/...` reports no issue
- `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run ./internal/raster/... ./internal/render/... ./internal/rig/... ./internal/schedule/... ./internal/service/...` reports no issue
- `GOMEMLIMIT=2GiB go test` on each of those packages, one at a time
**Commit:** `style: wrap the raster-to-service packages at 100 columns`

## 21. Wrap internal/storyboard to 100 columns

**What:** Depends on items 8, 10, 11, 14 and 17. As item 19, for `internal/storyboard`.
**Regression guard.** `LLL` runs lll alone (`--enable-only` clears gofmt and every other
linter), so a rewrapped doc-map continuation or a moved `//nolint` passes it yet fails `make lint`
and CI: Acceptance also runs the full config on the package(s).
**Files:** non-test .go files in internal/storyboard
**Read first:** internal/storyboard/doc.go — doc map; internal/storyboard/edit.go — decodeMoment (raw-string format), apply;
internal/storyboard/edit_beat.go — decodeTarget; internal/storyboard/fields.go — cameraFields, captionFields;
internal/storyboard/camera.go — validateForm; internal/testkit/docmap.go — names
**Tests:** existing.
**Acceptance:**
- `LLL ./internal/storyboard/...` reports no issue
- `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run ./internal/storyboard/...` reports no issue
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
**Commit:** `style(storyboard): wrap at 100 columns`

## 22. Wrap internal/{take,target,terminal,testkit,yamledit} to 100 columns

**What:** Depends on items 7 and 17. As item 19, for these packages.
**Files:** non-test .go files in internal/take, internal/target, internal/terminal,
internal/testkit, internal/yamledit
**Read first:** internal/testkit/docmap.go — CheckDocMap, names; internal/terminal/doc.go — file map (terminal.go, lifecycle.go lines);
internal/terminal/probe.go — launchProbe.String (wording pinned by probe_test.go TestLaunchProbe_String); internal/terminal/terminal.go — Options.validate;
internal/target/anchor.go — anchor.row; internal/take/file.go — readHeader; internal/yamledit/document.go — child
**Tests:** existing.
**Acceptance:**
- `LLL ./internal/take/... ./internal/target/... ./internal/terminal/... ./internal/testkit/... ./internal/yamledit/...` reports no issue
- `GOMEMLIMIT=2GiB go test` on each of those packages, one at a time
**Commit:** `style: wrap the take-to-yamledit packages at 100 columns`

## 23. Wrap internal/tune and confirm the module is clean

**What:** Depends on items 12, 15, 16 and 17–22. As item 19, for `internal/tune`; then the whole
module passes `lll`.
**Regression guard.** Acceptance also runs `GOOS=windows LLL ./...` and `GOOS=darwin LLL ./...`.
**Files:** non-test .go files in internal/tune
**Read first:** internal/tune/doc.go — package comment, file map (page/lanes.js line); internal/tune/pick.go — proposer.propose;
internal/tune/state.go — movedState; internal/tune/session.go — Session (buildPipeline field); internal/tune/server.go — server.guard;
.golangci.yml — lll settings; internal/testkit/docmap.go — CheckDocMap
**Tests:** existing.
**Acceptance:**
- `LLL ./...` reports no issue
- `GOOS=windows LLL ./...` reports no issue
- `GOOS=darwin LLL ./...` reports no issue
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Closes:** wrap-at-100-columns
**Commit:** `style(tune): wrap at 100 columns`
