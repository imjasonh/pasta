// Package sql declares the built-in SQL language config for pasta.
package sql

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "sql"
	extensions: [".sql"]
	comment_types: ["comment", "marginalia"]
}

Name: Config.grammar
