package photon

import (
	"regexp"
	"testing"
)

// semver matches a release version: MAJOR.MINOR.PATCH with no leading zeros and no "v".
var semver = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// TestVersionIsSemantic fails when VERSION holds anything but a bare semantic version, so a
// release that writes it wrongly is caught before it is tagged.
func TestVersionIsSemantic(t *testing.T) {
	t.Parallel()

	got := Version()

	if !semver.MatchString(got) {
		t.Errorf("Version() = %q, want MAJOR.MINOR.PATCH such as 1.0.0", got)
	}
}
