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
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/remote"
)

//go:embed all:cuemod
var embeddedFS embed.FS

// LoadResult is the parsed contents of one or more CUE files: zero or
// more analyzers and zero or more language declarations, plus an
// optional Config when the directory carried a config.cue.
type LoadResult struct {
	Analyzers []*dsl.Analyzer
	Languages []dsl.LanguageDecl
	Config    *Config
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
	// pasta.cue carries both the remote-imports manifest and the
	// project config — drop it from the rule list (the manifest is
	// vendored into the overlay separately by buildOverlay; the
	// config is read directly via LoadConfig).
	matches = filterManifest(matches)
	projectCfg, _, err := LoadConfig(dir)
	if err != nil {
		return LoadResult{}, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return LoadResult{}, err
	}
	overlay, remoteDirs, err := buildOverlay(abs, "")
	if err != nil {
		return LoadResult{}, err
	}
	if len(matches) == 0 && len(remoteDirs) == 0 {
		// A rule directory with neither local files nor a manifest
		// listing remote modules has nothing to load. (A directory
		// with only a manifest IS valid — its rules come entirely
		// from auto-enrolled remote modules.)
		return LoadResult{}, fmt.Errorf("no *.cue files in %s", dir)
	}
	cfg := &load.Config{
		Dir:     abs,
		Overlay: overlay,
	}

	var local LoadResult
	if len(matches) > 0 {
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
		for _, inst := range insts {
			if inst.Err != nil {
				return LoadResult{}, fmt.Errorf("load %s: %s", inst.Dir, cueErrDetails(inst.Err))
			}
			ctx := cuecontext.New()
			v := ctx.BuildInstance(inst)
			if err := v.Err(); err != nil {
				return LoadResult{}, fmt.Errorf("build %s: %s", inst.Dir, cueErrDetails(err))
			}
			if err := v.Validate(cue.Concrete(true)); err != nil {
				return LoadResult{}, fmt.Errorf("validate %s: %s", inst.Dir, cueErrDetails(err))
			}
			extracted, err := extractTopLevel(v)
			if err != nil {
				return LoadResult{}, err
			}
			local.Analyzers = append(local.Analyzers, extracted.Analyzers...)
			local.Languages = append(local.Languages, extracted.Languages...)
		}
	}

	// Auto-enroll analyzers from every imported remote module. This
	// is what lets a project publish ready-to-run rules: a consumer
	// that lists the module in pasta.cue gets every top-level
	// analyzer it exports as if those rules lived in .pasta/.
	fromRemote, err := loadRemoteAnalyzers(abs, remoteDirs, overlay)
	if err != nil {
		return LoadResult{}, err
	}
	merged, err := mergeAnalyzers(local, fromRemote)
	if err != nil {
		return LoadResult{}, err
	}
	applyConfig(projectCfg, merged.Analyzers)
	merged.Config = projectCfg
	return merged, nil
}

// cueErrDetails formats a CUE error with full per-position detail.
// CUE's default Error() summarizes disjunction failures as "N errors in
// empty disjunction" which hides the offending value. Details() expands
// every leaf so users see e.g. `check: "no_such_check": ...`.
func cueErrDetails(err error) string {
	cerr, ok := err.(errors.Error)
	if !ok {
		return err.Error()
	}
	return errors.Details(cerr, nil)
}

// LoadLang loads a single embedded lang/<name>/<name>.cue file and
// returns the decoded Config field as a LanguageDecl. langPath is the
// path WITHIN the embedded FS, e.g. "cuemod/lang/go/go.cue".
func LoadLang(langPath string) (dsl.LanguageDecl, error) {
	src, err := embeddedFS.ReadFile(langPath)
	if err != nil {
		return dsl.LanguageDecl{}, fmt.Errorf("read embedded %s: %w", langPath, err)
	}
	// Use a synthetic workdir so the overlay's cue.mod/pkg vendoring
	// doesn't collide with anything on disk.
	virtualPath := filepath.Join(string(filepath.Separator), "embedded", langPath)

	v, err := buildCUE(src, virtualPath)
	if err != nil {
		return dsl.LanguageDecl{}, err
	}
	cfgVal := v.LookupPath(cue.ParsePath("Config"))
	if !cfgVal.Exists() {
		return dsl.LanguageDecl{}, fmt.Errorf("%s: no Config field", langPath)
	}
	jb, err := cfgVal.MarshalJSON()
	if err != nil {
		return dsl.LanguageDecl{}, fmt.Errorf("marshal %s: %w", langPath, err)
	}
	var ld dsl.LanguageDecl
	if err := json.Unmarshal(jb, &ld); err != nil {
		return dsl.LanguageDecl{}, fmt.Errorf("decode %s: %w", langPath, err)
	}
	// Name comes from the directory base (e.g. cuemod/lang/go/go.cue
	// -> "go").
	ld.Name = filepath.Base(filepath.Dir(langPath))
	return ld, nil
}

