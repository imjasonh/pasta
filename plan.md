# Polyglot Analyzer Framework

A declarative DSL for cross-language AST analysis and transformation, inspired by Go's `analysis.Analyzer` framework.

## 1. Goal

Go's `golang.org/x/tools/go/analysis` package is one of the best-designed program analysis frameworks in any language. It makes three key things easy:

1. **Expressing patterns** — what AST shapes to look for
2. **Composing analyses** — rules declare facts they `Require` and `Provide`, and the framework handles scheduling
3. **Producing fixes** — diagnostics carry suggested edits that tools can apply automatically

But it only works for Go. The goal of this project is to generalize these ideas to work across many languages, using:

- **Tree-sitter** as the universal parser (100+ grammars, consistent CST model)
- **CUE** as the schema and constraint language (type-safe rules, validated at load time)
- **Per-language adapters** for semantic checks that require type information

The result is a framework where you write declarative rules in CUE that match tree-sitter patterns, emit typed facts, produce diagnostics, and suggest rewrites — and the same rule can target multiple languages where their tree-sitter grammars share node types.

## 2. Architecture

```
                    ┌──────────────────────────────┐
                    │         CUE Rule Files        │
                    │  (patterns, facts, rewrites)  │
                    └──────────────┬───────────────┘
                                   │ validated by
                    ┌──────────────▼───────────────┐
                    │       CUE Schema (schema.cue) │
                    │  #Pattern, #Rule, #Analyzer,  │
                    │  #Precondition, #Adapter       │
                    └──────────────┬───────────────┘
                                   │ loaded by
                    ┌──────────────▼───────────────┐
                    │         Runtime Engine         │
                    │                               │
                    │  1. Parse source (tree-sitter) │
                    │  2. Topo-sort rules by facts   │
                    │  3. Match patterns on CST      │
                    │  4. Check preconditions (adapter)│
                    │  5. Emit facts / diagnostics   │
                    │  6. Apply rewrites to source   │
                    └──────────────┬───────────────┘
                                   │ calls
                    ┌──────────────▼───────────────┐
                    │      Language Adapters         │
                    │  (Go, Rust, TS, Python, ...)  │
                    │                               │
                    │  Implement precondition checks │
                    │  like "not_used_after",        │
                    │  "not_named_result", etc.      │
                    └───────────────────────────────┘
```

### Pipeline

1. **Source → Tree-sitter parse** — produces a concrete syntax tree
2. **Load CUE rules** — validate against schema, resolve fact dependencies
3. **Topological sort** — order rules by `requires`/`provides` edges
4. **For each rule**: match pattern against CST nodes, run preconditions via adapter, fire effects (diagnose/rewrite/emit)
5. **Fixpoint iteration** — rules that both require and provide the same fact kind are re-run until no new facts are emitted
6. **Apply rewrites** — edit operations target byte ranges in the original source (no pretty-printing needed)

### Key design decisions

- **Byte-range edits, not AST transforms.** Rewrites target source text ranges obtained from tree-sitter node positions. This avoids the entire code-formatting problem and preserves comments, whitespace, and style.
- **Facts are typed and scoped.** Each fact has a CUE-validated schema. Facts are attached to AST nodes (keyed by byte range). The framework validates that `emit[].fact` references exist in `provides` and that `has_fact` predicates reference facts from `requires`.
- **Preconditions bridge declarative and imperative.** Purely structural checks live in the pattern language. Semantic checks (scope analysis, type resolution) are delegated to per-language adapters via `pre_conditions`. The rule stays declarative; the adapter is imperative but has a typed interface.
- **`node` accepts a union.** A single rule can match across multiple container node types (e.g. `node: "block" | "case_clause" | "communication_case"`) to avoid duplicating logic for every statement-list context.

## 3. Schema

### 3.1 Core Types

```cue
package analyzer

// ============================================================================
// Facts — typed data flowing between rules
// ============================================================================

#Fact: {
    kind:   string
    schema: _   // CUE constraints validate payloads at emit time
}

// ============================================================================
// Captures — bind AST nodes to names for use in predicates and rewrites
// ============================================================================

#Capture: {
    capture:     string           // bind name, referenced as @name
    pattern?:    #Pattern         // optionally constrain what matches
    quantifier?: "*" | "+" | "?"  // ?: optional, *: zero-or-more, +: one-or-more
}

// ============================================================================
// Predicates — constraints evaluated during pattern matching
// ============================================================================

#Predicate: {
    op: "eq" | "neq" | "matches"
      | "has_fact" | "not_has_fact"
      | "ancestor_is" | "type_is"
      | "field_absent"
      | "last_non_blank"
      | "nil_comparison"
      | "same_ident"
      | "token_eq"
      | "all_match"
      | "stmt_index_delta"
    args: [...string]
}
```

