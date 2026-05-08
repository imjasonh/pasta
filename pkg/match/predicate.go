package match

import (
	"regexp"
	"strings"

	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/tsutil"
)

// PredicateFunc evaluates a `where` predicate. Args are positional.
type PredicateFunc func(args []string, env *Env, caps Captures) bool

// CheckFunc evaluates a `pre_conditions` check. Args are named.
type CheckFunc func(args map[string]string, env *Env, caps Captures) bool

// PredicateRegistry holds both `where` predicates (positional) and
// `pre_conditions` checks (named).
type PredicateRegistry struct {
	Predicates map[string]PredicateFunc
	Checks     map[string]CheckFunc
}

// Eval dispatches a `where` predicate. Unknown ops fail closed.
func (r PredicateRegistry) Eval(p dsl.Predicate, env *Env, caps Captures) bool {
	fn, ok := r.Predicates[p.Op]
	if !ok {
		return false
	}
	return fn(p.Args, env, caps)
}

// EvalCheck dispatches a `pre_conditions` check by name.
func (r PredicateRegistry) EvalCheck(name string, args map[string]string, env *Env, caps Captures) bool {
	fn, ok := r.Checks[name]
	if !ok {
		return false
	}
	return fn(args, env, caps)
}

// DefaultRegistry returns the predicates + checks shipped with pasta.
//
// Predicates (positional `where`-form): structural and small-scale —
// equality, regex, nil_comparison, last_non_blank, etc.
//
// Checks (named `pre_conditions`-form): cross-cutting and parameterized
// over grammar specifics — `all_children_type`,
// `siblings_after_no_ident`, `ancestor_field_subtree_no_ident`,
// `decl_matches_idents`. The rule supplies grammar-specific arguments
// (function-like node types, var-spec node type, etc.), so these checks
// are language-agnostic.
func DefaultRegistry() PredicateRegistry {
	return PredicateRegistry{
		Predicates: map[string]PredicateFunc{
			"eq":               predEq,
			"neq":              predNeq,
			"matches":          predMatches,
			"not_matches":      predNotMatches,
			"token_eq":         predTokenEq,
			"nil_comparison":   predNilComparison,
			"last_non_blank":   predLastNonBlank,
			"same_ident":       predSameIdent,
			"empty":               predEmptyNamedChildren,
			"named_child_count":   predNamedChildCount,
			"prev_sibling_matches": predPrevSiblingMatches,
			"has_fact":         predHasFact,
			"not_has_fact":     predNotHasFact,
			// Stubs (not implemented; referenced in plan.md so schema
			// validation succeeds).
			"ancestor_is":      predStubFalse,
			"type_is":          predStubFalse,
			"field_absent":     predStubFalse,
			"all_match":        predStubFalse,
			"stmt_index_delta": predStubFalse,
		},
		Checks: map[string]CheckFunc{
			"all_children_type":               checkAllChildrenType,
			"siblings_after_no_ident":         checkSiblingsAfterNoIdent,
			"ancestor_field_subtree_no_ident": checkAncestorFieldSubtreeNoIdent,
			"decl_matches_idents":             checkDeclMatchesIdents,
		},
	}
}

// resolveCapture extracts a node by reference. ref may be "@name" (lookup
// in caps), or a "|"-separated list of "@name" alternatives — the first
// bound one wins. Non-@ values fail.
func resolveCapture(ref string, caps Captures) (tsutil.Node, bool) {
	for _, alt := range strings.Split(ref, "|") {
		alt = strings.TrimSpace(alt)
		if !strings.HasPrefix(alt, "@") {
			continue
		}
		if n, ok := caps[alt[1:]]; ok {
			return n, true
		}
	}
	return tsutil.Node{}, false
}

// ============================================================================
// Where predicates (positional args)
// ============================================================================

func predEq(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return false
	}
	return n.Text() == args[1]
}

func predNeq(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return false
	}
	return n.Text() != args[1]
}

