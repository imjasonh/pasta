// Package match implements pattern matching against tree-sitter nodes
// using the pasta DSL's #Pattern definitions.
package match

import (
	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/tsutil"
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
	StmtList     tsutil.StmtListProvider
	Predicates   PredicateRegistry
	FactStore    FactStore
	CurrentRule  string
}

// FactStore is the minimal interface the matcher needs from the fact store.
// Phase 1 doesn't read facts in any iferr rule, so this can be empty in
// tests; the registry's has_fact predicate uses it.
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

// FindAll walks the tree and returns every match of pattern p anchored at
// any node whose type is in p.Node. Each match's captures include the
// anchor node bound to "_root".
func FindAll(p *dsl.Pattern, root tsutil.Node, env *Env) []Match {
	var matches []Match
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
		named := n.NamedChildren()
		// Positional match: each Children[i] against named[i].
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

// matchAdjacent slides a window of size len(adj) across the container's
// statement list, attempting to match all elements at each window
// position. The first successful window wins. If an adjacent element has
// a `preceding` clause, the prior sibling (window_start-1) is matched
// against it.
func matchAdjacent(adj []dsl.Child, container tsutil.Node, env *Env, caps Captures) bool {
	stmts := env.StmtList(container)
	winSize := len(adj)
	if winSize == 0 {
		return true
	}
	if len(stmts) < winSize {
		return false
	}
	for start := 0; start+winSize <= len(stmts); start++ {
		try := caps.Clone()
		ok := true
		for i := 0; i < winSize; i++ {
			el := adj[i]
			if !matchChild(&el, stmts[start+i], env, try) {
				ok = false
				break
			}
			if el.Preceding != nil {
				// Look one statement back from the window's first element.
				prevIdx := start + i - 1
				if prevIdx < 0 {
					if !precedingOptional(el.Preceding) {
						ok = false
						break
					}
					continue
				}
				prev := stmts[prevIdx]
				if !matchChild(el.Preceding, prev, env, try) {
					if !precedingOptional(el.Preceding) {
						ok = false
						break
					}
				}
			}
		}
		if ok {
			// Commit the speculative captures.
			for k, v := range try {
				caps[k] = v
			}
			return true
		}
	}
	return false
}

func precedingOptional(c *dsl.Child) bool {
	return c != nil && c.Quantifier == "?"
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
