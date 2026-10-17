# bounded render memory — implementation plan

**Goal:** `photon render` holds a constant amount of memory however long the clip, by encoding in
two ffmpeg passes (palette, then GIF) instead of one graph that buffers every frame. The GIF
stays byte-identical to the one-pass encode.
**Date:** 2026-10-17
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** fbff5e1
**Closes:** render-memory-bound

**Sources:**
- `.capcom/issues/render-memory-bound.md` (the issue; its `## Acceptance` is the plan's acceptance)
- Notes `coding-conventions`, `package-layout`

**Ratified design calls** (user, 2026-10-17):
- **Frames for pass 2:** re-rastered and re-composed (the frame stream runs twice); nothing is spooled to disk.
- **Palette file:** `<work>/<clip>.palette.png` (the clip's work dir, `.palette.png` after the clip name), removed after the render, success or failure.
- **Dry run:** prints both command lines, palette pass first, one per line.

**Standing requirements:**
- skills: coding-standards
- Tests per AGENTS.md: single packages, `GOMEMLIMIT=2GiB`, never `./...` with `-race`, never two test commands at once.
- No committed file names anything matching the forbidden regex in `AGENTS.local.md`.
- Nothing is written under `/tmp` by production code (RAM on the Pi).

**Out of scope:**
- a Go-side palette; a lossless frame spool; any change to `dither`, `max_colors` or gifsicle
- raster or compose speed-ups

**Regression check (2026-10-17, fbff5e1):**
- 1: guard folded (work dir resolved for every render; shared per-clip palette accepted)
- 2: guard folded (bounded-colour synthetic frames)
- 3: guard folded (render.go Run comment, work-dir sites, whole-word tree-only grep) and decision applied (cmd/photon/render_test.go needs no edit)

## 1. Two-pass palette encode — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the `shellLine` doc comment in internal/render/encode.go said "the shell would split"; reworded to "break apart" so the Acceptance `grep -n 'split' internal/render/encode.go` prints nothing (comment text only).
NOTES (2026-10-17): `TestRunMatchesTheOnePassEncode` passed on the first argv (no pixel-format or PNG options needed); `onePassArgs` holds the old argv verbatim rather than reusing `rawInputArgs`, so the reference cannot drift with the production prefix.

**What:**
**Goal:** `photon render` runs ffmpeg twice — pass 1 `palettegen=max_colors=N` from raw RGBA on stdin into `<work>/<clip>.palette.png`; pass 2 raw RGBA on stdin plus that PNG through `paletteuse=dither=none` into the GIF — with no `split` in either graph; the GIF is byte-identical to the one-pass graph's for the same take; the palette file is gone after `Run` returns; `--dry-run` prints both lines and creates nothing.
Fix for issue `render-memory-bound`: the one-pass graph `[0:v]split[a][b];[a]palettegen…[p];[b][p]paletteuse…` buffers every raw frame until end of input.
**Approach (assumed at the header base):** in `internal/render/encode.go` replace `ffmpegArgs` with `paletteArgs(size, frame, palette)` (`… -i pipe:0 -vf palettegen=max_colors=%d -update 1 <palette>`) and `encodeArgs(size, frame, palette, out)` (`… -i pipe:0 -i <palette> -filter_complex [0:v][1:v]paletteuse=dither=%s <out>`), sharing the raw-input argument prefix in one helper. `plan.encode` stays the one streaming runner and is called once per pass; `writeFrames` is unchanged (rasterizer and compositor are pure per frame). In `render.go` `newPlan` resolves the palette path from `loaded.Work()` plus a `paletteSuffix = ".palette.png"` constant and keeps it on `plan`; `Run` on a dry run prints `shellLine` of each argv; otherwise it creates the output dir and the work dir, runs pass 1 then pass 2 (pass 2 never runs when pass 1 fails), and removes the palette in a deferred step whose error (other than not-exist) is joined into `Run`'s, never discarded. Update the `ffmpegProgram` / `ffmpegArgs` comments to the two passes.
If the byte-identity test fails, fix the argv (pixel formats, PNG options); never relax the test.
**Regression guard.** Accepted: `newPlan` resolves `loaded.Work()` for every render, an explicit take included, so a render whose work dir cannot resolve (no `--work`, `$PHOTON_WORK`, config `work` or HOME) now fails, dry run included; a real run creates the work dir with `0o755`, the same mode as the rig (`internal/rig/setup.go` `workDirMode`). Accepted limit under the ratified palette path: two concurrent renders of one clip share `<work>/<clip>.palette.png` and can clobber each other; keep the ratified name, do not make it unique.
**Files:** internal/render/encode.go, internal/render/render.go, internal/render/render_test.go
**Read first:** internal/render/render.go — Run, newPlan, plan, loadTake; internal/render/encode.go — ffmpegArgs, plan.encode;
internal/clip/clip.go — Clip.Work; internal/render/render_test.go — TestRunDryRunPrintsTheFfmpegLine
**Tests:** `render_test.go`: `TestRunDryRunPrintsTheFfmpegLine` pins both lines (palettegen line ending in `<work>/rendered.palette.png`, paletteuse line ending in the GIF path, no `split`) and that neither the GIF nor the palette exists; `TestRunDryRunDefaultsToTheShipPath` still ends in the ship path. New `TestRunMatchesTheOnePassEncode` (skips without ffmpeg): renders the fixture take through `Run` with gifsicle kept off PATH (`t.Setenv("PATH", <dir holding only an ffmpeg symlink>)`, no `t.Parallel`), runs the same frames through a test-only `onePassArgs` holding the old graph verbatim, and compares the GIF bytes; asserts the palette file is gone. `TestRunWritesAGIFOfTheScheduledLength` unchanged and passing.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render` passes; `go vet ./internal/render`; `grep -n 'split' internal/render/encode.go` prints nothing.
**Commit:** `fix(render): encode in two ffmpeg passes so memory stays flat`

## 2. Peak-memory test for both passes — ✅ DONE (2026-10-17)

Depends on item 1.

**What:**
**Goal:** a Linux-only test in `internal/render` proves each pass's ffmpeg peak RSS stays flat as the frame count grows, and that the measurement bites: the test-only one-pass graph's peak grows with it.
**Approach (assumed at the header base):** new file `internal/render/memory_linux_test.go`. A helper streams n synthetic 320×200 RGBA frames (content varies per frame) into a command line and returns `cmd.ProcessState.SysUsage().(*syscall.Rusage).Maxrss` (KiB on Linux). For `paletteArgs` and `encodeArgs` (pass 2 needs a palette: run pass 1 first into `t.TempDir()`), peak at 400 frames minus peak at 40 frames is under 16 MiB; for `onePassArgs` (from item 1) it is over 50 MiB. Skips without ffmpeg; frame size, counts and thresholds are named constants in the file.
**Regression guard.** The synthetic frames change per frame over a small fixed colour set (a flat background plus a moving bar of a few colours), never random noise: noise grows palettegen's histogram and paletteuse's colour cache with the distinct colours (probe: pass 1 204 MB -> 1.32 GB at 40 -> 400 frames), failing the bound and threatening the Pi.
**Files:** internal/render/memory_linux_test.go (new)
**Read first:** internal/render/encode.go — paletteArgs, encodeArgs (from item 1), ffmpegProgram; internal/render/render_test.go — onePassArgs (from item 1);
os/exec — Cmd.ProcessState.SysUsage, syscall.Rusage.Maxrss; internal/testkit/docmap.go — unmapped (skips _test.go, so no doc.go entry needed)
**Tests:** the file itself.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render -run TestEncodePeakMemory -v` passes and logs the six peaks.
**Commit:** `test(render): ffmpeg peak memory stays flat as the clip grows`

## 3. Describe the two-pass encode — ✅ DONE (2026-10-17)

NOTES (2026-10-17): internal/render/render.go needs no edit: item 1 already wrote Run's doc comment ("prints the two ffmpeg command lines instead, the palette pass first, one per line"), so it is absent from FILES.

Depends on item 1.

**What:**
**Goal:** every user-facing and package description of the render pipeline names the two passes and the palette file in the work dir, and `--dry-run` is described as printing the ffmpeg command lines.
**Approach (assumed at the header base):** `internal/render/doc.go` package comment: frames streamed twice, pass 1 a per-clip palette into `<work>/<clip>.palette.png`, pass 2 the GIF, the palette removed after; dry run prints both lines; `encode.go` file-map entry names both passes. `cmd/photon/render.go`: `Long` and the `--dry-run` flag help say "command lines". `README.md`: the `--dry-run` row and the render pipeline paragraph (near "streams the frames into ffmpeg for a per-clip palette"). `cmd/photon/render_test.go`: update any dry-run expectation the two-line output breaks. Rule for missed sites: every prose site describing a single ffmpeg command line or the frames streamed once, found by `grep -rn -i 'ffmpeg command line\|streams the frames' --include=*.go --include=*.md . | grep -v docs/plans/archived`.
**Regression guard.** cmd/photon/render_test.go dry-run rows match with strings.Contains on the shared raw-input prefix and the trailing " <gif>\n", so they pass after item 1 unchanged — drop cmd/photon/render_test.go from item 3's Files and Tests, and make its acceptance the existing TestRender and TestREADME runs passing unchanged.
Also: `internal/render/render.go` `Run`'s doc comment says it "prints both command lines, palette pass first"; the README work-dir paragraph names the transient `<work>/<clip>.palette.png` among what the work dir holds, and `cmd/photon/render.go`'s `--work` help reads "where the take is read and the palette written"; `doc.go` notes that concurrent renders of one clip share the palette file (item 1's accepted limit). The rule's grep is `grep -rnw -i -e 'ffmpeg command line' -e 'streams the frames' -e 'into ffmpeg' -e 'work dir' --include=*.go --include=*.md cmd internal README.md` (whole-word, tree only).
**Files:** internal/render/doc.go, internal/render/render.go, cmd/photon/render.go, README.md
**Read first:** internal/render/doc.go — package comment, file map (encode.go entry); cmd/photon/render.go — newRenderCommand Long, dry-run and work flag help;
internal/render/render.go — Run doc comment; README.md — command flag table (--dry-run row), work dir paragraph, render pipeline paragraph (before "## Environment variables");
cmd/photon/readme_test.go — TestREADMEDocumentsEveryCommand
**Tests:** the existing `TestRender` and `TestREADME` runs in `cmd/photon` pass unchanged.
**Acceptance:** `GOMEMLIMIT=2GiB go test ./internal/render -run TestDocMap`; `GOMEMLIMIT=2GiB go test ./cmd/photon -run 'TestRender|TestREADME'` passes unchanged; the guard's grep prints no hit that describes one pass or a work dir without the palette file.
**Commit:** `docs(render): describe the two-pass palette encode`
