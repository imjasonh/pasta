// iferr is a port of github.com/imjasonh/iferr-analyzer to the pasta DSL.
// It suggests inlining error assignments into if conditions where possible.
//
// Three rules:
//
//   inline_define   — handles `:=` assignments. Variable is introduced
//                     by the assignment.
//   inline_assign   — handles `=` assignments without a preceding
//                     `var X T` declaration.
//   inline_assign_with_decl — handles `=` assignments preceded by
//                     `var X T`, absorbing the declaration into the
//                     rewrite.
//
// All three look for the pattern:
//
//     <assign>     // err := foo()    or    err = foo()
//     <if_stmt>    // if err != nil { ... }
//
// and rewrite to:
//
//     if <assign>; <cond> { ... }
//
// (inline_assign_with_decl additionally deletes the `var X T` line.)
//
// The semantic checks (no later use, no named-result clash, etc.) are
// generic preconditions parameterized by Go grammar node names.

package go_iferr

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
	gopat "github.com/imjasonh/pasta/patterns/go"
)

// _ifNilCond is the shared shape: an if-statement with no init, whose
// condition is a binary `==`/`!=` against nil where the non-nil side
// is the last identifier of @lhs_list.
_ifNilCond: schema.#Capture & {
	capture: "if_stmt"
	pattern: {
		node: "if_statement"
		absent_fields: ["initializer"]
		fields: condition: {
			capture: "cond"
			pattern: {
				node: "binary_expression"
				fields: {
					left:     {capture: "cond_x"}
					operator: {capture: "cond_op"}
					right:    {capture: "cond_y"}
				}
				where: [
					{op: "matches", args:        ["@cond_op", "==|!="]},
					{op: "nil_comparison", args: ["@cond_x", "@cond_y", "@lhs_list"]},
				]
			}
		}
	}
}

// _noLaterUse: no LHS variable may be referenced anywhere in the
// enclosing function except inside the assign+if range.
_noLaterUseInFunctionBody: {
	check: "ancestor_field_subtree_no_ident"
	args: {
		anchor:         "@if_stmt"
		ancestor_types: gopat.FunctionLikeTypes
		field:          "body"
		source:         "@lhs_list"
		exclude_start:  string
		exclude_end:    "@if_stmt"
	}
}

// _notNamedResult: none of the LHS variables may be a named result
// parameter (bare returns implicitly read those).
_notNamedResult: {
	check: "ancestor_field_subtree_no_ident"
	args: {
		anchor:         "@if_stmt"
		ancestor_types: gopat.FunctionLikeTypes
		field:          "result"
		source:         "@lhs_list"
	}
}

// _allLHSAreIdents: every LHS expression must be a plain identifier.
// Rejects `t.val, err = bar()`.
_allLHSAreIdents: {
	check: "all_children_type"
	args: {cap: "@lhs_list", type: "identifier"}
}

go_iferr: schema.#Analyzer & {
	name:    "go_iferr"
	version: "0.1.0"
	doc:     "Suggests inlining error assignments into if conditions where possible"

	facts: {}

	rules: {
		// ============================================================
		// Rule 1: inline_define — short variable declarations (`:=`)
		//
		//   err := foo()                  if err := foo(); err != nil {
		//   if err != nil {       --->        return err
		//       return err                }
		//   }
		// ============================================================
		inline_define: {
			name:      "inline_define"
			doc:       "Inline := assignment into if condition"
			languages: [golang.Name]
			requires: []
			provides: []

			match: {
				node: gopat.StmtListContainers
				adjacent: [
					{
						capture: "assign"
						pattern: {
							node: "short_var_declaration"
							fields: {
								left:  {capture: "lhs_list"}
								right: {capture: "rhs"}
							}
						}
					},
					_ifNilCond,
				]
			}

			// Semantic gate: none of the variables introduced by the `:=`
			// may be referenced in the same block after the if.
			pre_conditions: [{
				check: "siblings_after_no_ident"
				args: {
					anchor: "@if_stmt"
					source: "@lhs_list"
				}
			}]

			rewrite_opts: {
				preserve_comments: true
			}

			diagnose: {
				message:  "can inline assignment into if statement"
				severity: "warning"
			}

			rewrite: edits: [
				{delete_from: "assign", delete_to: "cond"},
				{position: "before", anchor: "cond", text: "if @assign; "},
			]
		}

		// ============================================================
		// Rule 2: inline_assign — plain assignment, no preceding var-decl
		//
		//   err = foo()                   if err := foo(); err != nil {
		//   if err != nil {       --->        return err
		//       return err                }
		//   }
		// ============================================================
		inline_assign: {
			name:      "inline_assign"
			doc:       "Inline = assignment into if condition, converting to :="
			languages: [golang.Name]
			requires: []
			provides: []

			match: {
				node: gopat.StmtListContainers
				adjacent: [
					{
						capture: "assign"
						pattern: {
							node: "assignment_statement"
							fields: {
								left:     {capture: "lhs_list"}
								operator: {capture: "assign_op"}
								right:    {capture: "rhs"}
							}
							where: [{op: "token_eq", args: ["@assign_op", "="]}]
						}
					},
					_ifNilCond,
				]
			}

			pre_conditions: [
				_allLHSAreIdents,
				_noLaterUseInFunctionBody & {args: exclude_start: "@assign"},
				_notNamedResult,
			]

			rewrite_opts: {
				preserve_comments: true
			}

			diagnose: {
				message:  "can inline assignment into if statement"
				severity: "warning"
			}

			rewrite: edits: [
				{delete_from: "assign", delete_to: "cond"},
				{within: "assign", token: "=", replace_with: ":="},
				{position: "before", anchor: "cond", text: "if @assign; "},
			]
		}

		// ============================================================
		// Rule 3: inline_assign_with_decl — plain assignment with a
		//                                   preceding `var X T`
		//
		//   var err error                 if err := foo(); err != nil {
		//   err = foo()           --->        return err
		//   if err != nil {                }
		//       return err
		//   }
		// ============================================================
		inline_assign_with_decl: {
			name:      "inline_assign_with_decl"
			doc:       "Inline = assignment + preceding var-decl into if condition"
			languages: [golang.Name]
			requires: []
			provides: []

			match: {
				node: gopat.StmtListContainers
				adjacent: [
					{
						capture: "assign"
						pattern: {
							node: "assignment_statement"
							fields: {
								left:     {capture: "lhs_list"}
								operator: {capture: "assign_op"}
								right:    {capture: "rhs"}
							}
							where: [{op: "token_eq", args: ["@assign_op", "="]}]
						}
						preceding: {
							capture: "var_decl"
							pattern: {node: "var_declaration"}
						}
					},
					_ifNilCond,
				]
			}

			pre_conditions: [
				_allLHSAreIdents,
				_noLaterUseInFunctionBody & {args: exclude_start: "@var_decl"},
				_notNamedResult,
				// The captured var_decl must declare exactly the LHS
				// variables (non-blank, no initializer values).
				{
					check: "decl_matches_idents"
					args: {
						decl:        "@var_decl"
						source:      "@lhs_list"
						spec_type:   "var_spec"
						value_field: "value"
					}
				},
			]

			rewrite_opts: {
				preserve_comments: true
			}

			diagnose: {
				message:  "can inline assignment into if statement"
				severity: "warning"
			}

			rewrite: edits: [
				{delete_from: "var_decl", delete_to: "cond"},
				{within: "assign", token: "=", replace_with: ":="},
				{position: "before", anchor: "cond", text: "if @assign; "},
			]
		}
	}
}
