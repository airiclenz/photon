package yamledit

import (
	"errors"
	"strings"
	"testing"
)

// The block list every block case edits, one fragment per part: a comment opening the list, a
// one-line item with a comment, a multi-line block mapping with a `|` scalar and a comment
// inside it, a `|` scalar item, a last one-line item, and the lines after the list.
const (
	blockHead  = "list:\n  # the list opens\n"
	blockOne   = "  - one # first\n"
	blockTwo   = "  - name: two\n    text: |\n      line one\n\n      line two\n    # inside two\n    at: 1s\n"
	blockThree = "  - |\n    three\n"
	blockFour  = "  - four\n"
	blockTail  = "\n# after the list\nnext: 1\n"
	blockList  = blockHead + blockOne + blockTwo + blockThree + blockFour + blockTail
	blockNew   = "  - new\n"
)

// The flow list every flow case edits: a plain item, a flow mapping holding a quoted comma, a
// nested flow sequence and a last plain item, with a comment after the list.
const flowList = "list: [one, {x: 'a, b'}, [3, 4], four] # c\nnext: 1\n"

// The nested lists: a `via:` inside a camera move that is a list item, in block and flow form.
const (
	nestedBlock = "camera:\n  - pan:\n      via:\n        - {cell: {x: 1, y: 2}} # start\n" +
		"        - {text: 'Größe'}\n      over: 600ms\n  - zoom: 2\n"
	nestedFlow = "camera:\n  - pan: {via: [{cell: {x: 1, y: 2}}, 'a, b'], over: 600ms}\n  - zoom: 2\n"
)

// listPath and viaPath address the lists the cases edit.
var (
	listPath = Path{"list"}
	viaPath  = Path{"camera", "0", "pan", "via"}
)

// flow returns flowList with its items spelled as items.
func flow(items string) string {
	return "list: [" + items + "] # c\nnext: 1\n"
}

