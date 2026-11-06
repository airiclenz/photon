# Browser capture plan (browser capture, part B)

**Goal:** Close every open `browser-capture` issue: fix the four gaps part A left (empty area selector, selector-target edits, page cell scale, single-rectangle patches), add the headless Chromium capture source, record and check a web app end to end, render and tune a browser take, and film `photon tune`'s own page as an example.
**Date:** 2026-11-06
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 194a346

**Sources:**
- `.capcom/issues/browser-capture.md` and its children under `.capcom/issues/browser-capture/`
- Notes `grid-surfaces`, `package-layout`, `extension-model`, `glossary`, `coding-conventions`
- `docs/plans/archived/2026-11-06 - 03 - grid-layers-plan.md` (part A, its item NOTES)

**Ratified design calls (user, 2026-11-06):**
- **Scope:** one plan for all nine open `browser-capture` issues.
- **Selector position edits:** `position.target` accepts `{"selector", "nth", "area"}` as well as `{"text", ...}`; exactly one of text or selector.
- **Frame capture:** CDP `Page.startScreencast` in PNG; a pushed frame is a snapshot, its text layer and selector boxes read at that frame.
- **End of take on a url launch:** `end.keys` beside `launch.url` is a config problem; the take settles (no new frame for `end.idle`, up to `end.settle_timeout`), then photon closes the browser.
- **GIF palette:** a url-launch clip encodes as a terminal clip does (no dither).
- **Example GIF:** `docs/media/tune-page.gif`, shipped there by the example storyboard, embedded in the README.
- **Take v2 wire shape:** amended in place for multi-rectangle patches; no release wrote a v2 take (v0.3.0 writes v1).

**Closes:** browser-launch-config; browser-capture

**Standing requirements:**
- skills: coding-standards
- Pi 5 host: single packages with `GOMEMLIMIT=2GiB`; never `./...` with `-race`; one test command at a time.
- Every new non-test `.go` file is named in its package's `doc.go` (testkit `CheckDocMap`).
- An item that changes a package's imports or adds a package updates that row of Note `package-layout`.
- Chromium tests carry `//go:build browser`, skip without a browser unless `PHOTON_TEST_BROWSER_REQUIRED` is set (as `internal/tune/browser_test.go` does); Chromium is `/usr/bin/chromium` on this host.
- No AI attribution; no forbidden names (AGENTS.local.md).

**Out of scope:** browser capture on Windows (`refuseWithoutPTY` stays); dithering; any version bump.

