// Package css declares the built-in CSS language config for pasta.
package css

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "css"
	extensions: [".css"]
	comment_types: ["comment"]
}

Name: Config.grammar
