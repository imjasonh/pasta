// go_empty_else flags `if cond { ... } else { /* nothing */ }` and
// rewrites it to drop the empty else. Often a leftover from a refactor
// that removed the else body but forgot the braces.

package go_empty_else

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
)

go_empty_else: schema.#Analyzer & {
	name:    "go_empty_else"
	version: "0.1.0"
	doc:     "Flag and remove empty else branches"
	facts: {}

	rules: empty_else: {
		name: "empty_else"
		doc:  "if/else with empty else branch — drop the else"
		languages: [golang.Name]
		requires: []
		provides: []

		match: {
			node: "if_statement"
			fields: {
				consequence: {capture: "cons"}
				alternative: {
					capture: "alt"
					pattern: {node: "block"}
				}
			}
			where: [{op: "empty", args: ["@alt"]}]
		}

		diagnose: {
			message:  "empty else branch can be removed"
			severity: "hint"
		}

		// Delete the bytes from the end of the consequence (just after
		// the closing `}`) through the end of the empty alternative
		// (the `}` of the empty else). This removes ` else { }` while
		// preserving the consequence's closing brace.
		rewrite: edits: [{
			delete_from:      "cons"
			delete_from_end:  true
			delete_to:        "alt"
			delete_to_end:    true
		}]
	}
}
