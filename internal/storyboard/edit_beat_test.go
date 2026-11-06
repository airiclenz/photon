package storyboard

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTarget_YAMLFlow_WritesTheTargetsOneSpelling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		target Target
		want   string
	}{
		{name: "a text target", target: Target{Text: "ok"}, want: "{text: 'ok'}"},
		{name: "a text target's nth and area", target: Target{Text: "ok", Nth: 2, Area: "list"},
			want: "{text: 'ok', nth: 2, area: 'list'}"},
		{name: "the last match", target: Target{Text: "ok", Nth: NthLast},
			want: "{text: 'ok', nth: last}"},
		{name: "a selector target", target: Target{Selector: "li.item", Nth: 2},
			want: "{selector: 'li.item', nth: 2}"},
		{name: "a selector target's area", target: Target{Selector: "#save", Area: "form"},
			want: "{selector: '#save', area: 'form'}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			flow := test.target.YAMLFlow()

			if flow != test.want {
				t.Errorf("YAMLFlow: want %q, got %q", test.want, flow)
			}
			var decoded Target
			if err := yaml.Unmarshal([]byte(flow), &decoded); err != nil {
				t.Fatalf("decode %q: %v", flow, err)
			}
			if decoded != test.target {
				t.Errorf("%q decodes to %+v, want %+v", flow, decoded, test.target)
			}
		})
	}
}
