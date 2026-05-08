// hardcoded_localhost flags string literals containing localhost-style
// URLs or 127.0.0.1 addresses. Often a leftover from local development
// that ends up in committed code.

package hardcoded_localhost

import "github.com/imjasonh/pasta/schema"

hardcoded_localhost: schema.#Analyzer & {
	name:    "hardcoded_localhost"
	version: "0.1.0"
	doc:     "Flag string literals containing localhost or 127.0.0.1 URLs"
	facts: {}

	rules: localhost: {
		name:      "localhost"
		doc:       "String literal contains localhost or 127.0.0.1 URL"
		languages: ["*"]
		requires: []
		provides: []
		match: {
			node: ["string", "string_literal", "interpreted_string_literal", "raw_string_literal", "string_fragment"]
			where: [{
				op: "matches"
				args: ["@_root", "(localhost|127\\.0\\.0\\.1|0\\.0\\.0\\.0)(:[0-9]+)?"]
			}]
		}
		diagnose: {
			message:  "string contains localhost/127.0.0.1 — likely a leftover from local dev"
			severity: "warning"
		}
	}
}
