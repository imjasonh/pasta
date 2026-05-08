// needless_bool implements clippy's lint of the same name (W0125 in
// clippy_lints). It collapses redundant if-expressions whose only branch
// contents are the boolean literals `true` and `false`:
//
//   if cond { true } else { false }    ->   cond
//   if cond { false } else { true }    ->   !(cond)
//
// For the negated form, the condition is always wrapped in parens: it's
// uglier for simple identifiers but correct for compound expressions
// (e.g. `if x && y { false } else { true }` → `!(x && y)`, not `!x && y`).

package rust_needless_bool

import (
	"github.com/imjasonh/pasta/schema"
	"github.com/imjasonh/pasta/lang/rust"
)

rust_needless_bool: schema.#Analyzer & {
	name:    "rust_needless_bool"
	version: "0.1.0"
	doc:     "Collapse `if x { true } else { false }` patterns"
	facts: {}

	rules: {
		// if cond { true } else { false }  ->  cond
		bool_identity: {
			name:      "bool_identity"
			doc:       "Replace `if cond { true } else { false }` with `cond`"
			languages: [rust.Name]
			requires: []
			provides: []

			match: {
				node: "if_expression"
				fields: {
					condition: {capture: "cond"}
					consequence: {
						node: "block"
						children: [
							{capture: "lit_then", pattern: {node: "boolean_literal"}},
						]
					}
					alternative: {
						node: "else_clause"
						children: [{
							node: "block"
							children: [
								{capture: "lit_else", pattern: {node: "boolean_literal"}},
							]
						}]
					}
				}
				where: [
					{op: "token_eq", args: ["@lit_then", "true"]},
					{op: "token_eq", args: ["@lit_else", "false"]},
				]
			}

			diagnose: {
				message:  "this if-expression can be replaced by its condition"
				severity: "warning"
			}

			rewrite: edits: [{target: "_root", replacement: "@cond"}]
		}

		// if cond { false } else { true }  ->  !(cond)
		bool_negation: {
			name:      "bool_negation"
			doc:       "Replace `if cond { false } else { true }` with `!(cond)`"
			languages: [rust.Name]
			requires: []
			provides: []

			match: {
				node: "if_expression"
				fields: {
					condition: {capture: "cond"}
					consequence: {
						node: "block"
						children: [
							{capture: "lit_then", pattern: {node: "boolean_literal"}},
						]
					}
					alternative: {
						node: "else_clause"
						children: [{
							node: "block"
							children: [
								{capture: "lit_else", pattern: {node: "boolean_literal"}},
							]
						}]
					}
				}
				where: [
					{op: "token_eq", args: ["@lit_then", "false"]},
					{op: "token_eq", args: ["@lit_else", "true"]},
				]
			}

			diagnose: {
				message:  "this if-expression can be replaced by `!(condition)`"
				severity: "warning"
			}

			rewrite: edits: [{target: "_root", replacement: "!(@cond)"}]
		}
	}
}
