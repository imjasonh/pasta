// python_isinstance_singleton rewrites `isinstance(x, (T,))` (a tuple
// with a single type) to `isinstance(x, T)`. The tuple form is
// idiomatic when checking against multiple types but pointless when
// there's only one.

package python_isinstance_singleton

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

python_isinstance_singleton: schema.#Analyzer & {
	name:    "python_isinstance_singleton"
	version: "0.1.0"
	doc:     "Rewrite isinstance(x, (T,)) to isinstance(x, T)"
	facts: {}

	rules: singleton_tuple: {
		name: "singleton_tuple"
		doc:  "Single-element tuple in isinstance — drop the tuple"
		languages: [pylang.Name]
		requires: []
		provides: []

		match: {
			node: "call"
			fields: {
				function: {capture: "fn"}
				arguments: {
					node: "argument_list"
					children: [
						{capture: "obj"},
						{
							capture: "type_tuple"
							pattern: {
								node: "tuple"
								children: [{capture: "inner_type"}]
							}
						},
					]
				}
			}
			where: [
				{op: "eq", args: ["@fn", "isinstance"]},
				{op: "named_child_count", args: ["@type_tuple", "1"]},
			]
		}

		diagnose: {
			message:  "single-element tuple in isinstance(); pass the type directly"
			severity: "hint"
		}

		// Replace the entire (T,) tuple with just T.
		rewrite: edits: [{
			target:      "type_tuple"
			replacement: "@inner_type"
		}]
	}
}
