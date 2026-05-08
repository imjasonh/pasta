// Package go declares the built-in Go language config for pasta.
//
// Import as:
//
//	import golang "github.com/imjasonh/pasta/lang/go"
//
// then reference `golang.Config` in rules' languages list.
package go

import "github.com/imjasonh/pasta/schema"

// Config is the language registration for Go.
Config: schema.#Language & {
	grammar:    "go"
	extensions: [".go"]
	// tree-sitter-go uses one comment type for both `//` and `/* */`.
	comment_types: ["comment"]
}

// Name re-exports Config.grammar for convenience in rule `languages` lists.
Name: Config.grammar
