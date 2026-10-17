# Timing editor plan

**Goal:** `photon tune <storyboard> [<take>]` serves a local page that previews section timing,
zoom, `frame.width` and `frame.fps` against an existing take, frame for frame as `photon render`
composes it, and saves edits back into the storyboard by splicing only the edited scalars.
A target picker on the preview proposes targets checked by the one resolver.
**Date:** 2026-10-17
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 6824900
**Closes:** timing-editor; target-picker

**Sources:** issues `timing-editor` and `target-picker` (their `## Acceptance`); Notes
`package-layout`, `coding-conventions`, `glossary`; `internal/render/render.go`,
`internal/schedule/schedule.go`, `internal/compose/compose.go`, `internal/target/resolver.go`.

**Ratified design calls** (user, 2026-10-17):
- **Surface:** a local web page embedded in the binary (`go:embed`), plain HTML/CSS/JS, no build step, no network asset; not a TUI.
- **Command:** `photon tune <storyboard> [<take>]`; `--port` (default 0, any free port), `--config`, `--work`; binds 127.0.0.1 only, prints the URL, runs until Ctrl-C; no token, never opens a browser.
- **No `render --watch`.** `pause` and every `do` value show read-only, marked "needs photon record".
- **Editable:** beat `duration`, `hold`, `cut`, `zoom` (target, factor incl. auto = 0, in, out; add and remove), `frame.width`, `frame.fps`. `max_colors`, `font_size`, `scale`, `padding`, `line_height`, `cols`, `rows` read-only.
- **Layout:** preview top-left, inspector top-right (selected beat or, with none selected, the clip's frame fields); transport bar (play, step, time, Save, undo/redo, Revert); full-width lanes `beats`, `hold`, `zoom` on an output-time axis; a cut beat is a hatched marker.
- **Lanes:** the timeline is a list of generic lanes so a later captions lane plugs in; no caption code now.
- **Preview:** full fidelity (render's own compositor, pre-quantization), PNG; rasterized snapshots in an LRU capped at 1 GiB; play runs in real time and drops frames not ready.
- **Write format:** only touched keys change; defaults never inserted; a duration keeps its unit (`1800ms` stays ms, `2s` stays s), other or new values use `time.Duration.String()`; drags snap to 50ms, typed values take any whole ms.
- **Save:** in place (temp file + rename, mode kept), no backup; refused when the file changed on disk since load, or while any error stands; re-validated before writing.
- **Undo:** server-side undo/redo/Revert; files changed on disk reload automatically when nothing is unsaved, else a banner and Save refuses until Reload.
- **Bad edit:** the error shows on the beat (or the clip), the preview keeps the last good pipeline.
- **Picker:** in this plan. ⌖ arms pick mode; drag over cells of one row on the paused preview; proposal = escaped text, `nth` when repeated, an area when one contains it; resolver-checked, box drawn; actions: set as zoom target, copy as click / seen / drag-from / drag-to YAML.
- **Keys:** Space play/pause, ←/→ one frame, Shift+←/→ one second.
- **Tests:** Go only; the page's JS stays thin, all logic server-side.

**Standing requirements:**
- skills: coding-standards
- Pi host: one package at a time, `GOMEMLIMIT=2GiB`; never `./...` with `-race`.
- No forbidden name (AGENTS.local.md `forbidden:` line) in any committed file.
- New packages get `doc.go` naming every non-test file and a one-line `docmap_test.go`.

**Out of scope:** `render --watch`; captions; editing `do` actions; live-app authoring; a draft/low-scale preview; headless browser tests; GIF export from the page.

**Regression check (2026-10-17, 6824900):**
- 1: guard folded; supersedes the sole-constructor wording at storyboard.go:20, storyboard/doc.go:3, clip.go:30
- 3: guard folded
- 4: guard folded
- 5: guard folded
- 6: guard folded
- 7: recast
- 8: guard folded
- 9: recast
- 10: guard folded
- 11: recast
- 12: guard folded
- 4 (re-check): guard folded — the edit wire contract (Value spellings) made binding
- 7 (re-check): guard folded — Goal rewritten to the pixel signature; `zoom` snippet is the bare target flow mapping
- 8 (re-check): guard folded — the pick wire contract made binding
- 9 (re-check): guard folded — Goal narrowed to the asset test; external-URL pattern; `frameTime()` added; Values sent in item 4's spellings
- 11 (re-check): guard folded — Goal narrowed to the asset test; Approach geometry sentence deleted; posts `frameTime()` as `t`
- 7 (decision): recast — exported `(*compose.Compositor).Rect(t, take.Box)` with a compose table test; `Proposal` gains `Rect image.Rectangle` (output pixels at the picked time)
- 8 (decision): recast — the pick response's binding contract gains `rect` in output-frame pixels beside `box`; the pick handler test asserts it
- 9: rejected — item 9 sends durations as `<n>ms` and factor `0` for auto; item 4's binding spellings are a bare ms integer (`"1850"`) and `"auto"`
- 11 (decision): recast — the picker draws the resolved box from the response's `rect`, scaled by the displayed/natural size ratio

## 1. Storyboard from bytes and a clip with a replaced storyboard — ✅ DONE (2026-10-17)

**What:**
**Goal:** `storyboard.Parse(data []byte, path string) (*Storyboard, error)` decodes, validates
and defaults exactly as `storyboard.Load` does, and `(*clip.Clip).WithStoryboard(board)
(*clip.Clip, error)` returns a clip with the same config and work flag, cross-checked.
**Approach (assumed at the header base):** `storyboard.Load` reads the file and calls `Parse`;
the strict decoder (`KnownFields(true)`), empty-file tolerance, ship resolution against
`filepath.Dir(path)`, `validate` and `applyDefaults` move into `Parse`. `WithStoryboard` runs
`crossCheck` and returns a `*clip.ValidationError` as `Load` does. No behaviour change for
existing callers.
**Regression guard.** Every comment naming `Load` as the sole constructor or as `Path`'s source now names `Parse`/`WithStoryboard` too (find with `grep -rn "only way to get one\|Load read" internal/storyboard internal/clip`); this supersedes the sole-constructor wording at storyboard.go:20, storyboard/doc.go:3 and clip.go:30, while the validated, cross-checked invariant stays. Both doc.go file maps list `Parse` and `WithStoryboard`.
**Files:** internal/storyboard/storyboard.go, internal/storyboard/storyboard_test.go, internal/storyboard/doc.go, internal/clip/clip.go, internal/clip/clip_test.go, internal/clip/doc.go
**Read first:** internal/storyboard/storyboard.go — Load, applyDefaults; internal/clip/clip.go — Load, crossCheck, ValidationError;
internal/storyboard/storyboard_test.go — writeStoryboard, validBase; internal/clip/clip_test.go — validStoryboard
**Tests:** `Parse` of a fixture's bytes equals `Load` of the file (table: valid, unknown key,
empty, invalid zoom); `WithStoryboard` reports an undeclared area and keeps `Work()`/`Take()`.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/storyboard/ ./internal/clip/` ; `go vet ./internal/storyboard/ ./internal/clip/`
**Commit:** `refactor(storyboard): parse a storyboard from bytes; let a clip take a new storyboard`

## 2. Render's frame builders, exported — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the summary row's two-space indent is the named constant `summaryIndent` (magic-string rule), so `writeSummary` prints `summaryIndent + BeatReport(section)`; output unchanged

**What:**
**Goal:** `internal/render` exports `NewRasterizer(loaded *clip.Clip) (*raster.Rasterizer,
error)`, `NewCompositor(loaded *clip.Clip, recorded *take.Take, clipSchedule
*schedule.Schedule, rasterizer *raster.Rasterizer) (*compose.Compositor, error)`,
`SnapshotAt(snapshots []take.Snapshot, t time.Duration) int` and `BeatReport(section
schedule.Section) string`, and `render.Run` builds its frames and summary only through them.
**Approach (assumed at the header base):** rename `newRasterizer`, `newCompositor` and
`snapshotAt`; the frame argument of `newRasterizer` is read from `loaded.Storyboard.Frame`.
`BeatReport` returns one `writeSummary` row without its indent (`beat 3  1.00×  481ms of take
in 3s`, `beat 2  cut`); `writeSummary` prints `"  " + BeatReport(section)`. Output of `photon
render` stays byte-identical.
**Files:** internal/render/render.go, internal/render/encode.go, internal/render/render_test.go
**Read first:** internal/render/render.go — newPlan, newRasterizer, newCompositor, writeSummary; internal/render/encode.go — writeFrames, snapshotAt;
internal/render/render_test.go — TestSnapshotAt, TestRunWritesAGIFOfTheScheduledLength
**Tests:** existing render tests pass unchanged; `TestSnapshotAt` calls the exported name; a
table test of `BeatReport` for a kept and a cut section.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render/` ; `go vet ./internal/render/`
**Commit:** `refactor(render): export the rasterizer, compositor and beat-report builders`

## 3. yamledit: splice edits into YAML source — ✅ DONE (2026-10-17)

NOTES (2026-10-17): added internal/yamledit/scan.go, which the plan's Files line does not name. It holds the source-position and span scanners (line/column to byte offset, scalar, flow-collection and block-entry extents) as Document methods, because document.go alone ran to 577 lines and the house limit (Note coding-conventions) is about 400. doc.go's file map names it.
NOTES (2026-10-17): Path is `[]string`. A step indexes a sequence when the node it steps into is a sequence, and is written as a decimal (Path{"beats", "0", "duration"}). Path.String renders `beats[0].duration` for errors.
NOTES (2026-10-17): Quote single-quotes as planned, but falls back to a double-quoted, escaped string when s holds a line break or another unprintable rune, because single quotes would fold or lose those.
NOTES (2026-10-17): the errors are exported sentinels ErrNotFound, ErrKind, ErrExists and ErrUnsupported. ErrUnsupported covers block scalars, multi-line plain scalars, anchored/tagged scalars, multi-line source text, and any edit whose result no longer parses or no longer holds the edited node. Every edit is verified that way after the splice and rolled back if it fails, so a null block value (`a:` with nothing after) is refused instead of being mis-spliced.
NOTES (2026-10-17): Insert's "last present key named in after" is taken in document order: the new key goes after whichever named key comes last in the mapping. Delete supports mapping keys only (block, flow, and the first key of a `- id:` sequence item); a path ending in a sequence index is ErrKind.

**What:**
**Goal:** a new package `internal/yamledit` (imports only `gopkg.in/yaml.v3`) edits YAML source
bytes in place: set a scalar, insert a key into a mapping, delete a key, addressed by a path of
mapping keys and sequence indexes. Every byte outside the edited span is unchanged, in block
and flow mappings alike.
**Approach (assumed at the header base):** `Parse(src []byte) (*Document, error)` keeps the
source and its `yaml.Node` tree; `(*Document) Set(path Path, value string) error` replaces the
addressed scalar's source span (Line/Column to byte offset, quoted spans included);
`Insert(mapping Path, key, value string, after []string) error` writes `key: value` after the
last present key named in `after` (else as the first key), at the mapping's indent in a block
mapping and as `, key: value` in a flow mapping; `Delete(path Path) error` removes a block key
with its value lines, or a flow entry with its separator. `value` is YAML source text;
`Quote(s string) string` single-quotes a string. `Bytes()` returns the source; every edit
re-parses it so positions stay true. Errors name the path. Never re-encodes the node tree.
One deep module; `Document` the only exported type besides `Path`.
**Regression guard.** Line/Column → byte offset walks the line's runes (yaml.v3 counts Column in runes, not bytes). A scalar's span end is found by scanning the source (single quote with `''` doubling, double quote with backslash escapes, plain to `,`/`}`/`]`/` #`/EOL), never from `len(Value)`. In a block mapping that is a sequence item (`- id: 1`), an `Insert` with no `after` key present goes after the first key.
**Files:** internal/yamledit/doc.go, internal/yamledit/document.go, internal/yamledit/document_test.go, internal/yamledit/docmap_test.go, internal/yamledit/testdata/commented.yaml
**Read first:** internal/storyboard/storyboard.go — Load; internal/storyboard/storyboard_test.go — validBase; internal/testkit/docmap.go — CheckDocMap;
internal/storyboard/docmap_test.go — TestDocMap
**Tests:** round trip on `testdata/commented.yaml` (comments, blank lines, flow `frame:` and
`do:` entries, quoted scalars): each of set / insert-block / insert-flow / delete-block /
delete-flow yields the original bytes except the edited span (compare with an expected
golden string); missing path and wrong node kind error; `commented.yaml` carries a flow mapping with a
non-ASCII quoted scalar before the edited key; `Set` on a quoted scalar holding `''` and an
escape; insert-block into a `beats[i]` mapping.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/yamledit/` ; `go vet ./internal/yamledit/`
**Commit:** `feat(yamledit): splice scalar, insert and delete edits into YAML source`

## 4. tune: storyboard edits — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the signature is `apply(src []byte, path string, edit Edit) ([]byte, *storyboard.Storyboard, error)`, not the assumed `apply(src, edit) ([]byte, error)`. The Goal asks for the bytes and the parsed board together, and `storyboard.Parse` needs the storyboard's path (for error names and resolving Ship). `Edit` has JSON tags `beat`, `field`, `value`, matching the binding wire contract.
NOTES (2026-10-17): added internal/tune/value.go, which the plan's Files line does not name. It holds the value-spelling parsers (ms, frame integer, target flow text) and the duration formatters, because edit.go alone ran to 453 lines and the house limit is about 400. doc.go's file map names it.
NOTES (2026-10-17): a `zoom.target` edit on a block `zoom:` whose only key is `target` deletes the whole zoom and inserts it again as `zoom: {target: …}` in beat field order. The planned Delete + Insert(zoom, "target") would leave `zoom:` empty (null), and yamledit cannot insert into a null value. Block zooms with other keys, and all flow zooms, use Delete + Insert as planned.
NOTES (2026-10-17): "defaults never inserted" is enforced: setting a duration or ramp to the default of a key that is absent (hold 0, duration 0, zoom.in/zoom.out 1s) changes no bytes. `cut false` and `zoom.factor auto` on an absent key also change nothing. A key that is present is always set, even to its default.
NOTES (2026-10-17): `zoom.factor "0"` is refused as a spelling error (auto is spelled "auto"). Factors are written in Go's shortest decimal form ("1.80" is written as 1.8). A target value must be a one-line, braced flow mapping with no comment, and must decode strictly into storyboard.Target. Misspelled values give `<beat N: >field: want …`. Validation refusals wrap *storyboard.ValidationError.

Depends on items 1 and 3.

**What:**
**Goal:** `internal/tune` turns a typed edit into new storyboard bytes and the parsed
storyboard they decode to, under the ratified write format; the bytes are what Save will write
and the parsed board is what the preview shows.
**Approach (assumed at the header base):** `Edit{Beat int; Field string; Value string}` with
fields `duration`, `hold`, `cut`, `zoom` (add: Value is a target YAML; remove: empty),
`zoom.target`, `zoom.factor`, `zoom.in`, `zoom.out`, `frame.width`, `frame.fps`; Beat is the
beat's `id`, located in `beats` by its `id` node. `apply(src []byte, edit Edit) ([]byte,
error)` uses `yamledit`. Durations: whole ms; an old value ending `ms` is written as `<n>ms`,
one of the form `<number>s` as seconds with the fewest decimals, else `time.Duration.String()`.
A new key goes in struct field order (beat: `duration`, `hold`, `cut`, `zoom`; zoom: `target`,
`factor`, `in`, `out`). `cut` off and `zoom.factor` auto delete their key; zoom add inserts a
flow mapping `zoom: {target: {...}}`. After the splice, `storyboard.Parse` must accept the
bytes, else the edit is refused with its validation error. No HTTP here.
**Regression guard.** The `after` list for a beat key is every Beat field before it in struct order (`id, title, why, notes, do`, then `duration, hold, cut`). `zoom.target` is `Delete` + `Insert(zoom, "target", value, nil)` (item 3's `Set` replaces scalars only), in block and flow `zoom:` alike, so item 3's flow `Insert` must handle the first-key position. The beat is located by its index in `storyboard.Parse(src).Beats` (decode order is source order); yamledit's surface is not widened. `zoom.in`/`zoom.out` of 0 are refused with a message naming the 1s default.
The edit wire contract is binding: JSON `{"beat": <id>, "field": "<name>", "value": "<string>"}`; beat is the beat's id, 0 for frame.width/frame.fps; value spellings: durations (duration, hold, zoom.in, zoom.out) a decimal integer of milliseconds ("1850"); cut "true" or "false"; zoom.factor a decimal ("1.8") or "auto"; frame.width and frame.fps a decimal integer; zoom.target and zoom (add) a target as YAML flow text, exactly the `zoom` snippet a Proposal returns (`{text: '…', area: list, nth: 2}`); zoom remove is field "zoom" with value ""; any other spelling is refused with a message naming the field; a table test covers each spelling and one bad spelling per kind.
**Files:** internal/tune/doc.go, internal/tune/edit.go, internal/tune/edit_test.go, internal/tune/docmap_test.go, internal/tune/testdata/board.yaml
**Read first:** internal/storyboard/storyboard.go — applyDefaults; internal/storyboard/beat.go — Beat, Zoom, Target, DefaultZoomRamp;
internal/storyboard/validate.go — Beat.validate, Zoom.validate; examples/hello/hello.yaml — flow frame and do entries
**Tests:** table per field over `testdata/board.yaml` (comments, flow `frame:`): the expected
bytes differ only at the edited value; unit kept (`1800ms`→`1850ms`, `2s`→`2.35s`); insert
order; cut off and auto factor delete; an edit making `hold >= duration` is refused; `duration`
inserted into a cut beat that has none; `zoom.target` on a block and a flow `zoom:`
(`{target: {...}, factor: 2}`); `zoom.in` of 0 refused naming the 1s default.
Wire spellings table: each value spelling above accepted (`"1850"`, `"true"`/`"false"`,
`"1.8"`/`"auto"`, a frame integer, a target flow mapping, zoom remove `""`) and one bad
spelling per kind refused with a message naming the field.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/` ; `go vet ./internal/tune/`
**Commit:** `feat(tune): apply timing and zoom edits to storyboard source`

## 5. tune: session state, history and save — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the history type sits in its own internal/tune/history.go (not in the item's Files) — one primary type per file, and it keeps session.go near the ~400-line limit; doc.go maps it.
NOTES (2026-10-17): the on-disk baseline is the storyboard's bytes compared with bytes.Equal rather than a hash: the same check, and Revert needs the bytes anyway; the mode is read from the file at save time.
NOTES (2026-10-17): a rewritten take is read again and rebuilt on in Refresh whether or not edits are unsaved (it cannot conflict with them); only a changed storyboard turns a dirty session stale, and an unreadable or mis-sized take stands as a clip error (beat 0) that blocks Save until the take is rewritten.
NOTES (2026-10-17): an edit apply() refuses (misspelled, missing beat, invalid result) is returned by Apply and changes nothing — no history entry, no standing error; only a build failure on accepted bytes (schedule, compositor, cross-check) stands as an error and keeps the last good pipeline.
NOTES (2026-10-17): Revert pushes the on-disk bytes as a new version, so Undo brings the discarded edits back; with nothing unsaved it does nothing.
NOTES (2026-10-17): NewSession fails when the first build fails (as render would), so a session always has a good pipeline for items 6 and 7; the pipeline struct (board, clip, take, schedule, rasterizer, compositor) is what Frame/Pick will copy under the lock.
NOTES (2026-10-17): State's JSON carries durations as whole ms in `*_ms` fields; sections also carry start/duration/hold/cut for the lanes; problems are split per validation rule and attributed to a beat only when the beat the message names exists.
NOTES (2026-10-17): golangci-lint is not installed on this host, so only gofmt and go vet ran.

Depends on items 2 and 4.

**What:**
**Goal:** a `Session` holds one clip and take being tuned: the current bytes and their parsed
board, the schedule and compositor built from them (and the last good pair), per-beat and clip
errors, undo/redo/revert, reload on disk change, and a guarded save; `State()` reports
everything the page shows.
**Approach (assumed at the header base):** `NewSession(loaded *clip.Clip, takePath string)`
loads the take (refuse a take of another frame as `render` does), builds the rasterizer once
via `render.NewRasterizer`, and rebuilds schedule (`schedule.New`) and compositor
(`render.NewCompositor`) after every change; a build error is kept as the state's error
(attributed to a beat when the message names `beat <id>`), the last good pair stays.
History: a stack of byte versions; `Apply(Edit)`, `Undo`, `Redo`, `Revert`. `Refresh()`
compares the storyboard's hash and the take's size and mtime with the loaded ones: clean →
reload (rasterizer rebuilt only if the take or read-only frame fields changed), dirty → `Stale`.
`Save()` refuses with `ErrStale` (hash differs on disk), `ErrInvalid` (an error stands), else
writes temp + rename keeping the mode. `State` carries beats (id, title, duration, hold, cut,
zoom, read-only pauses), sections with `render.BeatReport` text and `Speed`, schedule
warnings, length, fps, width, errors, dirty, stale, can-undo/redo, a version counter.
**Regression guard.** Session owns one sync.Mutex guarding the bytes, board, schedule, compositor, last-good pair, history and errors; every exported Session method takes it; Frame and Pick copy the current pipeline pointers under the lock and do the raster/compose/PNG work outside it (the raster cache has its own lock, item 6). `Save` sets the on-disk baseline (hash, mode) to the bytes it wrote. `Reload()` drops history, loads the storyboard from disk and clears `Stale`; item 8 routes `/api/reload` to it.
**Files:** internal/tune/session.go, internal/tune/state.go, internal/tune/session_test.go, internal/tune/doc.go
**Read first:** internal/render/render.go — newPlan, loadTake, newCompositor; internal/schedule/schedule.go — New, Warnings;
internal/compose/zoom.go — zoomPlanner.plan; internal/clip/clip.go — Clip.Take; internal/take/file.go — Load
**Tests:** fixture clip + saved take in `t.TempDir()` (as `render_test.go` builds one): apply,
undo, redo, revert; a too-fast section yields the schedule warning; a zoom target not on screen
keeps the last good pipeline and blocks Save; Save writes; an outside write makes clean reload
and dirty stale; Save after outside write → `ErrStale`; Save then Refresh keeps the version and
can-undo; Reload after an outside write clears Stale and Save succeeds; Apply and State run in
parallel.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/` ; `go vet ./internal/tune/`
**Commit:** `feat(tune): keep a tuning session with history, reload and guarded save`

## 6. tune: frames on demand — ✅ DONE (2026-10-17)

NOTES (2026-10-17): internal/tune/session.go, not in **Files:**, carries the regression guard: `pipeline` gains a `rasters *rasterCache` field and `rasterizerFor` returns the good pipeline's cache with its reused rasterizer, or a new empty cache with a new rasterizer, so the old cache is never emptied in place and `Frame` copies it with the rest of the good pipeline under the lock.
NOTES (2026-10-17): Frame draws from the last good pipeline (the preview keeps it while an error stands) and adds an exported sentinel `ErrOutsideClip` for `out` outside [0, Length), so the HTTP item can map it; the LRU cap is injected through `newRasterCache(capacity)` into the cache's unexported `capacity` field, and the eviction tests drive the cache directly.

Depends on item 5.

**What:**
**Goal:** `(*Session).Frame(out time.Duration) ([]byte, error)` returns the PNG of the frame
`photon render` composes at output time `out` for the current storyboard, using a rasterized
snapshot cache bounded at 1 GiB.
**Approach (assumed at the header base):** `schedule.Source(out)` → `render.SnapshotAt` →
cached `rasterizer.Frame` → `compositor.Compose(out, src)` → `png.Encoder{CompressionLevel:
png.BestSpeed}`. The cache is an LRU keyed by snapshot index, sized as `4*w*h` bytes per entry,
cap a named constant `rasterCacheBytes = 1 << 30`, safe for concurrent requests, emptied when
the rasterizer is rebuilt. `out` outside `[0, Length)` is an error.
**Regression guard.** A rasterizer rebuild replaces the raster cache with a new one owned by that rasterizer/take generation, which `Frame` copies under the session lock with the rest of the pipeline; the old cache is never emptied in place, so a `Frame` still running against the old take cannot refill the new one with old rasters.
**Files:** internal/tune/frame.go, internal/tune/cache.go, internal/tune/frame_test.go, internal/tune/doc.go
**Read first:** internal/render/encode.go — writeFrames, snapshotAt; internal/schedule/schedule.go — Schedule.Source, Length;
internal/compose/compose.go — Compositor.Compose, Size; internal/raster/raster.go — Rasterizer.Frame, Size
**Tests:** after an edit to duration, hold, cut and zoom ramp, the decoded PNG at a fixed `out`
equals the frame built independently from the saved file: `clip.Load` → `take.Load` →
`schedule.New` → `render.NewRasterizer`/`NewCompositor`/`SnapshotAt` → `Compose`; and differs
from the pre-edit frame. LRU evicts past a small test cap (cap injectable via an unexported field).
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/` ; `go vet ./internal/tune/`
**Commit:** `feat(tune): serve composed frames with a bounded raster cache`

## 7. tune: target proposal from picked cells

Depends on item 5.

**What:** Recast at the regression check (2026-10-17).
**Goal:** `(*Session).Pick(out time.Duration, from, to image.Point) (Proposal, error)`
proposes a target for the cells of one row picked by a rectangle in output-frame pixels on the
snapshot shown at `out`, mapped through the exported `(*compose.Compositor).Cell(t, p)`, and returns
it only when `target.Resolver.Target` resolves it to a box covering those cells.
**Approach (assumed at the header base):** text = the row's picked text trimmed of blanks,
escaped with `regexp.QuoteMeta`; areas tried: every declared area containing the row, fewest
rows first, the first where the match is unique wins, else the smallest containing area (or
none) with `nth` = the match's index (omitted when 1). `Proposal{Target storyboard.Target; Box
take.Box; Snippets map[string]string}` with YAML for `zoom target`, `click`, `seen` (+`area`),
`drag-from`, `drag-to` (`from: {...}` / `to: {...}` per `DragAction`), quoted via
`yamledit.Quote`. Unresolvable or non-covering → error naming why.
**Regression guard.** Pick takes a rectangle in output-frame pixels, not cells — `Pick(out time.Duration, from, to image.Point) (Proposal, error)`; the pixel→cell mapping lives server-side: item 7 adds an exported `(*compose.Compositor).Cell(t time.Duration, p image.Point) (col, row int, ok bool)` that inverts the compositor's scale and zoom transform at output time t, with a compose table test (unzoomed, mid-ramp, fully zoomed, outside the screen → ok false); the picked row is the one under `from`, columns span from→to clamped to that row; item 7's Files gain internal/compose/compose.go (or zoom.go, wherever the transform lives) and internal/compose/compose_test.go. The box must cover the picked cells after edge blanks are trimmed (only an all-blank pick errors); areas of equal height are tried in name order.
The snippet keys are exactly `zoom`, `click`, `seen`, `drag-from`, `drag-to`; the `zoom` snippet is the bare target flow mapping (`{text: '…', nth: N, area: …}`), which item 11 posts verbatim as Edit.Value for `zoom` (absent) or `zoom.target` (present), per item 4's wire contract.
Item 7 also adds an exported `(*compose.Compositor).Rect(t time.Duration, b take.Box) image.Rectangle` returning the box's output-frame pixel rectangle at output time t (built on the unexported boxRect, crop and toOutput in internal/compose/compose.go), with a compose table test (unzoomed, fully zoomed); `Proposal` gains `Rect image.Rectangle` (output pixels at the picked time).
**Files:** internal/tune/pick.go, internal/tune/pick_test.go, internal/tune/doc.go, internal/compose/compose.go, internal/compose/compose_test.go
**Read first:** internal/compose/compose.go — Compositor.crop, Compositor.toOutput, Layout.boxRect; internal/target/resolver.go — Resolver.Target, pickMatch, rowsOf;
internal/target/anchor.go — area.rows; internal/compose/compose_test.go — newTestCompositor, TestComposeZoomedCursorMapsToTheZoomedTarget;
internal/storyboard/beat.go — Target, Nth, DragAction
**Tests:** table: unique text (no area, no nth); text repeated (nth); repeated text unique in
an area (area, no nth); regex metacharacters escaped; a blank pick errors; every proposal's box
covers the picked cells; a pick with an edge blank; two equal-height areas (name order wins); the `zoom` snippet is a
flow mapping item 4's `zoom.target` edit accepts.
`Cell` table test in compose: unzoomed, mid-ramp, fully zoomed, outside the screen → ok false.
`Rect` table test in compose: unzoomed, fully zoomed.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/compose/` ; `GOMEMLIMIT=2GiB go test ./internal/tune/` ; `go vet ./internal/compose/ ./internal/tune/`
**Commit:** `feat(tune): propose resolver-checked targets from picked cells`

