package yamledit

import (
	"errors"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

// Upsert writes `key: value` into the mapping at mapping, where value is YAML source text on one
// line, whether or not the key is there yet. A present scalar is rewritten by [Document.Set],
// with its refusals: a block scalar, a plain scalar over several lines, an anchored or tagged
// scalar. A present mapping or sequence is swapped whole by [Document.Replace]. An absent key is
// added by [Document.Insert] after the present keys that come before it in order, the mapping's
// full key order; a key order does not name goes after the mapping's last key. A mapping path
// that does not resolve is [ErrNotFound]; one naming a sequence or a scalar is [ErrKind]. Errors
// name the key's path.
func (d *Document) Upsert(mapping Path, key, value string, order []string) error {
	target := append(append(Path{}, mapping...), key)
	at, err := d.locate(target)
	if errors.Is(err, ErrNotFound) {
		return d.Insert(mapping, key, value, d.keysBefore(mapping, key, order))
	}
	if err != nil {
		return err
	}
	if at.parent == nil {
		return fmt.Errorf("%s: upsert into a %s: %w", target, kindName(at.sequence.Kind), ErrKind)
	}
	if at.node.Kind == yaml.ScalarNode {
		return d.Set(target, value)
	}
	return d.Replace(target, value)
}

// DeleteIfPresent removes the mapping key at path as [Document.Delete] does, and does nothing,
// returning nil, when the key or any mapping or sequence item on the way to it is absent. Every
// other error of Delete stands: a path stepping into a scalar is [ErrKind], as is one ending in
// a sequence index.
func (d *Document) DeleteIfPresent(path Path) error {
	_, err := d.locate(path)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return d.Delete(path)
}

// keysBefore returns the keys an absent key goes after: those order lists before it, or, when
// order does not name it, every key the mapping holds, so it goes last. It returns nil when
// mapping names no mapping, leaving Insert to report why.
func (d *Document) keysBefore(mapping Path, key string, order []string) []string {
	if index := slices.Index(order, key); index >= 0 {
		return order[:index]
	}
	at, err := d.locate(mapping)
	if err != nil || at.node.Kind != yaml.MappingNode {
		return nil
	}
	keys := make([]string, 0, len(at.node.Content)/2)
	for index := 0; index+1 < len(at.node.Content); index += 2 {
		keys = append(keys, at.node.Content[index].Value)
	}
	return keys
}
