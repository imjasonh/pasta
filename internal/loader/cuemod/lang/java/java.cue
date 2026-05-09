// Package java declares the built-in Java language config for pasta.
package java

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "java"
	extensions: [".java"]
	comment_types: ["line_comment", "block_comment"]
}

Name: Config.grammar
