// Package python is a library of reusable CUE fragments for
// authoring Python analyzers. Importable as:
//
//	import pypat "github.com/imjasonh/pasta/patterns/python"

package python

import "github.com/imjasonh/pasta/schema"

// =====================================================================
// Function-like nodes
// =====================================================================

// FunctionLikeTypes lists tree-sitter-python's function-bearing nodes.
FunctionLikeTypes: ["function_definition", "lambda"]

// =====================================================================
// Class-related nodes
// =====================================================================

// ClassLikeTypes is just `class_definition` today; included for
// symmetry with FunctionLikeTypes and forward-compat if Python ever
// gains another class form.
ClassLikeTypes: ["class_definition"]

// =====================================================================
// Reusable pattern fragments
// =====================================================================

Identifier: schema.#Pattern & {node: "identifier"}
None:       schema.#Pattern & {node: "none"}

// Call matches a Python `call` (= function-call expression). Combine
// with `fields.function` and `fields.arguments` to constrain.
Call: schema.#Pattern & {node: "call"}

// FunctionCall matches a top-level `name(...)` call where the function
// is a bare identifier. Captures `fn` for use in predicates.
//
//	pypat.FunctionCall & {_name: "input"}
FunctionCall: {
	_name: string

	schema.#Pattern
	node: "call"
	fields: {
		function: {capture: "fn", pattern: Identifier}
	}
	where: [{op: "eq", args: ["@fn", _name]}]
}
