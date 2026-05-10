// Command pasta runs CUE-defined analyzers over source files.
//
// Usage:
//
//	pasta [-fix] [-skip <dirs>] [-rules <dir>] [<source>...]
//	pasta [-fix] <rule.cue> <source> [<source>...]   single-rule form
//	pasta test [<rule-dir>...]                       run rules on their testdata/
//	pasta sync [<rule-dir>]                          fetch remote imports declared in <rule-dir>/pasta.cue
//
// With no positional rule argument, pasta loads every rule in
// `./.pasta/` (override with `-rules`) and analyzes the given sources.
// When no sources are given either, sources default to `./...`. The
// single-rule form is triggered when the first positional argument is
// an existing `.cue` file.
//
// A source argument ending in `/...` (or the literal `./...`) is
// expanded to every file under that directory whose extension maps
// to a registered language — Go-style "all packages below here".
// During expansion, directories named `.git`, `vendor`, `node_modules`,
// or `.pasta` are skipped by default; pass `-skip` with a
// comma-separated list to add more.
//
// The source file's extension determines the tree-sitter language; see
// internal/lang for the registered set. When more than one source file
// is supplied (directly or via `./...` expansion) they are analyzed as
// a single group with a shared fact store, so cross-file analyses see
// facts from every file in the run.
//
// `-fix` rewrites every source file in place with its fixed bytes —
// files whose fixed bytes are unchanged are left alone (mtime is not
// touched), so running over a clean tree is a no-op.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/lang"
	"github.com/imjasonh/pasta/internal/remote"
	"github.com/imjasonh/pasta/internal/runner"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "test":
			os.Exit(runTest(os.Args[2:]))
		case "sync":
			os.Exit(runSync(os.Args[2:]))
		}
	}
	os.Exit(runFix(os.Args[1:]))
}

// DefaultRulesDir is the conventional location for a project's pasta
// rule directory. `pasta`, `pasta sync`, and `pasta test` all default
// to this dir when no rule directory is specified on the command line.
const DefaultRulesDir = ".pasta"

func runFix(args []string) int {
	fs := flag.NewFlagSet("pasta", flag.ExitOnError)
	fix := fs.Bool("fix", false, "apply suggested fixes by rewriting each source file in place")
	skip := fs.String("skip", "", "comma-separated directory basenames to skip during ./... expansion (in addition to defaults: .git, vendor, node_modules, .pasta)")
	rulesDir := fs.String("rules", "", "directory of CUE rule files to load (default: ./"+DefaultRulesDir+")")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	analyzers, rawSources, code := selectRules(*rulesDir, fs.Args())
	if code != 0 {
		return code
	}
	if len(rawSources) == 0 {
		// No sources given: default to "./..." so `pasta` from a
		// project root with a .pasta/ dir Just Works.
		rawSources = []string{"./..."}
	}

	expanded, err := expandSources(rawSources, parseSkipDirs(*skip))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	specs := make([]runner.FileSpec, 0, len(expanded))
	exit := 0
	for _, src := range expanded {
		b, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", src, err)
			exit = 1
			continue
		}
		specs = append(specs, runner.FileSpec{Path: src, Src: b})
	}
	if len(specs) == 0 {
		return exit
	}

	results, err := runner.RunGroup(context.Background(), specs, analyzers, *fix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	for i, res := range results {
		for _, d := range res.Diagnostics {
			fmt.Fprintf(os.Stderr, "%s:%d: %s [%s]\n", res.Path, d.Line(), d.Message, d.Rule)
		}
		if !*fix {
			continue
		}
		// Skip the write when the rewrite is a no-op so we don't bump
		// mtimes for every file in a `./...` run — that would defeat
		// build caches and file watchers.
		if bytes.Equal(specs[i].Src, res.Fixed) {
			continue
		}
		if err := os.WriteFile(res.Path, res.Fixed, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", res.Path, err)
			exit = 1
		}
	}
	return exit
}

// selectRules picks between the directory form (`pasta [source...]`,
// rules loaded from -rules or ./.pasta/) and the legacy single-file
// form (`pasta rule.cue source...`, triggered when the first
// positional argument is an existing .cue file).
//
// Returns the loaded analyzers, the remaining positional args to be
// treated as sources, and a process exit code (0 = ok). Errors are
// printed to stderr.
func selectRules(rulesDirFlag string, positional []string) ([]*dsl.Analyzer, []string, int) {
	// Single-rule shortcut: first positional is an existing .cue
	// file. -rules wins if explicitly set, so users who want to mix
	// can still force directory mode.
	if rulesDirFlag == "" && len(positional) > 0 && strings.HasSuffix(positional[0], ".cue") {
		if info, err := os.Stat(positional[0]); err == nil && info.Mode().IsRegular() {
			a, err := runner.LoadRule(positional[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "load %s: %v\n", positional[0], err)
				return nil, nil, 1
			}
			return []*dsl.Analyzer{a}, positional[1:], 0
		}
	}

	dir := rulesDirFlag
	if dir == "" {
		dir = DefaultRulesDir
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		if rulesDirFlag != "" {
			fmt.Fprintf(os.Stderr, "rules directory %q: %v\n", dir, err)
		} else {
			fmt.Fprintf(os.Stderr, "no rules to run: pass a .cue rule file or create a ./%s/ directory (or use -rules <dir>)\n", DefaultRulesDir)
		}
		return nil, nil, 2
	}
	analyzers, err := runner.LoadRules(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load %s: %v\n", dir, err)
		return nil, nil, 1
	}
	return analyzers, positional, 0
}

