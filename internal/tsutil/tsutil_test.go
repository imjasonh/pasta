package tsutil

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseGo(t *testing.T, src string) (*gts.Tree, Node) {
	t.Helper()
	tree, root, err := Parse(t.Context(), grammars.GoLanguage(), []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if root.HasError() {
		t.Fatalf("parse error in source: %s", root.String())
	}
	return tree, root
}

func TestFieldLookup(t *testing.T) {
	tree, root := parseGo(t, `package p
func f() {
	if err != nil {
		return
	}
}
`)
	defer tree.Release()

	var ifStmt Node
	Walk(root, func(n Node) bool {
		if n.Type() == "if_statement" {
			ifStmt = n
			return false
		}
		return true
	})
	if !ifStmt.IsValid() {
		t.Fatal("no if_statement found")
	}

	cond := ifStmt.ChildByFieldName("condition")
	if !cond.IsValid() {
		t.Fatal("if_statement has no condition field")
	}
	if cond.Type() != "binary_expression" {
		t.Errorf("condition.Type = %q, want binary_expression", cond.Type())
	}

	if ifStmt.HasFieldName("initializer") {
		t.Error("expected initializer field to be absent")
	}
}
