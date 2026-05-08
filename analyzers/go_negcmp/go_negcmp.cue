// negcmp simplifies negated comparisons:
//
//   !(a == b)   ->   a != b
//   !(a != b)   ->   a == b
//
// Always semantically equivalent in Go. Pure structural — no preconditions,
// no fact passing, no comment preservation needed.

package go_negcmp

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
)

go_negcmp: schema.#Analyzer & {
	name:    "go_negcmp"
	version: "0.1.0"
	doc:     "Simplify negated comparisons: !(a == b) -> a != b"
	facts: {}

	rules: {
		// !(a == b) -> a != b
		not_eq: {
			name:      "not_eq"
			doc:       "Rewrite !(a == b) to a != b"
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

		// !(a != b) -> a == b
		not_neq: {
			name:      "not_neq"
			doc:       "Rewrite !(a != b) to a == b"
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
								where: [{op: "token_eq", args: ["@innerop", "!="]}]
							}
						}]
					}
				}
				where: [{op: "token_eq", args: ["@neg", "!"]}]
			}

			diagnose: {
				message:  "negated inequality can be simplified to =="
				severity: "hint"
			}

			rewrite: edits: [{
				target:      "_root"
				replacement: "@a == @b"
			}]
		}
	}
}
