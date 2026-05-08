// go_self_assignment flags `x = x` — assigning a variable to itself.
// Almost always a typo (the author meant a different RHS) or dead
// code. Includes the rewrite that simply deletes the statement.

package go_self_assignment

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
)

go_self_assignment: schema.#Analyzer & {
	name:    "go_self_assignment"
	version: "0.1.0"
	doc:     "Flag and remove `x = x` self-assignments"
	facts: {}

	rules: self_assign: {
		name: "self_assign"
		doc:  "x = x is a no-op, usually a typo"
		languages: [golang.Name]
		requires: []
		provides: []

		match: {
			node: "assignment_statement"
			fields: {
				left: {
					node: "expression_list"
					children: [{capture: "lhs", pattern: {node: "identifier"}}]
				}
				operator: {capture: "op"}
				right: {
					node: "expression_list"
					children: [{capture: "rhs", pattern: {node: "identifier"}}]
				}
			}
			where: [
				{op: "token_eq", args: ["@op", "="]},
				{op: "same_ident", args: ["@lhs", "@rhs"]},
			]
		}

		diagnose: {
			message:  "self-assignment x = x is a no-op"
			severity: "warning"
		}

		// Replace the statement with an empty string. The trailing
		// newline remains as part of surrounding source.
		rewrite: edits: [{
			target:      "_root"
			replacement: ""
		}]
	}
}
