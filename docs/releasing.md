# Releasing photon

A release is cut locally, from a clone of this repository, on Linux, macOS or Windows. The
judgment work (the release notes and the version to cut) is the maintainer's, or the agent
skills' in `.agents/skills/`: `release-notes-photon` drafts the notes and `release-photon` cuts
and publishes; everything deterministic is done by `go run ./cmd/release`, which builds,
publishes and checks. The tool is a dev tool in `cmd/release`: it is never shipped in a release.

One release publishes to five channels:

| Channel | Where | How |
| --- | --- | --- |
| GitHub release | `airiclenz/photon` | six archives and `SHA256SUMS` |
| Homebrew | `airiclenz/homebrew-tap`, `Formula/photon.rb` | a direct `photon X.Y.Z` commit |
| Scoop | `airiclenz/scoop-bucket`, `bucket/photon.json` | a direct `photon X.Y.Z` commit |
| winget | `microsoft/winget-pkgs`, `manifests/a/AiricLenz/Photon/X.Y.Z` | a komac pull request |
| npm | `registry.npmjs.org`: `@airiclenz/photon`, six platform packages | `npm publish` each |

The Homebrew formula and the Scoop manifest both depend on ffmpeg, and the winget installer
manifest declares `Gyan.FFmpeg` as a package dependency, so installing photon through any of the
three pulls it in. The npm packages declare no ffmpeg: npm cannot install it, so an npm user
installs it themselves. gifsicle stays optional everywhere.

**winget: submitted, then installable.** The winget step does not publish: komac opens a pull
request on `microsoft/winget-pkgs` (`AiricLenz.Photon`), which merges later, after winget's
automated validation and a moderator's review. When `publish` returns, the release is
*submitted* to winget and the step reads done; `winget install AiricLenz.Photon` reaches the
version only once that pull request has merged.

**npm: a wrapper and six platform packages.** The wrapper `@airiclenz/photon` installs the
`photon` command, a small Node.js launcher, and lists the six platform packages
`@airiclenz/photon-<os>-<cpu>` (`linux`, `darwin`, `win32` on `x64` and `arm64`) as optional
dependencies at the same version. Each platform package carries one release binary and is limited
by its `os` and `cpu` fields to its platform, so npm installs only the one that matches the host,
and the launcher runs it. Each binary is taken from the release's archives after they are checked
against its `SHA256SUMS`.

## Prerequisites

- **Go**, at the version `go.mod` names, and **git**.
- **gh**, signed in with `gh auth login` and allowed to push to `airiclenz/photon`,
  `airiclenz/homebrew-tap` and `airiclenz/scoop-bucket`. The tap and the bucket are cloned with
  `gh repo clone` and pushed with `git push`, so git must be able to push over the protocol gh
  uses (`gh auth setup-git` sets that up for HTTPS).
