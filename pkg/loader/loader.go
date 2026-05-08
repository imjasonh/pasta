// Package loader loads CUE rule files, validates them against the pasta
// schema, and decodes the result into [dsl.Analyzer] structs.
//
// Rules can `import "github.com/imjasonh/pasta/schema"` and `import "github.com/imjasonh/pasta/lang/<name>"`.
// The github.com/imjasonh/pasta module ships embedded in the binary; the loader exposes
// it to the CUE compiler by overlaying it under the user's
// cue.mod/pkg/github.com/imjasonh/pasta/ at load time. Users don't need a cue.mod of
// their own — one is synthesized if absent.
package loader

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"

	"github.com/imjasonh/pasta/pkg/dsl"
)

//go:embed all:cuemod
var embeddedFS embed.FS

// LoadResult is the parsed contents of one or more CUE files: zero or
// more analyzers and zero or more language declarations.
type LoadResult struct {
	Analyzers []*dsl.Analyzer
	Languages []dsl.LanguageDecl
}

// LoadFile loads a single .cue analyzer file from disk and returns the
// first analyzer found. Convenience wrapper around LoadDir for
// single-file callers; languages are ignored.
func LoadFile(path string) (*dsl.Analyzer, error) {
	res, err := loadPath(path)
	if err != nil {
		return nil, err
	}
	if len(res.Analyzers) == 0 {
		return nil, fmt.Errorf("no analyzer binding found in input")
	}
	return res.Analyzers[0], nil
}

// LoadDir loads every *.cue file in dir and returns all analyzers and
// language declarations found.
func LoadDir(dir string) (LoadResult, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.cue"))
	if err != nil {
		return LoadResult{}, err
	}
	if len(matches) == 0 {
		return LoadResult{}, fmt.Errorf("no *.cue files in %s", dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return LoadResult{}, err
	}
	overlay, err := buildOverlay(abs, "")
	if err != nil {
		return LoadResult{}, err
	}
	cfg := &load.Config{
		Dir:     abs,
		Overlay: overlay,
	}
	// Pass each absolute file path to load.Instances.
	absFiles := make([]string, len(matches))
	for i, m := range matches {
		af, err := filepath.Abs(m)
		if err != nil {
			return LoadResult{}, err
		}
		absFiles[i] = af
	}
	insts := load.Instances(absFiles, cfg)

	var out LoadResult
	for _, inst := range insts {
		if inst.Err != nil {
			return LoadResult{}, fmt.Errorf("load %s: %w", inst.Dir, inst.Err)
		}
		ctx := cuecontext.New()
		v := ctx.BuildInstance(inst)
		if err := v.Err(); err != nil {
			return LoadResult{}, fmt.Errorf("build %s: %w", inst.Dir, err)
		}
		if err := v.Validate(cue.Concrete(true)); err != nil {
			return LoadResult{}, fmt.Errorf("validate %s: %w", inst.Dir, err)
		}
		extracted, err := extractTopLevel(v)
		if err != nil {
			return LoadResult{}, err
		}
		out.Analyzers = append(out.Analyzers, extracted.Analyzers...)
		out.Languages = append(out.Languages, extracted.Languages...)
	}
	return out, nil
}

func loadPath(path string) (LoadResult, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return LoadResult{}, err
	}
	userDir := filepath.Dir(abs)
	overlay, err := buildOverlay(userDir, abs)
	if err != nil {
		return LoadResult{}, err
	}
	cfg := &load.Config{
		Dir:     userDir,
		Overlay: overlay,
	}
	insts := load.Instances([]string{abs}, cfg)
	if len(insts) == 0 {
		return LoadResult{}, fmt.Errorf("no CUE instances loaded from %s", abs)
	}
	if err := insts[0].Err; err != nil {
		return LoadResult{}, fmt.Errorf("load %s: %w", abs, err)
	}
	ctx := cuecontext.New()
	v := ctx.BuildInstance(insts[0])
	if err := v.Err(); err != nil {
		return LoadResult{}, fmt.Errorf("build %s: %w", abs, err)
	}
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return LoadResult{}, fmt.Errorf("validate %s: %w", abs, err)
	}
	return extractTopLevel(v)
}

