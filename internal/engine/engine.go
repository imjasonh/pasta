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
	"path/filepath"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/effect"
	"github.com/imjasonh/pasta/internal/factstore"
	"github.com/imjasonh/pasta/internal/lang"
	"github.com/imjasonh/pasta/internal/match"
	"github.com/imjasonh/pasta/internal/parsecache"
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
	input    FileInput
	tree     releaser
	root     tsutil.Node
	env      *match.Env
	suppress map[int]suppression
}

// releaser is the subset of *gts.Tree we actually need (Release).
type releaser interface{ Release() }

// Option configures a single RunGroup call. Options are applied in
// order; later options win on conflict.
type Option func(*runOpts)

type runOpts struct {
	cache *parsecache.Cache
}

// WithCache enables the persistent parse-result cache for this run.
// The cache is consulted only in the streaming path; runs that take
// the in-memory path (cross-file `requires`, fixpoint groups) skip
// it for correctness, since a file's diagnostics there may depend on
// facts emitted from other files. A nil cache is treated as no
// caching.
func WithCache(c *parsecache.Cache) Option {
	return func(o *runOpts) { o.cache = c }
}

// RunGroup parses every file in files, sharing a single fact store
// across them, and returns one Result per file (positionally aligned
// with files).
//
// The schedule is global across the union of all applicable rules.
//
// Two execution paths:
//
//   - Streaming (default): when no rule has cross-file `requires` and
//     no rule group is a fixpoint, each file is processed independently
//     — parse, run all groups, release the tree. Files are dispatched
//     across a worker pool so parsing and matching run in parallel.
//     Peak memory is O(workers × tree-size) instead of O(N × tree-size).
//     When WithCache is supplied, unchanged files skip parse + match
//     entirely on a cache hit.
//
//   - Legacy in-memory: when a rule's `requires` references facts
//     emitted by other rules, OR when any group is a fixpoint, every
//     file's tree is held in memory while the engine walks the rule
//     groups in topo order. This is necessary because a consumer rule
//     on file F may depend on facts emitted by a provider rule on file
//     F+N — the schedule must visit every file for the producer
//     group before any file for the consumer group.
func RunGroup(
	ctx context.Context,
	files []FileInput,
	analyzers []*dsl.Analyzer,
	opts ...Option,
) ([]Result, error) {
	if len(files) == 0 {
		return nil, nil
	}

	groups, err := scheduleGroups(analyzers)
	if err != nil {
		return nil, err
	}

	var o runOpts
	for _, opt := range opts {
		opt(&o)
	}

	if canStream(groups) {
		return runStreaming(ctx, files, groups, o.cache)
	}
	return runInMemory(ctx, files, groups)
}

// canStream reports whether the schedule is compatible with the
// per-file streaming path. The streaming path requires that each file
// can be processed independently of every other file in the group —
// meaning no rule needs to see facts emitted on a different file.
//
// That holds when (a) no rule group is a fixpoint group (those iterate
// across all files until convergence), and (b) no rule's `requires`
// list is non-empty (rules with `requires` consume facts that might
// have been emitted on any file in the group).
func canStream(groups []ruleGroup) bool {
	for _, g := range groups {
		if g.fixpoint {
			return false
		}
		for _, sr := range g.rules {
			if len(sr.rule.Requires) > 0 {
				return false
			}
		}
	}
	return true
}

