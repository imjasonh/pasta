// Package engine runs analyzers over a parsed source tree, producing
// diagnostics and edit operations.
package engine

import (
	"context"
	"fmt"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/effect"
	"github.com/imjasonh/pasta/internal/factstore"
	"github.com/imjasonh/pasta/internal/lang"
	"github.com/imjasonh/pasta/internal/match"
	"github.com/imjasonh/pasta/internal/tsutil"
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

// MaxFixpointIterations bounds the number of times a fixpoint group of
// rules will be re-run before pasta gives up. Reaching this limit
// almost always indicates a runaway emission (e.g. a rule emits a fact
// derived from data that shifts on every iteration). 50 is comfortably
// larger than the depth of any realistic monotone analysis.
const MaxFixpointIterations = 50

// Run parses src with l's tree-sitter grammar, runs every applicable
// rule across the given analyzers in topological order (with fixpoint
// iteration for cyclic dependencies), and returns aggregated
// diagnostics and edit ops.
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
	commentTypes := make(map[string]bool, len(l.CommentTypes))
	for _, t := range l.CommentTypes {
		commentTypes[t] = true
	}
	env := &match.Env{
		StmtList:     l.StmtList,
		Predicates:   match.DefaultRegistry(),
		FactStore:    store,
		CommentTypes: commentTypes,
		Index:        match.BuildIndex(root),
	}

	groups, err := scheduleGroups(analyzers, l)
	if err != nil {
		return Result{}, err
	}

	var res Result
	for _, group := range groups {
		if !group.fixpoint {
			for _, sr := range group.rules {
				if err := runRule(&sr, env, root, store, &res); err != nil {
					return res, err
				}
			}
			continue
		}
		// Fixpoint: re-run the group's rules until the fact store
		// stops growing or we hit the iteration cap. We DON'T
		// accumulate diagnostics or edit ops within the fixpoint loop
		// (only on the last iteration), to avoid duplicates: we run
		// the group once with effect-collection disabled to drive the
		// fact store to a fixed point, then once more to gather
		// diagnostics and edits at the converged state.
		for iter := 0; iter < MaxFixpointIterations; iter++ {
			before := store.Len()
			for _, sr := range group.rules {
				if err := runRule(&sr, env, root, store, nil); err != nil {
					return res, err
				}
			}
			if store.Len() == before {
				break
			}
		}
		// Final pass: gather effects with the converged fact store.
		for _, sr := range group.rules {
			if err := runRule(&sr, env, root, store, &res); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// runRule matches the rule, runs preconditions, and (if collect != nil)
// appends diagnostics and edit ops to the result. Facts are always
// emitted into the store, regardless of collect.
func runRule(sr *scheduledRule, env *match.Env, root tsutil.Node, store *factstore.Store, collect *Result) error {
	rule := sr.rule
	matches := match.FindAll(&rule.Match, root, env)
	for _, m := range matches {
		if !runPreconditions(&rule, env, m.Captures) {
			continue
		}
		emitFacts(&rule, m, store)
		if collect == nil {
			continue
		}
		diagAnchor := pickDiagAnchor(&rule, m)
		if rule.Diagnose != nil {
			collect.Diagnostics = append(collect.Diagnostics,
				effect.BuildDiagnostic(rule.Name, rule.Diagnose, diagAnchor, m.Captures))
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
				return fmt.Errorf("build ops for rule %s: %w", rule.Name, err)
			}
			collect.Ops = append(collect.Ops, ops...)
		}
	}
	return nil
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
		if !env.Predicates.EvalCheck(pc.Check, pc.Args, env, caps) {
			return false
		}
	}
	return true
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
