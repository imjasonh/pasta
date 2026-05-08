// ts_array_type_style rewrites the generic-array form `Array<T>` to
// the shorthand `T[]`. The two are interchangeable; teams pick one and
// stick with it. typescript-eslint has this lint but with the opposite
// default in some configs; encoding it as a pasta rule lets a project
// commit to a style consistently.

package ts_array_type_style

import (
	"pasta.dev/schema"
	tslang "pasta.dev/lang/typescript"
)

ts_array_type_style: schema.#Analyzer & {
	name:    "ts_array_type_style"
	version: "0.1.0"
	doc:     "Rewrite Array<T> to T[]"
	facts: {}

	rules: prefer_shorthand: {
		name: "prefer_shorthand"
		doc:  "Array<T> -> T[]"
		languages: [tslang.Name]
		requires: []
		provides: []

		match: {
			node: "generic_type"
			fields: {
				name: {capture: "outer", pattern: {node: "type_identifier"}}
				type_arguments: {
					node: "type_arguments"
					children: [{capture: "inner"}]
				}
			}
			where: [
				{op: "eq", args: ["@outer", "Array"]},
			]
		}

		diagnose: {
			message:  "Array<T> can be written as T[]"
			severity: "hint"
		}

		rewrite: edits: [{
			target:      "_root"
			replacement: "@inner[]"
		}]
	}
}
