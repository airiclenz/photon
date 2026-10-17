package engine

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// typingPins are the gaps, in milliseconds, the humanized profile draws for each string at
// typingSeed, generated once by an independent run of the same MINSTD profile and copied here as
// literals, so a change to a band, the odds or the draw order fails the pin draw for draw.
var typingPins = []struct {
	text string
	gaps []int64
}{
	{"hello world", []int64{35, 33, 33, 34, 42, 83, 42, 30, 37, 27}},
	{"ok - done!", []int64{35, 33, 83, 134, 83, 42, 30, 37, 27}},
	// The space after "make" draws a thinking pause.
	{"make it so", []int64{35, 33, 33, 34, 305, 44, 42, 71, 27}},
}

// typingTotals pins the summed gaps of a longer string, which crosses every band.
var typingTotals = []struct {
	text    string
	totalMS int64
}{
	{"the quick brown fox jumps over the lazy dog, then naps.", 2404},
}

func totalMS(gaps []time.Duration) int64 {
	var total time.Duration
	for _, gap := range gaps {
		total += gap
	}
	return total.Milliseconds()
}

func TestHumanize_Pins(t *testing.T) {
	t.Parallel()
	for _, pin := range typingPins {
		t.Run(pin.text, func(t *testing.T) {
			t.Parallel()

			gaps := humanize(pin.text)

			if len(gaps) != len(pin.gaps) {
				t.Fatalf("len(gaps) = %d, want %d", len(gaps), len(pin.gaps))
			}
			for i, gap := range gaps {
				if gap.Milliseconds() != pin.gaps[i] {
					t.Errorf("gap %d = %d ms, want %d ms", i, gap.Milliseconds(), pin.gaps[i])
				}
			}
		})
	}
}

func TestHumanize_Totals(t *testing.T) {
	t.Parallel()
	for _, pin := range typingTotals {
		t.Run(pin.text, func(t *testing.T) {
			t.Parallel()

			gaps := humanize(pin.text)

			if got := totalMS(gaps); got != pin.totalMS {
				t.Errorf("total = %d ms, want %d ms", got, pin.totalMS)
			}
			if want := utf8.RuneCountInString(pin.text) - 1; len(gaps) != want {
				t.Errorf("len(gaps) = %d, want %d (no gap after the last character)", len(gaps), want)
			}
		})
	}
}

// TestHumanize_Profile asserts the profile itself: every gap, classified by the character it
// follows, falls in that character's band, and no string takes more thinking pauses than the cap.
func TestHumanize_Profile(t *testing.T) {
	t.Parallel()
	texts := []string{typingTotals[0].text}
	for _, pin := range typingPins {
		texts = append(texts, pin.text)
	}
	for _, text := range texts {
		runes := []rune(text)
		pauses := 0
		for i, gap := range humanize(text) {
			ms := gap.Milliseconds()
			low, high := bandOf(runes[i])
			if runes[i] == ' ' && ms >= thinkingPauseMinMS {
				pauses++
				low, high = thinkingPauseMinMS, thinkingPauseMaxMS
			}
			if ms < low || ms > high {
				t.Errorf("%q gap %d after %q: %d ms outside %d-%d", text, i, runes[i], ms, low, high)
			}
		}
		if pauses > thinkingPauseLimit {
			t.Errorf("%q takes %d thinking pauses, cap is %d", text, pauses, thinkingPauseLimit)
		}
	}
}

// bandOf is the ordinary gap band after character, a space's thinking pause aside.
func bandOf(character rune) (low, high int64) {
	switch {
	case character == ' ':
		return spaceGapMinMS, spaceGapMaxMS
	case strings.ContainsRune(punctuationCharacters, character):
		return punctuationGapMinMS, punctuationGapMaxMS
	}
	return letterGapMinMS, letterGapMaxMS
}

func TestHumanize_Edges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"one rune", "a", 0},
		{"one multibyte rune", "—", 0},
		{"two runes", "ab", 1},
		{"multibyte runes count once", "a—b", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := len(humanize(tt.text))

			if got != tt.want {
				t.Errorf("len(humanize(%q)) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}
