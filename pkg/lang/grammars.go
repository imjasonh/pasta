package lang

import (
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Grammars maps grammar names (referenced from language CUE) to the
// gotreesitter language. Adding a new alias for a grammar already
// shipped with gotreesitter is a CUE change. Adding a brand-new
// grammar means registering it here.
//
// gotreesitter ships 206 grammars in-tree; only a subset is registered
// below. Add more entries as analyzers need them. The runtime cost of
// registering a grammar is just a function pointer until the grammar
// is actually loaded — gotreesitter lazy-decompresses grammar tables
// on first use.
var Grammars = map[string]func() *gts.Language{
	"go":         grammars.GoLanguage,
	"python":     grammars.PythonLanguage,
	"rust":       grammars.RustLanguage,
	"javascript": grammars.JavascriptLanguage,
	"typescript": grammars.TypescriptLanguage,
	"yaml":       grammars.YamlLanguage,
	"bash":       grammars.BashLanguage,
	"json":       grammars.JsonLanguage,
}

// grammarNames returns the registered names for error messages.
func grammarNames() []string {
	out := make([]string, 0, len(Grammars))
	for k := range Grammars {
		out = append(out, k)
	}
	return out
}
