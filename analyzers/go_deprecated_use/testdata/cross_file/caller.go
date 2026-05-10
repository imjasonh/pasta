package crossfile

func use() {
	OldExported() // want `OldExported is deprecated`
	NewExported() // OK
}
