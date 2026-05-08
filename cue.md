# Why CUE: Beyond Type-Safe Structs

How the polyglot analyzer framework exploits CUE's module system, constraint lattice, and computed validation to make rule authoring safe, composable, and extensible.

## The Surface-Level Win

CUE gives us type-safe struct definitions. Rules are validated against a schema at load time. Typos in field names are caught before the runtime runs. This alone is better than YAML or JSON, but it's table stakes — you could get this from JSON Schema or TypeScript types.

The real wins come from four CUE features that have no equivalent in configuration languages: the module system, constraint unification, computed validation, and conditional fields.

## 1. Modules: A Package Ecosystem for Analysis Rules

CUE has a real module system with versioned imports. This means the framework isn't just a tool — it's a platform with a package ecosystem.

### Module layout

```
analyzer.dev/schema           — core types (#Rule, #Pattern, #Analyzer)
analyzer.dev/patterns/go      — reusable Go pattern fragments
analyzer.dev/patterns/python  — reusable Python pattern fragments
analyzer.dev/patterns/rust    — reusable Rust pattern fragments
analyzer.dev/adapters/go      — Go adapter check declarations
analyzer.dev/adapters/rust    — Rust adapter check declarations
analyzer.dev/rules/errcheck   — the errcheck analyzer
analyzer.dev/rules/iferr      — the iferr analyzer
analyzer.dev/rules/taint      — the taint analyzer
```

### Pattern libraries

Language-specific pattern fragments become importable building blocks. Instead of every rule author re-specifying tree-sitter node structures from memory, they import validated, tested patterns:

```cue
// analyzer.dev/patterns/go

package go

import "analyzer.dev/schema"

// Match inside any Go statement-list container
#StmtList: schema.#Pattern & {
    node: ["block", "case_clause", "communication_case"]
}

// A nil comparison in either direction: x != nil or nil != x
#NilComparison: schema.#Pattern & {
    node: "binary_expression"
    fields: {
        left:     {capture: _}
        operator: {capture: _}
        right:    {capture: _}
    }
    where: [
        {op: "matches", args: [_, "==|!="]},
        {op: "nil_comparison", args: [_, _, _]},
    ]
}

// A function that returns an error type
#ErrorReturningFunc: schema.#Pattern & {
    node: "function_declaration"
    fields: {
        name: {capture: _}
        return_type: {
            capture: _
            pattern: {
                node: "type_identifier"
                where: [{op: "matches", args: [_, "error|Error"]}]
            }
        }
    }
}

// A short variable declaration
#ShortVarDecl: schema.#Pattern & {
    node: "short_var_declaration"
    fields: {
        left:  {capture: _}
        right: {capture: _}
    }
}

// A plain assignment (=, not += etc.)
#PlainAssign: schema.#Pattern & {
    node: "assignment_statement"
    fields: {
        left:     {capture: _}
        operator: {capture: _}
        right:    {capture: _}
    }
    where: [{op: "token_eq", args: [_, "="]}]
}
```

Rule authors then compose these fragments rather than building from scratch:

```cue
import (
    "analyzer.dev/schema"
    "analyzer.dev/patterns/go"
)

match: go.#StmtList & {
    adjacent: [
        {capture: "assign", pattern: go.#ShortVarDecl & {
            fields: {
                left:  {capture: "lhs_list"}
                right: {capture: "rhs"}
            }
        }},
        {capture: "if_stmt", pattern: {
            node: "if_statement"
            absent_fields: ["initializer"]
            fields: condition: go.#NilComparison & {
                // specialize captures for this rule
                fields: {
                    left:     {capture: "cond_x"}
                    operator: {capture: "cond_op"}
                    right:    {capture: "cond_y"}
                }
                where: [{op: "nil_comparison", args: [
                    "@cond_x", "@cond_y", "@lhs_list",
                ]}]
            }
        }},
    ]
}
```

The CUE evaluator unifies the imported `#NilComparison` with the rule's local specializations and verifies they're compatible. If the rule tries to add a field that conflicts with the pattern definition, it fails at load time.

### What this means for extensibility

Anyone can publish a CUE module. A security team publishes `securitycorp.com/patterns/sanitizers` with patterns for their internal sanitization functions. A platform team publishes `platform.io/rules/migrations` with rules that detect deprecated API usage across Go, Python, and TypeScript. Teams compose these with `import` and specialize them for their codebase — and CUE guarantees the composition is valid.