**Predicate reference:**

| Op | Args | Description |
|----|------|-------------|
| `eq` | `[@capture, "literal"]` | Capture's text equals literal |
| `neq` | `[@capture, "literal"]` | Capture's text does not equal literal |
| `matches` | `[@capture, "regex"]` | Capture's text matches regex |
| `has_fact` | `[@capture, "fact_kind"]` | Node has a fact of this kind attached |
| `not_has_fact` | `[@capture, "fact_kind"]` | Node does not have this fact |
| `ancestor_is` | `["node_type"]` | An ancestor of the current node has this type |
| `type_is` | `[@capture, "type"]` | Capture's inferred type matches (adapter-dependent) |
| `field_absent` | `["field_name"]` | The matched node does not have this named field |
| `last_non_blank` | `[@capture, @list]` | Capture is the last non-`_` identifier in a list |
| `nil_comparison` | `[@x, @y, @list]` | One of x/y is `nil`, the other is the last non-blank ident from the list |
| `same_ident` | `[@a, @b]` | Two captures have the same identifier name |
| `token_eq` | `[@capture, "literal"]` | The token value of a captured operator equals a literal |
| `all_match` | `[@list, "predicate", ...args]` | Every element of a captured list satisfies a sub-predicate |
| `stmt_index_delta` | `[@a, @b, "N"]` | Captures a and b are exactly N statements apart |

### 3.2 Pattern

```cue
#Pattern: {
    // Tree-sitter node type(s) to match.
    // String matches one type; list matches any of the types.
    // This avoids duplicating rules for every statement-list container.
    node: string | [...string]

    // Named children to match/capture. Keys are tree-sitter field names.
    fields?: {[string]: #Pattern | #Capture}

    // Match against anonymous children positionally.
    children?: [...#Pattern | #Capture]

    // Predicate constraints on the matched node.
    where?: [...#Predicate]

    // Match a sliding window of consecutive siblings within this
    // node's children. The runtime tries every (i, i+1, ..., i+N-1)
    // window across the child list.
    adjacent?: [...#Pattern | #Capture]

    // Match the statement immediately before this one in the same
    // statement list. Used for backward-looking patterns like
    // "absorb a preceding var declaration".
    preceding?: #Pattern | #Capture

    // Assert that named fields are absent from the node.
    // Solves patterns like "if must not have an init clause".
    absent_fields?: [...string]
}
```

**`node` as a union:** When `node` is a list, the pattern matches any node whose type is in the list. This is syntactic sugar — the runtime expands it into multiple match attempts. The primary use case is matching across statement-list containers:

```cue
match: {
    node: ["block", "case_clause", "communication_case"]
    adjacent: [...]
}
```

This replaces what would otherwise require three duplicate rules.

**Future extension — quantifiers within `adjacent`:** Each adjacent element is currently required and matches exactly one node. A future extension will honor `quantifier` (`?`, `*`, `+`) on adjacent elements, turning the sliding window into a small backtracking matcher. Useful for patterns like "any number of leading `defer` statements", "an optional cleanup call before the assign", or "three or more consecutive slice appends". Plan.md already declares `quantifier` on `#Capture` for this purpose; only `preceding` honors it today. To be added when a second analyzer needs it.

### 3.3 Diagnostics and Rewrites

```cue
#Severity: "error" | "warning" | "info" | "hint"

#Diagnostic: {
    message:  string          // may reference @captures
    severity: #Severity | *"warning"
}

#Edit:
    // Replace a captured region with new text
    {target: string, replacement: string} |
    // Insert text before or after a capture
    {position: "before" | "after", anchor: string, text: string} |
    // Delete everything from one capture to another (inclusive)
    {delete_from: string, delete_to: string} |
    // Replace a specific token within a captured node's source text
    {within: string, token: string, replace_with: string}

#Rewrite: {template: string} | {edits: [...#Edit]}

#RewriteOpts: {
    // When deleting a range, collect any comments within it and
    // float them above the replacement text.
    preserve_comments?: bool | *false

    // Maintain the indentation level of the original code.
    preserve_indent?: bool | *true
}
```

**Edit reference:**

| Edit type | Fields | Description |
|-----------|--------|-------------|
| Replace | `target`, `replacement` | Replace the byte range of a captured node with new text. `replacement` may contain `@capture` interpolations. |
| Insert | `position`, `anchor`, `text` | Insert text before or after a captured node. `text` may contain `@capture` interpolations. |
| Delete range | `delete_from`, `delete_to` | Delete everything from the start of one capture to the start of another. If `delete_from` references an uncaptured optional, falls back to the next available anchor in the adjacent sequence. |
| Token replace | `within`, `token`, `replace_with` | Find the first occurrence of a literal token in the source text of a captured node and replace it. Preserves all surrounding text including comments. |

