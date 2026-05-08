// rust_dbg_macro flags committed `dbg!(...)` macro invocations. The
// `dbg!` macro is a debugging convenience that prints to stderr and
// returns its argument; it almost never belongs in committed code.
// Often clippy's `dbg_macro` lint is left at warn-level by default; we
// fire as a hard error so it shows up in CI.

package rust_dbg_macro

import (
	"pasta.dev/schema"
	rustlang "pasta.dev/lang/rust"
)

rust_dbg_macro: schema.#Analyzer & {
	name:    "rust_dbg_macro"
	version: "0.1.0"
	doc:     "Flag committed dbg!() invocations"
	facts: {}

	rules: dbg_call: {
		name: "dbg_call"
		doc:  "dbg!() should not appear in committed code"
		languages: [rustlang.Name]
		requires: []
		provides: []

		match: {
			node: "macro_invocation"
			fields: {
				macro: {capture: "name"}
			}
			where: [{op: "eq", args: ["@name", "dbg"]}]
		}

		diagnose: {
			message:  "dbg!() macro invocation — remove before committing"
			severity: "error"
		}

		// Auto-fix: replace `dbg!(EXPR)` with `EXPR`. The macro
		// invocation's text is `dbg!(...)`; we replace its byte range
		// with the same text after trimming the leading `dbg!(` (5
		// bytes) and the trailing `)` (1 byte). dbg! returns its
		// argument unchanged, so this is semantically equivalent.
		rewrite: edits: [{
			target:      "_root"
			replacement: "@_root"
			trim_start:  5
			trim_end:    1
		}]
	}
}
