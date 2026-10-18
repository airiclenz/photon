package yamledit

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

var (
	// ErrNotFound reports a path naming a key or index the document does not hold.
	ErrNotFound = errors.New("no such node")
	// ErrKind reports a node of the wrong kind for the edit: a mapping where Set wants a scalar,
	// a scalar where Insert wants a mapping, a sequence item where Delete wants a mapping key.
	ErrKind = errors.New("wrong node kind")
	// ErrExists reports an Insert of a key the mapping already holds.
	ErrExists = errors.New("key already present")
	// ErrUnsupported reports a source form the splicer does not edit: a block or multi-line
	// plain scalar, an anchored or tagged scalar, a value spanning lines, or an edit whose result
	// no longer parses or no longer holds the edited node.
	ErrUnsupported = errors.New("unsupported YAML form")
)

// rootName is how a Path with no steps reads in an error.
const rootName = "(root)"

// Path addresses a node from the document's root: each step is a mapping key, or, where the
// node it steps into is a sequence, a decimal index into it. Path{"beats", "0", "duration"} is
// the first beat's duration.
type Path []string

// String renders the path as errors name it: keys joined by dots, indexes in brackets, as in
// beats[0].duration.
func (p Path) String() string {
	var out strings.Builder
	for _, step := range p {
		if isIndex(step) {
			out.WriteString("[" + step + "]")
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('.')
		}
		out.WriteString(step)
	}
	if out.Len() == 0 {
		return rootName
	}
	return out.String()
}

