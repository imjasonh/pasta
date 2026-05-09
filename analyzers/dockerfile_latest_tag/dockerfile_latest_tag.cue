// dockerfile_latest_tag flags `FROM image:latest` and implicit-latest
// `FROM image` (no tag). Pinning to a specific tag — better, a digest
// — makes builds reproducible. There's no auto-fix; the right tag
// depends on the project.

package dockerfile_latest_tag

import (
	"github.com/imjasonh/pasta/schema"
	dflang "github.com/imjasonh/pasta/lang/dockerfile"
)

dockerfile_latest_tag: schema.#Analyzer & {
	name:    "dockerfile_latest_tag"
	version: "0.1.0"
	doc:     "Flag FROM image:latest or implicit-latest"
	facts: {}

	rules: {
		// FROM image:latest
		explicit_latest: {
			name:      "explicit_latest"
			doc:       "FROM image:latest"
			languages: [dflang.Name]
			requires: []
			provides: []

			match: {
				node: "image_spec"
				children: [
					{capture: "name", pattern: {node: "image_name"}},
					{capture: "tag", pattern: {node: "image_tag"}},
				]
				where: [{op: "eq", args: ["@tag", ":latest"]}]
			}

			diagnose: {
				severity: "warning"
				message:  "pin a specific image tag (or digest) instead of `:latest`"
			}
		}

		// FROM image (no tag) — implicitly :latest
		implicit_latest: {
			name:      "implicit_latest"
			doc:       "FROM image without an explicit tag"
			languages: [dflang.Name]
			requires: []
			provides: []

			match: {
				node: "image_spec"
				children: [
					{capture: "name", pattern: {node: "image_name"}},
				]
				where: [{op: "named_child_count", args: ["@_root", "1"]}]
			}

			diagnose: {
				severity: "warning"
				message:  "image has no tag; pin a specific tag (or digest) instead of relying on implicit `:latest`"
			}
		}
	}
}
