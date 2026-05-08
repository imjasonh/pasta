package match

import (
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/lang"
	"github.com/imjasonh/pasta/pkg/tsutil"
)

func parseGo(t *testing.T, src string) tsutil.Node {
	t.Helper()
	_, root, err := tsutil.Parse(t.Context(), grammars.GoLanguage(), []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if root.HasError() {
		t.Fatalf("parse error in source: %s", root.String())
	}
	return root
}

func goEnv() *Env {
	return &Env{
		StmtList:   lang.All["go"].StmtList,
		Predicates: DefaultRegistry(),
	}
}

func TestMatchSimpleField(t *testing.T) {
	root := parseGo(t, `package p
func f() {
	if err != nil {
		return
	}
}
`)

	pat := &dsl.Pattern{
		Node: []string{"if_statement"},
		Fields: map[string]dsl.Child{
			"condition": {
				Pattern: &dsl.Pattern{
					Node: []string{"binary_expression"},
					Fields: map[string]dsl.Child{
						"left": {Capture: "expr"},
					},
				},
			},
		},
	}
	matches := FindAll(pat, root, goEnv())
	if len(matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(matches))
	}
	expr, ok := matches[0].Captures["expr"]
	if !ok {
		t.Fatalf("expr capture missing")
	}
	if expr.Text() != "err" {
		t.Errorf("expr text = %q, want %q", expr.Text(), "err")
	}
}

func TestAbsentFields(t *testing.T) {
	root := parseGo(t, `package p
func f() {
	if err := foo(); err != nil {
		return
	}
}
func foo() error { return nil }
`)
	// Pattern requires no initializer field — should NOT match.
	pat := &dsl.Pattern{
		Node:         []string{"if_statement"},
		AbsentFields: []string{"initializer"},
	}
	matches := FindAll(pat, root, goEnv())
	if len(matches) != 0 {
		t.Fatalf("len(matches) = %d, want 0", len(matches))
	}
}

func TestAbsentFieldsAccepts(t *testing.T) {
	root := parseGo(t, `package p
func f() {
	if true {
		return
	}
}
`)
	pat := &dsl.Pattern{
		Node:         []string{"if_statement"},
		AbsentFields: []string{"initializer"},
	}
	matches := FindAll(pat, root, goEnv())
	if len(matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(matches))
	}
}

func TestAdjacentBasic(t *testing.T) {
	root := parseGo(t, `package p
func f() error {
	err := foo()
	if err != nil {
		return err
	}
	return nil
}
func foo() error { return nil }
`)
	// Match a (short_var_declaration, if_statement) pair anywhere in
	// a block / case / select.
	pat := &dsl.Pattern{
		Node: []string{"statement_list"},
		Adjacent: []dsl.Child{
			{
				Capture: "assign",
				Pattern: &dsl.Pattern{Node: []string{"short_var_declaration"}},
			},
			{
				Capture: "if_stmt",
				Pattern: &dsl.Pattern{Node: []string{"if_statement"}},
			},
		},
	}
	matches := FindAll(pat, root, goEnv())
	if len(matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(matches))
	}
	m := matches[0]
	if got := m.Captures["assign"].Type(); got != "short_var_declaration" {
		t.Errorf("assign.Type = %q", got)
	}
	if got := m.Captures["if_stmt"].Type(); got != "if_statement" {
		t.Errorf("if_stmt.Type = %q", got)
	}
}

func TestNilComparisonPredicate(t *testing.T) {
	// err != nil where err is the last non-blank in [_, err]
	root := parseGo(t, `package p
func f() error {
	_, err := bar()
	if err != nil {
		return err
	}
	return nil
}
func bar() (int, error) { return 0, nil }
`)
	pat := &dsl.Pattern{
		Node: []string{"statement_list"},
		Adjacent: []dsl.Child{
			{
				Capture: "assign",
				Pattern: &dsl.Pattern{
					Node: []string{"short_var_declaration"},
					Fields: map[string]dsl.Child{
						"left": {Capture: "lhs_list"},
					},
				},
			},
			{
				Capture: "if_stmt",
				Pattern: &dsl.Pattern{
					Node: []string{"if_statement"},
					Fields: map[string]dsl.Child{
						"condition": {
							Capture: "cond",
							Pattern: &dsl.Pattern{
								Node: []string{"binary_expression"},
								Fields: map[string]dsl.Child{
									"left":     {Capture: "cond_x"},
									"operator": {Capture: "cond_op"},
									"right":    {Capture: "cond_y"},
								},
								Where: []dsl.Predicate{
									{Op: "matches", Args: []dsl.Arg{{Str: "@cond_op"}, {Str: "==|!="}}},
									{Op: "nil_comparison", Args: []dsl.Arg{{Str: "@cond_x"}, {Str: "@cond_y"}, {Str: "@lhs_list"}}},
								},
							},
						},
					},
				},
			},
		},
	}
	matches := FindAll(pat, root, goEnv())
	if len(matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(matches))
	}
}

func TestNilComparisonReversed(t *testing.T) {
	root := parseGo(t, `package p
func f() error {
	err := foo()
	if nil != err {
		return err
	}
	return nil
}
func foo() error { return nil }
`)
	pat := iferrConditionPattern()
	matches := FindAll(pat, root, goEnv())
	if len(matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(matches))
	}
}

