# photon

Record a terminal app's demo clip from a storyboard, check the take, and render it to a GIF.

photon runs the real program in a pty, performs the storyboard's beats — humanized typing, keys,
real mouse clicks, waits — records every screen change, and renders the shipped GIF with fixed
section lengths, zooms on on-screen targets and a drawn cursor. A re-record after the UI changes
is one command, not a hand performance.

Status: being set up. The work is tracked in `.capcom/issues/`.
