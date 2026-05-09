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
We currently have no adjacent quantifiers — even `?` was removed in
favor of splitting into separate rules. Bringing them back (`*` for
zero-or-more, `+` for one-or-more) would be useful for "any number of
leading defer statements" or "three or more consecutive slice
appends", but only when those shapes actually need to be matched
inside one rule rather than via fact-passing.

### Type-aware predicates
**Effort:** L. **Surfaced by:** rust_to_string_when_already_string,
go_unused_assignment, ts_redundant_assertion, java_string_pool_compare,
js_var_to_let (TDZ-safety check).
Most "is this rewrite safe" rules need type information pasta doesn't
have. Examples: `is x a String or a &str?` (Rust), `is this expression
nullable?` (TypeScript), `does this `var` introduce a TDZ
incompatibility?` (JavaScript). Wiring this up means either bringing
back per-language adapters with real type checkers (Go's `go/types`,
TS compiler, etc.) or shelling out to language servers. CUE side
would expose `type_is`, `is_nullable`, `same_object` (resolve to the
same declaration, not just same text — closes a known soundness gap
in `same_ident`), and similar predicates.

### `all_match` meta-predicate
**Effort:** M.
Invokes a sub-predicate on every named child of a captured list:
`{op: "all_match", args: [@list, "matches", "regex"]}`. Lets a rule
say "every LHS expression is an identifier" without a custom check.
DSL change: `Predicate.args` would need to allow nested predicate
specs.

### File-extension / filename gating per rule
**Effort:** S. **Surfaced by:** cpp_using_namespace_std (only really
problematic in `.h`/`.hpp`), `*_test.go` rules, `*.spec.ts`-only
rules.
Today rules attach to a *language* (`languages: ["cpp"]`) but can't
narrow to specific filenames or extensions. Schema once had a
`file_match: [...string]` field — we removed it as dead, but this is
the recurring use case. Wiring it through the engine is
straightforward: skip rules whose glob doesn't include the current
file's basename.

### Sibling-absence predicates
**Effort:** S. **Surfaced by:** dockerfile_no_user (no `USER`
directive in the file), bash_use_set_e (script body lacks `set -e`
near top), python_missing_dunder_init (class lacks `__init__`).
`absent_fields` covers field-level absence; nothing covers
"this stmt list lacks a child of type X". A predicate like
`{op: "no_child_of_type", args: [@cap, "type"]}` (or
`{op: "subtree_lacks", args: [@cap, "type"]}`) would close it.

### Conditional / branching rewrites (`yaml_truthy` shape)
**Effort:** M. **Surfaced by:** yaml_truthy (would be a single rule if
edits could branch on captured text), regex-canonicalizer rules,
case-normalization rules.
Today the rewrite shape is fixed at rule definition. A `cases:` edit
form — pattern→replacement table evaluated at rewrite time — would
let one rule say "if @x matches /yes|on/i emit `true`, else emit
`false`". Less composable than two rules with disjoint matchers, but
much more expressive for normalize-to-canonical-form patterns.

### Sub-extraction from string captures
**Effort:** L. **Surfaced by:** python_string_format_to_fstring
(needs to pull `{}` placeholders out of a format-string capture),
printf-style rewrites in any language.
Captures are opaque text from pasta's perspective. Pulling structured
content out of a string-literal capture (a list of `%s`/`%d` tokens,
the inside of a `{...}`, etc.) would unlock a class of refactor rules
that today need per-language Go code.

### Indentation-aware rewrites (whitespace-sensitive languages)
**Effort:** M. **Surfaced by:** python_redundant_else_after_return
(can't auto-fix because the else-block needs dedenting).
For Python and other indentation-significant languages, byte-range
edits aren't enough — the matcher needs to understand the captured
node's indent level and emit the rewrite at the right level. A
helper that computes "how indented is @cap relative to its enclosing
block" plus an interpolation-time `dedent` operation would cover the
common cases.

### Auto-import on rewrites
**Effort:** M. **Surfaced by:** any rule that introduces a name
from a different package or module than the original.
If a rewrite uses `errors.As` and the file doesn't already import
`errors`, the import block needs an entry added. No edit form
expresses "ensure `import "x"` exists at the top of the file". A
declarative `requires_import: ["errors"]` field on `#Rewrite`, with
language-specific import-block walkers, would handle most cases.

### `subtree_has_type` / `subtree_lacks_type`
**Effort:** S. **Surfaced by:** rust_unsafe_no_safety attempt.
A predicate `{op: "subtree_has", args: [@cap, "type"]}` that reports
whether a captured node's subtree contains any descendant of the
given type. Useful for "does this function body contain a `panic!()`".
We have helpers (`subtreeReferences`) — exposing them as a predicate is
straightforward.

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

### Predicate-arg-shape validation
**Effort:** S.
The schema enumerates `#Predicate.op` and (now) `#Precondition.check`
as closed sets, and the loader walks captures to catch typos. The
remaining gap is per-op arg shape — e.g. `ancestor_is` requires
args[1] to be a list of strings; `matches` requires args[1] to be a
valid Go regex; `nil_comparison` needs exactly three capture refs.
The current validator's `predicateCaptureArgs` table half-encodes
this; promoting it to a fuller per-op contract would catch more
authoring mistakes at load time.

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
