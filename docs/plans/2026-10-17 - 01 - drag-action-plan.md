# drag action — implementation plan

**Goal:** a storyboard can `drag`: press the left button at a target, move with it held along an
eased path, release at a target or an offset. The take logs the gesture, lint validates it, and the
rendered cursor follows it.
**Date:** 2026-10-17
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** b6cd49f
**Closes:** drag-action

**Sources:**
- `.capcom/issues/drag-action.md` (the issue; its `## Acceptance` is the plan's acceptance)
- Notes `glossary`, `package-layout`, `coding-conventions`

**Ratified design calls** (user, 2026-10-17):
- **Event kinds:** a drag's boxes are `target` events (Form `drag`, Text `from` / `to`); its cells are three new kinds `press`, `move`, `release`. `take.Event` gains no field, so `FormatVersion` stays 1 (the strict decoder would refuse a new field in an older photon).
- **Steps:** `steps` is optional, default 12 (`DefaultDragSteps`); an explicit value must be ≥1.
- **Easing:** the drag accelerates and decelerates: the engine places step i of n at smootherstep(i/n) of the path, at even time gaps; the drawn cursor eases from the press cell to the final move cell over the same span with the same curve.

**Standing requirements:**
- skills: coding-standards
- Tests per AGENTS.md: single packages, `GOMEMLIMIT=2GiB`, never `./...` with `-race`, never two test commands at once.
- No committed file names anything matching the forbidden regex in `AGENTS.local.md`.

**Out of scope:**
- buttons other than left; modifiers; curved or multi-segment paths
- `check` changes (no expect reads gestures)
- take format version bump

**Regression check (2026-10-17, b6cd49f):**
- 2: guard folded (README `offset` spelling, fixture files, `TestLoadRejects` base rules)
- 3: size-cap premise replaced (decision)
- 4: guard folded (decision: `dragged: 4,2 to 16,2`; target-event count 3)
- 5: guard folded (every click-only comment in internal/compose)
- 6: guard folded (decision: `dragged: 4,2 to 16,2` inside `frame.cols`; ffmpeg skip via `exec.LookPath`)
- 7: guard folded (wider missed-site grep, render help and doc, beat.go comment; doc-map clause dropped)

## 1. SGR motion report and the drag event kinds — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the drag events join the shared `sampleTake` fixture (documented as holding every event kind) rather than a separate take, so TestFile_SaveLoadRoundTrip and TestWrite_HeaderNamesFormatAndVersion together prove the round trip at version 1.

**What:**
**Goal:** `keys.Motion(x, y)` returns `ESC [ < 32 ; x+1 ; y+1 M`, and package `take` declares `EventPress` (`"press"`), `EventMove` (`"move"`) and `EventRelease` (`"release"`); a take holding them round-trips through `Write`/`Read` at `FormatVersion` 1.
**Approach (assumed at the header base):** in `internal/keys/keys.go` add a `motionButton = 32` constant (left button held, motion bit) beside `leftButton`, and give `sgrMouse` a button parameter so `Click`, `Release` and `Motion` share it. In `internal/take/event.go` add the three kinds with comments (each carries a Cell; Form `drag`), extend the `EventTarget` comment to "a click's or a drag's target" and the `Event` doc to name `drag` among the forms and Text `from`/`to` on a drag's target events. No new `Event` field.
**Files:** internal/keys/keys.go, internal/keys/keys_test.go, internal/take/event.go, internal/take/file_test.go
**Read first:** internal/keys/keys.go — sgrMouse, Click, Release, leftButton, pressFinal; internal/keys/keys_test.go — TestClickAndReleaseEncodeAnSGRReportAtTheCell; internal/take/event.go — EventKind consts, Event
internal/take/file_test.go — sampleTake, TestFile_SaveLoadRoundTrip, TestWrite_HeaderNamesFormatAndVersion; internal/take/file.go — Read, readLines (DisallowUnknownFields); internal/compose/cursor.go — trackCursor (reads only EventClick)
**Tests:** `keys_test.go`: `Motion` at (0,0) and (9,4) gives the exact bytes. `file_test.go`: a take with target(Form drag, Text from), press, move, release events round-trips and its header still says version 1.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/keys ./internal/take` passes; `go vet ./internal/keys ./internal/take`.
**Commit:** `feat(keys): SGR held-button motion report; take: press, move and release events`

## 2. The `drag` action in the storyboard, lint and README reference — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the README `drag` row says what the action does (press at `from`'s centre, eased held moves, release at `to`) but leaves when `to.text` is resolved to item 3's engine work; the full drag description in the storyboard reference stays with item 7.
NOTES (2026-10-17): added a strict-decode `TestLoadRejects` row ("unknown offset field") on top of the plan's rows, to pin that the inline `to` target still refuses unknown fields.
NOTES (2026-10-17): the lint test for `testdata/invalid/drag.yaml` passes `--config testdata/valid/photon.yaml`, as the undeclared-area row does, so the invalid/ config's own broken rule does not mix into the drag's output.

**What:**
**Goal:** a storyboard decodes `drag: {from: <target>, to: {text|offset…}, steps: n}`; `photon lint` reports a bad drag (both or neither of `to.text`/`to.offset`, a zero offset, `to.nth`/`to.area` with an offset, `steps` < 1, a bad `from`) and an undeclared `drag.from.area` / `drag.to.area`; the README documents every new field.
**Approach (assumed at the header base):** in `internal/storyboard/beat.go` add `Drag *DragAction \`yaml:"drag"\`` to `Action` (between `Click` and `Wait`) and types `DragAction{From Target "from"; To DragEnd "to"; Steps *int "steps"}` with `StepCount()` (default `DefaultDragSteps = 12`), `DragEnd{Target \`yaml:",inline"\`; Offset *Offset "offset"}` and `Offset{X "x"; Y "y"}` (cells, may be negative). In `validate.go` count `Drag` in `Action.validate` (message becomes "want exactly one of type, key, click, drag, wait or pause") and add `DragAction.validate`: `from.` via `Target.validate`; `to`: exactly one of `text`/`offset` ("to: want exactly one of text or offset"); with text, `Target.validate` under `to.`; with offset, `to.offset: want a non-zero x or y` for {0,0} and `to.nth`/`to.area: only a to.text takes …`; `steps: want ≥1, got n` when set below 1. In `internal/clip/clip.go` `namedAreas` add `do[i]: drag.from.area` and `do[i]: drag.to.area`. README: an Actions-table row for `drag` (fields `from`, `to`, `steps` default 12) and a short paragraph spelling `to.text`, `to.offset.x`, `to.offset.y` — `TestREADMEDocumentsEveryField` needs every new tag in backticks.
**Regression guard.** README also spells `to.offset` in backticks: `TestREADMEDocumentsEveryField` matches only "`tag`" or ".tag`", so `to.offset.x` does not cover the `offset` tag; every new tag stays the rule. Fixtures: the every-form drags go in `internal/storyboard/testdata/every-form.yaml`; the undeclared drag goes last in `internal/clip/testdata/undeclared/demo.yaml` and extends the `want` list of `TestLoad_UndeclaredAreaFailsNamingIt` (pinned by count and order); the bad-drag lint storyboard is a new fixture under `cmd/photon/testdata/invalid/`.
`TestLoadRejects` needs each `old` fragment exactly once in `validBase`: append any `validBase` drag after the pause (do[5]), using fragments found nowhere else in `validBase`, so the rows pinning `do[4]` stay valid; update the two rows pinning the old action-form message ("two action forms", "no action form").
**Files:** internal/storyboard/beat.go, internal/storyboard/validate.go, internal/storyboard/storyboard_test.go, internal/storyboard/testdata/every-form.yaml, internal/clip/clip.go, internal/clip/clip_test.go, internal/clip/testdata/undeclared/demo.yaml, cmd/photon/lint_test.go, cmd/photon/testdata/invalid/drag.yaml (new), README.md
**Read first:** internal/storyboard/validate.go — Action.validate, Target.validate, validateArea, prefixed; internal/storyboard/beat.go — Action, ClickAction, Target, Nth.UnmarshalYAML; internal/storyboard/storyboard_test.go — validBase, TestLoadEveryForm, TestLoadRejects
internal/clip/clip.go — namedAreas, crossCheck; internal/clip/clip_test.go — TestLoad_UndeclaredAreaFailsNamingIt; cmd/photon/readme_test.go — TestREADMEDocumentsEveryField, yamlTags; cmd/photon/lint_test.go — TestLint, runLint; README.md — Actions table
**Tests:** `TestLoadEveryForm` decodes a drag with `to.text` and one with `to.offset` (both added to `every-form.yaml`); `TestLoadRejects` rows for each error above, with the exact field path, and the two action-form rows updated; `clip_test.go` an undeclared `drag.to.area` is named (last in `want`); `lint_test.go` one bad-drag storyboard (`testdata/invalid/drag.yaml`) exits 1 naming the field.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard ./internal/clip ./cmd/photon -run 'TestLoad|TestLint|TestREADME'` passes.
**Commit:** `feat(storyboard): drag action with from, to and steps; lint validates it`

## 3. The engine performs a drag — ✅ DONE (2026-10-17)

NOTES (2026-10-17): `dragPath` returns `[]dragMove` (cell plus the 1-based step it lands on) rather than `[]take.Point`, so the send loop can sleep every step's gap while sending only the steps that change cell, as the approach requires; the deduplicated cells are exactly the plan's path.
NOTES (2026-10-17): both target ends resolve on one `Current()` snapshot taken at the start of the drag, which is also the grid the end cells clamp to.
NOTES (2026-10-17): consequential edit — internal/engine/engine.go: made necessary by the drag form (the `Engine` and `New` doc comments named only clicks).
NOTES (2026-10-17): a cancelled context or a failed send between the press and the release leaves the program's button held; the run ends at that failure anyway, so no release is sent on the failure path.

Depends on items 1 and 2.

**What:**
**Goal:** the engine performs a `drag`: it resolves `from` on the current screen, logs it, works out the end cell (a resolved `to` target's centre, logged, or the press cell plus the offset), clamps both to the screen, then sends the press, the eased motion reports and the release, logging each cell sent.
**Approach (assumed at the header base):** new file `internal/engine/drag.go` (the drag form's own file: one action form with its path helper): `formDrag = "drag"` joins the forms in `action.go` (`formOf`, `perform`). `(*Engine).drag(ctx, step, storyboard.DragAction)`: resolve `From` via `e.resolver.Target(e.term.Current(), …)` and log `EventTarget` with Text `from` (errors wrap as `drag: from: …`); for `To.Offset == nil` resolve `To.Target` on the screen as it stands and log `EventTarget` Text `to`; centre cells use the same `box.X + (box.W-1)/2` rule as `click` — extract a `centreOf(take.Box) take.Point` helper both call. Clamp to the snapshot grid (`len(Cells)` rows, row width cols). Pure func `dragPath(from, to take.Point, steps int) []take.Point`: for i in 1..steps the cell at `dragEase(i/steps)` of the way, rounded, consecutive duplicates dropped (a terminal reports motion only on a cell change); `dragEase` is smootherstep, commented as the curve compose's cursor uses. Send: log `EventPress` + `keys.Click` at from; per step sleep `dragStepGap` (40 ms, named beside `clickHoldGap`) then, if the step has a new cell, log `EventMove` + `keys.Motion`; sleep `dragStepGap`, log `EventRelease` + `keys.Release` at the end cell. Every step sleeps, sent or not, so timing stays even. Update `internal/engine/doc.go` package text and file map (`drag.go`).
**Files:** internal/engine/drag.go, internal/engine/action.go, internal/engine/doc.go, internal/engine/drag_test.go
**Read first:** internal/engine/action.go — formOf, perform, click, sendClick, clickHoldGap, sleepContext; internal/engine/engine.go — runBeat, logStep, BeatError; internal/engine/engine_test.go — fakeTerminal, screenOf, fakeCols, stepEvents, beatOf, TestEngine_Run_ActionForms
internal/target/resolver.go — Resolver.Target; internal/engine/doc.go — file map; internal/compose/zoom.go — loggedBox (skips non-click actions)
**Tests:** `drag_test.go` table over `dragPath`: straight horizontal, diagonal, from==to (no cells), steps > distance (no duplicates, last cell is `to`), eased spacing (first and last gaps ≤ the middle gap). Fake-terminal test (the `engine_test.go` fake): bytes sent are press, motions, release in order; events are target(from), [target(to)], press, moves, release; an offset past the right edge clamps to `cols-1`; a missing `from` fails with `drag: from:`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/engine -run 'Drag|TestDocMap'` passes; `go vet ./internal/engine`.
**Commit:** `feat(engine): perform a drag with eased motion reports`

## 4. The fixture TUI receives a drag — ✅ DONE (2026-10-17)

NOTES (2026-10-17): handleSequence in main.go now only parses (parseSGR) and hands the report to screen.handleMouse in mouse.go, which tracks the gesture (press cell, move count) and sets the status on release; the item selection moved out of handleSequence into screen.selectAt (main.go), called on press. main.go is 380 lines.
NOTES (2026-10-17): the engine fixture test also pins the drag's press cell (alpha's centre 4,2) and release cell (16,2), beyond the plan's counts-and-order check.

