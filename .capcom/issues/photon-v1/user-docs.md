---
title: "README: install, quick start and full reference"
status: done
type: task
priority: 2
deps: [record-command, render-command]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
closed: 2026-10-17T12:25:04Z
reason: Resolved by plan "2026-10-17 - 00 - photon-v1-plan" (archived in bae9f7a)
---

A project adopting photon must be able to do it from the README alone.

Wanted in `README.md`: install (`go install github.com/airiclenz/photon/cmd/photon@latest`) and
the external tools (ffmpeg, optional gifsicle); a quick start for a new project; the
`photon.yaml` reference; the storyboard reference with a worked beat; targets and areas;
expects; services and profiles; section durations, zoom and cursor; the take and the retake
loop; fonts and licences. Each command's `--help` matches the README.

## Acceptance

- Following the quick start on a clean machine with the fixture TUI produces a GIF.
- Every config and storyboard field appears in the reference.
