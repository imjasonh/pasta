// python_assert_tuple flags `assert (cond, "msg")` — a tuple-form
// assertion that's ALWAYS true (non-empty tuples are truthy). The
// author meant `assert cond, "msg"` (assertion with message). Real
// Python footgun, hard to spot in review.
//
// pylint W0129 (`assert-on-tuple`) covers this; encoding here gives
// a usable rewrite.

package python_assert_tuple

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

python_assert_tuple: schema.#Analyzer & {
	name:    "python_assert_tuple"
	version: "0.1.0"
	doc:     "Flag assert (cond, msg) — always true"
	facts: {}

	rules: tuple_assert: {
		name: "tuple_assert"
		doc:  "assert (cond, msg) is a tuple-truthiness bug"
		languages: [pylang.Name]
		requires: []
		provides: []

		match: {
			node: "assert_statement"
			children: [{
				capture: "tup"
				pattern: {
					node: "tuple"
					children: [
						{capture: "cond"},
						{capture: "msg"},
					]
				}
			}]
			where: [
				// Exactly two elements in the tuple.
				{op: "named_child_count", args: ["@tup", "2"]},
			]
		}

		diagnose: {
			message:  "assert with a tuple argument is always truthy; use `assert cond, msg`"
			severity: "error"
		}

		rewrite: edits: [{
			target:      "tup"
			replacement: "@cond, @msg"
		}]
	}
}
