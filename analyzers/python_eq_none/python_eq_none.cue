// pyeq_none implements PEP 8 / pylint E711:
//
//   x == None   ->   x is None
//   x != None   ->   x is not None
//
// `==` and `!=` test value equality (which may invoke __eq__);
// `is` / `is not` test identity. For None — a singleton — identity is
// the right semantic. The replacement is always equivalent for None.
//
// Four rules cover the four orientations × operator combinations:
// None-on-right with == or !=, and None-on-left with == or !=.

package python_eq_none

import (
	"pasta.dev/schema"
	"pasta.dev/lang/python"
)

python_eq_none: schema.#Analyzer & {
	name:    "python_eq_none"
	version: "0.1.0"
	doc:     "Replace `== None` / `!= None` with `is None` / `is not None`"
	facts: {}

	rules: {
		// x == None  ->  x is None
		eq_none_right: {
			name:      "eq_none_right"
			doc:       "Replace `<expr> == None` with `<expr> is None`"
			languages: [python.Name]
			requires: []
			provides: []

			match: {
				node: "comparison_operator"
				fields: {operators: {capture: "op"}}
				children: [
					{capture: "lhs"},
					{node: "none"},
				]
				where: [{op: "token_eq", args: ["@op", "=="]}]
			}

			diagnose: {
				message:  "comparison to None should use `is None`"
				severity: "warning"
			}

			rewrite: edits: [{target: "_root", replacement: "@lhs is None"}]
		}

		// x != None  ->  x is not None
		neq_none_right: {
			name:      "neq_none_right"
			doc:       "Replace `<expr> != None` with `<expr> is not None`"
			languages: [python.Name]
			requires: []
			provides: []

			match: {
				node: "comparison_operator"
				fields: {operators: {capture: "op"}}
				children: [
					{capture: "lhs"},
					{node: "none"},
				]
				where: [{op: "token_eq", args: ["@op", "!="]}]
			}

			diagnose: {
				message:  "comparison to None should use `is not None`"
				severity: "warning"
			}

			rewrite: edits: [{target: "_root", replacement: "@lhs is not None"}]
		}

		// None == x  ->  x is None
		eq_none_left: {
			name:      "eq_none_left"
			doc:       "Replace `None == <expr>` with `<expr> is None`"
			languages: [python.Name]
			requires: []
			provides: []

			match: {
				node: "comparison_operator"
				fields: {operators: {capture: "op"}}
				children: [
					{node: "none"},
					{capture: "rhs"},
				]
				where: [{op: "token_eq", args: ["@op", "=="]}]
			}

			diagnose: {
				message:  "comparison to None should use `is None`"
				severity: "warning"
			}

			rewrite: edits: [{target: "_root", replacement: "@rhs is None"}]
		}

		// None != x  ->  x is not None
		neq_none_left: {
			name:      "neq_none_left"
			doc:       "Replace `None != <expr>` with `<expr> is not None`"
			languages: [python.Name]
			requires: []
			provides: []

			match: {
				node: "comparison_operator"
				fields: {operators: {capture: "op"}}
				children: [
					{node: "none"},
					{capture: "rhs"},
				]
				where: [{op: "token_eq", args: ["@op", "!="]}]
			}

			diagnose: {
				message:  "comparison to None should use `is not None`"
				severity: "warning"
			}

			rewrite: edits: [{target: "_root", replacement: "@rhs is not None"}]
		}
	}
}
