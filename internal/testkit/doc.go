// Package testkit is photon's test support: helpers that several packages' tests share. It is
// imported from _test.go files only and imports nothing from photon, so any package can use it
// without a cycle.
//
// The file map:
//   - build.go: [BuildFixtureTUI], which builds the fixture TUI (package fixturetui beside this
//     one) once per test binary and returns its path.
//   - doc.go: this package comment and file map.
//   - docmap.go: [CheckDocMap], the structural test that a package's doc.go names every
//     non-test file beside it.
//   - eventually.go: [Eventually], which polls a condition until it holds or a deadline passes.
package testkit