The barrier to writing a new analyzer drops from "understand tree-sitter node types, write correct patterns from scratch, hope you got the structure right" to "import the pattern library for your language, fill in the captures and predicates specific to your check."

## 2. Constraint Unification: Rules That Can't Be Wrong

CUE's defining feature is that values can only become *more specific*, never less. Every value is a constraint, and combining two values produces their greatest lower bound in the lattice. This has three practical consequences for the framework.

### Fact dependency validation

The schema can enforce that every fact a rule claims to `require` is actually `provide`d by some other rule in the analyzer, and that every `emit` references a fact in `provides`:

```cue
#Analyzer: {
    facts: {[string]: #Fact}
    rules: {[string]: #Rule}

    // Collect all fact kinds provided across all rules
    _provided: {
        for rname, r in rules
        for f in r.provides {
            (f): true
        }
    }

    // Collect all fact kinds required across all rules
    _required: {
        for rname, r in rules
        for f in r.requires {
            (f): true
        }
    }

    // Every required fact must either be provided internally
    // or declared as an external dependency
    external_requires?: [...string]
    _external_set: {
        for f in external_requires if external_requires != _|_ {
            (f): true
        }
    }

    // Validation: unsatisfied requirements fail evaluation
    _unsatisfied: {
        for f, _ in _required
        if _provided[f] == _|_ && _external_set[f] == _|_ {
            (f): "ERROR: fact '\(f)' is required but never provided"
        }
    }

    // Every emit must reference a fact in provides
    rules: {[string]: #Rule & {
        if emit != _|_ {
            _valid_emit: true
            emit: [...{
                // fact must be one of the rule's provides
                _fact_check: {
                    for p in provides {
                        if p == fact { _ok: true }
                    }
                }
            }]
        }
    }}
}
```

If you write a rule that requires `"tainted"` but no rule provides it:

```
// CUE evaluation error:
// _unsatisfied.tainted: "ERROR: fact 'tainted' is required but never provided"
```

This catches a class of bug that would otherwise be a silent runtime failure — the rule matches, tries to check `has_fact`, finds nothing, and silently skips every match. With CUE constraints, it's a load-time error.

### Precondition validation against adapters

The adapter declares which checks it supports. CUE can verify that rules only reference available checks:

```cue
// analyzer.dev/adapters/go

package go

import "analyzer.dev/schema"

adapter: schema.#Adapter & {
    language: "go"
    checks: [
        "not_used_after",
        "not_named_result",
        "all_idents_lhs",
        "matching_var_decl",
        "same_object",
    ]
}
```

```cue
// In the schema, when loading rules targeting Go:

#Rule: {
    languages: [...string]
    pre_conditions?: [...#Precondition]

    // If this rule targets Go, every non-optional precondition
    // must reference a check the Go adapter provides
    _go_targeted: or([for l in languages {l == "go"}])
    if _go_targeted {
        pre_conditions: [...{
            optional: true
        } | {
            check: or(go.adapter.checks)
        }]
    }
}
```

Now if someone writes `check: "not_use_after"` (typo), CUE catches it:

```
// CUE evaluation error:
// pre_conditions.0.check: "not_use_after" not allowed by
//   or("not_used_after","not_named_result","all_idents_lhs",
//      "matching_var_decl","same_object")
```

### Rule inheritance that can't break invariants

CUE's unification means you can define a base rule and specialize it, and CUE enforces that the specialization is compatible:

```cue
// Base pattern for "assign then if" in Go
#AssignThenIf: #Rule & {
    languages: ["go"]
    requires:  []
    provides:  []

    rewrite_opts: {
        preserve_comments: true
        preserve_indent:   true
    }

    diagnose: {
        message:  "can inline assignment into if statement"
        severity: "warning"
    }

    match: {
        node: ["block", "case_clause", "communication_case"]
    }
}

// := specialization — adds pattern and preconditions
inline_define: #AssignThenIf & {
    name: "inline_define"
    doc:  "Inline := assignment into if condition"

    match: adjacent: [
        {capture: "assign", pattern: {node: "short_var_declaration", ...}},
        ...
    ]

    pre_conditions: [{
        check: "not_used_after"
        args: {vars: "@lhs_list", after: "@if_stmt", scope: "block"}
    }]
}

// = specialization — same base, more preconditions
inline_assign: #AssignThenIf & {
    name: "inline_assign"
    doc:  "Inline = assignment into if condition, converting to :="

    match: adjacent: [
        {capture: "assign", pattern: {node: "assignment_statement", ...}},
        ...
    ]

    pre_conditions: [
        {check: "all_idents_lhs", args: {lhs: "@lhs_list"}},
        {check: "not_used_after", args: {vars: "@lhs_list", after: "@if_stmt", scope: "function", ...}},
        {check: "not_named_result", args: {vars: "@lhs_list"}},
        {check: "matching_var_decl", args: {decl: "@var_decl", vars: "@lhs_list"}, optional: true},
    ]
}
```

