// negcmp_demo is a small rule packaged alongside the notgo language
// declaration in this directory. It rewrites `!(a == b)` to `a != b`,
// targeting any language whose tree-sitter grammar is "go".
//
// Because .notgo files (declared in notgo.cue) use the Go grammar, this
// rule fires on them automatically — no rule change required to support
// the new extension. The same rule would also fire on .go files; the
// runner restricts its run to whatever extensions the input file has.

package notgo_alias

import (
	"pasta.dev/schema"
	golang "pasta.dev/lang/go"
)

negcmp_demo: schema.#Analyzer & {
	name:    "negcmp_demo"
	version: "0.1.0"
	doc:     "Demo: !(a == b) -> a != b, applied through grammar matching"
	facts: {}

	rules: not_eq: {
		name: "not_eq"
		doc:  "Rewrite !(a == b) to a != b"
		// Match by grammar name; this fires on any language alias whose
		// grammar is "go", including the notgo alias declared above.
		languages: [golang.Name]
		requires: []
		provides: []

		match: {
			node: "unary_expression"
			fields: {
				operator: {capture: "neg"}
				operand: {
					node: "parenthesized_expression"
					children: [{
						capture: "inner"
						pattern: {
							node: "binary_expression"
							fields: {
								left:     {capture: "a"}
								operator: {capture: "innerop"}
								right:    {capture: "b"}
							}
							where: [{op: "token_eq", args: ["@innerop", "=="]}]
						}
					}]
				}
			}
			where: [{op: "token_eq", args: ["@neg", "!"]}]
		}

		diagnose: {
			message:  "negated equality can be simplified to !="
			severity: "hint"
		}

		rewrite: edits: [{
			target:      "_root"
			replacement: "@a != @b"
		}]
	}
}
