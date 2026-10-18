package yamledit

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// emptyFlowSequence is what a sequence reads as once [Document.DeleteItem] removes its only item.
const emptyFlowSequence = "[]"

// InsertItem adds value, YAML source text on one line, to the sequence at sequence so that it
// becomes the item at index, 0 through the sequence's length: 0 puts it first, the length last.
// A block sequence gets `- value` on its own line at its dashes' indent, just before the line of
// the dash now at index, or after the last item's last line, so comment and blank lines between
// items stay where they are. A flow sequence gets `value, ` before the item at index, `, value`
// after its last item, or value alone inside an empty `[]`. A path that does not resolve and an
// index out of range are [ErrNotFound]; a path naming a mapping or scalar is [ErrKind]; text
// spanning lines, an item whose dash does not start its line (the inner first item of
// `- - a`), and a result that does not parse are [ErrUnsupported]. Errors name the new item's
// path.
func (d *Document) InsertItem(sequence Path, index int, value string) error {
	target := itemPath(sequence, index)
	node, err := d.locateSequence(sequence, target)
	if err != nil {
		return err
	}
	if index < 0 || index > len(node.Content) {
		return fmt.Errorf("%s: %w", target, ErrNotFound)
	}
	if err := checkOneLine(value); err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}

	var edited []byte
	if isFlow(node) {
		edited, err = d.insertFlowItem(node, index, value)
	} else {
		line := strings.Repeat(" ", node.Column-1) + "- " + value + "\n"
		edited, err = d.insertBlockItem(node, index, line)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}
	return d.apply(target, edited, true)
}

// DeleteItem removes the item at index from the sequence at sequence: in a block sequence, its
// dash line through the last line of its content (block scalars and comment lines inside it
// included), leaving the comment and blank lines that trail it; in a flow sequence, the item and
// one separator beside it. Removing a sequence's only item leaves `[]` in its place, through
// [Document.Replace] and with its refusals (a block sequence at the document's root); whether the
// key goes too is the caller's call. A path that does not resolve and an index out of range are
// [ErrNotFound]; a path naming a mapping or scalar is [ErrKind]; an item whose dash does not
// start its line and a result that does not parse are [ErrUnsupported]. Errors name the item's
// path.
func (d *Document) DeleteItem(sequence Path, index int) error {
	target := itemPath(sequence, index)
	node, err := d.locateSequence(sequence, target)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(node.Content) {
		return fmt.Errorf("%s: %w", target, ErrNotFound)
	}
	if len(node.Content) == 1 {
		if err := d.Replace(sequence, emptyFlowSequence); err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
		return nil
	}

	edited, err := d.deleteItem(node, index)
	if err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}
	return d.apply(sequence, edited, true)
}

// MoveItem moves the item at from in the sequence at sequence so that it becomes the item at
// to, both 0 through the sequence's length less one; the items between shift by one, and from
// equal to to leaves the source as it is. The item's text travels verbatim — in a block
// sequence its dash line through its last line, block scalars and comment lines inside it
// included — and is cut as [Document.DeleteItem] cuts it and put back as [Document.InsertItem]
// places a new item, so the comment and blank lines between items stay where they are. A path
// that does not resolve and an index out of range are [ErrNotFound]; a path naming a mapping or
// scalar is [ErrKind]; an item whose dash does not start its line and a result that does not
// parse are [ErrUnsupported]. Errors name the item's path.
func (d *Document) MoveItem(sequence Path, from, to int) error {
	node, err := d.locateSequence(sequence, itemPath(sequence, from))
	if err != nil {
		return err
	}
	for _, index := range []int{from, to} {
		if index < 0 || index >= len(node.Content) {
			return fmt.Errorf("%s: %w", itemPath(sequence, index), ErrNotFound)
		}
	}
	if from == to {
		return nil
	}

	target := itemPath(sequence, to)
	edited, err := d.moveItem(sequence, node, from, to)
	if err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}
	return d.apply(target, edited, true)
}

// moveItem returns the source with the item at from of node, the sequence at sequence, cut out
// and spliced back in so that it is the item at to.
func (d *Document) moveItem(sequence Path, node *yaml.Node, from, to int) ([]byte, error) {
	start, end, err := d.itemSpan(node, from)
	if err != nil {
		return nil, err
	}
	text := string(d.src[start:end])

	cut, err := d.deleteItem(node, from)
	if err != nil {
		return nil, err
	}
	rest, err := Parse(cut)
	if err != nil {
		return nil, fmt.Errorf("cutting the item leaves invalid YAML (%v): %w", err, ErrUnsupported)
	}
	at, err := rest.locate(sequence)
	if err != nil {
		return nil, fmt.Errorf("cutting the item loses the sequence (%v): %w", err, ErrUnsupported)
	}
	if isFlow(at.node) {
		return rest.insertFlowItem(at.node, to, text)
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return rest.insertBlockItem(at.node, to, text)
}

// itemPath returns the path of the item at index of the sequence at sequence.
func itemPath(sequence Path, index int) Path {
	return append(append(Path{}, sequence...), strconv.Itoa(index))
}

// locateSequence resolves sequence, which must name a sequence; errors name target.
func (d *Document) locateSequence(sequence, target Path) (*yaml.Node, error) {
	at, err := d.locate(sequence)
	if err != nil {
		return nil, err
	}
	if at.node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: an item of a %s: %w", target, kindName(at.node.Kind), ErrKind)
	}
	return at.node, nil
}

