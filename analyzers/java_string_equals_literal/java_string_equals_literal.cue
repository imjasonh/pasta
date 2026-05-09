// java_string_equals_literal flips `x.equals("literal")` to
// `"literal".equals(x)`. Putting the literal on the receiver side
// avoids a NullPointerException when x is null. SonarQube and
// PMD both flag the same pattern.

package java_string_equals_literal

import (
	"github.com/imjasonh/pasta/schema"
	javalang "github.com/imjasonh/pasta/lang/java"
)

java_string_equals_literal: schema.#Analyzer & {
	name:    "java_string_equals_literal"
	version: "0.1.0"
	doc:     "Flip x.equals(\"literal\") to \"literal\".equals(x) for NPE safety"
	facts: {}

	rules: equals_literal: {
		name:      "equals_literal"
		doc:       "x.equals(\"literal\") -> \"literal\".equals(x)"
		languages: [javalang.Name]
		requires: []
		provides: []

		match: {
			node: "method_invocation"
			fields: {
				object: {capture: "obj"}
				name: {
					capture: "method"
					pattern: {node: "identifier"}
				}
				arguments: {
					capture: "args"
					pattern: {
						node: "argument_list"
						children: [{
							capture: "arg"
							pattern: {node: "string_literal"}
						}]
					}
				}
			}
			where: [
				{op: "eq", args: ["@method", "equals"]},
				{op: "named_child_count", args: ["@args", "1"]},
			]
		}

		diagnose: {
			severity: "warning"
			message:  "flip `equals` so the literal is the receiver — avoids NPE when the variable is null"
		}

		rewrite: edits: [{
			target:      "_root"
			replacement: "@arg.equals(@obj)"
		}]
	}
}
