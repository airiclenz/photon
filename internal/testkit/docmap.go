package testkit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docFile is the file a package's doc map lives in.
const docFile = "doc.go"

// CheckDocMap fails t for every non-test .go file in the calling package's directory that the
// package's doc.go does not name. It reads the working directory, which `go test` sets to the
// package's own directory, so a package enlists with a one-line test calling it.
//
// A package with no doc.go fails: the rule is that the map exists, not only that an existing one
// is complete. The check is a name check only; whether the line naming a file says anything
// useful stays a reviewer's call.
func CheckDocMap(t *testing.T) {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("locate the package directory: %v", err)
	}
	missing, err := unmapped(dir)
	if err != nil {
		t.Fatalf("check the doc map: %v", err)
	}
	for _, name := range missing {
		t.Errorf("%s is not named in %s: add the line saying what it holds to the package's file map",
			name, filepath.Join(filepath.Base(dir), docFile))
	}
}

// unmapped returns the non-test .go files in dir that dir's doc.go does not name, in directory
// order. A missing or unreadable doc.go is an error rather than an empty result, so an absent map
// never reads as a complete one.
func unmapped(dir string) ([]string, error) {
	doc, err := os.ReadFile(filepath.Join(dir, docFile))
	if err != nil {
		return nil, fmt.Errorf("read the package's %s: %w", docFile, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read the package directory: %w", err)
	}

	var missing []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !names(doc, name) {
			missing = append(missing, name)
		}
	}
	return missing, nil
}

// names reports whether doc names the file base as a name of its own, not as the tail of a
// longer one: a map that describes configwatch.go must not vouch for a watch.go it never mentions.
func names(doc []byte, base string) bool {
	needle := []byte(base)
	for from := 0; from+len(needle) <= len(doc); {
		at := bytes.Index(doc[from:], needle)
		if at < 0 {
			return false
		}
		at += from
		if at == 0 || !nameByte(doc[at-1]) {
			return true
		}
		from = at + 1
	}
	return false
}

// nameByte reports whether b can belong to a longer file name or path that a match is the tail of.
func nameByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	default:
		return b == '_' || b == '-' || b == '.' || b == '/'
	}
}
