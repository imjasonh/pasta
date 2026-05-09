// sql_select_star flags `SELECT *` queries (the bare-asterisk
// projection). Star projections are fragile: schema changes in the
// underlying table change query semantics silently, and they pull
// every column even when you only need a few. `SELECT * FROM`
// inside `EXISTS` or `COUNT(*)` is fine — those are aggregates
// against the whole row, not a projection. We match only the
// select_clause_body containing a bare asterisk_expression as its
// only child, which excludes function-call wrappers.

package sql_select_star

import (
	"github.com/imjasonh/pasta/schema"
	sqllang "github.com/imjasonh/pasta/lang/sql"
)

sql_select_star: schema.#Analyzer & {
	name:    "sql_select_star"
	version: "0.1.0"
	doc:     "Flag SELECT * — fragile under schema changes"
	facts: {}

	rules: select_star: {
		name:      "select_star"
		doc:       "SELECT * FROM ..."
		languages: [sqllang.Name]
		requires: []
		provides: []

		match: {
			node: "select_clause_body"
			children: [{
				capture: "star"
				pattern: {node: "asterisk_expression"}
			}]
			where: [{op: "named_child_count", args: ["@_root", "1"]}]
		}

		diagnose: {
			severity: "warning"
			message:  "SELECT * is fragile under schema changes; project the columns you need explicitly"
		}
	}
}
