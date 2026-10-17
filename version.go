// Package photon holds the version of photon, read from the VERSION file beside this one. It is
// the only package outside internal/ and cmd/photon, because go:embed reaches no file above a
// package's own directory and VERSION sits at the module root. It is not an API: it exists so
// the binary, however it is built, reports the version VERSION names.
package photon

import (
	_ "embed" // VERSION is embedded into the binary.
	"strings"
)

// versionFile is VERSION's contents: the one place photon's version is written, and the file a
// release updates.
//
//go:embed VERSION
var versionFile string

// Version returns photon's semantic version, such as "1.0.0", without a leading "v".
func Version() string {
	return strings.TrimSpace(versionFile)
}
