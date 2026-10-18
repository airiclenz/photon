package yamledit

import (
	"errors"
	"testing"
)

func TestReplace_Node_SwapsOnlyItsText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		src   string
		path  Path
		value string
		want  string
	}{
		{"a plain scalar", "a: 1\nb: 2\n", Path{"a"}, "{x: 1}", "a: {x: 1}\nb: 2\n"},
		{"a quoted scalar keeps its comment", "a: 'it''s' # note\nb: 2\n", Path{"a"}, "5", "a: 5 # note\nb: 2\n"},
		{
			"a block scalar keeps the comment on its key's line",
			"a: | # keep\n  line one\n  line two\nb: 2\n", Path{"a"}, "{x: 1}",
			"a: {x: 1} # keep\nb: 2\n",
		},
		{"a flow mapping", "a: {x: 1, y: [2, 3]} # c\nb: 2\n", Path{"a"}, "4500ms", "a: 4500ms # c\nb: 2\n"},
		{
			"a block mapping leaves the trailing blank and comment lines",
			"a:\n  x: 1\n  y:\n    - 2\n\n# trailing\nb: 2\n", Path{"a"}, "{x: 9}",
			"a: {x: 9}\n\n# trailing\nb: 2\n",
		},
		{
			"a block mapping in a sequence item",
			"items:\n  - name: a\n    to: # aim\n      text: 'x'\n    at: 1s\n", Path{"items", "0", "to"}, "{cell: {x: 1, y: 2}}",
			"items:\n  - name: a\n    to: {cell: {x: 1, y: 2}} # aim\n    at: 1s\n",
		},
		{"an indentless block sequence", "a:\n- 1\n- 2\nb: 3\n", Path{"a"}, "[3]", "a: [3]\nb: 3\n"},
		{"a block mapping at the end without a newline", "b: 2\na:\n  x: 1", Path{"a"}, "2", "b: 2\na: 2"},
		{"a scalar in a flow mapping", "a: {x: 1, y: 2}\n", Path{"a", "y"}, "{p: 3}", "a: {x: 1, y: {p: 3}}\n"},
		{"a flow mapping in a flow mapping", "a: {x: {p: 1}, y: 2}\n", Path{"a", "x"}, "7", "a: {x: 7, y: 2}\n"},
		{"a block mapping with CRLF lines", "a:\r\n  x: 1\r\nb: 2\r\n", Path{"a"}, "3", "a: 3\r\nb: 2\r\n"},
		{
			"a block mapping that is a sequence item",
			"a:\n  - x: 1 # gone\n    y: 2\n  - x: 3\n", Path{"a", "0"}, "{x: 2}",
			"a:\n  - {x: 2}\n  - x: 3\n",
		},
		{
			"a block scalar sequence item keeps its header comment",
			"a:\n  - | # keep\n    one\n    two\n  - b\n", Path{"a", "0"}, "'x'",
			"a:\n  - 'x' # keep\n  - b\n",
		},
		{"a block scalar item indented less than its dash's content", "- |\n line\n- b\n", Path{"0"}, "x", "- x\n- b\n"},
		{"a block sequence that is a sequence item", "a:\n  - - 1\n    - 2\n  - 3\n", Path{"a", "0"}, "[9]", "a:\n  - [9]\n  - 3\n"},
		{"an indentless sequence's item", "a:\n- x: 1\n  y: 2\nb: 3\n", Path{"a", "0"}, "5", "a:\n- 5\nb: 3\n"},
		{"an item below its dash", "a:\n  -\n    x: 1\n  - 2\n", Path{"a", "0"}, "{x: 2}", "a:\n  -\n    {x: 2}\n  - 2\n"},
		{
			"a sequence item leaves the trailing blank and comment lines",
			"a:\n  - x: 1\n    y: 2\n\n# trailing\nb: 2\n", Path{"a", "0"}, "{x: 9}",
			"a:\n  - {x: 9}\n\n# trailing\nb: 2\n",
		},
		{"a sequence item at the end without a newline", "a:\n  - x: 1\n    y: 2", Path{"a", "0"}, "7", "a:\n  - 7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := parse(t, tc.src)

			err := document.Replace(tc.path, tc.value)

			if err != nil {
				t.Fatalf("Replace(%s, %q): %v", tc.path, tc.value, err)
			}
			if got := string(document.Bytes()); got != tc.want {
				t.Errorf("Replace(%s, %q) =\n%q\nwant\n%q", tc.path, tc.value, got, tc.want)
			}
		})
	}
}

func TestReplace_Refuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		src   string
		path  Path
		value string
		want  error
	}{
		{"a missing key", "a: 1\n", Path{"b"}, "2", ErrNotFound},
		{"an anchored node", "a: &x {p: 1}\nb: *x\n", Path{"a"}, "2", ErrUnsupported},
		{"an alias", "a: &x 1\nb: *x\n", Path{"b"}, "2", ErrUnsupported},
		{"a tagged scalar", "a: !!str 5\n", Path{"a"}, "6", ErrUnsupported},
		{"a value spanning lines", "a: 1\n", Path{"a"}, "{x: 1,\ny: 2}", ErrUnsupported},
		{"a block mapping at the root", "x: 1\ny: 2\n", Path{}, "{x: 2}", ErrUnsupported},
		{"a plain scalar spanning lines", "a: one\n  two\nb: 2\n", Path{"a"}, "3", ErrUnsupported},
		{"a result that does not parse", "a: 1\nb: 2\n", Path{"a"}, "{x: 1", ErrUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := parse(t, tc.src)

			err := document.Replace(tc.path, tc.value)

			if !errors.Is(err, tc.want) {
				t.Fatalf("Replace(%s, %q) = %v, want %v", tc.path, tc.value, err, tc.want)
			}
			if got := string(document.Bytes()); got != tc.src {
				t.Errorf("a refused Replace changed the source to %q", got)
			}
		})
	}
}