**Regression check (2026-11-06, 194a346):**
- 1: guard folded
- 2: guard folded
- 3: guard folded
- 5: guard folded
- 6: guard folded
- 7: guard folded
- 8: guard folded
- 9: guard folded (writer's decision plus the reviewer's Settle guard)
- 10: guard folded
- 11: guard folded
- 13: guard folded (writer's decision plus the README guard); supersedes CHANGELOG.md [Unreleased] "refuse a url launch until browser capture lands"
- 14: guard folded (writer's decision plus the reviewer's viewport-only and gridSize guards)
- 15: guard folded (writer's decision plus the reviewer's compose, draft and oracle guards)
- 16: guard folded (writer's decision plus the reviewer's zero-value guard)
- 17: guard folded (writer's decision plus the reviewer's pick-response, seen, nth and inspector guards)
- 18: guard folded (writer's decision plus the reviewer's test-file, config and ship guards)
- 19: guard folded

## 1. Refuse an explicit empty area selector — ✅ DONE (2026-11-06)

NOTES (2026-11-06): internal/config/config_test.go is listed in Files but needed no change: no test there builds a `config.Area{Selector: ...}`; the reject and absent-selector cases live in area_test.go as the item's Tests line says (the absent case asserts the urlBase `thread` area loads with no selector and both edges, inside `TestLoadSelectorArea`).

**What:**
**Goal:** `areas.<name>.selector: ''` in `photon.yaml` is a config problem naming the area; an area without the key is still a whole-screen or edge area.
**Approach (assumed at the header base):** `config.Area.Selector` becomes `*string` (nil = key absent). `Area.validate` takes the selector branch when non-nil; `validateSelector` reports an empty one through `storyboard.CheckSelector`. Consumers to update: `clip.(*Clip).Selectors` (internal/clip/clip.go), `target` area construction (internal/target/anchor.go), and every test building a `config.Area{Selector: ...}` (internal/target/resolver_test.go, internal/config tests).
**Regression guard.** internal/config/area_test.go compares `Selector` as a string (`TestLoadSelectorArea`) and fails to build after the `*string` change: deref it there, and put the `selector: ''` reject case beside `TestLoadRejectsAreasOnAURLLaunch`. Find every site with `grep -rn '\.Selector\b\|config.Area{' internal cmd`.
**Files:** internal/config/area.go, internal/config/config_test.go, internal/clip/clip.go, internal/target/anchor.go, internal/target/resolver_test.go, internal/config/area_test.go
**Read first:** internal/config/area.go — Area, Area.validate, Area.validateSelector; internal/config/area_test.go — TestLoadSelectorArea, TestLoadRejectsAreasOnAURLLaunch; internal/target/anchor.go — compileArea; internal/clip/clip.go — Clip.Selectors; internal/target/resolver_test.go — selectorAreas
**Closes:** config-empty-area-selector
**Tests:** config cases (area_test.go, beside `TestLoadRejectsAreasOnAURLLaunch`): `selector: ''` refused naming `areas.<name>: selector`; selector absent with edges loads; a set selector still resolves (target tests unchanged in behaviour).
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/config/`, then `./internal/target/`, then `./internal/clip/`
**Commit:** `fix(config): refuse an explicit empty area selector`

## 2. Refuse end keys on a url launch — ✅ DONE (2026-11-06)

**What:**
**Goal:** a `photon.yaml` with `launch.url` and `end.keys` is refused with a problem under `end.keys`; a url launch gets no default end keys; a command launch keeps `DefaultEndKeys`.
**Approach (assumed at the header base):** in internal/config/config.go the defaults step that sets `End.Keys = DefaultEndKeys` runs only for a command launch; `End.validate` (or `Config` validation, which knows `IsBrowser`) reports keys beside a url. README's `end.keys` row says it is refused with `launch.url`, and that a url take ends when the page settles.
**Regression guard.** every doc comment and README line saying a take's end sends keys or defaults `end.keys` to ctrl+c gains "on a command launch" — the `End` and `DefaultEndKeys` docs in internal/config/config.go and README's `end.keys` and `end.idle` rows among them; find them with `grep -n 'DefaultEndKeys\|end\.keys\|keys are sent' internal/config/*.go README.md`.
**Files:** internal/config/config.go, internal/config/config_test.go, README.md
**Read first:** internal/config/config.go — Config.applyDefaults, Config.validate, End, DefaultEndKeys, Config.IsBrowser; internal/config/config_test.go — urlBase, TestLoadURLLaunch; README.md — end.keys and end.idle rows
**Tests:** url + keys refused; url without keys loads with `End.Keys` empty; command without keys gets `DefaultEndKeys`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/config/`, then `GOMEMLIMIT=2GiB go test -run README ./cmd/photon/`
**Commit:** `feat(config): a url launch ends by settling, not by keys`

## 3. Selector targets in camera position edits — ✅ DONE (2026-11-06)

NOTES (2026-11-06): consequential edit — internal/storyboard/edit.go: made necessary by the selector form of position.target (the Edit wire-spelling doc comment listed only the text form and said position.cell removes text, nth and area)

NOTES (2026-11-06): decodeTarget treats an empty text or selector string as absent, so {"text": "", "selector": "#a"} is a selector target; a malformed selector is left to the storyboard's own validation, which Apply runs on the edited result

**What:**
**Goal:** `position.cell` and `position.target` edits on a position naming a selector leave no selector key behind and the result validates; `position.target` with `{"selector": "...", "nth", "area"}` writes a selector target; a value with both or neither of text and selector is refused, never silently dropped.
**Approach (assumed at the header base):** in internal/storyboard/edit_camera.go add `keySelector` to `targetKeys`; `decodeTarget` requires exactly one of `Text`/`Selector`; `setPositionTarget` writes `selector:` quoted (and deletes `text`) or `text:` (and deletes `selector`), then nth, area, deletes `cell`. `KindTarget`'s doc in internal/storyboard/fields.go names the selector form.
**Regression guard.** yamledit refuses to delete a sequence item's only key, so `setPositionTarget` writes the chosen key (text or selector) first and deletes the other after it — never the `targetKeys` order loop as it stands, which deletes `text` before `selector` is inserted on a one-key `- text: ok` position.
**Files:** internal/storyboard/edit_camera.go, internal/storyboard/edit_camera_test.go, internal/storyboard/fields.go
**Read first:** internal/storyboard/edit_camera.go — setPositionTarget, writeTargetKey, decodeTarget, targetKeys; internal/storyboard/edit_camera_test.go — TestApply_PointEditOnAOneKeyPosition_WritesTheNewKeyFirst, positionEdit; internal/storyboard/validate.go — Target.validate; internal/tune/page/controls.js — targetOf
**Closes:** storyboard-selector-target-edits
**Tests:** edit table cases: cell over a selector position; text over a selector position; selector over a text position; selector over a cell; both keys refused; neither refused; each result re-parses and validates; "a selector replaces the only text" added to `TestApply_PointEditOnAOneKeyPosition_WritesTheNewKeyFirst`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
**Commit:** `fix(storyboard): position edits replace a selector target`

## 4. Page cell geometry includes frame.scale — ✅ DONE (2026-11-06)

NOTES (2026-11-06): no CHANGELOG entry — pageOutput's size and unit are unchanged, so nothing a user runs today (url launches are still refused) observes the new cell size.

**What:**
**Goal:** on a viewport frame a box maps to the pixel rectangle at the frame's scale: `pageLayout(frame).Cell` is device_scale × frame.scale on each side.
**Approach (assumed at the header base):** in internal/render/check.go `pageLayout` sets `Cell: {W: deviceScale*Scale, H: deviceScale*Scale}`; `Size` and `pageOutput`'s unit are unchanged (unit depends on Size and Scale only). Fix the doc comment that says the device scale lives only in the cell size.
**Files:** internal/render/check.go, internal/render/check_test.go
**Read first:** internal/render/check.go — pageLayout, pageOutput, captionOutput; internal/render/check_test.go — TestPageOutput_DeviceScale_SizesTheFrameAndCaptions; internal/compose/compose.go — Layout, Layout.boxRect, Output
**Closes:** page-layout-cell-scale
**Tests:** the `pageLayout` table expects the scaled cell; a scale-2 case asserts a box's corners land at `X*deviceScale*2`.
**Acceptance:** `GOMEMLIMIT=2GiB go test -run 'Page|Caption' ./internal/render/`
**Commit:** `fix(render): a page cell spans device scale times frame scale`

## 5. An image entry carries several patch rectangles — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the patch list's JSON key is `patches` (each patch keeps the short `rect`/`png` keys); `ErrMalformed`'s doc comment now also names a key frame that is not one patch at (0, 0). A non-key entry with an empty patch list decodes as no change rather than being refused (the encoder never writes one).

**What:**
**Goal:** a `take.ImageEntry` holds a key frame or a list of patches, each a rectangle and its PNG; a take written with several patches per snapshot loads back identically and decodes to the encoded frames.
**Approach (assumed at the header base):** in internal/take/take.go `ImageEntry` becomes `{Key bool; Patches []Patch}` with `Patch{Rect Box; PNG []byte}` (a key frame is one patch at (0,0) covering the frame); JSON keys stay short. Producer: `pixels.Encoder` (wraps its one rectangle as one patch for now). Consumers: `pixels.Decoder` (`decodeKey`, `applyPatch` loop over patches), take's own line-version check. No version bump: v2 never shipped.
**Regression guard.** `TestLoad_RefusesWhatItCannotRead`'s "image entry on version 1" fixture (internal/take/file_test.go) writes `img` with `rect`/`png`, which `decodeStrict` would refuse as unknown fields before `checkLineVersion`: rewrite it to the new wire shape (key plus one patch) so it still fails naming `"img"`. `decodeKey` refuses a key entry without exactly one patch at (0,0) covering the frame, wrapping `ErrMalformed` — never indexing `Patches[0]` unchecked.
**Files:** internal/take/take.go, internal/take/file_test.go, internal/pixels/pixels.go, internal/pixels/pixels_test.go
**Read first:** internal/take/take.go — ImageEntry; internal/pixels/pixels.go — Encoder.key, decodeKey, applyPatch; internal/take/file.go — checkLineVersion, decodeStrict; internal/take/file_test.go — TestLoad_RefusesWhatItCannotRead; internal/render/pipeline_test.go — saveImageTake
**Tests:** take round trip of an entry with two patches; decoder applies two patches; a patch outside the frame still wraps `ErrMalformed`; a key frame with no patch wraps `ErrMalformed` (`TestDecoder_RefusesAMalformedLayer`); the version-1 `img` fixture, rewritten to the new shape, is still refused naming `"img"`; existing pixels tests pass.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/take/`, then `GOMEMLIMIT=2GiB go test -short ./internal/pixels/`, then `GOMEMLIMIT=2GiB go test ./internal/render/`
**Commit:** `feat(take): an image entry holds several patch rectangles`

## 6. Encode far-apart changes as separate patches — ✅ DONE (2026-11-06)

NOTES (2026-11-06): `patchGap` is 32 pixels; each span's rectangle is tightened to the rows its own columns changed in (not the whole band's rows). The one-minute budget test is now two subtests, "typing" (4.8 MB) and "typing plus a turning spinner" (4.9 MB; 30.8 MB with a single bounding rectangle). The round-trip test's frame grew to 96×72 and paints one to three rectangles per step, failing if no entry carries more than one patch.
NOTES (2026-11-06): consequential edit — internal/pixels/pixels.go: the `Decoder` doc comment and doc.go's "costs one patch each" now say "one entry each", made necessary by an entry holding several patches.
NOTES (2026-11-06): no CHANGELOG entry: browser takes (the only image-layer takes) cannot be recorded yet, so nothing a user sees changes.

**What:**
**Goal:** two far-apart changes in one frame encode as a patch entry with one rectangle each, not a key frame; a one-minute 1280×800, 24 fps synthetic take with typing plus a turning spinner is ≤ 25 MB.
**Approach (assumed at the header base):** `Encoder.changed` returns rectangles: changed rows split into bands at gaps of ≥ `patchGap` unchanged rows, each band split into spans at gaps of ≥ `patchGap` unchanged columns, each span's bounding box one rectangle (`patchGap` a named constant). The key-frame trigger compares the rectangles' summed area with half the frame. Update the package comment in internal/pixels/doc.go, the "Frame storage" section of Note `grid-surfaces` and the pixels row of Note `package-layout` (every sentence naming "the changed rectangle").
**Regression guard.** the prose rule is every comment or Note sentence describing a patch as one rectangle, found by `grep -rn -i 'rectangle that changed\|changed rectangle' internal .capcom/knowledge` — the closed list above plus the `Encode` and `changed` doc comments in internal/pixels/pixels.go.
**Files:** internal/pixels/pixels.go, internal/pixels/pixels_test.go, internal/pixels/doc.go, .capcom/knowledge/grid-surfaces.md, .capcom/knowledge/package-layout.md
**Read first:** internal/pixels/pixels.go — Encoder.Encode, Encoder.changed, encodePNG, box; internal/pixels/pixels_test.go — TestEncoder_KeyFrameCadence, TestEncode_OneMinuteBrowserTakeFitsTheBudget, drawPageFrame; .capcom/knowledge/grid-surfaces.md — Frame storage
**Depends on:** item 5
**Closes:** pixels-multi-rect-patches; pixel-take-storage
**Tests:** a frame with two distant changes gives one entry with two patches; round trip at every index; the existing budget test plus a variant with a spinner far from the typing box, both ≤ 25 MB (skipped under `-short`).
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/pixels/`
**Commit:** `fix(pixels): one patch rectangle per far-apart change`

## 7. A Chromium launcher — ✅ DONE (2026-11-06)

NOTES (2026-11-06): `Options` carries URL, ExecPath, Width, Height, DeviceScale and Env only; FPS and Selectors are left to the items that first use them (screencast, selector boxes), since no code in this item reads them.
NOTES (2026-11-06): a set ExecPath (launch.browser) is the only browser tried: when it is missing, `findBrowser` fails with `ErrNoBrowser` rather than falling back to the discovery list, so a launch that names a browser never runs another.
NOTES (2026-11-06): the viewport and scale are set with chromedp's `EmulateViewport`/`EmulateScale`, which wrap `emulation.SetDeviceMetricsOverride`; NoSandbox is applied only as root on Linux (Chromium refuses its sandbox there). The Pi host's Chromium ran sandboxed as uid 1000; the ubuntu/macos runners are unverified until CI runs.
NOTES (2026-11-06): the package-layout row lists no photon imports ("—"): the launcher imports only chromedp so far; `take`, `pixels` and `keys` join the row as items 8-10 add those imports.
NOTES (2026-11-06): consequential edit — .capcom/knowledge/package-layout.md: made necessary by adding internal/browser (the grid-surfaces paragraph no longer calls it "planned").
NOTES (2026-11-06): the CI `browser` job is named "headless Chromium"; GitHub appends the matrix os, so the ubuntu check reads "headless Chromium (ubuntu-latest)".

**What:**
**Goal:** a new package `internal/browser` starts headless Chromium at a URL with a viewport and device scale, finds the browser from `launch.browser` or a fixed discovery list, and fails with `ErrNoBrowser` when none is found; CI runs its browser tests on Linux and macOS.
**Approach (assumed at the header base):** `browser.Options{URL, ExecPath string; Width, Height int; DeviceScale float64; FPS int; Env []string; Selectors []string}`; `findBrowser` tries ExecPath, then the names `internal/tune/browser_test.go`'s `browserNames` lists, via `exec.LookPath`. chromedp exec allocator (headless, `DisableGPU`; `NoSandbox` only where needed), `emulation.SetDeviceMetricsOverride` for viewport and scale, navigate and wait for load. State why chromedp is imported in doc.go. Add `./internal/browser` and a macos-latest leg to the `browser` job in .github/workflows/ci.yml. New package row in Note `package-layout` (imports `take`, `pixels`, `keys`).
**Regression guard.** `findBrowser` takes its name list as a parameter (production passes the fixed list); hermetic cases pass names that exist only in, or are absent from, a temp PATH dir (never `/usr/bin/google-chrome`, present on CI's ubuntu runner), the fake named `<name>.exe` on Windows. The list and tune's `browserNames` gain chromedp's darwin paths (`/Applications/Chromium.app/Contents/MacOS/Chromium`, `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`). The `browser` job drops "(ubuntu-latest)" from its name and vets and tests `./internal/browser`; every `browser`-tagged test is named `TestBrowser_*` so `-run Browser` runs it.
**Files:** internal/browser/doc.go, internal/browser/docmap_test.go, internal/browser/browser.go, internal/browser/find.go, internal/browser/find_test.go, internal/browser/browser_test.go, .github/workflows/ci.yml, .capcom/knowledge/package-layout.md, internal/tune/browser_test.go
**Read first:** internal/tune/browser_test.go — browserNames, findBrowser, newBrowser; .github/workflows/ci.yml — browser job; chromedp@v0.23.0/allocate.go — findExecPath, execLocations; internal/config/config.go — Launch.Browser; internal/testkit/docmap.go — CheckDocMap
**Tests:** hermetic `findBrowser` cases over a name list passed in (ExecPath set, PATH lookup in a temp dir, `<name>.exe` on Windows, none → `ErrNoBrowser`); a `browser`-tagged `TestBrowser_*` test that starts at an httptest page and reads `window.innerWidth` and `devicePixelRatio`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/browser/`, then `GOMEMLIMIT=2GiB go test -tags browser ./internal/browser/`, then `grep -n 'internal/browser' .github/workflows/ci.yml && grep -A12 '^  browser:' .github/workflows/ci.yml | grep -q macos-latest`
**Commit:** `feat(browser): start headless Chromium at a url and viewport`

## 8. Read a page's text layer and selector boxes — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the reader is the exported method `Browser.ReadText(selectors) ([]take.TextRun, map[string][]take.Box, error)` with a fixed 10s read timeout: item 9 is its first production caller, and an unexported method used only by `browser`-tagged tests would fail CI's untagged golangci-lint (`unused`).
NOTES (2026-11-06): "rects whose vertical extents overlap form one visual line" is read as overlapping by more than half the shorter extent (`sameLineShare`): glyph boxes of two tight lines (line-height below the font's height) overlap by a sliver and must stay two lines; a label beside a padded button still shares one top (tested hermetically and in Chromium).
NOTES (2026-11-06): the JS reports raw per-grapheme rects and the Go side does the dropping, trimming, line grouping and rounding, so the conversion cases run hermetically; a rendered whitespace grapheme (a newline or tab laid out as a gap) reads as one space, and a grapheme with a zero-width rect counts as not rendered.
NOTES (2026-11-06): "visible" text and selector boxes are those whose element is `visibility: visible`, that render a non-empty rect, and that touch the viewport (text or elements scrolled wholly out of the viewport have no run or box); a selector that matches nothing has no key in the map.
NOTES (2026-11-06): the `browser`-tagged Chromium test lives in a new file `internal/browser/textlayer_browser_test.go` (not in the item's Files): a file is tagged whole, and the hermetic conversion cases in `textlayer_test.go` must run untagged per the Acceptance's first command.
NOTES (2026-11-06): Note `package-layout`'s `internal/browser` row now lists the `take` import and the text-layer read, per the plan's standing requirement.

**What:**
**Goal:** the browser package reads the page's text layer as `take.TextRun`s and the box of every element each named selector picks, in CSS pixels; runs on one visual line share one top Y and a run's `Edges` map each byte to its CSS-px column.
**Approach (assumed at the header base):** an embedded script (`internal/browser/textlayer.js`) walks visible text nodes; one run per text-node line box (`Range.getClientRects`), per-grapheme rects for edges, rounded to ints; boxes for each selector by `querySelectorAll(...).getBoundingClientRect()`, visible ones in document order. Go side (`textlayer.go`) evaluates it and converts the JSON to runs (per-byte edges from per-grapheme columns, as `take.TextRun` requires) and `map[string][]take.Box`.
**Regression guard.** rects whose vertical extents overlap form one visual line, and every run on it takes the line's top and height (target's `linesOf` groups by exact `Box.Y`). A run's Text is the rendered text: graphemes with no client rect (collapsed whitespace) are dropped and runs trimmed, so `Edges` stays len(Text)+1. Each selector's query runs in its own try; a selector the page refuses (`button >` passes `CheckSelector`) is a Go error naming it.
**Files:** internal/browser/textlayer.js, internal/browser/textlayer.go, internal/browser/textlayer_test.go, internal/browser/doc.go
**Read first:** internal/take/text.go — TextRun, TextRun.Span; internal/target/resolver.go — linesOf, lineOf, selectorBoxes; internal/take/take.go — Snapshot.Runs, Snapshot.Selectors; internal/storyboard/validate.go — CheckSelector
**Depends on:** item 7
**Tests:** hermetic conversion cases (ASCII, multi-byte runes, an empty node, a pretty-printed node); a refused selector is an error naming it; a `browser`-tagged test on a fixture page: a two-word button resolves as one run, two inline spans on one line share Y, a selector's boxes match its elements, a label beside a padded button shares one top, a pretty-printed node's run holds only its rendered text.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/browser/`, then `GOMEMLIMIT=2GiB go test -tags browser ./internal/browser/`
**Commit:** `feat(browser): read the page's text runs and selector boxes`

## 9. Record snapshots from the screencast — ✅ DONE (2026-11-06)

NOTES (2026-11-06): `Browser.Close` now returns `(*take.Take, error)`: the take, plus the first recording error (a frame that does not decode or encode) joined with the browser's close error; a second call returns the same pair. `Options` gains `FPS` (must be positive, validated) and `Selectors`.
NOTES (2026-11-06): the browser now launches with `--force-device-scale-factor=<DeviceScale>` (constant `deviceScaleFlag`): with the emulated scale alone the screencast pushed 640x480 frames for a 640x480 viewport at scale 2; with the flag they are 1280x960 device pixels, which `Cell = device scale` assumes. The text-layer tests still pass (CSS px).
NOTES (2026-11-06): `Settle` counts its quiet spell from its own call at the earliest (`min(since newest frame, since the call)`), and returns false while a frame that arrived is not yet taken into the take: the screencast pushes a change's frame ~20-30 ms after the paint, so the terminal's rule (quiet measured from the last output alone) returned at once after a change on a page that had been still, and Close dropped the change's frame.
NOTES (2026-11-06): frames reach the recorder through a newest-frame slot rather than a queue: every frame is acked on arrival, and one arriving while the recorder is still busy with the previous replaces any frame waiting, so a host slower than the page records the newest picture, not a backlog. Close takes in a frame left in the slot before freezing; Freeze records only the frame held for its interval boundary.
NOTES (2026-11-06): a frame's text layer is read when the recorder takes it in, before its PNG is decoded; a Match records the live-read text layer with the held picture (or the last recorded one), so a take can show new text over the old picture for one frame until the change's frame arrives.
NOTES (2026-11-06): a frame held for its boundary is stamped at the boundary (last snapshot + 1/fps), also when Freeze or Close records it; a frame arriving a full interval after the last snapshot is recorded at once at its arrival.
NOTES (2026-11-06): the `browser`-tagged test lives in a new file `internal/browser/record_browser_test.go` (not in the item's Files): a file is tagged whole, and the hermetic cases in record_test.go must run untagged. The test drives the text changes itself (in-package `chromedp.Evaluate` on `b.tab`) after the first frame is recorded, since timer-driven changes raced the first frame under `-race`.
NOTES (2026-11-06): `Start` reads the text layer once with `options.Selectors` after the load and fails with `ReadText`'s error (a refused selector names itself), leaving no browser running; the newest-layer fallback in `readLayer` now covers only a read that fails mid-recording. Browser-tagged test `TestBrowser_Start_RefusedSelectorFailsNamingIt` in browser_test.go.
NOTES (2026-11-06): consequential edit — internal/browser/browser_test.go: made necessary by `Options.FPS` being required and `Close` returning the take (test sets `FPS: testFPS`, reads `Close`'s error from its second result).
NOTES (2026-11-06): consequential edit — internal/browser/textlayer_browser_test.go: made necessary by `Options.FPS` being required and `Close` returning the take.
NOTES (2026-11-06): consequential edit — .capcom/knowledge/package-layout.md: made necessary by internal/browser importing `pixels` (standing requirement); the row also names the screencast recording.
NOTES (2026-11-06): record.go is 390 lines of code plus doc comments, 430 in all, just over the ~400-line guide; the plan's Files list has no third file to split the screencast pumps into.
NOTES (2026-11-06): pre-existing, not this item's: `golangci-lint run --build-tags browser ./internal/browser/` flags an unchecked `fmt.Fprint` in browser_test.go:45 (item 7's test); CI lints untagged, so it does not fail CI.

**What:**
**Goal:** a started browser records a take: each screencast frame that changes the picture or the text layer is a snapshot with an image entry (via `pixels.Encoder`) or none, its runs and selector boxes; `Current`, `Match`, `Done`, `Event`, `Settle`, `Freeze` and `Close` (returning the `*take.Take`, Cols/Rows = viewport, Cell = device scale) behave as the terminal's do.
**Approach (assumed at the header base):** `Page.startScreencast` PNG, max size viewport × device scale, ack each frame; decode to RGBA, encode, read text layer (item 8), append under one lock; the clock starts at the first frame after load (the terminal's first-paint rebase). `Settle(quiet)` waits for no new frame for `quiet`; `Done` closes when the browser or target dies. Copy the terminal's sampler shapes, never import `terminal`.
**Regression guard.** the browser records at most one snapshot per 1/frame.fps of the take's clock — a screencast frame arriving sooner than that after the last recorded snapshot is held as pending (the newest pending frame replaces an older one) and recorded at the next 1/fps boundary, so the last picture before the page goes still is never lost and the take's rate is bounded by frame.fps, which item 6's 24 fps budget assumes; Options.FPS is the reader of this. `Settle` is `Settle(ctx context.Context, quiet, timeout time.Duration) bool`, as the terminal's: true after no new frame for quiet, false on timeout or a cancelled ctx.
**Files:** internal/browser/browser.go, internal/browser/record.go, internal/browser/record_test.go, internal/browser/doc.go
**Read first:** internal/terminal/sample.go — sample, addSnapshot, sameScreen; internal/terminal/lifecycle.go — Freeze, Settle; internal/pixels/pixels.go — Encoder.Encode; internal/take/take.go — Take, CellSize
**Depends on:** item 8; item 6
**Tests:** hermetic snapshot bookkeeping (dedupe, rebase, Match records) behind a frame feed, a frame sooner than 1/fps held pending and recorded at the next boundary, a `Settle` timeout; a `browser`-tagged test where a page changes text twice: the take has key frame + patch, runs per snapshot, loads back with `take.Load`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/browser/`, then `GOMEMLIMIT=2GiB go test -tags browser ./internal/browser/`
**Commit:** `feat(browser): record a take from the page's screencast`

## 10. Drive the page: typing, keys and pointer — ✅ DONE (2026-11-06)

NOTES (2026-11-06): keymap.go maps each keys name to a DOM key in chromedp's generated `kb` table (key, code, virtual key code) plus CDP modifiers, rather than repeating key/code/keyCode literals; Type and Key both dispatch kb's keyDown/char/keyUp per rune, dropping the char when ctrl or alt is held.
NOTES (2026-11-06): tests split across three files where the plan named one: input_test.go (external package) holds only the `engine.Source` compile assertion, keymap_test.go (internal, hermetic) covers keyEvents over every `keys.Names()` entry, typeEvents and mouseEvent, and input_browser_test.go (`//go:build browser`) logs keydown/input/mousedown/mousemove/mouseup into the DOM and reads them back through Match; an external test package cannot reach the unexported table, and the hermetic test cannot carry the browser tag.
NOTES (2026-11-06): .capcom/knowledge/package-layout.md browser row gains input dispatch and the `keys` import, per the plan's standing requirement on import changes.
NOTES (2026-11-06): Type and Key dispatch several CDP key events per call (Pointer one mouse event), each call capped by a 10 s inputTimeout; no mutex serialises concurrent callers (the doc comment says a concurrent call's events may interleave), so browser.go was left untouched.

**What:**
**Goal:** `*browser.Browser` implements `engine.Source`: `Type` sends each rune as a key event, `Key` sends every name `keys.Lookup` accepts that a page can receive (others are an error naming the key), `Pointer` dispatches press, move and release at the cell's CSS-px point with button and modifiers.
**Approach (assumed at the header base):** `Input.dispatchKeyEvent` keyDown/char/keyUp per rune; a key table in `internal/browser/keymap.go` from `keys` names to DOM key/code/keyCode; `Input.dispatchMouseEvent` with X,Y as given (1 cell = 1 CSS px).
**Regression guard.** input_test.go (an external test package) holds `var _ engine.Source = (*browser.Browser)(nil)`, so a wrong `Pointer` signature (it takes `keys.Pointer`, aliased as `engine.PointerEvent`) fails this item. `keys.Mods` are SGR bits (Shift=4, Alt=8, Ctrl=16), not CDP's modifier mask: convert them.
**Files:** internal/browser/input.go, internal/browser/keymap.go, internal/browser/input_test.go, internal/browser/doc.go
**Read first:** internal/engine/engine.go — Source, PointerEvent; internal/keys/keys.go — Lookup, Pointer, Mods; internal/tune/browser_test.go — scrub; internal/terminal/terminal.go — Key, Pointer
**Depends on:** item 9
**Tests:** the `engine.Source` compile assertion; hermetic keymap coverage over `keys` names; a `browser`-tagged test on a page logging `keydown`/`input`/`mousedown`/`mouseup` into the DOM, read back from the text layer.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/browser/`, then `GOMEMLIMIT=2GiB go test -tags browser ./internal/browser/`
**Commit:** `feat(browser): type, press keys and click on the page`

## 11. A fixture web app for tests — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the nav holds the items alpha/beta/gamma (a click shows "selected: <item>"), mirroring the fixture TUI's list, so later items have a selector target that also answers a click; typing shows "you typed: <text>", the Run button shows "working" plus the spinner for 1 s, then "clicked: Run".
NOTES (2026-11-06): buildFixture now takes the package, binary name and temp-dir pattern so BuildFixtureTUI and BuildFixtureWeb share it (each with its own sync.OnceValues); the page is a Go string constant in main.go rather than a go:embed file, keeping the plan's file list.

**What:**
**Goal:** `internal/testkit/fixtureweb` is a small Go program serving a fixture page on `$PHOTON_PORT` (an input, a button with text, a `nav` for a selector, a spinner, a status line that echoes input and clicks), and `testkit.BuildFixtureWeb` builds it as `BuildFixtureTUI` does.
**Approach (assumed at the header base):** standard library only; prints `ready` on stdout once listening; page embedded. Extend `internal/testkit/build.go` and its doc map.
**Regression guard.** the spinner is bounded: it shows only while a click's work is pending (about 1 s after the button press) and is then removed, so the page goes still and a take over it settles instead of waiting out `end.settle_timeout` (15 s). fixtureweb enlists its doc map with its own docmap_test.go; Note `package-layout`'s testkit row reads "fixture TUI and fixture web app builds".
**Files:** internal/testkit/fixtureweb/main.go, internal/testkit/fixtureweb/doc.go, internal/testkit/build.go, internal/testkit/doc.go, internal/testkit/build_test.go, internal/testkit/fixtureweb/docmap_test.go, .capcom/knowledge/package-layout.md
**Read first:** internal/testkit/build.go — BuildFixtureTUI, buildFixture, fixtureBuild, executableName; internal/testkit/build_test.go — TestBuildFixtureTUI_PathOutlivesTheTestThatBuiltIt; internal/testkit/fixturetui/docmap_test.go — TestDocMap; internal/service/testdata/svc/main.go — serve; internal/testkit/fixturetui/doc.go — package comment
**Tests:** a testkit test builds it, runs it on a free port and GETs the page.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/testkit/...`
**Commit:** `test(testkit): a fixture web app`

## 12. A rig-local capture seam — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the `capture` interface is `engine.Source` plus `Painted`, `Settle`, `Freeze`, `exitError` (how the program ended, wrapped by the rig as "... before its first paint") and `finish(ctx)` (end of take: the terminal adapter folds `Quit` with its quit keys and `end.kill_after`, then `Close`); `quitKeyGap` moved from record.go to capture.go beside its only user, the terminal adapter.

**What:**
**Goal:** `rig`'s record path drives the capture through a rig-local interface with no change to a terminal take's behaviour.
**Approach (assumed at the header base):** in internal/rig/record.go, `performBeats` and `endTake` take a small interface (`engine.Source` plus painted/exit, settle, freeze and end-of-take), with the terminal adapter wrapping `Painted`, `ExitCode`, `Settle`, `Freeze`, `Quit` + `Close`. One interface with two adapters (terminal now, browser in item 13), per Note `coding-conventions`.
**Files:** internal/rig/record.go, internal/rig/capture.go, internal/rig/doc.go
**Read first:** internal/rig/record.go — recorder.record, recorder.performBeats, recorder.endTake; internal/engine/engine.go — Source; internal/terminal/lifecycle.go — Terminal.Painted, Settle; internal/terminal/terminal_windows.go — Terminal stub methods; internal/rig/record_test.go — TestRecord_PassingTake
**Tests:** existing rig and `cmd/photon` record tests pass unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/rig/`, then `GOMEMLIMIT=2GiB go test -run Record ./cmd/photon/`
**Commit:** `refactor(rig): drive a take's capture through one seam`

## 13. Record and set up a url launch — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the capture seam's `finish` now returns `(*take.Take, error)` (the terminal adapter returns a nil error) so a browser's recording or close error reaches `Record`, which joins it after the beats' error: the take is still written but not checked, as for a failed beat.
NOTES (2026-11-06): the browser adapter's `Painted` is closed from the start: `browser.Start` returns once the page has loaded and the browser package has no first-frame signal; a wait answered before the screencast's first frame records nothing, which that package documents, and the first frame then carries the screen.
NOTES (2026-11-06): `launchURL` (the `${NAME}` expansion over `launchEnv`'s list, a later entry winning, an unset name an error naming `launch.url`) sits in record.go beside `openPage`, its tests in record_test.go, so env.go stays untouched; the browser gets the whole launch env as its extra env.
NOTES (2026-11-06): the url-launch refusal tests are replaced by `TestSetup_RunsOnAURLLaunch`, `TestLaunchURL` and `TestLaunchURL_UnsetReferenceFails`; the `browser`-tagged test (`browser && !windows`, since its service runs under sh) is `TestBrowser_Record_URLLaunchPassesItsCheck`, and skips on `browser.ErrNoBrowser` unless `PHOTON_TEST_BROWSER_REQUIRED` is set.
NOTES (2026-11-06): README beyond the two refusal sentences: "How a take works" steps 3 and 5 name the url launch, the external-programs list adds Chromium or Chrome, and the FAQ "Does it need a browser" answer names the url launch; `photon record --help` now says recording runs on Linux and macOS rather than "needs a unix pty".

**What:**
**Goal:** `photon setup` and `photon record` run a url-launch clip: services, profiles, `reset`, `run:` expects and `on_success` work as for a terminal take; the take types, clicks a text target and a selector target, waits, and `photon check` passes it.
**Approach (assumed at the header base):** drop `errBrowserCapture` and its tests; expand `${VAR}` in `launch.url` from `launchEnv`'s list; start `browser.Start` with the storyboard viewport, device scale, fps and `clip.Selectors()`; the browser adapter of item 12 settles then closes. `cmd/photon/record.go` help no longer says pty only. Update rig's row in Note `package-layout` and rig/doc.go.
**Regression guard.** add ./internal/rig to the `browser` job in .github/workflows/ci.yml (the `-tags browser -run Browser` test must run in CI on both legs item 7 creates); add .github/workflows/ci.yml to the item's Files line. Every README sentence saying `photon setup`/`photon record` refuse a url launch or browser capture is not available yet (the `launch.url` row, the url-lint paragraph) is rewritten; find them with `grep -n "not available yet\|refuse a url launch" README.md internal/rig/*.go`. Supersedes CHANGELOG.md [Unreleased]'s clause "`photon setup` and `photon record` refuse a url launch until browser capture lands": the item's changelog entry replaces it.
**Files:** internal/rig/record.go, internal/rig/setup.go, internal/rig/capture.go, internal/rig/record_test.go, internal/rig/browser_test.go, cmd/photon/record.go, internal/rig/doc.go, .capcom/knowledge/package-layout.md, .github/workflows/ci.yml, README.md
**Read first:** internal/rig/record.go — Record, errBrowserCapture, keySequences; internal/rig/setup.go — Setup; internal/rig/env.go — launchEnv; internal/service/expand.go — ExpandEnv; internal/rig/record_test.go — urlLaunchConfig, TestRecord_RefusesAURLLaunch
**Depends on:** items 2, 10, 11, 12
**Closes:** browser-capture-source
**Tests:** url expansion unit test; a `browser`-tagged rig test over fixtureweb as a service with `reset`, a `run:` expect and `on_success`: record, then check passes.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/rig/`, then `GOMEMLIMIT=2GiB go test -tags browser -run Browser ./internal/rig/`
**Commit:** `feat(rig): record a web app through the browser source`

## 14. Check a browser take against its frame — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the tune test (`TestSession_Build_PageTakeOfAnotherViewport_BuildsAtTheTakesSize`) cannot build a page preview yet, since `raster.New` refuses a viewport frame until item 15; it drives `Session.build` through the `buildPipeline` seam and asserts the clip handed to render is at the take's viewport and device scale and that `render.CheckFrame` accepts it, with the size problem standing.
NOTES (2026-11-06): a terminal take (zero Cell) under a frame with a viewport is refused too, with the viewport wording and the take named as "W×H terminal cells"; atTakeSize keeps writing cols×rows for such a take, so tune does not open on it.
NOTES (2026-11-06): consequential edit — README.md: made necessary by CheckFrame's new viewport and device-scale refusals (the `frame.viewport` and `frame.device_scale` rows now say `render` refuses a take of another one).

**What:**
**Goal:** `render.CheckFrame` accepts a browser take whose grid is the viewport and whose cell is the frame's device scale, and refuses, with a message naming the keys, a viewport or device_scale that differs from the take's, or an image-layer take under a frame without a viewport.
**Approach (assumed at the header base):** export `storyboard.Frame.GridSize` (today `gridSize`) and compare it in `CheckFrame` (internal/render/pipeline.go); compare `Scaling()` with `recorded.Cell`. Tune's `atTakeSize` (internal/tune/session.go) writes the viewport, not cols/rows, on a viewport frame.
**Regression guard.** a browser take is one whose recorded.Cell is non-zero (terminal takes record a zero Cell; render's image-layer test takes such as saveImageTake in internal/render/pipeline_test.go carry Image entries on a terminal frame and must keep passing) — the refusal of a browser take under a frame without a viewport keys on recorded.Cell, never on Snapshot.Image; atTakeSize must return a frame that CheckFrame accepts for a browser take (viewport from the take's Cols/Rows, device_scale from its Cell), with a test that a tune session on a browser take whose storyboard viewport differs from the take still builds its preview. The device-scale comparison runs only on a viewport frame (`frame.HasViewport()`); a terminal frame keeps today's cols/rows check and its exact wording. Exporting `gridSize` updates its callers in internal/storyboard/fields.go and camera.go, and every comment naming the take size as cols×rows (tune/doc.go, `State`'s Cols/Rows doc; `grep -n "cols×rows\|recorded at another size" internal/tune internal/render`).
**Files:** internal/storyboard/storyboard.go, internal/render/pipeline.go, internal/render/pipeline_test.go, internal/tune/session.go, internal/tune/session_test.go, internal/storyboard/fields.go, internal/storyboard/camera.go, internal/tune/doc.go, internal/tune/state.go
**Read first:** internal/render/pipeline.go — CheckFrame, takeSizeError, Build; internal/tune/session.go — atTakeSize; internal/storyboard/storyboard.go — Frame.gridSize, Frame.Scaling, Frame.HasViewport; internal/render/pipeline_test.go — saveImageTake
**Tests:** CheckFrame table: matching browser take passes; viewport, device_scale and missing-viewport mismatches fail with `ErrTakeSize`; terminal cases and saveImageTake's image take unchanged; a tune session on a browser take whose storyboard viewport differs from the take still builds its preview.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render/`, then `GOMEMLIMIT=2GiB go test ./internal/tune/`, then `./internal/storyboard/`
**Commit:** `feat(render): check a browser take's grid against its viewport`

## 15. Render a browser take

**What:**
**Goal:** `photon render` turns a browser take into a GIF with camera moves, a zoom and a caption; no terminal font or rasterizer is loaded for a url-launch clip; captions and the drawn cursor are the size lint lays out; terminal renders are byte-identical.
**Approach (assumed at the header base):** in internal/render/pipeline.go and render.go, a url clip skips `loadTerminalFaces` and the raster/draft-raster builds, uses `pageLayout` as the layout, and gives the compositor `pageOutput`'s unit (`compose.New` takes the unit, or an Output variant does); draft frames scale the image layer. `drawsAlike`/`canReuseRasters` stay correct for both kinds.
**Regression guard.** "browser take" means recorded.Cell non-zero (item 14's rule) and "url-launch clip" means config.IsBrowser(); the url branch keys on the clip, the take check on the Cell. `compose.New` keeps its signature (compose_test.go and camera_test.go call it); the unit rides on an Output variant or a Layout field. On an image-layer pipeline `DraftAt`/`FullDraftAt` key on the draft cache, not `draftRasterizer`, so a url clip's drafts never fall back to `FrameAt`. The url oracle composes with `pageLayout` and `pageOutput`'s unit and no rasterizer (`raster.New` refuses a viewport frame); terminal oracles are unchanged.
**Files:** internal/render/pipeline.go, internal/render/render.go, internal/render/render_test.go, internal/render/pipeline_test.go, internal/compose/compose.go
**Read first:** internal/render/pipeline.go — Pipeline.buildFrames, draftRastersFrom, canReuseRasters, drawsAlike, DraftAt; internal/render/render.go — newCompositor, loadTerminalFaces; internal/render/pipeline_test.go — oracleFramesOf
**Depends on:** items 4, 14
**Tests:** a hand-built image take with a url config: `FrameAt` against an oracle; `DraftAt` on a url clip draws through the draft cache; an ffmpeg-skipping end-to-end render with a camera position, a zoom and a caption; existing terminal tests pass unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render/`, then `GOMEMLIMIT=2GiB go test ./internal/compose/`
**Commit:** `feat(render): render a browser take from its image layer`

## 16. Tune a browser take

**What:**
**Goal:** `photon tune` opens a browser take, previews and saves timing, camera, zoom, caption, `max_colors` and scale edits, shows viewport and device_scale instead of the terminal-only frame fields, and refuses a reflow edit (`frame.font_size`, `line_height`, `cols`, `rows`, `padding`) with a problem that says a viewport frame takes no such key.
**Approach (assumed at the header base):** the refusal is `Frame.validatePage`'s, surfaced through `Session.Apply`'s existing 422; `State` reports viewport and device scale; internal/tune/page/panels.js lists the frame fields by clip kind; marker.js clamps the aim to the viewport.
**Regression guard.** same definitions as item 15. The refusal covers a non-zero reflow value: `decodeCount` accepts 0 and `validatePage` flags only a set key, so `padding: 0` on a viewport frame is not refused; a test pins that zero case.
**Files:** internal/tune/state.go, internal/tune/session_test.go, internal/tune/edit_test.go, internal/tune/page/panels.js, internal/tune/page/marker.js, internal/tune/pagejs_test.go
**Read first:** internal/tune/state.go — Session.State, State; internal/tune/session.go — Session.Apply; internal/storyboard/validate.go — Frame.validatePage; internal/tune/page/panels.js — framePanel; internal/tune/page/marker.js — moveTo, clamp; internal/tune/pagejs_test.go — loadPage
**Depends on:** item 15
**Tests:** session on a hand-built browser take: frames build, a timing edit saves, `frame.font_size` is refused, a zero `frame.padding` edit is pinned; pagejs test of the panel fields for a viewport state.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Commit:** `feat(tune): tune a browser take`

## 17. Pick selector targets in tune

**What:**
**Goal:** tune's picker on a browser take proposes the text run under the pick and, when one of the clip's selectors has a recorded box holding the pick, that selector target (smallest box first); "set as position target" writes either.
**Approach (assumed at the header base):** in internal/tune/pick.go a pass over `screen.Selectors` for `clip.Selectors()`; `TargetState` and `pointState` (internal/tune/state.go) carry `Selector`; `Target.YAMLFlow` (internal/storyboard/edit_beat.go) writes a selector target; picker.js shows and sends it to `position.target` (item 3).
**Regression guard.** same definitions as item 15. The pick response (internal/tune/server.go `pickResponse`/`responseOf`) keeps `target`/`box`/`rect`/`snippets` as the text proposal and adds the selector proposal as an extra key, so existing pick tests hold. A selector proposal carries no `seen` snippet (`Expect` has no selector form); its nth is the picked box's index among `selectorMatches` in reading order (within the area if any), resolver-checked as `proposer.checked` does for text. Inspector editing of a selector aim (controls.js, kinds.js) is out of scope.
**Files:** internal/tune/pick.go, internal/tune/pick_test.go, internal/tune/state.go, internal/storyboard/edit_beat.go, internal/tune/page/picker.js, internal/tune/server.go, internal/tune/server_test.go
**Read first:** internal/tune/pick.go — proposer.propose, proposer.checked, snippetsOf; internal/tune/server.go — pickResponse, responseOf; internal/target/resolver.go — selectorMatches; internal/tune/state.go — TargetState; internal/tune/page/picker.js — showProposal
**Depends on:** items 3, 16
**Closes:** pixel-take-render-tune
**Tests:** pick on a browser take inside a selector box proposes both, the selector one with no `seen` snippet and the nth of the picked box (second of two matches); existing server pick tests unchanged; YAMLFlow of a selector target; terminal pick tests unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/`, then `./internal/storyboard/`
**Commit:** `feat(tune): propose selector targets from a pick`

## 18. Example: film photon tune's page

**What:**
**Goal:** `examples/tune-page/` has a `photon.yaml` and storyboard that record `photon tune` on a take of `examples/hello` (play, scrub, drag a beat edge, save) with every expect passing, shipping `docs/media/tune-page.gif`.
**Approach (assumed at the header base):** setup builds photon and the hello take into `$PHOTON_WORK`; `reset` copies the hello storyboard into the work dir so saves never touch the repo; service `photon tune ... --port "$PHOTON_PORT"`, `ready: {line: '^photon tune: http'}`; `launch.url: http://127.0.0.1:${PHOTON_PORT_TUNE}/`.
**Regression guard.** add ./cmd/photon (with `-run TunePage`) to the `browser` job in .github/workflows/ci.yml so the example's record-and-check test runs in CI; add .github/workflows/ci.yml to the item's Files line. The `browser`-tagged test goes in its own `cmd/photon/tunepage_browser_test.go` with `//go:build browser && !windows` (record_test.go's `!windows` tag binds its whole file); the plain lint test stays in record_test.go. The tune service passes `--config ../hello/photon.yaml` (or `reset` copies hello's photon.yaml too): from the work dir tune finds no photon.yaml. `ship` reads `../../docs/media/tune-page.gif` (it resolves against the storyboard's dir); "shipping" in the Goal means `ship` resolves there.
**Files:** examples/tune-page/photon.yaml, examples/tune-page/tune-page.yaml, cmd/photon/record_test.go, cmd/photon/tunepage_browser_test.go, .github/workflows/ci.yml
**Read first:** cmd/photon/record_test.go — TestMain, testBinary, helloStoryboard; cmd/photon/lint_test.go — runLint; internal/config/find.go — Find; internal/service/service.go — Start; internal/tune/run.go — urlFormat; examples/hello/photon.yaml — reset
**Depends on:** item 17
**Tests:** `photon lint` of the example in a plain test that also asserts the loaded `Storyboard.Ship` ends in docs/media/tune-page.gif; a `browser`-tagged record-and-check test in a temp `--work`.
**Acceptance:** `GOMEMLIMIT=2GiB go test -run TunePage ./cmd/photon/`, then `GOMEMLIMIT=2GiB go test -tags browser -run TunePage ./cmd/photon/`
**Commit:** `docs(examples): film photon tune's own page`

## 19. Ship the tune-page GIF in the README

**What:**
**Goal:** `docs/media/tune-page.gif` is committed, rendered from item 18's example, and the README shows it and links the example.
**Approach (assumed at the header base):** run `photon setup`, `record`, `check` and `render` on `examples/tune-page/tune-page.yaml` on this host; add the image and link near the README's tune section.
**Regression guard.** `-run` is case-sensitive and the README tests are `TestREADMEDocumentsEveryField`/`TestREADMEDocumentsEveryCommand`, so Acceptance runs `-run README`; Acceptance also checks the README embeds the GIF and links the example.
**Files:** docs/media/tune-page.gif, README.md
**Read first:** README.md — tune section ("**Tuning** with `photon tune`"), examples/hello section; cmd/photon/readme_test.go — TestREADMEDocumentsEveryField, TestREADMEDocumentsEveryCommand, readREADME; internal/render/render.go — render output MkdirAll; .gitattributes — *.gif binary
**Depends on:** item 18
**Closes:** film-tune-page
**Tests:** the README test still passes.
**Acceptance:** `test -s docs/media/tune-page.gif`, then `grep -q 'docs/media/tune-page.gif' README.md && grep -q 'examples/tune-page' README.md`, then `GOMEMLIMIT=2GiB go test -run README ./cmd/photon/`
**Commit:** `docs(readme): show photon tune filmed by photon`
