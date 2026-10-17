# Camera moves, captions and the tune frame values — implementation plan

**Goal:** Give a clip a camera of two independent, overlapping tracks (zoom and pan) placed on
the clip's timeline, with a beat's `zoom` kept as a shorthand; let a storyboard declare captions
drawn over the composed frame; show the read-only frame values in the `photon tune` inspector.
**Date:** 2026-10-17
**Status:** draft — regression check pending
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

## 1. Tune inspector shows the read-only frame values

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
**Tests:** a session test asserting the seven values from the test storyboard's frame; extend the
page test if it pins the panel's text.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/`
- `grep -n "max_colors" internal/tune/state.go internal/tune/page/app.js`
**Commit:** `fix(tune): report and show the read-only frame values`

## 2. Storyboard clip clock: moments on the output timeline

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
**Files:** internal/storyboard/moment.go, internal/storyboard/moment_test.go, internal/storyboard/doc.go, internal/schedule/schedule_test.go
**Tests:** table tests for both decode forms, negative offset, unknown and cut beat, a bad scalar;
a schedule test asserting every section's `OutStart` equals `ClipTime({beat: id})` and
`Schedule.Length` equals `Length()`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/ ./internal/schedule/`
**Commit:** `feat(storyboard): add moments on the clip's output clock`

## 3. Golden of a beat zoom's crops

**What:** test-only. Pin today's beat-zoom framing before the camera replaces its mechanism.
Add `internal/compose/zoom_golden_test.go`: a board with two zoomed beats (one explicit factor,
one auto-fit, one with ramps shrunk to fit its section, one target near the frame's right edge so
the crop clamps), composed with the existing test helpers (`newTestCompositor`, `screen`,
`clickAt`). It samples `Compositor.crop` at every frame at 25 fps across the clip and compares
each rect, to 1e-6, against `internal/compose/testdata/beat-zoom-crops.golden` (one line per
frame, `%.6f`). An `-update` flag rewrites it, as `internal/raster`'s tests do.
**Files:** internal/compose/zoom_golden_test.go, internal/compose/testdata/beat-zoom-crops.golden
**Tests:** the golden test itself; it passes at the base.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/ -run Golden`
**Commit:** `test(compose): pin a beat zoom's crops in a golden`

## 4. Storyboard `camera`: schema and validation

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
**Files:** internal/storyboard/camera.go, internal/storyboard/storyboard.go, internal/storyboard/validate.go, internal/storyboard/storyboard_test.go, internal/storyboard/testdata/every-form.yaml, internal/storyboard/doc.go, internal/clip/clip.go, internal/clip/clip_test.go, cmd/photon/lint_test.go
**Tests:** table tests per rule; a clip test for an undeclared camera area; a `photon lint` case
rejecting an unknown area, `over: 0`, `to: 3.5`, a start past the clip, and two overlapping zooms.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/ ./internal/clip/ ./cmd/photon/ -run 'Lint|Camera|Parse|Load'`
**Commit:** `feat(storyboard): declare and validate camera moves`

## 5. Compose: a two-track camera behind the crop

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
**Files:** internal/compose/camera.go, internal/compose/zoom.go, internal/compose/compose.go, internal/compose/compose_test.go, internal/compose/camera_test.go, internal/compose/doc.go
**Tests:** camera unit tests: hold between moves, log-space zoom midpoint, pan midpoint; existing
zoom tests (move any that call `factorAt` to the camera); the item 3 golden unchanged.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/`
- `git diff --exit-code -- internal/compose/testdata/` (the golden is not rewritten)
**Commit:** `refactor(compose): drive the crop from a two-track camera`

## 6. Compose: a smooth pan path through waypoints

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
**Files:** internal/compose/path.go, internal/compose/path_test.go, internal/compose/camera.go, internal/compose/doc.go
**Tests:** endpoints exact; passes through each waypoint (within a pixel); over 400 samples the
heading changes by < 10° between consecutive samples everywhere, including at waypoints; equal
arc-length steps for linear e (within 2%); two points give the line.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/ -run 'Path|Camera'`
**Commit:** `feat(compose): pan along a smooth path through waypoints`

## 7. Compose: plan the storyboard's camera moves

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
**Tests:** the issue's scenario (zoom-in; pan starting during it; a `via` pan; a zoom-out starting
during that pan; across ≥ 2 beats) asserting factor and centre at start, middle and end of each
move; a target resolved on the end screen; a never-shown target refused; cursor position maps
through a panned crop.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/compose/ ./internal/render/`
**Commit:** `feat(compose): plan the storyboard's camera moves`

