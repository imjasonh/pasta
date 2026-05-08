// Package bash declares the built-in Bash language config.
package bash

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "bash"
	extensions: [".sh", ".bash"]
	comment_types: ["comment"]
}

Name: Config.grammar