// expandSources turns CLI source arguments into concrete file paths.
// An argument ending in `/...` (or the literal `./...` / `...`) is
// expanded to every file under that directory whose extension maps
// to a registered language; .golden files are excluded. Plain paths
// pass through unchanged. Directory basenames in skip are pruned
// during the walk.
func expandSources(args []string, skip map[string]bool) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, a := range args {
		if a == "..." || a == "./..." || strings.HasSuffix(a, "/...") {
			root := strings.TrimSuffix(a, "/...")
			if root == "" || a == "..." {
				root = "."
			}
			matches, err := walkSources(root, skip)
			if err != nil {
				return nil, fmt.Errorf("expand %s: %w", a, err)
			}
			for _, p := range matches {
				if !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
			continue
		}
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out, nil
}

// defaultSkipDirs are directory basenames pasta won't descend into
// during `./...` expansion. These hold third-party / vendored / VCS
// data, or rule definitions / their testdata, that almost no
// analyzer wants to lint — walking them turns `pasta -fix ./...`
// into an unintended whole-tree edit.
var defaultSkipDirs = map[string]bool{
	".git":         true,
	"vendor":       true,
	"node_modules": true,
	".pasta":       true,
}

// parseSkipDirs returns the union of defaultSkipDirs and the
// comma-separated user-supplied list. Empty entries are ignored.
func parseSkipDirs(extra string) map[string]bool {
	out := make(map[string]bool, len(defaultSkipDirs)+4)
	for k := range defaultSkipDirs {
		out[k] = true
	}
	for _, s := range strings.Split(extra, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out[s] = true
		}
	}
	return out
}

// walkSources walks root and returns every file with an extension
// pasta knows about (via lang.ByExt). .golden files are skipped, as
// are directories whose basename is in skip.
func walkSources(root string, skip map[string]bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			// Don't prune the walk root itself (e.g. when the user
			// explicitly aims `vendor/...` at a vendored tree they
			// do want to scan).
			if p != root && skip[name] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".golden") {
			return nil
		}
		if _, ok := lang.ByExt(filepath.Ext(name)); !ok {
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

// runSync resolves the manifest in each rule directory: fetches every
// declared remote module, populates the on-disk cache, and writes
// pasta.lock with the resolved commit SHAs and content hashes.
//
// Network access happens here — `pasta sync` is the one place we
// reach out. Subsequent `pasta` runs are offline as long as the lock
// matches the manifest and the cache still has the locked commit.
func runSync(args []string) int {
	if len(args) == 0 {
		args = []string{DefaultRulesDir}
	}
	cacheDir, err := remote.DefaultCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	f := &remote.GitFetcher{CacheDir: cacheDir}
	exit := 0
	for _, dir := range args {
		m, ok, err := remote.LoadManifest(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
			exit = 1
			continue
		}
		if !ok || len(m.Modules) == 0 {
			fmt.Fprintf(os.Stderr, "%s: no remote imports declared\n", dir)
			continue
		}
		lf, err := remote.Sync(dir, m, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
			exit = 1
			continue
		}
		// Print resolved versions in sorted order so output is
		// stable across runs.
		var paths []string
		for p := range lf.Modules {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		fmt.Fprintf(os.Stderr, "ok   %s (%d module%s)\n", dir, len(paths), plural(len(paths)))
		for _, p := range paths {
			e := lf.Modules[p]
			fmt.Fprintf(os.Stderr, "     %s %s %s\n", p, e.Version, shortSHA(e.Commit))
		}
	}
	return exit
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func shortSHA(s string) string {
	if len(s) < 12 {
		return s
	}
	return s[:12]
}

func runTest(args []string) int {
	if len(args) == 0 {
		args = []string{DefaultRulesDir}
	}
	exit := 0
	for _, dir := range args {
		report, err := runner.TestDir(context.Background(), dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
			exit = 1
			continue
		}
		if report.Failed() {
			fmt.Fprintf(os.Stderr, "FAIL %s (%d files):\n", dir, report.NumFiles)
			for _, f := range report.Failures {
				fmt.Fprintf(os.Stderr, "  %s\n", f)
			}
			exit = 1
		} else {
			fmt.Fprintf(os.Stderr, "ok   %s (%d files)\n", dir, report.NumFiles)
		}
	}
	return exit
}
