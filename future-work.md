# Future Work

This is the consolidated TODO for pasta. It pulls items from
[plan.md](./plan.md) (the design doc) and [cue.md](./cue.md) (the
CUE-leverage essay) that we haven't yet implemented, plus gaps that
surfaced while writing analyzers.

Each item has:
- a short description and motivation,
- a pointer to where it's discussed in plan.md / cue.md / the
  conversation that surfaced it,
- a rough effort estimate (S / M / L),
- a brief note on what's blocking it (if anything).

The framework today is plenty for many analyzers — 35 ship today
across 7 languages, with fact-passing, fixpoint scheduling, and
node-indexed matching. Most of the items below are about reducing
boilerplate, catching errors earlier, or unlocking cross-file analysis.

## Status of plan.md sections

| plan.md section | Status |
|---|---|
| §1 Goal — tree-sitter + CUE + adapters | ✅ |
| §2 Architecture pipeline | ✅ |
| §3.1 Facts | ✅ basic; lattice form deferred (§5 below) |
| §3.2 Patterns + node unions | ✅ |
| §3.2 Quantifiers in adjacent | ⚠️ `?` only; `*`/`+` deferred |
| §3.3 Diagnostics + rewrites | ✅ + byte-range trim |
| §3.4 Preconditions | ✅ now via predicate registry |
| §3.5 Rule + Analyzer + Adapter | ✅ Rule/Analyzer; adapter removed in favor of generic predicates |
| §3.6 Scheduling (topo + SCC + fixpoint) | ✅ |
| §4.1 nilcheck example | replaced with `go_negcmp` (the §4.1 transform was invalid Go) |
| §4.2 errcheck (fact passing) | ✅ `go_errcheck` |
| §4.3 taint (fixpoint) | ✅ `python_taint`, plus go/js/rust variants |
| §4.4 iferr (DSL stress test) | ✅ all 38 cases passing |
| §5 Go adapter | replaced with generic predicates parameterized by grammar specifics |
| §6.1–6.3 Runtime engine | ✅ |
| §7 Existing work / influences | reference doc |
| §8.1 Cross-file facts | ❌ open (see below) |
| §8.2 Lattice fact model | ❌ speculative |
| §8.3 Rewrite escape hatches | ⚠️ added `trim_start`/`trim_end`; broader story open |
| §8.4 Performance | ⚠️ node-type indexing done; early termination open |
| §8.5 Testing model | ✅ analysistest-style harness |
| §8.6 Cross-analyzer fact namespacing | ⚠️ implicitly done — facts are flat across analyzers in one Run |

---

## DSL extensions

### `*` and `+` quantifiers in `adjacent`
**Effort:** M. **Source:** plan.md §3.2.
The matcher already does backtracking for `?`. `*` (zero-or-more) and
`+` (one-or-more) follow the same shape but need extra bookkeeping for
greedy match-up-to-N then back off. Useful for "any number of leading
defer statements", "three or more consecutive slice appends".

### `type_is` predicate
**Effort:** L. **Source:** plan.md §3.1 / §5.
Checks that a captured expression's inferred TYPE matches a name. This
needs per-language type information — Go's `go/types`, TypeScript's
checker, etc. — which means either bringing back per-language adapters
or shelling out to language servers. The CUE rule layer is ready
(`type_is` is in the schema as a stub); the runtime side is the work.

### `all_match` meta-predicate
**Effort:** M. **Source:** plan.md §3.1.
Invokes a sub-predicate on every named child of a captured list:
`{op: "all_match", args: [@list, "matches", "regex"]}`. Lets a rule
say "every LHS expression is an identifier" without a custom predicate.
DSL change: `Predicate.args` would need to allow nested predicate
specs; today it's `[]string`.

### `same_object` predicate (plan.md §3.4)
**Effort:** L. **Source:** plan.md §3.4 / §5.
Checks that two captured identifiers resolve to the SAME declaration
(not just same text). This is what `same_ident` should be when scope
matters — but it requires full symbol resolution, which is per-language
adapter territory. Today `same_ident` does text equality, which is
adequate within a single function but conflates shadowed names.

### `file_match` filtering (plan.md §3.5)
**Effort:** S. **Source:** plan.md §3.5.
The schema declares `file_match?: [...string]` on `#Rule` for filename
globs (e.g. `["*_test.go"]` to restrict a rule to test files). The
runtime currently ignores this field. Wiring it through the engine is
straightforward: skip rules whose `file_match` doesn't include the
current file's basename.

### `subtree_has_type` / `subtree_lacks_type`
**Effort:** S. **Source:** conversation while writing rust_unsafe_no_safety.
A predicate `{op: "subtree_has", args: [@cap, "type"]}` that reports
whether a captured node's subtree contains any descendant of the
given type. Useful for "does this function body contain a `panic!()`".
We have helpers (`subtreeReferences`) — exposing them as a predicate is
straightforward.