func predMatches(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return false
	}
	re, err := regexp.Compile(args[1])
	if err != nil {
		return false
	}
	return re.MatchString(n.Text())
}

func predNotMatches(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return false
	}
	re, err := regexp.Compile(args[1])
	if err != nil {
		return false
	}
	return !re.MatchString(n.Text())
}

func predTokenEq(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return false
	}
	return strings.TrimSpace(n.Text()) == args[1]
}

func predSameIdent(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	a, ok1 := resolveCapture(args[0], caps)
	b, ok2 := resolveCapture(args[1], caps)
	if !ok1 || !ok2 {
		return false
	}
	return a.Text() == b.Text()
}

// predNilComparison: [@x, @y, @list].
// One of @x or @y is the literal nil node; the other is an identifier
// whose text equals the last non-blank identifier in @list.
func predNilComparison(args []string, env *Env, caps Captures) bool {
	if len(args) != 3 {
		return false
	}
	x, ok1 := resolveCapture(args[0], caps)
	y, ok2 := resolveCapture(args[1], caps)
	list, ok3 := resolveCapture(args[2], caps)
	if !ok1 || !ok2 || !ok3 {
		return false
	}
	xIsNil := x.Type() == "nil"
	yIsNil := y.Type() == "nil"
	if xIsNil == yIsNil {
		return false
	}
	target := lastNonBlankIdentText(list)
	if target == "" {
		return false
	}
	var other tsutil.Node
	if xIsNil {
		other = y
	} else {
		other = x
	}
	if other.Type() != "identifier" {
		return false
	}
	return other.Text() == target
}

func predLastNonBlank(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	cap0, ok1 := resolveCapture(args[0], caps)
	list, ok2 := resolveCapture(args[1], caps)
	if !ok1 || !ok2 {
		return false
	}
	t := lastNonBlankIdentText(list)
	if t == "" {
		return false
	}
	return cap0.Text() == t
}

func lastNonBlankIdentText(list tsutil.Node) string {
	if !list.IsValid() {
		return ""
	}
	kids := list.NamedChildren()
	for i := len(kids) - 1; i >= 0; i-- {
		k := kids[i]
		if k.Type() == "identifier" && k.Text() != "_" {
			return k.Text()
		}
	}
	return ""
}

func predStubTrue(args []string, env *Env, caps Captures) bool  { return true }
func predStubFalse(args []string, env *Env, caps Captures) bool { return false }

// predHasFact: [@cap, "kind"] — true if the captured node has a fact
// of the given kind in the env's FactStore.
func predHasFact(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	if env.FactStore == nil {
		return false
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return false
	}
	return env.FactStore.Has(n, args[1])
}

// predNotHasFact is the inverse of predHasFact.
func predNotHasFact(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return true
	}
	if env.FactStore == nil {
		return true
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return true
	}
	return !env.FactStore.Has(n, args[1])
}

// predEmptyNamedChildren: [@cap] — true if @cap has zero named children.
// Useful for matching empty blocks, empty argument lists, etc.
func predEmptyNamedChildren(args []string, env *Env, caps Captures) bool {
	if len(args) != 1 {
		return false
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return false
	}
	return len(n.NamedChildren()) == 0
}

// predPrevSiblingMatches: [@cap, "regex"] — true if the named child of
// @cap's parent immediately preceding @cap exists AND its source text
// matches the given regex.
//
// Unlike `adjacent` (which iterates the language's StmtList provider
// and skips comments), this predicate walks ALL named children — so it
// can find docstring-style comments or attribute markers that
// immediately precede a declaration.
func predPrevSiblingMatches(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return false
	}
	parent := n.Parent()
	if !parent.IsValid() {
		return false
	}
	siblings := parent.NamedChildren()
	prev := tsutil.Node{}
	for _, s := range siblings {
		if s.StartByte() == n.StartByte() && s.EndByte() == n.EndByte() {
			break
		}
		prev = s
	}
	if !prev.IsValid() {
		return false
	}
	re, err := regexp.Compile(args[1])
	if err != nil {
		return false
	}
	return re.MatchString(prev.Text())
}

