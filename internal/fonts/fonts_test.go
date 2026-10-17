package fonts

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

// licenceMarker is a phrase every SIL Open Font License text carries.
const licenceMarker = "SIL Open Font License"

// controlKey is the rune Noto Sans Symbols is last in the chain for.
const controlKey = '⌃'

func TestLoadEmbeddedParsesEveryFace(t *testing.T) {
	set, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	for _, face := range faces(set) {
		if face.Font == nil || face.Font.NumGlyphs() == 0 {
			t.Errorf("%s: want a parsed font with glyphs, got %v", face.Name, face.Font)
		}
	}
}

func TestLoadFallbackOrder(t *testing.T) {
	set, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := make([]string, 0, len(set.Fallbacks))
	for _, face := range set.Fallbacks {
		got = append(got, face.Name)
	}

	want := []string{fileSymbols2, fileMath, fileSymbols}
	if !slices.Equal(got, want) {
		t.Errorf("fallbacks: want %v, got %v", want, got)
	}
	if !hasRune(t, set.Fallbacks[len(set.Fallbacks)-1].Font, controlKey) {
		t.Errorf("%s: want a glyph for %q", fileSymbols, controlKey)
	}
}

func TestEveryFaceHasAnEmbeddedLicence(t *testing.T) {
	covered := map[string]bool{}
	for _, file := range slices.Concat(primaryFiles[:], fallbackFiles) {
		covered[file.name] = true
		text, err := fs.ReadFile(embedded, file.licence)
		if err != nil {
			t.Errorf("%s: read its licence: %v", file.name, err)
			continue
		}
		if !strings.Contains(string(text), licenceMarker) {
			t.Errorf("%s: licence %s does not read as an OFL text", file.name, file.licence)
		}
	}

	fonts, err := fs.Glob(embedded, "*.ttf")
	if err != nil {
		t.Fatalf("list the embedded fonts: %v", err)
	}
	for _, name := range fonts {
		if !covered[name] {
			t.Errorf("%s is embedded but has no role or licence", name)
		}
	}
}

func TestLoadOverrideDirReplacesTheEmbed(t *testing.T) {
	dir := t.TempDir()
	writeOverride(t, dir, "")
	// The bold file under the regular name proves the regular face came from dir.
	copyEmbedded(t, fileBold, filepath.Join(dir, fileRegular))

	set, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got, err := set.Regular.Font.Name(nil, sfnt.NameIDFull)
	if err != nil {
		t.Fatalf("read the regular face's name: %v", err)
	}
	if got != "Source Code Pro Bold" {
		t.Errorf("regular face: want the override's Source Code Pro Bold, got %q", got)
	}
}

func TestLoadOverrideDirFailures(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		corrupt bool
		wantErr string
	}{
		{"missing regular", fileRegular, false, "read " + fileRegular},
		{"missing italic", fileItalic, false, "read " + fileItalic},
		{"missing fallback", fileMath, false, "read " + fileMath},
		{"unparsable fallback", fileSymbols, true, "parse " + fileSymbols},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeOverride(t, dir, tc.file)
			if tc.corrupt {
				writeFile(t, filepath.Join(dir, tc.file), []byte("not a font"))
			}

			_, err := Load(dir)

			if err == nil {
				t.Fatal("want an error, got none")
			}
			if !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), dir) {
				t.Errorf("want an error naming %q in %s, got %v", tc.wantErr, dir, err)
			}
			if !tc.corrupt && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("want fs.ErrNotExist in the chain, got %v", err)
			}
		})
	}
}

// faces lists every face in set, primaries first.
func faces(set Set) []Face {
	return append([]Face{set.Regular, set.Bold, set.Italic}, set.Fallbacks...)
}

// hasRune reports whether font has a glyph for r.
func hasRune(t *testing.T, font *sfnt.Font, r rune) bool {
	t.Helper()
	var buffer sfnt.Buffer
	index, err := font.GlyphIndex(&buffer, r)
	if err != nil {
		t.Fatalf("look up %q: %v", r, err)
	}
	return index != 0
}

// writeOverride copies every embedded font file except skip into dir.
func writeOverride(t *testing.T, dir, skip string) {
	t.Helper()
	for _, file := range slices.Concat(primaryFiles[:], fallbackFiles) {
		if file.name != skip {
			copyEmbedded(t, file.name, filepath.Join(dir, file.name))
		}
	}
}

// copyEmbedded writes the embedded file name to path.
func copyEmbedded(t *testing.T, name, path string) {
	t.Helper()
	data, err := fs.ReadFile(embedded, name)
	if err != nil {
		t.Fatalf("read embedded %s: %v", name, err)
	}
	writeFile(t, path, data)
}

// writeFile writes data to path.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
