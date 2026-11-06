# Remaining issues plan

**Goal:** Close the five open issues: styled caption spans, camera and caption anchors at a cut
beat, drag buttons/modifiers/waypoints, and the winget and npm release channels.
**Date:** 2026-11-06
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** ac38674

**Regression check (2026-11-06, ac38674):**
- 1: guard folded
- 2: guard folded
- 3: guard folded
- 4: guard folded
- 5: guard folded
- 6: guard folded
- 7: guard folded
- 8: guard folded
- 9: guard folded
- 10: guard folded
- 11: guard folded
- 12: recast; yields to `cmd/release/doc.go` / `cmd/release/runner.go` (channel-repo reads go through channelRepos)
- 13: guard folded
- 9 (re-check): guard extended by decision — item 9 owns renderWingetManifests, wingetManifest, wingetManifests; item 10 only calls them
- 12 (re-check): guards folded — NPM_TOKEN checked before any download; smoke fake publishes npm and tallies move by one pass; failingChannels fails the registry read; an unknown npm probe refusing publish is accepted; smoke runs `photon version`
- 12 (re-check): decision applied — the probe checks the wrapper first and probes the six platform packages only when it is absent

**Sources:**
- `.capcom/issues/caption-char-styling.md`, `cut-beat-camera.md`, `drag-action-options.md`,
  `npm-release-channel.md`, `winget-release-channel.md`
