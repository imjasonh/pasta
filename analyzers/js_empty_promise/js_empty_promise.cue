// js_empty_promise flags `new Promise(() => {})` — a Promise whose
// executor never resolves or rejects. The Promise hangs forever. Almost
// always a bug (or test scaffolding left in production).

package js_empty_promise

import (
	"github.com/imjasonh/pasta/schema"
	jslang "github.com/imjasonh/pasta/lang/javascript"
)

js_empty_promise: schema.#Analyzer & {
	name:    "js_empty_promise"
	version: "0.1.0"
	doc:     "Flag `new Promise(() => {})` — a never-resolving Promise"
	facts: {}

	rules: empty_executor: {
		name: "empty_executor"
		doc:  "Promise constructor with empty executor body"
		languages: [jslang.Name]
		requires: []
		provides: []

		match: {
			node: "new_expression"
			fields: {
				constructor: {capture: "ctor"}
				arguments: {
					node: "arguments"
					children: [{
						capture: "exec"
						pattern: {
							node: "arrow_function"
							fields: {
								body: {
									capture: "body"
									pattern: {
										node: "statement_block"
									}
								}
							}
						}
					}]
				}
			}
			where: [
				{op: "eq", args: ["@ctor", "Promise"]},
				{op: "empty", args: ["@body"]},
			]
		}

		diagnose: {
			message:  "Promise with empty executor never resolves or rejects"
			severity: "error"
		}
	}
}
