package storyboard

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseRuns_Grammar(t *testing.T) {
	t.Parallel()

	white := RGB{R: 0xff, G: 0xff, B: 0xff}
	red := RGB{R: 0xff}
	teal := RGB{G: 0x80, B: 0x80}
	cases := []struct {
		name string
		text string
		want []Run
	}{
		{"no tags is one run", "Pick an item", []Run{{"Pick an item", white, WeightRegular}}},
		{"a doubled [ is a literal [", "Press [[Enter]", []Run{{"Press [Enter]", white, WeightRegular}}},
		{"a lone ] needs no escape", "a] b]", []Run{{"a] b]", white, WeightRegular}}},
		{"a colour span", "go [#ff0000]now[/] please", []Run{
			{"go ", white, WeightRegular}, {"now", red, WeightRegular}, {" please", white, WeightRegular},
		}},
		{"a bold span", "[bold]Saved[/]", []Run{{"Saved", white, WeightBold}}},
		{"a bold colour span", "a[bold #008080]b[/]", []Run{{"a", white, WeightRegular}, {"b", teal, WeightBold}}},
		{"tokens in either order, spaced", "[ #008080  bold ]b[ / ]", []Run{{"b", teal, WeightBold}}},
		{"an upper-case colour", "[#FF0000]x[/]", []Run{{"x", red, WeightRegular}}},
		{"a span crosses a newline", "one [bold]two\nthree[/]", []Run{
			{"one ", white, WeightRegular}, {"two\nthree", white, WeightBold},
		}},
		{"spans side by side", "[bold]a[/][#ff0000]b[/]", []Run{{"a", white, WeightBold}, {"b", red, WeightRegular}}},
		{"an escape inside a span", "[bold][[x[/]", []Run{{"[x", white, WeightBold}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runs, err := parseRuns(tc.text, white, WeightRegular)

			if err != nil {
				t.Fatalf("want no error, got %v", err)
			}
			if !reflect.DeepEqual(runs, tc.want) {
				t.Errorf("runs: want %+v, got %+v", tc.want, runs)
			}
		})
	}
}

func TestParseRuns_CaptionWeightUnderSpans(t *testing.T) {
	t.Parallel()
	white, red := RGB{R: 0xff, G: 0xff, B: 0xff}, RGB{R: 0xff}

	runs, err := parseRuns("a[#ff0000]b[/]", white, WeightBold)

	want := []Run{{"a", white, WeightBold}, {"b", red, WeightBold}}
	if err != nil || !reflect.DeepEqual(runs, want) {
		t.Errorf("want %+v from a bold caption's colour span, got %+v, %v", want, runs, err)
	}
}

func TestParseRuns_Refuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, text, want string
	}{
		{"unknown token", "Press [Enter]", `tag [Enter]: want bold or #rrggbb, got "Enter"`},
		{"short colour", "[#fff]x[/]", `tag [#fff]: want bold or #rrggbb, got "#fff"`},
		{"two colours", "[#ff0000 #00ff00]x[/]", "tag [#ff0000 #00ff00]: want at most one colour"},
		{"no token", "a [] b", "tag []: want bold or #rrggbb, got no token"},
		{"no closing ]", "Press [Enter", `tag "[Enter" has no closing ]`},
		{"unclosed span", "[bold]Saved", "span [bold] is never closed by [/]"},
		{"close without a span", "Saved[/]", "[/] closes no span"},
		{"nested tag", "[bold]a[#ff0000]b[/][/]", "tag [#ff0000] inside span [bold]: spans do not nest"},
		{"empty span", "a[bold][/]b", "span [bold] holds no text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseRuns(tc.text, RGB{}, WeightRegular)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
			if !errors.Is(err, errSpanMarkup) || !strings.Contains(err.Error(), "write [[ for a literal [") {
				t.Errorf("want the error to name the [[ escape, got %v", err)
			}
		})
	}
}

func TestParse_CaptionRuns(t *testing.T) {
	t.Parallel()

	source := strings.Replace(captionBase, "text: 'Done'", `text: "Tests [bold #00ff00]pass[/]\n"`, 1)
	board, err := Parse([]byte(source), "captions.yaml")

	if err != nil {
		t.Fatalf("want a styled caption to parse, got %v", err)
	}
	white := RGB{R: 0xff, G: 0xff, B: 0xff}
	want := []Run{{"Tests ", white, WeightRegular}, {"pass", RGB{G: 0xff}, WeightBold}}
	if got := board.Captions[1].Runs(); !reflect.DeepEqual(got, want) {
		t.Errorf("runs: want %+v with no trailing newline run, got %+v", want, got)
	}
	if got := board.Captions[1].Text; got != "Tests [bold #00ff00]pass[/]" {
		t.Errorf("text: want the markup kept as written, got %q", got)
	}
}

func TestParse_RefusesBadMarkupUnderTheCaption(t *testing.T) {
	t.Parallel()

	source := strings.Replace(captionBase, "text: 'Done'", "text: 'Press [Enter]'", 1)
	_, err := Parse([]byte(source), "captions.yaml")

	var invalid *ValidationError
	if !errors.As(err, &invalid) || len(invalid.Problems) != 1 {
		t.Fatalf("want one validation problem, got %v", err)
	}
	want := `captions[1]: text: tag [Enter]: want bold or #rrggbb, got "Enter" (write [[ for a literal [)`
	if got := invalid.Problems[0].Error(); !strings.Contains(got, want) {
		t.Errorf("problem: want %q, got %q", want, got)
	}
	if owner := OwnerOf(invalid.Problems[0]); owner != (Owner{Kind: OwnerCaption, Index: 1}) {
		t.Errorf("owner: want caption 1, got %+v", owner)
	}
}
