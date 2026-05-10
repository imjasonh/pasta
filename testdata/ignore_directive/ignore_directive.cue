// ignore_directive exercises the per-line `pasta:ignore` suppression
// directive. Two unrelated rules fire on the same shape (a `print`
// call) so we can verify selective suppression by name.

package ignore_directive

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

_printRule: {
	_msg: string
	out: {
		languages: [pylang.Name]
		requires: []
		provides: []
		match: {
			node: "call"
			fields: function: {capture: "fn"}
			where: [{op: "token_eq", args: ["@fn", "print"]}]
		}
		diagnose: message: _msg
	}
}

ignore_directive: schema.#Analyzer & {
	name:    "ignore_directive"
	version: "0.1.0"
	doc:     "Demo of per-line pasta:ignore directives"
	facts: {}

	rules: {
		print_call: (_printRule & {_msg: "print A"}).out & {
			name: "print_call"
			doc:  "first rule that fires on print()"
		}
		other_print: (_printRule & {_msg: "print B"}).out & {
			name: "other_print"
			doc:  "second rule that fires on print() — used to test selective suppression"
		}
	}
}
