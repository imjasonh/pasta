package a

import "errors"

func f(err error) bool {
	if errors.Is(err, nil) { // want `errors.Is\(x, nil\) can be simplified to x == nil`
		return true
	}
	if errors.Is(err, errSomething) { // OK: real sentinel comparison
		return true
	}
	return false
}

var errSomething = errors.New("x")
