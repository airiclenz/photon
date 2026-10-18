package yamledit

import "gopkg.in/yaml.v3"

// Has reports whether path resolves in the document as it stands: every mapping key on it is
// present and every sequence index is in range. The empty path is the root, always present.
func (d *Document) Has(path Path) bool {
	_, err := d.locate(path)
	return err == nil
}

// Scalar returns the decoded value of the scalar at path — its quotes removed, a doubled single
// quote read as one and escapes resolved — and true. It returns "" and false when path does not
// resolve or names a mapping, a sequence or an alias.
func (d *Document) Scalar(path Path) (string, bool) {
	at, err := d.locate(path)
	if err != nil || at.node.Kind != yaml.ScalarNode {
		return "", false
	}
	return at.node.Value, true
}
