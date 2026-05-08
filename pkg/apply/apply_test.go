package apply

import (
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/effect"
	"github.com/imjasonh/pasta/pkg/lang"
	"github.com/imjasonh/pasta/pkg/match"
	"github.com/imjasonh/pasta/pkg/tsutil"
)

// TestNilcheckEndToEnd runs the simplify-nil-eq example from plan.md §4.1
// against a tiny Go file: x == nil --> !x.
func TestNilcheckEndToEnd(t *testing.T) {
	src := []byte(`package p
func f(x *int) bool {
	if x == nil {
		return false
	}
	return true
}
`)
	tree, root, err := tsutil.Parse(t.Context(), grammars.GoLanguage(), src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	pat := &dsl.Pattern{
		Node: []string{"if_statement"},
		Fields: map[string]dsl.Child{
			"condition": {
				Capture: "cond",
				Pattern: &dsl.Pattern{
					Node: []string{"binary_expression"},
					Fields: map[string]dsl.Child{
						"left":     {Capture: "expr"},
						"operator": {Capture: "op"},
						"right":    {Node: []string{"nil"}},
					},
					Where: []dsl.Predicate{
						{Op: "token_eq", Args: []dsl.Arg{{Str: "@op"}, {Str: "=="}}},
					},
				},
			},
		},
	}

	env := &match.Env{
		StmtList:   lang.All["go"].StmtList,
		Predicates: match.DefaultRegistry(),
	}
	matches := match.FindAll(pat, root, env)
	if len(matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(matches))
	}
	caps := matches[0].Captures

	rw := &dsl.Rewrite{
		Edits: []dsl.Edit{
			{Target: "cond", Replacement: "!@expr"},
		},
	}
	ops, err := effect.BuildOps("simplify_nil_eq", rw, effect.BuildContext{
		Captures: caps,
		Root:     root,
	})
	if err != nil {
		t.Fatalf("BuildOps: %v", err)
	}

	got, err := Apply(src, ops, dsl.RewriteOpts{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := `package p
func f(x *int) bool {
	if !x {
		return false
	}
	return true
}
`
	if string(got) != want {
		t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
