// Package c declares the built-in C language config for pasta.
package c

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "c"
	extensions: [".c", ".h"]
	comment_types: ["comment"]
}

Name: Config.grammar
