// css_zero_unit drops the unit suffix from `0px` / `0em` / `0%`
// values. The unit on zero is meaningless; CSS treats `0` and `0px`
// identically. Stylelint's `length-zero-no-unit` covers the same
// ground.

package css_zero_unit

import (
	"github.com/imjasonh/pasta/schema"
	csslang "github.com/imjasonh/pasta/lang/css"
)

css_zero_unit: schema.#Analyzer & {
	name:    "css_zero_unit"
	version: "0.1.0"
	doc:     "Drop unit from 0px / 0em / 0% (the unit is meaningless on zero)"
	facts: {}

	rules: zero_unit: {
		name:      "zero_unit"
		doc:       "0px -> 0"
		languages: [csslang.Name]
		requires: []
		provides: []

		match: {
			node: "integer_value"
			children: [{
				capture: "unit"
				pattern: {node: "unit"}
			}]
			where: [{op: "matches", args: ["@_root", "^0(px|em|rem|pt|pc|in|cm|mm|ex|ch|vh|vw|vmin|vmax|%)$"]}]
		}

		diagnose: {
			severity: "hint"
			message:  "the unit on zero is meaningless; just write `0`"
		}

		// Replace the entire integer_value with just `0`.
		rewrite: edits: [{
			target:      "_root"
			replacement: "0"
		}]
	}
}
