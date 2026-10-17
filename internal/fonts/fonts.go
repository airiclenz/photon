package fonts

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// The font files, by role. Source Code Pro is the terminal face; the Noto files are its
// fallbacks.
const (
	fileRegular  = "SourceCodePro-Regular.ttf"
	fileBold     = "SourceCodePro-Bold.ttf"
	fileItalic   = "SourceCodePro-It.ttf"
	fileSymbols2 = "NotoSansSymbols2-Regular.ttf"
	fileMath     = "NotoSansMath-Regular.ttf"
	fileSymbols  = "NotoSansSymbols-Regular.ttf"
)

// The OFL texts that cover the font files. One text covers both Noto Sans Symbols families.
const (
	licenceSourceCodePro = "SourceCodePro-OFL.txt"
	licenceMath          = "NotoSansMath-OFL.txt"
	licenceSymbols       = "NotoSansSymbols-OFL.txt"
)

// embedded holds the font files and the licence texts that must travel with them.
//
//go:embed *.ttf *-OFL.txt
var embedded embed.FS

// fontFile is one font file and the licence text that covers it.
type fontFile struct {
	name    string
	licence string
}

// primaryFiles are Source Code Pro's styles, in the order Load fills them in.
var primaryFiles = [...]fontFile{
	{fileRegular, licenceSourceCodePro},
	{fileBold, licenceSourceCodePro},
	{fileItalic, licenceSourceCodePro},
}

// fallbackFiles is the fallback chain after Source Code Pro, in the order it is tried. Noto Sans
// Symbols comes last: it is there for the few runes (⌃ among them) the other two lack.
var fallbackFiles = []fontFile{
	{fileSymbols2, licenceSymbols},
	{fileMath, licenceMath},
	{fileSymbols, licenceSymbols},
}

// Face is one parsed font file: Name is its file name and Font the font the raster draws from.
type Face struct {
	Name string
	Font *sfnt.Font
}

// Set is the faces a clip is drawn with: Source Code Pro as Regular, Bold and Italic, and the
// Fallbacks tried in order for a rune none of those has.
type Set struct {
	Regular   Face
	Bold      Face
	Italic    Face
	Fallbacks []Face
}

// Load parses the faces. With dir empty they come from the embed; otherwise every file is read
// from dir, under the embedded file names, and none falls back to the embed. A file that is
// missing or does not parse is an error naming it.
func Load(dir string) (Set, error) {
	source := fs.FS(embedded)
	origin := "embedded fonts"
	if dir != "" {
		source = os.DirFS(dir)
		origin = "fonts dir " + dir
	}

	var primary [len(primaryFiles)]Face
	for i, file := range primaryFiles {
		face, err := loadFace(source, file.name)
		if err != nil {
			return Set{}, fmt.Errorf("fonts: %s: %w", origin, err)
		}
		primary[i] = face
	}
	fallbacks := make([]Face, 0, len(fallbackFiles))
	for _, file := range fallbackFiles {
		face, err := loadFace(source, file.name)
		if err != nil {
			return Set{}, fmt.Errorf("fonts: %s: %w", origin, err)
		}
		fallbacks = append(fallbacks, face)
	}
	return Set{Regular: primary[0], Bold: primary[1], Italic: primary[2], Fallbacks: fallbacks}, nil
}

// loadFace reads and parses the font file name from source.
func loadFace(source fs.FS, name string) (Face, error) {
	data, err := fs.ReadFile(source, name)
	if err != nil {
		return Face{}, fmt.Errorf("read %s: %w", name, err)
	}
	font, err := opentype.Parse(data)
	if err != nil {
		return Face{}, fmt.Errorf("parse %s: %w", name, err)
	}
	return Face{Name: name, Font: font}, nil
}
