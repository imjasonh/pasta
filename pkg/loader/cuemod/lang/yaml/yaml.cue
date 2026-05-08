// Package yaml declares the built-in YAML language config.
package yaml

import "pasta.dev/schema"

Config: schema.#Language & {
	grammar:    "yaml"
	extensions: [".yaml", ".yml"]
	comment_types: ["comment"]
}

Name: Config.grammar
