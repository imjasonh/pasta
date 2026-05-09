// Package php declares the built-in PHP language config for pasta.
package php

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "php"
	extensions: [".php"]
	comment_types: ["comment"]
}

Name: Config.grammar