// itemSpan returns the bytes the item at index of node occupies: in a block sequence its dash
// line through its last line, line break included; in a flow sequence its text alone.
func (d *Document) itemSpan(node *yaml.Node, index int) (int, int, error) {
	item := node.Content[index]
	if isFlow(node) {
		end, err := d.valueEnd(item)
		return d.offset(item.Line, item.Column), end, err
	}
	dash, err := d.dashLine(node, item)
	if err != nil {
		return 0, 0, err
	}
	return d.lineStarts[dash-1], d.itemEnd(dash, node.Column-1), nil
}

// dashLine returns the line of the dash that opens item in the block sequence node: item's own
// line, or, for an item below its dash, the nearest line above with a dash at the sequence's
// column. A dash with text before it on its line is [ErrUnsupported].
func (d *Document) dashLine(node, item *yaml.Node) (int, error) {
	for line := item.Line; line >= node.Line; line-- {
		at := d.offset(line, node.Column)
		if at >= len(d.src) || d.src[at] != '-' {
			continue
		}
		if len(bytes.TrimLeft(d.src[d.lineStarts[line-1]:at], " ")) != 0 {
			return 0, fmt.Errorf("edit an item whose dash does not start its line: %w",
				ErrUnsupported)
		}
		return line, nil
	}
	return 0, fmt.Errorf("find the item's dash: %w", ErrUnsupported)
}

// deleteItem returns the source without the item at index of node, a sequence of two items or
// more: a block item's lines, or a flow item with the separator after it, or, for the last, the
// one before it.
func (d *Document) deleteItem(node *yaml.Node, index int) ([]byte, error) {
	start, end, err := d.itemSpan(node, index)
	if err != nil {
		return nil, err
	}
	if !isFlow(node) {
		if end == len(d.src) && !bytes.HasSuffix(d.src, []byte("\n")) {
			start = withoutLineBreak(d.src, start)
		}
		return splice(d.src, start, end, ""), nil
	}
	if next := index + 1; next < len(node.Content) {
		item := node.Content[next]
		return splice(d.src, start, d.offset(item.Line, item.Column), ""), nil
	}
	previousEnd, err := d.valueEnd(node.Content[index-1])
	if err != nil {
		return nil, err
	}
	return splice(d.src, previousEnd, end, ""), nil
}

// insertBlockItem returns the source with lines, whole lines ending in a line break, in the
// block sequence node: before the dash line of the item at index, or, at the sequence's length,
// after the last item's last line.
func (d *Document) insertBlockItem(node *yaml.Node, index int, lines string) ([]byte, error) {
	if index < len(node.Content) {
		start, _, err := d.itemSpan(node, index)
		if err != nil {
			return nil, err
		}
		return splice(d.src, start, start, lines), nil
	}
	dash, err := d.dashLine(node, node.Content[len(node.Content)-1])
	if err != nil {
		return nil, err
	}
	at := d.itemEnd(dash, node.Column-1)
	if at == len(d.src) && !bytes.HasSuffix(d.src, []byte("\n")) {
		lines = "\n" + strings.TrimSuffix(lines, "\n")
	}
	return splice(d.src, at, at, lines), nil
}

// insertFlowItem returns the source with text in the flow sequence node as the item at index.
func (d *Document) insertFlowItem(node *yaml.Node, index int, text string) ([]byte, error) {
	if index < len(node.Content) {
		item := node.Content[index]
		start := d.offset(item.Line, item.Column)
		return splice(d.src, start, start, text+", "), nil
	}
	if len(node.Content) > 0 {
		end, err := d.valueEnd(node.Content[len(node.Content)-1])
		if err != nil {
			return nil, err
		}
		return splice(d.src, end, end, ", "+text), nil
	}
	open := d.offset(node.Line, node.Column)
	if open >= len(d.src) || d.src[open] != '[' {
		return nil, fmt.Errorf("find the empty flow sequence's bracket: %w", ErrUnsupported)
	}
	return splice(d.src, open+1, open+1, text), nil
}
