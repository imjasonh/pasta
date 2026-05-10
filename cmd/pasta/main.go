// Command pasta runs CUE-defined analyzers over source files.
//
// Usage:
//
//	pasta [-fix] [-skip <dirs>] <rule.cue> <source> [<source>...]
//	pasta test <rule-dir> [<rule-dir>...]            run rules on their testdata/
//
// A source argument ending in `/...` (or the literal `./...`) is
// expanded to every file under that directory whose extension maps
// to a registered language — Go-style "all packages below here".
// During expansion, directories named `.git`, `vendor`, or
// `node_modules` are skipped by default; pass `-skip` with a
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
	"github.com/imjasonh/pasta/internal/runner"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "test" {
		os.Exit(runTest(os.Args[2:]))
	}
	os.Exit(runFix(os.Args[1:]))
}

func runFix(args []string) int {
	fs := flag.NewFlagSet("pasta", flag.ExitOnError)
	fix := fs.Bool("fix", false, "apply suggested fixes by rewriting each source file in place")
	skip := fs.String("skip", "", "comma-separated directory basenames to skip during ./... expansion (in addition to defaults: .git, vendor, node_modules)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: pasta [-fix] [-skip <dirs>] <rule.cue> <source> [<source>...]")
		fmt.Fprintln(os.Stderr, "       pasta test <rule-dir> [<rule-dir>...]")
		return 2
	}
	rulePath := fs.Arg(0)
	rawSources := fs.Args()[1:]

	a, err := runner.LoadRule(rulePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load %s: %v\n", rulePath, err)
		return 1
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

	results, err := runner.RunGroup(context.Background(), specs, []*dsl.Analyzer{a}, *fix)
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
// data that almost no analyzer wants to lint, and walking them turns
// `pasta -fix rule.cue ./...` into an unintended whole-tree edit.
var defaultSkipDirs = map[string]bool{
	".git":         true,
	"vendor":       true,
	"node_modules": true,
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

func runTest(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: pasta test <rule-dir> [<rule-dir>...]")
		return 2
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
