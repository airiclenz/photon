---
title: "photon record and setup: one take end to end"
status: open
type: feature
priority: 2
deps: [beat-engine, take-services, check-command]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
---

`photon record <storyboard>` runs one take end to end, so a re-record is one command and a
retake loop is a shell one-liner.

The flow: load the storyboard and config; run `reset`; start the services (issue
`take-services`); launch the app in the pty with the config's env, minus the strip list; wait
for the first paint; perform every beat; end the take (wait for the app to go idle, send the
quit keys, kill it after the timeout; nothing painted on the way out is kept); write
`<work>/<clip>.take`; run `on_success`; check the take (issue `check-command`). The exit status
is the check's.

A take whose beats failed is still written, because it shows why, but it is not checked.
`photon setup` runs the config's `setup` command once.

## Acceptance

- An end-to-end test records the fixture TUI from a fixture `photon.yaml` and storyboard,
  writes the take and checks it; it runs on Linux and macOS CI.
- A failing beat writes the take, skips the check and exits 1.
- A retake loop (`until photon record …; do :; done`) works: every exit status is documented
  and tested.