// LoadFS is retained for tests that supply CUE via an in-memory fs.FS.
// It writes each file to a tempdir and calls LoadFile on the first.
func LoadFS(fsys fs.FS, paths ...string) (*dsl.Analyzer, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths supplied")
	}
	tmp, err := os.MkdirTemp("", "pasta-load-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	for _, p := range paths {
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		if err := os.WriteFile(filepath.Join(tmp, filepath.Base(p)), b, 0o644); err != nil {
			return nil, err
		}
	}
	return LoadFile(filepath.Join(tmp, filepath.Base(paths[0])))
}

// buildOverlay synthesizes any missing cue.mod for the user directory
// and vendors the embedded github.com/imjasonh/pasta module under
// <userDir>/cue.mod/pkg/github.com/imjasonh/pasta/.
func buildOverlay(userDir, userFile string) (map[string]load.Source, error) {
	overlay := map[string]load.Source{}

	// Synthesize a minimal cue.mod/module.cue if the user's directory
	// doesn't already have one. This declares the user's directory as
	// an anonymous module so its imports resolve via cue.mod/pkg/.
	userMod := filepath.Join(userDir, "cue.mod", "module.cue")
	if _, err := os.Stat(userMod); os.IsNotExist(err) {
		overlay[userMod] = load.FromString(`module: "local.pasta-rule"
language: version: "v0.13.0"
`)
	} else if err != nil {
		return nil, fmt.Errorf("stat %s: %w", userMod, err)
	}

	// Vendor github.com/imjasonh/pasta: walk embedded cuemod/ and place each file at
	// the user's cue.mod/pkg/github.com/imjasonh/pasta/<rel>.
	root := "cuemod"
	err := fs.WalkDir(embeddedFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := embeddedFS.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, root+"/")
		target := filepath.Join(userDir, "cue.mod", "pkg", "github.com/imjasonh/pasta", rel)
		overlay[target] = load.FromBytes(b)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// If the user's CUE file is anonymous (no package), the loader needs
	// the file path explicitly to know what to load. The overlay already
	// has the file on disk so no extra entry is needed.
	_ = userFile

	return overlay, nil
}

// extractTopLevel iterates non-definition top-level fields of v and
// classifies each as either an Analyzer or a LanguageDecl based on
// shape: Analyzer must have non-empty `name` and `rules`; LanguageDecl
// must have non-empty `grammar` and `extensions`.
func extractTopLevel(v cue.Value) (LoadResult, error) {
	iter, err := v.Fields(cue.Definitions(false))
	if err != nil {
		return LoadResult{}, fmt.Errorf("iterate fields: %w", err)
	}
	var out LoadResult
	for iter.Next() {
		key := iter.Selector().String()
		val := iter.Value()

		if a, ok := tryDecodeAnalyzer(val); ok {
			out.Analyzers = append(out.Analyzers, a)
			continue
		}
		if l, ok := tryDecodeLanguage(val); ok {
			l.Name = key
			out.Languages = append(out.Languages, l)
			continue
		}
	}
	return out, nil
}

func tryDecodeAnalyzer(v cue.Value) (*dsl.Analyzer, bool) {
	b, err := v.MarshalJSON()
	if err != nil {
		return nil, false
	}
	var a dsl.Analyzer
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, false
	}
	if a.Name == "" || len(a.Rules) == 0 {
		return nil, false
	}
	for k, r := range a.Rules {
		if r.Name == "" {
			r.Name = k
			a.Rules[k] = r
		}
	}
	return &a, true
}

func tryDecodeLanguage(v cue.Value) (dsl.LanguageDecl, bool) {
	b, err := v.MarshalJSON()
	if err != nil {
		return dsl.LanguageDecl{}, false
	}
	var l dsl.LanguageDecl
	if err := json.Unmarshal(b, &l); err != nil {
		return dsl.LanguageDecl{}, false
	}
	if l.Grammar == "" || len(l.Extensions) == 0 {
		return dsl.LanguageDecl{}, false
	}
	return l, true
}

// EmbeddedFS exposes the embedded github.com/imjasonh/pasta module so other packages
// (e.g. pkg/lang) can load language configs from the same source.
func EmbeddedFS() embed.FS { return embeddedFS }
