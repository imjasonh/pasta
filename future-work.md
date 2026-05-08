# Future Work

This is the consolidated TODO for pasta. It pulls items from
[cue.md](./cue.md) (the CUE-leverage essay) that we haven't yet
implemented, plus gaps that surfaced while writing analyzers.

Each item has a short description, a rough effort estimate (S / M / L),
and a brief note on context where useful.

The framework today is plenty for many analyzers — 35 ship today across
7 languages, with fact-passing, fixpoint scheduling, and node-indexed
matching. Most of the items below are about reducing boilerplate,
catching errors earlier, or unlocking cross-file analysis.

## DSL extensions

### `*` and `+` quantifiers in `adjacent`
**Effort:** M.
The matcher already does backtracking for `?`. `*` (zero-or-more) and
`+` (one-or-more) follow the same shape but need extra bookkeeping for
greedy match-up-to-N then back off. Useful for "any number of leading
defer statements", "three or more consecutive slice appends".

### `type_is` predicate
**Effort:** L.
Checks that a captured expression's inferred TYPE matches a name. This
needs per-language type information — Go's `go/types`, TypeScript's
checker, etc. — which means either bringing back per-language adapters
or shelling out to language servers. The CUE rule layer is ready
(`type_is` is in the schema as a stub); the runtime side is the work.

### `all_match` meta-predicate
**Effort:** M.
Invokes a sub-predicate on every named child of a captured list:
`{op: "all_match", args: [@list, "matches", "regex"]}`. Lets a rule
say "every LHS expression is an identifier" without a custom predicate.
DSL change: `Predicate.args` would need to allow nested predicate
specs.

### `same_object` predicate
**Effort:** L.
Checks that two captured identifiers resolve to the SAME declaration
(not just same text). This is what `same_ident` should be when scope
matters — but it requires full symbol resolution, which is per-language
adapter territory. Today `same_ident` does text equality, which is
adequate within a single function but conflates shadowed names.

### `file_match` filtering
**Effort:** S.
The schema declares `file_match?: [...string]` on `#Rule` for filename
globs (e.g. `["*_test.go"]` to restrict a rule to test files). The
runtime currently ignores this field. Wiring it through the engine is
straightforward: skip rules whose `file_match` doesn't include the
current file's basename.

### `subtree_has_type` / `subtree_lacks_type`
**Effort:** S. **Surfaced by:** rust_unsafe_no_safety attempt.
A predicate `{op: "subtree_has", args: [@cap, "type"]}` that reports
whether a captured node's subtree contains any descendant of the
given type. Useful for "does this function body contain a `panic!()`".
We have helpers (`subtreeReferences`) — exposing them as a predicate is
straightforward.

### Slicing on `@capture` interpolation
**Effort:** S. **Surfaced by:** dbg-macro rewrite.
Today we have edit-level `trim_start` / `trim_end`. A more general
interpolation form `@cap[N:M]` would let any text reference a substring
of a capture. Mostly cosmetic; `trim_start`/`trim_end` already covers
the common case.

### N-arg variadic rewrites
**Effort:** M. **Surfaced by:** Object.assign / [].concat rules.
`{...x, ...y, ...z}` from `Object.assign({}, x, y, z)` requires the
rewrite to know the number of arguments at runtime. The current
`replacement: "{...@x, ...@y}"` syntax is fixed. Options: (a) loop
construct in interpolation; (b) per-arity rules; (c) a "join captures"
edit form.

### Quantifier on `preceding`
**Effort:** S. **Surfaced by:** rust_unsafe_no_safety attempt.
`preceding` already accepts `quantifier: "?"`. Combined with a
`prev_sibling_does_not_match` predicate, this could express
"unsafe block without preceding SAFETY comment" cleanly. Today the
match would be: preceding optional + WHERE NOT preceded by safety
comment — needs negative matching (which we do have via
`prev_sibling_matches` + `not_matches`).

---

## CUE leverage (cue.md)

### Pattern libraries (cue.md §1.2)
**Effort:** L (worth it).
Build out `github.com/imjasonh/pasta/patterns/<lang>/` — importable CUE
definitions for common shapes:

