// Package engine runs analyzers over a parsed source tree, producing
// diagnostics and edit operations.
package engine

import (
	"context"
	"fmt"
	"sort"

	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/effect"
	"github.com/imjasonh/pasta/pkg/lang"
	"github.com/imjasonh/pasta/pkg/match"
	"github.com/imjasonh/pasta/pkg/tsutil"
)

// Result is the output of running one or more analyzers over a source.
type Result struct {
	Diagnostics []effect.Diagnostic
	Ops         []effect.Op
}

// Run parses src with l's tree-sitter grammar, runs each analyzer's rules
// in declaration order, and returns aggregated diagnostics and edit ops.
//
// Rules whose `languages` list does not include l.Name (and is not "*")
// are silently skipped. Pre-conditions are evaluated through the same
// predicate registry as `where` clauses.
func Run(
	ctx context.Context,
	src []byte,
	l lang.Language,
	analyzers []*dsl.Analyzer,
) (Result, error) {
	tree, root, err := tsutil.Parse(ctx, l.GetLanguage(), src)
	if err != nil {
		return Result{}, fmt.Errorf("parse: %w", err)
	}
	defer tree.Release()

	env := &match.Env{
		StmtList:   l.StmtList,
		Predicates: match.DefaultRegistry(),
	}

	var res Result
	for _, a := range analyzers {
		names := make([]string, 0, len(a.Rules))
		for k := range a.Rules {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, name := range names {
			rule := a.Rules[name]
			if !ruleAppliesToLanguage(&rule, l) {
				continue
			}
			matches := match.FindAll(&rule.Match, root, env)
			for _, m := range matches {
				if !runPreconditions(&rule, env, m.Captures) {
					continue
				}
				diagAnchor := pickDiagAnchor(&rule, m)
				if rule.Diagnose != nil {
					res.Diagnostics = append(res.Diagnostics, effect.BuildDiagnostic(rule.Name, rule.Diagnose, diagAnchor, m.Captures))
				}
				if rule.Rewrite != nil {
					var rwOpts dsl.RewriteOpts
					if rule.RewriteOpts != nil {
						rwOpts = *rule.RewriteOpts
					}
					adjacentSeq := captureNamesFromAdjacent(rule.Match.Adjacent)
					ops, err := effect.BuildOps(rule.Name, rule.Rewrite, effect.BuildContext{
						Captures:    m.Captures,
						AdjacentSeq: adjacentSeq,
						RewriteOpts: rwOpts,
						Root:        root,
					})
					if err != nil {
						return res, fmt.Errorf("build ops for rule %s: %w", rule.Name, err)
					}
					res.Ops = append(res.Ops, ops...)
				}
			}
		}
	}
	return res, nil
}

// ruleAppliesToLanguage returns true if the rule's `languages` list
// includes the given language by NAME or by GRAMMAR. This lets rules
// written for `["go"]` automatically apply to any user-defined language
// alias whose grammar is Go (e.g. an external `notgo` lang for `.notgo`
// files). To target a specific alias and not all Go-grammar languages,
// list the language name explicitly.
func ruleAppliesToLanguage(rule *dsl.Rule, l lang.Language) bool {
	if len(rule.Languages) == 0 {
		return true
	}
	for _, want := range rule.Languages {
		if want == "*" || want == l.Name || want == l.Grammar {
			return true
		}
	}
	return false
}

// pickDiagAnchor selects the node a diagnostic should be reported at.
// Heuristic: if the rule has an `adjacent` clause, use the first
// adjacent element's top-level capture (typically the "main" node the
// diagnostic refers to, e.g. "assign" in iferr). Otherwise fall back to
// the rule's matched root.
func pickDiagAnchor(rule *dsl.Rule, m match.Match) tsutil.Node {
	if len(rule.Match.Adjacent) > 0 {
		first := rule.Match.Adjacent[0]
		if first.Capture != "" {
			if n, ok := m.Captures[first.Capture]; ok {
				return n
			}
		}
	}
	return m.Anchor
}

// runPreconditions evaluates each pre_condition through the predicate
// registry. Optional checks pass when the predicate fails or when any
// referenced capture is unbound. Unknown predicate ops fail closed
// (unless marked optional).
func runPreconditions(rule *dsl.Rule, env *match.Env, caps match.Captures) bool {
	for _, pc := range rule.PreConditions {
		if pc.Optional {
			if !preconditionCapturesBound(pc, caps) {
				continue
			}
		}
		ok := env.Predicates.EvalCheck(pc.Check, pc.Args, env, caps)
		if !ok {
			if pc.Optional {
				continue
			}
			return false
		}
	}
	return true
}

// preconditionCapturesBound: every "@name" or "@a|@b" arg must resolve
// to at least one bound capture for the precondition to be considered
// "applicable" when marked optional.
func preconditionCapturesBound(pc dsl.Precondition, caps match.Captures) bool {
	for _, v := range pc.Args {
		if !atRefHasBoundCapture(v, caps) {
			return false
		}
	}
	return true
}

func atRefHasBoundCapture(arg string, caps match.Captures) bool {
	if len(arg) == 0 || arg[0] != '@' {
		return true // not a capture ref; treat as bound
	}
	for _, alt := range splitOr(arg) {
		name := alt
		if len(name) > 0 && name[0] == '@' {
			name = name[1:]
		}
		if _, ok := caps[name]; ok {
			return true
		}
	}
	return false
}

func splitOr(s string) []string {
	var out []string
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	out = append(out, s[last:])
	return out
}

// captureNamesFromAdjacent extracts the ordered list of capture names
// declared as the immediate top-level capture of each adjacent element.
// Used for delete_from fallback.
func captureNamesFromAdjacent(adj []dsl.Child) []string {
	out := make([]string, 0, len(adj)*2)
	for _, c := range adj {
		if c.Preceding != nil && c.Preceding.Capture != "" {
			out = append(out, c.Preceding.Capture)
		}
		if c.Capture != "" {
			out = append(out, c.Capture)
		}
	}
	return out
}
