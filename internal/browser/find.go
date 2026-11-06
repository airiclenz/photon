package browser

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNoBrowser is the error [Start] wraps when it finds no browser to run: the path
// [Options.ExecPath] names is not an executable, or, with none named, no name on the discovery
// list is.
var ErrNoBrowser = errors.New("no Chromium found")

// discoveryNames are the names and paths a browser is looked up under when the launch names
// none, in the order chromedp's own allocator tries them on a unix-like host, then the macOS
// application bundles it tries on darwin.
var discoveryNames = []string{
	"headless_shell",
	"headless-shell",
	"chromium",
	"chromium-browser",
	"google-chrome",
	"google-chrome-stable",
	"google-chrome-beta",
	"google-chrome-unstable",
	"/usr/bin/google-chrome",
	"/usr/local/bin/chrome",
	"/snap/bin/chromium",
	"chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
}

// findBrowser returns the path of the browser to run: execPath resolved when it is set, and
// otherwise the first of names found, each looked up as exec.LookPath does (a bare name on PATH,
// a path as it stands). A set execPath is the only one tried, so a launch that names a browser
// never runs another. It fails with an error wrapping [ErrNoBrowser] that says what it tried.
func findBrowser(execPath string, names []string) (string, error) {
	if execPath != "" {
		found, err := exec.LookPath(execPath)
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrNoBrowser, err)
		}
		return found, nil
	}
	for _, name := range names {
		if found, err := exec.LookPath(name); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("%w (tried %s; name one with launch.browser)",
		ErrNoBrowser, strings.Join(names, ", "))
}
