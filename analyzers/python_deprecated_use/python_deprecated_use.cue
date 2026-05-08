// python_deprecated_use flags calls to functions decorated with
// `@deprecated` (the convention from PEP 702 / typing_extensions or the
// `deprecated` PyPI package).
//
// Pass 1: walk decorated_definition wrappers; if any decorator's
//         expression is `deprecated` or `deprecated(...)`, emit a
//         fact attached to the inner function's name.
// Pass 2: walk call expressions; if the callee is an identifier with
//         the deprecated fact, warn.

package python_deprecated_use

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

python_deprecated_use: schema.#Analyzer & {
	name:    "python_deprecated_use"
	version: "0.1.0"
	doc:     "Flag calls to @deprecated functions"

	facts: {
		deprecated: {kind: "deprecated"}
	}

	rules: {
		mark_deprecated: {
			name:      "mark_deprecated"
			doc:       "Tag function definitions wrapped in @deprecated"
			languages: [pylang.Name]
			requires: []
			provides: ["deprecated"]

			match: {
				node: "decorated_definition"
				fields: {
					definition: {
						node: "function_definition"
						fields: {
							name: {capture: "fn_name"}
						}
					}
				}
				children: [{
					capture: "deco"
					pattern: {
						node: "decorator"
					}
				}]
				where: [{
					op:   "matches"
					args: ["@deco", "@deprecated"]
				}]
			}

			emit: [{
				fact:   "deprecated"
				attach: "fn_name"
			}]
		}

		find_calls: {
			name:      "find_calls"
			doc:       "Flag call sites of deprecated functions"
			languages: [pylang.Name]
			requires: ["deprecated"]
			provides: []

			match: {
				node: "call"
				fields: {
					function: {
						capture: "callee"
						pattern: {node: "identifier"}
					}
				}
				where: [{op: "has_fact", args: ["@callee", "deprecated"]}]
			}

			diagnose: {
				message:  "@callee is deprecated"
				severity: "warning"
			}
		}
	}
}
