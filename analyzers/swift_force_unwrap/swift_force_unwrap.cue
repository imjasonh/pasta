// swift_force_unwrap flags `x!` postfix-bang force unwraps. The
// operator crashes the program if x is nil — almost always a sign
// the author hasn't thought through the failure case. The right fix
// depends on intent: `if let`, `guard let`, `??`, or carefully
// proven non-nil with a comment. No auto-fix; flag for review.

package swift_force_unwrap

import (
	"github.com/imjasonh/pasta/schema"
	swlang "github.com/imjasonh/pasta/lang/swift"
)

swift_force_unwrap: schema.#Analyzer & {
	name:    "swift_force_unwrap"
	version: "0.1.0"
	doc:     "Flag x! force-unwrap operator"
	facts: {}

	rules: force_unwrap: {
		name:      "force_unwrap"
		doc:       "Flag postfix !"
		languages: [swlang.Name]
		requires: []
		provides: []

		match: {
			node: "postfix_expression"
			children: [
				{capture: "value"},
				{capture: "bang", pattern: {node: "bang"}},
			]
		}

		diagnose: {
			severity: "warning"
			message:  "force-unwrap will crash if the value is nil; consider `if let`, `guard let`, or `??`"
		}
	}
}
