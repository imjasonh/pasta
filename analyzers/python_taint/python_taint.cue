// python_taint is a port of plan.md §4.3 — a three-rule taint
// tracker for Python. Demonstrates fixpoint scheduling: the
// `propagate` rule both consumes and produces the `tainted` fact, so
// the engine groups it into a fixpoint SCC and re-runs it until the
// fact store stops growing.
//
// Sources, sinks, and propagation are all by IDENTIFIER NAME (the
// fact store's secondary by-name index). This makes the analysis
// scope-agnostic — the same name reused in different scopes is treated
// as a single tainted variable. Real precision needs scope-aware fact
// keys (plan.md §8 open question).
//
// Pass 1 — mark_sources: `x = input()` tags `x` as tainted.
// Pass 2 — propagate: `y = x` where `x` is tainted, tag `y`. SELF-LOOP.
// Pass 3 — check_sinks: `eval(z)` etc. where `z` is tainted, warn.

package python_taint

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

python_taint: schema.#Analyzer & {
	name:    "python_taint"
	version: "0.1.0"
	doc:     "Track taint from input() to eval()/exec()/os.system()"

	facts: {
		tainted: {kind: "tainted"}
	}

	rules: {
		// Pass 1: tag LHS identifiers of assignments whose RHS is a
		// known taint source.
		mark_sources: {
			name:      "mark_sources"
			doc:       "Tag LHS of assignment from a taint source"
			languages: [pylang.Name]
			requires: []
			provides: ["tainted"]

			match: {
				node: "assignment"
				fields: {
					left: {capture: "lhs", pattern: {node: "identifier"}}
					right: {
						capture: "rhs"
						pattern: {
							node: "call"
							fields: {
								function: {
									capture: "fn"
									pattern: {node: "identifier"}
								}
							}
						}
					}
				}
				where: [{op: "matches", args: ["@fn", "^(input|raw_input|getenv)$"]}]
			}

			emit: [{fact: "tainted", attach: "lhs"}]
		}

		// Pass 2: propagate taint through `y = x` assignments.
		// requires + provides "tainted" — the engine recognizes the
		// self-loop and runs this rule in a fixpoint.
		propagate: {
			name:      "propagate"
			doc:       "Propagate taint through identifier-to-identifier assignment"
			languages: [pylang.Name]
			requires: ["tainted"]
			provides: ["tainted"]

			match: {
				node: "assignment"
				fields: {
					left: {capture: "lhs", pattern: {node: "identifier"}}
					right: {capture: "rhs", pattern: {node: "identifier"}}
				}
				where: [{op: "has_fact", args: ["@rhs", "tainted"]}]
			}

			emit: [{fact: "tainted", attach: "lhs"}]
		}

		// Pass 3: at dangerous calls, fire if any argument identifier
		// is tainted.
		check_sinks: {
			name:      "check_sinks"
			doc:       "Flag tainted data flowing into eval/exec/system"
			languages: [pylang.Name]
			requires: ["tainted"]
			provides: []

			match: {
				node: "call"
				fields: {
					function: {
						capture: "sink"
						pattern: {node: "identifier"}
					}
					arguments: {
						node: "argument_list"
						children: [{capture: "arg", pattern: {node: "identifier"}}]
					}
				}
				where: [
					{op: "matches", args: ["@sink", "^(eval|exec|system)$"]},
					{op: "has_fact", args: ["@arg", "tainted"]},
				]
			}

			diagnose: {
				message:  "tainted data flows into @sink"
				severity: "error"
			}
		}
	}
}
