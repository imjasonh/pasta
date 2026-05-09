// dockerfile_apt_no_recommends flags `apt-get install ...` runs that
// don't pass `--no-install-recommends`. The default behavior pulls
// in recommended packages, fattening the image considerably; the
// flag is a one-line fix that downstream image-size pipelines
// (hadolint DL3015) flag too.

package dockerfile_apt_no_recommends

import (
	"github.com/imjasonh/pasta/schema"
	dflang "github.com/imjasonh/pasta/lang/dockerfile"
)

dockerfile_apt_no_recommends: schema.#Analyzer & {
	name:    "dockerfile_apt_no_recommends"
	version: "0.1.0"
	doc:     "Flag apt-get install without --no-install-recommends"
	facts: {}

	rules: missing_flag: {
		name:      "missing_flag"
		doc:       "RUN apt-get install ... should pass --no-install-recommends"
		languages: [dflang.Name]
		requires: []
		provides: []

		match: {
			node: "shell_fragment"
			where: [
				{op: "matches", args: ["@_root", "apt-get install"]},
				{op: "not_matches", args: ["@_root", "--no-install-recommends"]},
			]
		}

		diagnose: {
			severity: "warning"
			message:  "apt-get install should pass --no-install-recommends to keep the image small"
		}
	}
}
