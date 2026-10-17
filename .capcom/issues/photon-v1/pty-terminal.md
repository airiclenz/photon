---
title: Run the app in a pty and record its screens
status: open
type: feature
priority: 2
deps: [take-format]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
---

photon films the real program, so it runs it in a pty and emulates the terminal itself
(`charmbracelet/x/vt`), rather than screen-scraping a real terminal window.

Wanted, in `internal/terminal`:
- start a command in a pty sized cols × rows, read it through the emulator, and answer the
  terminal's queries;
- sample the screen no faster than fps, collapsing identical grids, with the clock starting
  at the first paint;
- settle (no paint for a given quiet time), quit by sending keys with a kill timeout, and close
  returning the Take;
- clamp the emulator's scroll margins to the screen, and keep the key and SGR mouse sequences
  in photon's own code;
- when a launch fails with EPERM, a probe naming which launch step the kernel refused and which
  processes hold the pty as their controlling terminal (from /proc on Linux, "unknown" elsewhere).

Windows has no pty: recording there refuses with a clear error, and everything else (lint,
check, render) works.

## Acceptance

- `internal/terminal` records a fixture TUI built from `testdata`: a test types into it,
  waits for its repaint, quits it, and finds the expected snapshots in the take.
- It builds and passes vet on Windows, where recording returns the "needs a unix pty" error.
