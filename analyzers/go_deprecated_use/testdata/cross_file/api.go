// Multi-file test case: the deprecated declaration lives in one file,
// the callers live in another. The two are analyzed as a single group
// with a shared fact store, so the by-name fact index propagates the
// `deprecated` tag from `OldExported` (this file) to its call sites
// (caller.go).

package crossfile

// Deprecated: use NewExported.
func OldExported() {}

func NewExported() {}
