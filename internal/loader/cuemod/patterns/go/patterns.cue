// Package go_patterns is a library of reusable CUE fragments for
// authoring Go analyzers — node-type constants, common pattern
// shapes, and predicate-arg helpers. Importable as:
//
//	import gopat "github.com/imjasonh/pasta/patterns/go"
//
// Adding a fragment here removes hand-rolled grammar knowledge from
// individual rules, so the next time tree-sitter-go renames a node
// type only this file has to change.

package go

import "github.com/imjasonh/pasta/schema"

// =====================================================================
// Container conventions
// =====================================================================

// StmtListContainers is the set of node types whose named children
// form an ordered statement list — the natural anchor for `adjacent`
// matching.
//
// gotreesitter's Go grammar wraps a function body's statements in a
// `statement_list` (under `block`), and each switch / select case
// wraps its body in its own `statement_list` too — so this single
// element suffices.
StmtListContainers: ["statement_list"]

// =====================================================================
// Function-like nodes
// =====================================================================

// FunctionLikeTypes lists the tree-sitter-go nodes that represent
// "something with a body": top-level functions, methods, and closures.
// Used as the `ancestor_types` arg to predicates that walk up looking
// for the enclosing function.
FunctionLikeTypes: ["function_declaration", "method_declaration", "func_literal"]

// =====================================================================
// Reusable pattern fragments
// =====================================================================

// Identifier matches an `identifier` node.
Identifier: schema.#Pattern & {node: "identifier"}

// FieldIdentifier matches a `field_identifier` node — the right side
// of a selector expression (the `Method` in `pkg.Method`).
FieldIdentifier: schema.#Pattern & {node: "field_identifier"}

// Nil matches a `nil` literal.
Nil: schema.#Pattern & {node: "nil"}

// SelectorExpression matches `pkg.field`. Pass through `fields:` to
// constrain pkg or field to specific captures or values.
SelectorExpression: schema.#Pattern & {node: "selector_expression"}

// CallExpression matches a function call. Use `fields.function` and
// `fields.arguments` to constrain.
CallExpression: schema.#Pattern & {node: "call_expression"}

// PackageCall is the structural shape of a `pkg.fn(...)` style call.
// Captures `pkg` (the package identifier on the left of the dot) and
// `fn` (the field identifier on the right) so the user's `where`
// clause can constrain them with `eq`, `matches`, etc., and the
// user's `fields.arguments` block can pin down the call shape.
//
// Typical use:
//
//	match: gopat.PackageCall & {
//	    fields: arguments: {
//	        node: "argument_list"
//	        children: [...]
//	    }
//	    where: [
//	        {op: "eq", args: ["@pkg", "errors"]},
//	        {op: "eq", args: ["@fn", "Is"]},
//	    ]
//	}
PackageCall: schema.#Pattern & {
	node: "call_expression"
	fields: {
		function: {
			node: "selector_expression"
			fields: {
				operand: {capture: "pkg", pattern: Identifier}
				field:   {capture: "fn", pattern: FieldIdentifier}
			}
		}
	}
}