### 3.4 Preconditions

```cue
#Precondition: {
    // The check name — must be provided by the language adapter.
    check: string

    // Arguments reference captures from the match pattern.
    args: {[string]: string}

    // If true and the check is not supported by the current adapter
    // (or a referenced capture was not bound), treat as "passed"
    // rather than "failed". Enables graceful degradation.
    optional?: bool | *false
}
```

Preconditions run **after** the pattern matches but **before** effects fire. If any non-optional precondition fails, the match is silently discarded.

**Standard precondition checks** (adapters should implement these where applicable):

| Check | Args | Description |
|-------|------|-------------|
| `not_used_after` | `vars`, `after`, `scope`, `exclude_range_start?`, `exclude_range_end?` | No variable in `vars` is referenced after `after`. `scope` is `"block"` (check remaining block statements) or `"function"` (check entire enclosing function, excluding the specified range). |
| `not_named_result` | `vars` | None of the variables is a named return parameter of the enclosing function. |
| `all_idents_lhs` | `lhs` | Every expression in the LHS list is a plain identifier (not a selector, index, etc.). |
| `matching_var_decl` | `decl`, `vars` | The captured declaration declares exactly the same set of non-blank variables as the LHS list, with no initializer values. |
| `same_object` | `a`, `b` | Two identifiers resolve to the same semantic object (same declaration). |

### 3.5 Rule and Analyzer

```cue
#Rule: {
    name:            string & =~"^[a-z][a-z0-9_]*$"
    doc:             string

    // Fact dependency graph
    requires:        [...string]   // fact kinds this rule reads
    provides:        [...string]   // fact kinds this rule emits

    // Scoping
    languages:       [...string] | *["*"]   // tree-sitter grammar names
    file_match?:     [...string]            // filename globs

    // Matching
    match:           #Pattern

    // Semantic gates (adapter-provided)
    pre_conditions?: [...#Precondition]

    // Rewrite behavior
    rewrite_opts?:   #RewriteOpts

    // Effects — at least one of these
    diagnose?:       #Diagnostic
    rewrite?:        #Rewrite
    emit?:           [...{fact: string, attach: string, payload: _}]
}

#Analyzer: {
    name:    string
    version: string
    doc?:    string
    facts:   {[string]: #Fact}
    rules:   {[string]: #Rule}
}

#Adapter: {
    language: string
    checks:   [...string]   // precondition checks this adapter implements
}
```

### 3.6 Scheduling Semantics

The runtime builds a dependency graph from `requires`/`provides` across all rules in all loaded analyzers:

- **DAG (no cycles):** Topological sort, run each rule once.
- **Self-cycles** (rule provides what it requires): Iterate that rule until no new facts are emitted (fixpoint).
- **Multi-rule cycles:** Run the entire strongly-connected component as a fixpoint group with a configurable iteration cap.

## 4. Examples

### 4.1 Pure Syntactic Transform — Nil Check Simplification (Go)

The simplest case: find a tree shape, produce a diagnostic and a rewrite. No facts, no preconditions.

```cue
nilcheck: #Analyzer & {
    name:    "nilcheck"
    version: "0.1.0"
    doc:     "Simplifies nil comparisons in Go"
    facts:   {}

    rules: simplify_nil_eq: {
        name:      "simplify_nil_eq"
        doc:       "Rewrite x == nil to !x in if-conditions"
        languages: ["go"]
        requires:  []
        provides:  []

        match: {
            node: "if_statement"
            fields: condition: {
                node: "binary_expression"
                fields: {
                    left:     {capture: "expr"}
                    operator: {node: "=="}
                    right:    {node: "nil"}
                }
            }
        }

        diagnose: {
            message:  "comparison to nil can be simplified"
            severity: "hint"
        }

        rewrite: edits: [{
            target:      "condition"
            replacement: "!@expr"
        }]
    }
}
```

### 4.2 Fact Passing — Unchecked Error Returns

Two rules cooperate via the fact store. The scheduler sees the `provides`/`requires` edge and runs Pass 1 before Pass 2.

