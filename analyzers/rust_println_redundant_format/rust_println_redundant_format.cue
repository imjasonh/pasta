// rust_println_redundant_format simplifies redundant `println!`
// invocations where the format string is just `"{}"` and the only
// argument is a string literal — e.g. `println!("{}", "hello")` →
// `println!("hello")`.

package rust_println_redundant_format

import (
	"github.com/imjasonh/pasta/schema"
	rustlang "github.com/imjasonh/pasta/lang/rust"
)

rust_println_redundant_format: schema.#Analyzer & {
	name:    "rust_println_redundant_format"
	version: "0.1.0"
	doc:     "Simplify println!(\"{}\", \"literal\") to println!(\"literal\")"
	facts: {}

	rules: simplify: {
		name:      "simplify"
		doc:       "println!(\"{}\", \"x\") -> println!(\"x\")"
		languages: [rustlang.Name]
		requires: []
		provides: []

		match: {
			node: "macro_invocation"
			children: [
				{capture: "macro", pattern: {node: "identifier"}},
				{
					capture: "args"
					pattern: {
						node: "token_tree"
						children: [
							{capture: "fmt", pattern: {node: "string_literal"}},
							{capture: "arg", pattern: {node: "string_literal"}},
						]
					}
				},
			]
			where: [
				{op: "eq", args: ["@macro", "println"]},
				{op: "eq", args: ["@fmt", "\"{}\""]},
				{op: "named_child_count", args: ["@args", "2"]},
			]
		}

		diagnose: {
			severity: "hint"
			message:  "redundant format placeholder; pass the literal directly"
		}

		// Replace the whole macro invocation with println!(@arg).
		// @arg is the original string literal (with surrounding "...").
		rewrite: edits: [{
			target:      "_root"
			replacement: "println!(@arg)"
		}]
	}
}
