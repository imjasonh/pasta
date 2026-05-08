// js_array_concat_spread rewrites `[].concat(x)` to the spread form
// `[...x]`. Same kind of cleanup as Object.assign → spread, but for
// arrays. Limited to the two-arg form (empty literal + one source);
// multi-arg `.concat(x, y)` would need `[...x, ...y]` which the
// rewrite syntax doesn't easily express today.

package js_array_concat_spread

import (
	"pasta.dev/schema"
	jslang "pasta.dev/lang/javascript"
)

js_array_concat_spread: schema.#Analyzer & {
	name:    "js_array_concat_spread"
	version: "0.1.0"
	doc:     "Rewrite [].concat(x) to [...x]"
	facts: {}

	rules: array_concat_to_spread: {
		name: "array_concat_to_spread"
		doc:  "[].concat(x) -> [...x]"
		languages: [jslang.Name]
		requires: []
		provides: []

		match: {
			node: "call_expression"
			fields: {
				function: {
					node: "member_expression"
					fields: {
						object: {
							capture: "obj"
							pattern: {node: "array"}
						}
						property: {capture: "prop", pattern: {node: "property_identifier"}}
					}
				}
				arguments: {
					capture: "args"
					pattern: {
						node: "arguments"
						children: [{capture: "src"}]
					}
				}
			}
			where: [
				{op: "eq", args: ["@prop", "concat"]},
				// Object must be `[]` empty array literal.
				{op: "empty", args: ["@obj"]},
				// Exactly one argument.
				{op: "named_child_count", args: ["@args", "1"]},
			]
		}

		diagnose: {
			message:  "[].concat(x) can be written as [...x]"
			severity: "hint"
		}

		rewrite: edits: [{
			target:      "_root"
			replacement: "[...@src]"
		}]
	}
}