- Release-tool port source: `AGENTS.local.md` § "Release-tool port source"; take its winget code
  at commit `81cd62d` (counts only the signed-in account's pull requests), not the older pin
- `docs/plans/archived/2026-10-17 - 03 - release-tooling-plan.md` item 4 (what the winget drop removed)

**Ratified design calls (user, 2026-11-06):**
- **Caption spans:** inline tags in `text`: `[#rrggbb]`, `[bold]`, `[bold #rrggbb]`, closed by `[/]`; `[[` is a literal `[`.
- **Cut-beat anchor:** `{beat: N}` on a cut beat resolves to the instant the cut lands (its sitting start); applies to positions, zooms and captions.
- **Drag path:** `via:` list of targets/offsets (offset from the previous point), all resolved on the start screen; `steps` per segment, each segment eased; straight drags render byte-identically.
- **Mouse buttons:** `button: left|middle|right` (default left) and `with: ctrl+shift` (key-action modifier names `shift`, `alt`, `ctrl`) on both `click` and `drag`.
- **npm layout:** wrapper `@airiclenz/photon` + six `@airiclenz/photon-<os>-<arch>` optionalDependencies with os/cpu fields; JS launcher; auth by `NPM_TOKEN` env only.
- **winget:** port the komac-based channel, package id `AiricLenz.Photon`; installer declares `Gyan.FFmpeg`; the step ends "submitted"; installable once winget-pkgs merges.

**Standing requirements:**
- skills: coding-standards
- Pi host: `GOMEMLIMIT=2GiB`, one package per `go test`, never `./...` with `-race`, never two test commands at once.
- The forbidden-names grep in `AGENTS.local.md` matches nothing the plan commits.
- Every new storyboard yaml tag appears in README (`TestREADMEDocumentsEveryField`).
- A new non-test file in a doc-mapped package is added to its `doc.go` map.

**Out of scope:** italic or background spans, theme colour names in spans; tune editors for drag
or via; a drawn cursor showing button or modifiers; a take format bump; a winget smoke check;
npm trusted publishing; any version bump.

## 1. Parse caption span markup into runs — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the grammar also refuses a `[` with no closing `]` (`tag "[Enter" has no closing ]`), a sixth error beside the five the plan lists plus "no token"; every markup error ends `(write [[ for a literal [)`.

NOTES (2026-11-06): validateColor's #rrggbb parsing moved into a shared parseColor (caption.go) so span colours reuse it; the caption's `text: missing` check now runs after its colour resolves (runs need the text colour), which changes only the order of problems a caption reports.

NOTES (2026-11-06): the styled-caption load test lives in caption_markup_test.go (TestParse_CaptionRuns, which also covers the trailing-newline case) rather than in testdata/every-form.yaml, so every-form's other consumers are untouched.

**What:**
**Goal:** a caption's `text` may hold span tags; loading turns it into styled runs, refusing malformed markup under `captions[i].text`.
**Approach (assumed at the header base):** new `internal/storyboard/caption_markup.go` owns the
grammar: `[` opens a tag unless doubled (`[[` → literal `[`); a tag holds space-separated tokens,
each `bold` or `#rrggbb` (at most one colour, at least one token); `[/]` closes; spans do not
nest; a span may cross `\n`; `]` needs no escape. Errors: unknown token, two colours, unclosed
span, `[/]` without a span, nested tag, empty span. `(*Caption).validate` (which runs before
`applyDefaults`) parses the trimmed text and stores `runs []Run` (`Run{Text string; Color RGB; Weight Weight}`), unstyled
runs carrying the caption's own `TextColor()` and `Font.Weight`; read via `Caption.Runs()`. Text
with no tags yields exactly one run equal to today's text. tune's `setCaptionText` needs no change
(strict reload refuses bad markup).
**Regression guard.** 1: every markup error message names the `[[` escape for a literal `[`,
since an existing caption holding a literal `[` (e.g. "Press [Enter]") now fails to load. Parse
validates before `applyDefaults`, so validate builds runs from `strings.TrimSuffix(c.Text, "\n")` with
weight `cmp.Or(c.Font.Weight, WeightRegular)` (tune's block-scalar `"All green\n"` gets no trailing
run); `TestLoadEveryForm` fills runs in each `wantCaptions` entry; `doc.go` maps `caption_markup.go` ([Run], the grammar).
**Files:** internal/storyboard/caption_markup.go, internal/storyboard/caption_markup_test.go, internal/storyboard/caption.go, internal/storyboard/storyboard_test.go, internal/storyboard/doc.go
**Read first:** internal/storyboard/caption.go — (*Caption).validate, (*Caption).applyDefaults, hexColor; internal/storyboard/storyboard.go — Parse;
internal/storyboard/storyboard_test.go — TestLoadEveryForm, TestLoadRejects_Captions; internal/storyboard/doc.go — file map;
internal/tune/session_test.go — captionsStoryboard
**Tests:** table test over every grammar rule and error text (each naming `[[`); a storyboard load with a styled caption; `TestLoadEveryForm` captions carry their runs; a text ending `\n` yields no trailing run.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
**Commit:** `feat(storyboard): parse styled spans in caption text`

## 2. Draw caption runs and document the markup — ✅ DONE (2026-11-06)

NOTES (2026-11-06): consequential edit — internal/caption/doc.go: made necessary by the text now rasterizing to one coverage mask per run colour, each run in its weight's face (the package comment named a single mask).

NOTES (2026-11-06): wrapLine was rewritten over byte offsets into a styledLine (the concatenated run text) rather than a word-by-word string build; a one-run line measures and wraps exactly as before (wrapped.png and every TestNewTextBlock_* case pass unchanged).

NOTES (2026-11-06): a block's line height, ascent and descent are the largest among the faces its runs use, so a one-weight caption keeps its old metrics; the README lint paragraph also gained the span-markup problems item 1 reports.

NOTES (2026-11-06): added TestNewTextBlock_Runs_WrapAcrossRunBoundariesKeepingTheirStyles (layout_test.go) beside the planned golden and span-colour pixel test (TestDraw_SpanColour_AppearsOnlyWhereTheSpanIs).

**What:** Depends on item 1.
**Goal:** captions draw each run in its colour and weight; a caption of one run matches the unchanged `wrapped.png` golden without `-update`; a new golden covers a coloured bold span wrapping across lines; README documents the markup.
**Approach (assumed at the header base):** `internal/caption`: `newTextBlock` / `wrapLine` work
over runs, measuring each fragment with its run's face (`faces.Regular` / `faces.Bold`), greedy
break at spaces across run boundaries; `placed` keeps one alpha mask per distinct colour, and
`Layer.Draw` draws each mask with `draw.DrawMask` under the box. A one-run caption takes today's
single-mask path unchanged. `photon lint` reaches it through `render.CheckCaptions` → `caption.New`.
README "### Captions": field table `text` row and a short markup example.
**Regression guard.** 2: README "### Captions" states that a literal `[` is written `[[`.
`layout_test.go`'s `wrapTestLines` (which calls `newTextBlock`) adapts to the run signature, its
`TestNewTextBlock_*` assertions unchanged.
**Files:** internal/caption/layout.go, internal/caption/caption.go, internal/caption/layout_test.go, internal/caption/golden_test.go, internal/caption/caption_test.go, internal/caption/testdata/styled.png, README.md
**Read first:** internal/caption/layout.go — newTextBlock, wrapLine, textBlock.mask; internal/caption/caption.go — placed, Layer.Draw;
internal/caption/layout_test.go — wrapTestLines; internal/caption/golden_test.go — assertMatchesGolden; README.md — ### Captions field table
**Tests:** `TestDraw_StyledSpan_MatchesItsGolden` (new `testdata/styled.png`); pixel test that span colour appears only inside the span's box region; existing goldens untouched; `wrapTestLines` on the run signature, `TestNewTextBlock_*` unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/caption/ && GOMEMLIMIT=2GiB go test ./internal/render/ -run Caption && GOMEMLIMIT=2GiB go test ./cmd/photon/ -run README`
**Commit:** `feat(caption): draw styled spans in their own colour and weight`
**Closes:** caption-char-styling

## 3. Anchor camera moves and captions at a cut beat — ✅ DONE (2026-11-06)

NOTES (2026-11-06): OutputClock.Resolve now delegates to SectionStart; clockSection's isCut field, read only by the removed refusal, is deleted as dead code.

NOTES (2026-11-06): the cut-beat caption reject cases in TestLoadRejects_Captions became one case showing a cut-beat anchor still faces the usual range checks (`at: {beat: 2}` lands on 3s, the stop's own instant); loading at/until on a cut beat is covered by new rows in TestParse_ResolvesCaptionValues, and positions/zooms by new entries in TestParse_ResolvesMoveStarts.

**What:**
**Goal:** `at`/`until` naming a cut beat resolve to that beat's sitting start plus offset and then pass the usual range checks; storyboards using it load, render and show in tune at the cut marker; README and glossary state the meaning.
**Approach (assumed at the header base):** `storyboard.OutputClock.Resolve` stops refusing a cut
beat and uses the instant `SectionStart` already returns (next kept beat's start; clip `Length`
if last, so a trailing cut needs a negative offset). Positions, zooms (`placeMove`) and captions
share it. Text targets need no compose change: `shownAt` finds them on the screen shown at that
instant. Prose guard: every message, comment, test and doc line saying an anchor may not name a
cut beat — `grep -rn "is cut, so it has no clip time\|missing or cut beat\|one that is cut" --include='*.go' --include='*.md' .`
README: position `at` row, camera and caption lint lists, Sections paragraph. Glossary: add a
**Cut beat** entry (recorded and checked, plays no time, anchors resolve to where the cut lands).
**Regression guard.** The tune test goes in `internal/tune/session_test.go` (no `state_test.go`
exists at the base): cut beat 1 of `sessionStoryboard`, which needs no take change, and anchor a
position at `{beat: 1}`. The prose guard covers `OutputClock.Resolve`'s doc comment too ("one that
is cut, which has no time on the output clock"), hence the grep's third pattern.
**Files:** internal/storyboard/clock.go, internal/storyboard/moment_test.go, internal/storyboard/storyboard_test.go, internal/compose/camera_test.go, internal/tune/session_test.go, README.md, .capcom/knowledge/glossary.md
**Read first:** internal/storyboard/clock.go — OutputClock.Resolve, OutputClock.SectionStart; internal/storyboard/camera.go — placeMove;
internal/storyboard/caption.go — validateTiming; internal/storyboard/storyboard_test.go — TestLoadRejects_Captions cutBeatCases;
internal/compose/camera.go — shownAt; internal/compose/camera_test.go — cameraTake (no cut beats; build the board by hand); internal/tune/session_test.go — newSessionFixtureOf
**Tests:** moment resolves on a middle, leading and trailing cut beat; caption `at`/`until` on a cut beat loads; compose aims a text target anchored at a cut beat on the next kept beat's first screen; tune state (in `session_test.go`, beat 1 of `sessionStoryboard` cut) gives such a position `start_ms` equal to the cut section's `StartMS`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/ && GOMEMLIMIT=2GiB go test ./internal/compose/ && GOMEMLIMIT=2GiB go test ./internal/tune/ && capcom check`
**Commit:** `feat(storyboard): resolve anchors at a cut beat to where the cut lands`
**Closes:** cut-beat-camera

## 4. Encode mouse buttons and modifiers — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the unexported leftButton/motionButton constants are replaced by the exported Button constants (LeftButton, MiddleButton, RightButton) and an unexported motionBit; Mods is a bitset type with Shift/Alt/Ctrl valued as their SGR bits (4/8/16). An empty name between `+` signs (`ctrl+`) is refused as `unknown modifier ""`. doc.go's left-button package wording is left for item 6, per the plan's guard.

**What:**
**Goal:** `internal/keys` encodes SGR press, motion and release for any of left/middle/right with any of shift/alt/ctrl, and parses the storyboard spellings.
**Approach (assumed at the header base):** extend `keys.go` (no new package): `Mouse{Button;
Mods}`, `ParseButton(string)` (`left|middle|right`), `ParseMods(string)` (`+`-joined `shift`,
`alt`, `ctrl`, the names the key table uses; error on unknown or repeated). `sgrMouse` codes:
button 0/1/2, +4 shift, +8 alt, +16 ctrl, +32 on motion; release repeats the press code with
final `m`. Add `MousePress/MouseMotion/MouseRelease(x, y, Mouse)`; existing `Click`, `Motion`,
`Release` delegate with the zero `Mouse` and keep their bytes.
**Regression guard.** `ParseButton("")` is left and `ParseMods("")` no modifiers (zero `Mouse`, no
error), so an absent `button`/`with` keeps every click and drag of `every-form.yaml` loading; table
rows for both. `doc.go`'s file map names the new mouse functions beside [Click] and [Release]; its
left-button wording, like every non-README Go comment on a left-only gesture, is item 6's.
**Files:** internal/keys/keys.go, internal/keys/keys_test.go, internal/keys/doc.go
**Read first:** internal/keys/keys.go — sgrMouse, Click, Motion, Release, leftButton; internal/keys/keys_test.go — TestClickAndReleaseEncodeAnSGRReportAtTheCell;
internal/keys/doc.go — package comment; internal/storyboard/validate.go — KeyAction.validate (the Lookup/Names refusal shape to mirror)
**Tests:** table of exact byte strings per button × modifier × press/motion/release; parse errors; the empty spellings parse to the zero `Mouse`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/keys/`
**Commit:** `feat(keys): encode mouse buttons and modifier keys`

## 5. Declare button, modifiers and via points in the storyboard — ✅ DONE (2026-11-06)

NOTES (2026-11-06): `button` and `with` live on one inline-embedded `storyboard.Press` struct shared by ClickAction and DragAction (not two field pairs); `Press.Mouse()` returns `(keys.Mouse, error)` rather than a bare `keys.Mouse`, so no parse error is discarded — the error is unreachable on a loaded storyboard, and item 6 calls it.
NOTES (2026-11-06): README line on the drawn cursor ("eases from the press cell to the drag's last move") is cursor prose, not an offset claim; left for item 7, which draws the cursor along every drag segment.

**What:** Depends on item 4.
**Goal:** `click` and `drag` accept `button` and `with`; `drag` accepts `via`, a list of targets or offsets; invalid values are refused on load; README documents all three.
**Approach (assumed at the header base):** `ClickAction` and `DragAction` (`beat.go`) gain
`Button string 'button'` and `With string 'with'`, validated through `keys.ParseButton` /
`keys.ParseMods` and exposed as a `keys.Mouse`. `DragAction.Via []DragEnd 'via'`, each validated
like `to` under `drag.via[i]`. An offset (in `via` or `to`) counts from the previous point — the
press cell when there is no `via`, so today's boards keep their meaning. `internal/clip/clip.go`
collects `drag.via[i].area` beside `drag.from.area`/`drag.to.area`. README actions table: `click`
and `drag` rows plus one example with `via`, `button` and `with`.
**Regression guard.** Every prose site saying an offset counts from the press cell or that click/drag use the left button
(`grep -rn -i 'press cell\|left button\|left click' README.md internal/storyboard`) is reworded; the README bullet list gains `via`
entries. `DragEnd.validate` takes its field name (`to` / `via[i]`), the `drag.to:` wordings byte-identical (`cmd/photon/lint_test.go`,
`TestLoadRejects`). The via-area case extends `internal/clip/testdata/undeclared/demo.yaml` (and its header tally) and the pinned list.
**Files:** internal/storyboard/beat.go, internal/storyboard/validate.go, internal/storyboard/storyboard_test.go, internal/storyboard/testdata/every-form.yaml, internal/clip/clip.go, internal/clip/clip_test.go, internal/clip/testdata/undeclared/demo.yaml, README.md
**Read first:** internal/storyboard/beat.go — ClickAction, DragEnd; internal/storyboard/validate.go — DragAction.validate, DragEnd.validate;
internal/storyboard/storyboard_test.go — TestLoadEveryForm (compares ClickAction with !=, DragAction with DeepEqual); internal/clip/clip.go — namedAreas;
internal/clip/clip_test.go — TestLoad_UndeclaredAreaFailsNamingIt; cmd/photon/lint_test.go — badDragStoryboard case
**Tests:** load and refusal cases for each field, a bad via entry filed under `drag.via[i]`; clip refuses an undeclared area in a via entry; the `drag.to:` messages unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/ && GOMEMLIMIT=2GiB go test ./internal/clip/ && GOMEMLIMIT=2GiB go test ./cmd/photon/ -run README`
**Commit:** `feat(storyboard): name a mouse button, modifiers and drag via points`

## 6. Perform clicks and drags with buttons, modifiers and via points — ✅ DONE (2026-11-06)

NOTES (2026-11-06): consequential edit — internal/engine/doc.go: made necessary by click and drag now sending any button with modifiers and dragging through via points (the package comment described left-only, straight drags)
NOTES (2026-11-06): consequential edit — internal/engine/engine_test.go: made necessary by sendClick taking the click's keys.Mouse (added a middle-button + shift click case to TestEngine_Run_ActionForms, where the click cases live; there is no action_test.go)
NOTES (2026-11-06): the multi-segment route is a new pure function dragRoute (per-segment dragPath, steps numbered across segments, last move of each non-final segment marked isVia on dragMove); "dragPath over two segments" is tested as TestDragRoute. A via point that is a target logs a target event with Text `via` (engine constant dragViaText) beside `from`/`to`; take.MoveVia (= "via") marks the move only
NOTES (2026-11-06): fixture TUI status suffix for a non-plain drag is " with <button>[+shift][+alt][+ctrl]" (e.g. "with right+ctrl"); any button's press starts a gesture, only a left press selects; wheel reports (bit 64) and button bits 3 are ignored. Fixture beats gain beat 5 "drag via" (right+ctrl, one via offset); target-event count pinned 3 -> 4
NOTES (2026-11-06): internal/compose/doc.go:26 ("from the press cell to the drag's final move") is left for item 7, whose guard grep names it

**What:** Depends on item 5.
**Goal:** the engine sends each click and drag with its declared button and modifiers and drags through every via point, `steps` eased moves per segment; the fixture TUI reports what it decoded.
**Approach (assumed at the header base):** `internal/engine/drag.go`: resolve from, each via, and
to on the one start snapshot (offsets from the previous point, `clampToScreen` each), log each
target as today; `sendDrag` takes the point list and `keys.Mouse`, runs `dragPath` per segment,
and logs the last sent move of every non-final segment with `EventMove` whose `Text` is `via`
(no new `Event` field; take `FormatVersion` stays 1). `sendClick` uses the click's `keys.Mouse`.
Fixture TUI `mouse.go`: decode button
and modifier bits for any code and add them to the drag status line, keeping
today's line byte-identical for a plain left drag.
**Regression guard.** 6: add a documented constant for the via move text (e.g. `take.MoveVia = "via"`) in internal/take/event.go,
written by engine and read by item 7's compose (add internal/take/event.go to Files); item 6 also owns every non-README Go comment saying a click or drag uses the left button only or that a drag offset counts from the press cell — `grep -rniE 'left button|left-button|press cell' --include='*.go' internal/` — while item 5 owns README and internal/storyboard prose.
`keys.Click`/`Motion`/`Release` stay (item 4 pins their bytes; engine and its tests call them). The fixture cannot tell a via motion
from any other, so its status adds only button and modifiers; the via cell is asserted from the take's via move and the motion bytes
in FIXTURE_INPUT_LOG (`assertFixtureInput`). A segment that sends no move (zero length) logs no via move.
**Files:** internal/engine/drag.go, internal/engine/action.go, internal/engine/drag_test.go, internal/engine/fixture_test.go, internal/testkit/fixturetui/mouse.go, internal/testkit/fixturetui/doc.go, internal/keys/keys.go, internal/keys/doc.go, internal/take/event.go
**Read first:** internal/engine/drag.go — sendDrag, dragPath, logTarget; internal/engine/action.go — sendClick;
internal/engine/fixture_test.go — assertFixtureInput, assertFixtureEvents (pins 3 target events); internal/testkit/fixturetui/mouse.go — handleMouse;
internal/take/event.go — EventMove
**Tests:** `dragPath` over two segments; event log marks corners `via` (through the take constant); a zero-length via segment logs no via move; fixture drag with a via point, right button and `ctrl` reports button and modifiers, its via cell checked from the take and the input log; a plain left drag reports today's line.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/engine/ && GOMEMLIMIT=2GiB go test ./internal/keys/ && GOMEMLIMIT=2GiB go test ./internal/testkit/...`
**Commit:** `feat(engine): drag through via points with any button and modifiers`

## 7. Draw the cursor along every drag segment — ✅ DONE (2026-11-06)

**What:** Depends on item 6.
**Goal:** the drawn cursor passes through each via point, easing segment by segment at the drag's pace; a drag without via renders as today.
**Approach (assumed at the header base):** `internal/compose/cursor.go` `trackCursor`: when
collapsing a beat's consecutive `markFollow` marks, keep every move whose event `Text` is `via`
as its own mark; `cursorTrack.position` already eases between kept marks. README cursor prose
names the via corners.
**Regression guard.** 7: compose reads the via marker through the take constant item 6 adds, never a string literal.
Every comment or README line saying the drag's cursor eases from the press to the final move
(`grep -rn -i "final move\|last move\|press cell" internal/compose README.md`) is reworded to name the via corners.
**Files:** internal/compose/cursor.go, internal/compose/cursor_test.go, internal/compose/doc.go, README.md
**Read first:** internal/compose/cursor.go — trackCursor, clickMark, markKindOf, cursorTrack.position; internal/compose/cursor_test.go — dragAt, TestCursorFollowsADrag;
internal/compose/doc.go — cursor paragraph; README.md — "Sections and cursor"
**Tests:** cursor sits on the via point at its mark's time and between marks stays on that segment; existing straight-drag cursor tests unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/compose/ && GOMEMLIMIT=2GiB go test ./cmd/photon/ -run README`
**Commit:** `feat(compose): draw the cursor through a drag's via points`
**Closes:** drag-action-options

## 8. List channel directories and the account's open pull requests — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the viewer query's answer shape is a named type (`viewerOpenPullsAnswer`) and the directory entry type a constant (`contentsDirType`), where the source inlined both; the source's `wingetPull`/`wingetViewerPull` test helpers are ported as `searchedPullsAnswer`/`viewerPullsAnswer` in channelrepos_test.go with literal repo strings, so no winget identifier lands before item 9/10.

**What:**
**Goal:** `channelRepos` can list a directory of a channel repo and the signed-in account's open pull requests against it, in the gh-backed and in-memory implementations.
**Approach (assumed at the header base):** port from the release-tool source at `81cd62d`:
`channelRepos.ListDirs` and `OpenPulls`, `pullRequest`, the gh query (author `@me` only), into
`channelrepos.go` beside `Exists`/`ReadFile`/`PublishFile`; matching state and failure hooks
in `memChannelRepos`. Rename every identifier, comment and temp prefix to photon.
**Regression guard.** 8: item 8 defines no winget constants — its tests use literal repo and dir strings — and it updates the "the tap and the bucket" prose in cmd/release/doc.go and cmd/release/runner.go to name channel repos generally (add both to Files).
`pullRequest` has no author and the in-memory `OpenPulls` returns every pull, so the author test is: the `gh pr list` argv carries
`--author @me`, the second call is the viewer GraphQL query, and a viewer pull whose `nameWithOwner` is another repo is dropped
(`pullsAnswering` ported with it).
**Files:** cmd/release/channelrepos.go, cmd/release/channelrepos_test.go, cmd/release/memchannels_test.go, cmd/release/doc.go, cmd/release/runner.go
**Read first:** cmd/release/channelrepos.go — channelRepos, execChannelRepos.gh, isNotFound; cmd/release/channelrepos_test.go — TestExecChannelReposRunsEachMethodsArgv,
TestExecChannelReposTellsANoFromABrokenCommand; cmd/release/memchannels_test.go — memChannelRepos, withFailure; cmd/release/repo_test.go — answering
**Tests:** gh argv and parsing for both methods; the search argv carries `--author @me`; a viewer pull on another repo is dropped.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/release/`
**Commit:** `feat(release): list channel directories and own open pull requests`

## 9. Render the winget manifests — ✅ DONE (2026-11-06)

NOTES (2026-11-06): the default-locale template constant is `wingetLocaleTemplate` (file `photon.locale.en-US.yaml.tmpl`, as the plan's Files line names it); the installer's `Dependencies: PackageDependencies: - PackageIdentifier: Gyan.FFmpeg` sits at the manifest root, after NestedInstallerType, so it applies to both architectures.
NOTES (2026-11-06): beyond the golden test, added TestRenderWingetInstallerNamesTheArchivesAndFFmpeg (field-level check of both installers, upper-cased hashes, alias and the FFmpeg dependency), winget rows in TestRenderRefusesABadOrMissingHash/TestRenderRefusesABadVersion and TestPackageDataReleaseURL; render_test.go now imports gopkg.in/yaml.v3 (already in go.mod) to check each manifest parses — test-only, so the tool's non-test imports stay stdlib and cobra.

**What:**
**Goal:** the release tool renders the three winget manifests for a version (portable zip, x64 and arm64, `photon.exe` alias `photon`, `Gyan.FFmpeg` dependency), matching committed goldens.
**Approach (assumed at the header base):** port the reference's three manifest templates into
`cmd/release/templates/winget/` and widen `render.go`'s embed/parse to them; add
`packageArchive.UpperSHA256`, `packageData.ReleaseURL`, `WingetPackageID` (`AiricLenz.Photon`),
`renderedFile`, `releasePageBase`. ShortDescription is `cmd/photon`'s root `Short`, read at item
time. Installer adds `Dependencies: PackageDependencies: - PackageIdentifier: Gyan.FFmpeg`.
**Regression guard.** 9: item 9 owns, in render.go, wingetPackageID, the three winget template-name constants, releasePageBase, renderedFile, packageArchive.UpperSHA256 and packageData.ReleaseURL/WingetPackageID
9: item 9 also owns renderWingetManifests, wingetManifest and wingetManifests in render.go, so its goldens call them; item 10's winget.go calls them and defines none of them
**Files:** cmd/release/render.go, cmd/release/render_test.go, cmd/release/templates/winget/photon.version.yaml.tmpl, cmd/release/templates/winget/photon.installer.yaml.tmpl, cmd/release/templates/winget/photon.locale.en-US.yaml.tmpl, cmd/release/testdata/winget/AiricLenz.Photon.yaml, cmd/release/testdata/winget/AiricLenz.Photon.installer.yaml, cmd/release/testdata/winget/AiricLenz.Photon.locale.en-US.yaml
**Read first:** cmd/release/render.go — packageTemplates, templateFiles, packageData, renderPackageFile, releaseDownloadBase;
cmd/release/render_test.go — goldenSums, TestRenderRefusesABadOrMissingHash; cmd/photon/root.go — Short
**Tests:** golden comparison of all three manifests rendered through renderWingetManifests; hashes upper-cased from a fixture SHA256SUMS.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/release/ -run Render`
**Commit:** `feat(release): render winget manifests`

## 10. Publish to winget — ✅ DONE (2026-11-06)

NOTES (2026-11-06): ported the probe's setup-pending branch for an unmerged first submission of another version (the reference behaviour, not named on the item's Goal line), and kept resolveWingetPackage's refusal when its listing read fails (TestPublishWingetRefusesAnUnreadablePackageListing).
NOTES (2026-11-06): a missing komac (exec.ErrNotFound) or no token is a setupPendingError, where the reference failed the step; any other `komac --version` failure still fails it. newPublishFake holds a done winget (an open pull request for the version), like releasedFake (a published manifest dir), so pre-existing publish tests run no komac; the winget tests use wingetPendingFake.
NOTES (2026-11-06): the publish verb's Long help names the winget step and its setup-pending cases (its last lines re-wrapped to keep the 100-column limit); the `status` text table pin gains a winget row.
NOTES (2026-11-06): consequential edit — cmd/release/runner.go: made necessary by winget.go running komac and `gh auth token` through the runner, which the runner's comment listed the commands of.
NOTES (2026-11-06): a first submission's manifest directory is made with os.MkdirTemp("", "photon-winget-") as in the reference, so TestPublishWingetSubmitFailureIsSetupPending leaves it under the system temp dir until its t.Cleanup removes it, not under t.TempDir().

**What:** Depends on items 8 and 9.
**Goal:** `status`, `publish` and `smoke` carry a `winget` step after `scoop`: done when the version's manifest dir is listed or the account's open PR names it, otherwise converged through komac; a missing komac or token reports setup-pending.
**Approach (assumed at the header base):** port the reference's `winget.go` (`probeWinget`,
`convergeWinget`, `komacArgs`, `submitWingetManifests`, `githubToken` from `GITHUB_TOKEN` else
`gh auth token`, token by env only and scrubbed from stderr, `resolveWingetPackage`,
`wingetSubmittedDetail`); restore `publishOptions.getenv`; add `stepWinget` to `publishSteps`
with no smoke check; add `winget.go` to `cmd/release/doc.go`.
**Regression guard.** 10: item 10's winget.go owns wingetRepo, wingetManifestDir and every probe/converge/komac/token symbol, and its Files add cmd/release/probes_test.go and cmd/release/status_test.go for the step-list pins (TestProbeStepsRunsEveryStepInPublishOrder, the status wantStates and step tables).
`releasedFake` (channels_test.go) holds a done winget; `failingChannels` also fails `wingetRepo` on ListDirs/OpenPulls. `convergeWinget`
wraps missing komac (`exec.ErrNotFound`) and no token in `setupPendingError` (smoke still runs); `mutatingCommands` gains `komac update`
and `komac submit`; `runTestPublishWith`/`channelRun` default `getenv` to an empty env. A failed winget probe refuses publish — accepted.
**Files:** cmd/release/winget.go, cmd/release/winget_test.go, cmd/release/steps.go, cmd/release/publish.go, cmd/release/publish_test.go, cmd/release/channels_test.go, cmd/release/probes_test.go, cmd/release/status_test.go, cmd/release/doc.go
**Read first:** cmd/release/steps.go — publishSteps; cmd/release/publish.go — runPublish, publishOptions; cmd/release/channels.go — setupPendingError;
cmd/release/publish_test.go — mutatingCommands, runTestPublishWith; cmd/release/channels_test.go — releasedFake; cmd/release/probes_test.go — failingChannels
**Tests:** probe states (listed, own open PR, none); converge update vs first submit; missing komac or token reports a setup-pending winget row and smoke still runs; token never in argv or output; step order in `status --json`; step-list pins and `TestProbesNeverReadAFailedCallAsDone` cover winget.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/release/`
**Commit:** `feat(release): publish releases to winget`

## 11. Build the npm packages — ✅ DONE (2026-11-06)

NOTES (2026-11-06): platform packages are named with npm's own os/cpu values, `@airiclenz/photon-<linux|darwin|win32>-<x64|arm64>` (the plan fixes only the `<os>-<arch>` shape), so each name matches its os/cpu fields and the launcher's `process.platform`-`process.arch` key; each stages in outDir/<name without scope> (wrapper in outDir/photon).
NOTES (2026-11-06): the API item 12 calls is `stageNpmPackages(version, archiveDir, sums, outDir) ([]npmPackage, error)` — npmPackage{name, dir} in publish order, six platform packages then the wrapper; it verifies all six archives against SHA256SUMS before writing anything (sentinel errNpmArchiveMismatch; errNoChecksum/errMalformedChecksum from checksumFor).
NOTES (2026-11-06): extraction reuses smoke.go's existing extractTarGzEntry/extractZipEntry (missing entry → errNoEntry) instead of a second copy; npm.go only dispatches by archive kind. smoke.go is unchanged.
NOTES (2026-11-06): the test fixture release is built by running dist with the existing fake toolchain (runTestDist/fakeToolchain), so the archives have the exact shape dist ships; binaries are compared to stubBinary, the text files to goldens under testdata/npm/. The launcher was also exercised by hand under node with a fake platform package (passes args, exits with the child's status, names the missing package).

**What:**
**Goal:** the release tool stages the wrapper and six platform npm packages for a version from the release archives, each archive checked against SHA256SUMS.
**Approach (assumed at the header base):** new `cmd/release/npm.go` (stdlib only): templates
under `cmd/release/templates/npm/` for the wrapper `package.json` (bin `photon` →
`bin/photon.js`, optionalDependencies on all six at the same version), the launcher
`bin/photon.js` (maps `process.platform`/`process.arch` darwin|linux|win32 × x64|arm64 to the
platform package, `spawnSync` with inherited stdio, exits with the child's status, names the
missing package on failure) and the platform `package.json` (`os`, `cpu`, files `bin/`).
Platform dirs get the binary extracted from the matching `.tar.gz`/`.zip` (mode 0755). Version
comes from the release being published.
**Regression guard.** 11: staging is a pure function over a directory of the six archives plus SHA256SUMS bytes — it never builds or downloads; item 12 supplies them.
`-run` is case-sensitive and the package spells initialisms upper (`fileSHA256`), so every new test name contains `Npm`
(never `NPM`) and the Acceptance's `-run 'Npm|Render'` selects them.
**Files:** cmd/release/npm.go, cmd/release/npm_test.go, cmd/release/render.go, cmd/release/templates/npm/package.json.tmpl, cmd/release/templates/npm/photon.js.tmpl, cmd/release/templates/npm/platform.package.json.tmpl, cmd/release/doc.go
**Read first:** cmd/release/render.go — templateFiles, packageTemplates, checksumFor; cmd/release/dist.go — distTargets, archiveName, fileSHA256;
cmd/release/githubrelease.go — checkUploadedArchives; cmd/release/doc.go — file map
**Tests:** staged trees for a fixture version from a fixture directory of the six archives and SHA256SUMS bytes (goldens under `cmd/release/testdata/npm/`); extraction from fake tar.gz and zip; a hash mismatch refuses; every test name contains `Npm`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/release/ -run 'Npm|Render'`
**Commit:** `feat(release): build the npm packages`

