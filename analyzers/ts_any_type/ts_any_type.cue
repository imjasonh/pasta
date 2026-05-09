// ts_any_type flags `: any` annotations. `any` opts out of TypeScript's
// type checking, defeating the language's primary value. Most of the
// time it's a sign that the author hasn't yet figured out the real
// type. There's no safe auto-fix — replacing `any` with `unknown`
// would catch many cases but breaks code that relies on the looser
// behavior; the right type is per-call-site work for a human.

package ts_any_type

import (
	"github.com/imjasonh/pasta/schema"
	tslang "github.com/imjasonh/pasta/lang/typescript"
)

ts_any_type: schema.#Analyzer & {
	name:    "ts_any_type"
	version: "0.1.0"
	doc:     "Flag `: any` type annotations"
	facts: {}

	rules: any_type: {
		name:      "any_type"
		doc:       "Flag `any` predefined type"
		languages: [tslang.Name]
		requires: []
		provides: []

		match: {
			node: "predefined_type"
			where: [{op: "eq", args: ["@_root", "any"]}]
		}

		diagnose: {
			message:  "`any` defeats TypeScript's type checking; use a specific type or `unknown`"
			severity: "warning"
		}
	}
}