// loadBytes is the core CUE-loading path: take src (the file
// contents) and a virtualPath where it logically lives, build the
// overlay (cue.mod synthesis + module vendoring), compile, validate,
// and extract Analyzers / Languages.
//
// `virtualPath` doesn't need to exist on disk — the bytes are passed
// directly via the overlay. Used by:
//   - loadPath, after reading the file from disk
//   - LoadLang, after reading from the embedded FS
//   - tests, which can pass arbitrary bytes without touching disk
func loadBytes(src []byte, virtualPath string) (LoadResult, error) {
	v, err := buildCUE(src, virtualPath)
	if err != nil {
		return LoadResult{}, err
	}
	return extractTopLevel(v)
}

// buildCUE runs the overlay → load → build → validate pipeline on
// src placed at virtualPath, returning a CUE value ready for decoding.
// Used by loadPath, LoadLang, loadBytes, and tests. Single-file
// callers don't auto-enroll remote-module analyzers — that's a
// LoadDir-only convenience — but the overlay still vendors any
// declared remote modules so explicit `import` statements resolve.
func buildCUE(src []byte, virtualPath string) (cue.Value, error) {
	workDir := filepath.Dir(virtualPath)
	overlay, _, err := buildOverlay(workDir, virtualPath)
	if err != nil {
		return cue.Value{}, err
	}
	overlay[virtualPath] = load.FromBytes(src)

	cfg := &load.Config{Dir: workDir, Overlay: overlay}
	insts := load.Instances([]string{virtualPath}, cfg)
	if len(insts) == 0 {
		return cue.Value{}, fmt.Errorf("no instances loaded from %s", virtualPath)
	}
	if err := insts[0].Err; err != nil {
		return cue.Value{}, fmt.Errorf("load %s: %s", virtualPath, cueErrDetails(err))
	}
	ctx := cuecontext.New()
	v := ctx.BuildInstance(insts[0])
	if err := v.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("build %s: %s", virtualPath, cueErrDetails(err))
	}
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return cue.Value{}, fmt.Errorf("validate %s: %s", virtualPath, cueErrDetails(err))
	}
	return v, nil
}

func loadPath(path string) (LoadResult, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return LoadResult{}, err
	}
	src, err := os.ReadFile(abs)
	if err != nil {
		return LoadResult{}, fmt.Errorf("read %s: %w", abs, err)
	}
	return loadBytes(src, abs)
}

// buildOverlay synthesizes any missing cue.mod for the user directory
// and vendors the embedded github.com/imjasonh/pasta module under
// <userDir>/cue.mod/pkg/github.com/imjasonh/pasta/.
//
// If userDir contains a pasta.cue manifest, every module it lists
// (resolved via the lockfile + cache) is also vendored under
// <userDir>/cue.mod/pkg/<modulePath>/, so user rules can import them
// the same way they import the built-in module.
//
// Returns the overlay plus a map of modulePath → on-disk cache dir
// for every remote module brought in (empty when no manifest). Only
// LoadDir consumes the dirs map — it auto-enrolls every analyzer
// found under those dirs as a runnable rule, so a project can list
// `imports` and have those rules just work without per-rule glue.
func buildOverlay(userDir, userFile string) (map[string]load.Source, map[string]string, error) {
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
		return nil, nil, fmt.Errorf("stat %s: %w", userMod, err)
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
		return nil, nil, err
	}

	// Vendor any remote modules declared in pasta.cue. If the
	// lockfile is missing or stale relative to the manifest,
	// vendorRemoteModules transparently runs a sync first — manifest
	// edits don't require a separate `pasta sync` step.
	remoteDirs, err := vendorRemoteModules(userDir, overlay)
	if err != nil {
		return nil, nil, err
	}

	// If the user's CUE file is anonymous (no package), the loader needs
	// the file path explicitly to know what to load. The overlay already
	// has the file on disk so no extra entry is needed.
	_ = userFile

	return overlay, remoteDirs, nil
}

