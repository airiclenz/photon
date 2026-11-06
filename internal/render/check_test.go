package render

import (
	"errors"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/compose"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// oversizedCaptionStoryboard is renderStoryboard with a caption, second in the list, whose box
// is wider than the 160-pixel frame: its text is one word, which no wrap can break.
const oversizedCaptionStoryboard = renderStoryboard + `captions:
  - {text: one, at: 100ms, for: 700ms}
  - {text: 'fartoowideforthisframe', at: 0s, for: 1s, font: {size: 72}}
`

// pageConfig is the url-launch tests' photon config: a page in place of renderConfig's command.
const pageConfig = `name: rendered
launch:
  url: http://127.0.0.1:8080/
`

// pageStoryboard is renderStoryboard on a page: a 160×100 CSS-pixel viewport at device scale 2,
// rasterized at scale 2 and shipped 160 pixels wide, beat 1 lasting 1s and beat 2 1.5s.
const pageStoryboard = `clip: rendered
ship: out/rendered.gif
frame: {viewport: {width: 160, height: 100}, device_scale: 2, scale: 2, width: 160, fps: 10, max_colors: 16}
beats:
  - id: 1
    title: typing
    duration: 1s
  - id: 2
    title: hold
    duration: 1500ms
`

// pageCaptionedStoryboard is pageStoryboard with one caption that sets no font.size.
const pageCaptionedStoryboard = pageStoryboard + `captions:
  - {text: one, at: 100ms, for: 700ms}
`

// designScale is the raster scale a caption's design pixels are measured at, the compositor's.
const designScale = 2

func TestCheckCaptions_CaptionsThatFit_AreClean(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		board string
	}{
		{name: "no captions", board: renderStoryboard},
		{name: "captions that fit", board: captionedStoryboard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loaded := loadRenderClipOf(t, tt.board)

			err := CheckCaptions(loaded)

			if err != nil {
				t.Errorf("CheckCaptions: %v, want nil", err)
			}
		})
	}
}

func TestCheckCaptions_BoxLargerThanTheFrame_IsTheCaptionsProblem(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, oversizedCaptionStoryboard)

	err := CheckCaptions(loaded)

	var invalid *storyboard.ValidationError
	if !errors.As(err, &invalid) || len(invalid.Problems) != 1 {
		t.Fatalf("CheckCaptions: %v, want a validation error with one problem", err)
	}
	want := storyboard.Owner{Kind: storyboard.OwnerCaption, Index: 1}
	if owner := storyboard.OwnerOf(invalid.Problems[0]); owner != want {
		t.Errorf("owner = %+v, want %+v", owner, want)
	}
	if !strings.Contains(err.Error(), "captions[1]: ") || !strings.Contains(err.Error(), "larger than the") {
		t.Errorf("error %q does not name captions[1] and the box larger than the frame", err)
	}
}

// TestCheckCaptions_URLClip_LoadsNoTerminalFont counts the process-wide terminal-font loads, so
// it runs before the parallel tests are let go.
func TestCheckCaptions_URLClip_LoadsNoTerminalFont(t *testing.T) {
	loaded := loadRenderClipWith(t, pageConfig, pageCaptionedStoryboard)
	before := terminalLoads.Load()

	err := CheckCaptions(loaded)

	if err != nil {
		t.Fatalf("CheckCaptions: %v, want nil", err)
	}
	if loads := terminalLoads.Load() - before; loads != 0 {
		t.Errorf("checking a url clip loaded the terminal fonts %d times, want none", loads)
	}
}

func TestCheckCaptions_URLClipWithoutAFontSize_LaysOutAt16CSSPixels(t *testing.T) {
	t.Parallel()
	// One word that no wrap can break: at 16 CSS pixels it is wider than the 160-pixel viewport
	// less the caption's margins; at half that, as when the device scale is left out, it fits.
	const word = "fartoowideforthisframe"
	tests := []struct {
		name      string
		size      string
		isTooWide bool
	}{
		{name: "the default of 16", size: "", isTooWide: true},
		{name: "half the default", size: ", font: {size: 8}", isTooWide: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := pageStoryboard + "captions:\n  - {text: " + word + ", at: 0s, for: 1s" +
				tt.size + "}\n"
			loaded := loadRenderClipWith(t, pageConfig, board)

			err := CheckCaptions(loaded)

			var invalid *storyboard.ValidationError
			if isTooWide := errors.As(err, &invalid); isTooWide != tt.isTooWide {
				t.Errorf("CheckCaptions: %v; want a caption too wide = %t", err, tt.isTooWide)
			}
		})
	}
}

func TestPageOutput_DeviceScale_SizesTheFrameAndCaptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		deviceScale float64
		wantSize    image.Point
	}{
		{name: "device scale 1", deviceScale: 1, wantSize: image.Pt(101*3, 51*3)},
		{name: "device scale 2", deviceScale: 2, wantSize: image.Pt(202*3, 102*3)},
		{name: "device scale 1.5 rounds each side", deviceScale: 1.5, wantSize: image.Pt(152*3, 77*3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			frame := storyboard.Frame{
				Viewport:    storyboard.Viewport{Width: 101, Height: 51},
				DeviceScale: tt.deviceScale,
				Scale:       3,
				Width:       200,
			}

			layout := pageLayout(frame)
			_, unit, err := pageOutput(frame)

			if err != nil {
				t.Fatalf("pageOutput: %v", err)
			}
			wantLayout := compose.Layout{
				Size:  tt.wantSize,
				Cell:  take.CellSize{W: tt.deviceScale, H: tt.deviceScale},
				Scale: frame.Scale,
			}
			if layout != wantLayout {
				t.Errorf("pageLayout = %+v, want %+v", layout, wantLayout)
			}
			// A caption's CSS pixel spans device_scale × scale pixels of the frame before it
			// is scaled down to frame.width.
			captionScale := unit * designScale * float64(layout.Size.X) / float64(frame.Width)
			if want := tt.deviceScale * float64(frame.Scale); math.Abs(captionScale-want) > 1e-9 {
				t.Errorf("caption scale = %g, want %g", captionScale, want)
			}
		})
	}
}