## 8. tune: local HTTP server

Depends on items 6 and 7.

**What:** Recast at the regression check (2026-10-17).
**Goal:** `tune.Run(ctx, loaded *clip.Clip, options Options, stdout, stderr io.Writer) error`
listens on `127.0.0.1:<port>` only, prints `photon tune: http://127.0.0.1:<port>/`, serves the
embedded page and a JSON API over the session, and returns nil when `ctx` ends.
**Approach (assumed at the header base):** `Options{Take string; Port int}`. Routes:
`GET /` and `GET /assets/*` (`//go:embed page`), `GET /api/state` (calls `Refresh` first),
`GET /api/frame?t=<ms>` (`image/png`), `POST /api/edit|undo|redo|revert|reload|save|pick`.
Requests whose `Host` is not `127.0.0.1:<port>` or `localhost:<port>` get 403; POST without
`Content-Type: application/json` gets 415; `ErrStale` → 409, `ErrInvalid` and refused edits →
422 with the message. `http.Server` with named timeouts; `Shutdown` on ctx done. Item 9 owns
the page; this item adds a placeholder `page/index.html`.
**Regression guard.** `POST /api/reload` calls `(*Session).Reload()` (item 5). Every route relies on the Session mutex (item 5: every exported Session method locks). The handler is built from the listen address, `newHandler(session, addr)`, called by `Run` after `net.Listen`; handler tests set `req.Host` to that address, and only the 403 test does not.
The pick wire contract is binding: POST /api/pick body `{"t": <output ms>, "from": {"x": <px>, "y": <px>}, "to": {"x": <px>, "y": <px>}}` in output-frame pixels; 200 response `{"target": {"text": "…", "nth": <int, omitted when 1>, "area": "<omitted when none>"}, "box": {"x","y","w","h"} (cells), "snippets": {"zoom", "click", "seen", "drag-from", "drag-to"}}`; a failed proposal is 422 `{"error": "<message>"}`; item 8's Tests gain a handler test that posts a pick on the fixture and asserts the box and the five snippet keys, and one that gets 422 for a blank pick.
The pick response's binding contract gains `"rect": {"x","y","w","h"}` in output-frame pixels beside `box` (cells); the pick handler test also asserts rect is non-empty and inside the frame size.
**Files:** internal/tune/server.go, internal/tune/run.go, internal/tune/server_test.go, internal/tune/page/index.html, internal/tune/doc.go
**Read first:** internal/render/render.go — Run, Options; cmd/photon/render.go — newRenderCommand; internal/compose/compose.go — Compositor;
cmd/photon/main.go — execute; internal/testkit/docmap.go — CheckDocMap
**Tests:** `httptest` against the handler: state JSON; frame PNG; edit then frame changes;
save 409 after an outside write; Reload clears that 409; a frame GET and an edit POST fired
concurrently; foreign Host 403; form POST 415; a pick POST on the fixture returns the box, a non-empty
`rect` inside the frame size and the five snippet keys; a blank pick gets 422. `Run` on port 0 listens on
`127.0.0.1` (assert the listener address) and stops on cancel.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/` ; `go vet ./internal/tune/`
**Commit:** `feat(tune): serve the tuning session on 127.0.0.1`

## 9. Page: preview, inspector and transport

Depends on item 8.

**What:** Recast at the regression check (2026-10-17).
**Goal:** the page files (`index.html`, `style.css`, `app.js`) are embedded, every asset
`index.html` references is served by the handler, no embedded file fetches an external URL,
and `app.js` defines `window.tune` with every contract member (the test greps the names).
**Approach (assumed at the header base):** `page/index.html`, `page/style.css`, `page/app.js`
(no framework) show the ratified layout's preview, inspector and transport bar. One frame
request in flight; play runs in real time, picking the frame at wall-clock position and
dropping frames not ready; the inspector's fields edit through the API.
Inspector: the selected beat's fields (durations as ms inputs; factor with an "auto" box; zoom
add/remove; read-only pauses marked "needs photon record"; the beat's `BeatReport` row, red
above 6×; its error), else `frame.width`/`fps` editable and the other frame fields read-only.
Transport: play, ⏮/⏭ (previous/next section start), time, Save, undo, redo, Revert. Keys per
the header. Polls `/api/state` every second; stale banner with Reload. Exposes
`window.tune = {state, select(id), seek(ms), edit(e)}` for items 10 and 11.
**Regression guard.** item 9 defines the page's JS contract as binding text: `window.tune = {state() (the last /api/state JSON), selected() (beat id or null), select(id), time() (output ms), frameTime() (output ms of the frame in the <img>), seek(ms), pause(), edit(edit) (Promise of the new state), onState(fn), frame (the preview <img> element)}`; items 10 and 11 use only this contract; page items' Goals are verified by the asset test; play and edit behaviour are checked by the manual steps under Tests. The asset test's external-URL check matches a URL in fetch position only, by the pattern `((src|href)\s*=\s*["']?|url\(\s*["']?|(from|import)\s*\(?\s*["']|fetch\(\s*["'])(https?:)?//`; SVG/XML namespace URIs are allowed. The inspector sends Edit.Value only in item 4's binding wire spellings (durations a bare ms integer, factor `auto`, cut `true`/`false`).
**Files:** internal/tune/page/index.html, internal/tune/page/style.css, internal/tune/page/app.js, internal/tune/page_test.go
**Read first:** internal/schedule/schedule.go — Section, Schedule; internal/render/render.go — writeSummary;
internal/compose/compose.go — Compositor.Size; internal/storyboard/beat.go — Beat, Zoom; examples/hello/hello.yaml — frame, beats
**Tests:** `page_test.go`: every embedded file referenced by `index.html` is served; no
embedded file has a URL in fetch position (the guard's pattern; the table includes a quoted
`<script src="https://…">` and an ES `import x from 'https://…'`); SVG/XML namespace URIs
allowed; `app.js` names every `window.tune` contract member.
Manual: run `photon tune examples/hello/hello.yaml` on a recorded take, edit a duration, play.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/` ; `go vet ./internal/tune/`
**Commit:** `feat(tune): add the preview, inspector and transport page`

## 10. Page: timeline lanes

Depends on item 9.

**What:**
**Goal:** below the transport, lanes `beats`, `hold` and `zoom` on an output-time axis show
every section, its hold and its zoom ramps; dragging a section's end, its hold edge or a ramp
edge edits the value (50ms snap); a cut beat is a hatched marker that selects the beat; a
section above 6× is flagged; click or drag on the axis scrubs.
**Approach (assumed at the header base):** `page/timeline.js` builds lanes from one list of
lane descriptors (`{name, draw(section), handles(section)}`), so another lane is one more
descriptor. Drags preview locally and send one edit on release.
**Regression guard.** `timeline.js` uses only item 9's `window.tune` contract (`time()` for the playhead, `onState(fn)` to redraw after each poll, edit or undo, plus `select`, `seek`, `edit`); app.js is not edited. The Goal is verified by the asset test plus the manual steps under Tests (item 9's guard).
**Files:** internal/tune/page/timeline.js, internal/tune/page/index.html, internal/tune/page/style.css
**Read first:** internal/tune/page/app.js — window.tune (item 9); internal/schedule/schedule.go — Section; internal/render/render.go — writeSummary;
examples/hello/hello.yaml — beats
**Tests:** the item 9 asset test covers the new file. Manual: drag each handle on the hello
clip; undo restores.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/`
**Commit:** `feat(tune): add draggable timeline lanes`

## 11. Page: target picker

Depends on item 10.

**What:** Recast at the regression check (2026-10-17).
**Goal:** `picker.js` is embedded, referenced by `index.html` and served, and contains
`/api/pick` and `navigator.clipboard.writeText` (the test greps both).
**Approach (assumed at the header base):** `page/picker.js`: the ⌖ button arms pick mode on the
paused preview; dragging across cells of one row calls `/api/pick`, draws the resolved box,
shows the proposal, and offers "Set as zoom target" (an edit on the selected beat, adding the
zoom if absent) and copy buttons for click, seen, drag-from and drag-to snippets
(`navigator.clipboard.writeText`).
**Regression guard.** the picker converts the drag's display coordinates to output-frame pixels with the <img>'s natural/displayed size ratio and posts them to /api/pick; no geometry is added to State and internal/tune/state.go leaves item 11's Files. The pick posts item 9's `frameTime()` (the output ms of the frame in the <img>) as `t`, never `time()`.
The picker draws the resolved box from the response's `rect`, scaled to the displayed <img> by the displayed/natural size ratio.
**Files:** internal/tune/page/picker.js, internal/tune/page/index.html, internal/tune/page/style.css
**Read first:** internal/compose/compose.go — Compositor.crop, Compositor.toOutput, Layout.boxRect; internal/target/resolver.go — Resolver.Target;
examples/hello/hello.yaml — beat 3 click target `beta`; examples/hello/photon.yaml — areas list, status; internal/storyboard/beat.go — Zoom, Target
**Tests:** asset test covers the file and greps it for `/api/pick` and
`navigator.clipboard.writeText`; the pixel→cell mapping is item 7's `Cell` table test, the box's
pixel rectangle item 7's `Rect` table test.
Manual: pick `beta` on the hello clip,
set as zoom target, Save, `git diff` shows only the zoom line.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/tune/` ; `go vet ./internal/tune/`
**Commit:** `feat(tune): pick targets from the preview`

## 12. `photon tune` command and docs

Depends on item 8.

**What:**
**Goal:** `photon tune <storyboard> [<take>]` with `--port`, `--config`, `--work` runs
`tune.Run`; README's command table and "The commands" section, Note `package-layout`, and the
doc maps describe `tune` and `yamledit`.
**Approach (assumed at the header base):** `cmd/photon/tune.go` mirrors `render.go`
(`clip.Load`, `runE`, exit statuses paragraph in Long); registered in `newRootCommand`. Note
rows: `internal/yamledit` (YAML source splicing; imports none) and `internal/tune` (the timing
editor; imports `clip`, `render`, `schedule`, `compose`, `raster`, `target`, `storyboard`,
`take`, `yamledit`, `config`); nothing imports `tune`.
**Regression guard.** `cmd/photon/tune.go` wraps `cmd.Context()` with `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` (main passes `context.Background()` and nothing calls `signal.Notify`); Long and README say Ctrl-C exits 0. README's section is `## Commands`: every row of its command, Flags and Exit-status tables that lists commands gains `tune` (grep `render` in README.md:132-171), and Flags gains a `--port` row.
**Files:** cmd/photon/tune.go, cmd/photon/root.go, cmd/photon/doc.go, cmd/photon/tune_test.go, README.md, .capcom/knowledge/package-layout.md
**Read first:** cmd/photon/render.go — newRenderCommand; cmd/photon/root.go — newRootCommand; cmd/photon/main.go — execute; cmd/photon/exit.go — runE, exitCodeFor;
cmd/photon/readme_test.go — TestREADMEDocumentsEveryCommand; .capcom/knowledge/package-layout.md — package table; README.md — ## Commands
**Tests:** `tune_test.go`: bad args exit 2; `--port` parsed. README tests cover the command
and flags.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./cmd/photon/` ; `go vet ./cmd/photon/` ; `capcom check` ; `grep -n 'internal/tune' .capcom/knowledge/package-layout.md`
**Commit:** `feat(tune): add the photon tune command and document it`
