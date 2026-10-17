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

// fixtureBuild builds the fixture once per test binary; every later call shares its result.
var fixtureBuild = sync.OnceValues(buildFixture)

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

// buildFixture compiles the fixture into a fresh temporary directory with the go command on PATH
// (`go test` puts its own toolchain first) and returns the executable's path.
func buildFixture() (string, error) {
	dir, err := os.MkdirTemp("", fixtureDirPattern)
	if err != nil {
		return "", fmt.Errorf("create the build directory: %w", err)
	}
	path := filepath.Join(dir, executableName(fixtureBinary, runtime.GOOS))

	command := exec.Command("go", "build", "-o", path, fixturePackage)
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build %s: %w\n%s", fixturePackage, err, output)
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
