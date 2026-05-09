// js_double_equals rewrites `==` / `!=` to `===` / `!==`. JavaScript's
// loose equality coerces operand types, which is almost always
// surprising; the strict variants compare without coercion. ESLint
// `eqeqeq` covers the same ground; this is a structural auto-fix.

package js_double_equals

import (
	"github.com/imjasonh/pasta/schema"
	jslang "github.com/imjasonh/pasta/lang/javascript"
)

_eqRule: {
	_from: "==" | "!="
	_to:   "===" | "!=="

	out: {
		languages: [jslang.Name]
		requires: []
		provides: []

		match: {
			node: "binary_expression"
			fields: operator: {capture: "op"}
			where: [{op: "token_eq", args: ["@op", _from]}]
		}

		diagnose: {
			severity: "warning"
			message:  "use strict equality `\(_to)` instead of `\(_from)`"
		}

		// Surgical: replace only the operator token. Operand bytes
		// (and any comments around them) are untouched.
		rewrite: edits: [{
			target:      "op"
			replacement: _to
		}]
	}
}

js_double_equals: schema.#Analyzer & {
	name:    "js_double_equals"
	version: "0.1.0"
	doc:     "Replace == / != with === / !=="
	facts: {}

	rules: {
		eq:  (_eqRule & {_from: "==", _to: "==="}).out & {name: "eq",  doc: "== -> ==="}
		neq: (_eqRule & {_from: "!=", _to: "!=="}).out & {name: "neq", doc: "!= -> !=="}
	}
}
