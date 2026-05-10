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

## Pre-commit hook

`scripts/pre-commit.sh` runs the same checks as CI (gofmt, go vet,
go build, go test). It MUST be installed as the local pre-commit
hook before any work in this repo. If `.git/hooks/pre-commit` is
missing or doesn't point at the script, install it before doing
anything else:

```
ln -sf ../../scripts/pre-commit.sh .git/hooks/pre-commit
```

Then verify with `ls -l .git/hooks/pre-commit`. The hook runs
automatically on every `git commit`.

NEVER skip the hook. Do not pass `--no-verify` to `git commit`. Do
not set `core.hooksPath` to bypass it. Do not edit / delete /
chmod -x the script to dodge a check. If a check fails, fix the
underlying issue; don't reach for the bypass.

If the hook is somehow not running (the developer cloned without
running setup), reinstall it via the symlink above before
committing — re-installing is always preferable to skipping.

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
| `internal/loader/`            | CUE loader. Embeds the built-in `github.com/imjasonh/pasta` module under `internal/loader/cuemod/`, and vendors any remote modules declared in the rule directory's `pasta.cue` into the same overlay. |
| `internal/loader/cuemod/`     | The embedded built-in CUE module: `schema/`, `lang/<name>/`, `patterns/<name>/`. |
| `internal/remote/`            | Remote rule imports: `pasta.cue` manifest + `pasta.lock` lockfile, git-based fetcher, on-disk cache under `$XDG_CACHE_HOME/pasta/modules/`. Flat deps only — a remote module declaring its own remote imports is rejected. |
| `internal/lang/`              | Runtime language registry. `grammars.go` is the only Go-side language code (maps grammar name → tree-sitter `GetLanguage`). |
| `internal/tsutil/`            | gotreesitter `Node` wrapper that carries source bytes + language + file-id, so callers don't have to thread them. |
| `internal/match/`             | Pattern matcher: node unions, fields, adjacent windows, preceding, predicates (positional), checks (named). |
| `internal/factstore/`         | Per-run fact store with dual indexing — by (kind, file-id, byte-range) and by (kind, identifier-text). The by-name index is file-agnostic so facts propagate across files in a multi-file group. |
| `internal/effect/`            | Compiles edits to byte-range ops, handles `@capture` interpolation, comment preservation, and `trim_start`/`trim_end`. |
| `internal/apply/`             | Applies ops to source bytes with conflict detection. |
| `internal/engine/`            | Top-level orchestrator. SCC scheduler with fixpoint groups for cyclic rule deps. `Run` is the single-file entry point; `RunGroup` runs a set of files with a shared fact store. |
| `internal/runner/`            | Programmatic API used by both the CLI and Go tests. `LoadRules`, `RunFile`, `RunGroup`, `TestDir`. |
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
    multi_pkg/          # optional: subdir = multi-file analysis group
      api.<ext>
      caller.<ext>
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
- Files **directly in `testdata/`** are run as independent
  single-file groups (each gets its own fresh fact store). Each
  **subdirectory of `testdata/`** is run as ONE multi-file group:
  every source file under that subdir (recursively) is analyzed with
  a shared fact store, so a fact emitted in one file is visible at
  query sites in the others. Use subdirs to test cross-file
  analyzers (see `analyzers/go_unused_export/` for an example).

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
- **By-name fact lookup is scope-blind AND file-blind.** The
  factstore's secondary index keys facts by identifier text only —
  no scope, no file. Two functions that both define `x` (in the
  same file or in different files of a multi-file group) will share
  facts on `x`. Scope-blindness has always been the case; the
  file-blindness is deliberate — it's what makes cross-file
  analysis work via `has_fact` / `not_has_fact`. Testdata uses
  unique names where bleed would corrupt the test; production
  precision needs scope-aware fact keys (see future-work.md).

## The `.pasta/` convention

Projects keep their rules in `./.pasta/` at the repo root. Bare
`pasta` / `pasta -fix` / `pasta sync` / `pasta test` all default to
this directory; pass `-rules <dir>` (or, for sync/test, an explicit
positional dir) to override. `.pasta` is added to the `./...` walk's
default skip list so the rules and their testdata aren't picked up
as project sources.

The single-rule shortcut still works: when the first positional arg
is an existing `.cue` file, `pasta rule.cue source...` loads that
one file as before.

## Remote imports

A rule directory can declare external rule modules in a `pasta.cue`
manifest at its root (typically `./.pasta/pasta.cue`):

```cue
imports: {
    "github.com/alice/lint-rules": "v1.2.3"
}
```

Sync is implicit on load. When LoadDir sees a manifest, it checks
`remote.IsInSync(manifest, lockfile)`; if the lockfile is missing
or out of sync (different version pinned, new entry added, removed
entry still present) it calls `remote.Sync` to resolve via
`git ls-remote`, fetch into
`$XDG_CACHE_HOME/pasta/modules/<path>@<commit>/`, and write a
fresh `pasta.lock`. The clean-match path is offline.

`pasta sync` is still a top-level command, but it's no longer
required for day-to-day use:

- `pasta sync [dir]` — eager refresh. Useful when a manifest pins
  a branch or moving tag and you want the latest commit pulled in
  now rather than on next run.
- `pasta sync --check [dir]` — non-destructive verification. Exits
  non-zero with a reason if the lockfile drifts from the manifest;
  doesn't write files. CI uses this to fail builds where someone
  edited the manifest without committing the regenerated lockfile.

Tamper detection (cached files no longer hash to the locked digest)
remains a hard error on every load — implicit sync doesn't paper
over it.

Two ways to consume a remote module:

1. **Auto-enroll**: by default, every top-level analyzer in every
   imported module is loaded as if it lived in `.pasta/`. A
   `.pasta/` containing only a manifest + lockfile is valid — its
   rules come entirely from the imports.

2. **Explicit import**: a local `.cue` file can also `import
   "github.com/alice/lint-rules/<subpath>"` and reference values
   from it (helpers, recipes, partial analyzers). Useful for
   composing rather than running verbatim.

Naming policy at merge time (loader.go:`mergeAnalyzers`):
- Local analyzer with the same name as a remote one → local wins,
  stderr warning. Lets projects patch a remote rule without forking.
- Two remote analyzers with the same name → hard error. No
  principled way to pick a winner.
- Remote modules can suppress auto-enrollment of helpers by making
  them definitions (`#Recipe` instead of `Recipe`) — `extractTopLevel`
  iterates with `cue.Definitions(false)`.

v1 limits: flat deps only (a remote module's own `pasta.cue` with
non-empty `imports` is a hard error during sync); the version string
is a literal git ref (tag / branch / full SHA), no semver
constraints; `git` is the fetcher. The Fetcher interface in
`internal/remote/remote.go` is the single seam — swap it later for
OCI / native CUE modules without changing the manifest format.

## Open work

[future-work.md](./future-work.md) tracks what's deliberately not yet
done — pattern libraries that exist but could grow, predicates left as
stubs, cross-file facts, scope-aware fact keys, and a handful of DSL
extensions that haven't proven necessary yet.
