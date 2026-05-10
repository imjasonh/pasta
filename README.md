# 🍝 pasta

`pasta` is a a polyglot static-analysis and structured edit tool.

Using `pasta`, you can express [AST](https://en.wikipedia.org/wiki/Abstract_syntax_tree) states that you want to flag to users. For example, an empty JS promise (`new Promise(() => {})`). Running `pasta js_empty_promise.cue index.js` would warn when this is found, highlighting this anti-pattern.

Rules can also define an automatix fix, in which case `pasta -fix` will make the edit directly.

`pasta` uses tree-sitter (specifically, [`gotreesitter`](https://github.com/odvcencio/gotreesitter)) for parsing, and [CUE](https://cuelang.org/) for rule schemas. It's heavily inspired by Go's `golang.org/x/tools/go/analysis`.

The intention of `pasta` is to be able to declaratively describe rules for ASTs in any supported language, and quickly and reproducibly flag and fix findings. You can hook this up to your editor and/or CI to automatically flag and potentially fix violations of the rules you've specified.

## Status

Rules are defined in CUE files loaded at runtime. The framework ships generic predicates (parameterized over grammar specifics)
so semantic checks like "no later use" and "no named-result clash" are expressed in CUE.

The repo includes some analyzers as runnable examples. Single-language rules use a `<lang>_` prefix;
cross-language rules (which match every grammar) have no prefix.

Rules with a ✏️ include an automatic rewrite for `-fix`.

**Cross-language**

| Path | What it does |
|---|---|
| [todo_format](./analyzers/todo_format/todo_format.cue)                       | Flag `TODO`/`FIXME`/`XXX`/`HACK` comments without an owner: `TODO(name): ...` |
| [hardcoded_credentials](./analyzers/hardcoded_credentials/hardcoded_credentials.cue) | String literals that look like AWS access keys, GitHub tokens, Slack tokens, or PEM private keys |
| [hardcoded_localhost](./analyzers/hardcoded_localhost/hardcoded_localhost.cue) | String literals containing `localhost` / `127.0.0.1` / `0.0.0.0` URLs |

**Go**

| Path | What it does |
|---|---|
| [go_iferr](./analyzers/go_iferr/go_iferr.cue) ✏️                              | Inline error assignment into the following `if err != nil` (port of [imjasonh/iferr-analyzer](https://github.com/imjasonh/iferr-analyzer); 20 positive + 18 negative test cases) |
| [go_negcmp](./analyzers/go_negcmp/go_negcmp.cue) ✏️                           | `!(a == b)` → `a != b`, `!(a != b)` → `a == b` |
| [go_errors_is_nil](./analyzers/go_errors_is_nil/go_errors_is_nil.cue) ✏️      | `errors.Is(err, nil)` → `err == nil` |
| [go_empty_else](./analyzers/go_empty_else/go_empty_else.cue) ✏️               | Drop `else { }` empty-else branches |
| [go_self_assignment](./analyzers/go_self_assignment/go_self_assignment.cue) ✏️ | Delete `x = x` self-assignments |
| [go_panic_empty](./analyzers/go_panic_empty/go_panic_empty.cue)               | Flag `panic("")` with empty message |
| [go_string_concat_empty](./analyzers/go_string_concat_empty/go_string_concat_empty.cue) ✏️ | Drop empty operand in `"" + x` / `x + ""` |
| [go_for_range_one_literal](./analyzers/go_for_range_one_literal/go_for_range_one_literal.cue) | Flag `for _, v := range []T{x} {...}` — equivalent to a plain assignment |
| [go_errcheck](./analyzers/go_errcheck/go_errcheck.cue) ✏️                     | Flag and rewrite `foo()` to `_ = foo()` when foo returns error (fact passing) |
| [go_deprecated_use](./analyzers/go_deprecated_use/go_deprecated_use.cue)      | Flag calls to functions whose doc comment contains `Deprecated:` (fact passing, works cross-file) |
| [go_unused_export](./analyzers/go_unused_export/go_unused_export.cue)         | Flag exported funcs that no file in the analysis group calls (cross-file fact passing) |
| [go_taint](./analyzers/go_taint/go_taint.cue)                                  | Track taint from `os.Getenv` through assignments to `exec.Command` (fact passing + fixpoint) |
| [go_api_migration](./analyzers/go_api_migration/go_api_migration.cue) ✏️       | Worked example: ship a `.cue` adapter for breaking API changes -- added trailing arg (`widget.Render(x)` → `widget.Render(x, nil)`) and rename (`widget.OldName` → `widget.NewName`) |

**Python**

| Path | What it does |
|---|---|
| [python_eq_none](./analyzers/python_eq_none/python_eq_none.cue) ✏️                 | `x == None` → `x is None`, `x != None` → `x is not None` (PEP 8 E711, both orientations) |
| [python_bare_except](./analyzers/python_bare_except/python_bare_except.cue) ✏️     | `except:` → `except Exception:` |
| [python_isinstance_singleton](./analyzers/python_isinstance_singleton/python_isinstance_singleton.cue) ✏️ | `isinstance(x, (T,))` → `isinstance(x, T)` |
| [python_dict_get_redundant_none](./analyzers/python_dict_get_redundant_none/python_dict_get_redundant_none.cue) ✏️ | `d.get(k, None)` → `d.get(k)` |
| [python_assert_tuple](./analyzers/python_assert_tuple/python_assert_tuple.cue) ✏️  | `assert (cond, msg)` → `assert cond, msg` (real footgun -- tuple is always truthy) |
| [python_explicit_object_base](./analyzers/python_explicit_object_base/python_explicit_object_base.cue) ✏️ | `class Foo(object):` → `class Foo:` (Py3 inherits from object implicitly) |
| [python_redundant_else_after_return](./analyzers/python_redundant_else_after_return/python_redundant_else_after_return.cue) | Flag `if c: return x; else: y` — outdent the else (pylint R1705) |
| [python_mutable_default](./analyzers/python_mutable_default/python_mutable_default.cue) | Flag mutable default args (`def f(x=[])`) |
| [python_deprecated_use](./analyzers/python_deprecated_use/python_deprecated_use.cue) | Flag calls to `@deprecated`-decorated functions (fact passing) |
| [python_taint](./analyzers/python_taint/python_taint.cue)                              | Track taint from `input()` through assignments to `eval`/`exec`/`system` (fact passing + fixpoint propagation) |
| [python_method_no_self](./analyzers/python_method_no_self/python_method_no_self.cue)   | Flag class methods missing `self`/`cls` as first parameter (uses `ancestor_is`) |

**Rust**

| Path | What it does |
|---|---|
| [rust_needless_bool](./analyzers/rust_needless_bool/rust_needless_bool.cue) ✏️ | `if cond { true } else { false }` → `cond`; `if cond { false } else { true }` → `!(cond)` (clippy `needless_bool`) |
| [rust_println_panic](./analyzers/rust_println_panic/rust_println_panic.cue) ✏️ | Drop redundant `println!()` immediately before `panic!()` |
| [rust_println_redundant_format](./analyzers/rust_println_redundant_format/rust_println_redundant_format.cue) ✏️ | `println!("{}", "hello")` → `println!("hello")` |
| [rust_dbg_macro](./analyzers/rust_dbg_macro/rust_dbg_macro.cue) ✏️             | Flag committed `dbg!()` invocations and rewrite `dbg!(expr)` to `expr` |
| [rust_deprecated_use](./analyzers/rust_deprecated_use/rust_deprecated_use.cue) | Flag calls to `#[deprecated]` functions (fact passing) |
| [rust_taint](./analyzers/rust_taint/rust_taint.cue)                            | Track taint from `env::var()` through let bindings to `Command::new` (fact passing + fixpoint) |

**JavaScript**

| Path | What it does |
|---|---|
| [js_object_assign_spread](./analyzers/js_object_assign_spread/js_object_assign_spread.cue) ✏️ | `Object.assign({}, x)` → `{...x}` |
| [js_array_concat_spread](./analyzers/js_array_concat_spread/js_array_concat_spread.cue) ✏️   | `[].concat(x)` → `[...x]` |
| [js_template_no_subst](./analyzers/js_template_no_subst/js_template_no_subst.cue) ✏️         | `` `abc` `` → `'abc'` when no interpolation |
| [js_double_equals](./analyzers/js_double_equals/js_double_equals.cue) ✏️                     | `==` / `!=` → `===` / `!==` (no implicit type coercion) |
| [js_var_to_let](./analyzers/js_var_to_let/js_var_to_let.cue) ✏️                              | `var x` → `let x` (block-scoped, no hoisting) |
| [js_empty_promise](./analyzers/js_empty_promise/js_empty_promise.cue)                        | Flag `new Promise(() => {})` with empty executor |
| [js_taint](./analyzers/js_taint/js_taint.cue)                                                | Track taint from `req.query` / `req.body` / `req.params` to `eval` / `Function` (fact passing + fixpoint) |

**TypeScript**

| Path | What it does |
|---|---|
| [ts_array_type_style](./analyzers/ts_array_type_style/ts_array_type_style.cue) ✏️ | `Array<T>` → `T[]` |
| [ts_any_type](./analyzers/ts_any_type/ts_any_type.cue)                            | Flag `: any` annotations (defeat TypeScript's type checking) |

**YAML**

| Path | What it does |
|---|---|
| [yaml_truthy](./analyzers/yaml_truthy/yaml_truthy.cue) ✏️             | `Yes`/`On`/`True`/etc. → `true`; `No`/`Off`/`False`/etc. → `false` |
| [yaml_empty_value](./analyzers/yaml_empty_value/yaml_empty_value.cue) | Flag keys with no value (parses as null) |

**Bash**

| Path | What it does |
|---|---|
| [bash_eval_use](./analyzers/bash_eval_use/bash_eval_use.cue) | Flag `eval` invocations (code-injection hazard) |

**C**

| Path | What it does |
|---|---|
| [c_gets_unsafe](./analyzers/c_gets_unsafe/c_gets_unsafe.cue) | Flag `gets()` (CWE-242, removed in C11) — use `fgets()` |

**C++**

| Path | What it does |
|---|---|
| [cpp_using_namespace_std](./analyzers/cpp_using_namespace_std/cpp_using_namespace_std.cue) | Flag `using namespace std;` (pollutes the global namespace) |

**Java**

| Path | What it does |
|---|---|
| [java_string_equals_literal](./analyzers/java_string_equals_literal/java_string_equals_literal.cue) ✏️ | `x.equals("foo")` → `"foo".equals(x)` (NPE-safe) |
| [java_finalizer](./analyzers/java_finalizer/java_finalizer.cue) | Flag `protected void finalize()` overrides (deprecated since Java 9) |

**Swift**

| Path | What it does |
|---|---|
| [swift_force_unwrap](./analyzers/swift_force_unwrap/swift_force_unwrap.cue) | Flag `x!` force-unwrap operator (crashes on nil) |

**Ruby**

| Path | What it does |
|---|---|
| [ruby_unless_else](./analyzers/ruby_unless_else/ruby_unless_else.cue) | Flag `unless ... else ... end` — invert to `if` and swap branches |

**PHP**

| Path | What it does |
|---|---|
| [php_loose_equality](./analyzers/php_loose_equality/php_loose_equality.cue) ✏️ | `==` / `!=` → `===` / `!==` (no type coercion) |

**SQL**

| Path | What it does |
|---|---|
| [sql_select_star](./analyzers/sql_select_star/sql_select_star.cue) | Flag `SELECT *` (fragile under schema changes) |

**Dockerfile**

| Path | What it does |
|---|---|
| [dockerfile_latest_tag](./analyzers/dockerfile_latest_tag/dockerfile_latest_tag.cue) | Flag `FROM image:latest` and implicit-latest `FROM image` |
| [dockerfile_apt_no_recommends](./analyzers/dockerfile_apt_no_recommends/dockerfile_apt_no_recommends.cue) | Flag `apt-get install` without `--no-install-recommends` |

**HTML**

| Path | What it does |
|---|---|
| [html_deprecated_tags](./analyzers/html_deprecated_tags/html_deprecated_tags.cue) | Flag `<center>`, `<font>`, `<marquee>`, `<blink>`, `<strike>`, `<big>`, `<tt>` |

**CSS**

| Path | What it does |
|---|---|
| [css_zero_unit](./analyzers/css_zero_unit/css_zero_unit.cue) ✏️ | Drop unit on zero (`0px` → `0`) — the unit is meaningless |

## Use

```
go install github.com/imjasonh/pasta/cmd/pasta@latest

# Project-style: drop your rules in ./.pasta/ and just run pasta.
# Rules are loaded from ./.pasta/, sources default to ./...
mkdir -p .pasta && cp path/to/some-rule.cue .pasta/
pasta              # report
pasta -fix         # apply fixes

# Same, but pointing at a different rule directory.
pasta -rules path/to/rule-dir
pasta -rules path/to/rule-dir ./...
pasta -fix -rules path/to/rule-dir file.go

# Single-rule shortcut: first positional arg is a .cue file.
pasta path/to/rule.cue file.go [file.go ...]
pasta path/to/rule.cue ./...
pasta -fix path/to/rule.cue ./...

# `./...` recurses from the current dir; `pkg/...` is `pkg/` and below.
# Files whose extension doesn't map to a registered language are skipped.
# `.git`, `vendor`, `node_modules`, and `.pasta` are skipped by default;
# use `-skip` with a comma-separated list to add more (e.g. `dist,build`).
pasta -skip dist,build ./...

# Run rules in a directory against its testdata/. Defaults to ./.pasta/.
pasta test
pasta test path/to/rule-dir

# Fetch any remote rule modules declared in <rule-dir>/pasta.cue and
# write a pasta.lock with resolved commit SHAs (network access).
# Defaults to ./.pasta/.
pasta sync
pasta sync path/to/rule-dir
```

When more than one source file is supplied (directly or via `./...`
expansion) `pasta` analyzes them as a **single group**: a fact store is
shared across the files, so cross-file analyzers like
[`go_unused_export`](./analyzers/go_unused_export/go_unused_export.cue)
or [`go_deprecated_use`](./analyzers/go_deprecated_use/go_deprecated_use.cue)
can answer "is this name called anywhere in this codebase?" in one
invocation. A single source path runs as a one-file group (fresh fact
store), matching the historical behavior.

A rule directory has shape:

```
my-rule/
  my-rule.cue
  testdata/
    foo.go                  # top-level files: each is its own
    foo.go.golden           # one-file group (fresh fact store)
    bar.py
    bar.py.golden
    cross_pkg/              # subdirectory: ONE multi-file group with
      api.go                # a shared fact store across its files
      caller.go             # (recursive)
      caller.go.golden
```

`pasta test` discovers `*.cue` rules in the directory, walks `testdata/`
for source files in any registered language, runs the rules, and verifies:

1. Every diagnostic emitted by a rule matches a `// want "regex"` marker
   on the same line of the source. `// want:+N "regex"` shifts the
   expected line by N (useful when the rewrite itself deletes the
   marker line).
2. Every `// want` marker is satisfied by exactly one diagnostic.
3. If a `<file>.golden` exists, the `-fix` output matches it byte-for-byte.

Files directly under `testdata/` are run as independent single-file
groups. Each subdirectory of `testdata/` is run as one multi-file
group sharing a fact store — use subdirs to test cross-file analyzers
with realistic multi-file inputs.

## Remote rule imports

Rule directories can pull in rule modules published in other
repositories. Declare them in a `pasta.cue` manifest at the rule
directory root (typically `./.pasta/pasta.cue`):

```cue
// .pasta/pasta.cue
imports: {
    "github.com/alice/lint-rules": "v1.2.3"
}
```

The next `pasta` run resolves the version, fetches the module,
and writes `./.pasta/pasta.lock` pinning the commit SHA — sync is
implicit. Subsequent runs are offline as long as the lockfile is
in sync with the manifest. `pasta sync` still exists if you want
to refresh a moving ref (branch / tag) eagerly, and
`pasta sync --check` reports drift without writing files for CI
gating.

**Every top-level analyzer the module exports is auto-enrolled**, so
listing the module is enough to start running its rules — no
per-rule stub in `.pasta/` needed. A `.pasta/` containing only a
manifest is valid; its rules come entirely from the imports.

```
my-project/
  .pasta/
    pasta.cue       # imports: { "github.com/alice/lint-rules": "v1.2.3" }
    pasta.lock      # written by `pasta sync`
  src/...
```

`pasta` (or `pasta -fix`) from the project root then runs alice's
rules over `./...`.

If you want to override a rule from a remote module, drop a local
analyzer with the same name into `.pasta/` — the local version
wins, and pasta prints a warning to stderr so the suppression is
visible. Two remote modules exporting an analyzer with the same
name is an error (resolve by renaming, dropping one of the
imports, or shadowing both with a local rule).

Rule files in remote modules can also be `import`ed by name from
your local `.cue` files when you want to compose rather than just
auto-enroll:

```cue
import "github.com/alice/lint-rules/python_taint"
```

Modules are cached under `$XDG_CACHE_HOME/pasta/modules/` and keyed
by commit, so re-tagging upstream after a sync can't silently change
what your rules see — `pasta` re-uses the locked SHA until you run
`pasta sync` again. The cache is hash-verified on every load: if the
cached files no longer match the lockfile's recorded digest, pasta
refuses to load and tells you which dir to remove.

Publishing a rule module is just `git push` plus `git tag`: any
public repo whose `https://<path>.git` URL `git ls-remote` can
resolve will work. Versions are git refs (tags, branches, or full
SHAs) — there's no semver resolution, and a remote module is not
allowed to declare its own remote imports (flat deps only in v1).

## Use case: shipping adapters for breaking changes

Library authors can use `pasta` rules as **codemods that travel with a
release**. When a breaking API change lands, ship a `.cue` file
alongside the version bump and downstream consumers can run
`pasta -fix upgrade_v1.2.3.cue ./...` to migrate their call sites mechanically.

The `.cue` file expresses the rewrite once, in a tree-aware way, and
runs against any caller's source -- no separate per-codebase script,
and no need for the library author to publish (or each consumer to
write) a one-off migrator.

[`analyzers/go_api_migration`](./analyzers/go_api_migration/go_api_migration.cue)
is a working example covering two of the most common shapes:

- **Added trailing argument.** v1.2.3 of a fictional `widget` library
  added a trailing `opts *Options` parameter to `widget.Render`. The
  rule matches *only* the pre-migration single-arg call shape (using
  `named_child_count`), rewrites `widget.Render(x)` to
  `widget.Render(x, nil)`, and is a no-op once a codebase has been
  migrated, so re-running it is safe.

- **Rename.** v1.3.0 renamed `widget.OldName` to `widget.NewName`.
  The rule matches the selector expression itself (not the call), so
  it rewrites both `widget.OldName` value references and
  `widget.OldName(...)` calls in one pass.

Each rule emits a diagnostic *and* a rewrite. Without `-fix`, `pasta`
behaves as a CI lint pointing at unmigrated call sites (`go build` is also a hint); with `-fix` it
edits them in place. The same pattern extends naturally to:

- Removed arguments (`delete_from`/`delete_to` between captures).
- Argument reorder (capture each arg, reassemble in the new order).
- Removed APIs that need a hand-written replacement (emit a
  diagnostic only -- leave the rewrite off so a human handles it).

The full test, with positive and negative cases (different package,
different method, already-migrated arity), lives in
[`analyzers/go_api_migration/testdata/a.go`](./analyzers/go_api_migration/testdata/a.go)
and its `.golden` counterpart.

## LSP

The repo also has an [LSP](https://en.wikipedia.org/wiki/Language_Server_Protocol) server, `pastals`. The [`.editors/`](./editors/) directory has instructions about setting this up for your IDE; I've only tested it with Zed.

If you specify rules in your repo at `pasta.cue` or `.pasta/**/*.cue`, these rules will be loaded and evaluated.

-----

Working in this repo? See [CLAUDE.md](./CLAUDE.md) for layout, how
to add a new analyzer or language, and conventions worth knowing.

See [cue.md](./cue.md) for the case for CUE as the rule schema, and
[future-work.md](./future-work.md) for what's deliberately not yet done.
