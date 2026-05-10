// project_config exercises `config.cue`: two rules are defined and
// the project config disables one. The remaining rule still fires;
// the disabled one is silent.

package project_config

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

_printRule: {
	_msg: string
	out: {
		languages: [pylang.Name]
		requires: []
		provides: []
		match: {
			node: "call"
			fields: function: {capture: "fn"}
			where: [{op: "token_eq", args: ["@fn", "print"]}]
		}
		diagnose: message: _msg
	}
}

project_config: schema.#Analyzer & {
	name:    "project_config"
	version: "0.1.0"
	doc:     "Demo of config.cue rule disabling"
	facts: {}

	rules: {
		kept: (_printRule & {_msg: "kept rule fired"}).out & {
			name: "kept"
			doc:  "stays enabled"
		}
		dropped: (_printRule & {_msg: "this should never appear"}).out & {
			name: "dropped"
			doc:  "disabled by config.cue"
		}
	}
}
