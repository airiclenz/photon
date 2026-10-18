package caption

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/storyboard"
)

// boxedCaption is a one-second caption of short text in an opaque box, with extra placement
// fields appended.
func boxedCaption(placement string) string {
	return "  - {text: Hi, at: 0s, for: 1s, box: {opacity: 1}" + placement + "}\n"
}

// assertNear fails when got is more than a pixel from want.
func assertNear(t *testing.T, what string, got int, want float64) {
	t.Helper()
	if math.Abs(float64(got)-want) > 1 {
		t.Errorf("%s = %d, want %.1f", what, got, want)
	}
}

func TestNew_PlaceAndAlign_PutTheBoxAMarginInFromTheEdges(t *testing.T) {
	margin := edgeMargin * testUnit
	for _, place := range []string{"top", "middle", "bottom"} {
		for _, align := range []string{"left", "middle", "right"} {
			t.Run(place+" "+align, func(t *testing.T) {
				layer := newTestLayer(t, boxedCaption(", place: "+place+", align: "+align), testOut, testUnit)

				box := inkRect(drawAt(layer, testOut, 0))

				wantX := map[string]float64{
					"left":   margin,
					"middle": float64(testOut.X-box.Dx()) / 2,
					"right":  float64(testOut.X-box.Dx()) - margin,
				}[align]
				wantY := map[string]float64{
					"top":    margin,
					"middle": float64(testOut.Y-box.Dy()) / 2,
					"bottom": float64(testOut.Y-box.Dy()) - margin,
				}[place]
				assertNear(t, "box left", box.Min.X, wantX)
				assertNear(t, "box top", box.Min.Y, wantY)
			})
		}
	}
}

func TestNew_OffsetAndPosition_MoveTheBoxInDesignPixels(t *testing.T) {
	margin := edgeMargin * testUnit
	tests := []struct {
		name      string
		placement string
		wantLeft  float64
		wantTop   float64
	}{
		{"offset from top left", ", place: top, align: left, offset: {x: 20, y: 10}", margin + 10, margin + 5},
		{"negative offset", ", place: top, align: left, offset: {x: 0, y: -8}", margin, margin - 4},
		{"position", ", position: {x: 100, y: 40}", 50, 20},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			layer := newTestLayer(t, boxedCaption(test.placement), testOut, testUnit)

			box := inkRect(drawAt(layer, testOut, 0))

			assertNear(t, "box left", box.Min.X, test.wantLeft)
			assertNear(t, "box top", box.Min.Y, test.wantTop)
		})
	}
}

func TestNew_BoxPastAnEdge_IsClampedInsideTheFrame(t *testing.T) {
	tests := []struct {
		name      string
		placement string
		corner    func(image.Rectangle) image.Point
		want      image.Point
	}{
		{"offset past the bottom right", ", place: bottom, align: right, offset: {x: 400, y: 400}",
			func(box image.Rectangle) image.Point { return box.Max }, testOut},
		{"offset past the top left", ", place: top, align: left, offset: {x: -400, y: -400}",
			func(box image.Rectangle) image.Point { return box.Min }, image.Point{}},
		{"position past the bottom right", ", position: {x: 5000, y: 5000}",
			func(box image.Rectangle) image.Point { return box.Max }, testOut},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			layer := newTestLayer(t, boxedCaption(test.placement), testOut, testUnit)

			box := inkRect(drawAt(layer, testOut, 0))

			if got := test.corner(box); got != test.want {
				t.Errorf("box %v: corner %v, want %v", box, got, test.want)
			}
		})
	}
}

func TestNew_BoxLargerThanTheFrame_IsRefused(t *testing.T) {
	tests := []struct {
		name    string
		caption string
		out     image.Point
	}{
		{"too wide", "  - {text: " + strings.Repeat("W", 120) + ", at: 0s, for: 1s}\n", testOut},
		{"too tall", "  - {text: \"one\\ntwo\\nthree\", at: 0s, for: 1s}\n", image.Pt(600, 40)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(parseBoard(t, test.caption), loadFaces(t), test.out, testUnit)

			if err == nil || !strings.HasPrefix(err.Error(), "caption: captions[0]: ") {
				t.Errorf("New error = %v, want one naming captions[0]", err)
			}
			if got, want := storyboard.OwnerOf(err), (storyboard.Owner{Kind: storyboard.OwnerCaption}); got != want {
				t.Errorf("OwnerOf(New error) = %+v, want %+v", got, want)
			}
		})
	}
}
