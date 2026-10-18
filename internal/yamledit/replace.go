package yamledit

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Replace puts value, YAML source text on one line, in place of the whole node at path — a plain,
// quoted or block scalar, or a flow or block mapping or sequence — whatever the old node's kind
// and the new text's: a mapping may become a scalar and a scalar a flow mapping. A scalar's or a
// flow collection's text is swapped where it stands. A block collection or block scalar must be
// a block mapping's value or a block sequence item. As a mapping's value, its key's line keeps
// the key and any comment on it, takes value after the colon, and loses the lines below it that
// the value held. As a sequence item, value takes its place after the dash and the item's lines
// go with it, but for the comment on a block scalar's header line, which stays. Either way,
// comment and blank lines trailing the value stay with what follows. An anchored, tagged or
// alias node, a plain scalar running over several lines, and a block collection or block scalar
// at the document's root are refused with [ErrUnsupported], as is a result that does not parse
// or no longer holds the path; a path that does not resolve is [ErrNotFound]. Errors name the
// path.
func (d *Document) Replace(path Path, value string) error {
	at, err := d.locate(path)
	if err != nil {
		return err
	}
	if err := checkOneLine(value); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	node := at.node
	if node.Kind == yaml.AliasNode || node.Anchor != "" || node.Style&yaml.TaggedStyle != 0 {
		return fmt.Errorf("%s: replace an anchored, tagged or alias node: %w", path, ErrUnsupported)
	}
	start, end, text, err := d.replaceSpan(at, value)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return d.apply(path, splice(d.src, start, end, text), true)
}

// replaceSpan returns the bytes the node at at occupies and the text that replaces them with
// value.
func (d *Document) replaceSpan(at located, value string) (int, int, string, error) {
	node := at.node
	isBlockScalar := node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0
	if node.Kind == yaml.ScalarNode && !isBlockScalar {
		start, end, err := d.scalarSpan(node, at.isInFlow)
		return start, end, value, err
	}
	if isFlow(node) {
		end, err := d.valueEnd(node)
		return d.offset(node.Line, node.Column), end, value, err
	}
	if at.sequence != nil {
		return d.blockItemSpan(at, value)
	}
	return d.blockValueSpan(at, value)
}

// blockItemSpan returns the span and text that put value in place of a block collection or a
// block scalar that is a block sequence item: from the node's start after its dash to the end of
// the item's last line, its line break kept, with any comment on a block scalar's header line
// carried over. A collection's first line holds its content, so nothing on it is carried.
func (d *Document) blockItemSpan(at located, value string) (int, int, string, error) {
	start := d.offset(at.node.Line, at.node.Column)
	text := value
	if at.node.Kind == yaml.ScalarNode {
		header := string(d.src[start:withoutLineBreak(d.src, d.lineEnd(at.node.Line))])
		if comment := lineComment(header); comment != "" {
			text += " " + comment
		}
	}
	end := d.itemEnd(at.node.Line, at.sequence.Column-1)
	return start, withoutLineBreak(d.src, end), text, nil
}

// blockValueSpan returns the span and text that put value in place of a block collection or a
// block scalar that is a block mapping's value: from just past its key's colon to the end of the
// entry's last line, its line break kept, with any comment on the key's line carried over.
func (d *Document) blockValueSpan(at located, value string) (int, int, string, error) {
	if at.parent == nil || isFlow(at.parent) {
		return 0, 0, "", fmt.Errorf("replace a block %s at the document's root: %w",
			kindName(at.node.Kind), ErrUnsupported)
	}
	colon, err := d.keyEnd(at.key)
	if err != nil {
		return 0, 0, "", err
	}
	for colon < len(d.src) && (d.src[colon] == ' ' || d.src[colon] == '\t') {
		colon++
	}
	if colon >= len(d.src) || d.src[colon] != ':' {
		return 0, 0, "", fmt.Errorf("find the colon after the key: %w", ErrUnsupported)
	}

	text := " " + value
	keyLine := string(d.src[colon+1 : withoutLineBreak(d.src, d.lineEnd(at.key.Line))])
	if comment := lineComment(keyLine); comment != "" {
		text += " " + comment
	}
	end := d.entryEnd(at.key, at.parent.Content[0].Column-1)
	return colon + 1, withoutLineBreak(d.src, end), text, nil
}

// keyEnd returns the offset just past a block mapping key's text: a quoted key to its closing
// quote, a plain one after its value, which a one-line plain scalar spells verbatim.
func (d *Document) keyEnd(key *yaml.Node) (int, error) {
	start := d.offset(key.Line, key.Column)
	if start < len(d.src) && (d.src[start] == '\'' || d.src[start] == '"') {
		end, isClosed := scanQuoted(d.src, start)
		if !isClosed {
			return 0, fmt.Errorf("find the key's closing quote: %w", ErrUnsupported)
		}
		return end, nil
	}
	end := start + len(key.Value)
	if end > len(d.src) || string(d.src[start:end]) != key.Value {
		return 0, fmt.Errorf("find the key's text: %w", ErrUnsupported)
	}
	return end, nil
}

// withoutLineBreak returns end moved back before the line break src holds just before it, if
// any.
func withoutLineBreak(src []byte, end int) int {
	if end > 0 && src[end-1] == '\n' {
		end--
	}
	if end > 0 && src[end-1] == '\r' {
		end--
	}
	return end
}

// lineComment returns the comment that text, the rest of a line after a key's colon, ends with,
// or "": a `#` at its start or after a blank opens one.
func lineComment(text string) string {
	for index := range len(text) {
		isAfterBlank := index == 0 || text[index-1] == ' ' || text[index-1] == '\t'
		if text[index] == '#' && isAfterBlank {
			return strings.TrimRight(text[index:], " \t")
		}
	}
	return ""
}
