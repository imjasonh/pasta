// go_for_range_one_literal flags `for _, v := range []T{x} { ... }`.
// Iterating once over a single-element slice literal is just a
// roundabout assignment: `v := x; ...`. No auto-fix because the
// rewrite needs to inline the loop body, which involves rewriting
// the `_, v :=` binding back to a plain `v :=` AND deleting the for
// keyword + braces. Lint-only is enough to surface the pattern.

package go_for_range_one_literal

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
)

go_for_range_one_literal: schema.#Analyzer & {
	name:    "go_for_range_one_literal"
	version: "0.1.0"
	doc:     "Flag for-range over a single-element slice literal"
	facts: {}

	rules: single_element: {
		name:      "single_element"
		doc:       "for _, v := range []T{x} { ... } is just v := x; ..."
		languages: [golang.Name]
		requires: []
		provides: []

		match: {
			node: "for_statement"
			fields: {
				body: {capture: "body"}
			}
			children: [
				{capture: "range_clause", pattern: {
					node: "range_clause"
					fields: right: {
						capture: "lit"
						pattern: {
							node: "composite_literal"
							fields: body: {
								capture: "lit_body"
								pattern: {
									node: "literal_value"
									children: [{capture: "elem"}]
								}
							}
						}
					}
				}},
				{capture: "loop_body"},
			]
			where: [{op: "named_child_count", args: ["@lit_body", "1"]}]
		}

		diagnose: {
			severity: "hint"
			message:  "single-element slice literal in for-range; bind directly with `v := x`"
		}
	}
}
