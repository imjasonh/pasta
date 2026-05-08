// python_mutable_default flags function default arguments that are
// mutable (list, dict, set). Pythons evaluates default values once at
// function-definition time, so mutating the default persists across
// calls — a classic Python footgun.
//
// pylint W0102 covers this; encoding here lets it be enforced cross-
// project without configuring pylint.

package python_mutable_default

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

python_mutable_default: schema.#Analyzer & {
	name:    "python_mutable_default"
	version: "0.1.0"
	doc:     "Flag mutable default arguments (list/dict/set literals)"
	facts: {}

	rules: mutable_default: {
		name: "mutable_default"
		doc:  "Default arg is a mutable container literal"
		languages: [pylang.Name]
		requires: []
		provides: []

		match: {
			// Both untyped (`x=[]`) and typed (`x: list = []`) defaults.
			node: ["default_parameter", "typed_default_parameter"]
			fields: {
				value: {
					capture: "val"
					pattern: {
						node: ["list", "dictionary", "set"]
					}
				}
			}
		}

		diagnose: {
			message:  "mutable default argument; use None and create the container inside the function"
			severity: "warning"
		}
	}
}
