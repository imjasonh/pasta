// c_gets_unsafe flags `gets()` calls. The function reads from stdin
// without any bound on the destination buffer; CWE-242. C11 removed
// it from the standard library; lots of older code still calls it.
// Use `fgets(buf, sizeof(buf), stdin)` instead.

package c_gets_unsafe

import (
	"github.com/imjasonh/pasta/schema"
	clang "github.com/imjasonh/pasta/lang/c"
)

c_gets_unsafe: schema.#Analyzer & {
	name:    "c_gets_unsafe"
	version: "0.1.0"
	doc:     "Flag gets() — CWE-242, removed in C11"
	facts: {}

	rules: gets_call: {
		name:      "gets_call"
		doc:       "gets(buf) is unsafe; use fgets()"
		languages: [clang.Name]
		requires: []
		provides: []

		match: {
			node: "call_expression"
			fields: function: {
				capture: "fn"
				pattern: {node: "identifier"}
			}
			where: [{op: "eq", args: ["@fn", "gets"]}]
		}

		diagnose: {
			severity: "error"
			message:  "gets() is unsafe (CWE-242, removed in C11); use fgets() instead"
		}
	}
}
