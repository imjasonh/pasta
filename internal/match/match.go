// Package match implements pattern matching against tree-sitter nodes
// using the pasta DSL's #Pattern definitions.
package match

import (
	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/tsutil"
)

// Captures is a map from capture name to the matched node.
type Captures map[string]tsutil.Node

// Clone returns a shallow copy of c.
func (c Captures) Clone() Captures {
	out := make(Captures, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}

// Env carries language- and rule-set-specific state needed for matching.
type Env struct {
	StmtList    tsutil.StmtListProvider
	Predicates  PredicateRegistry
	FactStore   FactStore
	CurrentRule string

	// CommentTypes is the set of tree-sitter node types treated as
	// comments for the current language. Predicates that count or
	// list named children skip these so e.g.
	// `named_child_count: ["@args", "1"]` matches `f(/* note */ x)`.
	CommentTypes map[string]bool

	// Index maps tree-sitter node type to all named nodes of that type
	// in the current parse tree. Built once per parse, consulted by
	// FindAll to skip the per-rule full-tree walk. Optional: when nil,
	// FindAll falls back to tsutil.Walk.
	Index *NodeIndex
}

// NodeIndex is a per-parse-tree map from node type to every named node
// of that type. Built in O(N) at parse time; turns FindAll's per-rule
// cost from O(tree) to O(target-type-count).
type NodeIndex struct {
	ByType map[string][]tsutil.Node
}

// BuildIndex walks root once and indexes every named node by type.
func BuildIndex(root tsutil.Node) *NodeIndex {
	idx := &NodeIndex{ByType: map[string][]tsutil.Node{}}
	tsutil.Walk(root, func(n tsutil.Node) bool {
		idx.ByType[n.Type()] = append(idx.ByType[n.Type()], n)
		return true
	})
	return idx
}

// FactStore is the minimal interface the matcher needs from the fact
// store — only the has_fact / not_has_fact predicates consult it. Tests
// that don't exercise fact-aware rules can leave Env.FactStore nil.
type FactStore interface {
	Has(node tsutil.Node, kind string) bool
}

// Match is a single successful match: the captures bound during structural
// matching, the anchor node the rule was matched at, and a reference to
// the rule for diagnostic/effect emission.
type Match struct {
	Anchor   tsutil.Node
	Captures Captures
}

// FindAll returns every match of pattern p anchored at any node whose
// type is in p.Node. Each match's captures include the anchor node
// bound to "_root".
//
// When env.Index is set and p.Node is non-empty, candidates are
// retrieved from the index in O(target-type-count). When the index is
// nil OR p.Node is empty (match-any-type), falls back to a tree walk.
func FindAll(p *dsl.Pattern, root tsutil.Node, env *Env) []Match {
	var matches []Match

	if env.Index != nil && len(p.Node) > 0 {
		seen := map[[2]uint32]bool{}
		for _, t := range p.Node {
			for _, n := range env.Index.ByType[t] {
				key := [2]uint32{n.StartByte(), n.EndByte()}
				if seen[key] {
					continue
				}
				seen[key] = true
				caps := Captures{"_root": n}
				if matchPattern(p, n, env, caps) {
					matches = append(matches, Match{Anchor: n, Captures: caps})
				}
			}
		}
		return matches
	}

	tsutil.Walk(root, func(n tsutil.Node) bool {
		if !typeMatches(p.Node, n.Type()) {
			return true
		}
		caps := Captures{"_root": n}
		if matchPattern(p, n, env, caps) {
			matches = append(matches, Match{Anchor: n, Captures: caps})
		}
		return true
	})
	return matches
}

// matchPattern is the core recursive matcher. It mutates caps in place when
// it succeeds and returns true; on failure it returns false (caller is
// responsible for branching with Clone if speculative matching is needed).
func matchPattern(p *dsl.Pattern, n tsutil.Node, env *Env, caps Captures) bool {
	if !n.IsValid() {
		return false
	}
	if !typeMatches(p.Node, n.Type()) {
		return false
	}

	for _, fld := range p.AbsentFields {
		if n.HasFieldName(fld) {
			return false
		}
	}

	for fieldName, child := range p.Fields {
		fc := n.ChildByFieldName(fieldName)
		if !fc.IsValid() {
			return false
		}
		if !matchChild(&child, fc, env, caps) {
			return false
		}
	}

	if len(p.Children) > 0 {
		// Positional match against named children, skipping comments.
		// Comments-as-children are tree-sitter's representation of
		// `f(/* note */ x)` and similar — they should not throw off
		// positional matching of semantic children.
		named := nonCommentNamedChildren(n, env)
		if len(p.Children) > len(named) {
			return false
		}
		for i, child := range p.Children {
			if !matchChild(&child, named[i], env, caps) {
				return false
			}
		}
	}

	if len(p.Adjacent) > 0 {
		if !matchAdjacent(p.Adjacent, n, env, caps) {
			return false
		}
	}

	for _, pred := range p.Where {
		if !env.Predicates.Eval(pred, env, caps) {
			return false
		}
	}

	return true
}

// matchChild dispatches between capture-form and pattern-form.
func matchChild(c *dsl.Child, n tsutil.Node, env *Env, caps Captures) bool {
	if c.Capture != "" {
		// Bind the capture; optionally constrain by inner pattern.
		if inner := c.AsPattern(); inner != nil {
			if !matchPattern(inner, n, env, caps) {
				return false
			}
		}
		caps[c.Capture] = n
		return true
	}
	// Pattern-form.
	inner := c.AsPattern()
	if inner == nil {
		// Empty child matches any node.
		return true
	}
	return matchPattern(inner, n, env, caps)
}

// matchAdjacent slides starting positions across the container's
// statement list and tries to match the adj sequence. Each element
// consumes exactly one statement.
//
// If an element has a `preceding` clause, the matcher also requires
// the statement immediately before the matched element to satisfy
// that preceding pattern.
func matchAdjacent(adj []dsl.Child, container tsutil.Node, env *Env, caps Captures) bool {
	stmts := env.StmtList(container)
	if len(adj) == 0 {
		return true
	}
	if len(stmts) < len(adj) {
		return false
	}
	for start := 0; start <= len(stmts)-len(adj); start++ {
		try := caps.Clone()
		if matchAdjSeq(adj, stmts, start, env, try) {
			for k, v := range try {
				caps[k] = v
			}
			return true
		}
	}
	return false
}

// matchAdjSeq tries to match adj starting at stmts[startIdx]. Returns
// true if the entire sequence matches; binds captures into caps on
// success.
func matchAdjSeq(adj []dsl.Child, stmts []tsutil.Node, startIdx int, env *Env, caps Captures) bool {
	if len(adj) == 0 {
		return true
	}
	if startIdx >= len(stmts) {
		return false
	}
	el := adj[0]
	try := caps.Clone()
	if !matchChild(&el, stmts[startIdx], env, try) {
		return false
	}
	if el.Preceding != nil {
		prevIdx := startIdx - 1
		if prevIdx < 0 {
			return false
		}
		if !matchChild(el.Preceding, stmts[prevIdx], env, try) {
			return false
		}
	}
	if !matchAdjSeq(adj[1:], stmts, startIdx+1, env, try) {
		return false
	}
	for k, v := range try {
		caps[k] = v
	}
	return true
}

// typeMatches reports whether actual is in the wantList. An empty wantList
// matches any type.
func typeMatches(wantList []string, actual string) bool {
	if len(wantList) == 0 {
		return true
	}
	for _, w := range wantList {
		if w == actual {
			return true
		}
	}
	return false
}