// runStreaming processes each file independently across a worker pool
// of size GOMAXPROCS. Trees are released as soon as their file's rules
// finish, keeping peak memory bounded by the worker count rather than
// the file count.
//
// When cache is non-nil, each file is looked up by (lang, source-hash)
// before parsing — a hit returns the cached diagnostics + ops
// directly. Cache writes happen after a fresh parse + match completes.
func runStreaming(
	ctx context.Context,
	files []FileInput,
	groups []ruleGroup,
	cache *parsecache.Cache,
) ([]Result, error) {
	store := factstore.New()
	results := make([]Result, len(files))

	workers := runtime.GOMAXPROCS(0)
	if workers > len(files) {
		workers = len(files)
	}
	if workers < 1 {
		workers = 1
	}

	// errgroup gives us: bounded concurrency (SetLimit), automatic
	// context cancellation on the first error (WithContext), and
	// first-error propagation through Wait. Each iteration writes to
	// its own results[i] slot, so there's no shared mutable state in
	// the closure beyond the fact store (mutex-protected).
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)
	for i := range files {
		i := i
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			return processFile(gctx, files[i], groups, store, &results[i], cache)
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

// processFile parses one file, runs every applicable rule group against
// it, then releases the tree. Used by the streaming path.
//
// When cache is non-nil, it's consulted before parsing. A hit copies
// the cached Diagnostics + Ops straight into out and returns. A miss
// runs the full pipeline and then writes the result back to the
// cache, so the next run over unchanged bytes is a cheap read.
func processFile(
	ctx context.Context,
	f FileInput,
	groups []ruleGroup,
	store *factstore.Store,
	out *Result,
	cache *parsecache.Cache,
) error {
	var key string
	if cache != nil {
		key = parsecache.HashFile(f.Lang.Name, f.Src)
		if e, ok := cache.Get(key); ok {
			out.Diagnostics = append(out.Diagnostics, e.Diagnostics...)
			out.Ops = append(out.Ops, e.Ops...)
			return nil
		}
	}
	s, err := newFileState(ctx, f, store)
	if err != nil {
		return err
	}
	defer s.tree.Release()
	for _, group := range groups {
		if err := runGroupOnFile(group, s, store, out); err != nil {
			return err
		}
	}
	if cache != nil {
		// Copy slices so a future append on `out` doesn't mutate
		// what we hand the cache. The cache encodes immediately, so
		// snapshotting here is fine even though out is shared.
		cache.Put(key, parsecache.Entry{
			Diagnostics: append([]effect.Diagnostic(nil), out.Diagnostics...),
			Ops:         append([]effect.Op(nil), out.Ops...),
		})
	}
	return nil
}

// runInMemory is the legacy path used when the schedule has cross-file
// or fixpoint dependencies. Every file's tree is held in memory while
// the engine walks the rule groups in topo order, so a consumer group
// sees facts emitted by every producer group on every file.
func runInMemory(
	ctx context.Context,
	files []FileInput,
	groups []ruleGroup,
) ([]Result, error) {
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
		s, err := newFileState(ctx, f, store)
		if err != nil {
			return nil, err
		}
		states = append(states, s)
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

// newFileState parses one file and builds its match environment +
// suppression map. The caller owns the returned tree and must call
// Release when done with it.
func newFileState(ctx context.Context, f FileInput, store *factstore.Store) (fileState, error) {
	tree, root, err := tsutil.Parse(ctx, f.Lang.GetLanguage(), f.Src, f.FileID)
	if err != nil {
		return fileState{}, fmt.Errorf("parse %s: %w", f.FileID, err)
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
	return fileState{
		input:    f,
		tree:     tree,
		root:     root,
		env:      env,
		suppress: parseSuppressions(root, commentTypes),
	}, nil
}

// runGroupOnFile runs every rule in group that applies to s's language,
// using s's parse tree and env. When collect is nil only facts are
// emitted; when non-nil, diagnostics and edit ops are appended.
func runGroupOnFile(group ruleGroup, s fileState, store *factstore.Store, collect *Result) error {
	base := filepath.Base(s.input.FileID)
	for _, sr := range group.rules {
		if !ruleAppliesToLanguage(&sr.rule, s.input.Lang) {
			continue
		}
		if !ruleAppliesToFile(&sr.rule, base) {
			continue
		}
		if err := runRule(&sr, s.env, s.root, store, collect, s.suppress); err != nil {
			return err
		}
	}
	return nil
}

// ruleAppliesToFile evaluates a rule's file_match globs against the
// file's basename. An empty list means "no filter — apply to every
// file the language filter already accepted".
func ruleAppliesToFile(rule *dsl.Rule, base string) bool {
	if len(rule.FileMatch) == 0 {
		return true
	}
	for _, pat := range rule.FileMatch {
		if ok, _ := filepath.Match(pat, base); ok {
			return true
		}
	}
	return false
}

// runRule matches the rule, runs preconditions, and (if collect != nil)
// appends diagnostics and edit ops to the result. Facts are always
// emitted into the store, regardless of collect.
//
// `suppress` carries any `pasta:ignore` directives parsed out of the
// file. A diagnostic anchored on a suppressed line is dropped along
// with the rule's rewrite for that match — facts still emit, since
// they're internal state and dropping them would change downstream
// rule behavior.
func runRule(sr *scheduledRule, env *match.Env, root tsutil.Node, store *factstore.Store, collect *Result, suppress map[int]suppression) error {
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
		if isSuppressed(suppress, rule.Name, effect.ComputeLine(diagAnchor.Src, diagAnchor.StartByte())) {
			continue
		}
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
