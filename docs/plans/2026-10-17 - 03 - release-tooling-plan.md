# Release tooling plan

**Goal:** photon can be released from `VERSION` alone: a CHANGELOG, a workflow that tags `v`+VERSION
when `VERSION` changes on `main`, and a `cmd/release` tool that builds the archives, publishes the
GitHub release, the Homebrew formula and the Scoop manifest, reports status and smoke-tests them.
**Date:** 2026-10-17
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 4c5822b

**Regression check (2026-10-17, 4c5822b):**
- plan-wide: restructured into 8 items (old 2, 4, 5, 6, 9, 10, 11, 12, 13, 14 folded into a bulk port, item 2; old 7, 8, 10, 13 recast as adaptations 4–7; old 15 is 8), since the source's `cmd/release` files form one compile cycle and `unused` lint flags early symbols (writer decision); compile-order guards of old 2, 5, 6, 9, 10, 11, 12, 13 dropped as moot.
- 2: recast (bulk port); guards folded from old 4 (workflow greps), old 7 (`/dist/` ignored), old 12 (no test rolls up the live CHANGELOG).
- 3: guard folded (per-file acceptance, sentence-based missed-site rule, test imports).
- 4: recast (was 10); guards folded from old 8 (embed only `templates/*.tmpl`, winget render symbols), old 9 (winget-only seam methods), old 11 (five status steps).
- 5: recast (was 13); old 13's Goal guards moot under the narrowed Goal; its dist+smoke acceptance kept by writer decision.
- 6: recast (was 7); guards folded from old 7 (fixture writes the OFL texts, flat entry names); old 7's mtime test dropped with the mtime claim.
- 7: recast (was 8); Depends on 4 for the shared `render_test.go`.
- 8: guard folded (was 15); README install lines dated to the first published release (writer decision).
- 2 (re-check): guard folded (the source's product blurb becomes photon's description; `issue tracker` grep).
- 4 (re-check): guard folded (scope is every file the `winget|komac` grep prints: adds `channels_test.go`, `main.go`, `status.go`, `runner.go`, the tag workflow; `smoke.go`/`smoke_test.go` hold no winget code).
- 5 (re-check): guard folded (the fake keeps the zip test's platform bite; "photon X.Y.Z" comments fixed, formula template and golden in Files); dist+smoke acceptance removed and Depends on items 2 and 4 (writer decision, supersedes the line above).
- 6 (re-check): guard folded (`writeReleaseTree` writes the OFL texts, `smoke_test.go` in Files, whole-package test; dist help names the font licences).

**Sources:**
- `.capcom/issues/release-setup.md` (the issue; it stays open until the first release is cut)
- `AGENTS.local.md` — "Release-tool port source" (the port source and its pinned commit) and the `forbidden:` line
- `docs/handoffs/2026-10-17 - 00 - version-file-and-release-tooling.md`
- Notes `package-layout`, `coding-conventions`, `extension-model`

**Ratified design calls:**
- **Scope:** full port — CHANGELOG, tag workflow, `cmd/release` with `dist`, `status`, `prepare`, `publish`, `smoke`, release docs (user, 2026-10-17).
- **Channels:** GitHub release, Homebrew tap, Scoop bucket; no winget (user, 2026-10-17).
- **Smoke version check:** `photon version` keeps printing the bare version; smoke compares its trimmed output to it (user, 2026-10-17).
- **Layout:** self-contained `package main` in `cmd/release`, a dev tool never shipped; layout docs amended (user, 2026-10-17).
- **ffmpeg:** the formula `depends_on "ffmpeg"`, the Scoop manifest `"depends": "ffmpeg"`; gifsicle stays optional (user, 2026-10-17).
- **CHANGELOG seed:** `## [Unreleased]` carries plain-language bullets for what photon does today (user, 2026-10-17).
- **Make:** no release targets; the tool runs as `go run ./cmd/release <verb>` (user, 2026-10-17).
- **Archives:** carry `LICENSE`, `README.md` and the three OFL texts from `internal/fonts/` (writer, 2026-10-17).

**Standing requirements:**
- skills: coding-standards
- Port behaviour from the release-tool port source; `photon` replaces the source tool's name everywhere. No file this plan adds or edits under `cmd/release`, `.github`, `docs/releasing.md`, `CHANGELOG.md` or `README.md` matches the `forbidden:` pattern (AGENTS.md and `.capcom/` name the issue register by design): `! grep -rniE "$(sed -n 's/^forbidden: //p' AGENTS.local.md)" <paths>`.
- Repo slug `airiclenz/photon`, binary `photon`, main package `./cmd/photon`, tap `airiclenz/homebrew-tap` (`Formula/photon.rb`), bucket `airiclenz/scoop-bucket` (`bucket/photon.json`).
- Tests hermetic (no network, `t.TempDir()` only); Pi host: `GOMEMLIMIT=2GiB`, one test command at a time.
- Every new `cmd/release` file is named in `cmd/release/doc.go`'s file map in the item that adds it.

**Out of scope:**
- Cutting, tagging or publishing any release; changing `VERSION`.
- winget, npm or any other channel; Makefile release targets.
- Repo-local release skills (the global `cut-release` / `release-notes` skills drive the tool).

## 1. Seed CHANGELOG.md — ✅ DONE (2026-10-17)

NOTES (2026-10-17): the only version numbers in the file are the Keep a Changelog 1.1.0 and SemVer 2.0.0 links in the verbatim preamble; the font licence is named without its version so no other number appears.

**What:**
**Goal:** `CHANGELOG.md` at the repo root follows Keep a Changelog 1.1.0 + SemVer: the `# Changelog` title, the format paragraph, and `## [Unreleased]` holding flat plain-language bullets for what photon does today; no release heading.
**Approach (assumed at the header base):** copy the port source's `CHANGELOG.md` preamble (title, intro line, format paragraph) verbatim. Write 6–12 bullets from `README.md` (record a take from a storyboard, `check`, `render` to GIF, services and profiles, targets/areas, zoom and drawn cursor, drag, embedded fonts, `lint`, `setup`, `version`), each one sentence a user would understand. No version number anywhere in the file.
**Files:** the root changelog only (created here; no other item touches it)
**Read first:** README.md — ## The commands, ## Services and profiles, ## Sections, zoom and cursor, ## Fonts and licences; cmd/photon/root.go — newRootCommand (the six commands the bullets name); port source CHANGELOG.md — preamble (title, intro, format paragraph); port source cmd/release/prepare.go — rollUpChangelog, checkPrepare (needs `## [Unreleased]` with at least one `- ` bullet and no version heading)
**Tests:** none (docs).
**Acceptance:** `head -1 CHANGELOG.md | grep -qx '# Changelog'`; `grep -qx '## \[Unreleased\]' CHANGELOG.md`; `! grep -qE '^## \[[0-9]' CHANGELOG.md`; `test "$(sed -n '/^## \[Unreleased\]/,$p' CHANGELOG.md | grep -c '^- ')" -ge 6`; forbidden grep on `CHANGELOG.md`.
**Commit:** `docs: add CHANGELOG with the unreleased v1 features`

## 2. Bulk port of the release tool — ✅ DONE (2026-10-17)

NOTES (2026-10-17): source doc_test.go dropped and replaced by docmap_test.go (testkit.CheckDocMap); doc.go's file-map lead-in now names TestDocMap, its imports sentence admits docmap_test.go's internal/testkit import, and the ADR references in doc.go and versionbump_test.go were removed (photon has no ADR).
NOTES (2026-10-17): the winget code, templates and goldens are ported with the id renamed to AiricLenz.Photon (manifests/a/AiricLenz/Photon) so the package compiles; item 4 deletes them.

**What:** Recast at the regression check (2026-10-17).
**Goal:** `go run ./cmd/release --help` lists `dist`, `status`, `prepare`, `publish`, `smoke`; `go vet`, `go test` and the targeted lint pass on `./cmd/release/`; the version-bump tests pass; the forbidden grep is clean on `cmd/release` and `.github`.
**Regression guard.** The workflow keeps the tag step: Acceptance greps `getRef`, `createTag`, `fetch-depth: 0` and `contents: write` in it. `/dist/` goes into `.gitignore` (the port source ignores it), so a plain `dist` run leaves the checkout clean for the gate. No test rolls up the live `CHANGELOG.md`: after the first cut its Unreleased holds no bullet and `rollUpChangelog` refuses. The source's product blurb (`desc` / `"description"`) becomes photon's root `Short` (`cmd/photon/root.go`) in every file `grep -rni 'issue tracker' cmd/release` prints: both templates, the formula golden, `render_test.go`'s expectation, `probes_test.go`, and the winget locale template and golden until item 4 deletes them.
**Approach (assumed at the header base):** one mechanical item porting the whole compile unit at once: every file of the port source's `cmd/release` (production, tests, `templates/`, `testdata/`) plus `.github/scripts/version-bump.sh` and `.github/workflows/tag-on-version-bump.yml`, winget included for now so it compiles. The source `doc_test.go` is replaced by photon's `cmd/release/docmap_test.go` calling `testkit.CheckDocMap`, and `doc.go`'s map is rewritten for photon. Extract with `git -C <src> archive <commit> <paths> | tar -x -C .`, then rename mechanically: the tool name in all cases → `photon`; repo slug → `airiclenz/photon`; `binaryName` `photon`, `mainPackage` `./cmd/photon`; tap formula `Formula/photon.rb`; bucket `bucket/photon.json`; template and golden file names; temp-dir prefixes `photon-`; the winget id and manifest dir renamed to a photon form. Comments that mention the source's skills are rewritten generically. The script stays executable (`git update-index --chmod=+x`). The behaviour adaptations of items 4–7 are NOT in this item.
**Files:** cmd/release/ (whole directory), .github/scripts/version-bump.sh, .github/workflows/tag-on-version-bump.yml, .gitignore
**Read first:** port source cmd/release/main.go — newRootCommand; port source cmd/release/repo.go — the repo-slug const; port source cmd/release/dist.go — binaryName, mainPackage, buildArchive;
port source cmd/release/templates — the formula template, scoop.json.tmpl; port source cmd/release/render_test.go — TestRenderScoopManifest, goldenSums; port source cmd/release/versionbump_test.go — newBumpRepo;
port source .github/workflows/tag-on-version-bump.yml — github-script getRef/createTag/createRef; internal/testkit/docmap.go — CheckDocMap
**Tests:** the ported tests (including the seven `TestVersionBump*` cases) and `docmap_test.go`.
**Acceptance:** `go run ./cmd/release --help` lists `dist`, `status`, `prepare`, `publish`, `smoke`; `go vet ./cmd/release/`; `go test -c -o /dev/null ./cmd/release/`; `GOMEMLIMIT=2GiB go test ./cmd/release/`; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run ./cmd/release/`; `bash -n .github/scripts/version-bump.sh`; `test -x .github/scripts/version-bump.sh`; `W=.github/workflows/tag-on-version-bump.yml; grep -q getRef $W && grep -q createTag $W && grep -q 'fetch-depth: 0' $W && grep -q 'contents: write' $W`; `grep -qx '/dist/' .gitignore`; `! grep -rni 'issue tracker' cmd/release`; forbidden grep on `cmd/release .github`.
**Commit:** `feat(release): port the release tool and the tag-on-version-bump workflow`

## 3. Layout docs admit cmd/release — ✅ DONE (2026-10-17)

NOTES (2026-10-17): AGENTS.md's opening paragraph now also names the module root package (it named only `cmd/photon` and `internal/`), so it states the full package set the Goal lists; the last two lines of that paragraph were rewrapped.
NOTES (2026-10-17): cmd/release tests also import gopkg.in/yaml.v3 besides internal/testkit; the prose keeps the plan's mandated wording "its tests use `internal/testkit`", which does not claim exclusivity.

Depends on item 2.

**What:**
**Goal:** every prose statement of photon's package layout names `cmd/release` as the release tool — a self-contained `package main`, stdlib + cobra only, never shipped in an archive — beside `cmd/photon`, `internal/` and the root package; Note `package-layout` has a `cmd/release` row.
**Regression guard.** Word the imports "non-test code imports only the standard library and cobra; its tests use `internal/testkit`", in the prose and in the `package-layout` row's imports column. The missed-site rule is sentence-based: read the sentence around each hit of `grep -rnE "cmd/photon|internal/|module root" AGENTS.md version.go .capcom/knowledge README.md`. Acceptance checks every file, not any one.
**Approach (assumed at the header base):** amend `AGENTS.md` (opening paragraph: "binary is built from `cmd/photon` and every other package lives under `internal/`"), the package comment in `version.go` ("the only package outside internal/ and cmd/photon"), Note `package-layout` (table row + Rules), Note `coding-conventions` (Layout bullet), Note `extension-model` (the line on where packages live). Rule for missed sites: every sentence naming `cmd/photon`, `internal/` or the root package as the package set (prose wraps across lines, so read each hit's sentence). Notes are hand-edited, then `capcom check`.
**Files:** AGENTS.md, version.go, .capcom/knowledge/package-layout.md, .capcom/knowledge/coding-conventions.md, .capcom/knowledge/extension-model.md
**Read first:** .capcom/knowledge/package-layout.md — package table, Rules, root-package paragraph; version.go — package comment; AGENTS.md — opening paragraph; .capcom/knowledge/coding-conventions.md — Layout bullet; .capcom/knowledge/extension-model.md — Decision paragraph; version_test.go — TestVersionIsSemantic
**Tests:** none (docs); `go vet .` for the comment edit.
**Acceptance:** `test -z "$(grep -L 'cmd/release' AGENTS.md version.go .capcom/knowledge/{package-layout,coding-conventions,extension-model}.md)"`; ``grep -q '^| `cmd/release`' .capcom/knowledge/package-layout.md``; `capcom check`; `GOMEMLIMIT=2GiB go test .`
**Commit:** `docs: name cmd/release in the package layout`

## 4. Drop the winget channel — ✅ DONE (2026-10-17)

NOTES (2026-10-17): TestPublishWingetConvergesFromRemoteStateOnceTheTagExists was recast as TestPublishChannelsConvergeFromRemoteStateOnceTheTagExists (pending tap, dirty tree, VERSION moved on: publish pushes only the formula and never main) instead of dropped, so the channel steps' resume-without-the-gate coverage survives.
NOTES (2026-10-17): runChannelPublish was removed rather than stripped of its getenv: without the token it duplicated runTestPublish, which its remaining callers now use; publishFake.respond now refuses unexpected commands itself (respondTools, fakeChannels and publishFake.mu removed with komac).
NOTES (2026-10-17): winget-only test scaffolding also removed: memChannelRepo's dirs/pulls fields, methodListDirs/methodOpenPulls, probes_test's unparsable var, the OpenPulls-only `commands` field of TestExecChannelReposTellsANoFromABrokenCommand, and render_test's gopkg.in/yaml.v3 import (cmd/release tests now import only internal/testkit beyond the stdlib).
NOTES (2026-10-17): comments that named komac or `gh auth token` (runner.go seam comment, doc.go seam paragraph) and the seam's "file or directory" wording in channelrepos.go now describe the tap and bucket only.
NOTES (2026-10-17): setupPendingError (channels.go) and publish.go's errors.As branch for it are kept although no converge returns one now that winget is gone; a generic mechanism outside this item's file list, left as a candidate cleanup.

Depends on item 2.

**What:** Recast at the regression check (2026-10-17).
**Goal:** no winget code, step, status row, smoke probe, template, golden, channel-repo path or test remains: `! grep -rni winget cmd/release .github`; the channels left are GitHub release, Homebrew and Scoop.
**Regression guard.** `render.go` embeds and parses only `templates/*.tmpl`; drop `releasePageBase`, `ReleaseURL`, `WingetPackageID`, `UpperSHA256`, `renderedFile` and their tests with the winget templates. Drop `ListDirs`, `OpenPulls`, `pullRequest` and `viewerOpenPullsQuery` from `channelRepos`, `execChannelRepos` and `memChannelRepos`. Status tests: wantStates is tag, release, homebrew, scoop, smoke; step counts read `len(publishSteps)`. Scope is the rule, not a list: every file `grep -rli 'winget\|komac' cmd/release .github` prints — `channels_test.go` (the `TestPublishWinget*` tests, `publishFake`'s komac methods, `runChannelPublish`'s getenv), and the help and comments in `main.go`, `status.go`, `runner.go` and the tag workflow, which then name the three channels left. `smoke.go`/`smoke_test.go` hold no winget code: there is no smoke probe to drop.
**Approach (assumed at the header base):** delete `winget.go`, `templates/winget/`, `testdata/winget/`, the winget step in `steps.go`, the winget package resolution and its env lookup in `publish.go`, and every winget test case and fake method; remove `winget.go` from `doc.go`'s map.
**Files:** cmd/release/ (winget.go, templates/winget/, testdata/winget/ removed; render.go, render_test.go, channelrepos.go, channelrepos_test.go, memchannels_test.go, channels_test.go, steps.go, publish.go, publish_test.go, status.go, status_test.go, probes_test.go, main.go, runner.go, doc.go edited), .github/workflows/tag-on-version-bump.yml
**Read first:** cmd/release/channels_test.go — publishFake, runChannelPublish, releasedFake, TestPublishWinget*; cmd/release/steps.go — publishSteps, stepWinget; cmd/release/render.go — templateFiles, packageTemplates, packageData;
cmd/release/channelrepos.go — channelRepos, execChannelRepos.OpenPulls, ListDirs; cmd/release/probes_test.go — wingetHolding, TestProbeStepsRunsEveryStepInPublishOrder; cmd/release/publish.go — publishOptions.getenv, runPublish;
cmd/release/status_test.go — TestStatusJSONKeysArePinned; cmd/release/main.go — newRootCommand
**Tests:** the remaining ported tests, with winget cases (`channels_test.go`'s included) removed; status wantStates lists the five steps.
**Acceptance:** `! grep -rni 'winget\|komac' cmd/release .github`; `go vet ./cmd/release/`; `go test -c -o /dev/null ./cmd/release/`; `GOMEMLIMIT=2GiB go test ./cmd/release/`; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run ./cmd/release/`; forbidden grep on `cmd/release .github`.
**Commit:** `feat(release): drop the winget channel`

## 5. Smoke accepts the bare version — ✅ DONE (2026-10-17)

NOTES (2026-10-17): `smokeFake.respond` now takes the host (`respond(goos, goarch)` returns the responder; the switch moved to `respondOn`), so the fake answers the bare version only for the host's own stub build and `0.0.0` for any other; `runTestSmoke` passes its options' host, the direct `smokeRun` test linux/amd64, and the two command-tree tests `runtime.GOOS/GOARCH` as the tree itself uses.
NOTES (2026-10-17): the "photon 1.2.3 fails" proof is a new TestSmokeChecks case ("a version behind photon's name is not the bare version", brew reporting "photon 0.24.11") on the suite's 0.24.11 fixture rather than a literal 1.2.3.

Depends on items 2 and 4.

**What:** Recast at the regression check (2026-10-17).
**Goal:** smoke's version check (`checkReportsVersion`) compares `strings.TrimSpace` of `photon version` output to the bare version, the ratified bare-version call.
**Regression guard.** remove the `go run ./cmd/release dist --out "$T" && go run ./cmd/release smoke --dist-dir "$T"` acceptance command (it needs a published release and exits 1 on a correct port); the targeted smoke unit test (bare "1.2.3\n" passes, "photon 1.2.3" fails) is the item's proof. Item 5 also declares "Depends on items 2 and 4" since both edit smoke.go/smoke_test.go (writer, 2026-10-17). `smokeFake` prints the bare version only when the extracted stub is the expected host's build (else a wrong version), so `TestSmokeRunsTheHostsZipArchive` keeps its platform bite; `brewReports` and the `reports "..."` wantResults become bare. Every comment naming the "photon X.Y.Z" output (`smoke.go`'s `versionArgument`, the formula template's and golden's `test do`) states the bare version.
**Approach (assumed at the header base):** change `checkReportsVersion` in `smoke.go` from a name-prefixed line to the trimmed bare version; its callers (the host-binary and Homebrew checks) need no change.
**Files:** cmd/release/smoke.go, cmd/release/smoke_test.go, cmd/release/templates/photon.rb.tmpl, cmd/release/testdata/photon-0.0.0.rb
**Read first:** cmd/release/smoke.go — checkReportsVersion, checkHostBinary, versionArgument; cmd/release/homebrew.go — checkHomebrew; cmd/release/smoke_test.go — smokeFake.respond, newSmokeFake, TestSmokeChecks, TestSmokeRunsTheHostsZipArchive;
cmd/release/templates/photon.rb.tmpl — test do; cmd/photon/version.go — newVersionCommand
**Tests:** output `"1.2.3\n"` passes and `"photon 1.2.3"` fails for version `1.2.3`; the smoke fake answers bare (wrong version for a non-host stub), and the zip-archive test still proves the host's build ran.
**Acceptance:** `GOMEMLIMIT=2GiB go test -run Smoke ./cmd/release/`; `GOMEMLIMIT=2GiB go test -run Render ./cmd/release/`; `! grep -rn '"photon X.Y.Z"' cmd/release`; `go vet ./cmd/release/`; `go test -c -o /dev/null ./cmd/release/`; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run ./cmd/release/`; forbidden grep on `cmd/release`.
**Commit:** `feat(release): smoke accepts the bare version`

## 6. Archives carry the font licences — ✅ DONE (2026-10-17)

NOTES (2026-10-17): `testSourceTreeAt` and `writeReleaseTree` share one new fixture writer, `writeTestDocuments` in `dist_test.go`, over a `testDocuments` map (LICENSE, README.md and the three OFL texts under `internal/fonts/`); document sources are joined with `filepath.FromSlash` so the slash-separated `documentFiles` paths resolve on any host.

Depends on item 2.

**What:** Recast at the regression check (2026-10-17).
**Goal:** every dist archive holds `LICENSE`, `README.md` and the three `internal/fonts/*-OFL.txt` texts (`documentFiles`); a test asserts it.
**Regression guard.** Extend `testSourceTreeAt` to write the three `internal/fonts/*-OFL.txt` files, or every dist test fails opening them, and add them to the archive-entries test's want list. Name each entry `path.Join(stem, path.Base(document))` so the texts sit flat beside `LICENSE`; the entries test pins the flat names. `writeReleaseTree` (`smoke_test.go`, behind `publishedRelease`, `rebuiltSums`, `rebuiltAssetContent` and `memRepo.CloneAt`) writes the three OFL texts too, or shares one writer with `testSourceTreeAt`; the whole package's tests run. dist's Long help names the font licences beside `LICENSE` and `README.md`.
**Approach (assumed at the header base):** add the three OFL paths to `documentFiles` in `dist.go`; flatten the entry names; update `dist_test.go`.
**Files:** cmd/release/dist.go, cmd/release/dist_test.go, cmd/release/smoke_test.go
**Read first:** cmd/release/dist.go — documentFiles, buildArchive, newDistCommand; cmd/release/dist_test.go — testSourceTreeAt, TestDistArchivesHoldTheBinaryAndDocsUnderTheStem, readArchive; cmd/release/smoke_test.go — writeReleaseTree, publishedRelease;
cmd/release/publish_test.go — rebuiltSums, rebuiltAssetContent; cmd/release/memrepo_test.go — memRepo.CloneAt; internal/fonts — SourceCodePro-OFL.txt, NotoSansMath-OFL.txt, NotoSansSymbols-OFL.txt
**Tests:** the archive-entries test lists the binary, `LICENSE`, `README.md` and the three OFL texts flat under the stem, for every target; the smoke and publish tests pass over `writeReleaseTree`'s tree.
**Acceptance:** `GOMEMLIMIT=2GiB go test -run Dist ./cmd/release/`; `GOMEMLIMIT=2GiB go test ./cmd/release/`; `T=$(mktemp -d); go run ./cmd/release dist --out "$T" && test -s "$T/SHA256SUMS" && tar -tzf "$T"/photon_*_linux_amd64.tar.gz | grep -q SourceCodePro-OFL.txt`; `go vet ./cmd/release/`; `go test -c -o /dev/null ./cmd/release/`; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run ./cmd/release/`; forbidden grep on `cmd/release`.
**Commit:** `feat(release): archives carry the font licences`

## 7. Formula and manifest pull in ffmpeg — ✅ DONE (2026-10-17)

Depends on items 2 and 4 (both edit `cmd/release/render_test.go`).

**What:** Recast at the regression check (2026-10-17).
**Goal:** the Homebrew formula template has `depends_on "ffmpeg"`, the Scoop manifest template `"depends": "ffmpeg"`; the goldens are updated and the render tests check it.
**Approach (assumed at the header base):** add the dependency to both templates; regenerate the formula golden under `testdata/` and the Scoop expectation in `render_test.go`; add a check for the ffmpeg dependency in both rendered outputs.
**Files:** cmd/release/templates/photon.rb.tmpl, cmd/release/templates/scoop.json.tmpl, cmd/release/testdata/photon-0.0.0.rb, cmd/release/render_test.go
**Read first:** cmd/release/render_test.go — renderedScoopManifest, TestRenderScoopManifest, TestRenderFormulaMatchesTheGoldenFormula, goldenSums; cmd/release/templates — photon.rb.tmpl, scoop.json.tmpl; cmd/release/testdata/photon-0.0.0.rb;
cmd/release/scoop.go — renderScoopManifest (json.Valid); cmd/release/homebrew.go — renderFormula
**Tests:** the render tests with the updated goldens; both outputs carry the ffmpeg dependency.
**Acceptance:** `GOMEMLIMIT=2GiB go test -run Render ./cmd/release/`; `grep -q 'depends_on "ffmpeg"' cmd/release/templates/photon.rb.tmpl`; `grep -q '"depends": "ffmpeg"' cmd/release/templates/scoop.json.tmpl`; `go vet ./cmd/release/`; `go test -c -o /dev/null ./cmd/release/`; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' Makefile) run ./cmd/release/`; forbidden grep on `cmd/release`.
**Commit:** `feat(release): the formula and manifest pull in ffmpeg`

## 8. Release docs and install instructions

Depends on items 1 and 4.

**What:**
**Goal:** `docs/releasing.md` documents prerequisites (`gh`, Go; `brew`/`scoop` for their smokes), one-time setup (tap and bucket repos, `gh` auth), the verbs as `go run ./cmd/release <verb>`, the gate, the tag workflow and resuming a failed publish; README's Install section adds Homebrew, Scoop and release-archive installs beside `go install`, with ffmpeg noted as pulled in by brew/scoop.
**Regression guard.** README's Homebrew, Scoop and release-archive install lines say they apply from the first published release; `go install` stays the primary path until then. Acceptance checks each section: `## Prerequisites`, `## One-time setup`, resuming, the tag workflow, and release archives and ffmpeg inside README's Install section.
**Approach (assumed at the header base):** port the port source's `docs/releasing.md` structure without winget and rewrite for photon; edit README `## Install` only. Everything a user adds goes under `## [Unreleased]`; nobody hand-tags or bumps — say so in `docs/releasing.md`.
**Files:** docs/releasing.md, README.md
**Read first:** README.md — ## Install (go install line, ffmpeg and gifsicle bullets, Windows note); cmd/photon/readme_test.go — TestREADMEDocumentsEveryCommand; port source docs/releasing.md — Prerequisites, One-time setup, Cutting a release, The gate, Resuming a failed publish; CHANGELOG.md — ## [Unreleased]
**Tests:** `GOMEMLIMIT=2GiB go test ./cmd/photon/` (README doc tests still pass).
**Acceptance:** `grep -q 'go run ./cmd/release' docs/releasing.md`; `grep -q '^## Prerequisites' docs/releasing.md`; `grep -q '^## One-time setup' docs/releasing.md`; `grep -qi 'resum' docs/releasing.md`; `grep -q 'tag-on-version-bump' docs/releasing.md`; `grep -q 'brew install airiclenz/tap/photon' README.md`; `grep -qi scoop README.md`; `I=$(sed -n '/^## Install/,/^## [^I]/p' README.md); echo "$I" | grep -q releases && echo "$I" | grep -qi ffmpeg && echo "$I" | grep -qi 'first.*release'`; `! grep -qi winget docs/releasing.md`; `GOMEMLIMIT=2GiB go test ./cmd/photon/`; forbidden grep on `docs/releasing.md README.md`.
**Commit:** `docs: describe releasing photon and installing it from Homebrew and Scoop`
