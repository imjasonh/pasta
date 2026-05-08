// Package lang exposes the registered set of tree-sitter languages.
//
// Language metadata (name, file extensions, comment node types) is
// declared in CUE under pasta.dev/lang/<name> and loaded at startup
// from the embedded built-in module shipped by pkg/loader. The only
// non-CUE component is pkg/lang/grammars.go, a small map from grammar
// name to its gotreesitter GetLanguage function — required because
// gotreesitter grammars are normal Go imports.
//
// Adding a new language alias for an existing grammar is a CUE-only
// change. Adding a brand-new grammar requires editing grammars.go.
package lang

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	gts "github.com/odvcencio/gotreesitter"

	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/loader"
	"github.com/imjasonh/pasta/pkg/tsutil"
)

// Language is the runtime view of a registered language.
type Language struct {
	Name         string
	Grammar      string
	Extensions   []string
	CommentTypes []string
	GetLanguage  func() *gts.Language
}

// All is the live registry, populated from the embedded pasta.dev module.
var All map[string]Language

func init() {
	m, err := loadEmbedded()
	if err != nil {
		panic(fmt.Sprintf("lang: load embedded: %v", err))
	}
	All = m
}

// loadEmbedded walks the embedded pasta.dev/lang/* tree, compiling each
// language CUE file with its schema dependency. Each file exports a
// `Config` value of shape #Language; we decode that into Language.
func loadEmbedded() (map[string]Language, error) {
	embeddedFS := loader.EmbeddedFS()

	// Read schema source so we can satisfy the lang packages' import.
	schemaSrc, err := embeddedFS.ReadFile("cuemod/schema/schema.cue")
	if err != nil {
		return nil, fmt.Errorf("read schema: %w", err)
	}

	out := map[string]Language{}
	err = fs.WalkDir(embeddedFS, "cuemod/lang", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".cue") {
			return nil
		}
		langSrc, err := embeddedFS.ReadFile(p)
		if err != nil {
			return err
		}

		// Compile schema + lang file together. Strip imports and the
		// `pasta.dev/schema.` prefix from references so this works as
		// one anonymous compilation unit. (We use the heavier
		// load.Instances pipeline only for user CUE; for our own
		// already-trusted built-ins, this is simpler and faster.)
		combined := stripPackage(string(schemaSrc)) + "\n" +
			rewriteSchemaRefs(stripPackageAndImports(string(langSrc)))

		ctx := cuecontext.New()
		v := ctx.CompileString(combined, cue.Filename(p))
		if err := v.Err(); err != nil {
			return fmt.Errorf("compile %s: %w", p, err)
		}
		cfg := v.LookupPath(cue.ParsePath("Config"))
		if !cfg.Exists() {
			return fmt.Errorf("%s: no Config field", p)
		}
		var raw struct {
			Grammar      string   `json:"grammar"`
			Extensions   []string `json:"extensions"`
			CommentTypes []string `json:"comment_types"`
		}
		jb, err := cfg.MarshalJSON()
		if err != nil {
			return fmt.Errorf("marshal %s: %w", p, err)
		}
		if err := json.Unmarshal(jb, &raw); err != nil {
			return fmt.Errorf("decode %s: %w", p, err)
		}
		gl, ok := Grammars[raw.Grammar]
		if !ok {
			return fmt.Errorf("%s: unknown grammar %q (registered: %v)", p, raw.Grammar, grammarNames())
		}

		// Use the directory name as the language name; the package
		// name in the CUE file matches.
		name := lastPathSegment(strings.TrimSuffix(p, ".cue"))
		// `p` is like `cuemod/lang/go/go.cue`; strip filename, take
		// directory base.
		dir := p[:strings.LastIndex(p, "/")]
		name = lastPathSegment(dir)
		out[name] = Language{
			Name:         name,
			Grammar:      raw.Grammar,
			Extensions:   raw.Extensions,
			CommentTypes: raw.CommentTypes,
			GetLanguage:  gl,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no languages found under cuemod/lang")
	}
	return out, nil
}

func lastPathSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// stripPackage removes the leading `package X` line.
func stripPackage(src string) string {
	out := make([]byte, 0, len(src))
	for _, line := range splitLines(src) {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "package ") {
			continue
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	return string(out)
}

// stripPackageAndImports removes the package line AND any `import (...)`
// or `import "..."` blocks.
func stripPackageAndImports(src string) string {
	src = stripPackage(src)
	// Remove import blocks naively. CUE's import syntax: either a single
	// `import "path"` or an `import ( ... )` block.
	for {
		idx := strings.Index(src, "import")
		if idx < 0 {
			break
		}
		// Find the boundary at idx-0 — must be at start of line or
		// preceded by whitespace.
		if idx > 0 {
			prev := src[idx-1]
			if prev != '\n' && prev != ' ' && prev != '\t' {
				break
			}
		}
		rest := src[idx:]
		// Match `import (` ... `)`
		if strings.HasPrefix(rest, "import (") || strings.HasPrefix(rest, "import(") {
			end := strings.Index(rest, ")")
			if end < 0 {
				break
			}
			src = src[:idx] + rest[end+1:]
			continue
		}
		// Match single-line `import "..."` or `import name "..."`
		nl := strings.Index(rest, "\n")
		if nl < 0 {
			src = src[:idx]
			break
		}
		src = src[:idx] + rest[nl+1:]
	}
	return src
}

// rewriteSchemaRefs strips the `schema.` prefix from references like
// `schema.#Language` so they resolve against the inlined schema.
func rewriteSchemaRefs(src string) string {
	return strings.ReplaceAll(src, "schema.#", "#")
}

func splitLines(s string) []string {
	out := make([]string, 0, 32)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// ByExt returns the language registered for the given file extension
// (which must include the leading ".").
func ByExt(ext string) (Language, bool) {
	for _, l := range All {
		for _, e := range l.Extensions {
			if e == ext {
				return l, true
			}
		}
	}
	return Language{}, false
}

// ByName returns the language registered with the given name.
func ByName(name string) (Language, bool) {
	l, ok := All[name]
	return l, ok
}

// Register adds (or replaces) a language in the live registry. The
// LanguageDecl's `Grammar` must reference a registered grammar in the
// Grammars map. Returns an error if the grammar is unknown.
//
// Used by the runner to register languages declared in a user's rule
// directory before executing rules.
func Register(d dsl.LanguageDecl) error {
	gl, ok := Grammars[d.Grammar]
	if !ok {
		return fmt.Errorf("language %q references unknown grammar %q (registered: %v)",
			d.Name, d.Grammar, grammarNames())
	}
	All[d.Name] = Language{
		Name:         d.Name,
		Grammar:      d.Grammar,
		Extensions:   d.Extensions,
		CommentTypes: d.CommentTypes,
		GetLanguage:  gl,
	}
	return nil
}

// StmtList is the language's statement-list provider for adjacency
// matching: every named child of the container, with comments filtered
// out.
func (l Language) StmtList(container tsutil.Node) []tsutil.Node {
	skip := make(map[string]bool, len(l.CommentTypes))
	for _, t := range l.CommentTypes {
		skip[t] = true
	}
	all := container.NamedChildren()
	out := make([]tsutil.Node, 0, len(all))
	for _, n := range all {
		if skip[n.Type()] {
			continue
		}
		out = append(out, n)
	}
	return out
}