- For the winget step: **komac** (`cargo install --locked komac`, `brew install komac` or
  `winget install komac`; https://github.com/russellbanks/Komac), and a GitHub token it opens the
  `microsoft/winget-pkgs` pull request with: `GITHUB_TOKEN` when it is set, else the token gh is
  signed in with (`gh auth token`). The token reaches komac through its environment only and is
  never printed.
- For the npm step: **npm** (it ships with Node.js, https://nodejs.org), and `NPM_TOKEN` set to an
  npm automation token of an account that may publish under the `@airiclenz` scope. An automation
  token publishes without the one-time password two-factor authentication otherwise asks for, and
  `publish` never prompts. The token reaches npm through its environment only, through a
  temporary userconfig that names `${NPM_TOKEN}`, never its value.
- Optional, for the Homebrew smoke check: **brew**, with photon installed through it, so `smoke`
  can check that `brew upgrade photon` reaches the new version. The Scoop check reads the
  bucket's manifest through gh and needs no **scoop** on the host. The npm check installs the
  wrapper into a temporary prefix with **npm** and runs its `photon`. winget has no smoke check: a
  submitted version is not installable yet. A check whose tool is missing prints `SKIP`, which
  never counts as a pass.

## One-time setup

- **gh auth.** Run `gh auth login` once, then `gh auth setup-git` so `git push` to the tap and the
  bucket uses the same credentials.
- **The tap and the bucket.** `publish` writes into `airiclenz/homebrew-tap` and
  `airiclenz/scoop-bucket` and never creates them. While one of them does not exist, `status`
  reports its step `setup-pending` and `publish` skips it and still succeeds. Create the missing
  one, then run `publish` again for the same version:

  ```sh
  gh repo create airiclenz/homebrew-tap --public
  gh repo create airiclenz/scoop-bucket --public
  ```

  An existing but empty repository is fine: the formula and the manifest are what `publish`
  writes.
- **komac and the GitHub token.** While komac is not installed, or there is neither a
  `GITHUB_TOKEN` nor a gh sign-in, `publish` reports winget `setup-pending` and skips it. Install
  komac (see *Prerequisites*), then run `publish` again for the same version.
- **The first winget submission.** Until `microsoft/winget-pkgs` has `AiricLenz.Photon`, the
  winget step renders the three manifests (version, installer and en-US locale) from the release's
  `SHA256SUMS` and submits them with `komac submit`, opening the new-package pull request; once the
  package is listed, every later release is a `komac update`. While that first pull request is
  open, a later version's winget step is `setup-pending` (a second `komac submit` would only open
  a rival new-package pull request): wait for it to merge, then run `publish` again for that
  version. A first `komac submit` that fails is `setup-pending` too: the manifests are kept in the
  temporary directory the step names, with the `komac submit` command to run on them by hand.
- **The npm token.** Create an automation token on npmjs.com for an account that may publish
  under the `@airiclenz` scope (the scope is the `airiclenz` user or organisation), and export it
  as `NPM_TOKEN` in the shell `publish` runs in. While it is not set, `publish` reports npm
  `setup-pending` and skips it before downloading anything; while npm is not installed, the step
  is `setup-pending` too. The first `npm publish` of each package creates it, public.

## Between releases

Every user-visible change adds a plain-language bullet under `## [Unreleased]` in `CHANGELOG.md`,
in the commit that makes the change. Nobody hand-edits a release heading, bumps `VERSION` or
creates a tag: `prepare` makes the release heading and the `VERSION` bump, and CI makes the tag.

## Cutting a release

```sh
go run ./cmd/release status                       # the gate, the baseline, each publish step
go run ./cmd/release prepare vX.Y.Z               # two local commits: CHANGELOG rollup, VERSION bump
go run ./cmd/release publish vX.Y.Z --notes-file /tmp/notes.md --dry-run
go run ./cmd/release publish vX.Y.Z --notes-file /tmp/notes.md
```

Write the release notes first, from the `## [Unreleased]` bullets and the commits since the last
release. Keep the notes file outside the repository, so it never dirties the tree the gate checks.

### The verbs

- `dist [--src DIR] [--out DIR]` cross-builds photon for linux, darwin and windows on amd64 and
  arm64 into `photon_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) plus `SHA256SUMS`, by
  default under `dist/` (ignored by git; `--out` is emptied first). Each archive holds the binary,
  `LICENSE`, `README.md` and the four font licences from `internal/fonts/`. The archives are
  byte-identical across runs over the same commit. `publish` runs the same build itself, in a
  fresh clone at the tag.
- `status [vX.Y.Z] [--json]` is read-only apart from fetching origin. It reports the gate, the
  baseline (the latest published release and the commits since) and each publish step (tag,
  release, homebrew, scoop, winget, npm, smoke) as done, pending, setup-pending or unknown. It
  exits 0 when the gate clears and 1 when it is blocked or a check could not run, printing its
  report either way.
- `prepare vX.Y.Z` commits `chore(release): cut vX.Y.Z`, which inserts `## [X.Y.Z] - <today>` in
  `CHANGELOG.md` directly under `## [Unreleased]`, so the unreleased bullets move under the release
  and an empty `## [Unreleased]` stays at the top, then `chore(version): bump version to vX.Y.Z`,
  which writes the bare `X.Y.Z` to `VERSION`. It fetches origin's `main` and its tags first, and
  refuses when that fails. It pushes nothing and refuses on a dirty tree, an empty
  `## [Unreleased]`, or a version not above the highest `vX.Y.Z` tag.
- `publish vX.Y.Z --notes-file F [--dry-run] [--tag-timeout D]` pushes the commit the gate
  cleared at `HEAD` to `main`, refusing and pushing nothing if `HEAD` has moved since, waits
  (10 minutes by default) for CI to tag the `VERSION` bump, creates the GitHub release, commits
  the rendered formula and Scoop manifest, submits the version to winget with komac, publishes
  the npm packages, and finally runs `smoke`. Once the tag is on origin, a
  rerun checks only the gate's CI half, over the tag, until the GitHub release is done (see
  *The gate*). `--dry-run` runs the gate and the probes and prints what each pending step would
  do, changing nothing. It never prompts and never reads stdin.
