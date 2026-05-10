package runner_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imjasonh/pasta/internal/loader"
	"github.com/imjasonh/pasta/internal/remote"
	"github.com/imjasonh/pasta/internal/runner"
)

// fakeFetcher serves staged module contents from on-disk dirs keyed
// by "<modulePath>@<commit>". Mirrors the real GitFetcher's
// "target exists → don't rewrite" semantics so behaviour is faithful
// to production. Same shape as the loader's test fake; duplicated
// here because _test.go types don't cross packages.
type fakeFetcher struct {
	dirs map[string]string // "<modulePath>@<commit>" -> on-disk dir
}

func (f *fakeFetcher) Fetch(path, ver string) (string, string, error) {
	// e2e tests always go through the lockfile path, so Fetch is
	// never expected. Fail loudly if it is.
	return "", "", os.ErrNotExist
}

func (f *fakeFetcher) FetchCommit(path, commit string) (string, error) {
	dir, ok := f.dirs[path+"@"+commit]
	if !ok {
		return "", os.ErrNotExist
	}
	return dir, nil
}

// TestRemoteAnalyzerFiresEndToEnd is the load → engine → diagnostics
// e2e for remote rule imports: it stages a fake remote module
// containing a real working analyzer, points the loader at a fake
// fetcher, then drives runner.LoadRules + runner.RunGroup over a
// real Go source file and verifies the analyzer actually fires.
//
// Catches the regression where the loader returns analyzers but
// something downstream (engine, factstore, effect) silently drops
// remote-tagged rules. The unit tests in internal/loader/ stop at
// "Analyzers returned"; this one proves the wired pipeline runs.
func TestRemoteAnalyzerFiresEndToEnd(t *testing.T) {
	// 1. Stage a remote module containing a complete, working
	//    analyzer. We match `package_clause` (one per Go file,
	//    unambiguous) so the diagnostic count is deterministic.
	remoteDir := t.TempDir()
	remoteSrc := `package rules

import "github.com/imjasonh/pasta/schema"

flag_pkg: schema.#Analyzer & {
	name:    "flag_pkg"
	version: "0.1.0"
	doc:     "remote analyzer that flags Go package clauses"
	facts: {}
	rules: r: {
		name: "r"
		doc:  "flag the package clause"
		languages: ["go"]
		requires: []
		provides: []
		match: {node: "package_clause"}
		diagnose: {
			message:  "remote-analyzer-fired"
			severity: "warning"
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(remoteDir, "rules.cue"), []byte(remoteSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Point the loader's fetcher at a fake serving that dir.
	commit := strings.Repeat("e", 40)
	cleanup := loader.SetFetcherForTesting(&fakeFetcher{
		dirs: map[string]string{
			"example.com/alice/rules@" + commit: remoteDir,
		},
	})
	t.Cleanup(cleanup)

	// 3. Build a project layout: a .pasta/ with manifest + lockfile,
	//    and a Go source file outside .pasta/.
	project := t.TempDir()
	pastaDir := filepath.Join(project, ".pasta")
	if err := os.MkdirAll(pastaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `imports: {"example.com/alice/rules": "v1.0.0"}` + "\n"
	if err := os.WriteFile(filepath.Join(pastaDir, remote.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := remote.HashTree(remoteDir)
	if err != nil {
		t.Fatal(err)
	}
	lf := struct {
		Modules map[string]remote.LockedModule `json:"modules"`
	}{Modules: map[string]remote.LockedModule{
		"example.com/alice/rules": {Version: "v1.0.0", Commit: commit, Hash: hash},
	}}
	lfBytes, _ := json.MarshalIndent(lf, "", "  ")
	if err := os.WriteFile(filepath.Join(pastaDir, remote.LockFile), lfBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Load the rules through the runner — same path the CLI uses.
	analyzers, err := runner.LoadRules(pastaDir)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(analyzers) != 1 {
		t.Fatalf("expected 1 analyzer auto-enrolled from remote; got %d", len(analyzers))
	}
	if got := analyzers[0].Name; got != "flag_pkg" {
		t.Fatalf("Name = %q, want flag_pkg", got)
	}
	if got := analyzers[0].Source; got != "example.com/alice/rules" {
		t.Errorf("Source = %q, want example.com/alice/rules", got)
	}

	// 5. Run the loaded analyzer over a real Go source file.
	src := []byte("package main\n\nfunc main() {}\n")
	srcPath := filepath.Join(project, "main.go")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := runner.RunGroup(context.Background(),
		[]runner.FileSpec{{Path: srcPath, Src: src}},
		analyzers, false)
	if err != nil {
		t.Fatalf("RunGroup: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// 6. The analyzer's match: package_clause. Source has exactly
	//    one — assert one diagnostic with the expected message on
	//    line 1.
	diags := results[0].Diagnostics
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Message, "remote-analyzer-fired") {
		t.Errorf("diagnostic message = %q, want it to contain remote-analyzer-fired", diags[0].Message)
	}
	if diags[0].Line() != 1 {
		t.Errorf("diagnostic on line %d, want 1", diags[0].Line())
	}
	if diags[0].Rule != "r" {
		t.Errorf("diagnostic rule = %q, want r", diags[0].Rule)
	}
}
