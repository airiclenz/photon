package take

import (
	"fmt"
	"strconv"
	"strings"
)

// Cell is one cell of a snapshot: the grapheme it shows, how many columns that grapheme occupies
// (0 for the cells a wide grapheme covers after its first), and every style attribute a picture
// of it needs.
type Cell struct {
	// Grapheme is one user-perceived character: one or more code points (a base letter and its
	// combining marks stay together). Empty in a cell a wide grapheme covers.
	Grapheme  string `json:"g,omitempty"`
	Width     int    `json:"w"`
	FG        Color  `json:"fg,omitzero"`
	BG        Color  `json:"bg,omitzero"`
	Bold      bool   `json:"b,omitempty"`
	Faint     bool   `json:"f,omitempty"`
	Italic    bool   `json:"i,omitempty"`
	Underline bool   `json:"u,omitempty"`
	Reverse   bool   `json:"rv,omitempty"`
}

// ColorKind says which form a [Color] takes.
type ColorKind uint8

// The forms of a cell colour.
const (
	// ColorDefault is the terminal's own foreground or background, which the theme decides.
	ColorDefault ColorKind = iota
	// ColorPalette is entry Index of the 256-colour palette, kept unresolved so the theme decides
	// what the sixteen ANSI colours look like, exactly as a real terminal's theme does.
	ColorPalette
	// ColorRGB is the truecolor value R, G, B.
	ColorRGB
)

// The spellings of a colour in the take file: "" is the default, "@N" palette entry N and
// "#rrggbb" a truecolor value.
const (
	palettePrefix = "@"
	rgbPrefix     = "#"
	rgbHexDigits  = 6
)

// Color is a cell colour as the program asked for it. The zero value is [ColorDefault]; Index is
// read only for [ColorPalette], and R, G, B only for [ColorRGB].
type Color struct {
	Kind  ColorKind
	Index uint8
	R     uint8
	G     uint8
	B     uint8
}

// MarshalText spells the colour as the take file stores it: "", "@N" or "#rrggbb".
func (c Color) MarshalText() ([]byte, error) {
	switch c.Kind {
	case ColorDefault:
		return []byte{}, nil
	case ColorPalette:
		return []byte(palettePrefix + strconv.Itoa(int(c.Index))), nil
	case ColorRGB:
		return fmt.Appendf(nil, "%s%02x%02x%02x", rgbPrefix, c.R, c.G, c.B), nil
	}
	return nil, fmt.Errorf("colour kind %d is not one a take stores", c.Kind)
}

// UnmarshalText reads a colour [Color.MarshalText] spelled; any other spelling is an error.
func (c *Color) UnmarshalText(text []byte) error {
	spelling := string(text)
	switch {
	case spelling == "":
		*c = Color{}
		return nil
	case strings.HasPrefix(spelling, palettePrefix):
		index, err := strconv.ParseUint(spelling[len(palettePrefix):], 10, 8)
		if err != nil {
			return fmt.Errorf("colour %q: not a palette index 0-255", spelling)
		}
		*c = Color{Kind: ColorPalette, Index: uint8(index)}
		return nil
	case strings.HasPrefix(spelling, rgbPrefix) && len(spelling) == len(rgbPrefix)+rgbHexDigits:
		value, err := strconv.ParseUint(spelling[len(rgbPrefix):], 16, 32)
		if err != nil {
			return fmt.Errorf("colour %q: not #rrggbb", spelling)
		}
		*c = Color{Kind: ColorRGB, R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value)}
		return nil
	}
	return fmt.Errorf("colour %q: want \"\", @N or #rrggbb", spelling)
}
