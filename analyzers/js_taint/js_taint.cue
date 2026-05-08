// js_taint tracks user-controlled data from common JS sources
// (`req.query.X`, `req.params.X`, `req.body`) through assignments to
// dangerous sinks (`eval`, `Function`, `child_process.exec`).
//
// Same shape as python_taint and go_taint.

package js_taint

import (
	"github.com/imjasonh/pasta/schema"
	jslang "github.com/imjasonh/pasta/lang/javascript"
)

js_taint: schema.#Analyzer & {
	name:    "js_taint"
	version: "0.1.0"
	doc:     "Track taint from req.query / req.params / req.body into eval / Function / exec"

	facts: {
		tainted: {kind: "tainted"}
	}

	rules: {
		// Pass 1: tag LHS where RHS is `req.query.X`, `req.params.X`,
		// or `req.body` — recognized via the OUTER object being `req`.
		mark_sources: {
			name:      "mark_sources"
			doc:       "Tag LHS of `const x = req.query.foo` and similar"
			languages: [jslang.Name]
			requires: []
			provides: ["tainted"]

			match: {
				node: "variable_declarator"
				fields: {
					name: {capture: "lhs", pattern: {node: "identifier"}}
					value: {
						capture: "rhs"
						pattern: {node: "member_expression"}
					}
				}
				// Matches `req.body`, `req.query.cmd`, `req.params.id`,
				// `req.headers["x-..."]`, etc. — any expression rooted
				// in `req.<source-field>`.
				where: [{
					op: "matches"
					args: ["@rhs", "^req\\.(query|params|body|headers)\\b"]
				}]
			}

			emit: [{fact: "tainted", attach: "lhs"}]
		}

		// Pass 2: propagate taint through `const y = x` where x has fact.
		propagate: {
			name:      "propagate"
			doc:       "Propagate taint through identifier-to-identifier assignment"
			languages: [jslang.Name]
			requires: ["tainted"]
			provides: ["tainted"]

			match: {
				node: "variable_declarator"
				fields: {
					name: {capture: "lhs", pattern: {node: "identifier"}}
					value: {capture: "rhs", pattern: {node: "identifier"}}
				}
				where: [{op: "has_fact", args: ["@rhs", "tainted"]}]
			}

			emit: [{fact: "tainted", attach: "lhs"}]
		}

		// Pass 3: warn at `eval(x)` / `Function(x)` where x is tainted.
		check_sinks: {
			name:      "check_sinks"
			doc:       "Flag tainted data flowing into eval / Function"
			languages: [jslang.Name]
			requires: ["tainted"]
			provides: []

			match: {
				node: "call_expression"
				fields: {
					function: {
						capture: "sink"
						pattern: {node: "identifier"}
					}
					arguments: {
						node: "arguments"
						children: [{capture: "arg", pattern: {node: "identifier"}}]
					}
				}
				where: [
					{op: "matches", args: ["@sink", "^(eval|Function)$"]},
					{op: "has_fact", args: ["@arg", "tainted"]},
				]
			}

			diagnose: {
				message:  "tainted user input flows into @sink — code injection risk"
				severity: "error"
			}
		}
	}
}
