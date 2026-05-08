// go_errcheck is a port of plan.md §4.2 — a two-rule analyzer that
// uses fact passing to find call sites where an error return is
// silently discarded.
//
// Pass 1: walk function declarations whose result is `error` and emit a
//         `returns_error` fact attached to the function's name. The
//         fact store also indexes the fact by the identifier's TEXT,
//         so call sites can find it without scope analysis.
//
// Pass 2: walk expression statements that are bare call expressions
//         (`foo()` on a line by itself, with no assignment). If the
//         function name has a `returns_error` fact, the return is
//         being discarded — fire the diagnostic.
//
// This demonstrates rule scheduling: the engine topologically sorts
// rules by `requires`/`provides`, so mark_error_funcs always runs
// before find_unchecked even though their alphabetical order is
// reversed.
//
// Limitations: name-only fact lookup means a call to any `foo` matches
// any error-returning `foo`, regardless of scope or package. Real
// cross-file / cross-package errcheck would need scope-aware fact
// keys (plan.md §8 open question).

package go_errcheck

import (
	"pasta.dev/schema"
	golang "pasta.dev/lang/go"
)

go_errcheck: schema.#Analyzer & {
	name:    "go_errcheck"
	version: "0.1.0"
	doc:     "Flag function calls whose error return value is discarded"

	facts: {
		returns_error: {kind: "returns_error"}
	}

	rules: {
		// Pass 1: tag every function declaration that returns `error`.
		mark_error_funcs: {
			name:      "mark_error_funcs"
			doc:       "Tag function declarations whose result type is error"
			languages: [golang.Name]
			requires: []
			provides: ["returns_error"]

			match: {
				node: "function_declaration"
				fields: {
					name: {capture: "fn_name"}
					result: {
						capture: "ret"
						pattern: {node: "type_identifier"}
					}
				}
				where: [{op: "eq", args: ["@ret", "error"]}]
			}

			emit: [{
				fact:   "returns_error"
				attach: "fn_name"
				payload: {func_name: "@fn_name"}
			}]
		}

		// Pass 2: flag bare-call expression statements where the
		// function name has the returns_error fact.
		find_unchecked: {
			name:      "find_unchecked"
			doc:       "Flag calls whose error return is discarded"
			languages: [golang.Name]
			requires: ["returns_error"]
			provides: []

			match: {
				node: "expression_statement"
				children: [{
					capture: "call"
					pattern: {
						node: "call_expression"
						fields: {
							function: {
								capture: "callee"
								pattern: {node: "identifier"}
							}
						}
						where: [{op: "has_fact", args: ["@callee", "returns_error"]}]
					}
				}]
			}

			diagnose: {
				message:  "error return value of @callee is discarded"
				severity: "warning"
			}

			// Auto-fix: replace the bare `foo()` statement with
			// `_ = foo()` to make the discard explicit. The rewrite
			// doesn't try to GUESS the right error handling — that's
			// for the developer — but the explicit blank-assign at
			// least surfaces the intent in code review.
			rewrite: edits: [{
				target:      "_root"
				replacement: "_ = @call"
			}]
		}
	}
}
