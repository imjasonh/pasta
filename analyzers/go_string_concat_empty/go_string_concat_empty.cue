// go_string_concat_empty rewrites `"" + x` and `x + ""` to just `x`.
// Concatenating an empty string is a no-op; the inserted operand is
// almost always leftover from a refactor. This is a pure
// simplification — `staticcheck`'s SA4020 covers similar ground.

package go_string_concat_empty

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
)

_concatRule: {
	_emptySide: "left" | "right"

	out: {
		languages: [golang.Name]
		requires: []
		provides: []

		match: {
			node: "binary_expression"
			fields: {
				operator: {capture: "op"}
				if _emptySide == "left" {
					left:  {pattern: {node: "interpreted_string_literal"}, capture: "empty"}
					right: {capture: "keep"}
				}
				if _emptySide == "right" {
					left:  {capture: "keep"}
					right: {pattern: {node: "interpreted_string_literal"}, capture: "empty"}
				}
			}
			where: [
				{op: "token_eq", args: ["@op", "+"]},
				{op: "eq", args: ["@empty", "\"\""]},
				// Skip when both sides are empty — both empty_left
				// and empty_right would otherwise emit overlapping
				// rewrites for `"" + ""`.
				{op: "neq", args: ["@keep", "\"\""]},
			]
		}

		diagnose: {
			severity: "hint"
			message:  "concatenating with an empty string is a no-op"
		}

		// Replace _root with just the kept operand. @keep's bytes
		// (and any comments inside it) are preserved verbatim by
		// interpolation.
		rewrite: edits: [{
			target:      "_root"
			replacement: "@keep"
		}]
	}
}

go_string_concat_empty: schema.#Analyzer & {
	name:    "go_string_concat_empty"
	version: "0.1.0"
	doc:     "Drop empty-string operands from string concatenation"
	facts: {}

	rules: {
		empty_left: (_concatRule & {_emptySide: "left"}).out & {
			name: "empty_left"
			doc:  "\"\" + x -> x"
		}
		empty_right: (_concatRule & {_emptySide: "right"}).out & {
			name: "empty_right"
			doc:  "x + \"\" -> x"
		}
	}
}
