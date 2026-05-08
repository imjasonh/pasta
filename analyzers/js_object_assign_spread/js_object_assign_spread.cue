// js_object_assign_spread rewrites `Object.assign({}, src)` to the
// spread form `{...src}`. The spread form is shorter, doesn't allocate
// an intermediate empty object as the target, and is more readable.
// Limited here to the single-source two-arg form because the rewrite
// for N sources would need an N-way join (`{...a, ...b, ...c}`) which
// the current CUE rewrite syntax can't easily express.

package js_object_assign_spread

import (
	"pasta.dev/schema"
	jslang "pasta.dev/lang/javascript"
)

js_object_assign_spread: schema.#Analyzer & {
	name:    "js_object_assign_spread"
	version: "0.1.0"
	doc:     "Rewrite Object.assign({}, x) to {...x}"
	facts: {}

	rules: object_assign_to_spread: {
		name: "object_assign_to_spread"
		doc:  "Object.assign({}, src) -> {...src}"
		languages: [jslang.Name]
		requires: []
		provides: []

		match: {
			node: "call_expression"
			fields: {
				function: {
					node: "member_expression"
					fields: {
						object:   {capture: "obj", pattern: {node: "identifier"}}
						property: {capture: "prop", pattern: {node: "property_identifier"}}
					}
				}
				arguments: {
					capture: "args"
					pattern: {
						node: "arguments"
						children: [
							{
								capture: "target"
								pattern: {
									node: "object"
								}
							},
							{capture: "src"},
						]
					}
				}
			}
			where: [
				{op: "eq", args: ["@obj", "Object"]},
				{op: "eq", args: ["@prop", "assign"]},
				// Exactly two arguments.
				{op: "named_child_count", args: ["@args", "2"]},
				// Target must be `{}` empty object literal.
				{op: "empty", args: ["@target"]},
			]
		}

		diagnose: {
			message:  "Object.assign({}, x) can be written as {...x}"
			severity: "hint"
		}

		rewrite: edits: [{
			target:      "_root"
			replacement: "{...@src}"
		}]
	}
}
