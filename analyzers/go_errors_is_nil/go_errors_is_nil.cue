// go_errors_is_nil rewrites `errors.Is(err, nil)` to `err == nil`.
// `errors.Is` was designed for sentinel comparisons; comparing to nil
// goes through unnecessary indirection and is slower. The plain
// equality is more idiomatic and identical in semantics.

package go_errors_is_nil

import (
	"pasta.dev/schema"
	golang "pasta.dev/lang/go"
	gopat "pasta.dev/patterns/go"
)

go_errors_is_nil: schema.#Analyzer & {
	name:    "go_errors_is_nil"
	version: "0.1.0"
	doc:     "Rewrite errors.Is(x, nil) to x == nil"
	facts: {}

	rules: simplify: {
		name: "simplify"
		doc:  "errors.Is(x, nil) -> x == nil"
		languages: [golang.Name]
		requires: []
		provides: []

		match: gopat.PackageCall & {
			fields: arguments: {
				node: "argument_list"
				children: [
					{capture: "err"},
					{node: "nil"},
				]
			}
			where: [
				{op: "eq", args: ["@pkg", "errors"]},
				{op: "eq", args: ["@fn", "Is"]},
			]
		}

		diagnose: {
			message:  "errors.Is(x, nil) can be simplified to x == nil"
			severity: "hint"
		}

		rewrite: edits: [{target: "_root", replacement: "@err == nil"}]
	}
}
