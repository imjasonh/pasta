// Command pasta runs CUE-defined analyzers over source files.
//
// Usage:
//
//	pasta [-fix] <rule.cue> <source>      run a rule on a source file
//	pasta test <rule-dir> [<rule-dir>...] run rules on their testdata/
//
// The source file's extension determines the tree-sitter language; see
// pkg/lang for the registered set.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/runner"
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
	sources := fs.Args()[1:]

	a, err := runner.LoadRule(rulePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load %s: %v\n", rulePath, err)
		return 1
	}

	exit := 0
	for _, src := range sources {
		b, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", src, err)
			exit = 1
			continue
		}
		res, err := runner.RunFile(context.Background(), src, b, []*dsl.Analyzer{a}, *fix)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", src, err)
			exit = 1
			continue
		}
		for _, d := range res.Diagnostics {
			fmt.Fprintf(os.Stderr, "%s:%d: %s [%s]\n", src, d.Line(), d.Message, d.Rule)
		}
		if *fix {
			os.Stdout.Write(res.Fixed)
		}
	}
	return exit
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
