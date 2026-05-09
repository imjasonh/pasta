// php_loose_equality rewrites `==` / `!=` to `===` / `!==`. PHP's
// loose equality coerces operand types in surprising ways
// (`"0" == false` is true; `"abc" == 0` was true before PHP 8).
// Strict equality compares without coercion. Mirror of
// js_double_equals.

package php_loose_equality

import (
	"github.com/imjasonh/pasta/schema"
	phplang "github.com/imjasonh/pasta/lang/php"
)

_eqRule: {
	_from: "==" | "!="
	_to:   "===" | "!=="

	out: {
		languages: [phplang.Name]
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

		rewrite: edits: [{
			target:      "op"
			replacement: _to
		}]
	}
}

php_loose_equality: schema.#Analyzer & {
	name:    "php_loose_equality"
	version: "0.1.0"
	doc:     "Replace == / != with === / !=="
	facts: {}

	rules: {
		eq:  (_eqRule & {_from: "==", _to: "==="}).out & {name: "eq",  doc: "== -> ==="}
		neq: (_eqRule & {_from: "!=", _to: "!=="}).out & {name: "neq", doc: "!= -> !=="}
	}
}
