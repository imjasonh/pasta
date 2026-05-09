// Package cpp declares the built-in C++ language config for pasta.
package cpp

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "cpp"
	extensions: [".cpp", ".cc", ".cxx", ".hpp", ".hh"]
	comment_types: ["comment"]
}

Name: Config.grammar
