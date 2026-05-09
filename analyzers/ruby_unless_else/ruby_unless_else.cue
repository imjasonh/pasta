// ruby_unless_else flags `unless ... else ... end`. The combination
// of negation (unless) and a then-clause (else) is hard to reason
// about — readers have to mentally invert the condition twice. The
// idiomatic fix is `if cond ... else ... end` with the branches
// swapped, but pasta doesn't auto-fix that here: ruby block bodies
// are awkward to swap surgically (newlines, comments, multi-statement
// branches), so this is lint-only and a human handles the swap.

package ruby_unless_else

import (
	"github.com/imjasonh/pasta/schema"
	rblang "github.com/imjasonh/pasta/lang/ruby"
)

ruby_unless_else: schema.#Analyzer & {
	name:    "ruby_unless_else"
	version: "0.1.0"
	doc:     "Flag `unless` with an `else` branch — invert to `if` and swap"
	facts: {}

	rules: unless_else: {
		name:      "unless_else"
		doc:       "unless cond ... else ... end -> if cond ... else (swapped) ... end"
		languages: [rblang.Name]
		requires: []
		provides: []

		match: {
			node: "unless"
			fields: alternative: {node: "else"}
		}

		diagnose: {
			severity: "warning"
			message:  "`unless` with an `else` branch is hard to read; rewrite as `if` with the branches swapped"
		}
	}
}