```cue
errcheck: #Analyzer & {
    name:    "errcheck"
    version: "0.1.0"
    doc:     "Detects unchecked error returns across languages"

    facts: {
        returns_error: {
            kind:   "returns_error"
            schema: {func_name: string, positions: [...int]}
        }
        error_checked: {
            kind:   "error_checked"
            schema: {call_site: string, checked: bool}
        }
    }

    rules: {
        // Pass 1: scan declarations, emit facts
        mark_error_funcs: {
            name:      "mark_error_funcs"
            doc:       "Identify functions whose return type includes error"
            languages: ["go", "rust", "typescript"]
            requires:  []
            provides:  ["returns_error"]

            match: {
                node: "function_declaration"
                fields: {
                    name: {capture: "fn_name"}
                    return_type: {
                        capture: "ret"
                        pattern: {
                            node: "type_identifier"
                            where: [{
                                op:   "matches"
                                args: ["@ret", "error|Error|Result"]
                            }]
                        }
                    }
                }
            }

            emit: [{
                fact:    "returns_error"
                attach:  "fn_name"
                payload: {func_name: "@fn_name", positions: [0]}
            }]
        }

        // Pass 2: query facts at call sites
        find_unchecked: {
            name:      "find_unchecked"
            doc:       "Flag calls where error return is discarded"
            languages: ["go", "rust", "typescript"]
            requires:  ["returns_error"]
            provides:  ["error_checked"]

            match: {
                node: "expression_statement"
                children: [{
                    capture: "call"
                    pattern: {
                        node: "call_expression"
                        fields: function: {capture: "callee"}
                        where: [{
                            op:   "has_fact"
                            args: ["@callee", "returns_error"]
                        }]
                    }
                }]
            }

            diagnose: {
                message:  "error return value of @callee is not checked"
                severity: "warning"
            }
        }
    }
}
```

### 4.3 Fixpoint Iteration — Taint Tracking

A rule that both requires and provides the same fact kind. The scheduler iterates it until no new facts are emitted.

```cue
taint: #Analyzer & {
    name:    "taint"
    version: "0.1.0"
    doc:     "Basic taint propagation from sources to sinks"

    facts: tainted: {
        kind:   "tainted"
        schema: {source: string, label: string}
    }

    rules: {
        mark_sources: {
            name:      "mark_sources"
            doc:       "Mark user-input sources as tainted"
            languages: ["python", "javascript", "typescript", "go"]
            requires:  []
            provides:  ["tainted"]

            match: {
                node: "call_expression"
                fields: function: {
                    capture: "fn"
                    pattern: {
                        node: "identifier"
                        where: [{
                            op:   "matches"
                            args: ["@fn", "input|readline|req\\.query|os\\.Args"]
                        }]
                    }
                }
            }

            emit: [{
                fact:    "tainted"
                attach:  "fn"
                payload: {source: "@fn", label: "user_input"}
            }]
        }

        // Fixpoint: reads and writes "tainted"
        propagate_assignments: {
            name:      "propagate_assignments"
            doc:       "Propagate taint through assignments"
            languages: ["*"]
            requires:  ["tainted"]
            provides:  ["tainted"]

            match: {
                node: "assignment_expression"
                fields: {
                    left: {capture: "lhs"}
                    right: {
                        capture: "rhs"
                        pattern: {
                            node: "identifier"
                            where: [{
                                op:   "has_fact"
                                args: ["@rhs", "tainted"]
                            }]
                        }
                    }
                }
            }

            emit: [{
                fact:    "tainted"
                attach:  "lhs"
                payload: {source: "@rhs", label: "propagated"}
            }]
        }

        check_sinks: {
            name:      "check_sinks"
            doc:       "Warn when tainted data reaches a dangerous sink"
            languages: ["*"]
            requires:  ["tainted"]
            provides:  []

            match: {
                node: "call_expression"
                fields: {
                    function: {
                        node: "identifier"
                        where: [{
                            op:   "matches"
                            args: ["@_", "exec|eval|query|system"]
                        }]
                    }
                    arguments: {
                        capture: "arg"
                        pattern: {
                            node: "identifier"
                            where: [{
                                op:   "has_fact"
                                args: ["@arg", "tainted"]
                            }]
                        }
                    }
                }
            }

            diagnose: {
                message:  "tainted data from @arg flows into dangerous sink"
                severity: "error"
            }
        }
    }
}
```

### 4.4 Full Complexity — iferr-analyzer