// isIndex reports whether step is spelled as a sequence index: decimal digits only.
func isIndex(step string) bool {
	if step == "" {
		return false
	}
	for _, r := range step {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Document is YAML source held for editing. Each edit splices new text over exactly the span it
// changes, so every byte outside that span — comments, blank lines, quoting, key order, flow or
// block style — is kept as written; the node tree is never re-encoded. After a splice the source
// is parsed again, so positions stay true for the next edit, and the edit is refused, leaving
// the document as it was, when the result does not parse or no longer holds what it edited.
// A Document is not safe for concurrent use.
type Document struct {
	src        []byte
	root       *yaml.Node
	lineStarts []int
}

// Parse holds src for editing. It fails when src is not YAML or holds no document; of a stream
// of several documents only the first is addressable. Parse keeps its own copy of src.
func Parse(src []byte) (*Document, error) {
	var file yaml.Node
	if err := yaml.Unmarshal(src, &file); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	if file.Kind != yaml.DocumentNode || len(file.Content) == 0 {
		return nil, errors.New("parse YAML: the source holds no document")
	}
	owned := bytes.Clone(src)
	return &Document{src: owned, root: file.Content[0], lineStarts: lineStarts(owned)}, nil
}

// Bytes returns a copy of the source as it stands after every edit so far.
func (d *Document) Bytes() []byte {
	return bytes.Clone(d.src)
}

// Set replaces the source text of the scalar at path with value, YAML source text on one line
// (a plain `1850ms`, or a string through [Quote]). The scalar may be plain or quoted, in a block
// or a flow mapping; a block scalar (`|`, `>`), a plain scalar running over several lines and an
// anchored or tagged scalar are refused with [ErrUnsupported]. A path that does not resolve is
// [ErrNotFound]; one naming a mapping or sequence is [ErrKind]. Errors name the path.
func (d *Document) Set(path Path, value string) error {
	at, err := d.locate(path)
	if err != nil {
		return err
	}
	if at.node.Kind != yaml.ScalarNode {
		return fmt.Errorf("%s: set a %s: %w", path, kindName(at.node.Kind), ErrKind)
	}
	if err := checkOneLine(value); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	start, end, err := d.scalarSpan(at.node, at.isInFlow)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return d.apply(path, splice(d.src, start, end, value), true)
}

// Insert adds `key: value` to the mapping at mapping, where value is YAML source text on one
// line. after names the keys that belong before key; the new entry goes right after whichever
// of them comes last in the mapping, or, when none is present, first. A block mapping gets its
// own line at the mapping's indent; a block mapping that is a sequence item (`- id: 1`) with no
// after key present gets it after its first key, so the dash keeps its key. A flow mapping gets
// `, key: value` (or `key: value, ` as its first entry). A key already present is [ErrExists];
// a path that does not name a mapping is [ErrNotFound] or [ErrKind]. Errors name the new key's
// path.
func (d *Document) Insert(mapping Path, key, value string, after []string) error {
	target := append(append(Path{}, mapping...), key)
	at, err := d.locate(mapping)
	if err != nil {
		return err
	}
	if at.node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: insert into a %s: %w", target, kindName(at.node.Kind), ErrKind)
	}
	if keyIndex(at.node, key) >= 0 {
		return fmt.Errorf("%s: %w", target, ErrExists)
	}
	for _, text := range []string{key, value} {
		if err := checkOneLine(text); err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
	}

	anchor := lastPresent(at.node, after)
	var edited []byte
	if isFlow(at.node) {
		edited, err = d.insertFlow(at.node, anchor, key+": "+value)
	} else {
		edited, err = d.insertBlock(at.node, anchor, key+": "+value)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}
	return d.apply(target, edited, true)
}

// Delete removes the mapping key at path with its value: in a block mapping, the key's line and
// every line of its value, leaving the comments and blank lines around them; in a flow mapping,
// the entry and one separator beside it. The first key of a block sequence item (`- id: 1`) is
// removed up to the next key, so the dash moves onto it; that key as the item's only key is
// [ErrUnsupported]. A path ending in a sequence index, or the empty path, is [ErrKind]. Errors
// name the path.
func (d *Document) Delete(path Path) error {
	at, err := d.locate(path)
	if err != nil {
		return err
	}
	if at.parent == nil {
		return fmt.Errorf("%s: delete what is not a mapping key: %w", path, ErrKind)
	}
	var start, end int
	if isFlow(at.parent) {
		start, end, err = d.flowEntrySpan(at)
	} else {
		start, end, err = d.blockEntrySpan(at)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return d.apply(path, splice(d.src, start, end, ""), false)
}

// Quote spells s as a YAML string that reads back as exactly s: single-quoted with each `'`
// doubled, or, when s holds a line break or another unprintable rune that single quotes cannot
// carry, double-quoted with escapes.
func Quote(s string) string {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return strconv.Quote(s)
		}
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// located is a resolved path: the node, and when it is a mapping value, its mapping and key, or
// when it is a sequence item, its sequence.
type located struct {
	node     *yaml.Node
	parent   *yaml.Node
	key      *yaml.Node
	keyIndex int
	sequence *yaml.Node
	isInFlow bool
}

// locate walks path from the root. Its errors name the whole path and wrap [ErrNotFound] or
// [ErrKind].
func (d *Document) locate(path Path) (located, error) {
	at := located{node: d.root, isInFlow: isFlow(d.root)}
	for _, step := range path {
		next, err := child(at.node, step)
		if err != nil {
			return located{}, fmt.Errorf("%s: %w", path, err)
		}
		next.isInFlow = at.isInFlow || isFlow(next.node)
		at = next
	}
	return at, nil
}

// child takes one step from node: a key of a mapping, an index of a sequence.
func child(node *yaml.Node, step string) (located, error) {
	switch node.Kind {
	case yaml.MappingNode:
		index := keyIndex(node, step)
		if index < 0 {
			return located{}, fmt.Errorf("key %q: %w", step, ErrNotFound)
		}
		return located{
			node: node.Content[index+1], parent: node, key: node.Content[index], keyIndex: index,
		}, nil
	case yaml.SequenceNode:
		index, err := strconv.Atoi(step)
		if err != nil || index < 0 || index >= len(node.Content) {
			return located{}, fmt.Errorf("index %q: %w", step, ErrNotFound)
		}
		return located{node: node.Content[index], sequence: node}, nil
	default:
		return located{}, fmt.Errorf("step %q into a %s: %w", step, kindName(node.Kind), ErrKind)
	}
}

// keyIndex returns the index in mapping.Content of the key named key, or -1.
func keyIndex(mapping *yaml.Node, key string) int {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return index
		}
	}
	return -1
}

// lastPresent returns the index in mapping.Content of the key, among names, that comes last in
// the mapping, or -1 when none of them is present.
func lastPresent(mapping *yaml.Node, names []string) int {
	found := -1
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		for _, name := range names {
			if mapping.Content[index].Value == name {
				found = index
			}
		}
	}
	return found
}

// apply parses edited and, when the result still holds path (or, for a delete, no longer
// does), makes it the document. Otherwise the document is left as it was.
func (d *Document) apply(path Path, edited []byte, isPresentWanted bool) error {
	next, err := Parse(edited)
	if err != nil {
		return fmt.Errorf("%s: the edit leaves invalid YAML (%v): %w", path, err, ErrUnsupported)
	}
	_, err = next.locate(path)
	if isPresentWanted && err != nil {
		return fmt.Errorf("%s: the edit loses the node (%v): %w", path, err, ErrUnsupported)
	}
	if !isPresentWanted && err == nil {
		return fmt.Errorf("%s: the edit leaves the key in place: %w", path, ErrUnsupported)
	}
	*d = *next
	return nil
}