- `smoke [vX.Y.Z] [--dist-dir D]` checks a published release from the outside: the annotated tag,
  the downloaded archives against `SHA256SUMS`, each binary's build stamp against the tagged
  commit, the version in the host binary's `photon version` against the bare version (a commit it
  prints must match the binary's own build stamp; `, dirty` only warns), Homebrew, the Scoop
  manifest, and an npm install of the wrapper whose `photon` reports the version. With
  `--dist-dir`, that local build's `SHA256SUMS` must equal the published one.

Every verb takes a version as `vX.Y.Z` or bare `X.Y.Z`, and prints its full description with
`go run ./cmd/release <verb> --help`.

### The gate

`status` and `publish` clear a cut only when the working tree is clean, the gate commit (`HEAD`
with its trailing `CHANGELOG.md`/`VERSION`-only commits peeled off) is on `origin/main`,
`origin/main` is contained in `HEAD`, and `ci.yml` has a successful run for the gate commit or one
of the peeled commits. There is no bypass: red CI blocks the release until it is fixed.

Once the version's tag is on origin, `publish` builds from the tag, not the checkout, so while
the GitHub release is not done it checks only the gate's CI half, over the tag: `ci.yml` must
have a successful run for the tag's commit with its trailing `CHANGELOG.md`/`VERSION`-only
commits peeled off, or for a peeled commit `origin/main` contains. The working tree, `HEAD` and
`VERSION` at `HEAD` are not consulted, and the step table's gate row reads `CI only`. Once the
release is done too, the gate is not consulted at all.

### The tag

You never tag. `VERSION` holds a bare `X.Y.Z` and the tag is `v` + VERSION: when a push to `main`
changes `VERSION`, the `tag-on-version-bump` workflow (`.github/workflows/tag-on-version-bump.yml`)
creates the annotated tag on that commit. `publish` waits for it.

## Resuming a failed publish

`publish` converges: before each step it asks the remote whether that step is already done and
skips it if so. After a failure, fix the cause and run the **same** command again. There is no
local state to clean up, and a second run cannot duplicate a release or a channel commit; a draft
release an interrupted run left is completed, and only the assets it lacks are uploaded.
`go run ./cmd/release status vX.Y.Z` shows which steps are done.

- A step reported `setup-pending` did not fail the run; do its one-time setup (above) and run
  `publish` again.
- A tag that has not appeared within `--tag-timeout` fails the run; check the
  `tag-on-version-bump` workflow's run on GitHub, then run `publish` again, with a longer
  `--tag-timeout` if CI was only slow.
- winget reads done once its pull request is open, not once it merges. A pull request that was
  closed without merging leaves the version with neither a manifest nor an open pull request, so
  the step reads pending again and a `publish` re-run submits it anew. A failed `komac update`
  fails the run; fix the cause (often the token) and run `publish` again.
- npm publishes only the packages the registry lacks, the six platform packages first and the
  wrapper last, and reads done once the wrapper is there; a run interrupted part-way resumes with
  the missing ones. The registry never takes the same version of a package twice, so a broken npm
  release is fixed with a new version, like every other.
- A failed smoke check fails the run after everything else is published; fix the cause and run
  `go run ./cmd/release smoke vX.Y.Z`, or `publish` again.

To abandon a cut before `publish` has run, undo the two `prepare` commits with
`git reset --keep HEAD~2`. Once `publish` has pushed, nothing is undone; a broken release is fixed
with a new version.
