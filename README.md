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
| [go_errcheck](./analyzers/go_errcheck/go_errcheck.cue) ✏️                     | Flag and rewrite `foo()` to `_ = foo()` when foo returns error (fact passing) |
| [go_deprecated_use](./analyzers/go_deprecated_use/go_deprecated_use.cue)      | Flag calls to functions whose doc comment contains `Deprecated:` (fact passing) |
| [go_taint](./analyzers/go_taint/go_taint.cue)                                  | Track taint from `os.Getenv` through assignments to `exec.Command` (fact passing + fixpoint) |
| [go_api_migration](./analyzers/go_api_migration/go_api_migration.cue) ✏️       | Worked example: ship a `.cue` adapter for breaking API changes — added trailing arg (`widget.Render(x)` → `widget.Render(x, nil)`) and rename (`widget.OldName` → `widget.NewName`) |

**Python**

| Path | What it does |
|---|---|
| [python_eq_none](./analyzers/python_eq_none/python_eq_none.cue) ✏️                 | `x == None` → `x is None`, `x != None` → `x is not None` (PEP 8 E711, both orientations) |
| [python_bare_except](./analyzers/python_bare_except/python_bare_except.cue) ✏️     | `except:` → `except Exception:` |
| [python_isinstance_singleton](./analyzers/python_isinstance_singleton/python_isinstance_singleton.cue) ✏️ | `isinstance(x, (T,))` → `isinstance(x, T)` |
| [python_dict_get_redundant_none](./analyzers/python_dict_get_redundant_none/python_dict_get_redundant_none.cue) ✏️ | `d.get(k, None)` → `d.get(k)` |
| [python_assert_tuple](./analyzers/python_assert_tuple/python_assert_tuple.cue) ✏️  | `assert (cond, msg)` → `assert cond, msg` (real footgun — tuple is always truthy) |
| [python_mutable_default](./analyzers/python_mutable_default/python_mutable_default.cue) | Flag mutable default args (`def f(x=[])`) |
| [python_deprecated_use](./analyzers/python_deprecated_use/python_deprecated_use.cue) | Flag calls to `@deprecated`-decorated functions (fact passing) |
| [python_taint](./analyzers/python_taint/python_taint.cue)                              | Track taint from `input()` through assignments to `eval`/`exec`/`system` (fact passing + fixpoint propagation) |
| [python_method_no_self](./analyzers/python_method_no_self/python_method_no_self.cue)   | Flag class methods missing `self`/`cls` as first parameter (uses `ancestor_is`) |

**Rust**

| Path | What it does |
|---|---|
| [rust_needless_bool](./analyzers/rust_needless_bool/rust_needless_bool.cue) ✏️ | `if cond { true } else { false }` → `cond`; `if cond { false } else { true }` → `!(cond)` (clippy `needless_bool`) |
| [rust_println_panic](./analyzers/rust_println_panic/rust_println_panic.cue) ✏️ | Drop redundant `println!()` immediately before `panic!()` |
| [rust_dbg_macro](./analyzers/rust_dbg_macro/rust_dbg_macro.cue) ✏️             | Flag committed `dbg!()` invocations and rewrite `dbg!(expr)` to `expr` |
| [rust_deprecated_use](./analyzers/rust_deprecated_use/rust_deprecated_use.cue) | Flag calls to `#[deprecated]` functions (fact passing) |
| [rust_taint](./analyzers/rust_taint/rust_taint.cue)                            | Track taint from `env::var()` through let bindings to `Command::new` (fact passing + fixpoint) |

**JavaScript**

| Path | What it does |
|---|---|
| [js_object_assign_spread](./analyzers/js_object_assign_spread/js_object_assign_spread.cue) ✏️ | `Object.assign({}, x)` → `{...x}` |
| [js_array_concat_spread](./analyzers/js_array_concat_spread/js_array_concat_spread.cue) ✏️   | `[].concat(x)` → `[...x]` |
| [js_template_no_subst](./analyzers/js_template_no_subst/js_template_no_subst.cue) ✏️         | `` `abc` `` → `'abc'` when no interpolation |
| [js_empty_promise](./analyzers/js_empty_promise/js_empty_promise.cue)                        | Flag `new Promise(() => {})` with empty executor |
| [js_taint](./analyzers/js_taint/js_taint.cue)                                                | Track taint from `req.query` / `req.body` / `req.params` to `eval` / `Function` (fact passing + fixpoint) |

**TypeScript**

| Path | What it does |
|---|---|
| [ts_array_type_style](./analyzers/ts_array_type_style/ts_array_type_style.cue) ✏️ | `Array<T>` → `T[]` |

**YAML**

| Path | What it does |
|---|---|
| [yaml_truthy](./analyzers/yaml_truthy/yaml_truthy.cue) ✏️             | `Yes`/`On`/`True`/etc. → `true`; `No`/`Off`/`False`/etc. → `false` |
| [yaml_empty_value](./analyzers/yaml_empty_value/yaml_empty_value.cue) | Flag keys with no value (parses as null) |

**Bash**

| Path | What it does |
|---|---|
| [bash_eval_use](./analyzers/bash_eval_use/bash_eval_use.cue) | Flag `eval` invocations (code-injection hazard) |

## Use

```
go install github.com/imjasonh/pasta/cmd/pasta@latest

# Run a rule against one or more sources.
pasta path/to/rule.cue file.go [file.go ...]

# Apply suggested fixes (writes to stdout).
pasta -fix path/to/rule.cue file.go

# Run all rules in a directory against its testdata/.
pasta test path/to/rule-dir
```

A rule directory has shape:

```
my-rule/
  my-rule.cue
  testdata/
    foo.go
    foo.go.golden    # optional: expected output after `-fix`
    bar.py
    bar.py.golden
```

`pasta test` discovers `*.cue` rules in the directory, walks `testdata/`
for source files in any registered language, runs the rules, and verifies:

1. Every diagnostic emitted by a rule matches a `// want "regex"` marker
   on the same line of the source. `// want:+N "regex"` shifts the
   expected line by N (useful when the rewrite itself deletes the
   marker line).
2. Every `// want` marker is satisfied by exactly one diagnostic.
3. If a `<file>.golden` exists, the `-fix` output matches it byte-for-byte.

## Use case: shipping breaking-change adapters

Library authors can use `pasta` rules as **codemods that travel with a
release**. When a breaking API change lands, ship a `.cue` file
alongside the version bump and downstream consumers can run
`pasta -fix` to migrate their call sites mechanically.

The `.cue` file expresses the rewrite once, in a tree-aware way, and
runs against any caller's source — no separate per-codebase script,
and no need for the library author to publish (or each consumer to
write) a one-off migrator.

[`analyzers/go_api_migration`](./analyzers/go_api_migration/go_api_migration.cue)
is a worked example covering two of the most common shapes:

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
behaves as a CI lint pointing at unmigrated call sites; with `-fix` it
edits them in place. The same pattern extends naturally to:

- Removed arguments (`delete_from`/`delete_to` between captures).
- Argument reorder (capture each arg, reassemble in the new order).
- Removed APIs that need a hand-written replacement (emit a
  diagnostic only — leave the rewrite off so a human handles it).

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
