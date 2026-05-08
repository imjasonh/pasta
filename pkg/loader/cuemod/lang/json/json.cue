// Package json declares the built-in JSON language config.
package json

import "pasta.dev/schema"

Config: schema.#Language & {
	grammar:    "json"
	extensions: [".json"]
	// JSON spec doesn't allow comments; tree-sitter-json may have a
	// `comment` node type for non-strict parses. Including for safety.
	comment_types: ["comment"]
}

Name: Config.grammar
