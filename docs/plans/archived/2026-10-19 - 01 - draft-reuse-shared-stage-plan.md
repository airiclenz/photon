# Draft reuse shared stage — implementation plan

**Goal:** A rebuild whose new draft geometry equals its new raster geometry always shares the new
exact rasterizer and raster cache as its draft stage, whatever draft stage the reused pipeline had.
**Date:** 2026-10-19
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** ef4242d

**Regression check (2026-10-19, ef4242d):**
- 1: guard folded (no-puts check on the previous pipeline's own draft cache; widened comment grep); writer decision applied — `isSharedStage` clause kept

**Sources:**
- `.capcom/issues/draft-reuse-shared-stage.md` (defect and acceptance)
- `docs/plans/archived/2026-10-19 - 00 - draft-frame-speed-plan.md` (item 1, the draft stage this fixes)
- Notes `coding-conventions`, `package-layout`, `glossary`

**Ratified design calls (writer, 2026-10-19):**
- **Scope:** of the ready issues, only `draft-reuse-shared-stage` has code work left.
  `tune-page-js-check`'s parse criterion is met by `TestPage_Scripts_Parse` (goja, `18d7c88`);
  its manual `photon tune` session stays the user's. `tune-preview-playback-speed` waits only on
  the user's by-eye playback check. Both stay open and are not named on any `**Closes:**` line.

**Standing requirements:**
- skills: coding-standards
- Tests on this host: single packages, `GOMEMLIMIT=2GiB`, never `./...` with `-race`, never two at once.
- No committed file names the reference implementation (AGENTS.local.md forbidden names).

**Out of scope:** the draft geometry calculation; exact-stage reuse (`canReuseRasters`); the tune
page and server; the manual checks named above.

## 1. Share the new exact stage before reusing a previous draft stage — ✅ DONE (2026-10-19)

**What:** Fixes the defect filed as `draft-reuse-shared-stage` (deferred from plan
`2026-10-19 - 00 - draft-frame-speed-plan` item 1): a pipeline with its own draft stage, rebuilt
into one whose draft geometry is its raster geometry, keeps the old draft rasterizer and draft
cache, so each snapshot is rasterized twice.
**Regression guard.** The no-puts check must bite: also assert that drawing every draft adds no puts
to the previous pipeline's own draft cache (a `countingCache` from the first Build that has drawn
nothing yet); before the item it gets one put per snapshot. Find the comments to update with
`grep -n "draft stage\|draft rasterizer and draft cache\|draft rasterizer and its\|draft rasterizer and the cache\|exact rasterizer and cache" internal/render/*.go` (it catches the `draftRastersFrom` doc comment the Approach's grep misses), and treat the rule as every comment that orders draft reuse against the exact stage.
**Goal:** in `internal/render`, whenever a built pipeline's draft geometry equals
`rasterGeometry` of its frame, its `draftRasterizer` is its `rasterizer` and its `draftCache` is
its raster cache — with or without `BuildOptions.Reuse`, and whatever draft stage the reused
pipeline held. A draft stage of the previous pipeline is reused only when the new draft geometry
differs from the new raster geometry and the existing reuse conditions hold.
**Approach (assumed at the header base):** in `Pipeline.draftRastersFrom`
(`internal/render/pipeline.go`), test `geometry == rasterGeometry(p.loaded.Storyboard.Frame)` and
return `rasterizer, cache` before asking `canReuseDraftRasters`. Keep `canReuseDraftRasters`'s
`isSharedStage` clause and its doc sentence — it stays reachable after the reorder (a previous
shared stage rebuilt as `narrowedStoryboard` has draft geometry equal to the previous raster
geometry but not the new one). Update every comment that states the draft-stage reuse order or claims the draft
stage is the exact stage — find them with
`grep -n "draft stage\|draft rasterizer and draft cache\|draft rasterizer and its" internal/render/*.go`
(at base: the `draftRastersFrom` and `canReuseDraftRasters` doc comments, `BuildOptions`, and the
package comment in `internal/render/doc.go`).
**Files:** `internal/render/pipeline.go`, `internal/render/doc.go`, `internal/render/pipeline_draft_test.go`
**Read first:** `internal/render/pipeline.go` — draftRastersFrom, canReuseDraftRasters, BuildOptions; `internal/render/pipeline_draft_test.go` — TestBuild_Reuse_ASharedDraftStage_CarriesOverOnlyWithItsExactStage, narrowedStoryboard, newCountingCache, drawEveryDraft; `internal/render/doc.go` — package comment
**Tests:** add a case to `internal/render/pipeline_draft_test.go`: build from `narrowedStoryboard`
with a draft cache (asserting its draft stage is its own, not its exact stage), rebuild into
`rescaledStoryboard(1)` with `Reuse`, assert `built.draftRasterizer == built.rasterizer` and
`built.draftCache` is the new exact cache, and that drawing every draft puts nothing into the
draft cache passed to the rebuild nor into the previous pipeline's own draft cache (a
`countingCache` from the first Build that has drawn nothing yet; before the item it gets one put
per snapshot). The existing reuse tables
(`TestBuild_Reuse_KeepsTheDraftRastersWhileTheDraftGeometryHolds`,
`TestBuild_Reuse_ASharedDraftStage_CarriesOverOnlyWithItsExactStage`) keep passing unchanged.
**Acceptance:**
- `go build ./...`
- `GOMEMLIMIT=2GiB go test ./internal/render/ -run 'TestBuild|TestPipeline_DraftAt|TestDraftGeometry|TestDocMap'`
- `GOMEMLIMIT=2GiB go test ./internal/render/`
- `go vet ./internal/render/`
- The new test fails against the pre-item `internal/render/pipeline.go`.
**Commit:** `fix(render): share the exact stage on a draft-reusing rebuild`
**Closes:** draft-reuse-shared-stage
