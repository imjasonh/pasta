package loader

import (
	"testing"
	"testing/fstest"
)

func TestLoadRoundTrip(t *testing.T) {
	src := `package sample

import "github.com/imjasonh/pasta/schema"

sample: schema.#Analyzer & {
	name:    "sample"
	version: "0.1.0"
	doc:     "test analyzer"
	facts: {}
	rules: simplify_nil_eq: {
		name:      "simplify_nil_eq"
		doc:       "Rewrite x == nil to !x in if-conditions"
		languages: ["go"]
		requires:  []
		provides:  []

		match: {
			node: "if_statement"
			fields: condition: {
				node: "binary_expression"
				fields: {
					left:     {capture: "expr"}
					operator: {node: "=="}
					right:    {node: "nil"}
				}
			}
		}

		diagnose: {
			message:  "comparison to nil can be simplified"
			severity: "hint"
		}

		rewrite: edits: [{
			target:      "condition"
			replacement: "!@expr"
		}]
	}
}
`
	fsys := fstest.MapFS{
		"sample.cue": &fstest.MapFile{Data: []byte(src)},
	}
	a, err := LoadFS(fsys, "sample.cue")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if a.Name != "sample" {
		t.Errorf("Name = %q, want sample", a.Name)
	}
	if a.Version != "0.1.0" {
		t.Errorf("Version = %q, want 0.1.0", a.Version)
	}
	if got := len(a.Rules); got != 1 {
		t.Fatalf("Rules count = %d, want 1", got)
	}
	r, ok := a.Rules["simplify_nil_eq"]
	if !ok {
		t.Fatalf("rule simplify_nil_eq missing; got %v", a.Rules)
	}
	if r.Name != "simplify_nil_eq" {
		t.Errorf("rule name = %q, want simplify_nil_eq", r.Name)
	}
	if got, want := r.Match.Node, []string{"if_statement"}; !equal(got, want) {
		t.Errorf("Match.Node = %v, want %v", got, want)
	}
	cond, ok := r.Match.Fields["condition"]
	if !ok {
		t.Fatalf("condition field missing")
	}
	cp := cond.AsPattern()
	if cp == nil {
		t.Fatalf("condition has no inline pattern")
	}
	if got, want := cp.Node, []string{"binary_expression"}; !equal(got, want) {
		t.Errorf("condition.node = %v, want %v", got, want)
	}
	if r.Diagnose == nil || r.Diagnose.Message == "" {
		t.Errorf("diagnose missing")
	}
	if r.Rewrite == nil || len(r.Rewrite.Edits) != 1 {
		t.Fatalf("rewrite missing or wrong shape")
	}
	e := r.Rewrite.Edits[0]
	if e.Target != "condition" || e.Replacement != "!@expr" {
		t.Errorf("edit = %+v, want target=condition replacement=!@expr", e)
	}
}

func TestLoadNodeUnion(t *testing.T) {
	src := `package sample

import "github.com/imjasonh/pasta/schema"

sample: schema.#Analyzer & {
	name: "sample"
	version: "0.1.0"
	facts: {}
	rules: r: {
		name: "r"
		doc: "x"
		requires: []
		provides: []
		languages: ["go"]
		match: node: ["statement_list", "expression_case", "default_case"]
	}
}
`
	fsys := fstest.MapFS{"a.cue": &fstest.MapFile{Data: []byte(src)}}
	a, err := LoadFS(fsys, "a.cue")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := a.Rules["r"]
	want := []string{"statement_list", "expression_case", "default_case"}
	if !equal(r.Match.Node, want) {
		t.Errorf("Match.Node = %v, want %v", r.Match.Node, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
