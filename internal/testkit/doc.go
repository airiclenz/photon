// Package testkit is photon's test support: helpers that several packages' tests share. It is
// imported from _test.go files only and imports nothing from photon, so any package can use it
// without a cycle.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - docmap.go: [CheckDocMap], the structural test that a package's doc.go names every
//     non-test file beside it.
package testkit
