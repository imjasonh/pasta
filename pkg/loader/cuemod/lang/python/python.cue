// Package python declares the built-in Python language config for pasta.
package python

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "python"
	extensions: [".py"]
	// Python `"""..."""` parses as a string node, not a comment.
	comment_types: ["comment"]
}

Name: Config.grammar