// vendorRemoteModules reads dir/pasta.cue + dir/pasta.lock, copies
// each cached module's files into overlay under
// <dir>/cue.mod/pkg/<modulePath>/, and returns the modulePath →
// on-disk cache dir map so the caller can also enroll the modules'
// analyzers as rules.
//
// Sync is implicit: if the lockfile is missing or stale relative to
// the manifest, this calls remote.Sync first to resolve versions,
// fetch new commits, and write the lockfile. That makes manifest
// edits "just work" on the next run instead of requiring an
// out-of-band `pasta sync`. An explicit `pasta sync` (and
// `pasta sync --check`) still exists for warming the cache and CI
// gating. The fetcher is obtained from newDefaultFetcher (a package
// var) so tests can swap in a fake without going through
// DefaultCacheDir.
func vendorRemoteModules(dir string, overlay map[string]load.Source) (map[string]string, error) {
	manifest, ok, err := remote.LoadManifest(dir)
	if err != nil {
		return nil, err
	}
	if !ok || len(manifest.Modules) == 0 {
		return nil, nil
	}
	f, err := newDefaultFetcher()
	if err != nil {
		return nil, err
	}
	lf, lfExists, err := remote.LoadLockfile(dir)
	if err != nil {
		return nil, err
	}
	// Run sync transparently when the lock is missing or doesn't
	// match the manifest. The fetcher's Fetch (version → commit) is
	// only called on entries that actually need re-resolving;
	// everything else passes through Sync's cache-reuse path.
	if !lfExists {
		lf, err = remote.Sync(dir, manifest, f)
		if err != nil {
			return nil, fmt.Errorf("auto-sync %s: %w", dir, err)
		}
	} else if inSync, _ := remote.IsInSync(manifest, lf); !inSync {
		lf, err = remote.Sync(dir, manifest, f)
		if err != nil {
			return nil, fmt.Errorf("auto-sync %s: %w", dir, err)
		}
	}
	dirs, err := remote.VendorDirs(manifest, lf, f)
	if err != nil {
		return nil, err
	}
	for modPath, modDir := range dirs {
		err := filepath.WalkDir(modDir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			rel, rerr := filepath.Rel(modDir, p)
			if rerr != nil {
				return rerr
			}
			target := filepath.Join(dir, "cue.mod", "pkg", filepath.FromSlash(modPath), rel)
			overlay[target] = load.FromBytes(b)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("vendor %s: %w", modPath, err)
		}
	}
	return dirs, nil
}

// newDefaultFetcher returns the production GitFetcher pointed at the
// resolved cache dir. Indirected through a package var so tests can
// swap it via SetFetcherForTesting.
var newDefaultFetcher = func() (remote.Fetcher, error) {
	cache, err := remote.DefaultCacheDir()
	if err != nil {
		return nil, err
	}
	return &remote.GitFetcher{CacheDir: cache}, nil
}

// SetFetcherForTesting replaces the fetcher used to resolve remote
// module imports. Returns a cleanup function that restores the
// previous fetcher; callers should defer it or pass it to t.Cleanup.
//
// Exported so tests in other packages (notably the runner's
// load→engine→diagnostics e2e) can stage in-memory modules without
// touching git or the real cache dir.
func SetFetcherForTesting(f remote.Fetcher) func() {
	prev := newDefaultFetcher
	newDefaultFetcher = func() (remote.Fetcher, error) { return f, nil }
	return func() { newDefaultFetcher = prev }
}

// filterManifest drops the pasta.cue manifest from a list of *.cue
// glob results — it carries imports + project config, not rule
// definitions, and would fail rule-schema validation if loaded as one.
func filterManifest(paths []string) []string {
	out := paths[:0]
	for _, p := range paths {
		if filepath.Base(p) == remote.ManifestFile {
			continue
		}
		out = append(out, p)
	}
	return out
}

