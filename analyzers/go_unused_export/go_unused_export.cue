// go_unused_export is a cross-file analyzer that flags exported
// (capital-letter) function declarations that are never called from
// any file in the analysis group. It demonstrates the multi-file
// fact-passing model: rule `mark_called` runs across every file in
// the group first, populating the by-name fact index with one
// `called` entry per callee text. Once that pass is done across the
// whole group, `find_unused_exports` walks declarations and asks the
// shared store "is anything calling this name?" — picking up calls
// from sibling files via the file-agnostic by-name index.
//
// Schedule: mark_called provides `called`, find_unused_exports
// requires `called`; the engine topologically orders them, so
// every file's calls are recorded BEFORE any declaration is checked.
//
// Limitations (same as go_errcheck): the by-name lookup is
// scope-blind. A local function shadowing a package-level name will
// share facts. The matcher also only records BARE-identifier callees
// (`Foo()`); selector calls like `pkg.Foo()` and method calls like
// `obj.Foo()` are not tracked, so an exported `Foo` reached only
// through those forms would be reported as unused. For a demo of
// cross-file analysis that's fine; for a production unused-exports
// linter you'd want scope-aware fact keys plus selector-aware call
// matching (see future-work.md).

package go_unused_export

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
)

go_unused_export: schema.#Analyzer & {
	name:    "go_unused_export"
	version: "0.1.0"
	doc:     "Flag exported Go funcs that no file in the group calls"

	facts: {
		called: {kind: "called"}
	}

	rules: {
		// Pass 1: tag every callee identifier with `called`. The
		// fact is attached to the identifier node so the by-name
		// index keys it by callee text — visible across files.
		mark_called: {
			name:      "mark_called"
			doc:       "Record that an identifier is the callee of some call expression"
			languages: [golang.Name]
			requires: []
			provides: ["called"]

			match: {
				node: "call_expression"
				fields: {
					function: {
						capture: "callee"
						pattern: {node: "identifier"}
					}
				}
			}

			emit: [{
				fact:   "called"
				attach: "callee"
			}]
		}

		// Pass 2: flag exported function declarations whose name
		// has no `called` fact anywhere in the group.
		find_unused_exports: {
			name:      "find_unused_exports"
			doc:       "Diagnose exported funcs that no file calls"
			languages: [golang.Name]
			requires: ["called"]
			provides: []

			match: {
				node: "function_declaration"
				fields: {
					name: {capture: "fn_name"}
				}
				where: [
					{op: "matches", args: ["@fn_name", "^[A-Z]"]},
					{op: "not_has_fact", args: ["@fn_name", "called"]},
				]
			}

			diagnose: {
				message:  "exported func @fn_name is never called"
				severity: "warning"
			}
		}
	}
}
