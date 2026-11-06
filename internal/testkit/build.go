package testkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// fixturePackage is the import path of the fixture TUI's main package.
const fixturePackage = "github.com/airiclenz/photon/internal/testkit/fixturetui"

// fixtureBinary is the fixture executable's base name, before the platform's suffix.
const fixtureBinary = "fixturetui"

// fixtureDirPattern names the temporary directory the fixture is built into.
const fixtureDirPattern = "photon-fixturetui-"

// fixtureWebPackage is the import path of the fixture web app's main package.
const fixtureWebPackage = "github.com/airiclenz/photon/internal/testkit/fixtureweb"

// fixtureWebBinary is the fixture web app's base name, before the platform's suffix.
const fixtureWebBinary = "fixtureweb"

// fixtureWebDirPattern names the temporary directory the fixture web app is built into.
const fixtureWebDirPattern = "photon-fixtureweb-"

// fixtureBuild builds the fixture TUI once per test binary; every later call shares its result.
var fixtureBuild = sync.OnceValues(func() (string, error) {
	return buildFixture(fixturePackage, fixtureBinary, fixtureDirPattern)
})

// fixtureWebBuild builds the fixture web app once per test binary; every later call shares its
// result.
var fixtureWebBuild = sync.OnceValues(func() (string, error) {
	return buildFixture(fixtureWebPackage, fixtureWebBinary, fixtureWebDirPattern)
})

// BuildFixtureTUI builds the fixture TUI (package fixturetui) once per test binary and returns
// the executable's path; later calls, from any test, return the same path without building again.
// A failed build fails t, and every later call fails the same way.
//
// The executable lives in a directory this package owns, not in t's temporary directory, so it
// outlives the test that first asked for it. The directory sits under the system temporary
// directory and is not removed when the test binary exits.
func BuildFixtureTUI(t testing.TB) string {
	t.Helper()

	path, err := fixtureBuild()
	if err != nil {
		t.Fatalf("build the fixture TUI: %v", err)
	}
	return path
}

// BuildFixtureWeb builds the fixture web app (package fixtureweb) once per test binary and returns
// the executable's path, the way [BuildFixtureTUI] builds the fixture TUI: later calls return the
// same path, a failed build fails t and every later call, and the executable outlives the test
// that first asked for it.
//
// The app serves its page on 127.0.0.1:$PHOTON_PORT and prints "ready" once it listens.
func BuildFixtureWeb(t testing.TB) string {
	t.Helper()

	path, err := fixtureWebBuild()
	if err != nil {
		t.Fatalf("build the fixture web app: %v", err)
	}
	return path
}

// buildFixture compiles the main package pkg, as an executable named binary, into a fresh
// temporary directory named after dirPattern, with the go command on PATH (`go test` puts its own
// toolchain first), and returns the executable's path.
func buildFixture(pkg, binary, dirPattern string) (string, error) {
	dir, err := os.MkdirTemp("", dirPattern)
	if err != nil {
		return "", fmt.Errorf("create the build directory: %w", err)
	}
	path := filepath.Join(dir, executableName(binary, runtime.GOOS))

	command := exec.Command("go", "build", "-o", path, pkg)
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build %s: %w\n%s", pkg, err, output)
	}
	return path, nil
}

// executableName returns base as an executable's file name on goos: with ".exe" on Windows.
func executableName(base, goos string) string {
	if goos == "windows" {
		return base + ".exe"
	}
	return base
}
