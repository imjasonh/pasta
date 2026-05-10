// Package engine runs analyzers over parsed source trees, producing
// diagnostics and edit operations.
//
// Two entry points: Run for a single file (the historical API used by
// many tests) and RunGroup for a set of files that share a fact store.
// Run is a thin wrapper around RunGroup with a one-element file slice.
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

// Result is the output of running one or more analyzers over a single
// source file.
type Result struct {
	Diagnostics []effect.Diagnostic
	Ops         []effect.Op
}

// FileInput describes one source file to run analyzers over as part of
// a group. FileID disambiguates nodes from this file from nodes of
// other files in the group; using the file path is fine.
type FileInput struct {
	FileID string
	Src    []byte
	Lang   lang.Language
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
// rule across the given analyzers, and returns aggregated diagnostics
// and edit ops for that one file.
func Run(
	ctx context.Context,
	src []byte,
	l lang.Language,
	analyzers []*dsl.Analyzer,
) (Result, error) {
	outs, err := RunGroup(ctx, []FileInput{{FileID: "", Src: src, Lang: l}}, analyzers)
	if err != nil {
		return Result{}, err
	}
	if len(outs) == 0 {
		return Result{}, nil
	}
	return outs[0], nil
}

// fileState is the parsed-and-prepared state for one file in a group.
type fileState struct {
	input FileInput
	tree  releaser
	root  tsutil.Node
	env   *match.Env
}

// releaser is the subset of *gts.Tree we actually need (Release).
type releaser interface{ Release() }

// RunGroup parses every file in files, sharing a single fact store
// across them, and returns one Result per file (positionally aligned
// with files).
//
// The schedule is global across the union of all applicable rules.
// For each topo group the engine walks the files; non-fixpoint groups
// run once, fixpoint groups iterate emit-only until the fact store
// stops growing and then do one final collection pass.
func RunGroup(
	ctx context.Context,
	files []FileInput,
	analyzers []*dsl.Analyzer,
) ([]Result, error) {
	if len(files) == 0 {
		return nil, nil
	}

	store := factstore.New()
	states := make([]fileState, 0, len(files))
	results := make([]Result, len(files))

	// Register the release defer up front so a parse failure mid-loop
	// still cleans up the trees we already parsed — `states` is the
	// closed-over slice, so anything appended before the failure is
	// covered.
	defer func() {
		for _, s := range states {
			s.tree.Release()
		}
	}()

	for _, f := range files {
		tree, root, err := tsutil.Parse(ctx, f.Lang.GetLanguage(), f.Src, f.FileID)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f.FileID, err)
		}
		commentTypes := make(map[string]bool, len(f.Lang.CommentTypes))
		for _, t := range f.Lang.CommentTypes {
			commentTypes[t] = true
		}
		env := &match.Env{
			StmtList:     f.Lang.StmtList,
			Predicates:   match.DefaultRegistry(),
			FactStore:    store,
			CommentTypes: commentTypes,
			Index:        match.BuildIndex(root),
		}
		states = append(states, fileState{input: f, tree: tree, root: root, env: env})
	}

	groups, err := scheduleGroups(analyzers)
	if err != nil {
		return nil, err
	}

	for _, group := range groups {
		if !group.fixpoint {
			for i, s := range states {
				if err := runGroupOnFile(group, s, store, &results[i]); err != nil {
					return nil, err
				}
			}
			continue
		}
		// Fixpoint: re-run emit-only across all files until the store
		// stops growing, then do one final collection pass on each
		// file. We don't accumulate diagnostics or edit ops during the
		// emit loop to avoid duplicates.
		for iter := 0; iter < MaxFixpointIterations; iter++ {
			before := store.Len()
			for _, s := range states {
				if err := runGroupOnFile(group, s, store, nil); err != nil {
					return nil, err
				}
			}
			if store.Len() == before {
				break
			}
		}
		for i, s := range states {
			if err := runGroupOnFile(group, s, store, &results[i]); err != nil {
				return nil, err
			}
		}
	}
	return results, nil
}

// runGroupOnFile runs every rule in group that applies to s's language,
// using s's parse tree and env. When collect is nil only facts are
// emitted; when non-nil, diagnostics and edit ops are appended.
func runGroupOnFile(group ruleGroup, s fileState, store *factstore.Store, collect *Result) error {
	for _, sr := range group.rules {
		if !ruleAppliesToLanguage(&sr.rule, s.input.Lang) {
			continue
		}
		if err := runRule(&sr, s.env, s.root, store, collect); err != nil {
			return err
		}
	}
	return nil
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
