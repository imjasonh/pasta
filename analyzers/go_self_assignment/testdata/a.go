package a

func f(x, y int) int {
	x = x // want `self-assignment x = x is a no-op`
	x = y // OK
	y = x // OK
	return x + y
}

func g(x int) int {
	x = x + 1 // OK: not self-assignment
	return x
}
