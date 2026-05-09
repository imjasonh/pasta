package a

func f() {
	panic("") // want `panic("") gives no information`
}

func g() {
	panic("something went wrong") // OK
}

func h(err error) {
	panic(err) // OK: forwarding err
}
