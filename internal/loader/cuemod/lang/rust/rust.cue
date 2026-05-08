// Package rust declares the built-in Rust language config for pasta.
package rust

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "rust"
	extensions: [".rs"]
	// Doc comments (`///`, `/** */`) are a separate node type.
	comment_types: ["line_comment", "block_comment", "doc_comment"]
}

Name: Config.grammar
