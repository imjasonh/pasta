// Package engine runs analyzers over a parsed source tree, producing
// diagnostics and edit operations.
package engine

import (
	"context"
	"fmt"

	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/effect"
	"github.com/imjasonh/pasta/pkg/factstore"
	"github.com/imjasonh/pasta/pkg/lang"
	"github.com/imjasonh/pasta/pkg/match"
	"github.com/imjasonh/pasta/pkg/tsutil"
)

// Result is the output of running one or more analyzers over a source.
type Result struct {
	Diagnostics []effect.Diagnostic
	Ops         []effect.Op
}

// scheduledRule pairs a rule with the analyzer that owns it, so we can
// schedule rules globally across analyzers.
type scheduledRule struct {
	analyzer *dsl.Analyzer
	name     string
	rule     dsl.Rule
}

// Run parses src with l's tree-sitter grammar, runs every applicable
// rule across the given analyzers in topological order (by
// requires/provides), and returns aggregated diagnostics and edit ops.
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

	store := factstore.New()
	env := &match.Env{
		StmtList:   l.StmtList,
		Predicates: match.DefaultRegistry(),
		FactStore:  store,
	}

	scheduled, err := scheduleRules(analyzers, l)
	if err != nil {
		return Result{}, err
	}

	var res Result
	for _, sr := range scheduled {
		rule := sr.rule
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
			emitFacts(&rule, m, store)
		}
	}
	return res, nil
}

// emitFacts records each fact declared in the rule's `emit` clause,
// resolving the `attach` capture to the node the fact is attached to.
// Payloads with `@name` interpolation are flattened to plain strings.
func emitFacts(rule *dsl.Rule, m match.Match, store *factstore.Store) {
	for _, em := range rule.Emit {
		anchor, ok := m.Captures[em.Attach]
		if !ok {
			continue
		}
		payload := flattenPayload(em.Payload, m.Captures)
		store.Emit(em.Fact, anchor, payload)
	}
}

// flattenPayload converts a CUE-decoded payload (any) into a flat
// map[string]any with @capture interpolations resolved.
func flattenPayload(p any, caps match.Captures) map[string]any {
	m, ok := p.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = effect.Interpolate(s, caps)
		} else {
			out[k] = v
		}
	}
	return out
}

// scheduleRules collects every rule applicable to language l across all
// analyzers and orders them topologically by requires/provides. A rule
// that requires fact F runs after every rule that provides F (within
// the same Run). Cycles error out — fixpoint groups are future work.
func scheduleRules(analyzers []*dsl.Analyzer, l lang.Language) ([]scheduledRule, error) {
	var all []scheduledRule
	for _, a := range analyzers {
		for name, rule := range a.Rules {
			if !ruleAppliesToLanguage(&rule, l) {
				continue
			}
			all = append(all, scheduledRule{analyzer: a, name: name, rule: rule})
		}
	}

	// Index rules by what they provide.
	providers := map[string][]int{}
	for i, r := range all {
		for _, f := range r.rule.Provides {
			providers[f] = append(providers[f], i)
		}
	}

	// Build edges: for each rule, depend on every rule providing what
	// it requires. Self-loops are dropped (a rule that requires its own
	// provides is permitted; it just runs once with an empty store).
	indeg := make([]int, len(all))
	out := make([][]int, len(all))
	for i, r := range all {
		for _, req := range r.rule.Requires {
			for _, j := range providers[req] {
				if j == i {
					continue
				}
				out[j] = append(out[j], i)
				indeg[i]++
			}
		}
	}

	// Kahn's algorithm. Within ready set, prefer the rule whose
	// (analyzer, rule-name) sorts first, for deterministic ordering.
	var ready []int
	for i := range all {
		if indeg[i] == 0 {
			ready = append(ready, i)
		}
	}
	sortReady(ready, all)

	var ordered []scheduledRule
	for len(ready) > 0 {
		i := ready[0]
		ready = ready[1:]
		ordered = append(ordered, all[i])
		for _, j := range out[i] {
			indeg[j]--
			if indeg[j] == 0 {
				ready = append(ready, j)
			}
		}
		sortReady(ready, all)
	}
	if len(ordered) != len(all) {
		return nil, fmt.Errorf("rule dependency cycle: SCC scheduling not yet implemented")
	}
	return ordered, nil
}

func sortReady(ready []int, all []scheduledRule) {
	for i := 1; i < len(ready); i++ {
		for j := i; j > 0; j-- {
			a, b := ready[j-1], ready[j]
			if all[a].analyzer.Name > all[b].analyzer.Name ||
				(all[a].analyzer.Name == all[b].analyzer.Name && all[a].name > all[b].name) {
				ready[j-1], ready[j] = ready[j], ready[j-1]
				continue
			}
			break
		}
	}
}

// ruleAppliesToLanguage returns true if the rule's `languages` list
// includes the given language by NAME or by GRAMMAR.
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
		return true
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
