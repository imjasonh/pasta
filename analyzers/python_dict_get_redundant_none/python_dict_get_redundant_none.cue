// python_dict_get_redundant_none rewrites `d.get(k, None)` to `d.get(k)`.
// `dict.get` already returns `None` when the key is missing, so passing
// `None` as the explicit default is redundant.

package python_dict_get_redundant_none

import (
	"pasta.dev/schema"
	pylang "pasta.dev/lang/python"
)

python_dict_get_redundant_none: schema.#Analyzer & {
	name:    "python_dict_get_redundant_none"
	version: "0.1.0"
	doc:     "Drop redundant None default in dict.get(k, None)"
	facts: {}

	rules: redundant_none: {
		name: "redundant_none"
		doc:  "d.get(k, None) -> d.get(k)"
		languages: [pylang.Name]
		requires: []
		provides: []

		match: {
			node: "call"
			fields: {
				function: {
					capture: "func_attr"
					pattern: {
						node: "attribute"
						fields: {
							object:    {capture: "obj"}
							attribute: {capture: "method"}
						}
					}
				}
				arguments: {
					capture: "args"
					pattern: {
						node: "argument_list"
						children: [
							{capture: "key"},
							{capture: "default", pattern: {node: "none"}},
						]
					}
				}
			}
			where: [
				{op: "eq", args: ["@method", "get"]},
				{op: "named_child_count", args: ["@args", "2"]},
			]
		}

		diagnose: {
			message:  "dict.get(k, None) — None is the default; drop the second argument"
			severity: "hint"
		}

		rewrite: edits: [{
			target:      "_root"
			replacement: "@obj.get(@key)"
		}]
	}
}
