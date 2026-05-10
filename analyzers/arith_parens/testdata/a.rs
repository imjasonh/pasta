fn slice(endp: i32, begin: i32, consume: i32) -> i32 {
	endp - begin + consume // want "parenthesize arithmetic sub-expression"
}

fn mul_add(a: i32, b: i32, c: i32) -> i32 {
	a + b * c // want "parenthesize arithmetic sub-expression"
}

fn explicit(a: i32, b: i32, c: i32) -> i32 {
	(a - b) + c
}
