// rust_deprecated_use flags calls to functions marked with the
// `#[deprecated]` attribute. The Rust compiler emits a warning, but
// for cases where you want to fail the build (or for environments
// where rustc warnings are silenced), this lints at the syntactic
// level.
//
// Pass 1: walk function items whose immediately-preceding named
//         sibling is an attribute_item containing `deprecated`.
// Pass 2: walk call expressions; if the callee identifier has the
//         `deprecated` fact, warn.

package rust_deprecated_use

import (
	"pasta.dev/schema"
	rustlang "pasta.dev/lang/rust"
)

rust_deprecated_use: schema.#Analyzer & {
	name:    "rust_deprecated_use"
	version: "0.1.0"
	doc:     "Flag calls to functions marked #[deprecated]"

	facts: {
		deprecated: {kind: "deprecated"}
	}

	rules: {
		mark_deprecated: {
			name:      "mark_deprecated"
			doc:       "Tag function items with a #[deprecated] attribute"
			languages: [rustlang.Name]
			requires: []
			provides: ["deprecated"]

			match: {
				node: "function_item"
				fields: {
					name: {capture: "fn_name"}
				}
				where: [{
					op: "prev_sibling_matches"
					args: ["@_root", "deprecated"]
				}]
			}

			emit: [{
				fact:   "deprecated"
				attach: "fn_name"
			}]
		}

		find_calls: {
			name:      "find_calls"
			doc:       "Flag call sites of deprecated functions"
			languages: [rustlang.Name]
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
