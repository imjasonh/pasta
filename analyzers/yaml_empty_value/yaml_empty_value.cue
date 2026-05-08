// yaml_empty_value flags a key with no value (e.g. `key:` followed by
// end-of-line and the next key, not by a nested mapping). YAML
// interprets this as `null`, which is rarely what the author intended;
// they typically meant either an empty string `""` or to delete the
// key. Common source of bugs in CI pipelines and Helm charts.
//
// No auto-fix: we don't know whether the intent was `""`, `null`
// explicit, or to delete the key.

package yaml_empty_value

import (
	"pasta.dev/schema"
	yamllang "pasta.dev/lang/yaml"
)

yaml_empty_value: schema.#Analyzer & {
	name:    "yaml_empty_value"
	version: "0.1.0"
	doc:     "Flag YAML keys whose value is implicitly null"
	facts: {}

	rules: missing_value: {
		name: "missing_value"
		doc:  "key: with no value (parses as null)"
		languages: [yamllang.Name]
		requires: []
		provides: []

		match: {
			node: "block_mapping_pair"
			absent_fields: ["value"]
		}

		diagnose: {
			message:  "YAML key has no value (parses as null); did you mean `\"\"` or to delete the key?"
			severity: "warning"
		}
	}
}
