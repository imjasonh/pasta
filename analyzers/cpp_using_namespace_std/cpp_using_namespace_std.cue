// cpp_using_namespace_std flags `using namespace std;`. The directive
// is harmless in a small `.cpp` but disastrous in a header — it
// pollutes every translation unit that #includes the header. Even in
// `.cpp` files it makes the source ambiguous (which `count` did you
// mean?). Many style guides (Google, LLVM) ban it outright.
//
// Pasta has no per-rule file-extension gating today (would need the
// `file_match:` field re-added — see future-work.md), so we flag
// the directive in any C++ source. False-positive rate is low: this
// is widely considered an antipattern even outside headers.

package cpp_using_namespace_std

import (
	"github.com/imjasonh/pasta/schema"
	cpplang "github.com/imjasonh/pasta/lang/cpp"
)

cpp_using_namespace_std: schema.#Analyzer & {
	name:    "cpp_using_namespace_std"
	version: "0.1.0"
	doc:     "Flag `using namespace std;`"
	facts: {}

	rules: using_std: {
		name:      "using_std"
		doc:       "Flag using namespace std at file scope"
		languages: [cpplang.Name]
		requires: []
		provides: []

		match: {
			node: "using_declaration"
			where: [{op: "matches", args: ["@_root", "^using\\s+namespace\\s+std\\s*;"]}]
		}

		diagnose: {
			severity: "warning"
			message:  "`using namespace std;` pollutes the global namespace; qualify names instead"
		}
	}
}
