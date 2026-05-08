// rust_println_panic flags adjacent `println!(...)` followed by
// `panic!(...)` — the println is redundant since panic! already prints
// the message to stderr before unwinding. The pattern almost always
// indicates a copy-paste error or print-before-error left over from
// debugging.

package rust_println_panic

import (
	"pasta.dev/schema"
	golang "pasta.dev/lang/rust"
)

rust_println_panic: schema.#Analyzer & {
	name:    "rust_println_panic"
	version: "0.1.0"
	doc:     "Flag adjacent println!() and panic!() — the println is redundant"
	facts: {}

	rules: pair: {
		name: "pair"
		doc:  "println!() right before panic!() — drop the println"
		languages: [golang.Name]
		requires: []
		provides: []

		match: {
			node: "block"
			adjacent: [
				{
					capture: "println"
					pattern: {
						node: "expression_statement"
						children: [{
							node: "macro_invocation"
							fields: {
								macro: {capture: "p_macro"}
							}
						}]
					}
				},
				{
					capture: "panic"
					pattern: {
						node: "expression_statement"
						children: [{
							node: "macro_invocation"
							fields: {
								macro: {capture: "panic_macro"}
							}
						}]
					}
				},
			]
			where: [
				{op: "matches", args: ["@p_macro", "^(println|eprintln|print|eprint)$"]},
				{op: "matches", args: ["@panic_macro", "^panic$"]},
			]
		}

		diagnose: {
			message:  "println! before panic! — panic! already prints the message"
			severity: "warning"
		}

		// Delete the println line and the newline that follows it.
		// `delete_from: println` (start) to `delete_to: panic` (start)
		// removes everything from the start of the println statement
		// through the leading whitespace of the panic line, leaving
		// just the panic on its own indented line.
		rewrite: edits: [{
			delete_from: "println"
			delete_to:   "panic"
		}]
	}
}