// predNamedChildCount: [@cap, "N"] — true if @cap has exactly N named
// children.
func predNamedChildCount(args []string, env *Env, caps Captures) bool {
	if len(args) != 2 {
		return false
	}
	n, ok := resolveCapture(args[0], caps)
	if !ok {
		return false
	}
	want := 0
	for _, c := range args[1] {
		if c < '0' || c > '9' {
			return false
		}
		want = want*10 + int(c-'0')
	}
	return len(n.NamedChildren()) == want
}

// ============================================================================
// Pre-condition checks (named args)
// ============================================================================

// checkAllChildrenType: every named child of @capture has the given type.
//
// Args:
//
//	cap   — capture ref (@name).
//	type  — expected node type for every named child.
//
// Used by iferr's all_idents_lhs to verify the LHS is all plain
// identifiers (no selectors, indices, etc.).
func checkAllChildrenType(args map[string]string, env *Env, caps Captures) bool {
	cap0, ok := resolveCapture(args["cap"], caps)
	if !ok {
		return false
	}
	want := args["type"]
	if want == "" {
		return false
	}
	for _, c := range cap0.NamedChildren() {
		if c.Type() != want {
			return false
		}
	}
	return true
}

// checkSiblingsAfterNoIdent: in the named children of @anchor's parent
// that come AFTER @anchor, no descendant is an identifier whose text
// matches a non-blank identifier text in @source's named children.
//
// Args:
//
//	anchor — capture ref. The container whose later siblings are scanned
//	         is anchor's parent.
//	source — capture ref. Provides the set of identifier texts to check
//	         against; "_" is excluded.
//
// Used by iferr's not_used_after with scope=block.
func checkSiblingsAfterNoIdent(args map[string]string, env *Env, caps Captures) bool {
	anchor, ok := resolveCapture(args["anchor"], caps)
	if !ok {
		return false
	}
	src, ok := resolveCapture(args["source"], caps)
	if !ok {
		return false
	}
	names := nonBlankIdentTexts(src, false)
	if len(names) == 0 {
		return true
	}
	parent := anchor.Parent()
	if !parent.IsValid() {
		return true
	}
	afterEnd := anchor.EndByte()
	for _, sib := range parent.NamedChildren() {
		if sib.StartByte() < afterEnd {
			continue
		}
		if subtreeReferences(sib, names, 0, 0) {
			return false
		}
	}
	return true
}

// checkAncestorFieldSubtreeNoIdent: walk up from @anchor; find the first
// ancestor whose type is in `ancestor_types` (a "|"-separated list);
// navigate to the named field given by `field`; verify no descendant
// identifier in that subtree has text matching a non-blank identifier
// text in @source's named children (or any descendant's text — see
// `source_recursive`). Optionally exclude a byte range.
//
// Args:
//
//	anchor          — capture ref.
//	ancestor_types  — "|"-separated tree-sitter types (e.g.
//	                  "function_declaration|method_declaration|func_literal").
//	field           — named field of the ancestor (e.g. "body" or "result").
//	source          — capture ref providing the names to look for.
//	exclude_start?  — capture ref (or fallback list); excludes nodes whose
//	                  byte range falls within [exclude_start.Start,
//	                  exclude_end.End).
//	exclude_end?    — capture ref (or fallback list).
//
// Used by iferr's not_used_after (function scope) and not_named_result.
func checkAncestorFieldSubtreeNoIdent(args map[string]string, env *Env, caps Captures) bool {
	anchor, ok := resolveCapture(args["anchor"], caps)
	if !ok {
		return false
	}
	types := splitOr(args["ancestor_types"])
	if len(types) == 0 {
		return false
	}
	field := args["field"]
	src, ok := resolveCapture(args["source"], caps)
	if !ok {
		return false
	}

	var exStart, exEnd uint32
	hasExcl := false
	if s, sok := resolveCapture(args["exclude_start"], caps); sok {
		if e, eok := resolveCapture(args["exclude_end"], caps); eok {
			exStart = s.StartByte()
			exEnd = e.EndByte()
			hasExcl = true
		}
	}

	// Walk up from anchor's parent.
	for cur := anchor.Parent(); cur.IsValid(); cur = cur.Parent() {
		if !typeIn(types, cur.Type()) {
			continue
		}
		var target tsutil.Node
		if field == "" {
			target = cur
		} else {
			target = cur.ChildByFieldName(field)
		}
		if !target.IsValid() {
			return true
		}
		names := nonBlankIdentTexts(src, false)
		if len(names) == 0 {
			return true
		}
		if hasExcl {
			return !subtreeReferences(target, names, exStart, exEnd)
		}
		return !subtreeReferences(target, names, 0, 0)
	}
	return true
}

