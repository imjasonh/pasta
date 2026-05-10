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
	"github.com/imjasonh/pasta/internal/loader"
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
		case "bump":
			os.Exit(runBump(os.Args[2:]))
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

	analyzers, cfg, rawSources, code := selectRules(*rulesDir, fs.Args())
	if code != 0 {
		return code
	}
	if len(rawSources) == 0 {
		// No sources given: default to "./..." so `pasta` from a
		// project root with a .pasta/ dir Just Works.
		rawSources = []string{"./..."}
	}

	expanded, err := expandSources(rawSources, parseSkipDirs(*skip, cfg))
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
// Returns the loaded analyzers, the project config (nil for the
// single-file form or when no config.cue is present), the remaining
// positional args to be treated as sources, and a process exit code
// (0 = ok). Errors are printed to stderr.
func selectRules(rulesDirFlag string, positional []string) ([]*dsl.Analyzer, *loader.Config, []string, int) {
	// Single-rule shortcut: first positional is an existing .cue
	// file. -rules wins if explicitly set, so users who want to mix
	// can still force directory mode.
	if rulesDirFlag == "" && len(positional) > 0 && strings.HasSuffix(positional[0], ".cue") {
		if info, err := os.Stat(positional[0]); err == nil && info.Mode().IsRegular() {
			a, err := runner.LoadRule(positional[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "load %s: %v\n", positional[0], err)
				return nil, nil, nil, 1
			}
			return []*dsl.Analyzer{a}, nil, positional[1:], 0
		}
	}

	dir := rulesDirFlag
	if dir == "" {
		dir = DefaultRulesDir
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		if rulesDirFlag != "" {
			// os.Stat already includes the path in its error string
			// ("stat /tmp/foo: no such file..."), so don't double up.
			if err != nil {
				fmt.Fprintf(os.Stderr, "rules directory: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "rules directory %q is not a directory\n", dir)
			}
		} else {
			fmt.Fprintf(os.Stderr, "no rules to run: pass a .cue rule file or create a ./%s/ directory (or use -rules <dir>)\n", DefaultRulesDir)
		}
		return nil, nil, nil, 2
	}
	p, err := runner.LoadProject(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load %s: %v\n", dir, err)
		return nil, nil, nil, 1
	}
	return p.Analyzers, p.Config, positional, 0
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

// parseSkipDirs returns the union of defaultSkipDirs, any `skip` list
// the project config declared, and the comma-separated user-supplied
// list. Empty entries are ignored.
func parseSkipDirs(extra string, cfg *loader.Config) map[string]bool {
	out := make(map[string]bool, len(defaultSkipDirs)+8)
	for k := range defaultSkipDirs {
		out[k] = true
	}
	if cfg != nil {
		for _, s := range cfg.Skip {
			if s = strings.TrimSpace(s); s != "" {
				out[s] = true
			}
		}
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
// As of implicit sync, plain `pasta` runs already do this on the fly
// when the lockfile is missing or stale, so `pasta sync` is no longer
// required for day-to-day use. It survives for two reasons:
//   - Explicit refresh: re-resolve a moving ref (branch / tag) to its
//     current commit even when the lockfile already has an entry.
//   - CI gating via --check: report drift without writing files, so
//     a CI job can fail the build when a contributor edited the
//     manifest without committing the regenerated lockfile.
func runSync(args []string) int {
	check := false
	rest := args[:0]
	for _, a := range args {
		if a == "--check" || a == "-check" {
			check = true
			continue
		}
		rest = append(rest, a)
	}
	args = rest
	defaulted := false
	if len(args) == 0 {
		args = []string{DefaultRulesDir}
		defaulted = true
	}
	cacheDir, err := remote.DefaultCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	f := &remote.GitFetcher{CacheDir: cacheDir}
	exit := 0
	for _, dir := range args {
		// Distinguish "no rule directory" from "rule directory with
		// no manifest". The first is a misconfiguration when
		// explicit; a benign no-op when defaulted (so plain
		// `pasta sync` from any project doesn't fail).
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			if defaulted {
				fmt.Fprintf(os.Stderr, "nothing to sync (./%s/ not present)\n", DefaultRulesDir)
				continue
			}
			fmt.Fprintf(os.Stderr, "%s: rule directory not found\n", dir)
			exit = 2
			continue
		}
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
		if check {
			// --check is non-destructive: load the lockfile and
			// compare to the manifest. Exit non-zero on any drift,
			// don't touch the filesystem.
			lf, lfOk, lerr := remote.LoadLockfile(dir)
			if lerr != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, lerr)
				exit = 1
				continue
			}
			if !lfOk {
				fmt.Fprintf(os.Stderr, "%s: lockfile missing; run `pasta sync %s`\n", dir, dir)
				exit = 1
				continue
			}
			if inSync, reason := remote.IsInSync(m, lf); !inSync {
				fmt.Fprintf(os.Stderr, "%s: out of sync: %s; run `pasta sync %s`\n", dir, reason, dir)
				exit = 1
				continue
			}
			fmt.Fprintf(os.Stderr, "ok   %s (lockfile up to date)\n", dir)
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

// runBump updates each module's version pin in pasta.cue to the
// highest semver tag the upstream advertises, then runs sync to
// refresh the lockfile. Modules pinned to a ref with no semver
// tags (a branch name, a date-tagged release, a full SHA) are
// reported as skipped — those already have well-defined update
// semantics that don't need a "bump" step.
//
// Argument shape: positional args are either rule directories (when
// they exist on disk) or module paths to narrow the bump within
// ./.pasta/. Plain `pasta bump` bumps every module in ./.pasta/.
func runBump(args []string) int {
	// Split args into rule dirs vs. module-path filters. An
	// existing directory becomes a dir; everything else is treated
	// as a module-path filter.
	var dirs []string
	var modFilter map[string]bool
	for _, a := range args {
		if info, err := os.Stat(a); err == nil && info.IsDir() {
			dirs = append(dirs, a)
			continue
		}
		if modFilter == nil {
			modFilter = map[string]bool{}
		}
		modFilter[a] = true
	}
	if len(dirs) == 0 {
		dirs = []string{DefaultRulesDir}
	}
	cacheDir, err := remote.DefaultCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	f := &remote.GitFetcher{CacheDir: cacheDir}
	exit := 0
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "%s: rule directory not found\n", dir)
			exit = 2
			continue
		}
		results, err := remote.Bump(dir, modFilter, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
			exit = 1
			// Still print partial results below so the user knows
			// what did get bumped before the failure.
		}
		if len(results) == 0 && err == nil {
			fmt.Fprintf(os.Stderr, "%s: no remote imports declared\n", dir)
			continue
		}
		for _, r := range results {
			switch {
			case r.Skipped == "" && r.NewVersion != "":
				fmt.Fprintf(os.Stderr, "bump %s %s -> %s\n", r.Module, r.OldVersion, r.NewVersion)
			case r.Skipped == "already up to date":
				fmt.Fprintf(os.Stderr, "ok   %s already at %s\n", r.Module, r.OldVersion)
			default:
				fmt.Fprintf(os.Stderr, "skip %s (%s)\n", r.Module, r.Skipped)
			}
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
