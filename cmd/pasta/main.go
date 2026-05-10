// Command pasta runs CUE-defined analyzers over source files.
//
// Usage:
//
//	pasta [-fix] <rule.cue> <source> [<source>...]   run a rule on one or more files
//	pasta test <rule-dir> [<rule-dir>...]            run rules on their testdata/
//
// A source argument ending in `/...` (or the literal `./...`) is
// expanded to every file under that directory whose extension maps
// to a registered language — Go-style "all packages below here".
//
// The source file's extension determines the tree-sitter language; see
// internal/lang for the registered set. When more than one source file
// is supplied (directly or via `./...` expansion) they are analyzed as
// a single group with a shared fact store, so cross-file analyses see
// facts from every file in the run.
//
// When `-fix` is set: a single literal source path writes fixed bytes
// to stdout (so it can be piped); two or more files (or any `./...`
// expansion) write fixed bytes back to each source file in place.
package main

import (
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
	fix := fs.Bool("fix", false, "apply suggested fixes and write the result to stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: pasta [-fix] <rule.cue> <source> [<source>...]")
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

	expanded, hadExpansion, err := expandSources(rawSources)
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
	// Single literal file with -fix writes to stdout (pipe-friendly,
	// historical behavior). Multiple files or any `./...` expansion
	// rewrites each source in place — there's no useful single-stream
	// representation for a group of fixed files.
	writeInPlace := *fix && (hadExpansion || len(specs) > 1)
	for _, res := range results {
		for _, d := range res.Diagnostics {
			fmt.Fprintf(os.Stderr, "%s:%d: %s [%s]\n", res.Path, d.Line(), d.Message, d.Rule)
		}
		if !*fix {
			continue
		}
		if writeInPlace {
			if err := os.WriteFile(res.Path, res.Fixed, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", res.Path, err)
				exit = 1
			}
			continue
		}
		os.Stdout.Write(res.Fixed)
	}
	return exit
}

// expandSources turns CLI source arguments into concrete file paths.
// An argument ending in `/...` (or the literal `./...` / `...`) is
// expanded to every file under that directory whose extension maps
// to a registered language; .golden files are excluded. Plain paths
// pass through unchanged. The returned bool reports whether ANY
// argument was an expansion — used to decide between in-place and
// stdout output for -fix.
func expandSources(args []string) ([]string, bool, error) {
	seen := map[string]bool{}
	var out []string
	hadExpansion := false
	for _, a := range args {
		if a == "..." || a == "./..." || strings.HasSuffix(a, "/...") {
			hadExpansion = true
			root := strings.TrimSuffix(a, "/...")
			if root == "" || a == "..." {
				root = "."
			}
			matches, err := walkSources(root)
			if err != nil {
				return nil, false, fmt.Errorf("expand %s: %w", a, err)
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
	return out, hadExpansion, nil
}

// walkSources walks root and returns every file with an extension
// pasta knows about (via lang.ByExt). .golden files are skipped.
func walkSources(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
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
