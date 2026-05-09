// yaml_truthy normalizes non-canonical boolean values to lowercase
// `true` / `false`. YAML 1.1 accepted Yes/No/On/Off/True/False/yes/no/
// on/off/TRUE/FALSE as booleans; YAML 1.2 narrows to true/false. Many
// downstream parsers (notably Go's gopkg.in/yaml.v2) still tolerate
// the older spelling, but mixing them in one file is a real source of
// confusion. yamllint catches this; encoding here gives an auto-fix.
//
// Implementation: two rule families, one per syntactic position of a
// "value", because tree-sitter-yaml treats keys and values
// identically as plain_scalar nodes — we have to distinguish by
// ancestor / field path:
//
//   1. mapping value: `key: yes` — match the block_mapping_pair and
//      descend through .value -> flow_node -> plain_scalar.
//   2. sequence element: `[a, yes, b]` — match plain_scalar with a
//      flow_sequence ancestor (block_sequence_item also covers the
//      block-style `- yes` form).
//
// A bare `on:` mapping KEY is NOT a value in either shape, so it's
// untouched (this is what GitHub Actions YAML relies on).

package yaml_truthy

import (
	"github.com/imjasonh/pasta/schema"
	yamllang "github.com/imjasonh/pasta/lang/yaml"
)

// _mappingValueRule: match block_mapping_pair, descend to value's
// plain_scalar, regex check.
_mappingValueRule: {
	_regex:       string
	_replacement: "true" | "false"

	out: {
		languages: [yamllang.Name]
		requires: []
		provides: []

		match: {
			node: "block_mapping_pair"
			fields: value: {
				node: "flow_node"
				children: [{
					capture: "scalar"
					pattern: {node: "plain_scalar"}
				}]
			}
			where: [{op: "matches", args: ["@scalar", _regex]}]
		}

		diagnose: {
			severity: "warning"
			message:  "non-canonical boolean; use `\(_replacement)`"
		}

		rewrite: edits: [{
			target:      "scalar"
			replacement: _replacement
		}]
	}
}

// _sequenceElementRule: match plain_scalar whose ancestor is a
// flow_sequence or block_sequence_item. This catches `[a, yes]` and
// `- yes` shapes without flagging mapping keys.
_sequenceElementRule: {
	_regex:       string
	_replacement: "true" | "false"

	out: {
		languages: [yamllang.Name]
		requires: []
		provides: []

		match: {
			node: "plain_scalar"
			where: [
				{op: "matches", args: ["@_root", _regex]},
				{op: "ancestor_is", args: ["@_root", ["flow_sequence", "block_sequence_item"]]},
			]
		}

		diagnose: {
			severity: "warning"
			message:  "non-canonical boolean; use `\(_replacement)`"
		}

		rewrite: edits: [{
			target:      "_root"
			replacement: _replacement
		}]
	}
}

_truthyRegex: "^(True|TRUE|Yes|YES|yes|On|ON|on)$"
_falsyRegex:  "^(False|FALSE|No|NO|no|Off|OFF|off)$"

yaml_truthy: schema.#Analyzer & {
	name:    "yaml_truthy"
	version: "0.1.0"
	doc:     "Normalize Yes/No/On/Off/True/False to true/false (values only, not keys)"
	facts: {}

	rules: {
		mapping_value_true: (_mappingValueRule & {
			_regex:       _truthyRegex
			_replacement: "true"
		}).out & {
			name: "mapping_value_true"
			doc:  "key: Yes/On/True -> key: true"
		}
		mapping_value_false: (_mappingValueRule & {
			_regex:       _falsyRegex
			_replacement: "false"
		}).out & {
			name: "mapping_value_false"
			doc:  "key: No/Off/False -> key: false"
		}
		sequence_element_true: (_sequenceElementRule & {
			_regex:       _truthyRegex
			_replacement: "true"
		}).out & {
			name: "sequence_element_true"
			doc:  "[a, Yes/On/True, b] -> [a, true, b]"
		}
		sequence_element_false: (_sequenceElementRule & {
			_regex:       _falsyRegex
			_replacement: "false"
		}).out & {
			name: "sequence_element_false"
			doc:  "[a, No/Off/False, b] -> [a, false, b]"
		}
	}
}
