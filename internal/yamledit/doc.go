// Package yamledit edits YAML source in place: it sets a scalar, inserts a key into a mapping,
// deletes a key, replaces a whole node with one line of YAML, upserts a key, and inserts,
// deletes and moves a sequence item, each addressed by a [Path] of mapping keys and sequence
// indexes, by splicing new text over exactly the span the edit changes. Every byte outside that
// span — comments, blank lines, quoting, key order, block or flow style — stays as written, which
// is what lets a tool save an edit into a file a person keeps by hand. It never re-encodes the
// node tree. It also answers queries on the source as it stands: whether a path is present, and
// what the scalar at a path decodes to.
//
// gopkg.in/yaml.v3 is the parser because its yaml.Node carries each node's line and column, the
// anchor every splice starts from; the end of a span is found by scanning the source, never
// from the decoded value, so quoting and escapes keep their written length.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - document.go: the [Document] and its [Path], [Parse], the Set, Insert and Delete edits, and
//     [Quote].
//   - query.go: the Has and Scalar queries: whether a path is present, a scalar's decoded value.
//   - replace.go: the Replace edit: a scalar, flow or block node swapped for one line of YAML.
//   - scan.go: positions and spans in the source: line and column to byte offset, a scalar's or
//     flow collection's extent, a block entry's lines.
//   - sequence.go: the InsertItem, DeleteItem and MoveItem edits: a block or flow sequence's
//     items added at an index, removed, and moved verbatim to another index.
//   - upsert.go: the Upsert and DeleteIfPresent edits: set, replace or insert a key whether or
//     not it is there, and delete a key that may be absent.
package yamledit
