# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

- `photon tune`: a target picker on the preview. Press ⌖ Pick and drag across the cells of one row on the paused frame. The page proposes a target that the resolver has checked and draws its box. You can set it as the selected beat's zoom target, or copy it as click, seen, drag-from or drag-to YAML.
- `photon tune <storyboard> [<take>]` serves a timing editor on `127.0.0.1` (`--port`, default any free port) and prints its address. The page previews the take frame for frame as `photon render` composes it. You can edit each beat's `duration`, `hold`, `cut` and `zoom` and the clip's `frame.width` and `frame.fps`, with undo, redo and revert. Save writes only the edited values back into the storyboard, so comments and layout stay. Ctrl-C stops it.

## [0.1.0] - 2026-10-17

- `photon record <storyboard>` runs your terminal app in a pty, performs the storyboard's beats (humanized typing, named keys, real mouse clicks and drags, waits) and saves every screen change as a take, so a re-record after the UI changes is one command instead of a hand performance.
- `photon check <storyboard> [<take>]` judges a take against the storyboard's expects and prints a check table; `record` checks every take it writes and exits 0 only when every beat and expect passed, so `until photon record ...; do :; done` is the whole retake loop.
- `photon render <storyboard> [<take>]` turns a take into the shipped GIF: each beat becomes a section of exactly its `duration`, played at 1×, held or sped up to fit, and a `cut` beat is dropped from the clip. `--dry-run` prints the ffmpeg command lines instead, and memory stays flat however long the clip.
- Services and profiles: `photon.yaml` can start helper processes, such as a fake backend, before the launch, wait until each is ready, hand out their ports as `PHOTON_PORT_<NAME>` and stop them after the take; `photon record --profile <name>` swaps their commands or env and runs an `on_success` command after a good take.
- Targets and areas: clicks, drags and zooms aim at on-screen text found by a regex, optionally within a named band of rows that is found afresh on every screen, so a storyboard follows the app's layout as it moves.
- A zoom pushes into its target and eases back out within its section, and a drawn cursor glides to each click, rings on the press and follows the button through a drag.
- Frames are drawn with fonts embedded in the binary (Source Code Pro with Noto Sans Symbols 2, Math and Symbols as fallbacks, under the SIL Open Font License), so a clip renders the same on every machine; a project can supply its own faces through the config's `fonts` directory.
- `photon lint <storyboard>` prints every problem in a storyboard and its config without recording anything, and `photon setup <storyboard>` runs the config's `setup` once to build the rig.
- `photon version` and `photon --version` print photon's version.
- Maintainers can cut a release with `go run ./cmd/release` (`dist`, `status`, `prepare`, `publish`, `smoke`), and bumping `VERSION` on `main` now tags the release automatically.
- Every release archive carries the SIL Open Font License texts of the embedded fonts (`SourceCodePro-OFL.txt`, `NotoSansMath-OFL.txt`, `NotoSansSymbols-OFL.txt`) beside `LICENSE` and `README.md`.
- Installing photon with Homebrew or Scoop now installs ffmpeg with it, so `photon render` works out of the box; gifsicle stays optional.
- photon installs with Homebrew (`brew install airiclenz/tap/photon`), with Scoop from the `airiclenz/scoop-bucket` bucket, or from the release archives on GitHub, beside `go install`; `docs/releasing.md` describes how a release is cut.
