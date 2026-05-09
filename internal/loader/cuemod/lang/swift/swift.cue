// Package swift declares the built-in Swift language config for pasta.
package swift

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "swift"
	extensions: [".swift"]
	comment_types: ["comment", "multiline_comment"]
}

Name: Config.grammar
