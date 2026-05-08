# pasta

A polyglot static-analysis framework: tree-sitter for parsing, CUE for rule
schemas. Inspired by Go's `golang.org/x/tools/go/analysis`.

See [plan.md](./plan.md) for the design rationale and DSL specification.

## Status

Rules are CUE files loaded at runtime — no Go code per analyzer. The
framework ships generic predicates (parameterized over grammar specifics)
so semantic checks like "no later use" and "no named-result clash" are
expressed in CUE, not Go.

Shipped analyzers — single-language rules use a `<lang>_` prefix;
cross-language rules (which match every grammar) have no prefix. ✏️
marks rules that include an automatic rewrite.

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

**Rust**

| Path | What it does |
|---|---|
| [rust_needless_bool](./analyzers/rust_needless_bool/rust_needless_bool.cue) ✏️ | `if cond { true } else { false }` → `cond`; `if cond { false } else { true }` → `!(cond)` (clippy `needless_bool`) |
| [rust_println_panic](./analyzers/rust_println_panic/rust_println_panic.cue) ✏️ | Drop redundant `println!()` immediately before `panic!()` |
| [rust_dbg_macro](./analyzers/rust_dbg_macro/rust_dbg_macro.cue)                | Flag committed `dbg!()` invocations |
| [rust_deprecated_use](./analyzers/rust_deprecated_use/rust_deprecated_use.cue) | Flag calls to `#[deprecated]` functions (fact passing) |

**JavaScript**

| Path | What it does |
|---|---|
| [js_object_assign_spread](./analyzers/js_object_assign_spread/js_object_assign_spread.cue) ✏️ | `Object.assign({}, x)` → `{...x}` |
| [js_array_concat_spread](./analyzers/js_array_concat_spread/js_array_concat_spread.cue) ✏️   | `[].concat(x)` → `[...x]` |
| [js_template_no_subst](./analyzers/js_template_no_subst/js_template_no_subst.cue) ✏️         | `` `abc` `` → `'abc'` when no interpolation |
| [js_empty_promise](./analyzers/js_empty_promise/js_empty_promise.cue)                        | Flag `new Promise(() => {})` with empty executor |

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
   on the same line of the source.
2. Every `// want` marker is satisfied by exactly one diagnostic.
3. If a `<file>.golden` exists, the `-fix` output matches it byte-for-byte.

## Languages

Registered grammars (see `pkg/lang/lang.go`):

| Language | Extension | Comment node types |
|----------|-----------|--------------------|
| Go       | `.go`     | `comment` |
| Python   | `.py`     | `comment` |
| Rust     | `.rs`     | `line_comment`, `block_comment`, `doc_comment` |

Adding a language is a small data change in `pkg/lang/lang.go` — wire the
smacker tree-sitter grammar import and list its comment node types.

## Tests

```
go test ./...
```

The top-level `pasta_test.go` walks `analyzers/*/` and runs each via
`runner.TestDir`. Adding an analyzer is `mkdir analyzers/foo` + write
`foo.cue` and `testdata/`; no Go test wrapper required.

## Layout

| Path | What it is |
|------|------------|
| `schema/schema.cue`     | CUE schema for analyzer/rule/pattern types. |
| `pkg/dsl/`              | Go structs mirroring the schema. |
| `pkg/loader/`           | CUE → Go decoder, validates rules at load time. |
| `pkg/lang/`             | Language registry: extension → grammar + comment types. |
| `pkg/tsutil/`           | Tree-sitter Node abstraction. |
| `pkg/match/`            | Pattern matcher: node unions, fields, adjacent windows, preceding, predicates, pre-condition checks. |
| `pkg/effect/`           | Compiles edits to byte-range ops with @-interpolation and comment preservation. |
| `pkg/apply/`            | Applies ops to source bytes with conflict detection. |
| `pkg/engine/`           | Top-level orchestrator. |
| `pkg/runner/`           | Programmatic API used by both the CLI and Go tests. |
| `analyzers/iferr/`      | iferr analyzer (CUE + testdata). |
| `analyzers/negcmp/`     | negcmp analyzer (CUE + testdata). |
| `cmd/pasta/`            | CLI. |
| `pasta_test.go`         | One root test that exercises every directory under `analyzers/`. |