## 12. Publish to npm

**What:** Recast at the regression check (2026-11-06). Depends on items 10 and 11.
**Goal:** an `npm` step after `winget` publishes missing platform packages then the wrapper; smoke installs the wrapper into a temp prefix and runs `photon version` (through checkReportsVersion); without npm or `NPM_TOKEN` it reports setup-pending.
**Approach (assumed at the header base):** in `npm.go`: probe each package with `npm view
<pkg>@<version> version` through the channelRepos seam (wrapper first; see the guard); converge runs `npm
publish <dir> --access public` with a temp userconfig holding
`//registry.npmjs.org/:_authToken=${NPM_TOKEN}`, the token passed by env only; smoke runs `npm
install --prefix <tmp> @airiclenz/photon@<version>` then `<tmp>/node_modules/.bin/photon
version` through `checkReportsVersion`, skipping with a note when npm is absent. Add `stepNpm`.
**Regression guard.** 12: (a) registry reads and publishes go through the channelRepos seam per doc.go/runner.go (the documented rule the draft broke): new channelRepos methods for "is <pkg>@<version> on the registry" and "publish this package dir", with the npm exec implementation in channelrepos.go and state plus failure hooks in memChannelRepos — the runner is not used for the probe or publish; (b) probe: npm not found (exec.ErrNotFound) → setup-pending; E404 or empty `npm view` output → pending; any other failure → unknown; NPM_TOKEN is read through publishOptions.getenv only in converge, unset → setupPendingError, never os.Getenv in a probe; (c) converge downloads the six archives with repo.DownloadAsset into a temp dir and checks them against SHA256SUMS as checkUploadedArchives does, then calls item 11's staging; (d) smoke skips (as checkScoop skips an unreadable manifest) when the wrapper version is not on the registry, and fails only when it is published and does not report the version; on Windows it runs `node_modules/.bin/photon.cmd`; (e) Files add cmd/release/channelrepos.go, cmd/release/memchannels_test.go, cmd/release/probes_test.go, cmd/release/status_test.go
The item yields to the documented rule in `cmd/release/doc.go` (seam paragraph) and `cmd/release/runner.go` (package comment):
channel-repo reads go through channelRepos, and the runner's fakes answer only toolchain, brew and released binaries.
(f) convergeNpm reads NPM_TOKEN through publishOptions.getenv first and returns setupPendingError when it is unset, before any temp dir, download or staging (releasedFake holds content only for SHA256SUMS);
(g) newSmokeFake publishes all seven packages and respondOn answers `npm install --prefix`, writing a host stub at node_modules/.bin/photon, so every TestSmokeChecks tally moves by one pass, stated once;
(h) failingChannels also fails the new registry-read method on all seven package names with the same non-ErrNotFound error; (i) a failed npm probe (e.g. `npm view` ENOTFOUND/ETIMEDOUT) reads unknown and refuses publish — accepted;
(j) smoke runs `photon version` through checkReportsVersion (versionArgument), never `--version`.
12: the probe checks the wrapper `@airiclenz/photon` first and reports done as soon as it is on the registry (the wrapper is published last), probing the six platform packages only when the wrapper is absent
**Files:** cmd/release/npm.go, cmd/release/npm_test.go, cmd/release/channelrepos.go, cmd/release/memchannels_test.go, cmd/release/steps.go, cmd/release/smoke_test.go, cmd/release/publish_test.go, cmd/release/probes_test.go, cmd/release/status_test.go
**Read first:** cmd/release/steps.go — publishSteps; cmd/release/channelrepos.go — channelRepos, execChannelRepos; cmd/release/memchannels_test.go — memChannelRepos;
cmd/release/smoke.go — checkReportsVersion; cmd/release/smoke_test.go — smokeFake.respondOn; cmd/release/probes_test.go — failingChannels; cmd/release/channels_test.go — releasedFake
**Tests:** partial publish resumes with only the missing packages; wrapper published last; token never in argv or output; probe reads setup-pending without npm, pending on E404 or empty output, unknown on any other failure (offline `npm view` refuses publish); the probe reads done from the wrapper alone and probes the six platform packages only when the wrapper is absent; unset `NPM_TOKEN` is setup-pending before any download; `failingChannels` fails the registry read so `TestProbesNeverReadAFailedCallAsDone` covers npm; converge refuses a downloaded archive that mismatches SHA256SUMS; smoke pass (`photon version`), fail, skip when npm is absent and skip when the wrapper version is unpublished; every `TestSmokeChecks` tally moves by one pass; step-list pins carry `npm`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/release/`
**Commit:** `feat(release): publish releases to npm`

## 13. Document the winget and npm channels

**What:** Depends on items 10 and 12.
**Goal:** every place naming photon's release channels names all five; `docs/releasing.md` covers winget and npm prerequisites, one-time setup and resume notes; README shows `winget install AiricLenz.Photon` and `npm install -g @airiclenz/photon`.
**Approach (assumed at the header base):** `docs/releasing.md`: channel table and count;
prerequisites komac, `GITHUB_TOKEN`, npm, `NPM_TOKEN` (automation token, `@airiclenz` scope);
one-time setup; "submitted" vs installable after the winget-pkgs merge; ffmpeg not declared by
npm. Prose guard: every line naming the channel set —
`grep -rniE 'three channels|homebrew,? and scoop|tap (and|or) (the )?bucket' --include='*.md' --include='*.go' --include='*.yml' .`
**Regression guard.** The rule is every line naming the channel set, in any wording (a line about one channel alone stays); find it with
`grep -rniE 'homebrew|scoop' --include='*.md' --include='*.go' --include='*.yml' . | grep -vE 'cmd/release/(homebrew|scoop)|docs/plans/|\.capcom/|CHANGELOG'`.
The Files list is whatever the rule finds — at the base it also hits the help and comments in publish.go, steps.go, doc.go and smoke.go
(e.g. status.go's step list, main.go's root Long, smoke.go's Long, docs/releasing.md's table, README's Install).
**Files:** docs/releasing.md, README.md, .agents/skills/release-photon/SKILL.md, .claude/skills/release-photon/SKILL.md, cmd/release/main.go, cmd/release/status.go, cmd/release/runner.go, cmd/release/publish.go, cmd/release/steps.go, cmd/release/doc.go, cmd/release/smoke.go, .github/workflows/tag-on-version-bump.yml
**Read first:** docs/releasing.md — channel table, prerequisites, one-time setup; cmd/release/publish.go — newPublishCommand Long;
cmd/release/smoke.go — newSmokeCommand Long; cmd/release/status.go — newStatusCommand Long; cmd/release/main.go — root Long; README.md — Install
**Tests:** none beyond the existing help-text and README tests.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/release/ && GOMEMLIMIT=2GiB go test ./cmd/photon/ -run README`
**Commit:** `docs(release): document the winget and npm channels`
**Closes:** winget-release-channel; npm-release-channel
