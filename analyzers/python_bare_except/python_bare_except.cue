// python_bare_except rewrites `except:` (catches everything including
// KeyboardInterrupt and SystemExit) to `except Exception:` (catches only
// regular exceptions). Bare except is one of the most common
// Python footguns; pylint flags it but doesn't auto-fix.

package python_bare_except

import (
	"github.com/imjasonh/pasta/schema"
	pylang "github.com/imjasonh/pasta/lang/python"
)

python_bare_except: schema.#Analyzer & {
	name:    "python_bare_except"
	version: "0.1.0"
	doc:     "Rewrite bare `except:` to `except Exception:`"
	facts: {}

	rules: bare_except: {
		name: "bare_except"
		doc:  "bare except clause catches too much; use Exception"
		languages: [pylang.Name]
		requires: []
		provides: []

		match: {
			node: "except_clause"
			// Bare except has no `value` (the exception type) — it's
			// just `except:` followed by the body.
			absent_fields: ["value"]
		}

		diagnose: {
			message:  "bare except: catches everything including KeyboardInterrupt; use `except Exception:`"
			severity: "warning"
		}

		// `within` transforms the @_root interpolation; `target` then
		// emits the actual byte-range replacement using the transformed
		// text. Together they replace `except` with `except Exception`
		// in-place, preserving the trailing `:` and body.
		rewrite: edits: [
			{within: "_root", token: "except", replace_with: "except Exception"},
			{target: "_root", replacement: "@_root"},
		]
	}
}