// checkDeclMatchesIdents: @decl is structurally a declaration node (e.g.
// var_declaration in Go) whose named children of `spec_type` each lack a
// `value_field`, and whose set of identifier-descendant texts (filtered
// to non-blank) equals the same set extracted from @source.
//
// Args:
//
//	decl        — capture ref (the candidate declaration).
//	source      — capture ref (the LHS list whose names we want to match).
//	spec_type   — the inner-spec node type (e.g. "var_spec").
//	value_field — the field on a spec that, if present, indicates an
//	              initializer value (e.g. "value").
//
// Used by iferr's matching_var_decl. Marked optional in the rule so an
// unbound @decl (no preceding declaration captured) doesn't fail.
func checkDeclMatchesIdents(args map[string]string, env *Env, caps Captures) bool {
	decl, ok := resolveCapture(args["decl"], caps)
	if !ok {
		return false
	}
	src, ok := resolveCapture(args["source"], caps)
	if !ok {
		return false
	}
	specType := args["spec_type"]
	valueField := args["value_field"]

	specs := []tsutil.Node{}
	for _, c := range decl.NamedChildren() {
		if c.Type() == specType {
			specs = append(specs, c)
		}
	}
	if len(specs) == 0 {
		return false
	}
	for _, s := range specs {
		if valueField != "" && s.HasFieldName(valueField) {
			return false
		}
	}
	declNames := nonBlankIdentTexts(decl, true)
	srcNames := nonBlankIdentTexts(src, false)
	return setEqual(declNames, srcNames)
}

// ----- helpers -----

// nonBlankIdentTexts collects the texts of identifier nodes within a
// subtree. If recursive is false, only direct named children are
// considered. The "_" blank identifier is excluded.
func nonBlankIdentTexts(n tsutil.Node, recursive bool) map[string]bool {
	out := map[string]bool{}
	if !n.IsValid() {
		return out
	}
	if recursive {
		tsutil.Walk(n, func(c tsutil.Node) bool {
			if c.Type() == "identifier" && c.Text() != "_" {
				out[c.Text()] = true
			}
			return true
		})
	} else {
		for _, c := range n.NamedChildren() {
			if c.Type() == "identifier" && c.Text() != "_" {
				out[c.Text()] = true
			}
		}
	}
	return out
}

// subtreeReferences reports whether any identifier in n's subtree has
// text in names. If exStart < exEnd, identifiers whose byte range is
// fully inside that interval are skipped.
func subtreeReferences(n tsutil.Node, names map[string]bool, exStart, exEnd uint32) bool {
	hasExcl := exStart < exEnd
	found := false
	tsutil.Walk(n, func(c tsutil.Node) bool {
		if found {
			return false
		}
		if c.Type() == "identifier" && names[c.Text()] {
			if hasExcl && c.StartByte() >= exStart && c.EndByte() <= exEnd {
				return true
			}
			found = true
			return false
		}
		return true
	})
	return found
}

func typeIn(set []string, t string) bool {
	for _, s := range set {
		if s == t {
			return true
		}
	}
	return false
}

func splitOr(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			out = append(out, strings.TrimSpace(s[last:i]))
			last = i + 1
		}
	}
	out = append(out, strings.TrimSpace(s[last:]))
	return out
}

func setEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
