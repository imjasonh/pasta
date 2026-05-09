package a

func f(x string) string {
	a := "" + x // want "concatenating with an empty string"
	b := x + "" // want "concatenating with an empty string"
	c := x + "y"
	d := "y" + x
	e := "" + ""
	return a + b + c + d + e
}
