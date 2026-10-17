package testkit

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// fixtureSource is the body of every Go file a fixture package holds.
const fixtureSource = "package fixture\n"

func TestDocMap(t *testing.T) {
	CheckDocMap(t)
}

func TestUnmappedReportsTheFilesTheMapOmits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		doc   string
		files []string
		want  []string
	}{
		{
			name:  "every file named",
			doc:   "// alpha.go leads, beta.go follows.\n// doc.go holds this map.\n",
			files: []string{"alpha.go", "beta.go"},
			want:  nil,
		},
		{
			name:  "an unnamed file is reported",
			doc:   "// alpha.go leads.\n// doc.go holds this map.\n",
			files: []string{"alpha.go", "orphan.go"},
			want:  []string{"orphan.go"},
		},
		{
			name:  "test files and non-Go files are exempt",
			doc:   "// alpha.go leads.\n// doc.go holds this map.\n",
			files: []string{"alpha.go", "alpha_test.go", "support_test.go", "fixture.yaml"},
			want:  nil,
		},
		{
			name:  "a name that is only a substring of a longer one is not named",
			doc:   "// configwatch.go polls the file.\n// doc.go holds this map.\n",
			files: []string{"configwatch.go", "watch.go"},
			want:  []string{"watch.go"},
		},
		{
			name:  "a name inside a path is not named",
			doc:   "// internal/beta.go is elsewhere.\n// doc.go holds this map.\n",
			files: []string{"beta.go"},
			want:  []string{"beta.go"},
		},
		{
			name:  "doc.go must name itself",
			doc:   "// alpha.go leads.\n",
			files: []string{"alpha.go"},
			want:  []string{"doc.go"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := fixture(t, tc.doc, tc.files...)
			got, err := unmapped(dir)
			if err != nil {
				t.Fatalf("unmapped: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("unmapped = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUnmappedRejectsAPackageWithNoDocFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write(t, dir, "alpha.go", fixtureSource)

	if _, err := unmapped(dir); err == nil {
		t.Fatal("unmapped accepted a package with no doc.go; a missing map must fail")
	}
}

// fixture builds a package directory holding doc as its doc.go plus one Go file per name.
func fixture(t *testing.T, doc string, files ...string) string {
	t.Helper()

	dir := t.TempDir()
	write(t, dir, docFile, doc)
	for _, name := range files {
		write(t, dir, name, fixtureSource)
	}
	return dir
}

// write creates dir/name holding body.
func write(t *testing.T, dir, name, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
