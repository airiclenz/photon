# Camera moves, captions and the tune frame values — implementation plan

**Goal:** Give a clip a camera of two independent, overlapping tracks (zoom and pan) placed on
the clip's timeline, with a beat's `zoom` kept as a shorthand; let a storyboard declare captions
drawn over the composed frame; show the read-only frame values in the `photon tune` inspector.
**Date:** 2026-10-17
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** a55b7d1
**Closes:** (per item)

**Sources:**
- `.capcom/issues/camera-moves.md`, `.capcom/issues/clip-captions.md`,
  `.capcom/issues/tune-frame-readonly-values.md`
- Notes `package-layout`, `glossary`, `coding-conventions`; `README.md` (Storyboard reference,
  "Sections, zoom and cursor", "Fonts and licences")

**Ratified design calls (user, 2026-10-17):**
- **Clip clock:** `at` is `{beat: N, offset: D}` (the beat's section start on the output clock plus D, negative allowed, offset default 0) or a bare clip time (`at: 4500ms`); `over`/`for` are output-clock lengths.
- **Pan screen:** a pan's points resolve on the screen shown at the move's end, walking back to the move's start when absent there.
- **Overlap:** two moves on one track that overlap are a lint error; a beat's `zoom` occupies both tracks for its section.
- **Location:** top-level `camera:` and `captions:` lists in the storyboard.
- **Via curve:** centripetal Catmull-Rom through start, waypoints and end, arc-length parameterized, one ease over the whole path; `ease: smooth|linear`, default smooth.
- **Edges:** the pan track keeps its true centre; the crop is clamped inside the frame per magnification, as today.
- **Caption font:** embed Source Sans 3 Regular and Bold (OFL) as the caption default; the config may name font files instead.
- **Caption end:** exactly one of `for: <length>` and `until: <moment>`.
- **Tune:** `photon tune` shows camera moves and captions on its timeline and edits their timing and main fields (items 14-18).

**Writer's calls (2026-10-17, binding):** a cell point is `cell: {x, y}` (0-based column, row);
caption YAML keys use `color` as `max_colors` does; caption placement words are
`place: top|middle|bottom` and `align: left|middle|right`; fades are linear in opacity.

**Standing requirements:**
- skills: coding-standards
- Each new file is added to its package's `doc.go` map where the package has one.
- Forbidden names per `AGENTS.local.md` never land in a file.

**Out of scope:**
- In `photon tune`: adding, removing or reordering camera moves or captions; editing a pan's `via`, a caption's font, colours, box, fade, offset or position (edit the YAML).
- Camera moves inside a cut beat; per-character caption styling; caption text wrapping.

**Regression check (2026-10-17, a55b7d1):**
- 2: guard folded (writer's decision)
- 4: guard folded
- 5: guard folded
- 6: guard folded
- 8: guard folded (writer's decision)
- 9: guard folded (writer's decision on Acceptance)
- 10: guard folded
- 11: guard folded
- 12: guard folded; supersedes internal/compose/doc.go's imports sentence (item 13 rewrites it)
- 13: guard folded (writer's decision)
- 14: guard folded
- 15: guard folded (writer's decision on Replace)
- 16: guard folded (writer's decision)
- 17: guard folded; supersedes the internal/tune/page/timeline.js lane-model header
- 18: guard folded (writer's decision on Acceptance)

## 1. Tune inspector shows the read-only frame values — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the page test did not pin the panel's text, so instead of extending an existing assertion it gains TestPage_App_ReadOnlyFrameFields_AreStateKeys, which checks that app.js's READ_ONLY_FRAME_FIELDS lists the seven keys and that /api/state sends each of them; gofmt realigned the existing State fields and the State literal around the seven new ones

**Closes:** tune-frame-readonly-values
**What:**
**Goal:** `/api/state` reports the frame's `cols`, `rows`, `padding`, `font_size`, `line_height`,
`scale` and `max_colors`, and the inspector's Clip frame panel prints each name with its value,
read-only.
**Approach (assumed at the header base):** add flat fields to `tune.State` beside `FPS` and
`Width` (json keys exactly the storyboard's YAML keys), filled where `State` is built from
`board.Frame`. In `internal/tune/page/app.js` replace the `READ_ONLY_FRAME_FIELDS` sentence with
one read-only row per field (`name value`), keeping the existing `read-only` class.
**Files:** internal/tune/state.go, internal/tune/session_test.go, internal/tune/page/app.js, internal/tune/page_test.go
**Read first:** internal/tune/state.go — State, (*Session).State; internal/tune/page/app.js — framePanel,
READ_ONLY_FRAME_FIELDS, fieldRow; internal/tune/session_test.go — sessionStoryboard, newSessionFixture;
internal/storyboard/storyboard.go — Frame; internal/tune/server_test.go — decodeResponse
**Tests:** a session test asserting the seven values from the test storyboard's frame; extend the
page test if it pins the panel's text.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `grep -n "max_colors" internal/tune/state.go internal/tune/page/app.js`
**Commit:** `fix(tune): report and show the read-only frame values`

## 2. Storyboard clip clock: moments on the output timeline — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the mapping form requires `beat` ≥1 (`{offset: 1s}` and `{beat: 0}` are decode errors), so beat 0 only ever comes from the bare clip-time scalar; the doc.go package comment's yaml.v3 sentence also names Moment's two forms
NOTES (2026-10-17): golangci-lint is not installed on this host; gofmt and go vet are clean

**What:**
**Goal:** `storyboard` has a `Moment` type that decodes from a duration scalar (`4500ms`) or a
mapping `{beat: N, offset: D}`, and a `Storyboard` method that turns a Moment into an output-clock
time; a `Length` method gives the clip's length; both agree with `schedule.Schedule`.
**Approach (assumed at the header base):** new file `internal/storyboard/moment.go`: `Moment`
with a custom `UnmarshalYAML` (scalar parsed by `time.ParseDuration`; mapping strict-decoded).
`(*Storyboard).ClipTime(Moment) (time.Duration, error)` sums the durations of the kept beats
before beat N in storyboard order and adds the offset; errors for an unknown beat or a cut beat.
`(*Storyboard).Length()` sums the kept beats' durations. The rig runs beats in storyboard order,
so this equals `schedule.Section.OutStart`. No validation wiring here; items 4 and 10 use it.
**Regression guard.** `Moment` has exported fields `Beat int` and `Offset time.Duration` so Go code (compose and tune tests) can build one; Beat 0 means a bare clip time whose value is Offset (beat ids are ≥1 per validateIDs); its UnmarshalYAML rejects an unknown mapping key itself (yaml.v3 Node.Decode is non-strict), with a test for `{beat: 2, ofset: 1s}`.
**Files:** internal/storyboard/moment.go, internal/storyboard/moment_test.go, internal/storyboard/doc.go, internal/schedule/schedule_test.go
**Read first:** internal/storyboard/beat.go — Nth.UnmarshalYAML, nodeKindName, Beat; internal/storyboard/storyboard.go — Storyboard,
Parse (KnownFields), applyDefaults; internal/schedule/schedule.go — New, Section.OutStart, Schedule.Length; internal/schedule/schedule_test.go —
recording, mustSchedule, beat; internal/storyboard/validate.go — validateIDs; internal/storyboard/doc.go — file map
**Tests:** table tests for both decode forms, negative offset, unknown and cut beat, a bad scalar,
an unknown mapping key (`{beat: 2, ofset: 1s}`) rejected;
a schedule test asserting every section's `OutStart` equals `ClipTime({beat: id})` and
`Schedule.Length` equals `Length()`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/ ./internal/schedule/`
**Commit:** `feat(storyboard): add moments on the clip's output clock`

## 3. Golden of a beat zoom's crops — ✅ DONE (2026-10-17)

NOTES (2026-10-17): each golden line leads with the frame's output time in milliseconds (`<ms> X Y W H`, the rect at `%.6f`) so a mismatch names its frame; the time is compared exactly, the rect to 1e-6.
NOTES (2026-10-17): the board has two idle beats around the two zoomed ones (beat 2: explicit 2×, 5 s of ramps shrunk into its 3 s section; beat 4: auto-fit 1.59× on a snapshot-resolved target at columns 30-47, clamped against the right edge), so the crop returns to the full frame between and after the zooms.

**What:** test-only. Pin today's beat-zoom framing before the camera replaces its mechanism.
Add `internal/compose/zoom_golden_test.go`: a board with two zoomed beats (one explicit factor,
one auto-fit, one with ramps shrunk to fit its section, one target near the frame's right edge so
the crop clamps), composed with the existing test helpers (`newTestCompositor`, `screen`,
`clickAt`). It samples `Compositor.crop` at every frame at 25 fps across the clip and compares
each rect, to 1e-6, against `internal/compose/testdata/beat-zoom-crops.golden` (one line per
frame, `%.6f`). An `-update` flag rewrites it, as `internal/raster`'s tests do.
**Files:** internal/compose/zoom_golden_test.go, internal/compose/testdata/beat-zoom-crops.golden
**Read first:** internal/compose/compose.go — (*Compositor).crop, rectF; internal/compose/zoom.go — zoomPlanner.planSection,
zoomPlan.factorAt, autoFitFactor, zoomBox; internal/compose/compose_test.go — newTestCompositor, screen, clickAt, beatStart,
zoomClickTake, testLayout; internal/raster/raster_test.go — update flag
**Tests:** the golden test itself; it passes at the base.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/ -run Golden`
**Commit:** `test(compose): pin a beat zoom's crops in a golden`

## 4. Storyboard `camera`: schema and validation — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the camera's rules live in camera.go beside its types (validate.go only calls `validateCamera`), so doc.go's validate.go entry now reads "every other validation rule"; overlaps are checked pairwise (each move against every earlier move on its track and every kept beat's zoom section), which reports the same problems as a sorted sweep and names both holders
NOTES (2026-10-17): an absent `at` is the clip's start (Moment's zero value) and an absent `ease` is set to `smooth` by applyDefaults; the lint fixture uses `over: 0s` because yaml.v3 refuses a unitless `0` for a duration; both lint cases pass `--config` validConfig
NOTES (2026-10-17): the README gains one `camera` row in the Storyboard Header table naming every new tag; item 8 writes the full reference

Depends on item 2.
**What:**
**Goal:** a storyboard may carry a top-level `camera:` list; `Parse` rejects every bad move and
`photon lint` reports the problems.
**Approach (assumed at the header base):** new file `internal/storyboard/camera.go`. A move has
exactly one of `zoom: {to, at, over, ease}` and `pan: {to | via, at, over, ease}`. `to` for a zoom
is a factor in [1, 3] (reuse `minZoomFactor`/`maxZoomFactor`). A pan point is a target (inline
`text`, `nth`, `area`) or `cell: {x, y}`, exactly one; `via` holds ≥ 2 points, the last being the
destination; exactly one of `to` and `via`. `ease` is `smooth` (default) or `linear`. Validation
(problems under `camera[i]: …`, through the existing `reporter`): `over` > 0; `at` resolves
(item 2); start in [0, Length) and start+over ≤ Length; a cell inside `frame.cols`×`frame.rows`;
target regex and area spelling as `Target.validate` does; per track, sorted intervals
[start, start+over) must not overlap each other nor any kept beat's `zoom` section (which occupies
both tracks), the problem naming both. Extend `clip.namedAreas` with `camera[i]: pan.…area` fields.
Add a camera to `testdata/every-form.yaml`.
**Regression guard.** Adding `Camera` turns TestREADMEDocumentsEveryField/storyboard red, so this item adds every new
yaml tag to README.md's Storyboard reference in backticks (`camera`, `pan`, `via`, `at`, `over`, `ease`, `cell`, and `beat` if
Moment carries it), item 8 writing the reference around them, and runs `README` in cmd/photon. `clip.Load` cross-checks areas only
when the storyboard loads clean, so the lint case is two fixtures: `invalid/camera.yaml` and `invalid/camera-area.yaml`.
**Files:** internal/storyboard/camera.go, internal/storyboard/storyboard.go, internal/storyboard/validate.go, internal/storyboard/storyboard_test.go, internal/storyboard/testdata/every-form.yaml, internal/storyboard/doc.go, internal/clip/clip.go, internal/clip/clip_test.go, cmd/photon/lint_test.go, cmd/photon/testdata/invalid/camera.yaml, cmd/photon/testdata/invalid/camera-area.yaml, README.md
**Read first:** internal/storyboard/validate.go — validate, reporter, Target.validate, validateArea, minZoomFactor; internal/storyboard/storyboard.go —
Storyboard, Parse, applyDefaults; internal/clip/clip.go — namedAreas, crossCheck, Load; internal/storyboard/storyboard_test.go — validBase, TestLoadRejects,
TestLoadEveryForm; internal/storyboard/testdata/every-form.yaml (beats 2-3 zoom [2s,12s), so any camera move must sit in beat 1's [0,2s)); cmd/photon/lint_test.go — TestLint, runLint; cmd/photon/readme_test.go — TestREADMEDocumentsEveryField, yamlTags
**Tests:** table tests per rule; a clip test for an undeclared camera area; two `photon lint` cases:
`invalid/camera.yaml` rejecting `over: 0`, `to: 3.5`, a start past the clip, and two overlapping zooms, and
`invalid/camera-area.yaml` (with `--config` validConfig) rejecting an unknown area; the README test passes.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/ ./internal/clip/ ./cmd/photon/ -run 'Lint|Camera|Parse|Load|README'`
**Commit:** `feat(storyboard): declare and validate camera moves`

## 5. Compose: a two-track camera behind the crop — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the two tracks share one generic `move[V]` type and one `trackAt` evaluator (the zoom track blends in log space, the pan track with `lerp`) rather than two separate segment types; TestZoomRampScalesGeometrically moved from compose_test.go to camera_test.go and now drives a camera's push and pull moves.

Depends on item 3.
**What:**
**Goal:** the compositor's crop comes from a camera of a zoom track and a pan track that each hold
their last state between moves; a beat's `zoom` is planned as moves on those tracks, and the item 3
golden passes unchanged.
**Approach (assumed at the header base):** new file `internal/compose/camera.go`: a `camera` with
two sorted segment lists, built once in `New` (the Compositor stays immutable and safe for
concurrent use). A zoom segment eases magnification from the track's previous value to its own in
log space (`exp(lerp(ln f0, ln f1, ease(p)))`, matching `zoomPlan.factorAt`'s geometric ramp); a
pan segment eases the centre along a straight line (item 6 adds curves); an instant segment
(`over` 0) sets a value. Defaults: factor 1, centre the frame's centre. A beat zoom becomes: an
instant pan to its box's centre at the section start, a zoom 1→factor over `In`, a zoom
factor→1 ending at the section end over `Out` (ramps shrunk as `planSection` does). `crop` reads
factor and centre at t and clamps as today. `zoomPlan.factorAt` and the `zooms` field go; the
box-finding (`zoomBox`, `loggedBox`, `autoFitFactor`) stays in `zoom.go`.
**Regression guard.** Every reader of `compositor.zooms` is rewritten against the camera, not only `factorAt` callers:
cursor_test.go's TestComposeZoomAimsAtADragsLoggedBox (reads `zooms[0].Centre`) and compose_test.go's
TestZoomRampScalesGeometrically, TestComposeZoomResolvesItsTargetOnTheSnapshot, TestComposeZoomBoxWalksBackThroughTheBeat and
TestZoomRampsShrinkToFitTheSection keep their coverage on the camera's tracks.
**Files:** internal/compose/camera.go, internal/compose/zoom.go, internal/compose/compose.go, internal/compose/compose_test.go, internal/compose/cursor_test.go, internal/compose/camera_test.go, internal/compose/doc.go
**Read first:** internal/compose/zoom.go — zoomPlan, factorAt, zoomPlanner.plan, planSection, zoomBox, easeInOut; internal/compose/compose.go — Compositor, New,
crop, Compose, Cell, Rect; internal/compose/compose_test.go — newTestCompositor, zoomClickTake, TestZoomRampScalesGeometrically, TestZoomRampsShrinkToFitTheSection,
TestComposeZoomResolvesItsTargetOnTheSnapshot; internal/compose/cursor_test.go — TestComposeZoomAimsAtADragsLoggedBox; internal/compose/doc.go — file map
**Tests:** camera unit tests: hold between moves, log-space zoom midpoint, pan midpoint; existing
zoom tests moved to the camera, each keeping what it asserts: TestZoomRampScalesGeometrically the log-space ramp,
TestComposeZoomResolvesItsTargetOnTheSnapshot the pan-track centre and auto-fit factor, TestComposeZoomBoxWalksBackThroughTheBeat
and TestComposeZoomAimsAtADragsLoggedBox the pan-track centre, TestZoomRampsShrinkToFitTheSection the zoom segments' 1.5 s and
2.5 s ramps; the item 3 golden unchanged.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/`
- `git diff --exit-code -- internal/compose/testdata/` (the golden is not rewritten)
**Commit:** `refactor(compose): drive the crop from a two-track camera`

## 6. Compose: a smooth pan path through waypoints — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the waypoints and ease ride on the shared `move[V]` as `Via []V` and `Ease storyboard.Ease` (empty means smooth), so `newCamera`'s signature and zoom.go's beat-zoom moves stay unchanged. `newCamera` turns each pan into a `panSegment` that holds its path, starting where the pan track rests or the previous pan ended. `trackAt` now takes any entry that exposes its `move` and applies the ease, which means a zoom move's `Ease: linear` also takes effect once item 7 sets it.

Depends on item 5.
**What:**
**Goal:** `compose` has a path type that, given a start point and ≥ 1 further points, returns the
position at eased progress e ∈ [0, 1] along a centripetal Catmull-Rom curve through all points,
by arc length, with no corner at a waypoint; a pan segment moves along it.
**Approach (assumed at the header base):** new file `internal/compose/path.go`: centripetal
Catmull-Rom (alpha 0.5), phantom end points by reflection (`2·P0−P1`, `2·Pn−Pn−1`), an
arc-length table of a named constant number of samples per span, built once; position at e is
the point at `e·total` length. Two points degenerate to the straight line. Pan segments in
`camera.go` hold a path instead of two endpoints; `ease` linear skips `easeInOut`.
**Regression guard.** A zero centripetal knot interval (a `to` or `via` point equal to the one before it, e.g. a pan to the
centre a beat zoom just set) divides 0/0 into a NaN crop: the path drops consecutive coincident points and holds still when one
distinct point remains. Pan segments change from two endpoints to a path, so `camera_test.go` changes with them.
**Files:** internal/compose/path.go, internal/compose/path_test.go, internal/compose/camera.go, internal/compose/camera_test.go, internal/compose/doc.go
**Read first:** internal/compose/camera.go — pan segment (item 5); internal/compose/zoom.go — easeInOut, clamp01;
internal/compose/cursor.go — lerp; internal/compose/compose.go — pointF; internal/compose/doc.go — file map
**Tests:** endpoints exact; passes through each waypoint (within a pixel); over 400 samples the
heading changes by < 10° between consecutive samples everywhere, including at waypoints; equal
arc-length steps for linear e (within 2%); two points give the line; coincident consecutive points
give no NaN, and one distinct point holds still.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/ -run 'Path|Camera'`
**Commit:** `feat(compose): pan along a smooth path through waypoints`

## 7. Compose: plan the storyboard's camera moves — ✅ DONE (2026-10-17)

NOTES (2026-10-17): re-derived from New in compose.go assembling the camera's tracks: item 5 moved the assembly (newCamera) into zoomPlanner.plan in internal/compose/zoom.go, so plan now appends the camera moves planMoves (camera.go) returns before building the camera; zoomPlanner keeps its name, its doc now says it plans the whole camera.
NOTES (2026-10-17): the snapshot walk-back zoomBox did is extracted into zoomPlanner.resolveShown (zoom.go) and shared by a pan's target points instead of copied; the beat-zoom golden and box tests pass unchanged.
NOTES (2026-10-17): consequential edit — internal/render/render.go: made necessary by New now failing on a camera pan's target (NewCompositor's doc named only an area or a zoom target).
NOTES (2026-10-17): no CHANGELOG entry here: item 8 owns the camera's CHANGELOG line under Unreleased.

Depends on items 4 and 6.
**What:**
**Goal:** the compositor plans every `camera` move of the storyboard onto its tracks together with
the beat zooms, the cursor follows the camera, and a clip whose moves overlap across the two
tracks over at least two beats composes.
**Approach (assumed at the header base):** in `camera.go` (or a new `camera_plan.go` if it passes
~400 lines): times from `board.ClipTime` (item 2). A pan's points resolve with the `resolver` on the
snapshot shown at the move's end (`schedule.Source` of the end, clamped inside the clip), walking
back to the move's start as `zoomBox` walks; a cell point maps through `Layout.cellCentre`. The
path starts at the pan track's centre at the move's start. A target never shown fails `New` with
`compose: camera[i]: pan: …`, wrapping `target.ErrNotOnScreen`. The drawn cursor needs no change:
it maps through `crop`.
**Files:** internal/compose/camera.go, internal/compose/camera_test.go, internal/compose/compose.go, internal/compose/doc.go
**Read first:** internal/compose/zoom.go — zoomBox, loggedBox (the walk-back to copy); internal/schedule/schedule.go — Schedule.Source, Schedule.Length,
Section.OutStart; internal/compose/compose.go — New, Layout.cellCentre, crop, drawCursor, toOutput; internal/render/render.go — NewCompositor (doc says it fails only on
an area or zoom target); internal/compose/compose_test.go — newTestCompositor, screen, beatStart, TestNewRefusesAZoomTargetTheBeatNeverShows, TestComposeZoomedCursorMapsToTheZoomedTarget
**Tests:** the issue's scenario (zoom-in; pan starting during it; a `via` pan; a zoom-out starting
during that pan; across ≥ 2 beats) asserting factor and centre at start, middle and end of each
move; a target resolved on the end screen; a never-shown target refused; cursor position maps
through a panned crop.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/ ./internal/render/`
**Commit:** `feat(compose): plan the storyboard's camera moves`

## 8. Document the camera — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the Header table's existing `camera` row (added by item 4) now links to the new Camera subsection; no other README section touched.
NOTES (2026-10-17): `capcom check` exits 1 on the untouched tree too ("AGENTS.md:30: the capcom block is out of date; run capcom agents to refresh it"); pre-existing, AGENTS.md is untouched by this run, and the check reports nothing for the edited Note.

Depends on item 7.
**Closes:** camera-moves
**What:** README: in "Storyboard reference" add a `### Camera` subsection (the `camera:` list,
both move forms, `at`/`over`/`ease`, cell points, `via`, the overlap rule, the lint checks) with a
YAML example; in "Sections, zoom and cursor" (heading text unchanged) say a beat's `zoom` is a
shorthand for an in, hold and out on the camera's tracks, that the camera holds its last state
between moves, that the crop clamps at the edges and the cursor follows the camera. Update Note
`package-layout`'s `compose` row ("camera and drawn cursor over each frame") via
`.capcom/knowledge/package-layout.md` and run `capcom check`. CHANGELOG line under Unreleased
travels in the sidecar.
**Regression guard.** Acceptance uses `-run README` (the tests are TestREADME…, -run is case-sensitive). The README names
every new yaml tag in backticks, including `beat` if Moment carries that tag.
**Files:** README.md, .capcom/knowledge/package-layout.md
**Read first:** README.md — "Storyboard reference", "Sections, zoom and cursor"; cmd/photon/readme_test.go — TestREADMEDocumentsEveryField, yamlTags;
.capcom/knowledge/package-layout.md — `internal/compose` row
**Tests:** the README test in `cmd/photon` passes.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run README`
- `capcom check`
**Commit:** `docs: describe the camera`

## 9. Embedded caption faces and the config's `caption_font` — ✅ DONE (2026-10-17)

NOTES (2026-10-17): fonts are from the adobe-fonts/source-sans 3.052R release (TTF zip); the licence text is that tag's LICENSE.md, with its CRLF line endings converted to LF to match the other OFL texts and .gitattributes.
NOTES (2026-10-17): loadFace was split into loadFace + parseFace so LoadCaption can parse a file read by path; the caption_font readability check returns a *ValidationError from Load after resolvePaths, separate from the schema validation pass.
NOTES (2026-10-17): consequential edit — docs/releasing.md: made necessary by cmd/release/dist.go now packing four font licences, not three

**What:**
**Goal:** `fonts` embeds Source Sans 3 Regular and Bold with their OFL text and loads them as a
caption set, or the files the config's `caption_font: {regular, bold}` names; release archives
carry the new licence; the README's fonts section lists it.
**Approach (assumed at the header base):** download `SourceSans3-Regular.ttf`,
`SourceSans3-Bold.ttf` and the OFL text from the adobe-fonts/source-sans GitHub release into
`internal/fonts/` (`SourceSans3-OFL.txt`; the embed glob already covers both patterns).
`fonts.CaptionSet{Regular, Bold Face}` and `fonts.LoadCaption(regular, bold string)`: an empty
path is the embedded face. The `fonts` override dir stays terminal-only. `config.Config` gains
`CaptionFont struct{Regular, Bold string}` (`caption_font`), each resolved against `Dir` like
`Fonts` and checked readable by validation. Add the OFL text to the licence list in
`cmd/release/dist.go` and its help text.
**Regression guard.** It edits README, so Acceptance adds `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run README`; README
documents `caption_font` with `regular` and `bold` (photon.yaml table row). The caption faces get their own `fontFile` table
(name + SourceSans3-OFL.txt), added to TestEveryFaceHasAnEmbeddedLicence's `slices.Concat`; dist_test.go's `testDocuments` and
archive want list gain the OFL text; the readability check runs after `resolvePaths`; fonts/doc.go names the new faces and API.
**Files:** internal/fonts/fonts.go, internal/fonts/fonts_test.go, internal/fonts/doc.go, internal/fonts/SourceSans3-Regular.ttf, internal/fonts/SourceSans3-Bold.ttf, internal/fonts/SourceSans3-OFL.txt, internal/config/config.go, internal/config/config_test.go, cmd/release/dist.go, cmd/release/dist_test.go, README.md
**Read first:** internal/fonts/fonts.go — Load, loadFace, primaryFiles, fallbackFiles, embedded; internal/fonts/fonts_test.go — TestEveryFaceHasAnEmbeddedLicence,
writeOverride; internal/config/config.go — Load, validate, resolvePaths; cmd/release/dist.go — documentFiles, newDistCommand; cmd/release/dist_test.go — testDocuments,
TestDistArchivesHoldTheBinaryAndDocsUnderTheStem; cmd/photon/readme_test.go — TestREADMEDocumentsEveryField; internal/fonts/doc.go
**Tests:** `LoadCaption("", "")` parses both; a path override; a missing file names it; config
resolves and rejects an unreadable `caption_font` file, and accepts a relative one with the cwd elsewhere; the licence
test covers the caption table; dist test lists the new licence; the README test passes.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/fonts/ ./internal/config/ ./cmd/release/`
- `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run README`
**Commit:** `feat(fonts): embed Source Sans 3 as the caption face`

## 10. Storyboard `captions`: schema and validation — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the caption rules live in caption.go beside their types (validate.go only calls `validateCaptions`), as camera.go does; applyDefaults trims one trailing newline from `text` and fills size (frame.font_size), weight regular, place bottom / align middle (only when `position` is absent), color #ffffff and box #000000 at 0.6
NOTES (2026-10-17): `box.opacity` is a *float64 so an explicit `opacity: 0` (no box) differs from the 0.6 default; `for: 0s` without `until` reads as no end ("want exactly one of for or until"); a caption inheriting a frame.font_size outside [6, 72] is reported as `font.size: … from frame.font_size`; offset and position are a new `Pixels {x, y}` type in design pixels (beat.go's `Offset` counts cells)
NOTES (2026-10-17): the README gains one `captions` row in the Storyboard Header table naming every new tag; item 13 writes the full reference

Depends on item 2.
**What:**
**Goal:** a storyboard may carry a top-level `captions:` list; `Parse` rejects every bad caption
and `photon lint` reports it.
**Approach (assumed at the header base):** new file `internal/storyboard/caption.go`. A caption:
`text` (non-empty; lines split on `\n`, one trailing newline trimmed), `at` (Moment), exactly one
of `for` (> 0) and `until` (Moment), `font: {size, weight: regular|bold}` (size in the unit of
`frame.font_size`, default `frame.font_size`, in named-constant bounds [6, 72]), `place`
(default `bottom`), `align` (default `middle`), `offset: {x, y}` in design pixels, or
`position: {x, y}` (the box's top-left, design pixels) instead of `place`/`align`/`offset`,
`color` (`#rrggbb`, default `#ffffff`), `box: {color, opacity}` (default `#000000`, 0.6; opacity
in [0, 1]), `fade: {in, out}` (≥ 0, in+out ≤ the caption's length). Times resolve through item 2:
start in [0, Length), stop after start and ≤ Length; a cut or unknown beat is a problem.
Problems read `captions[i]: …`. Add a caption to `testdata/every-form.yaml`.
**Regression guard.** This item adds every new caption tag to README.md's Storyboard reference in backticks (`captions`,
`until`, `font`, `size`, `weight`, `place`, `align`, `position`, `color`, `box`, `opacity`, `fade`), item 13 writing the
reference around them, so TestREADMEDocumentsEveryField stays green. The lint case uses a new fixture `invalid/captions.yaml`,
linted with `--config` validConfig; `./internal/storyboard/` runs unfiltered so TestLoadEveryForm and TestLoadRejects run.
**Files:** internal/storyboard/caption.go, internal/storyboard/storyboard.go, internal/storyboard/validate.go, internal/storyboard/storyboard_test.go, internal/storyboard/testdata/every-form.yaml, internal/storyboard/doc.go, cmd/photon/lint_test.go, cmd/photon/testdata/invalid/captions.yaml, README.md
**Read first:** internal/storyboard/storyboard.go — Storyboard, Parse, applyDefaults; internal/storyboard/validate.go — validate, prefixed, validateHeader;
internal/storyboard/storyboard_test.go — validBase, TestLoadRejects, TestLoadEveryForm, TestParseMatchesLoad; cmd/photon/lint_test.go — TestLint, validConfig;
internal/storyboard/testdata/every-form.yaml; cmd/photon/readme_test.go — TestREADMEDocumentsEveryField, yamlTags
**Tests:** table tests per rule; a `photon lint` case rejecting a stop before start, a time past
the clip, an unknown beat, a size out of bounds, fades longer than the caption; the README test passes.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/`
- `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run 'Lint|README'`
**Commit:** `feat(storyboard): declare and validate captions`

## 11. Caption package: timing, placement and drawing

Depends on items 9 and 10.
**What:**
**Goal:** a new package `internal/caption` turns a storyboard's captions into a layer that draws
each caption onto a shipped frame at output time t, at its place, colours and fade opacity,
inside the frame.
**Approach (assumed at the header base):** one deep module: `caption.New(board, faces
fonts.CaptionSet, out image.Point, unit float64) (*Layer, error)` resolves times (item 2),
measures lines with `golang.org/x/image/font` at the pixel size `frame.font_size` maps to in
`internal/raster` times the caption's size ratio, lays the block out (named-constant edge margin
and box padding in design pixels, times `unit`), clamps it inside `out`, and errors when it
cannot fit. `(*Layer).Draw(dst *image.RGBA, t)` paints the box then the text, alpha = box/text
opacity × fade (linear ramps; 1 between; 0 outside). Immutable after `New`, safe for concurrent
use. Add `doc.go`. Imports `storyboard` and `fonts` only.
**Regression guard.** `New` rasterizes each caption's text to an alpha mask so `Draw` only composites: tune serves frames
concurrently on one Compositor and an opentype `font.Face` is not safe for concurrent use. The shipped em is caption size ×
designScale (2) × `unit` pixels — the shipped terminal text's em at size = `frame.font_size` — not `internal/raster`'s
source-pixel size. `internal/caption/docmap_test.go` calls `testkit.CheckDocMap`, as every package's does.
**Files:** internal/caption/doc.go, internal/caption/caption.go, internal/caption/layout.go, internal/caption/caption_test.go, internal/caption/layout_test.go, internal/caption/docmap_test.go
**Read first:** internal/compose/compose.go — Compositor.unit, Compose; internal/compose/cursor.go — designScale; internal/raster/glyph.go — newFace, glyphs.mu,
pixelsPerPoint; internal/render/render.go — NewRasterizer; internal/tune/frame.go — Session.Frame; internal/fonts/fonts.go — Face, Set;
internal/testkit/docmap.go — CheckDocMap
**Tests:** timing per anchor form; fade alpha at 0, mid and end of each ramp; placement for each
`place`×`align`, offset and `position`; clamping; too-wide refused; pixels of text colour appear
inside the box and none outside it; a caption at size = `frame.font_size` has the shipped terminal text's em; concurrent
`Draw` calls on one layer.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/caption/`
**Commit:** `feat(caption): lay out and draw a clip's captions`

## 12. Captions on every composed frame

Depends on items 7 and 11.
**What:**
**Goal:** `photon render` and `photon tune` frames show the storyboard's captions in output
space, over the cursor, unscaled by any zoom.
**Approach (assumed at the header base):** `compose.New` gains a `fonts.CaptionSet` parameter and
builds the layer with `caption.New(board, faces, c.out, c.unit())`; `Compose` draws it last.
`render.NewCompositor` loads faces with `fonts.LoadCaption` from `loaded.Config.CaptionFont`;
`tune` reaches it through `render.NewCompositor` unchanged. Update every `compose.New` caller
(tests included). Update Note `package-layout`: add the `internal/caption` row and `caption`,
`fonts` to `compose`'s imports; run `capcom check`.
**Regression guard.** `caption.New` with no captions returns an empty layer without reading the frame or the faces, so
existing callers (boards with `Frame.FontSize` 0) may pass `fonts.CaptionSet{}`. The zoom test sets box opacity 1 and compares
the box rectangle: a 0.6 box blends over the zoomed frame. Superseded: internal/compose/doc.go's "It imports schedule, target,
take and storyboard"; item 13 rewrites it with `caption` and `fonts`.
**Files:** internal/compose/compose.go, internal/compose/compose_test.go, internal/compose/cursor_test.go, internal/render/render.go, internal/render/render_test.go, .capcom/knowledge/package-layout.md
**Read first:** internal/compose/compose.go — New, Compositor, Compose, unit; internal/render/render.go — NewCompositor, NewRasterizer; internal/tune/session.go —
Session.build, rasterizerFor; internal/compose/compose_test.go — newTestCompositor; internal/tune/frame_test.go; internal/render/render_test.go —
TestRunWritesAGIFOfTheScheduledLength, loadRenderClip; .capcom/knowledge/package-layout.md
**Tests:** a zoomed section leaves a caption's box rectangle (box opacity 1) identical to the unzoomed frame's; a
caption drawn over the cursor; a render test (skips without ffmpeg) with two captions, one faded,
checking each appears and disappears at its times; changing a beat's `duration` moves a
beat-anchored caption.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/ ./internal/render/ ./internal/tune/`
- `capcom check`
**Commit:** `feat(render): draw captions over the composed frame`

## 13. Document captions

Depends on item 12.
**Closes:** clip-captions
**What:** README: in "Storyboard reference" add a `### Captions` subsection (every key, defaults,
the anchor forms shared with the camera, lint checks, output-space behaviour, only `photon render`
needed after an edit) with an example; in "Fonts and licences" the caption faces and
`caption_font` (if item 9 left it out); `photon.yaml` reference row for `caption_font`. Update the
`compose` and `storyboard` doc maps for captions. CHANGELOG line travels in the sidecar.
**Regression guard.** Acceptance uses `-run 'README|DocMap'`. internal/compose/doc.go's imports sentence gains `caption`
and `fonts` (item 12 made it stale).
**Files:** README.md, internal/compose/doc.go, internal/storyboard/doc.go
**Read first:** README.md — Storyboard reference, Fonts and licences, photon.yaml table; cmd/photon/readme_test.go — TestREADMEDocumentsEveryField, yamlTags;
internal/compose/doc.go; internal/storyboard/doc.go
**Tests:** README and doc-map tests pass.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./cmd/photon/ ./internal/compose/ ./internal/storyboard/ -run 'README|DocMap'`
**Commit:** `docs: describe captions`

## 14. Tune state reports camera moves and captions

Depends on items 4 and 10.
**What:**
**Goal:** `/api/state` carries a `camera` list and a `captions` list, one entry per storyboard
entry in order, each with its index, its resolved start and stop in whole milliseconds on the
output clock, and the fields items 15-16 edit.
**Approach (assumed at the header base):** in `internal/tune/state.go` add `CameraState`
(`index`, `kind` zoom|pan, `start_ms`, `end_ms`, `at`, `over_ms`, `ease`, `factor` for a zoom,
`target`/`cell` for a pan to, `via_count`) and `CaptionState` (`index`, `text`, `start_ms`,
`stop_ms`, `at`, `for_ms` or `until`, `place`, `align`). A moment travels as
`{"beat": N, "offset_ms": D}`, beat 0 meaning a bare clip time of D (mirrors item 2's `Moment`).
Times come from `board.ClipTime`. Validation problems prefixed `camera[i]:`/`captions[i]:` stay
clip-level errors, as other non-beat problems are.
**Regression guard.** `beatNamed` returns 0 for a message starting `camera[` or `captions[`; otherwise a problem such as
"camera[0]: zoom overlaps beat 2's zoom", left by an outside write (Refresh → rebuild), lands on beat 2.
**Files:** internal/tune/state.go, internal/tune/session_test.go
**Read first:** internal/tune/state.go — State, Session.State, beatStates, problemsOf, beatNamed; internal/tune/session.go — Session.rebuild, Session.build;
internal/tune/session_test.go — newSessionFixture, sessionStoryboard, TestSession_State_ReportsTheSchedule; internal/tune/server_test.go — TestHandler_State_ReportsTheSession
**Tests:** a session test on a storyboard with one zoom, one `via` pan and two captions asserting
every reported field; times move when a beat's duration is edited; a Refresh onto a storyboard with a
`camera[0]: … beat 2 …` problem reports it with `Problem.Beat` 0.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'State|Session'`
**Commit:** `feat(tune): report camera moves and captions in the state`

## 15. Tune edits a camera move's fields

Depends on item 14.
**What:**
**Goal:** an `Edit` with `field` `camera.at`, `camera.over`, `camera.ease` or `camera.to` and an
`index` changes that camera move in the storyboard source, spliced like every other tune edit,
undoable, and refused when the result fails validation.
**Approach (assumed at the header base):** `Edit` gains `Index int` (`json:"index"`), used only by
`camera.*` and `caption.*` fields (paths `camera[i].<kind>.<key>`). Values: `at` a moment as YAML
flow text (`{beat: 3, offset: 200ms}` or `4500ms`), replacing the whole node; `over` integer ms
in the old unit (`value.go`); `ease` `smooth` deletes the key, `linear` sets it; `to` a decimal
for a zoom, target flow text (a pick proposal's snippet) or `{cell: {x: 3, y: 4}}` for a pan,
refused on a `via` pan. Replacing a mapping or scalar node with one-line flow text needs a
`yamledit.Document.Replace(path, value)` unless `splicer` already has an equivalent; add it to
`yamledit` with its own tests. New file `internal/tune/edit_camera.go` keeps `edit.go` under ~400
lines; extend `editFields` and the `Edit` doc comment.
**Regression guard.** Settle yamledit Replace here: `Replace(path, value)` replaces any node — plain/quoted/block scalar, flow or block mapping — with one-line flow text, in a new file internal/yamledit/replace.go (document.go is already ~400 lines) with internal/yamledit/replace_test.go; Files lists those instead of document.go/document_test.go.
Positional `Edit{1, …}` literals in edit_test.go and session_test.go are rewritten keyed; `camera.*`/`caption.*` route before
`splicer.apply`'s beat lookup and `editName` returns `camera[i]: <field>` for them; yamledit/doc.go's comment and map name Replace.
**Files:** internal/tune/edit.go, internal/tune/edit_camera.go, internal/tune/edit_test.go, internal/tune/session_test.go, internal/tune/value.go, internal/tune/doc.go, internal/yamledit/replace.go, internal/yamledit/replace_test.go, internal/yamledit/doc.go
**Read first:** internal/tune/edit.go — Edit, editFields, editName, splicer.apply, splicer.replaceTarget, splicer.writeDuration; internal/tune/value.go — formatDuration,
parseTarget, parseMilliseconds; internal/yamledit/document.go — Document.Set, Document.Delete, Document.Insert, Document.apply; internal/yamledit/scan.go — scalarSpan;
internal/tune/edit_test.go — TestApply_Edit_ChangesOnlyTheEditedText, TestApply_EditOnWhatIsNotThere_IsRefused, readBoard; internal/tune/testdata/board.yaml
**Tests:** each field on block and flow forms; `at` switching between beat and clip forms; an edit
that makes two zooms overlap is refused naming both, reading `camera[i]: <field>: …`; undo restores the bytes; `Replace` on
plain, quoted and block scalars and on flow and block mappings.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ ./internal/yamledit/`
**Commit:** `feat(tune): edit camera moves`

## 16. Tune edits a caption's fields

Depends on item 15.
**What:**
**Goal:** an `Edit` with `field` `caption.at`, `caption.for`, `caption.until`, `caption.text`,
`caption.place` or `caption.align` and an `index` changes that caption in the storyboard source,
undoable, and refused when the result fails validation.
**Approach (assumed at the header base):** new file `internal/tune/edit_caption.go` on item 15's
`Index` and node replacement. `for` (integer ms) and `until` (moment flow text) are exclusive:
setting one deletes the other. `text` is written as a one-line double-quoted YAML string with
`\n` escapes, replacing a block scalar too. `place`/`align` equal to their default delete the key.
**Regression guard.** Rests on item 15's Replace for replacing a block-scalar caption text; no yamledit change of its own.
**Files:** internal/tune/edit_caption.go, internal/tune/edit.go, internal/tune/edit_test.go, internal/tune/doc.go
**Read first:** internal/tune/edit.go — splicer.writeScalar, splicer.deleteKey, keysBefore, splicer.lookup; internal/yamledit/document.go — Quote, Document.Delete,
Document.Insert; internal/yamledit/scan.go — scalarSpan; internal/tune/edit_test.go — TestApply_InvalidResult_IsRefusedWithTheValidationError; internal/tune/doc.go
**Tests:** each field; `for`→`until` swap; multi-line text round-trips through `storyboard.Parse`;
a stop before the start is refused.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run Edit`
**Commit:** `feat(tune): edit captions`

## 17. Tune page shows camera moves and captions

Depends on item 14.
**What:**
**Goal:** the timeline draws a zoom lane, a pan lane and a captions lane under the beats, one bar
per entry from its start to its stop, and selecting a bar shows its fields in the inspector.
**Approach (assumed at the header base):** in `internal/tune/page/timeline.js` add the three lanes
from `state.camera`/`state.captions`, scaled as beat sections are; in `app.js` a selection kind
for camera and caption entries with a read-only panel; styles in `style.css`. If `timeline.js`
passes ~400 lines, put the lanes in a new `page/lanes.js`, loaded from `index.html` and listed in
`page_test.go`'s `pageFileNames`.
**Regression guard.** Lanes hook in through a registry `window.tuneLanes` that `lanes.js` fills (loaded before
`timeline.js`) and `render` reads, a lane giving a whole-state `draw(state)` beside the per-section one; this supersedes the
`timeline.js` header's lane model (lines 7-9), which is rewritten. `selected()` stays a beat id or null; a new
`selectedEntry()` → `{kind, index}` joins `tuneContract`, `takeState` keeping an entry whose index still exists. The new lanes
are named `camera zoom`, `pan` and `captions`.
**Files:** internal/tune/page/timeline.js, internal/tune/page/app.js, internal/tune/page/style.css, internal/tune/page/index.html, internal/tune/page/lanes.js, internal/tune/page_test.go
**Read first:** internal/tune/page/timeline.js — lanes, render, laneRow, selectable, follow, markSelection; internal/tune/page/app.js — window.tune, select, takeState,
renderInspector, beatOf; internal/tune/page_test.go — pageFileNames, tuneContract, TestPage_IndexReferences_AreServed; internal/tune/page/index.html;
internal/tune/state.go — State
**Tests:** page test asserting the lanes' element ids/classes and the state keys the page reads
exist in `State`'s json tags; `selectedEntry` is in `tuneContract`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run Page`
**Commit:** `feat(tune): show camera moves and captions on the timeline`

## 18. Tune page edits camera moves and captions

Depends on items 15, 16 and 17.
**What:**
**Goal:** the inspector panel of a selected camera move or caption edits the fields items 15-16
accept, sending `{index, field, value}` edits; a pan's `to` can be set from a pick, as a zoom
target is; the README's `photon tune` section says so.
**Approach (assumed at the header base):** inputs in `app.js` built like the beat panel's (ms
fields, selects for `ease`/`place`/`align`, a textarea for caption text, a moment input taking
the flow spelling); in `picker.js` offer "use as pan target" beside the zoom option when a pan is
selected, sending the proposal's target snippet as `camera.to`.
**Regression guard.** Acceptance uses `-run 'Page|README'`. `isTyping` also covers `HTMLTextAreaElement`; a refusal is keyed
by beat or by `{kind, index}` and the camera and caption panels call `errorRows` for their own entry; the pan-target button is
markup in `index.html` beside `#pick-zoom`, sending `snippets.zoom` (`SNIPPET_ZOOM`) as `camera.to`.
**Files:** internal/tune/page/app.js, internal/tune/page/picker.js, internal/tune/page/index.html, internal/tune/page/style.css, internal/tune/page_test.go, README.md
**Read first:** internal/tune/page/app.js — edit, applyEdit, millisecondsField, zoomPanel, errorRows, isTyping, onKey, window.tune; internal/tune/page/picker.js —
setAsZoomTarget, showProposal, wire; internal/tune/page/index.html — pick-result, pick-zoom; internal/tune/page_test.go — TestPage_Picker_CallsPickAndClipboard,
tuneContract; internal/tune/edit.go — editFields; cmd/photon/readme_test.go — TestREADMEDocumentsEveryCommand
**Tests:** page test asserting every `camera.*`/`caption.*` field the page sends is in
`editFields`; the README test passes.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ ./cmd/photon/ -run 'Page|README'`
**Commit:** `feat(tune): edit camera moves and captions in the page`
