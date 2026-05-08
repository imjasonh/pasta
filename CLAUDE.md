# Working in this repo

Notes for future coding agents (and humans). The user-facing pitch lives
in [README.md](./README.md); this file is the maintenance / navigation
side.

## Workflow rules

NEVER `git commit` or `git push` without Jason's explicit approval in
the current turn. "Make this change", "fix that", or "do these things"
is **not** approval to commit. He will say "commit", "commit and
push", or "ship it" when he wants a commit. Approval given earlier in
the conversation does **not** carry forward — every commit needs its
own go-ahead. When work is in a committable state, stop and report
it; let him decide whether to commit.

## Running everything

```
go test ./...
```

The top-level `pasta_test.go` walks `analyzers/*/` and `testdata/*/` and
runs each directory via `internal/runner.TestDir`. There are no per-analyzer
Go test files — `go test ./...` is the whole verification surface.

The CLI also has a `test` mode that does the same thing on a single
directory:

```
go run ./cmd/pasta test analyzers/go_iferr
```

Useful when you're iterating on one analyzer and want a tighter loop
than `go test`.

## Layout

| Path | What it is |
|------|------------|
| `internal/dsl/`               | Go structs mirroring the CUE schema. `dsl.Arg` is a sum of `string` and `[]string` for predicate args. |
| `internal/loader/`            | CUE loader. Embeds the built-in `github.com/imjasonh/pasta` module under `internal/loader/cuemod/`. |
| `internal/loader/cuemod/`     | The embedded built-in CUE module: `schema/`, `lang/<name>/`, `patterns/<name>/`. |
| `internal/lang/`              | Runtime language registry. `grammars.go` is the only Go-side language code (maps grammar name → tree-sitter `GetLanguage`). |
| `internal/tsutil/`            | gotreesitter `Node` wrapper that carries source bytes + language, so callers don't have to thread them. |
| `internal/match/`             | Pattern matcher: node unions, fields, adjacent windows, preceding, predicates (positional), checks (named). |
| `internal/factstore/`         | Per-run fact store with dual indexing — by (kind, byte-range) and by (kind, identifier-text). |
| `internal/effect/`            | Compiles edits to byte-range ops, handles `@capture` interpolation, comment preservation, and `trim_start`/`trim_end`. |
| `internal/apply/`             | Applies ops to source bytes with conflict detection. |
| `internal/engine/`            | Top-level orchestrator. SCC scheduler with fixpoint groups for cyclic rule deps. |
| `internal/runner/`            | Programmatic API used by both the CLI and Go tests. `LoadRules`, `RunFile`, `TestDir`. |
| `analyzers/<name>/`      | A shipped analyzer: a `<name>.cue` rule + `testdata/` (sources and `.golden` files). |
| `testdata/<name>/`       | Extension/integration demos (e.g. `notgo_alias` showing user-supplied language modules). |
| `cmd/pasta/`             | CLI. |
| `pasta_test.go`          | One root test that exercises every directory under `analyzers/` and `testdata/`. |

## Adding an analyzer

Mechanically: `mkdir analyzers/<name>/` and create:

```
analyzers/<name>/
  <name>.cue            # imports github.com/imjasonh/pasta/{schema,lang/...,patterns/...}
  testdata/
    a.<ext>             # source with `// want "regex"` markers
    a.<ext>.golden      # optional: expected output after -fix
```

Naming convention:
- Single-language rules: `<lang>_<name>` (e.g. `go_iferr`, `python_taint`).
- Cross-language rules: bare name (e.g. `todo_format`, `hardcoded_credentials`).

Test data:
- Each `// want "regex"` (or `# want`, `-- want`) anchors a diagnostic
  expectation on the same line.
- Use `// want:+1 "regex"` to anchor it on the next line — useful when
  a rewrite is going to delete the line that holds the marker.
- If a rule has a rewrite, ship a `.golden` showing the post-`-fix`
  source.

## Adding a language alias

Two cases:

1. **The grammar is already linked in.** Add a CUE file under
   `internal/loader/cuemod/lang/<name>/<name>.cue` declaring the
   `Config` value (grammar name, extensions, comment node types).
   No Go change required. See the existing `lang/go/`, `lang/python/`,
   etc. as templates.

2. **The grammar isn't linked in yet.** Add an entry to
   `internal/lang/grammars.go` mapping the new grammar name to its
   `gotreesitter` `GetLanguage` function, then do (1).

Users can also publish their own external CUE module that adds
languages — see `testdata/notgo_alias/` for a working example. The
rule directory's `*.cue` files can declare `#Language` values inline
and the runner registers them at startup.

## Conventions worth keeping in mind

- **Rules are pure CUE.** No per-analyzer Go code. All semantic checks
  go through the predicate / check registry in `internal/match/predicate.go`.
- **`pasta.dev` is not our domain.** The built-in module is published
  as `github.com/imjasonh/pasta`. All imports use that path.
- **Schema-first.** When adding a predicate or extending an edit form,
  update both `internal/loader/cuemod/schema/schema.cue` AND the
  corresponding Go side. Schema rejects unknown fields, so omissions
  surface as load errors.
- **Want markers and rewrites can clash.** If a rewrite deletes the
  line that holds a `// want` marker, use `// want:+N` on a different
  line. Common for `delete_from`-style rewrites.
- **Tree.Release() pools the arena.** gotreesitter recycles `Node`
  storage when the tree is released. Anything you cache from the tree
  (diagnostics, edit ops) must be self-contained — see how
  `effect.Diagnostic` snapshots byte ranges and the line number rather
  than holding a `Node` reference.
- **By-name fact lookup is scope-blind.** The factstore's secondary
  index keys facts by identifier text. Two functions that both define
  `x` will share facts on `x`. Acceptable for the single-file analyses
  shipped today; testdata uses unique variable names per function to
  avoid bleed.

## Open work

[future-work.md](./future-work.md) tracks what's deliberately not yet
done — pattern libraries that exist but could grow, predicates left as
stubs, cross-file facts, scope-aware fact keys, and a handful of DSL
extensions that haven't proven necessary yet.
