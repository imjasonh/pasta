// notgo demonstrates two CUE-module capabilities:
//
//  1. A user can declare a NEW language entirely in CUE — without a
//     pasta release — by importing pasta.dev/schema and emitting a
//     #Language value. The language's `grammar` field references a
//     grammar registered in the pasta binary.
//
//  2. A language can EXTEND an existing one by reusing its `grammar`
//     and `comment_types`. Here, .notgo files are tree-sitter-go under
//     a different file extension. Any rule whose `languages` list
//     includes the underlying grammar (e.g. `[golang.Name]`) fires on
//     .notgo files automatically — no rule changes required.
//
// The runner picks up #Language values from any *.cue in this
// directory and registers them at startup, so the framework knows
// `.notgo` files exist.

package notgo_alias

import (
	"pasta.dev/schema"
	golang "pasta.dev/lang/go"
)

notgo: schema.#Language & {
	// Reuse the Go grammar so all Go rules apply.
	grammar:    golang.Config.grammar
	extensions: [".notgo"]
	// Same comment node types as Go.
	comment_types: golang.Config.comment_types
}