func TestSequenceEdits_KeepEveryOtherByte(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
		edit edit
		want string
	}{
		{
			"block insert at 0 goes below the list's opening comment", blockList,
			func(d *Document) error { return d.InsertItem(listPath, 0, "new") },
			blockHead + blockNew + blockOne + blockTwo + blockThree + blockFour + blockTail,
		},
		{
			"block insert in the middle", blockList,
			func(d *Document) error { return d.InsertItem(listPath, 2, "new") },
			blockHead + blockOne + blockTwo + blockNew + blockThree + blockFour + blockTail,
		},
		{
			"block insert at the end leaves the trailing lines", blockList,
			func(d *Document) error { return d.InsertItem(listPath, 4, "new") },
			blockHead + blockOne + blockTwo + blockThree + blockFour + blockNew + blockTail,
		},
		{
			"block delete the first", blockList,
			func(d *Document) error { return d.DeleteItem(listPath, 0) },
			blockHead + blockTwo + blockThree + blockFour + blockTail,
		},
		{
			"block delete a multi-line mapping in the middle", blockList,
			func(d *Document) error { return d.DeleteItem(listPath, 1) },
			blockHead + blockOne + blockThree + blockFour + blockTail,
		},
		{
			"block delete a | scalar", blockList,
			func(d *Document) error { return d.DeleteItem(listPath, 2) },
			blockHead + blockOne + blockTwo + blockFour + blockTail,
		},
		{
			"block delete the last", blockList,
			func(d *Document) error { return d.DeleteItem(listPath, 3) },
			blockHead + blockOne + blockTwo + blockThree + blockTail,
		},
		{
			"block delete the only item leaves []", "a:\n  - x # gone\nb: 1\n",
			func(d *Document) error { return d.DeleteItem(Path{"a"}, 0) },
			"a: []\nb: 1\n",
		},
		{
			"block move a | scalar up", blockList,
			func(d *Document) error { return d.MoveItem(listPath, 2, 0) },
			blockHead + blockThree + blockOne + blockTwo + blockFour + blockTail,
		},
		{
			"block move a multi-line mapping down to the end", blockList,
			func(d *Document) error { return d.MoveItem(listPath, 1, 3) },
			blockHead + blockOne + blockThree + blockFour + blockTwo + blockTail,
		},
		{
			"block move down by one", blockList,
			func(d *Document) error { return d.MoveItem(listPath, 0, 1) },
			blockHead + blockTwo + blockOne + blockThree + blockFour + blockTail,
		},
		{
			"block move to the same index", blockList,
			func(d *Document) error { return d.MoveItem(listPath, 1, 1) },
			blockList,
		},
		{
			"block comment lines between items stay where they stand", "l:\n  - a\n  # between\n  - b\n",
			func(d *Document) error { return d.MoveItem(Path{"l"}, 1, 0) },
			"l:\n  - b\n  - a\n  # between\n",
		},
		{
			"flow insert at 0", flowList,
			func(d *Document) error { return d.InsertItem(listPath, 0, "new") },
			flow("new, one, {x: 'a, b'}, [3, 4], four"),
		},
		{
			"flow insert in the middle", flowList,
			func(d *Document) error { return d.InsertItem(listPath, 2, "new") },
			flow("one, {x: 'a, b'}, new, [3, 4], four"),
		},
		{
			"flow insert at the end", flowList,
			func(d *Document) error { return d.InsertItem(listPath, 4, "{y: 1}") },
			flow("one, {x: 'a, b'}, [3, 4], four, {y: 1}"),
		},
		{
			"flow insert into an empty list", "list: [] # c\nnext: 1\n",
			func(d *Document) error { return d.InsertItem(listPath, 0, "new") },
			flow("new"),
		},
		{
			"flow delete the first", flowList,
			func(d *Document) error { return d.DeleteItem(listPath, 0) },
			flow("{x: 'a, b'}, [3, 4], four"),
		},
		{
			"flow delete in the middle", flowList,
			func(d *Document) error { return d.DeleteItem(listPath, 1) },
			flow("one, [3, 4], four"),
		},
		{
			"flow delete the last", flowList,
			func(d *Document) error { return d.DeleteItem(listPath, 3) },
			flow("one, {x: 'a, b'}, [3, 4]"),
		},
		{
			"flow delete the only item leaves []", flow("x"),
			func(d *Document) error { return d.DeleteItem(listPath, 0) },
			flow(""),
		},
		{
			"flow move up", flowList,
			func(d *Document) error { return d.MoveItem(listPath, 3, 0) },
			flow("four, one, {x: 'a, b'}, [3, 4]"),
		},
		{
			"flow move down", flowList,
			func(d *Document) error { return d.MoveItem(listPath, 0, 2) },
			flow("{x: 'a, b'}, [3, 4], one, four"),
		},
		{
			"flow move to the end", flowList,
			func(d *Document) error { return d.MoveItem(listPath, 1, 3) },
			flow("one, [3, 4], four, {x: 'a, b'}"),
		},
		{
			"flow move to the same index", flowList,
			func(d *Document) error { return d.MoveItem(listPath, 2, 2) },
			flowList,
		},
		{
			"nested block via insert", nestedBlock,
			func(d *Document) error { return d.InsertItem(viaPath, 1, "{cell: {x: 3, y: 4}}") },
			swap(t, nestedBlock, "        - {text:", "        - {cell: {x: 3, y: 4}}\n        - {text:"),
		},
		{
			"nested block via delete", nestedBlock,
			func(d *Document) error { return d.DeleteItem(viaPath, 0) },
			swap(t, nestedBlock, "        - {cell: {x: 1, y: 2}} # start\n", ""),
		},
		{
			"nested block via move", nestedBlock,
			func(d *Document) error { return d.MoveItem(viaPath, 0, 1) },
			swap(t, nestedBlock,
				"        - {cell: {x: 1, y: 2}} # start\n        - {text: 'Größe'}\n",
				"        - {text: 'Größe'}\n        - {cell: {x: 1, y: 2}} # start\n"),
		},
		{
			"nested flow via insert", nestedFlow,
			func(d *Document) error { return d.InsertItem(viaPath, 2, "{text: x}") },
			swap(t, nestedFlow, "'a, b']", "'a, b', {text: x}]"),
		},
		{
			"nested flow via delete", nestedFlow,
			func(d *Document) error { return d.DeleteItem(viaPath, 1) },
			swap(t, nestedFlow, ", 'a, b']", "]"),
		},
		{
			"nested flow via move", nestedFlow,
			func(d *Document) error { return d.MoveItem(viaPath, 1, 0) },
			swap(t, nestedFlow, "[{cell: {x: 1, y: 2}}, 'a, b']", "['a, b', {cell: {x: 1, y: 2}}]"),
		},
		{
			"a root sequence", "- a\n- b\n",
			func(d *Document) error { return d.InsertItem(nil, 2, "c") },
			"- a\n- b\n- c\n",
		},
		{
			"an indentless sequence insert", "a:\n- 1\n- 2\nb: 3\n",
			func(d *Document) error { return d.InsertItem(Path{"a"}, 1, "x") },
			"a:\n- 1\n- x\n- 2\nb: 3\n",
		},
		{
			"an indentless sequence delete", "a:\n- 1\n- x: 1\n  y: 2\nb: 3\n",
			func(d *Document) error { return d.DeleteItem(Path{"a"}, 1) },
			"a:\n- 1\nb: 3\n",
		},
		{
			"an item below its dash moves whole", "a:\n  -\n    x: 1\n  - 2\n",
			func(d *Document) error { return d.MoveItem(Path{"a"}, 0, 1) },
			"a:\n  - 2\n  -\n    x: 1\n",
		},
		{
			"insert at the end without a trailing newline", "a:\n  - 1\n  - 2",
			func(d *Document) error { return d.InsertItem(Path{"a"}, 2, "3") },
			"a:\n  - 1\n  - 2\n  - 3",
		},
		{
			"delete the last without a trailing newline", "a:\n  - 1\n  - 2",
			func(d *Document) error { return d.DeleteItem(Path{"a"}, 1) },
			"a:\n  - 1",
		},
		{
			"move the last up without a trailing newline", "a:\n  - 1\n  - 2",
			func(d *Document) error { return d.MoveItem(Path{"a"}, 1, 0) },
			"a:\n  - 2\n  - 1",
		},
		{
			"move to the end without a trailing newline", "a:\n  - 1\n  - 2",
			func(d *Document) error { return d.MoveItem(Path{"a"}, 0, 1) },
			"a:\n  - 2\n  - 1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := parse(t, tc.src)

			err := tc.edit(document)

			if err != nil {
				t.Fatalf("edit: %v", err)
			}
			if got := string(document.Bytes()); got != tc.want {
				t.Errorf("edit =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestSequenceEdits_Refuse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		src      string
		edit     edit
		want     error
		wantPath string
	}{
		{"insert past the end", blockList,
			func(d *Document) error { return d.InsertItem(listPath, 5, "x") }, ErrNotFound, "list[5]"},
		{"insert before the start", flowList,
			func(d *Document) error { return d.InsertItem(listPath, -1, "x") }, ErrNotFound, "-1"},
		{"insert into a missing sequence", blockList,
			func(d *Document) error { return d.InsertItem(Path{"nope"}, 0, "x") }, ErrNotFound, "nope"},
		{"delete past the end", blockList,
			func(d *Document) error { return d.DeleteItem(listPath, 4) }, ErrNotFound, "list[4]"},
		{"move from past the end", flowList,
			func(d *Document) error { return d.MoveItem(listPath, 4, 0) }, ErrNotFound, "list[4]"},
		{"move to past the end", blockList,
			func(d *Document) error { return d.MoveItem(listPath, 0, 4) }, ErrNotFound, "list[4]"},
		{"insert into a mapping", blockList,
			func(d *Document) error { return d.InsertItem(nil, 0, "x") }, ErrKind, "[0]"},
		{"delete from a scalar", blockList,
			func(d *Document) error { return d.DeleteItem(Path{"next"}, 0) }, ErrKind, "next[0]"},
		{"insert text spanning lines", blockList,
			func(d *Document) error { return d.InsertItem(listPath, 0, "a\nb") }, ErrUnsupported, "list[0]"},
		{"insert text breaking the YAML", flowList,
			func(d *Document) error { return d.InsertItem(listPath, 0, "{") }, ErrUnsupported, "list[0]"},
		{"insert before a dash with text before it", "- - a\n  - b\n",
			func(d *Document) error { return d.InsertItem(Path{"0"}, 0, "x") }, ErrUnsupported, "[0][0]"},
		{"delete the only item of a root block sequence", "- a\n",
			func(d *Document) error { return d.DeleteItem(nil, 0) }, ErrUnsupported, "[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := parse(t, tc.src)

			err := tc.edit(document)

			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("error %q does not name the path %s", err, tc.wantPath)
			}
			if got := string(document.Bytes()); got != tc.src {
				t.Errorf("a refused edit changed the source:\n%s", got)
			}
		})
	}
}
