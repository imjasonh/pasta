// Multi-file scenario: api.go and caller.go are analyzed as one
// group with a shared fact store, so `caller.go`'s call to
// `UsedAcrossFiles` populates a `called` fact whose by-name entry is
// visible while `find_unused_exports` walks `api.go`'s declarations.
//
// `UnusedAcrossFiles` has no caller in either file, so it fires.

package multipkg

func UsedAcrossFiles() {} // OK — called from caller.go

func UnusedAcrossFiles() {} // want `exported func UnusedAcrossFiles is never called`
