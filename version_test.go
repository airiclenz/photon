package photon

import (
	"regexp"
	"runtime/debug"
	"strings"
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

func TestBuild(t *testing.T) {
	t.Parallel()

	const revision = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{
			name: "clean tree",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: revision},
				{Key: "vcs.modified", Value: "false"},
			}},
			want: "photon 1.2.3 (0123456)",
		},
		{
			name: "dirty tree",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: revision},
				{Key: "vcs.modified", Value: "true"},
			}},
			want: "photon 1.2.3 (0123456, dirty)",
		},
		{
			name: "no VCS settings",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "GOOS", Value: "linux"},
			}},
			want: "photon 1.2.3",
		},
		{
			name: "nil build info",
			info: nil,
			want: "photon 1.2.3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := build("1.2.3", tt.info)

			if got != tt.want {
				t.Errorf("build() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildStartsWithVersion fails when Build drops the version VERSION names, whatever build
// info the test binary carries.
func TestBuildStartsWithVersion(t *testing.T) {
	t.Parallel()

	got := Build()

	if want := "photon " + Version(); got != want && !strings.HasPrefix(got, want+" (") {
		t.Errorf("Build() = %q, want %q or %q followed by the commit", got, want, want)
	}
}
