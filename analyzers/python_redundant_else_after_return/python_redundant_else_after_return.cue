// python_redundant_else_after_return flags an `else` clause whose
// preceding `if` body ends with a return — the else is unreachable
// only when the if doesn't return, so the else branch can be
// outdented and the `else:` keyword dropped. Pylint R1705 covers
// the same ground.
//
// This rule is lint-only: a correct rewrite needs to dedent the
// else block, and pasta's edit primitives are byte-range-only with
// no awareness of Python's indentation grammar. The user dedents
// after applying the diagnostic. Tracked in future-work.md as the
// "indentation-aware rewrites" gap.

package python_redundant_else_after_return

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

python_redundant_else_after_return: schema.#Analyzer & {
	name:    "python_redundant_else_after_return"
	version: "0.1.0"
	doc:     "Flag `else` after a `return`; the else can be dropped"
	facts: {}

	rules: redundant_else: {
		name:      "redundant_else"
		doc:       "if x: return ...; else: ... -> if x: return ...; ..."
		languages: [pylang.Name]
		requires: []
		provides: []

		match: {
			node: "if_statement"
			fields: {
				consequence: {
					capture: "if_body"
					pattern: {
						node: "block"
						// Single-statement if-body whose only child is
						// a return. The general case (any block ending
						// in a return) needs a `last_child_is` predicate
						// pasta doesn't have — see future-work.md.
						children: [{
							capture: "ret"
							pattern: {node: "return_statement"}
						}]
					}
				}
				alternative: {
					capture: "else"
					pattern: {node: "else_clause"}
				}
			}
			where: [
				{op: "named_child_count", args: ["@if_body", "1"]},
			]
		}

		diagnose: {
			severity: "hint"
			message:  "redundant `else` after `return` — outdent the else branch"
		}
	}
}