If someone tries to specialize `#AssignThenIf` and accidentally changes `severity` to an invalid value or removes a required field, CUE fails. The base definition acts as a contract that all specializations must satisfy. This is fundamentally different from YAML anchors or JSON `$ref` — those are copy-paste mechanisms with no validation. CUE's unification is a logical operation that guarantees consistency.

### What this means for extensibility

Third-party rule authors get guardrails for free. They can't misspell fact names. They can't reference adapter checks that don't exist. They can't break invariants established by base patterns. The framework validates rule correctness at load time — the same way a type checker validates code at compile time — which means the runtime can trust that every rule it receives is structurally sound.

## 3. Computed Validation: Derived Metadata You Never Maintain

CUE can compute values from other values during evaluation. This eliminates entire categories of bookkeeping that rule authors would otherwise have to maintain by hand.

### Automatic dependency graphs

```cue
#Analyzer: {
    rules: {[string]: #Rule}

    // Automatically derived — never maintained by hand
    _dependency_edges: [
        for rname, r in rules
        for req in r.requires
        for pname, p in rules
        if list.Contains(p.provides, req) {
            {from: pname, to: rname, fact: req}
        }
    ]

    // Detect cycles (rules that require their own provides)
    _fixpoint_rules: [
        for rname, r in rules
        for f in r.provides
        if list.Contains(r.requires, f) {
            {rule: rname, fact: f}
        }
    ]

    // Auto-generated documentation
    _rule_index: {
        for rname, r in rules {
            (rname): {
                doc:       r.doc
                languages: r.languages
                requires:  r.requires
                provides:  r.provides
                has_fix:   r.rewrite != _|_
            }
        }
    }
}
```

The runtime doesn't need to compute the dependency graph — CUE already has it. The `_fixpoint_rules` list tells the scheduler which rules need iteration. The `_rule_index` can be exported as documentation. None of this requires any code in the runtime; it's all declarative computation in the schema.

### Auto-generated help URLs and diagnostic metadata

```cue
#Rule: {
    name: string

    // Auto-derived from rule name
    _help_url: "https://analyzer.dev/rules/\(name)"

    diagnose?: #Diagnostic & {
        // Inject URL into every diagnostic automatically
        url: _help_url
    }
}
```

Every diagnostic gets a documentation URL without the rule author doing anything. The URL is derived from the rule name, validated by CUE's string interpolation, and injected via unification.

### Capture validation

CUE can verify that `@capture` references in rewrites and diagnostics actually correspond to captures defined in the match pattern:

```cue
#Rule: {
    match: #Pattern

    // Recursively collect all capture names from the pattern
    _captures: _collectCaptures(match)

    // Every @reference in rewrite templates must be a valid capture
    if rewrite != _|_ {
        rewrite: {
            if edits != _|_ {
                edits: [...{
                    if replacement != _|_ {
                        // validate @references in replacement
                    }
                    if text != _|_ {
                        // validate @references in text
                    }
                }]
            }
        }
    }
}
```

While CUE can't do arbitrary string parsing (you'd need the runtime to fully validate `@capture` interpolations), it can validate the structural references — that `target`, `anchor`, `delete_from`, `delete_to`, and `within` in edit operations reference capture names that exist in the pattern.

### What this means for extensibility

Rule authors focus on the semantics of their analysis — what to match, what to check, what to rewrite. The bookkeeping — dependency graphs, documentation, URL generation, structural validation — is derived automatically. When a rule changes, everything derived from it updates consistently. There's no manual synchronization to get wrong.

## 4. Conditional Fields: Language-Aware Polymorphism

