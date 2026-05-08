// yaml_truthy normalizes non-canonical boolean values to lowercase
// `true` / `false`. YAML 1.1 accepted Yes/No/On/Off/True/False/yes/no/
// on/off/TRUE/FALSE as booleans; YAML 1.2 narrows to true/false. Many
// downstream parsers (notably Go's gopkg.in/yaml.v2) still tolerate the
// older spelling, but mixing them in one file is a real source of
// confusion. yamllint catches this; encoding here gives an auto-fix.

package yaml_truthy

import (
	"github.com/imjasonh/pasta/schema"
	yamllang "github.com/imjasonh/pasta/lang/yaml"
)

yaml_truthy: schema.#Analyzer & {
	name:    "yaml_truthy"
	version: "0.1.0"
	doc:     "Normalize Yes/No/On/Off/True/False to true/false"
	facts: {}

	rules: {
		// YAML scalar with a non-canonical truthy value.
		non_canonical_true: {
			name: "non_canonical_true"
			doc:  "Use lowercase `true` instead of Yes/On/True/etc."
			languages: [yamllang.Name]
			requires: []
			provides: []

			match: {
				node: ["plain_scalar"]
				where: [{
					op: "matches"
					args: ["@_root", "^(True|TRUE|Yes|YES|yes|On|ON|on)$"]
				}]
			}

			diagnose: {
				message:  "non-canonical boolean; use `true`"
				severity: "warning"
			}

			rewrite: edits: [{target: "_root", replacement: "true"}]
		}

		non_canonical_false: {
			name: "non_canonical_false"
			doc:  "Use lowercase `false` instead of No/Off/False/etc."
			languages: [yamllang.Name]
			requires: []
			provides: []

			match: {
				node: ["plain_scalar"]
				where: [{
					op: "matches"
					args: ["@_root", "^(False|FALSE|No|NO|no|Off|OFF|off)$"]
				}]
			}

			diagnose: {
				message:  "non-canonical boolean; use `false`"
				severity: "warning"
			}

			rewrite: edits: [{target: "_root", replacement: "false"}]
		}
	}
}