## 8. Document the camera

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
**Files:** README.md, .capcom/knowledge/package-layout.md
**Tests:** the README test in `cmd/photon` passes.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./cmd/photon/ -run Readme`
- `capcom check`
**Commit:** `docs: describe the camera`

## 9. Embedded caption faces and the config's `caption_font`

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
**Files:** internal/fonts/fonts.go, internal/fonts/fonts_test.go, internal/fonts/SourceSans3-Regular.ttf, internal/fonts/SourceSans3-Bold.ttf, internal/fonts/SourceSans3-OFL.txt, internal/config/config.go, internal/config/config_test.go, cmd/release/dist.go, README.md
**Tests:** `LoadCaption("", "")` parses both; a path override; a missing file names it; config
resolves and rejects an unreadable `caption_font` file; dist test lists the new licence.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/fonts/ ./internal/config/ ./cmd/release/`
**Commit:** `feat(fonts): embed Source Sans 3 as the caption face`

## 10. Storyboard `captions`: schema and validation

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
**Files:** internal/storyboard/caption.go, internal/storyboard/storyboard.go, internal/storyboard/validate.go, internal/storyboard/storyboard_test.go, internal/storyboard/testdata/every-form.yaml, internal/storyboard/doc.go, cmd/photon/lint_test.go
**Tests:** table tests per rule; a `photon lint` case rejecting a stop before start, a time past
the clip, an unknown beat, a size out of bounds, fades longer than the caption.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/storyboard/ ./cmd/photon/ -run 'Lint|Caption|Parse'`
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
**Files:** internal/caption/doc.go, internal/caption/caption.go, internal/caption/layout.go, internal/caption/caption_test.go, internal/caption/layout_test.go
**Tests:** timing per anchor form; fade alpha at 0, mid and end of each ramp; placement for each
`place`×`align`, offset and `position`; clamping; too-wide refused; pixels of text colour appear
inside the box and none outside it.
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
**Files:** internal/compose/compose.go, internal/compose/compose_test.go, internal/compose/cursor_test.go, internal/render/render.go, internal/render/render_test.go, .capcom/knowledge/package-layout.md
**Tests:** a zoomed section leaves a caption's pixels identical to the unzoomed frame's; a
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
**Files:** README.md, internal/compose/doc.go, internal/storyboard/doc.go
**Tests:** README and doc-map tests pass.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./cmd/photon/ ./internal/compose/ ./internal/storyboard/ -run 'Readme|DocMap'`
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
**Files:** internal/tune/state.go, internal/tune/session_test.go
**Tests:** a session test on a storyboard with one zoom, one `via` pan and two captions asserting
every reported field; times move when a beat's duration is edited.
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
**Files:** internal/tune/edit.go, internal/tune/edit_camera.go, internal/tune/edit_test.go, internal/tune/value.go, internal/tune/doc.go, internal/yamledit/document.go, internal/yamledit/document_test.go
**Tests:** each field on block and flow forms; `at` switching between beat and clip forms; an edit
that makes two zooms overlap is refused naming both; undo restores the bytes.
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
**Files:** internal/tune/edit_caption.go, internal/tune/edit.go, internal/tune/edit_test.go, internal/tune/doc.go
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
**Files:** internal/tune/page/timeline.js, internal/tune/page/app.js, internal/tune/page/style.css, internal/tune/page/index.html, internal/tune/page/lanes.js, internal/tune/page_test.go
**Tests:** page test asserting the lanes' element ids/classes and the state keys the page reads
exist in `State`'s json tags.
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
**Files:** internal/tune/page/app.js, internal/tune/page/picker.js, internal/tune/page_test.go, README.md
**Tests:** page test asserting every `camera.*`/`caption.*` field the page sends is in
`editFields`; the README test passes.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ ./cmd/photon/ -run 'Page|Readme'`
**Commit:** `feat(tune): edit camera moves and captions in the page`