A port of [github.com/imjasonh/iferr-analyzer](https://github.com/imjasonh/iferr-analyzer) that exercises every extension to the DSL: `adjacent`, `preceding`, `absent_fields`, `nil_comparison`, `token_eq`, preconditions, extended edits, and `node` as a union.

The original Go analyzer (~370 lines of imperative code) inlines error assignments into if conditions:

```go
// Before                          // After
err := foo()                       if err := foo(); err != nil {
if err != nil {                        return err
    return err                     }
}
```

It handles `:=` and `=` assignments, multi-return (`, err := bar()`), reversed nil comparisons (`nil != err`), comment preservation, preceding var-decl absorption, and numerous negative cases (used-after-if, named result parameters, selector LHS, non-consecutive statements, etc.).

```cue
iferr: #Analyzer & {
    name:    "iferr"
    version: "0.1.0"
    doc:     "Suggests inlining error assignments into if conditions where possible"

    facts: {}

    rules: {

        // ============================================================
        // Rule 1: inline_define
        //
        // Handles short variable declarations (:=)
        //
        //   err := foo()          _, err := bar()
        //   if err != nil { }     if err != nil { }
        //
        // Also handles err == nil and reversed nil != err.
        //
        // Corresponds to: checkStmts() with assign.Tok == token.DEFINE
        // ============================================================

        inline_define: {
            name:      "inline_define"
            doc:       "Inline := assignment into if condition"
            languages: ["go"]
            requires:  []
            provides:  []

            match: {
                // node as a union — matches inside block, case clause,
                // or comm clause without duplicating the rule.
                //
                // Corresponds to the type switch in run():
                //   case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause
                node: ["block", "case_clause", "communication_case"]

                // adjacent matches a sliding window of consecutive
                // siblings. The runtime tries every (i, i+1) pair.
                adjacent: [
                    // --- Statement i: the assignment ---
                    {
                        capture: "assign"
                        pattern: {
                            node: "short_var_declaration"
                            fields: {
                                left:  {capture: "lhs_list"}
                                right: {capture: "rhs"}
                            }
                        }
                    },

                    // --- Statement i+1: the if ---
                    {
                        capture: "if_stmt"
                        pattern: {
                            node: "if_statement"

                            // The if must NOT already have an init clause.
                            // Corresponds to: ifStmt.Init != nil check
                            absent_fields: ["initializer"]

                            fields: {
                                condition: {
                                    capture: "cond"
                                    pattern: {
                                        node: "binary_expression"
                                        fields: {
                                            left:     {capture: "cond_x"}
                                            operator: {capture: "cond_op"}
                                            right:    {capture: "cond_y"}
                                        }
                                        where: [
                                            // Operator is == or !=
                                            {op: "matches", args: [
                                                "@cond_op", "==|!=",
                                            ]},

                                            // One side is nil, the other is
                                            // the last non-blank LHS ident.
                                            // Handles both err != nil and
                                            // nil != err.
                                            //
                                            // Corresponds to:
                                            //   isNilComparison()
                                            //   lastNonBlankLHS()
                                            {op: "nil_comparison", args: [
                                                "@cond_x", "@cond_y",
                                                "@lhs_list",
                                            ]},
                                        ]
                                    }
                                }
                            }
                        }
                    },
                ]
            }

            pre_conditions: [
                // No LHS variable is used after the if statement.
                // For := the scope is limited to the block.
                //
                // Corresponds to: usedAfterIf() with DEFINE
                //
                // Blocks: errUsedAfter, equalNilUsedAfter,
                //   resultUsedAfter, resultBlanksUsedAfter,
                //   errUsedAfterIfElse
                {
                    check: "not_used_after"
                    args: {
                        vars:  "@lhs_list"
                        after: "@if_stmt"
                        scope: "block"
                    }
                },
            ]

            rewrite_opts: {
                // Collect comments between assignment and if, float
                // them above the rewritten if statement.
                //
                // Corresponds to: commentPrefix loop in checkStmts()
                preserve_comments: true
                preserve_indent:   true
            }

            diagnose: {
                message:  "can inline assignment into if statement"
                severity: "warning"
            }

            // Replace assignment+if-preamble with "if <assign>; <cond>"
            //
            // Corresponds to: TextEdit in checkStmts()
            //   NewText: commentPrefix + "if " + assignText + "; "
            rewrite: edits: [
                {delete_from: "assign", delete_to: "cond"},
                {position: "before", anchor: "cond", text: "if @assign; "},
            ]
        }


        // ============================================================
        // Rule 2: inline_assign
        //
        // Handles plain assignments (=) with conversion to :=
        //
        //   var err error
        //   err = foo()          ->  if err := foo(); err != nil {
        //   if err != nil { }            return err
        //                            }
        //
        // Additional constraints vs :=:
        //   - All LHS must be plain identifiers (no selectors)
        //   - None can be a named result parameter
        //   - Usage check spans entire function, not just block
        //   - Preceding var decl may be absorbed
        //   - = becomes :=
        //
        // Corresponds to: checkStmts() with assign.Tok == token.ASSIGN
        // ============================================================

        inline_assign: {
            name:      "inline_assign"
            doc:       "Inline = assignment into if condition, converting to :="
            languages: ["go"]
            requires:  []
            provides:  []

            match: {
                node: ["block", "case_clause", "communication_case"]
                adjacent: [
                    // --- Statement i: the assignment ---
                    {
                        capture: "assign"
                        pattern: {
                            node: "assignment_statement"
                            fields: {
                                left:     {capture: "lhs_list"}
                                operator: {capture: "assign_op"}
                                right:    {capture: "rhs"}
                            }
                            where: [
                                // Must be =, not +=, -=, etc.
                                {op: "token_eq", args: ["@assign_op", "="]},
                            ]
                        }

                        // Optionally capture a preceding var declaration
                        // to absorb into the rewrite.
                        //
                        //   var err error       <- captured, absorbed
                        //   err = foo()
                        //   if err != nil { }
                        //
                        // Corresponds to: matchingVarDecl()
                        preceding: {
                            capture:    "var_decl"
                            quantifier: "?"
                            pattern:    {node: "var_declaration"}
                        }
                    },

                    // --- Statement i+1: the if ---
                    {
                        capture: "if_stmt"
                        pattern: {
                            node: "if_statement"
                            absent_fields: ["initializer"]
                            fields: {
                                condition: {
                                    capture: "cond"
                                    pattern: {
                                        node: "binary_expression"
                                        fields: {
                                            left:     {capture: "cond_x"}
                                            operator: {capture: "cond_op"}
                                            right:    {capture: "cond_y"}
                                        }
                                        where: [
                                            {op: "matches", args: [
                                                "@cond_op", "==|!=",
                                            ]},
                                            {op: "nil_comparison", args: [
                                                "@cond_x", "@cond_y",
                                                "@lhs_list",
                                            ]},
                                        ]
                                    }
                                }
                            }
                        }
                    },
                ]
            }

            pre_conditions: [
                // Every LHS must be a plain identifier.
                // Blocks: selectorLHS (t.val is a selector)
                //
                // Corresponds to: allIdentsLHS()
                {
                    check: "all_idents_lhs"
                    args: {lhs: "@lhs_list"}
                },

                // No LHS variable used outside assign+if range,
                // anywhere in the enclosing function.
                //
                // Scope is "function" because the variable lives in
                // an outer scope. Checks for uses both after the if
                // and before the assignment (e.g. for-loop conditions).
                //
                // Blocks: usedInOuterScope, usedInForCondition
                //
                // Corresponds to: usedAfterIf() with ASSIGN
                {
                    check: "not_used_after"
                    args: {
                        vars:                "@lhs_list"
                        after:               "@if_stmt"
                        scope:               "function"
                        exclude_range_start: "@assign"
                        exclude_range_end:   "@if_stmt"
                    }
                },

                // None of the LHS vars is a named result parameter.
                // Bare returns implicitly read named results but those
                // reads don't appear in TypesInfo.Uses.
                //
                // Blocks: namedReturn, funcLitNamedReturn
                //
                // Corresponds to: assignsToNamedResult()
                {
                    check: "not_named_result"
                    args: {vars: "@lhs_list"}
                },

                // If var_decl was captured, verify it declares exactly
                // the same non-blank variables with no initializers.
                // Optional because var_decl capture itself is optional.
                //
                // Corresponds to: matchingVarDecl()
                {
                    check:    "matching_var_decl"
                    args:     {decl: "@var_decl", vars: "@lhs_list"}
                    optional: true
                },
            ]

            rewrite_opts: {
                preserve_comments: true
                preserve_indent:   true
            }

            diagnose: {
                message:  "can inline assignment into if statement"
                severity: "warning"
            }

            rewrite: edits: [
                // Delete from var_decl (or assign) through cond.
                // Falls back to assign if var_decl uncaptured.
                {delete_from: "var_decl", delete_to: "cond"},

                // Convert = to := in the assignment source text.
                // Corresponds to the tokOffset manipulation in iferr.go.
                {within: "assign", token: "=", replace_with: ":="},

                // Insert the if-init.
                {position: "before", anchor: "cond", text: "if @assign; "},
            ]
        }
    }
}
```

#### iferr test coverage map

**Positive cases (diagnostic fires):**

| Test case | Matched by | Key mechanism |
|-----------|-----------|---------------|
| `basic()` | `inline_define` | Simple `:=` + `if err != nil` |
| `blankErr()` | `inline_define` | `_, err := bar()` — `nil_comparison` finds last non-blank |
| `multiLineBody()` | `inline_define` | Multi-line if body irrelevant to match |
| `multipleBlanks()` | `inline_define` | `_, _, err := baz()` — same logic |
| `namedErr()` | `inline_define` | Works with any identifier name |
| `equalNilPositive()` | `inline_define` | `err == nil` — `matches` accepts `==\|!=` |
| `assignWithVar()` | `inline_assign` | `var err error` absorbed by `preceding` |
| `commentOnAssignLine()` | `inline_define` | `preserve_comments` handles inline comments |
| `commentOnIfLine()` | `inline_define` | If-line comment stays with the if |
| `commentOnBothLines()` | `inline_define` | Both preserved |
| `commentBetweenStmts()` | `inline_define` | Between-comment floated above rewritten if |
| `commentNolint()` | `inline_define` | `//nolint` directive preserved |
| `commentMixed()` | `inline_define` | Between floats, if-line stays |
| `commentInFuncLit()` | `inline_define` | Source text preservation keeps func literal comments |
| `multiReturnUnusedAfter()` | `inline_define` | `x` only used inside if body |
| `inSwitchCase()` | `inline_define` | `node` union includes `case_clause` |
| `inSelectClause()` | `inline_define` | `node` union includes `communication_case` |
| `ifWithElse()` | `inline_define` | Else is part of if_stmt |
| `reversedNil()` | `inline_define` | `nil != err` — `nil_comparison` handles both orderings |
| `equalNilWithElse()` | `inline_define` | `err == nil` with else, err used in else (still in if_stmt) |

**Negative cases (no diagnostic):**

| Test case | What blocks it | DSL mechanism |
|-----------|---------------|---------------|
| `errUsedAfter()` | err used after if | `not_used_after`, scope: block |
| `alreadyHasInit()` | if has init clause | `absent_fields: ["initializer"]` |
| `equalNilUsedAfter()` | err returned after if | `not_used_after` |
| `notNilComparison()` | Compares to `other`, not nil | `nil_comparison` fails |
| `nonConsecutive()` | Statement between assign and if | `adjacent` — not consecutive |
| `resultUsedAfter()` | result used after if | `not_used_after` catches all LHS vars |
| `selectorLHS()` | `t.val` on LHS | `all_idents_lhs` fails |
| `namedReturn()` | Named return parameter | `not_named_result` fails |
| `usedInOuterScope()` | err used in outer scope | `not_used_after`, scope: function |
| `usedInForCondition()` | `i` in for condition | `not_used_after` with exclude_range |
| `funcLitNamedReturn()` | Named return in func literal | `not_named_result` |
| `resultBlanksUsedAfter()` | result from 3-return used | `not_used_after` |
| `compoundCondition()` | Top-level op is `&&` | Pattern expects `==\|!=` at top level |
| `errUsedAfterIfElse()` | err used after if/else | `not_used_after` |

## 5. Go Adapter Specification

The Go adapter implements the precondition checks that require access to Go's type system (`go/types`) and scope analysis. Each check is a function that receives resolved AST nodes (from captures) and returns a boolean.

```go
// Adapter interface (conceptual)
type Adapter interface {
    // Language returns the tree-sitter grammar name.
    Language() string

    // Checks returns the list of precondition check names
    // this adapter supports.
    Checks() []string

    // RunCheck executes a named precondition check with the
    // given captures and arguments. Returns true if the
    // precondition is satisfied (match should proceed).
    RunCheck(
        check string,
        captures map[string]CapturedNode,
        args map[string]string,
    ) (bool, error)
}
```

### Check implementations

**`not_used_after`**: Walks `pass.TypesInfo.Uses` looking for references to any captured LHS variable after the if statement. For scope `"block"`, checks only remaining statements in the current block. For scope `"function"`, checks the entire enclosing function, excluding uses within the `exclude_range_start..exclude_range_end` range. Also catches backward references (variables used before the assignment, e.g. in for-loop conditions).

**`all_idents_lhs`**: Verifies every LHS expression is an `*ast.Ident`, not a selector (`a.b`), index (`a[i]`), or other compound expression. Required because `:=` only works with plain identifiers.

**`not_named_result`**: Finds the tightest enclosing function (traversing through `FuncDecl` and `FuncLit`), checks if any LHS variable resolves to a named result parameter. Bare `return` statements implicitly read named results without appearing in `TypesInfo.Uses`.

**`matching_var_decl`**: Checks that a captured `var` declaration declares exactly the same set of non-blank variables as the assignment LHS, with no initializer values. If it matches, the rewrite absorbs (deletes) the declaration.

## 6. Runtime Engine Design

### 6.1 Components

```
┌─────────────────────────────────────────────────┐
│                  CLI / API                       │
│  parse flags, load configs, output results       │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│              Rule Loader (CUE)                   │
│  validate schema, expand node unions,            │
│  resolve fact references, build dependency graph │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│              Scheduler                           │
│  topo-sort by requires/provides,                 │
│  detect SCCs for fixpoint groups,                │
│  manage iteration caps                           │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│              Pattern Matcher                     │
│  walk tree-sitter CST,                           │
│  match patterns (fields, children, adjacent),    │
│  evaluate predicates, bind captures              │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│           Precondition Runner                    │
│  dispatch checks to language adapter,            │
│  handle optional checks, unbound captures        │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│              Effect Executor                     │
│  emit facts to fact store,                       │
│  collect diagnostics,                            │
│  compute edit operations                         │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│              Edit Applicator                     │
│  sort edits by position, resolve conflicts,      │
│  apply to source bytes, preserve comments        │
└──────────────────────────────────────────────────┘
```

### 6.2 Pattern Matching Algorithm

1. **Node type check**: If `node` is a string, compare directly. If it's a list, succeed if the tree-sitter node type is in the list.

2. **Field matching**: For each entry in `fields`, find the corresponding named child in the tree-sitter node. If the field value is a `#Pattern`, recurse. If it's a `#Capture`, bind the node (optionally constraining with `pattern`).

3. **`absent_fields`**: For each field name in the list, verify the tree-sitter node does NOT have a child with that field name. Fail the match if any are present.

4. **`adjacent` matching**: Iterate through the parent's children with a sliding window of size `len(adjacent)`. For each window position, attempt to match all elements. The first successful window wins. If `preceding` is specified on an element, also attempt to match the child at `window_start - 1`.

5. **Predicate evaluation**: After structural matching succeeds, evaluate all `where` predicates. Predicates may reference captures bound during structural matching. All predicates must pass for the match to succeed.

6. **Capture binding**: Successfully matched captures are collected into a map from name to `(node, byte_range)`. This map is passed to preconditions and edit construction.

### 6.3 Edit Application

Edits are compiled into a list of `(start_byte, end_byte, replacement_text)` operations:

1. **`delete_from` / `delete_to`**: `start = capture[delete_from].start`, `end = capture[delete_to].start`. If `delete_from` references an unbound optional capture, fall back to the next bound capture in the adjacent sequence.

2. **`within` / `token` / `replace_with`**: Find the first occurrence of `token` in the source text of the captured node. Replace it in-place. This operates on the source text, not the AST, preserving surrounding comments and whitespace.

3. **`position: "before"` / `"after"`**: Insert text at the start or end byte of the anchor capture.

4. **`@capture` interpolation**: In replacement text, `@name` is replaced with the original source text of the captured node.

5. **Comment preservation**: When `preserve_comments` is true, scan the deleted byte range for comments (using tree-sitter's comment node types). Collect their text, prepend them to the replacement text with appropriate indentation.

6. **Conflict resolution**: Multiple edits to overlapping byte ranges are resolved by applying inner edits first (smallest range), then outer edits adjusted for the byte-offset shifts.

## 7. Existing Work and Influences

| Project | Relationship |
|---------|-------------|
| [Go analysis framework](https://pkg.go.dev/golang.org/x/tools/go/analysis) | Primary inspiration. Fact passing, requires/provides, suggested fixes. |
| [Tree-sitter](https://tree-sitter.github.io/) | Parser foundation. Provides CSTs for 100+ languages. |
| [CUE](https://cuelang.org/) | Schema and constraint language. Validates rules at load time. |
| [ast-grep](https://ast-grep.github.io/) | Similar idea (tree-sitter patterns for search/replace). Lacks fact passing and preconditions. |
| [Semgrep](https://semgrep.dev/) | Security-focused multi-language analysis. Different pattern syntax, less focus on transforms. |
| [Comby](https://comby.dev/) | Structural search/replace with holes. Simpler than tree-sitter patterns but less precise. |
| [iferr-analyzer](https://github.com/imjasonh/iferr-analyzer) | Stress test that drove the DSL extensions. |

## 8. Open Questions

1. **Cross-file facts.** The current schema attaches facts to nodes within a single file. For cross-file analysis (e.g. taint tracking across modules), facts need a `scope: "file" | "package" | "module"` field and a persistence mechanism.

2. **CUE's lattice model for facts.** CUE values can only become more specific (lattice join). This maps naturally to monotone dataflow analysis — facts could be CUE values that unify rather than overwrite, giving lattice-based analysis for free. Worth exploring.

3. **Escape hatches.** For rewrites that require computation (e.g. generating import statements), the current template/edit system may be insufficient. Options include Starlark snippets, CUE expressions, or a `transform` function in the adapter.

4. **Performance.** Tree-sitter parsing is fast, but pattern matching with `adjacent` windows and predicate evaluation could be expensive on large files. The runtime should index nodes by type and use early termination.

5. **Testing.** Go's `analysistest` package provides a great model — testdata files with `// want` comments and `.golden` files for fix verification. The CUE framework should have an equivalent, with language-agnostic test harnesses.

6. **Rule composition across analyzers.** If analyzer A provides fact `"tainted"` and analyzer B requires it, they should compose automatically when loaded together. The current schema supports this at the type level but the runtime needs to handle cross-analyzer fact namespacing.
