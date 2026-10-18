package yamledit

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// lineStarts returns the byte offset each line of src starts at.
func lineStarts(src []byte) []int {
	starts := []int{0}
	for index, char := range src {
		if char == '\n' {
			starts = append(starts, index+1)
		}
	}
	return starts
}

// offset turns a yaml.v3 position, 1-based line and column with the column counted in runes,
// into a byte offset into the source.
func (d *Document) offset(line, column int) int {
	if line < 1 || line > len(d.lineStarts) {
		return len(d.src)
	}
	at := d.lineStarts[line-1]
	for count := 1; count < column && at < len(d.src) && d.src[at] != '\n'; count++ {
		_, size := utf8.DecodeRune(d.src[at:])
		at += size
	}
	return at
}

// lineEnd returns the offset just past line's line break: the start of the next line.
func (d *Document) lineEnd(line int) int {
	if line < len(d.lineStarts) {
		return d.lineStarts[line]
	}
	return len(d.src)
}

// startsLine reports whether node is the first thing on its line, after indentation.
func (d *Document) startsLine(node *yaml.Node) bool {
	start := d.lineStarts[node.Line-1]
	lead := d.src[start:d.offset(node.Line, node.Column)]
	return len(bytes.TrimLeft(lead, " ")) == 0
}

// entryEnd returns the offset just past the last line of the block mapping entry whose key is
// key, in a mapping indented by indent: the key's line, then every following line indented
// deeper, or at the same indent and opening an indentless sequence item. Blank and comment lines
// count only when a line of the entry follows them, so trailing ones stay with what comes next.
func (d *Document) entryEnd(key *yaml.Node, indent int) int {
	return d.blockEnd(key.Line, indent, true)
}

// itemEnd returns the offset just past the last line of the block sequence item that starts on
// line, in a sequence whose dashes are indented by indent: that line, then every following line
// indented deeper, with blank and comment lines counted as [Document.entryEnd] counts them.
func (d *Document) itemEnd(line, indent int) int {
	return d.blockEnd(line, indent, false)
}

// blockEnd returns the offset just past the last line of the block that starts on first: every
// following line indented deeper than indent, and, when isDashKept, one at indent that opens a
// sequence item. Blank and comment lines count only when a line of the block follows them.
func (d *Document) blockEnd(first, indent int, isDashKept bool) int {
	end := d.lineEnd(first)
	for line := first + 1; line <= len(d.lineStarts); line++ {
		text := strings.TrimRight(string(d.src[d.lineStarts[line-1]:d.lineEnd(line)]), "\r\n")
		content := strings.TrimLeft(text, " ")
		if strings.TrimSpace(content) == "" || content[0] == '#' {
			continue
		}
		lead := len(text) - len(content)
		isDash := content == "-" || strings.HasPrefix(content, "- ")
		if lead < indent || (lead == indent && !(isDashKept && isDash)) {
			break
		}
		end = d.lineEnd(line)
	}
	return end
}

// scalarSpan returns the bytes the scalar's source text occupies, found by scanning the source
// from its position: a quoted scalar to its closing quote, a plain one to the end of its line,
// a comment, or — inside a flow collection — the next `,`, `}` or `]`.
func (d *Document) scalarSpan(node *yaml.Node, isInFlow bool) (int, int, error) {
	start := d.offset(node.Line, node.Column)
	if start >= len(d.src) {
		return 0, 0, fmt.Errorf("find the scalar's text: %w", ErrUnsupported)
	}
	switch d.src[start] {
	case '&', '!':
		return 0, 0, fmt.Errorf("edit an anchored or tagged scalar: %w", ErrUnsupported)
	case '|', '>':
		return 0, 0, fmt.Errorf("edit a block scalar: %w", ErrUnsupported)
	case '\'', '"':
		end, isClosed := scanQuoted(d.src, start)
		if !isClosed {
			return 0, 0, fmt.Errorf("find the scalar's closing quote: %w", ErrUnsupported)
		}
		return start, end, nil
	}
	end := scanPlain(d.src, start, isInFlow)
	if string(d.src[start:end]) != node.Value {
		return 0, 0, fmt.Errorf("edit a plain scalar spanning lines: %w", ErrUnsupported)
	}
	return start, end, nil
}

// valueEnd returns the offset just past a value inside a flow collection: a scalar's text, or
// a nested flow collection's closing bracket.
func (d *Document) valueEnd(node *yaml.Node) (int, error) {
	if node.Kind == yaml.ScalarNode {
		_, end, err := d.scalarSpan(node, true)
		return end, err
	}
	start := d.offset(node.Line, node.Column)
	if !isFlow(node) || start >= len(d.src) {
		return 0, fmt.Errorf("find the end of a %s: %w", kindName(node.Kind), ErrUnsupported)
	}
	end, isClosed := scanFlowCollection(d.src, start)
	if !isClosed {
		return 0, fmt.Errorf("find the flow collection's closing bracket: %w", ErrUnsupported)
	}
	return end, nil
}

// scanQuoted returns the offset just past the quoted scalar opening at start: single quotes
// escape a quote by doubling it, double quotes by a backslash.
func scanQuoted(src []byte, start int) (int, bool) {
	quote := src[start]
	for index := start + 1; index < len(src); index++ {
		switch {
		case quote == '"' && src[index] == '\\':
			index++
		case src[index] != quote:
		case quote == '\'' && index+1 < len(src) && src[index+1] == '\'':
			index++
		default:
			return index + 1, true
		}
	}
	return 0, false
}

// scanPlain returns the offset just past the plain scalar starting at start, trailing blanks
// excluded.
func scanPlain(src []byte, start int, isInFlow bool) int {
	end := start
	for index := start; index < len(src); index++ {
		char := src[index]
		isAfterBlank := index > start && (src[index-1] == ' ' || src[index-1] == '\t')
		isFlowIndicator := char == ',' || char == '}' || char == ']'
		isComment := char == '#' && isAfterBlank
		if char == '\n' || char == '\r' || isComment || (isInFlow && isFlowIndicator) {
			break
		}
		if char != ' ' && char != '\t' {
			end = index + 1
		}
	}
	return end
}

// scanFlowCollection returns the offset just past the bracket closing the flow collection that
// opens at start, skipping quoted scalars and comments. A quote opens a scalar only where a
// scalar can start, so the apostrophe in a plain `it's` is not one.
func scanFlowCollection(src []byte, start int) (int, bool) {
	depth := 0
	previous := byte(' ')
	for index := start; index < len(src); index++ {
		char := src[index]
		switch {
		case (char == '\'' || char == '"') && strings.IndexByte("{[,:? \t\n", previous) >= 0:
			end, isClosed := scanQuoted(src, index)
			if !isClosed {
				return 0, false
			}
			index = end - 1
		case char == '#' && strings.IndexByte(" \t\n", previous) >= 0:
			for index < len(src) && src[index] != '\n' {
				index++
			}
		case char == '{' || char == '[':
			depth++
		case char == '}' || char == ']':
			depth--
			if depth == 0 {
				return index + 1, true
			}
		}
		if index < len(src) {
			previous = src[index]
		}
	}
	return 0, false
}
