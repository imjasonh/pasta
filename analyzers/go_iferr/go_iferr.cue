// iferr is a port of github.com/imjasonh/iferr-analyzer to the pasta DSL.
// It suggests inlining error assignments into if conditions where possible.
//
// Two rules cooperate:
//
//   inline_define — handles `:=` assignments. Simpler scope rules apply
//                   because the variable is introduced by the assignment.
//   inline_assign — handles `=` assignments and may also absorb a
//                   preceding `var X T` declaration. Stricter scope and
//                   shape checks apply because the variable lives in an
//                   outer scope.
//
// Both rules look for the pattern:
//
//     <assign>     // err := foo()    or    err = foo()
//     <if_stmt>    // if err != nil { ... }
//
// and rewrite it to:
//
//     if <assign>; <cond> { ... }
//
// The semantic checks (no later use, no named-result clash, etc.) are
// expressed as generic `pre_conditions` parameterized by Go grammar
// node names — there is no Go-specific code in the framework.

package go_iferr

import (
	"pasta.dev/schema"
	golang "pasta.dev/lang/go"
)

go_iferr: schema.#Analyzer & {
	name:    "go_iferr"
	version: "0.1.0"
	doc:     "Suggests inlining error assignments into if conditions where possible"

	facts: {}

	// Reusable fragment: tree-sitter-go function-like nodes.
	let _go_funcs = "function_declaration|method_declaration|func_literal"

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
				node: ["statement_list"]
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
					{
						capture: "if_stmt"
						pattern: {
							node: "if_statement"
							absent_fields: ["initializer"]
							fields: {
								condition: {
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
					},
				]
			}

			// Semantic gate: none of the variables introduced by the `:=`
			// may be referenced in the same block after the if. Inlining
			// would push the declaration into the if-init scope, where
			// later uses can no longer see it.
			pre_conditions: [{
				check: "siblings_after_no_ident"
				args: {
					anchor: "@if_stmt"
					source: "@lhs_list"
				}
			}]

			rewrite_opts: {
				preserve_comments: true
				preserve_indent:   true
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
		// Rule 2: inline_assign — plain assignments (`=`)
		//
		//   var err error                 if err := foo(); err != nil {
		//   err = foo()           --->        return err
		//   if err != nil {                }
		//       return err
		//   }
		// ============================================================
		inline_assign: {
			name:      "inline_assign"
			doc:       "Inline = assignment into if condition, converting to :="
			languages: [golang.Name]
			requires: []
			provides: []

			match: {
				node: ["statement_list"]
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
							capture:    "var_decl"
							quantifier: "?"
							pattern: {node: "var_declaration"}
						}
					},
					{
						capture: "if_stmt"
						pattern: {
							node: "if_statement"
							absent_fields: ["initializer"]
							fields: {
								condition: {
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
					},
				]
			}

			pre_conditions: [
				// Every LHS expression must be a plain identifier.
				// Rejects `t.val, err = bar()`.
				{
					check: "all_children_type"
					args: {cap: "@lhs_list", type: "identifier"}
				},

				// No LHS variable may be referenced anywhere in the
				// enclosing function except inside the assign+if range
				// (and the absorbed var_decl, if any).
				{
					check: "ancestor_field_subtree_no_ident"
					args: {
						anchor:         "@if_stmt"
						ancestor_types: _go_funcs
						field:          "body"
						source:         "@lhs_list"
						exclude_start:  "@var_decl|@assign"
						exclude_end:    "@if_stmt"
					}
				},

				// None of the LHS variables may be a named result
				// parameter. Bare returns implicitly read named results
				// without producing identifier references.
				{
					check: "ancestor_field_subtree_no_ident"
					args: {
						anchor:         "@if_stmt"
						ancestor_types: _go_funcs
						field:          "result"
						source:         "@lhs_list"
					}
				},

				// If a `var_decl` was captured, it must declare exactly
				// the LHS variables (non-blank, no initializer values).
				// Optional: when `var_decl` is unbound the check is
				// skipped, and the rule still fires.
				{
					check: "decl_matches_idents"
					args: {
						decl:        "@var_decl"
						source:      "@lhs_list"
						spec_type:   "var_spec"
						value_field: "value"
					}
					optional: true
				},
			]

			rewrite_opts: {
				preserve_comments: true
				preserve_indent:   true
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
