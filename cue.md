# Why CUE

Pasta uses [CUE](https://cuelang.org/) as its rule schema and configuration
language. The choice gives us four things YAML or JSON Schema don't:

1. **Type-safe rule definitions** validated at load time. Typos in field
   names are caught before the runtime runs.

2. **A real module system with versioned imports.** Built-in language
   configs and user-defined extensions live in the same `pasta.dev`
   namespace. Anyone can publish a CUE module that adds a new language
   alias or a custom rule pack and import it from their own analyzer
   directory; pasta loads it via the same `cue/load` machinery.

3. **Constraint unification** rather than copy-paste. Definitions like
   `#Analyzer`, `#Pattern`, `#Rule` aren't just types — they're
   constraints, and a user's rule is the unification of their value with
   those constraints. Specializing a base rule via `#Base & {...}`
   guarantees the result satisfies the base.

4. **Computed values** from other values during evaluation. Dependency
   graphs, validation comprehensions, derived metadata can all live in
   the schema rather than the runtime.

| Capability | YAML/JSON | JSON Schema | CUE |
|---|---|---|---|
| Typed field definitions | No | Yes | Yes |
| Import / module system | No | $ref (fragile) | Real packages with versioning |
| Cross-field validation | No | Limited | Full constraint system |
| Inheritance with validation | No | allOf (shallow) | Lattice unification (deep) |
| Computed derived fields | No | No | Yes |
| Conditional definitions | No | if/then (awkward) | Natural conditionals |
| Composition guarantees | No | No | Lattice semantics |

For the framework's own self-respect: a tool whose entire purpose is
static analysis ought to have its configuration language do static
analysis too.

## What we currently use

- **Modules:** `pasta.dev/schema` (DSL types), `pasta.dev/lang/<name>`
  (language configs). Users can supply their own modules; see
  `testdata/notgo_alias/` for an example.
- **Type-safe rule loading:** every shipped rule is unified with
  `schema.#Analyzer`. Misspelled fields fail compilation.
- **Auto-discovered language definitions:** any `*.cue` file in a rule
  directory that has the shape of `#Language` is registered at
  startup, so a user can declare a new file extension or grammar alias
  inline next to their rules.

## What CUE could do that we don't yet exploit

Several CUE-specific wins remain unrealized — pattern libraries
(`pasta.dev/patterns/<lang>` with reusable fragments like
`#NilComparison`), load-time fact dependency validation, capture
validation, conditional-field language polymorphism, and a
lattice-style fact model. Each is tracked in
[future-work.md](./future-work.md) under the "CUE leverage" section.
