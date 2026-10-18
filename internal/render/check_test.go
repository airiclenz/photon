package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/airiclenz/photon/internal/storyboard"
)

// oversizedCaptionStoryboard is renderStoryboard with a caption, second in the list, whose box
// is wider than the 160-pixel frame.
const oversizedCaptionStoryboard = renderStoryboard + `captions:
  - {text: one, at: 100ms, for: 700ms}
  - {text: 'far too wide for this frame', at: 0s, for: 1s, font: {size: 72}}
`

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
