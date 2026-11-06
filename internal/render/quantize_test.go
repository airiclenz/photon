package render

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

// gradientSide is the width and height of gradientImage: 64×64 pixels, 4096 distinct colours.
const gradientSide = 64

// gradientImage is an opaque image whose every pixel is a colour of its own, red rising along x,
// green along y and blue along both. Its bounds start away from the origin so a bounds mistake
// shows.
func gradientImage(t *testing.T) *image.RGBA {
	t.Helper()
	img := image.NewRGBA(image.Rect(3, 5, 3+gradientSide, 5+gradientSide))
	for y := range gradientSide {
		for x := range gradientSide {
			img.SetRGBA(3+x, 5+y, color.RGBA{
				R: uint8(x * 4),
				G: uint8(y * 4),
				B: uint8((x + y) * 2),
				A: 0xff,
			})
		}
	}
	return img
}

// distinctColourCount is the number of distinct colours inside img's bounds.
func distinctColourCount(t *testing.T, img *image.RGBA) int {
	t.Helper()
	seen := make(map[color.RGBA]bool)
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			seen[img.RGBAAt(x, y)] = true
		}
	}
	return len(seen)
}

func TestQuantize_Gradient_HoldsAtMostMaxColors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		maxColors int
	}{
		{name: "2 colours", maxColors: 2},
		{name: "16 colours", maxColors: 16},
		{name: "256 colours", maxColors: 256},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			img := gradientImage(t)

			got := Quantize(img, tt.maxColors)

			if count := distinctColourCount(t, got); count > tt.maxColors || count < 2 {
				t.Errorf("Quantize(gradient, %d) holds %d colours, want 2..%d",
					tt.maxColors, count, tt.maxColors)
			}
		})
	}
}

func TestQuantize_Gradient_KeepsBounds(t *testing.T) {
	t.Parallel()
	img := gradientImage(t)

	got := Quantize(img, 16)

	if got.Bounds() != img.Bounds() {
		t.Errorf("Quantize bounds = %v, want %v", got.Bounds(), img.Bounds())
	}
}

func TestQuantize_OpaqueInput_StaysOpaque(t *testing.T) {
	t.Parallel()
	img := gradientImage(t)

	got := Quantize(img, 16)

	bounds := got.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if alpha := got.RGBAAt(x, y).A; alpha != 0xff {
				t.Fatalf("pixel (%d, %d) alpha = %d, want 255", x, y, alpha)
			}
		}
	}
}

func TestQuantize_FewerColoursThanMax_IsPixelIdentical(t *testing.T) {
	t.Parallel()
	colours := []color.RGBA{
		{R: 0x10, G: 0x20, B: 0x30, A: 0xff},
		{R: 0xf0, G: 0xe0, B: 0xd0, A: 0xff},
		{R: 0x80, G: 0x00, B: 0x80, A: 0xff},
		{R: 0x00, G: 0xff, B: 0x00, A: 0xff},
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetRGBA(x, y, colours[(x+y)%len(colours)])
		}
	}
	want := bytes.Clone(img.Pix)

	got := Quantize(img, 16)

	if got.Bounds() != img.Bounds() || !bytes.Equal(got.Pix, want) {
		t.Errorf("Quantize of a 4-colour image with max 16 changed its pixels")
	}
}

func TestQuantize_TwiceOnOneImage_GivesEqualPixels(t *testing.T) {
	t.Parallel()
	img := gradientImage(t)
	before := bytes.Clone(img.Pix)

	first := Quantize(img, 16)
	second := Quantize(img, 16)

	if !bytes.Equal(first.Pix, second.Pix) {
		t.Errorf("two Quantize calls on one image gave different pixels")
	}
	if !bytes.Equal(img.Pix, before) {
		t.Errorf("Quantize changed its input image")
	}
}
