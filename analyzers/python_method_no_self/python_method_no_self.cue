// python_method_no_self flags methods declared inside a class whose
// first parameter isn't `self` or `cls`. Demonstrates the
// `ancestor_is` predicate: rule fires only on function definitions
// whose ancestor chain includes a `class_definition`.
//
// Caveats:
//   - Nested functions inside class methods are skipped: ancestor_is
//     is true for them too, but they're not "methods" — they're
//     closures. We accept this false positive for the demo.
//   - `@staticmethod`-decorated methods are valid without self/cls
//     and would fire here. Real implementation should look at
//     decorators; left as a refinement.

package python_method_no_self

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
	pypat "github.com/imjasonh/pasta/patterns/python"
)

python_method_no_self: schema.#Analyzer & {
	name:    "python_method_no_self"
	version: "0.1.0"
	doc:     "Flag class methods whose first parameter isn't self/cls"
	facts: {}

	rules: missing_self: {
		name: "missing_self"
		doc:  "Class method without self/cls as first parameter"
		languages: [pylang.Name]
		requires: []
		provides: []

		match: {
			node: "function_definition"
			fields: {
				name: {capture: "name"}
				parameters: {capture: "params"}
			}
			where: [
				// Only inside class definitions.
				{op: "ancestor_is", args: ["@_root", pypat.ClassLikeTypes]},
				// First param isn't self/cls. The empty-params case
				// `()` also matches (regex doesn't match it, so
				// not_matches fires).
				{
					op: "not_matches"
					args: ["@params", "^\\((self|cls)([,)]|: )"]
				},
			]
		}

		diagnose: {
			message:  "method `@name` is missing `self` (or `cls`) as first parameter"
			severity: "warning"
		}
	}
}
