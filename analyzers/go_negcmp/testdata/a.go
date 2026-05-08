package a

func eq(a, b int) bool {
	if !(a == b) { // want "negated comparison can be simplified to .!=."
		return false
	}
	return true
}

func neq(a, b int) bool {
	if !(a != b) { // want "negated comparison can be simplified to .==."
		return false
	}
	return true
}

func compound(a, b, c int) bool {
	// Top-level operator is &&, not !; should not match.
	if a == b && c != 0 {
		return true
	}
	return false
}

func plain(a, b int) bool {
	// Already a plain comparison; should not match.
	if a == b {
		return true
	}
	return false
}

func mixedOps(a, b int) bool {
	x := !(a == b) // want "negated comparison can be simplified to .!=."
	y := !(a != b) // want "negated comparison can be simplified to .==."
	_ = x
	_ = y
	return false
}

// Comments inside the parens are preserved by the surgical rewrite.
func keepsComments(a, b int) bool {
	return !( /* leading */ a /* mid */ == b /* trailing */ ) // want "negated comparison can be simplified to .!=."
}
