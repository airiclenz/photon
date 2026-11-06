---
title: A Windows service's child can escape its Job object
status: active
type: bug
priority: 0
created: 2026-11-06T12:51:40Z
---

joinGroup assigns the service's shell to its Job object only after the shell has started, so a child the shell launches first is outside the job: TerminateJobObject does not end it and Stop returns while it still runs. CI run 42219702722 hit it: TestStart_TCPReadiness_ServesUntilStop on windows-latest found the service still accepting connections after Stop.

## Why

An escaped service outlives the take, keeps its port, and blocks the release gate when CI hits the race.

## Acceptance

- The shell starts suspended and resumes only once it is in the job, so every process it starts is in the job
- CI is green on windows-latest

## Log

- 2026-11-06 Airic Lenz: Start the shell with CREATE_SUSPENDED, assign it to the job, then resume its thread via a Toolhelp thread snapshot.
