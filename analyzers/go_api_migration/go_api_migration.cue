// go_api_migration is a worked example of using pasta to ship an
// adapter for a breaking API change. Hypothetical scenario:
//
//   - v1.2.2 of `widget` exposed `widget.Render(name string) string`.
//   - v1.2.3 added a trailing `opts *Options` parameter:
//         `widget.Render(name string, opts *Options) string`.
//     `opts` may be nil to keep the old behavior.
//   - v1.3.0 renamed `widget.OldName` to `widget.NewName`.
//
// Library authors can ship this .cue file alongside the release so
// downstream callers can run `pasta -fix go_api_migration.cue ./...`
// to migrate their code automatically. Each rule:
//
//   - Matches *only* the pre-migration call shape (so re-running the
//     fix is a no-op once the codebase is up to date).
//   - Leaves unrelated callers untouched (different package, different
//     method, already-migrated arity).
//   - Emits a diagnostic so `pasta` (without `-fix`) can be wired into
//     CI as a warning surface before the fix is committed.
//
// This pattern generalizes to renames, added trailing args, removed
// args (use `delete_from`/`delete_to` between captures), and changes
// in argument order (capture each arg, reassemble in the new order in
// the rewrite template).
package go_api_migration

import (
	"github.com/imjasonh/pasta/schema"
	golang "github.com/imjasonh/pasta/lang/go"
	gopat "github.com/imjasonh/pasta/patterns/go"
)

go_api_migration: schema.#Analyzer & {
	name:    "go_api_migration"
	version: "0.1.0"
	doc:     "Adapt callers to widget's v1.2.3 / v1.3.0 breaking changes"
	facts: {}

	rules: {
		// v1.2.3: widget.Render gained a trailing `opts *Options` arg.
		// Pre-migration single-arg callers get `nil` appended.
		render_add_opts: {
			name: "render_add_opts"
			doc:  "widget.Render(x) -> widget.Render(x, nil) (v1.2.3 added opts)"
			languages: [golang.Name]
			requires: []
			provides: []

			match: gopat.PackageCall & {
				fields: arguments: {
					capture: "args"
					pattern: {
						node: "argument_list"
						children: [{capture: "arg"}]
					}
				}
				where: [
					{op: "eq", args: ["@pkg", "widget"]},
					{op: "eq", args: ["@fn", "Render"]},
					// Match *only* the old single-arg shape so re-runs are no-ops.
					{op: "named_child_count", args: ["@args", "1"]},
				]
			}

			diagnose: {
				message:  "widget.Render gained an opts *Options arg in v1.2.3; pass nil to preserve previous behavior"
				severity: "warning"
			}

			rewrite: edits: [{
				target:      "_root"
				replacement: "widget.Render(@arg, nil)"
			}]
		}

		// v1.3.0: widget.OldName was renamed to widget.NewName.
		// Pure rename — no argument shape change.
		old_name_rename: {
			name: "old_name_rename"
			doc:  "widget.OldName -> widget.NewName (v1.3.0 rename)"
			languages: [golang.Name]
			requires: []
			provides: []

			match: schema.#Pattern & {
				node: "selector_expression"
				fields: {
					operand: {capture: "pkg", pattern: gopat.Identifier}
					field:   {capture: "fn", pattern: gopat.FieldIdentifier}
				}
				where: [
					{op: "eq", args: ["@pkg", "widget"]},
					{op: "eq", args: ["@fn", "OldName"]},
				]
			}

			diagnose: {
				message:  "widget.OldName was renamed to widget.NewName in v1.3.0"
				severity: "warning"
			}

			rewrite: edits: [{
				target:      "_root"
				replacement: "widget.NewName"
			}]
		}
	}
}
