// python_explicit_object_base drops the redundant `(object)` base
// from class definitions. In Python 3 every class is a new-style
// class implicitly inheriting from object; the explicit base is a
// Python 2 holdover that adds visual noise.

package python_explicit_object_base

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

python_explicit_object_base: schema.#Analyzer & {
	name:    "python_explicit_object_base"
	version: "0.1.0"
	doc:     "Drop redundant `(object)` base in class definitions"
	facts: {}

	rules: drop_object: {
		name:      "drop_object"
		doc:       "class Foo(object): -> class Foo:"
		languages: [pylang.Name]
		requires: []
		provides: []

		match: {
			node: "class_definition"
			fields: superclasses: {
				capture: "supers"
				pattern: {
					node: "argument_list"
					children: [{
						capture: "obj"
						pattern: {node: "identifier"}
					}]
				}
			}
			// Constraints at the outer pattern's where run AFTER
			// field captures are bound, so @supers and @obj are
			// available here.
			where: [
				{op: "named_child_count", args: ["@supers", "1"]},
				{op: "eq", args: ["@obj", "object"]},
			]
		}

		diagnose: {
			message:  "explicit `(object)` base is redundant in Python 3"
			severity: "hint"
		}

		// Surgical: delete the entire `(object)` argument list.
		rewrite: edits: [{
			target:      "supers"
			replacement: ""
		}]
	}
}
