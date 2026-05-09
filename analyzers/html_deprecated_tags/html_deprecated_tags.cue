// html_deprecated_tags flags HTML4-era presentational tags that
// should be replaced with semantic markup + CSS. Drop-in CSS fixes:
//   <center>x</center> -> <div style="text-align: center;">x</div>
//   <font color="red"> -> <span style="color: red;">
//   <marquee>          -> use CSS animation or just remove.
// No auto-fix; the right replacement depends on context.

package html_deprecated_tags

import (
	"github.com/imjasonh/pasta/schema"
	htmllang "github.com/imjasonh/pasta/lang/html"
)

html_deprecated_tags: schema.#Analyzer & {
	name:    "html_deprecated_tags"
	version: "0.1.0"
	doc:     "Flag HTML4-era presentational tags"
	facts: {}

	rules: deprecated: {
		name:      "deprecated"
		doc:       "Flag <center>, <font>, <marquee>, <blink>, <strike>"
		languages: [htmllang.Name]
		requires: []
		provides: []

		match: {
			node: "start_tag"
			children: [{
				capture: "name"
				pattern: {node: "tag_name"}
			}]
			where: [{op: "matches", args: ["@name", "^(center|font|marquee|blink|strike|big|tt)$"]}]
		}

		diagnose: {
			severity: "warning"
			message:  "deprecated HTML tag; use semantic markup with CSS instead"
		}
	}
}
