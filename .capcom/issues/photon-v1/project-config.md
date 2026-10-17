---
title: "Per-project photon.yaml: rig, launch, env, end of take, theme"
status: open
type: feature
priority: 2
deps: [scaffold]
created: 2026-10-17T08:54:41Z
---

Everything about how a project's app is filmed — how its rig is built and reset, what is
launched, with what environment, how it is quit, how its screen is laid out — must come from
a per-project config, `photon.yaml`, shared by every storyboard of that project. photon itself
knows no app.

The config holds:
- `work`: the rig's work dir (default `~/.cache/<name>-photon`; `PHOTON_WORK` and `--work`
  override it).
- `setup`: a command that builds the rig, run by `photon setup`; `reset`: a command run before
  every take.
- `launch`: the command line to film, its working dir and env. An env strip list removes the
  operator's variables that would steer the app away from the rig (e.g. `MYAPP_*`).
- `services`: see issue `take-services`.
- `end`: how a take ends: the keys that quit the app, the idle wait first, the kill timeout.
- `areas`: see issue `declarative-areas`.
- `theme`: the 16 ANSI colours plus default fg/bg the raster draws with, with a default.
- `fonts`: an optional dir that overrides the embedded faces.

photon finds the config through `--config`, else the nearest `photon.yaml` at or above the
storyboard's directory. Decoding is strict, as for storyboards, and `photon lint` validates it.

## Acceptance

- `internal/config` loads and validates the schema; table tests cover every rule.
- The lookup order (`--config`, then the walk up) is tested.
- A minimal config (launch one command on a stage dir, no services) lints clean.
