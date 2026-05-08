package tsutil

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestSmokeParseGo(t *testing.T) {
	src := []byte(`package p

func f() error {
	err := foo()
	if err != nil {
		return err
	}
	return nil
}
`)

	parser := gts.NewParser(grammars.GoLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root.Type(grammars.GoLanguage()) != "source_file" {
		t.Fatalf("root type = %q, want source_file", root.Type(grammars.GoLanguage()))
	}
	if root.HasError() {
		t.Fatalf("parse produced error nodes: %s", root.SExpr(grammars.GoLanguage()))
	}
}