// insertBlock returns the source with entry on its own line in the block mapping, after the
// entry whose key is at anchor in mapping.Content (-1 for first).
func (d *Document) insertBlock(mapping *yaml.Node, anchor int, entry string) ([]byte, error) {
	first := mapping.Content[0]
	line := strings.Repeat(" ", first.Column-1) + entry + "\n"
	if anchor < 0 {
		if d.startsLine(first) {
			at := d.lineStarts[first.Line-1]
			return splice(d.src, at, at, line), nil
		}
		anchor = 0
	}
	at := d.entryEnd(mapping.Content[anchor], first.Column-1)
	if at == len(d.src) && !bytes.HasSuffix(d.src, []byte("\n")) {
		line = "\n" + strings.TrimSuffix(line, "\n")
	}
	return splice(d.src, at, at, line), nil
}

// insertFlow returns the source with entry in the flow mapping, after the entry whose key is at
// anchor in mapping.Content (-1 for first).
func (d *Document) insertFlow(mapping *yaml.Node, anchor int, entry string) ([]byte, error) {
	if anchor >= 0 {
		end, err := d.valueEnd(mapping.Content[anchor+1])
		if err != nil {
			return nil, err
		}
		return splice(d.src, end, end, ", "+entry), nil
	}
	if len(mapping.Content) > 0 {
		start := d.offset(mapping.Content[0].Line, mapping.Content[0].Column)
		return splice(d.src, start, start, entry+", "), nil
	}
	open := d.offset(mapping.Line, mapping.Column)
	if open >= len(d.src) || d.src[open] != '{' {
		return nil, fmt.Errorf("find the empty flow mapping's brace: %w", ErrUnsupported)
	}
	return splice(d.src, open+1, open+1, entry), nil
}

// blockEntrySpan returns the bytes a block mapping entry occupies: its key's line through the
// last line of its value, or, for the first key of a sequence item, the key up to the next key.
func (d *Document) blockEntrySpan(at located) (int, int, error) {
	if d.startsLine(at.key) {
		indent := at.parent.Content[0].Column - 1
		return d.lineStarts[at.key.Line-1], d.entryEnd(at.key, indent), nil
	}
	nextKey := at.keyIndex + 2
	if nextKey >= len(at.parent.Content) {
		return 0, 0, fmt.Errorf("delete a sequence item's only key: %w", ErrUnsupported)
	}
	next := at.parent.Content[nextKey]
	return d.offset(at.key.Line, at.key.Column), d.offset(next.Line, next.Column), nil
}

// flowEntrySpan returns the bytes a flow mapping entry occupies with one separator: up to the
// next entry's key, or back to the previous entry's value when it is the last.
func (d *Document) flowEntrySpan(at located) (int, int, error) {
	start := d.offset(at.key.Line, at.key.Column)
	end, err := d.valueEnd(at.node)
	if err != nil {
		return 0, 0, err
	}
	if next := at.keyIndex + 2; next < len(at.parent.Content) {
		nextKey := at.parent.Content[next]
		return start, d.offset(nextKey.Line, nextKey.Column), nil
	}
	if at.keyIndex == 0 {
		return start, end, nil
	}
	previousEnd, err := d.valueEnd(at.parent.Content[at.keyIndex-1])
	if err != nil {
		return 0, 0, err
	}
	return previousEnd, end, nil
}

// checkOneLine refuses text that spans lines: the splicer writes one-line source only.
func checkOneLine(text string) error {
	if strings.ContainsAny(text, "\r\n") {
		return fmt.Errorf("source text %q spans lines: %w", text, ErrUnsupported)
	}
	return nil
}

// splice returns a copy of src with src[start:end] replaced by text.
func splice(src []byte, start, end int, text string) []byte {
	out := make([]byte, 0, len(src)-(end-start)+len(text))
	out = append(out, src[:start]...)
	out = append(out, text...)
	return append(out, src[end:]...)
}

// isFlow reports whether node is a flow mapping or sequence (`{…}`, `[…]`).
func isFlow(node *yaml.Node) bool {
	isCollection := node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode
	return isCollection && node.Style&yaml.FlowStyle != 0
}

// kindName names a node kind in an error.
func kindName(kind yaml.Kind) string {
	switch kind {
	case yaml.ScalarNode:
		return "scalar"
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.AliasNode:
		return "alias"
	default:
		return "document"
	}
}
