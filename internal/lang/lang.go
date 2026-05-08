// Package lang exposes the registered set of tree-sitter languages.
//
// Language metadata (name, file extensions, comment node types) is
// declared in CUE under github.com/imjasonh/pasta/lang/<name> and loaded at startup
// from the embedded built-in module shipped by internal/loader. The only
// non-CUE component is internal/lang/grammars.go, a small map from grammar
// name to its gotreesitter GetLanguage function — required because
// gotreesitter grammars are normal Go imports.
//
// Adding a new language alias for an existing grammar is a CUE-only
// change. Adding a brand-new grammar requires editing grammars.go.
package lang

import (
	"fmt"
	"io/fs"
	"strings"

	gts "github.com/odvcencio/gotreesitter"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/loader"
	"github.com/imjasonh/pasta/internal/tsutil"
)

// Language is the runtime view of a registered language.
type Language struct {
	Name         string
	Grammar      string
	Extensions   []string
	CommentTypes []string
	GetLanguage  func() *gts.Language
}

// All is the live registry, populated from the embedded github.com/imjasonh/pasta module.
var All map[string]Language

func init() {
	m, err := loadEmbedded()
	if err != nil {
		panic(fmt.Sprintf("lang: load embedded: %v", err))
	}
	All = m
}

// loadEmbedded walks the embedded github.com/imjasonh/pasta/lang/*
// tree, loading each lang/<name>/<name>.cue through the same overlay-
// based CUE loader as user rule files.
func loadEmbedded() (map[string]Language, error) {
	embeddedFS := loader.EmbeddedFS()
	out := map[string]Language{}
	err := fs.WalkDir(embeddedFS, "cuemod/lang", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".cue") {
			return nil
		}
		ld, err := loader.LoadLang(p)
		if err != nil {
			return err
		}
		gl, ok := Grammars[ld.Grammar]
		if !ok {
			return fmt.Errorf("%s: unknown grammar %q (registered: %v)", p, ld.Grammar, grammarNames())
		}
		out[ld.Name] = Language{
			Name:         ld.Name,
			Grammar:      ld.Grammar,
			Extensions:   ld.Extensions,
			CommentTypes: ld.CommentTypes,
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
