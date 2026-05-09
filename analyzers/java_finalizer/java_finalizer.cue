// java_finalizer flags `finalize()` overrides. Object.finalize is
// deprecated since Java 9 (and effectively unusable since the GC
// gives no timing guarantees). Use try-with-resources or
// java.lang.ref.Cleaner instead.

package java_finalizer

import (
	"github.com/imjasonh/pasta/schema"
	javalang "github.com/imjasonh/pasta/lang/java"
)

java_finalizer: schema.#Analyzer & {
	name:    "java_finalizer"
	version: "0.1.0"
	doc:     "Flag finalize() overrides — use Cleaner or try-with-resources"
	facts: {}

	rules: finalizer: {
		name:      "finalizer"
		doc:       "Flag protected void finalize()"
		languages: [javalang.Name]
		requires: []
		provides: []

		match: {
			node: "method_declaration"
			fields: {
				name: {capture: "name"}
				parameters: {
					capture: "params"
					pattern: {node: "formal_parameters"}
				}
			}
			where: [
				{op: "eq", args: ["@name", "finalize"]},
				// No-arg finalize() — the JLS finalizer signature.
				{op: "empty", args: ["@params"]},
			]
		}

		diagnose: {
			severity: "warning"
			message:  "Object.finalize is deprecated; use try-with-resources or java.lang.ref.Cleaner"
		}
	}
}
