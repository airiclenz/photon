---
title: launch.strip does not reach a url launch's browser
status: open
type: bug
priority: 3
created: 2026-11-06T20:29:21Z
---

A url launch passes the env built by `launchEnv` (photon's env minus `launch.strip`, plus `launch.env` and photon's vars) to Chromium through `chromedp.Env` (`internal/browser/browser.go`, `allocatorOptions`). chromedp only *adds* those vars on top of the Go process env (`ExecAllocator.setCmdEnv`: `cmd.Env = append(os.Environ(), …)`) unless `chromedp.NoInheritEnv` is set. So every variable `launch.strip` names still reaches the browser process, and its children.

## Why

Projects use `launch.strip` to keep secrets and host-specific settings out of a take. For a url launch it silently does nothing, unlike the terminal path, where the pty child gets exactly the built env.

## Acceptance

- The browser process env is exactly `browser.Options.Env` (e.g. add `chromedp.NoInheritEnv`), with nothing inherited beyond it
- A test pins it: a variable set in photon's env and named in `launch.strip` is absent from the page's browser process (a Chromium `//go:build browser` test, or a unit test on the allocator options)
- Terminal launches unchanged