Depends on item 3.

**What:**
**Goal:** the fixture TUI enables button-event mouse tracking, and after a left-button press, ≥1 held-button motion and a release it shows `dragged: <x>,<y> to <x>,<y> after <n> moves` (0-based cells); the engine's fixture test drives a drag and the input log holds press, motions, release in that order.
**Approach (assumed at the header base):** in `internal/testkit/fixturetui` switch `enableSGRMouse`/`disableSGRMouse` from mode 1000 to 1002 (button-event tracking), move `parseSGRPress` into a new `mouse.go` generalised to `parseSGR(sequence) (report, bool)` returning button, cell and press/release (main.go is near its size cap), and track the drag in `screen` (press cell, move count) so `handleSequence` keeps the item selection on press and sets the status on a release that followed ≥1 move. Add the status prefix as a named constant beside `selectedPrefix`. Update the fixture's `doc.go` text and map. In `internal/engine/fixture_test.go` add a beat `drag` that drags from `alpha` to `{offset: {x: 12}}` with `steps: 6`, waits for `dragged: 4,2 to 16,2`, and extend `assertFixtureInput` / `assertFixtureEvents` with the drag's expected bytes (from `dragPath`) and events.
**Regression guard.** The drag from `alpha` presses at alpha's centre (4,2), so the expected status is `dragged: 4,2 to 16,2` everywhere item 4 names it. `assertFixtureEvents` pins `counts[take.EventTarget] != 2`: the drag logs one more target event (`from`; an offset `to` logs none), so the pin becomes 3; also check that the press, move and release counts are ≥1 and in order.
**Files:** internal/testkit/fixturetui/main.go, internal/testkit/fixturetui/mouse.go, internal/testkit/fixturetui/doc.go, internal/engine/fixture_test.go
**Read first:** internal/testkit/fixturetui/main.go — handleSequence, parseSGRPress, enableSGRMouse, screen, selectedPrefix; internal/engine/fixture_test.go — fixtureBeats, assertFixtureInput, assertFixtureEvents, betaCentre; internal/engine/action.go — click, sendClick
internal/testkit/fixturetui/doc.go — package comment, file map; internal/keys/keys.go — Click, Release, sgrMouse
**Tests:** the extended `TestEngine_Run_FixtureTUI` (status `dragged: 4,2 to 16,2`, 3 target events, press/move/release counts ≥1 and in order).
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/testkit/... ./internal/engine -run 'Fixture|TestDocMap'` passes.
**Commit:** `test(fixturetui): report a received drag; engine fixture test drags`

## 5. The drawn cursor follows a drag; zoom aims at a drag's box — ✅ DONE (2026-10-17)

NOTES (2026-10-17): trackCursor keeps only a drag's final move (consecutive moves of one beat collapse into the last), so position eases press-to-final-move in one smootherstep curve; `clickMark` keeps its name and gains a `Kind` (markGlide for click/press, markFollow for the final move, markRing for the release).
NOTES (2026-10-17): compose defines its own `dragFromText`/`dragToText` ("from"/"to") for the drag target events' Text, since engine's constants of the same name are unexported and compose does not import engine.

Depends on items 1 and 2.

**What:**
**Goal:** in a render, the cursor glides to a drag's press cell as it does to a click, eases (smootherstep) from the press cell to the final move cell between their output times, sits at the release cell, rings on the press and on the release (not on moves), and fades out `cursorLinger` after the beat's last press or release; a zoom whose target equals a drag's `from` (or its `to` target) in the same beat aims at that logged box.
**Approach (assumed at the header base):** in `internal/compose/cursor.go`, `trackCursor` also collects `EventPress`, `EventMove`, `EventRelease` (dropped when `schedule.Output` gives no time, as clicks are); `clickMark` gains what `position`/`ring`/`opacity` need to tell a glide-and-ring mark (click, press), a follow mark (move) and a ring-only mark (release). `position` eases with the existing `easeInOut` from the press point to the last move before the release, then holds; a drag with no moves holds at the press. In `zoom.go` `loggedBox` also accepts a target event whose action is a `Drag`: Text `from` matches `drag.From == beat.Zoom.Target`, Text `to` matches `drag.To.Offset == nil && drag.To.Target == beat.Zoom.Target`. Update `doc.go` text ("click cursor" → cursor over clicks and drags) and the map line for cursor.go.
**Regression guard.** The doc fix is a rule, not the closed list above: every comment in internal/compose that says the cursor or the logged zoom box concerns clicks only names drags too — `doc.go`'s package text and map, `compose.go`'s `Compositor`/`New`/`drawCursor` comments, `cursor.go`, and `zoom.go`'s `zoomBox`/`loggedBox`. Find them with `grep -n "click" internal/compose/*.go | grep -v _test`.
**Files:** internal/compose/cursor.go, internal/compose/zoom.go, internal/compose/doc.go, internal/compose/compose.go, internal/compose/cursor_test.go
**Read first:** internal/compose/cursor.go — trackCursor, clickMark, cursorTrack.position, cursorTrack.opacity, cursorTrack.ring; internal/compose/zoom.go — loggedBox, zoomBox, easeInOut; internal/compose/compose_test.go — clickAt, beatStart, newTestCompositor, centroidWhere
internal/compose/doc.go — package comment; internal/compose/compose.go — Compositor, New
**Tests:** `cursor_test.go` with a hand-built take and schedule: position at press, at the drag's time midpoint (exactly the path midpoint, smootherstep(0.5)=0.5), near the start (less than the linear share), at the release; ring present at press and release, absent mid-drag; opacity still 1 between press and release and falling after release + linger; a drag in a cut beat draws nothing; a zoom on a drag's `from` target uses the logged box. One test draws a frame mid-drag via the `Compositor` and finds the cursor's dot pixels at the expected point.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/compose` passes; `go vet ./internal/compose`.
**Commit:** `feat(compose): the cursor follows a drag; zoom aims at a drag's box`

## 6. A recorded drag passes record, check and render — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the drag beat follows its drag with `wait: {screen: 'dragged: 4,2 to 16,2', timeout: 5s}` so the beat-end expect does not race the fixture's status redraw; the storyboard opens with the usual ready-wait beat, and the take assertion also pins the press cell (4,2) and the release cell (16,2).

Depends on items 4 and 5.

**What:**
**Goal:** a storyboard that drags in the fixture TUI lints clean, records a take whose beats and expects all pass (exit 0), and — when ffmpeg is on PATH — renders it.
**Approach (assumed at the header base):** in `cmd/photon/record_test.go` add a storyboard built from `projectHeader` + `openBeat`-style beats: a beat that drags `from: {text: alpha}` `to: {offset: {x: 12}}` with `steps: 6` and expects `seen: 'dragged: 4,2 to 16,2'`. Run `lint`, `record`; assert exit 0 and the check table's PASS row; load the take and assert it holds press, ≥1 move and release events in order. Then `render` the take when `ffmpeg` is found (skip otherwise, as internal/render's `TestRunWritesAGIFOfTheScheduledLength` does).
**Regression guard.** Same correction as item 4: the expect reads `seen: 'dragged: 4,2 to 16,2'`, and the end cell must lie inside the test storyboard's `frame.cols`. cmd/photon's render tests never call ffmpeg (all run `--dry-run`); skip with `exec.LookPath("ffmpeg")` + `t.Skip` written in cmd/photon, after internal/render's `TestRunWritesAGIFOfTheScheduledLength` (its `ffmpegProgram` is unexported).
**Files:** cmd/photon/record_test.go
**Read first:** cmd/photon/record_test.go — writeProject, projectHeader, openBeat, TestRecordHello, assertTake, runLint; internal/render/render_test.go — TestRunWritesAGIFOfTheScheduledLength; cmd/photon/render_test.go — TestRender
cmd/photon/render.go — newRenderCommand (--output)
**Tests:** the new test above.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/photon -run 'Record|Drag'` passes.
**Commit:** `test(record): a dragging storyboard records, checks and renders`

## 7. Docs and Notes describe the drag

Depends on items 2, 3 and 5.

**What:**
**Goal:** the README's take, mouse and cursor prose, the `glossary` Note and the `package-layout` Note describe the drag.
**Approach (assumed at the header base):** README: "The take and the retake loop" names `click` or `drag` among failing actions and adds presses, moves and releases to the take's log; under Actions, the mouse note says a drag needs the program to enable button-event (1002) or any-event (1003) tracking; "Targets and areas" says a target is what a `click`, `drag` or `zoom` aims at; "Sections, zoom and cursor" describes the eased follow and the two rings. Notes via hand edit then `capcom check`: `glossary` Action list gains `drag`, Target line names drags; `package-layout` `keys` row reads "SGR mouse press, motion and release", `target` row "(clicks, drags, zooms, `seen`)". Rule for any missed site: every prose line listing the action forms or saying what a target is aimed by — find them with `grep -rn "type\`, \`key\`\|click or zoom\|click, zoom" README.md .capcom internal`.
**Regression guard.** The missed-site rule also matches "clicks, zooms", "click cursor" and "a click of the same beat" and searches `cmd`, excluding closed issues: `grep -rn "type\`, \`key\`\|click or zoom\|click, zoom\|clicks, zooms\|click cursor\|a click of the same beat" README.md .capcom/knowledge internal cmd`. Known sites: README "a `click` or a `zoom` aims at" (Targets and areas), "serves clicks, zooms and `seen`", the zoom sentence "box a click of the same beat … logged"; render's `--help` Long ("composes the zooms and the click cursor", `cmd/photon/render.go`); `internal/render/doc.go` package text; the `Target` comment in `internal/storyboard/beat.go` ("a click or zoom resolves"). internal/compose hits are item 5's. The doc-map clause is dropped from the Goal: the .go edits here are comments and help text only.
**Files:** README.md, .capcom/knowledge/glossary.md, .capcom/knowledge/package-layout.md, cmd/photon/render.go, internal/render/doc.go, internal/storyboard/beat.go
**Read first:** README.md — "The take and the retake loop", "Actions", "Targets and areas", "Sections, zoom and cursor"; .capcom/knowledge/glossary.md — Action, Target; .capcom/knowledge/package-layout.md — internal/keys row, internal/target row
cmd/photon/readme_test.go — TestREADMEDocumentsEveryField; cmd/photon/render.go — newRenderCommand Long
**Tests:** `cmd/photon` README tests; `capcom check`; `go vet ./cmd/photon ./internal/render ./internal/storyboard`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/photon -run TestREADME` passes; `capcom check` reports no problems.
**Commit:** `docs: describe the drag action`
