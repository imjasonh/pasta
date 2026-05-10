// go_arith_parens flags arithmetic expressions whose operands are
// themselves unparenthesized arithmetic expressions, requiring the
// programmer to parenthesize sub-expressions that rely on operator
// precedence or associativity rules to evaluate the way they look.
//
// The classic bug this prevents is the C-style:
//
//     args->endp - args->begin_argv + consume
//
// which evaluates left-to-right as `(endp - begin) + consume` per Go's
// (and C's) associativity rules — but in many real-world reports the
// programmer intended `endp - (begin + consume)`. Forcing the parens
// makes the chosen grouping explicit so reviewers and tools can see
// it instead of having to mentally apply precedence tables.
//
// The auto-fix wraps the inner sub-expression in parentheses,
// preserving tree-sitter's parse (which is the language-defined
// meaning). If the result reads wrong, that's the bug surfacing — fix
// it by hand.
//
// Scope: arithmetic operators only — multiplicative
// (`* / % << >> & &^`) and additive (`+ - | ^`). Comparison and
// logical operators are out of scope; `a < b && c < d` reads cleanly
// even without inner parens.

package go_arith_parens

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
)

// _arithOp matches the text of any binary arithmetic operator. Both
// sides of an outer/inner pair are checked against this so the rule
// stays out of comparison (`==`, `<`, ...) and logical (`&&`, `||`)
// territory.
_arithOp: "^([+\\-*/%&|^]|<<|>>|&\\^)$"

// _arithRule emits a rule for one side (left or right) of the outer
// binary expression. Splitting by side keeps each pattern shape
// concrete; one rule for both would need a disjunction the matcher
// doesn't currently express.
_arithRule: {
	_side: "left" | "right"

	out: {
		languages: [golang.Name]
		requires: []
		provides: []

		match: {
			node: "binary_expression"
			fields: {
				operator: {capture: "outerop"}
				if _side == "left" {
					left: {
						capture: "inner"
						pattern: {
							node: "binary_expression"
							fields: {operator: {capture: "innerop"}}
							where: [{op: "matches", args: ["@innerop", _arithOp]}]
						}
					}
				}
				if _side == "right" {
					right: {
						capture: "inner"
						pattern: {
							node: "binary_expression"
							fields: {operator: {capture: "innerop"}}
							where: [{op: "matches", args: ["@innerop", _arithOp]}]
						}
					}
				}
			}
			where: [{op: "matches", args: ["@outerop", _arithOp]}]
		}

		diagnose: {
			severity: "warning"
			message:  "parenthesize arithmetic sub-expression to make evaluation order explicit"
		}

		// Wrap the inner binary_expression in parens. Two pure
		// insertions — `(` before, `)` after — preserve the inner
		// bytes (and any comments) verbatim.
		rewrite: edits: [
			{position: "before", anchor: "inner", text: "("},
			{position: "after", anchor: "inner", text: ")"},
		]
	}
}

go_arith_parens: schema.#Analyzer & {
	name:    "go_arith_parens"
	version: "0.1.0"
	doc:     "Require parentheses around nested arithmetic sub-expressions"
	facts: {}

	rules: {
		left: (_arithRule & {_side: "left"}).out & {
			name: "left"
			doc:  "Inner arithmetic on the left of an outer arithmetic op needs parens"
		}
		right: (_arithRule & {_side: "right"}).out & {
			name: "right"
			doc:  "Inner arithmetic on the right of an outer arithmetic op needs parens"
		}
	}
}
