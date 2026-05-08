// rust_taint tracks values from `std::env::var(...)` and
// `std::env::args()` through `let` bindings to dangerous sinks
// (`std::process::Command::new(x)`).
//
// Same shape as the Python / Go / JS taint analyzers.

package rust_taint

import (
	"pasta.dev/schema"
	rustlang "pasta.dev/lang/rust"
)

rust_taint: schema.#Analyzer & {
	name:    "rust_taint"
	version: "0.1.0"
	doc:     "Track taint from std::env into Command::new"

	facts: {
		tainted: {kind: "tainted"}
	}

	rules: {
		// Pass 1: tag `let x = std::env::var(...)`.
		// We match the let_declaration's pattern (lhs identifier) and
		// value (a call_expression to std::env::var or env::var or var).
		mark_sources: {
			name:      "mark_sources"
			doc:       "Tag LHS of `let x = env::var(...)` and similar"
			languages: [rustlang.Name]
			requires: []
			provides: ["tainted"]

			match: {
				node: "let_declaration"
				fields: {
					pattern: {capture: "lhs", pattern: {node: "identifier"}}
					value: {
						capture: "rhs"
						pattern: {node: "call_expression"}
					}
				}
				// Match the call's text against any of the env-source
				// shapes Rust code uses.
				where: [{
					op: "matches"
					args: ["@rhs", "(std::)?env::(var|var_os|args)\\("]
				}]
			}

			emit: [{fact: "tainted", attach: "lhs"}]
		}

		// Pass 2: propagate via `let y = x;` where x has fact.
		propagate: {
			name:      "propagate"
			doc:       "Propagate taint through identifier-to-identifier let bindings"
			languages: [rustlang.Name]
			requires: ["tainted"]
			provides: ["tainted"]

			match: {
				node: "let_declaration"
				fields: {
					pattern: {capture: "lhs", pattern: {node: "identifier"}}
					value: {capture: "rhs", pattern: {node: "identifier"}}
				}
				where: [{op: "has_fact", args: ["@rhs", "tainted"]}]
			}

			emit: [{fact: "tainted", attach: "lhs"}]
		}

		// Pass 3: warn on `Command::new(x)` where x is tainted. Match
		// the call as a whole and use a regex on the function (which
		// will be the scoped_identifier text "Command::new", possibly
		// prefixed).
		check_sinks: {
			name:      "check_sinks"
			doc:       "Flag tainted data flowing into Command::new"
			languages: [rustlang.Name]
			requires: ["tainted"]
			provides: []

			match: {
				node: "call_expression"
				fields: {
					function: {capture: "fn"}
					arguments: {
						node: "arguments"
						children: [{capture: "arg", pattern: {node: "identifier"}}]
					}
				}
				where: [
					{op: "matches", args: ["@fn", "(^|::)Command::new$"]},
					{op: "has_fact", args: ["@arg", "tainted"]},
				]
			}

			diagnose: {
				message:  "tainted data from environment flows into Command::new"
				severity: "error"
			}
		}
	}
}
