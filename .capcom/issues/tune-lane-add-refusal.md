---
title: Show a refused lane add in photon tune
status: open
type: bug
priority: 3
labels: [tune]
created: 2026-10-18T12:32:57Z
---

A refused lane add (for example an overlap) is shown only as `app.js`'s refusal on the entry
index it named (`internal/tune/page/app.js:137`, `edit` sets `refusal` to the change's owner),
and the lane's add button swallows the rejection (`internal/tune/page/timeline.js:220-229`,
`addButton`). The refusal is therefore invisible unless that entry is in the inspector. Found
closing out plan `2026-10-18 - 01 - open-issues-plan` (item 9).

## Acceptance

- A refused lane add shows its refusal message on the page whatever the inspector holds.
- A page test drives a refused add from a lane's add button and asserts the message is shown.
