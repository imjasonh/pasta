// arith_parens flags arithmetic expressions whose operands are
// themselves unparenthesized arithmetic expressions, requiring the
// programmer to parenthesize sub-expressions that rely on operator
// precedence or associativity rules to evaluate the way they look.
//
// The classic bug this prevents is the C-style:
//
//     args->endp - args->begin_argv + consume
//
// which evaluates left-to-right as `(endp - begin) + consume` per the
// C-family associativity rules — but in many real-world reports the
// programmer intended `endp - (begin + consume)`. Forcing the parens
// makes the chosen grouping explicit so reviewers and tools can see
// it instead of having to mentally apply precedence tables.
//
// The auto-fix wraps the inner sub-expression in parentheses,
// preserving tree-sitter's parse (which is the language-defined
// meaning). If the result reads wrong, that's the bug surfacing — fix
// it by hand.
//
// Cross-language: every C-family tree-sitter grammar uses
// `binary_expression` with `left`/`right`/`operator` fields and the
// operator child carries the literal token. The rule lists each
// language explicitly rather than `["*"]` so we don't accidentally
// fire on grammars where `binary_expression` means something else.
//
// Scope: arithmetic operators only — multiplicative
// (`* / % << >> & &^`) and additive (`+ - | ^`) plus `**` for
// languages that have it. Comparison and logical operators are out of
// scope; their conventional usage rarely surprises readers. Operators
// absent from a given language (Go's `&^` in C, JS's `**` in Java)
// simply never appear in source so over-coverage is harmless.

package arith_parens

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
	clang "github.com/imjasonh/pasta/lang/c"
	cpplang "github.com/imjasonh/pasta/lang/cpp"
	javalang "github.com/imjasonh/pasta/lang/java"
	jslang "github.com/imjasonh/pasta/lang/javascript"
	tslang "github.com/imjasonh/pasta/lang/typescript"
	rustlang "github.com/imjasonh/pasta/lang/rust"
	phplang "github.com/imjasonh/pasta/lang/php"
)

// _arithOp matches the text of any binary arithmetic operator across
// the C-family languages this rule covers. Both sides of an
// outer/inner pair are checked against this so the rule stays out of
// comparison (`==`, `<`, ...) and logical (`&&`, `||`) territory.
_arithOp: "^([+\\-*/%&|^]|<<|>>|&\\^|\\*\\*)$"

// _supportedLanguages is the set of grammars where `binary_expression`
// has the field shape (`left`, `right`, `operator`) this rule expects.
_supportedLanguages: [
	golang.Name,
	clang.Name,
	cpplang.Name,
	javalang.Name,
	jslang.Name,
	tslang.Name,
	rustlang.Name,
	phplang.Name,
]

// _arithRule emits a rule for one side (left or right) of the outer
// binary expression. Splitting by side keeps each pattern shape
// concrete; one rule for both would need a disjunction the matcher
// doesn't currently express.
_arithRule: {
	_side: "left" | "right"

	out: {
		languages: _supportedLanguages
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

arith_parens: schema.#Analyzer & {
	name:    "arith_parens"
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
