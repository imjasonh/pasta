// Package ruby declares the built-in Ruby language config for pasta.
package ruby

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "ruby"
	extensions: [".rb"]
	comment_types: ["comment"]
}

Name: Config.grammar
