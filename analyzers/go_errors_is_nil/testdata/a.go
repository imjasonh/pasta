package a

import "errors"

func f(err error) bool {
	// want:+1 `errors.Is\(x, nil\) can be simplified to x == nil`
	if errors.Is(err, nil) {
		return true
	}
	if errors.Is(err, errSomething) { // OK: real sentinel comparison
		return true
	}
	return false
}

var errSomething = errors.New("x")
