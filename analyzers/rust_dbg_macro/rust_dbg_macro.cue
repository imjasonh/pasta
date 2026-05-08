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

		// Auto-fix not implemented: the obvious `dbg!(EXPR)` → `EXPR`
		// rewrite requires extracting EXPR from inside the macro's
		// `(...)`. Pasta's `within` edit replaces the FIRST literal
		// match, so for `dbg!(foo().bar())` it strips the inner `)`
		// rather than the outer one. A clean rewrite needs byte-range
		// arithmetic on the captured token_tree (e.g. `target: tt with
		// trim_first: 1, trim_last: 1`) — a small framework extension.
		// TODO(framework): byte-range trim on capture interpolation.
	}
}