func TestNilComparisonNotNil(t *testing.T) {
	// notNilComparison: err != other (not nil)
	root := parseGo(t, `package p
func f() error {
	err := foo()
	other := errOther
	if err != other {
		return err
	}
	return nil
}
var errOther = error(nil)
func foo() error { return nil }
`)
	pat := iferrConditionPattern()
	matches := FindAll(pat, root, goEnv())
	if len(matches) != 0 {
		t.Fatalf("len(matches) = %d, want 0", len(matches))
	}
}

// TestAdjacentOptionalQuantifier verifies that an adjacent element
// with quantifier: "?" can match either 0 or 1 statements via the
// matcher's backtracking implementation.
func TestAdjacentOptionalQuantifier(t *testing.T) {
	pat := &dsl.Pattern{
		Node: []string{"statement_list"},
		Adjacent: []dsl.Child{
			{
				Capture:    "decl",
				Quantifier: "?",
				Pattern:    &dsl.Pattern{Node: []string{"var_declaration"}},
			},
			{
				Capture: "assign",
				Pattern: &dsl.Pattern{Node: []string{"assignment_statement"}},
			},
		},
	}

	// Case 1: var_decl is present.
	root := parseGo(t, `package p
func f() {
	var err error
	err = foo()
}
func foo() error { return nil }
`)
	matches := FindAll(pat, root, goEnv())
	if len(matches) != 1 {
		t.Fatalf("with-decl: len(matches) = %d, want 1", len(matches))
	}
	if got := matches[0].Captures["decl"].Type(); got != "var_declaration" {
		t.Errorf("with-decl: decl capture type = %q, want var_declaration", got)
	}

	// Case 2: var_decl is missing — should still match because of `?`.
	root = parseGo(t, `package p
import "errors"
var sentinel = errors.New("x")
func f() {
	err = foo()
	_ = err
}
var err error
func foo() error { return nil }
`)
	matches = FindAll(pat, root, goEnv())
	if len(matches) != 1 {
		t.Fatalf("without-decl: len(matches) = %d, want 1", len(matches))
	}
	if matches[0].Captures["decl"].IsValid() {
		t.Errorf("without-decl: decl capture should be unbound, got %s",
			matches[0].Captures["decl"].Type())
	}
	if got := matches[0].Captures["assign"].Type(); got != "assignment_statement" {
		t.Errorf("without-decl: assign capture type = %q, want assignment_statement", got)
	}
}

// TestIndexedFindMatchesWalkResults verifies that FindAll returns the
// same set of matches whether or not env.Index is set — the index is
// strictly an optimization.
func TestIndexedFindMatchesWalkResults(t *testing.T) {
	root := parseGo(t, `package p
func a() {}
func b() {}
func c() {}
type T struct {
	x int
}
`)
	pat := &dsl.Pattern{Node: []string{"function_declaration"}}

	envWithIndex := &Env{
		StmtList:   lang.All["go"].StmtList,
		Predicates: DefaultRegistry(),
		Index:      BuildIndex(root),
	}
	withIdx := FindAll(pat, root, envWithIndex)

	envNoIndex := &Env{
		StmtList:   lang.All["go"].StmtList,
		Predicates: DefaultRegistry(),
	}
	withoutIdx := FindAll(pat, root, envNoIndex)

	if len(withIdx) != len(withoutIdx) {
		t.Fatalf("indexed count = %d, walk count = %d (must match)", len(withIdx), len(withoutIdx))
	}
	if len(withIdx) != 3 {
		t.Errorf("expected 3 function_declarations, got %d", len(withIdx))
	}
}

func TestNonConsecutive(t *testing.T) {
	root := parseGo(t, `package p
import "fmt"
func f() error {
	err := foo()
	fmt.Println("hi")
	if err != nil {
		return err
	}
	return nil
}
func foo() error { return nil }
`)
	pat := iferrConditionPattern()
	matches := FindAll(pat, root, goEnv())
	if len(matches) != 0 {
		t.Fatalf("len(matches) = %d, want 0 (non-consecutive)", len(matches))
	}
}

// iferrConditionPattern is a partial iferr inline_define pattern: structural
// only, no preconditions. Used by several tests above.
func iferrConditionPattern() *dsl.Pattern {
	return &dsl.Pattern{
		Node: []string{"statement_list"},
		Adjacent: []dsl.Child{
			{
				Capture: "assign",
				Pattern: &dsl.Pattern{
					Node: []string{"short_var_declaration"},
					Fields: map[string]dsl.Child{
						"left":  {Capture: "lhs_list"},
						"right": {Capture: "rhs"},
					},
				},
			},
			{
				Capture: "if_stmt",
				Pattern: &dsl.Pattern{
					Node:         []string{"if_statement"},
					AbsentFields: []string{"initializer"},
					Fields: map[string]dsl.Child{
						"condition": {
							Capture: "cond",
							Pattern: &dsl.Pattern{
								Node: []string{"binary_expression"},
								Fields: map[string]dsl.Child{
									"left":     {Capture: "cond_x"},
									"operator": {Capture: "cond_op"},
									"right":    {Capture: "cond_y"},
								},
								Where: []dsl.Predicate{
									{Op: "matches", Args: []dsl.Arg{{Str: "@cond_op"}, {Str: "==|!="}}},
									{Op: "nil_comparison", Args: []dsl.Arg{{Str: "@cond_x"}, {Str: "@cond_y"}, {Str: "@lhs_list"}}},
								},
							},
						},
					},
				},
			},
		},
	}
}
