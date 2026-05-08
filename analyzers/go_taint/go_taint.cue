// go_taint tracks user-controlled values from common Go sources
// (`os.Getenv`, `os.Args`) through assignment chains to dangerous
// sinks (`exec.Command`). Same three-rule shape as python_taint:
//
//   mark_sources    requires=[]         provides=[tainted]
//   propagate       requires=[tainted]  provides=[tainted]   (self-loop)
//   check_sinks     requires=[tainted]  provides=[]
//
// The propagate rule's self-cycle drives the engine's fixpoint
// scheduler; chains of length > 1 require multiple iterations to
// reach the sink.

package go_taint

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
	gopat "github.com/imjasonh/pasta/patterns/go"
)

go_taint: schema.#Analyzer & {
	name:    "go_taint"
	version: "0.1.0"
	doc:     "Track taint from os.Getenv / os.Args into exec.Command"

	facts: {
		tainted: {kind: "tainted"}
	}

	rules: {
		// Pass 1: tag LHS where RHS is `os.Getenv(...)`.
		mark_sources: {
			name:      "mark_sources"
			doc:       "Tag LHS of `x := os.Getenv(...)` and similar"
			languages: [golang.Name]
			requires: []
			provides: ["tainted"]

			match: {
				node: ["short_var_declaration", "assignment_statement"]
				fields: {
					left: {
						node: "expression_list"
						children: [{capture: "lhs", pattern: gopat.Identifier}]
					}
					right: {
						node: "expression_list"
						children: [{
							capture: "rhs"
							pattern: gopat.PackageCall
						}]
					}
				}
				where: [
					{op: "eq", args: ["@pkg", "os"]},
					{op: "matches", args: ["@fn", "^(Getenv|LookupEnv)$"]},
				]
			}

			emit: [{fact: "tainted", attach: "lhs"}]
		}

		// Pass 2: propagate via simple `y = x` / `y := x` assignments.
		// Self-cycle drives fixpoint iteration.
		propagate: {
			name:      "propagate"
			doc:       "Propagate taint through identifier-to-identifier assignment"
			languages: [golang.Name]
			requires: ["tainted"]
			provides: ["tainted"]

			match: {
				node: ["short_var_declaration", "assignment_statement"]
				fields: {
					left: {
						node: "expression_list"
						children: [{capture: "lhs", pattern: gopat.Identifier}]
					}
					right: {
						node: "expression_list"
						children: [{capture: "rhs", pattern: gopat.Identifier}]
					}
				}
				where: [{op: "has_fact", args: ["@rhs", "tainted"]}]
			}

			emit: [{fact: "tainted", attach: "lhs"}]
		}

		// Pass 3: warn when a tainted identifier flows into
		// `exec.Command(name, ...)`. We check the first positional
		// argument since that's the command being invoked.
		check_sinks: {
			name:      "check_sinks"
			doc:       "Flag tainted data flowing into exec.Command"
			languages: [golang.Name]
			requires: ["tainted"]
			provides: []

			match: gopat.PackageCall & {
				fields: arguments: {
					node: "argument_list"
					children: [{capture: "arg", pattern: gopat.Identifier}]
				}
				where: [
					{op: "eq", args: ["@pkg", "exec"]},
					{op: "eq", args: ["@fn", "Command"]},
					{op: "has_fact", args: ["@arg", "tainted"]},
				]
			}

			diagnose: {
				message:  "tainted data from environment flows into exec.Command"
				severity: "error"
			}
		}
	}
}
