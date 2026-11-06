// Command fixtureweb is the small web app photon's browser-capture tests film. It stands in for a
// real web app: it serves one page on 127.0.0.1:$PHOTON_PORT (photon hands a service its port in
// that variable) and prints "ready" on stdout once it listens, so a service's ready line can wait
// for it. Any path but / answers 404. It runs until it is stopped.
//
// The page holds a title, a nav of the items alpha/beta/gamma (a selector target), a text input,
// a Run button and a status line. Typing in the input shows "you typed: <text>" on the status
// line and a click on a nav item shows "selected: <item>". A click on Run shows "working" with a
// spinner beside the button for about a second, then removes the spinner and shows "clicked:
// Run"; the spinner is the page's only animation, so the page goes still once the work is done
// and a take over it settles.
//
// Tests get its path from testkit.BuildFixtureWeb.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - main.go: the fixture: the embedded page, the listener and the page handler.
package main
