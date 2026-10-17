package storyboard

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// momentHolder decodes a moment under a key, as a storyboard field holds one.
type momentHolder struct {
	At Moment `yaml:"at"`
}

func TestMoment_UnmarshalYAML_Decodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		yaml string
		want Moment
	}{
		{name: "bare clip time", yaml: "at: 4500ms", want: Moment{Offset: 4500 * time.Millisecond}},
		{name: "zero clip time", yaml: "at: 0s", want: Moment{}},
		{name: "beat without offset", yaml: "at: {beat: 3}", want: Moment{Beat: 3}},
		{name: "beat with offset", yaml: "at: {beat: 2, offset: 1s}", want: Moment{Beat: 2, Offset: time.Second}},
		{
			name: "beat with negative offset",
			yaml: "at: {beat: 2, offset: -200ms}",
			want: Moment{Beat: 2, Offset: -200 * time.Millisecond},
		},
		{
			name: "block mapping",
			yaml: "at:\n  beat: 4\n  offset: 1500ms\n",
			want: Moment{Beat: 4, Offset: 1500 * time.Millisecond},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var holder momentHolder

			err := yaml.Unmarshal([]byte(test.yaml), &holder)

			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if holder.At != test.want {
				t.Errorf("At = %+v, want %+v", holder.At, test.want)
			}
		})
	}
}

func TestMoment_UnmarshalYAML_Rejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{name: "scalar without a unit", yaml: "at: 4500", want: `want a duration such as 4500ms, got "4500"`},
		{name: "scalar that is no duration", yaml: "at: soon", want: `got "soon"`},
		{name: "sequence", yaml: "at: [1, 2]", want: "got a sequence"},
		{name: "unknown mapping key", yaml: "at: {beat: 2, ofset: 1s}", want: `unknown key "ofset"`},
		{name: "mapping without a beat", yaml: "at: {offset: 1s}", want: "beat: want a beat id ≥1, got 0"},
		{name: "beat below one", yaml: "at: {beat: -1}", want: "beat: want a beat id ≥1, got -1"},
		{name: "offset that is no duration", yaml: "at: {beat: 2, offset: soon}", want: "moment:"},
		{name: "duplicate key", yaml: "at: {beat: 2, beat: 3}", want: "already defined"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var holder momentHolder

			err := yaml.Unmarshal([]byte(test.yaml), &holder)

			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("Unmarshal error = %v, want it to contain %q", err, test.want)
			}
		})
	}
}

// clockBoard is a storyboard of a 2 s beat, a cut beat, a 3 s beat and a 1 s beat: kept beats
// start at 0, 2 s and 5 s on the output clock, and the clip runs 6 s.
func clockBoard() *Storyboard {
	return &Storyboard{Beats: []Beat{
		{ID: 1, Duration: 2 * time.Second},
		{ID: 2, Duration: 4 * time.Second, Cut: true},
		{ID: 3, Duration: 3 * time.Second},
		{ID: 5, Duration: time.Second},
	}}
}

func TestStoryboard_ClipTime_Resolves(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		moment Moment
		want   time.Duration
	}{
		{name: "bare clip time", moment: Moment{Offset: 4500 * time.Millisecond}, want: 4500 * time.Millisecond},
		{name: "first beat", moment: Moment{Beat: 1}, want: 0},
		{name: "beat after a cut beat", moment: Moment{Beat: 3}, want: 2 * time.Second},
		{name: "beat with offset", moment: Moment{Beat: 5, Offset: 250 * time.Millisecond}, want: 5250 * time.Millisecond},
		{name: "beat with negative offset", moment: Moment{Beat: 3, Offset: -500 * time.Millisecond}, want: 1500 * time.Millisecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := clockBoard().ClipTime(test.moment)

			if err != nil {
				t.Fatalf("ClipTime: %v", err)
			}
			if got != test.want {
				t.Errorf("ClipTime = %s, want %s", got, test.want)
			}
		})
	}
}

func TestStoryboard_ClipTime_Refuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		moment Moment
		want   string
	}{
		{name: "unknown beat", moment: Moment{Beat: 4}, want: "beat 4 is not in the storyboard"},
		{name: "cut beat", moment: Moment{Beat: 2}, want: "beat 2 is cut, so it has no clip time"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := clockBoard().ClipTime(test.moment)

			if err == nil || err.Error() != test.want {
				t.Errorf("ClipTime error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStoryboard_Length_SumsTheKeptBeats(t *testing.T) {
	t.Parallel()

	got := clockBoard().Length()

	if want := 6 * time.Second; got != want {
		t.Errorf("Length = %s, want %s", got, want)
	}
}
