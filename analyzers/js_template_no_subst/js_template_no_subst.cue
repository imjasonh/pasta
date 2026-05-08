// js_template_no_subst rewrites template literals that have no `${...}`
// interpolation to plain single-quoted strings. Backtick strings without
// substitutions are still valid JavaScript, but they invite future
// drift (someone adds `${x}` instead of switching the type) and are
// harder to grep for.

package js_template_no_subst

import (
	"pasta.dev/schema"
	jslang "pasta.dev/lang/javascript"
)

js_template_no_subst: schema.#Analyzer & {
	name:    "js_template_no_subst"
	version: "0.1.0"
	doc:     "Replace backtick template strings with plain strings when no interpolation"
	facts: {}

	rules: no_substitution: {
		name: "no_substitution"
		doc:  "`...` -> '...' when there's no ${...}"
		languages: [jslang.Name]
		requires: []
		provides: []

		match: {
			node: "template_string"
			where: [
				// No `${` in the literal text.
				{op: "not_matches", args: ["@_root", "\\$\\{"]},
				// Skip multi-line templates (often deliberate); these
				// would require switching to multi-string concat.
				{op: "not_matches", args: ["@_root", "\\n"]},
				// Skip strings containing single quotes (would need
				// escaping; out of scope for this naive rewrite).
				{op: "not_matches", args: ["@_root", "'"]},
			]
		}

		diagnose: {
			message:  "template literal without interpolation; use a regular string"
			severity: "hint"
		}

		// Strip the backticks (first and last char of the captured
		// node) and emit a single-quoted string with the same content.
		// We use within-edit text manipulation: replace leading and
		// trailing backticks with single quotes.
		rewrite: edits: [
			{within: "_root", token: "`", replace_with: "'"},
			{within: "_root", token: "`", replace_with: "'"},
			{target: "_root", replacement: "@_root"},
		]
	}
}
