// file_match exercises the rule-level `file_match` glob: the rule
// only runs on files whose basename matches one of its patterns. Here
// we flag `print(...)` calls only inside `*_test.py` files.

package file_match

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

file_match: schema.#Analyzer & {
	name:    "file_match"
	version: "0.1.0"
	doc:     "Demo of rule-level file_match globs"
	facts: {}

	rules: print_in_test: {
		name: "print_in_test"
		doc:  "flag print() calls but only in test files"
		languages: [pylang.Name]
		requires: []
		provides: []
		file_match: ["*_test.py"]

		match: {
			node: "call"
			fields: function: {capture: "fn"}
			where: [{op: "token_eq", args: ["@fn", "print"]}]
		}

		diagnose: message: "print in test file"
	}
}
