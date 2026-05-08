// Built-in language metadata for pasta. Loaded at startup from embed.FS.
//
// Each entry is a Language. Each Language references a `grammar` by name;
// the grammar must be registered in internal/lang/grammars.go (a small Go map
// of name → tree-sitter GetLanguage). Adding a brand-new grammar still
// requires a Go-side change because tree-sitter grammars are linked via
// cgo. Everything else (extensions, comment types, language aliases) is
// just data here.
//
// Users can supply their own CUE language config via `--lang-config
// <path>`; the entries merge on top of these defaults (later wins).

languages: {
	go: {
		grammar:    "go"
		extensions: [".go"]
		// tree-sitter-go uses one comment type for both `//` and `/* */`.
		comment_types: ["comment"]
	}
	python: {
		grammar:    "python"
		extensions: [".py"]
		// Python `"""..."""` parses as a string node, not a comment.
		comment_types: ["comment"]
	}
	rust: {
		grammar:    "rust"
		extensions: [".rs"]
		// Doc comments (`///`, `/** */`) are a separate node type.
		comment_types: ["line_comment", "block_comment", "doc_comment"]
	}
}
