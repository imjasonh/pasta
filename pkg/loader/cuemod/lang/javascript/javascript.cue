// Package javascript declares the built-in JavaScript language config.
package javascript

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "javascript"
	extensions: [".js", ".mjs", ".cjs"]
	comment_types: ["comment"]
}

Name: Config.grammar
