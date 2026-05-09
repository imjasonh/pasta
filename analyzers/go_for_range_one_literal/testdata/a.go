package a

func f() {
	for _, v := range []int{42} { // want "single-element slice literal"
		_ = v
	}

	for _, v := range []int{1, 2, 3} { // OK: > 1 element
		_ = v
	}

	xs := []int{42}
	for _, v := range xs { // OK: not a literal
		_ = v
	}

	for _, v := range []string{"hello"} { // want "single-element slice literal"
		_ = v
	}
}