### Slicing on `@capture` interpolation
**Effort:** S. **Source:** dbg-macro rewrite.
Today we have edit-level `trim_start` / `trim_end`. A more general
interpolation form `@cap[N:M]` would let any text reference a
substring of a capture. Mostly cosmetic; `trim_start`/`trim_end`
already covers the common case.

### N-arg variadic rewrites
**Effort:** M. **Source:** Object.assign / [].concat rules.
`{...x, ...y, ...z}` from `Object.assign({}, x, y, z)` requires the
rewrite to know the number of arguments at runtime. The current
`replacement: "{...@x, ...@y}"` syntax is fixed. Options: (a) loop
construct in interpolation; (b) per-arity rules; (c) a "join captures"
edit form.

### Quantifier on `preceding`
**Effort:** S. **Source:** rust_unsafe_no_safety attempt.
`preceding` already accepts `quantifier: "?"`. Combined with a
`prev_sibling_does_not_match` predicate, this could express
"unsafe block without preceding SAFETY comment" cleanly. Today the
match would be: preceding optional + WHERE NOT preceded by safety
comment — needs negative matching (which we do have via
`prev_sibling_matches` + `not_matches`).

---

## CUE leverage (cue.md)

### Pattern libraries (cue.md §1.2)
**Effort:** L (worth it). **Source:** cue.md §1.2.
Build out `pasta.dev/patterns/<lang>/` — importable CUE definitions
for common shapes:

```cue
// pasta.dev/patterns/go
#NilComparison: schema.#Pattern & {...}
#ShortVarDecl:  schema.#Pattern & {...}
#ErrorReturningFunc: schema.#Pattern & {...}
#StmtList:      schema.#Pattern & {...}
```

Today every rule re-specifies these from scratch. iferr, errcheck,
taint, deprecated_use, and several others have hand-rolled patterns
for "function declaration with name capture" or "assignment from
identifier to identifier". Extracting them shrinks rule files
substantially and makes new rules easier to author. Highest ROI item
in this list.

### Load-time fact-dependency validation (cue.md §2.1)
**Effort:** S. **Source:** cue.md §2.1.
Schema-side comprehensions that fail compilation when a rule's
`requires:` includes a fact no rule `provides:`. cue.md sketches the
exact CUE for this. Catches typos in fact names at load time rather
than at runtime when no diagnostics fire.

### Load-time capture validation (cue.md §3)
**Effort:** M. **Source:** cue.md §3.
Walk the rule's match pattern, collect capture names. Walk every
`@name` reference in `replacement`, `text`, `message`, and predicate
args; verify each resolves to a defined capture. Today typos pass
through silently (the @-interpolation just emits the literal text).
Would have caught bugs I hit while writing analyzers.

### Predicate-name validation (cue.md §2.2 adapted)
**Effort:** S. **Source:** cue.md §2.2.
The schema already enumerates valid `op` values for `#Predicate`, so
unknown op names are a CUE error. We could extend the same idea to
`pre_conditions[].check` and to predicate-arg shapes (e.g.
`ancestor_is` requires args[1] to look like a node-type list).

### Rule inheritance refactor (cue.md §2.3)
**Effort:** S. **Source:** cue.md §2.3.
Refactor iferr's two rules (`inline_define` and `inline_assign`) to
extend a shared `#IferrBase` definition with the common shape (node
union, diagnose message, rewrite opts). Demonstrates CUE unification.

### Conditional-field language polymorphism (cue.md §4)
**Effort:** M. **Source:** cue.md §4.
Write ONE `#TaintAnalyzer` definition that, given a `_lang` parameter,
generates the language-specific source/sink/assignment patterns:

```cue
#TaintAnalyzer: {
    _lang: "go" | "python" | "rust" | "javascript"
    rules: {
        if _lang == "go" { ... go-specific match shapes ... }
        if _lang == "python" { ... python ... }
        ...
    }
}
```

Today we have four near-identical taint analyzers (go_taint,
python_taint, rust_taint, js_taint). Conditional fields would let one
definition produce all four.

### Lattice-model facts (cue.md §5)
**Effort:** L. **Source:** cue.md §5.
Speculative. Facts as CUE values that unify monotonically rather than
overwriting. Convergence detection becomes "did the CUE value change?"
Worth prototyping when sophisticated dataflow analyses arrive.

### Auto-generated diagnostic metadata (cue.md §3)
**Effort:** S. **Source:** cue.md §3.
Derive a `_help_url` from each rule's name, inject into every
diagnostic. Add a `severity` default tied to rule category. Pure
CUE — no runtime change.

---

## Fact system

### Cross-file / cross-package facts (plan.md §8.1)
**Effort:** L. **Source:** plan.md §8.1.
The runner today processes one file at a time with a fresh fact
store per file. Real production analysis (errcheck across packages,
deprecated-use tracking across files, dead-code detection) needs
facts to flow between files.

Sketch:
1. Walk all input files; group by package / module per a CUE-declared
   convention.
