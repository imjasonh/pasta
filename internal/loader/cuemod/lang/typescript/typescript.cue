// Package typescript declares the built-in TypeScript language config.
package typescript

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "typescript"
	extensions: [".ts"]
	comment_types: ["comment"]
}

Name: Config.grammar
