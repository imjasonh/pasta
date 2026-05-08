package a

// OldFn does something.
//
// Deprecated: use NewFn instead.
func OldFn() {}

// NewFn does something better.
func NewFn() {}

// Deprecated: still deprecated even with no other doc.
func ReallyOld() {}

func caller() {
	OldFn()     // want `OldFn is deprecated`
	NewFn()     // OK
	ReallyOld() // want `ReallyOld is deprecated`
}
