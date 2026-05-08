// todo_format flags TODO/FIXME/XXX/HACK comments that lack an owner or
// issue link in parentheses. Encourages `TODO(jason): ...` or
// `TODO(#123): ...` over anonymous TODOs that nobody owns.
//
// Cross-language by design: matches every node type tree-sitter uses
// for comments. Adding a language with a new comment node name only
// requires extending the `node` union below.

package todo_format

import "github.com/imjasonh/pasta/schema"

todo_format: schema.#Analyzer & {
	name:    "todo_format"
	version: "0.1.0"
	doc:     "Flag TODO/FIXME comments without an owner: TODO(name): ..."
	facts: {}

	rules: anonymous_todo: {
		name:      "anonymous_todo"
		doc:       "TODO/FIXME/XXX/HACK comments must include an owner in parentheses"
		languages: ["*"]
		requires: []
		provides: []

		match: {
			node: ["comment", "line_comment", "block_comment", "doc_comment"]
			where: [{
				op: "matches"
				// Matches a comment that contains a TODO-keyword that is
				// NOT immediately followed by an opening parenthesis.
				// (?-s) is regex flag for non-multiline; .* across lines.
				args: ["@_root", "\\b(TODO|FIXME|XXX|HACK)\\b(?:[^(]|$)"]
			}]
		}

		diagnose: {
			message:  "TODO/FIXME without owner; use `TODO(name): ...` or `TODO(#issue): ...`"
			severity: "warning"
		}
	}
}