CUE's conditional fields let you write definitions that adapt based on context. This enables patterns that work across languages with language-specific behavior:

```cue
#ErrorType: {
    _lang: string

    // The pattern adapts to the language's error representation
    pattern: {
        if _lang == "go" {
            node: "type_identifier"
            where: [{op: "matches", args: ["@_", "error"]}]
        }
        if _lang == "rust" {
            node: "generic_type"
            fields: type: {
                node: "type_identifier"
                where: [{op: "matches", args: ["@_", "Result"]}]
            }
        }
        if _lang == "typescript" {
            node: "type_reference"
            where: [{op: "matches", args: ["@_", "Promise"]}]
        }
    }
}
```

This lets you write a single `errcheck` analyzer definition that expands into language-specific patterns at load time:

```cue
errcheck: #Analyzer & {
    rules: {
        for lang in ["go", "rust", "typescript"] {
            "mark_errors_\(lang)": #Rule & {
                name:      "mark_errors_\(lang)"
                languages: [lang]

                match: {
                    node: "function_declaration"
                    fields: return_type: (#ErrorType & {_lang: lang}).pattern
                }
            }
        }
    }
}
```

One definition, three language-specific rules, all validated at load time.

### What this means for extensibility

Adding a new language to an existing analyzer is often just adding a new branch to a conditional and a new entry in a comprehension. The structural validation ensures the new branch produces a valid pattern. You don't need to fork the entire rule — you extend it.

## 5. The Lattice Model for Facts (Speculative)

This is more speculative, but potentially the deepest win. CUE's value lattice has a property that maps directly to program analysis theory: values can only become more specific (more constrained), never less. This is exactly the monotonicity requirement for sound dataflow analysis.

Consider taint tracking. Currently, facts are opaque blobs that the runtime stores and retrieves. But if facts were CUE values, the framework could *unify* facts rather than overwriting them:

```cue
// Two taint facts on the same node
fact1: {source: "user_input", labels: ["xss"]}
fact2: {source: "user_input", labels: ["sqli"]}

// CUE unification produces:
unified: {source: "user_input", labels: ["xss", "sqli"]}
```

This gives you lattice-based analysis for free. The fact store becomes a CUE value that grows monotonically. The fixpoint check is just "did the CUE value change after this iteration?" The soundness guarantee comes from CUE's type system rather than from careful coding in the runtime.

This is worth prototyping but isn't necessary for v1. It's the kind of thing that becomes important when people start writing sophisticated interprocedural analyses.

## 6. Practical Implications for Tool Architecture

### What CUE handles (load time)

- Schema validation of all rules
- Fact dependency graph construction and cycle detection
- Precondition check validation against adapter declarations
- Rule inheritance validation (base + specialization compatibility)
- Module resolution and import validation
- Diagnostic metadata derivation (URLs, documentation)
- Pattern library composition validation

### What the runtime handles (execution time)

- Tree-sitter parsing
- Pattern matching against CSTs
- Fact store management
- Precondition dispatch to adapters
- Edit computation and application
- Fixpoint iteration scheduling

### The boundary

CUE guarantees that every rule the runtime receives is structurally sound: its fact references resolve, its preconditions are valid, its patterns compose correctly, and its rewrites reference real captures. The runtime can skip all defensive validation and focus on execution.

This is the same relationship a type checker has to a runtime: the type checker proves properties statically so the runtime doesn't have to check them dynamically. CUE is, in effect, the type checker for the analyzer framework.

## 7. Summary: Why Not Just YAML

| Capability | YAML/JSON | JSON Schema | CUE |
|---|---|---|---|
| Typed field definitions | No | Yes | Yes |
| Import/module system | No | $ref (fragile) | Real packages with versioning |
| Cross-field validation | No | Limited | Full constraint system |
| Inheritance with validation | No | allOf (shallow) | Lattice unification (deep) |
| Computed derived fields | No | No | Yes |
| Conditional definitions | No | if/then (awkward) | Natural conditionals |
| Ecosystem/package registry | No | No | Yes (cue modules) |
| Composition guarantees | No | No | Lattice semantics |

YAML gives you data. JSON Schema gives you validated data. CUE gives you a type system for your configuration — with modules, generics (definitions), inheritance (unification), and static analysis (constraints). For a framework whose entire purpose is static analysis, having its configuration language also do static analysis is not just convenient — it's architecturally coherent.
