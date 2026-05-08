// python_eq_none implements PEP 8 / pylint E711:
//
//   x == None   ->   x is None
//   x != None   ->   x is not None
//
// `==` and `!=` test value equality (which may invoke __eq__);
// `is` / `is not` test identity. For None — a singleton — identity is
// the right semantic. The replacement is always equivalent for None.
//
// Each rule rewrites the captured operator node in place (`==` -> `is`,
// `!=` -> `is not`) — the surrounding expression bytes are untouched,
// so any comments or whitespace around the operator survive.

package python_eq_none

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

// _isExprFor maps a comparison op to its `is`/`is not` replacement.
_isExprFor: {
	"==": "is"
	"!=": "is not"
}

// _noneCmpRule builds a rule that matches `<operand> <op> None` (or
// the mirror form, depending on the children list passed in) and
// replaces the operator token in place. The result is the value of
// the underscore-suffixed `out` field, which fits #Rule cleanly
// without leaking the helper's input fields into the schema.
_noneCmpRule: {
	_op:     "==" | "!="
	_isExpr: _isExprFor[_op]
	_children: [...]

	out: {
		languages: [pylang.Name]
		requires: []
		provides: []

		match: {
			node:   "comparison_operator"
			fields: {operators: {capture: "operator"}}
			children: _children
			where: [{op: "token_eq", args: ["@operator", _op]}]
		}

		diagnose: {
			severity: "warning"
			message:  "comparison to None should use `\(_isExpr) None`"
		}

		// Surgical: replace ONLY the operator token. Whitespace and
		// comments around it (e.g. `x /* note */ == None`) are
		// preserved because their bytes aren't touched.
		rewrite: edits: [{
			target:      "operator"
			replacement: _isExpr
		}]
	}
}

python_eq_none: schema.#Analyzer & {
	name:    "python_eq_none"
	version: "0.1.0"
	doc:     "Replace `== None` / `!= None` with `is None` / `is not None`"
	facts: {}

	rules: {
		eq_none_right: (_noneCmpRule & {
			_op: "=="
			_children: [{capture: "operand"}, {node: "none"}]
		}).out & {name: "eq_none_right", doc: "x == None -> x is None"}

		neq_none_right: (_noneCmpRule & {
			_op: "!="
			_children: [{capture: "operand"}, {node: "none"}]
		}).out & {name: "neq_none_right", doc: "x != None -> x is not None"}

		eq_none_left: (_noneCmpRule & {
			_op: "=="
			_children: [{node: "none"}, {capture: "operand"}]
		}).out & {name: "eq_none_left", doc: "None == x -> x is None"}

		neq_none_left: (_noneCmpRule & {
			_op: "!="
			_children: [{node: "none"}, {capture: "operand"}]
		}).out & {name: "neq_none_left", doc: "None != x -> x is not None"}
	}
}
