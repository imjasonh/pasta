// Package html declares the built-in HTML language config for pasta.
package html

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "html"
	extensions: [".html", ".htm"]
	comment_types: ["comment"]
}

Name: Config.grammar
