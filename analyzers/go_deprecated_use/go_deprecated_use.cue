// go_deprecated_use flags calls to functions whose preceding doc
// comment includes the conventional Go marker `Deprecated:`. This is
// the standard mechanism `staticcheck` uses, but encoding it here
// shows how a multi-rule analyzer using fact passing can implement it
// from scratch.
//
// Pass 1: walk function declarations whose immediately-preceding named
//         sibling is a comment containing `Deprecated:`. Emit a
//         `deprecated` fact attached to the function's name.
// Pass 2: walk call expressions; if the callee identifier has a
//         `deprecated` fact (matched by name), warn.

package go_deprecated_use

import (
	"pasta.dev/schema"
	golang "pasta.dev/lang/go"
)

go_deprecated_use: schema.#Analyzer & {
	name:    "go_deprecated_use"
	version: "0.1.0"
	doc:     "Flag calls to functions documented as `Deprecated:`"

	facts: {
		deprecated: {kind: "deprecated"}
	}

	rules: {
		mark_deprecated: {
			name:      "mark_deprecated"
			doc:       "Tag function declarations whose doc comment contains `Deprecated:`"
			languages: [golang.Name]
			requires: []
			provides: ["deprecated"]

			match: {
				node: "function_declaration"
				fields: {
					name: {capture: "fn_name"}
				}
				where: [{
					op: "prev_sibling_matches"
					args: ["@_root", "Deprecated:"]
				}]
			}

			emit: [{
				fact:    "deprecated"
				attach:  "fn_name"
				payload: {func_name: "@fn_name"}
			}]
		}

		find_calls: {
			name:      "find_calls"
			doc:       "Flag call sites of deprecated functions"
			languages: [golang.Name]
			requires: ["deprecated"]
			provides: []

			match: {
				node: "call_expression"
				fields: {
					function: {
						capture: "callee"
						pattern: {node: "identifier"}
					}
				}
				where: [{op: "has_fact", args: ["@callee", "deprecated"]}]
			}

			diagnose: {
				message:  "@callee is deprecated"
				severity: "warning"
			}
		}
	}
}