2. For each topo level of the rule graph: run all "fact-emitting"
   rules over every file in scope, accumulating into a shared store.
3. After the emit phase converges, run "fact-consuming" rules over
   every file, gathering diagnostics.

This is a substantial refactor of `engine.Run` and `runner.TestDir`.

### Scope-aware fact keys
**Effort:** M. **Source:** taint analyzers' caveat.
Today the fact store has a secondary by-name index that's
scope-blind: a name shadowed in another function still hits a fact
keyed by that name. The taint testdata works around this by using
unique variable names per function. Real precision needs facts keyed
by `(name, scope-id)` where scope-id is derived from the enclosing
function or block.

### Cross-analyzer fact namespacing (plan.md §8.6)
**Effort:** S. **Source:** plan.md §8.6.
Today fact `kinds` are flat strings across all loaded analyzers. Two
analyzers using `kind: "tainted"` for different purposes would
collide. A scoped form like `analyzerName.kind` would prevent it. Low
priority until two analyzers actually conflict.

### Fact `scope` field (plan.md §8.1)
**Effort:** M. **Source:** plan.md §8.1.
`#Fact` could grow a `scope: "node" | "file" | "package" | "module"`
field that controls how the runtime keys the fact. Most facts today
are implicitly node-scoped (with the by-name secondary index acting
as a poor file-scope). Explicit scope makes the storage strategy
declarative and ties into cross-file work.

---

## Rewrite escape hatches (plan.md §8.3)

The current edit primitives (`target`/`replacement`,
`position`/`anchor`, `delete_from`/`delete_to`, `within`,
`trim_start`/`trim_end`) cover most syntactic rewrites. They struggle
when the new code can't be assembled by stitching captures and
literals — e.g. generating import statements, computing
correctly-quoted JSON for a captured value, or producing different
text per call site.

Options worth exploring (none yet implemented):

1. **CUE expressions in replacement text.** `replacement: "if \(strings.ToUpper(@name)) ..."`.
   Constrains computation to what CUE can express, which is
   surprisingly powerful (string ops, comprehensions). Plan.md §8.3
   suggests this.

2. **Starlark snippets per rewrite.** Bigger surface area, escapes
   the CUE-only invariant. plan.md §8.3 mentions this as a possibility
   but it's significant scope and complicates security/sandboxing.

3. **Transform functions registered in Go.** Rules reference a
   transform by name; Go-side registry computes the new text. Lowest
   effort, breaks the "rules are pure CUE" promise. Useful escape
   hatch for rare cases (one or two transforms instead of a generic
   mechanism).

Pick when a real use case forces the choice.

---

## Performance

### Early termination per rule
**Effort:** S. **Source:** plan.md §8.4.
For rules that only `Diagnose` (no `Emit`, no `Rewrite`), once a
match has been recorded the matcher could short-circuit further
exploration of the same anchor's subtree. Minor speedup; relevant
mostly for rules that match deeply-nested anchors.

### Index incremental update during fixpoint
**Effort:** S. **Source:** node-indexing commit.
The node-type index is built once per parse. During fixpoint
iteration, the underlying tree doesn't change, so the index stays
valid. If we ever support incremental edits during a Run, the index
would need to update.

---

## Tooling / UX

### `pasta init` to scaffold a new analyzer
**Effort:** S.
A subcommand that creates `analyzers/<name>/<name>.cue` with a
minimal template, plus `testdata/` with a placeholder source and
golden file. Lowers the barrier to first analyzer.

### Better diagnostic output
**Effort:** S.
Today the CLI prints `path:line: message [rule]`. Adding column
ranges (we have byte ranges in Diagnostic.StartByte/EndByte) and
optionally a snippet of the offending source would match what
modern linters output.

### `pasta lint` over a project (vs. `pasta -fix` per file)
**Effort:** M.
A subcommand that walks the working tree, detects languages by
extension, runs every shipped analyzer applicable to each file, and
reports. Aggregating results into a JSON or SARIF format would
integrate with CI tools.

### Comment preservation polish
**Effort:** S. **Source:** iferr edge cases.
The current preserveComments logic floats comments in the deleted
range to before the inserted text. Edge cases (multi-line block
comments at unusual indents, comments spanning the deletion boundary)
are tested via iferr's testdata but the implementation could be
clearer about its assumptions.

### Test-marker `// want:+N` for line offset
**Effort:** done. Recently added, but worth noting because it solved a
recurring testdata problem (the rewrite removing the line that holds
the `// want` marker). See pkg/runner/runner.go.

---

## Speculative

### Lattice-model facts (cue.md §5)
See above under CUE leverage. Worth a prototype when an analysis
needs it.

### Tree-sitter queries as a backend
The pattern matcher is hand-written. Tree-sitter ships its own query
language with field constraints, predicates, and capture quantifiers.
Compiling pasta patterns to TS queries could eliminate a chunk of
matcher code, at the cost of being constrained to what TS queries
can express. Worth investigating if the matcher grows much more.
