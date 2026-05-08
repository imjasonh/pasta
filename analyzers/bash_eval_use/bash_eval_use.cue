// bash_eval_use flags `eval` invocations. `eval` re-parses its
// arguments as shell, which is a code-injection hazard whenever any
// arg is influenced by user input or external data. Almost always
// avoidable with arrays or `printf -v`.
//
// No auto-fix: replacing eval safely requires understanding what the
// caller is trying to do (variable indirection, dynamic command
// construction, etc.).

package bash_eval_use

import (
	"github.com/imjasonh/pasta/schema"
	bashlang "github.com/imjasonh/pasta/lang/bash"
)

bash_eval_use: schema.#Analyzer & {
	name:    "bash_eval_use"
	version: "0.1.0"
	doc:     "Flag eval — code-injection hazard"
	facts: {}

	rules: eval_call: {
		name: "eval_call"
		doc:  "eval is a code-injection vector"
		languages: [bashlang.Name]
		requires: []
		provides: []

		match: {
			node: "command"
			fields: {
				name: {capture: "cmd"}
			}
			where: [{op: "eq", args: ["@cmd", "eval"]}]
		}

		diagnose: {
			message:  "eval re-parses its arguments as shell — code-injection hazard, prefer arrays"
			severity: "warning"
		}
	}
}
