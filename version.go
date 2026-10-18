// Package photon holds the version of photon, read from the VERSION file beside this one, and
// the build string that pairs it with the commit the binary was built from. It is the only
// library package outside internal/, because go:embed reaches no file above a package's own
// directory and VERSION sits at the module root; the other packages outside internal/ are the
// binaries cmd/photon and cmd/release. cmd/release is the release tool, a self-contained package
// main never shipped in an archive: its non-test code imports only the standard library and
// cobra, and its tests use internal/testkit. It is not an API: it exists so the binary, however
// it is built, reports the version VERSION names.
package photon

import (
	_ "embed" // VERSION is embedded into the binary.
	"runtime/debug"
	"strings"
)

// versionFile is VERSION's contents: the one place photon's version is written, and the file a
// release updates.
//
//go:embed VERSION
var versionFile string

// programName opens the build string.
const programName = "photon"

// The build settings Go stamps into a binary built inside a VCS checkout.
const (
	revisionSetting = "vcs.revision"
	modifiedSetting = "vcs.modified"
)

// shortRevisionLength is how many leading characters of the commit hash the build string shows.
const shortRevisionLength = 7

// dirtyMarker follows the commit when the binary was built from a tree with uncommitted changes.
const dirtyMarker = ", dirty"

// Version returns photon's semantic version, such as "1.0.0", without a leading "v".
func Version() string {
	return strings.TrimSpace(versionFile)
}

// Build returns the build string, such as "photon 1.0.0 (a1b2c3d)" or
// "photon 1.0.0 (a1b2c3d, dirty)": the version, the short commit and whether the tree had
// uncommitted changes, read from the build info Go stamps into the binary. A binary built
// without VCS info, such as by `go run` or from a module download, gives "photon 1.0.0".
func Build() string {
	info, hasInfo := debug.ReadBuildInfo()
	if !hasInfo {
		return build(Version(), nil)
	}
	return build(Version(), info)
}

// build formats the build string from version and info; a nil info or one without a
// vcs.revision setting gives the version alone.
func build(version string, info *debug.BuildInfo) string {
	bare := programName + " " + version
	if info == nil {
		return bare
	}
	var revision string
	isModified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case revisionSetting:
			revision = setting.Value
		case modifiedSetting:
			isModified = setting.Value == "true"
		}
	}
	if revision == "" {
		return bare
	}
	if len(revision) > shortRevisionLength {
		revision = revision[:shortRevisionLength]
	}
	if isModified {
		revision += dirtyMarker
	}
	return bare + " (" + revision + ")"
}
