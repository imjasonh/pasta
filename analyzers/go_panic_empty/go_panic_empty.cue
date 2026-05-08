// go_panic_empty flags `panic("")` calls — an empty string panic
// produces a useless stack trace. If you want to crash, give the
// reader something to work with.

package go_panic_empty

import (
	"pasta.dev/schema"
	golang "pasta.dev/lang/go"
)

go_panic_empty: schema.#Analyzer & {
	name:    "go_panic_empty"
	version: "0.1.0"
	doc:     "Flag panic(\"\") with empty string"
	facts: {}

	rules: empty_panic: {
		name: "empty_panic"
		doc:  "panic with empty string is uninformative"
		languages: [golang.Name]
		requires: []
		provides: []

		match: {
			node: "call_expression"
			fields: {
				function: {capture: "fn", pattern: {node: "identifier"}}
				arguments: {
					node: "argument_list"
					children: [{
						capture: "msg"
						pattern: {
							node: "interpreted_string_literal"
						}
					}]
				}
			}
			where: [
				{op: "eq", args: ["@fn", "panic"]},
				{op: "matches", args: ["@msg", "^\"\"$"]},
			]
		}

		diagnose: {
			message:  "panic(\"\") gives no information; include a message"
			severity: "warning"
		}
	}
}
