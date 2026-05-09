// Package dockerfile declares the built-in Dockerfile language config.
//
// Dockerfile has no canonical extension — the file is usually named
// `Dockerfile` with no extension. Pasta's testdata layout uses one
// file per language by extension, so we register `.dockerfile` as
// the synthetic test extension. Real-world dispatch (e.g. via the
// LSP) can match by filename pattern at the editor layer.
package dockerfile

import "github.com/imjasonh/pasta/schema"

Config: schema.#Language & {
	grammar:    "dockerfile"
	extensions: [".dockerfile"]
	comment_types: ["comment"]
}

Name: Config.grammar