```cue
// github.com/imjasonh/pasta/patterns/go
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
**Effort:** S.
Schema-side comprehensions that fail compilation when a rule's
`requires:` includes a fact no rule `provides:`. cue.md sketches the
exact CUE for this. Catches typos in fact names at load time rather
than at runtime when no diagnostics fire.

### Load-time capture validation (cue.md §3)
**Effort:** M.
Walk the rule's match pattern, collect capture names. Walk every
`@name` reference in `replacement`, `text`, `message`, and predicate
args; verify each resolves to a defined capture. Today typos pass
through silently (the @-interpolation just emits the literal text).
Would have caught bugs hit while writing analyzers.

### Predicate-name validation (cue.md §2.2 adapted)
**Effort:** S.
The schema already enumerates valid `op` values for `#Predicate`, so
unknown op names are a CUE error. We could extend the same idea to
`pre_conditions[].check` and to predicate-arg shapes (e.g.
`ancestor_is` requires args[1] to look like a node-type list).

### Rule inheritance refactor (cue.md §2.3)
**Effort:** S.
Refactor iferr's two rules (`inline_define` and `inline_assign`) to
extend a shared `#IferrBase` definition with the common shape (node
union, diagnose message, rewrite opts). Demonstrates CUE unification.

### Conditional-field language polymorphism (cue.md §4)
**Effort:** M.
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
**Effort:** L.
Speculative. Facts as CUE values that unify monotonically rather than
overwriting. Convergence detection becomes "did the CUE value change?"
Worth prototyping when sophisticated dataflow analyses arrive.

### Auto-generated diagnostic metadata (cue.md §3)
**Effort:** S.
Derive a `_help_url` from each rule's name, inject into every
diagnostic. Add a `severity` default tied to rule category. Pure
CUE — no runtime change.

---

## Fact system

### Cross-file / cross-package facts
**Effort:** L.
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
**Effort:** M. **Surfaced by:** taint analyzers' caveat.
Today the fact store has a secondary by-name index that's
scope-blind: a name shadowed in another function still hits a fact
keyed by that name. The taint testdata works around this by using
unique variable names per function. Real precision needs facts keyed
by `(name, scope-id)` where scope-id is derived from the enclosing
function or block.

### Cross-analyzer fact namespacing
**Effort:** S.
Today fact `kinds` are flat strings across all loaded analyzers. Two
analyzers using `kind: "tainted"` for different purposes would
collide. A scoped form like `analyzerName.kind` would prevent it. Low
priority until two analyzers actually conflict.

### Fact `scope` field
**Effort:** M.
`#Fact` could grow a `scope: "node" | "file" | "package" | "module"`
field that controls how the runtime keys the fact. Most facts today
are implicitly node-scoped (with the by-name secondary index acting
as a poor file-scope). Explicit scope makes the storage strategy
declarative and ties into cross-file work.

---

## Rewrite escape hatches

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
   surprisingly powerful (string ops, comprehensions).

2. **Starlark snippets per rewrite.** Bigger surface area, escapes
   the CUE-only invariant. Significant scope and complicates
   security/sandboxing.

3. **Transform functions registered in Go.** Rules reference a
   transform by name; Go-side registry computes the new text. Lowest
   effort, breaks the "rules are pure CUE" promise. Useful escape
   hatch for rare cases (one or two transforms instead of a generic
   mechanism).

Pick when a real use case forces the choice.

---

## Performance

### Early termination per rule
**Effort:** S.
For rules that only `Diagnose` (no `Emit`, no `Rewrite`), once a
match has been recorded the matcher could short-circuit further
exploration of the same anchor's subtree. Minor speedup; relevant
mostly for rules that match deeply-nested anchors.

### Index incremental update during fixpoint
**Effort:** S.
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
**Effort:** S. **Surfaced by:** iferr edge cases.
The current preserveComments logic floats comments in the deleted
range to before the inserted text. Edge cases (multi-line block
comments at unusual indents, comments spanning the deletion boundary)
are tested via iferr's testdata but the implementation could be
clearer about its assumptions.

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
