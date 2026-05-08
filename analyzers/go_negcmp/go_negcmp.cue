// negcmp simplifies negated comparisons:
//
//   !(a == b)   ->   (a != b)
//   !(a != b)   ->   (a == b)
//
// Always semantically equivalent in Go.
//
// Implementation: surgical, two edits per match. We flip the inner
// comparison operator in place and delete the outer `!` (everything
// from the start of the match up to the start of the parens). The
// parens stay — `(a != b)` is fine syntax and keeping them avoids any
// precedence-bug risk from operand expressions that aren't simple
// identifiers.
//
// The surgical approach preserves comments inside the parens, e.g.
// `!(a /* note */ == b)` -> `(a /* note */ != b)`.

package go_negcmp

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
)

// _negcmpRule builds a rule that flips an `==`/`!=` inside `!(...)`.
_negcmpRule: {
	_from: "==" | "!="
	_to:   "==" | "!="

	out: {
		languages: [golang.Name]
		requires: []
		provides: []

		match: {
			node: "unary_expression"
			fields: {
				operator: {capture: "neg"}
				operand: {
					capture: "parens"
					pattern: {
						node: "parenthesized_expression"
						children: [{
							node: "binary_expression"
							fields: {
								operator: {capture: "innerop"}
							}
							where: [{op: "token_eq", args: ["@innerop", _from]}]
						}]
					}
				}
			}
			where: [{op: "token_eq", args: ["@neg", "!"]}]
		}

		diagnose: {
			severity: "hint"
			message:  "negated comparison can be simplified to `\(_to)`"
		}

		// Two surgical edits:
		//   1. Replace the inner operator (== <-> !=).
		//   2. Delete the outer `!` (bytes from _root start to the
		//      open paren of the parens capture).
		rewrite: edits: [
			{target: "innerop", replacement: _to},
			{delete_from: "_root", delete_to: "parens"},
		]
	}
}

go_negcmp: schema.#Analyzer & {
	name:    "go_negcmp"
	version: "0.1.0"
	doc:     "Simplify negated comparisons: !(a == b) -> (a != b)"
	facts: {}

	rules: {
		not_eq: (_negcmpRule & {_from: "==", _to: "!="}).out & {
			name: "not_eq"
			doc:  "Rewrite !(a == b) to (a != b)"
		}
		not_neq: (_negcmpRule & {_from: "!=", _to: "=="}).out & {
			name: "not_neq"
			doc:  "Rewrite !(a != b) to (a == b)"
		}
	}
}
