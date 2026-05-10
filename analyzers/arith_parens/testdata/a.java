class A {
	static int slice(int endp, int begin, int consume) {
		return endp - begin + consume; // want "parenthesize arithmetic sub-expression"
	}

	static int mulAdd(int a, int b, int c) {
		return a + b * c; // want "parenthesize arithmetic sub-expression"
	}

	static int explicit(int a, int b, int c) {
		return (a - b) + c;
	}
}
