// js_var_to_let rewrites `var` declarations to `let`. `var` is
// function-scoped and hoisted; `let` is block-scoped and not. ESLint's
// `no-var` flags this; the surgical rewrite is the obvious fix.
//
// Caveat: `let` is stricter (TDZ, no redeclaration). A pure structural
// swap is unsafe in cases where code relies on var's hoisting or
// function-scoping. This is a useful starting suggestion that the
// developer should review — particularly for `var` inside loops, or
// when the variable is referenced before its declaration. Pasta
// doesn't have scope analysis, so the rule is structural-only.
//
// `var` is a JavaScript reserved word so the keyword `var` won't
// appear as an identifier; the within-token replace is unambiguous.

package js_var_to_let

import (
	"github.com/imjasonh/pasta/schema"
	jslang "github.com/imjasonh/pasta/lang/javascript"
)

js_var_to_let: schema.#Analyzer & {
	name:    "js_var_to_let"
	version: "0.1.0"
	doc:     "Replace var declarations with let"
	facts: {}

	rules: var_to_let: {
		name:      "var_to_let"
		doc:       "var x -> let x"
		languages: [jslang.Name]
		requires: []
		provides: []

		match: {
			node: "variable_declaration"
		}

		diagnose: {
			message:  "use `let` instead of `var`"
			severity: "warning"
		}

		rewrite: edits: [
			{within: "_root", token: "var", replace_with: "let"},
			{target: "_root", replacement: "@_root"},
		]
	}
}