// loadRemoteAnalyzers walks each vendored remote module dir, loads
// every CUE package within, and returns the analyzers + language
// declarations it finds. Subdirectories named `cue.mod` (CUE module
// metadata) and `pasta.cue` files (pasta manifests, not rules) are
// skipped. Each subdirectory containing `*.cue` files is loaded as
// one CUE package, mirroring how LoadDir treats the local rule dir.
//
// remoteDirs is the modulePath → cache-dir map returned by
// vendorRemoteModules; overlay is the same map passed to that call
// so CUE can resolve cross-module imports during the package builds.
func loadRemoteAnalyzers(userDir string, remoteDirs map[string]string, overlay map[string]load.Source) (LoadResult, error) {
	var out LoadResult
	// Iterate in sorted order so error messages and duplicate-detection
	// are deterministic.
	paths := make([]string, 0, len(remoteDirs))
	for p := range remoteDirs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, modPath := range paths {
		modDir := remoteDirs[modPath]
		// Find every distinct subdirectory (relative to modDir) that
		// contains *.cue files. Each becomes one CUE package that
		// we load by its import path. Loading by import path
		// requires cfg.Dir to be the userDir (which has the
		// synthesized cue.mod/module.cue); cfg.Dir = pkgDir confuses
		// CUE because cue.mod/pkg/<...> isn't itself a module root.
		pkgRels := map[string]bool{}
		err := filepath.WalkDir(modDir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				// Skip a remote module's own cue.mod (its module
				// metadata). We're scanning for rules, not for
				// nested CUE module config.
				if d.Name() == "cue.mod" {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if name == remote.ManifestFile {
				return nil
			}
			if !strings.HasSuffix(name, ".cue") {
				return nil
			}
			rel, err := filepath.Rel(modDir, filepath.Dir(p))
			if err != nil {
				return err
			}
			pkgRels[filepath.ToSlash(rel)] = true
			return nil
		})
		if err != nil {
			return LoadResult{}, fmt.Errorf("scan %s: %w", modPath, err)
		}
		rels := make([]string, 0, len(pkgRels))
		for r := range pkgRels {
			rels = append(rels, r)
		}
		sort.Strings(rels)
		for _, rel := range rels {
			importPath := modPath
			if rel != "." {
				importPath = modPath + "/" + rel
			}
			cfg := &load.Config{Dir: userDir, Overlay: overlay}
			insts := load.Instances([]string{importPath}, cfg)
			for _, inst := range insts {
				if inst.Err != nil {
					return LoadResult{}, fmt.Errorf("load %s: %s", importPath, cueErrDetails(inst.Err))
				}
				ctx := cuecontext.New()
				v := ctx.BuildInstance(inst)
				if err := v.Err(); err != nil {
					return LoadResult{}, fmt.Errorf("build %s: %s", importPath, cueErrDetails(err))
				}
				// Deliberately skip Validate(Concrete(true)): a
				// remote module may legitimately export incomplete
				// helpers (recipe templates, partial schemas) next
				// to fully-concrete analyzers. extractTopLevel +
				// tryDecodeAnalyzer already filter out anything
				// that doesn't marshal to a valid Analyzer JSON, so
				// incomplete fields are silently skipped here
				// instead of erroring.
				extracted, err := extractTopLevel(v)
				if err != nil {
					return LoadResult{}, err
				}
				// Tag each analyzer with its source module so
				// merge-time collision messages can point at the
				// right place. Source is otherwise unused.
				for _, a := range extracted.Analyzers {
					a.Source = modPath
				}
				out.Analyzers = append(out.Analyzers, extracted.Analyzers...)
				out.Languages = append(out.Languages, extracted.Languages...)
			}
		}
	}
	return out, nil
}

// mergeAnalyzers combines local and remote LoadResults into one.
// Naming policy:
//   - Local analyzers always take precedence: a local analyzer with
//     the same name as a remote one wins, and we print a warning to
//     stderr so the user knows the remote version was suppressed.
//     This lets projects patch a remote rule without forking the
//     module.
//   - Two remote analyzers with the same name (from different
//     modules) is an error — there's no principled way for us to
//     pick a winner, and silently dropping one would surprise either
//     publisher.
//   - Two analyzers with the same name from the SAME remote module
//     is also an error (the publisher messed up).
//
// Languages are concatenated as-is; the lang registry already
// rejects duplicates at registration time.
func mergeAnalyzers(local, remote LoadResult) (LoadResult, error) {
	out := LoadResult{Languages: append(local.Languages, remote.Languages...)}
	// Index local analyzers by name so collisions are O(1) to
	// detect. Local analyzers retain insertion order in the output.
	localByName := map[string]bool{}
	for _, a := range local.Analyzers {
		localByName[a.Name] = true
		out.Analyzers = append(out.Analyzers, a)
	}
	// Track which remote module first contributed each name so we
	// can produce a useful error on inter-module collision.
	remoteByName := map[string]string{}
	for _, a := range remote.Analyzers {
		if localByName[a.Name] {
			fmt.Fprintf(os.Stderr, "pasta: local analyzer %q overrides remote one from %s\n", a.Name, a.Source)
			continue
		}
		if prev, ok := remoteByName[a.Name]; ok {
			return LoadResult{}, fmt.Errorf("analyzer name collision: %q is exported by both %s and %s; rename one or stop importing the other",
				a.Name, prev, a.Source)
		}
		remoteByName[a.Name] = a.Source
		out.Analyzers = append(out.Analyzers, a)
	}
	return out, nil
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
			if err := validateCaptures(a); err != nil {
				return LoadResult{}, fmt.Errorf("analyzer %q: %w", a.Name, err)
			}
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
// (e.g. internal/lang) can load language configs from the same source.
func EmbeddedFS() embed.FS { return embeddedFS }
