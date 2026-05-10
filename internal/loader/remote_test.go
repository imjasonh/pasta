package loader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imjasonh/pasta/internal/remote"
)

// fakeFetcher serves pre-staged module contents from an on-disk dir
// keyed by (modulePath, commit). Mirrors the fake in
// internal/remote/remote_test.go but kept separate so we don't have to
// export the test type.
type fakeFetcher struct {
	dirs map[string]string // "<modulePath>@<commit>" -> dir
}

func (f *fakeFetcher) Fetch(path, ver string) (string, string, error) {
	// Tests always go through the lockfile path here, so Fetch is
	// not exercised. Keep an honest stub so misuse fails loudly.
	return "", "", os.ErrNotExist
}

func (f *fakeFetcher) FetchCommit(path, commit string) (string, error) {
	dir, ok := f.dirs[path+"@"+commit]
	if !ok {
		return "", os.ErrNotExist
	}
	return dir, nil
}

// TestLoadDirWithRemoteImport stages a fake remote module on disk,
// writes a pasta.cue + pasta.lock pointing at it, and verifies the
// loader vendors the module into the overlay so a rule that imports
// it loads cleanly.
func TestLoadDirWithRemoteImport(t *testing.T) {
	// Stage the "remote" module: a CUE package providing one helper
	// value that the user rule will consume.
	remoteDir := t.TempDir()
	remoteSrc := `package rules

import "github.com/imjasonh/pasta/schema"

// Recipe is a reusable analyzer skeleton — the user rule unifies
// with it and overrides the rule-specific bits.
Recipe: schema.#Analyzer & {
	version: "0.1.0"
	facts: {}
}
`
	if err := os.WriteFile(filepath.Join(remoteDir, "rules.cue"), []byte(remoteSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	commit := strings.Repeat("a", 40)
	f := &fakeFetcher{dirs: map[string]string{
		"example.com/alice/rules@" + commit: remoteDir,
	}}

	// Swap the loader's fetcher constructor for the duration of the
	// test. The original is restored via t.Cleanup.
	prev := newDefaultFetcher
	t.Cleanup(func() { newDefaultFetcher = prev })
	newDefaultFetcher = func() (remote.Fetcher, error) { return f, nil }

	// User rule directory: declares the import, provides a manifest,
	// and writes a lockfile that pins the same fake commit.
	userDir := t.TempDir()
	manifest := `imports: {"example.com/alice/rules": "v1.0.0"}` + "\n"
	if err := os.WriteFile(filepath.Join(userDir, remote.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	// Compute the hash the loader will demand on every load; the
	// lockfile commits to it so a tampered cache fails closed.
	hash, err := remote.HashTree(remoteDir)
	if err != nil {
		t.Fatal(err)
	}
	lf := remote.Lockfile{Modules: map[string]remote.LockedModule{
		"example.com/alice/rules": {Version: "v1.0.0", Commit: commit, Hash: hash},
	}}
	lfBytes, _ := json.MarshalIndent(struct {
		Modules map[string]remote.LockedModule `json:"modules"`
	}{lf.Modules}, "", "  ")
	if err := os.WriteFile(filepath.Join(userDir, remote.LockFile), lfBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	ruleSrc := `package myrule

import (
	"github.com/imjasonh/pasta/schema"
	"example.com/alice/rules"
)

myrule: rules.Recipe & schema.#Analyzer & {
	name: "myrule"
	rules: r: {
		name: "r"
		doc: "uses a remote helper"
		languages: ["go"]
		requires: []
		provides: []
		match: {node: "identifier"}
		diagnose: {message: "x", severity: "hint"}
	}
}
`
	if err := os.WriteFile(filepath.Join(userDir, "myrule.cue"), []byte(ruleSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := LoadDir(userDir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(res.Analyzers) != 1 {
		t.Fatalf("expected 1 analyzer, got %d", len(res.Analyzers))
	}
	a := res.Analyzers[0]
	if a.Name != "myrule" {
		t.Errorf("Name = %q, want myrule", a.Name)
	}
	if a.Version != "0.1.0" {
		t.Errorf("Version = %q, want 0.1.0 (inherited from remote Recipe)", a.Version)
	}
}

// TestLoadDirRejectsManifestWithoutLockfile verifies that declaring
// remote imports without first running `pasta sync` produces a clear
// error rather than silently ignoring the manifest.
func TestLoadDirRejectsManifestWithoutLockfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, remote.ManifestFile),
		[]byte(`imports: {"example.com/alice/rules": "v1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "r.cue"), []byte(`package r`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("expected error for missing lockfile")
	}
	if !strings.Contains(err.Error(), "pasta sync") {
		t.Errorf("expected error to suggest `pasta sync`; got %v", err)
	}
}

// stageRemoteModule writes a single-file CUE package on disk and
// returns its directory. Tests pass that directory through the fake
// fetcher to make it look like a fetched remote module.
func stageRemoteModule(t *testing.T, pkg, file, src string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, file), []byte("package "+pkg+"\n\n"+src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeManifestAndLock writes a .pasta-style manifest + matching
// hash-locked lockfile into dir. Returns nothing — the caller's only
// observable is that LoadDir(dir) now resolves the listed module.
func writeManifestAndLock(t *testing.T, dir, modPath, ver, commit, modDir string) {
	t.Helper()
	manifest := `imports: {"` + modPath + `": "` + ver + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, remote.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := remote.HashTree(modDir)
	if err != nil {
		t.Fatal(err)
	}
	lf := struct {
		Modules map[string]remote.LockedModule `json:"modules"`
	}{
		Modules: map[string]remote.LockedModule{
			modPath: {Version: ver, Commit: commit, Hash: hash},
		},
	}
	b, _ := json.MarshalIndent(lf, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, remote.LockFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadDirAutoEnrollsRemoteAnalyzers verifies that listing a
// module in pasta.cue is enough to enroll its analyzers — no per-rule
// stub in .pasta/ required. This is the "publish + use directly"
// flow: a project ships rules, downstream consumers list it and run.
func TestLoadDirAutoEnrollsRemoteAnalyzers(t *testing.T) {
	remoteDir := stageRemoteModule(t, "rules", "rules.cue", `import "github.com/imjasonh/pasta/schema"

xss: schema.#Analyzer & {
	name: "xss"
	version: "0.1.0"
	facts: {}
	rules: r: {
		name: "r"
		doc: "x"
		languages: ["go"]
		requires: []
		provides: []
		match: {node: "identifier"}
		diagnose: {message: "y", severity: "hint"}
	}
}
`)
	commit := strings.Repeat("e", 40)
	f := &fakeFetcher{dirs: map[string]string{
		"example.com/alice/rules@" + commit: remoteDir,
	}}
	prev := newDefaultFetcher
	t.Cleanup(func() { newDefaultFetcher = prev })
	newDefaultFetcher = func() (remote.Fetcher, error) { return f, nil }

	// User dir contains ONLY pasta.cue + pasta.lock. No local rule
	// files. The expectation is that LoadDir still returns the
	// remote analyzer.
	userDir := t.TempDir()
	writeManifestAndLock(t, userDir, "example.com/alice/rules", "v1.0.0", commit, remoteDir)
	res, err := LoadDir(userDir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(res.Analyzers) != 1 {
		t.Fatalf("expected 1 auto-enrolled analyzer, got %d", len(res.Analyzers))
	}
	a := res.Analyzers[0]
	if a.Name != "xss" {
		t.Errorf("Name = %q, want xss", a.Name)
	}
	if a.Source != "example.com/alice/rules" {
		t.Errorf("Source = %q, want example.com/alice/rules", a.Source)
	}
}

// TestLoadDirLocalOverridesRemote verifies the collision policy:
// when a local analyzer has the same name as a remote one, the local
// version wins (so projects can patch a remote rule without forking
// the module).
func TestLoadDirLocalOverridesRemote(t *testing.T) {
	remoteDir := stageRemoteModule(t, "rules", "rules.cue", `import "github.com/imjasonh/pasta/schema"

shared: schema.#Analyzer & {
	name: "shared"
	version: "0.1.0"
	doc: "remote version"
	facts: {}
	rules: r: {
		name: "r"
		doc: "x"
		languages: ["go"]
		requires: []
		provides: []
		match: {node: "identifier"}
		diagnose: {message: "y", severity: "hint"}
	}
}
`)
	commit := strings.Repeat("f", 40)
	f := &fakeFetcher{dirs: map[string]string{
		"example.com/alice/rules@" + commit: remoteDir,
	}}
	prev := newDefaultFetcher
	t.Cleanup(func() { newDefaultFetcher = prev })
	newDefaultFetcher = func() (remote.Fetcher, error) { return f, nil }

	userDir := t.TempDir()
	writeManifestAndLock(t, userDir, "example.com/alice/rules", "v1.0.0", commit, remoteDir)
	if err := os.WriteFile(filepath.Join(userDir, "shared.cue"), []byte(`package myrule

import "github.com/imjasonh/pasta/schema"

shared: schema.#Analyzer & {
	name: "shared"
	version: "0.2.0"
	doc: "local override"
	facts: {}
	rules: r: {
		name: "r"
		doc: "x"
		languages: ["go"]
		requires: []
		provides: []
		match: {node: "identifier"}
		diagnose: {message: "y", severity: "hint"}
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := LoadDir(userDir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(res.Analyzers) != 1 {
		t.Fatalf("expected exactly 1 analyzer (local wins), got %d", len(res.Analyzers))
	}
	if got := res.Analyzers[0].Doc; got != "local override" {
		t.Errorf("Doc = %q, want local override (remote should be suppressed)", got)
	}
}

// TestLoadDirRejectsRemoteRemoteCollision verifies that if two
// imported modules export an analyzer with the same name, LoadDir
// errors rather than silently picking one.
func TestLoadDirRejectsRemoteRemoteCollision(t *testing.T) {
	mod1 := stageRemoteModule(t, "rules", "rules.cue", `import "github.com/imjasonh/pasta/schema"

dup: schema.#Analyzer & {
	name: "dup"
	version: "0.1.0"
	facts: {}
	rules: r: {name: "r", doc: "x", languages: ["go"], requires: [], provides: [], match: {node: "identifier"}, diagnose: {message: "y", severity: "hint"}}
}
`)
	mod2 := stageRemoteModule(t, "rules", "rules.cue", `import "github.com/imjasonh/pasta/schema"

dup: schema.#Analyzer & {
	name: "dup"
	version: "0.2.0"
	facts: {}
	rules: r: {name: "r", doc: "x", languages: ["go"], requires: [], provides: [], match: {node: "identifier"}, diagnose: {message: "y", severity: "hint"}}
}
`)
	c1 := strings.Repeat("1", 40)
	c2 := strings.Repeat("2", 40)
	f := &fakeFetcher{dirs: map[string]string{
		"example.com/alice/rules@" + c1: mod1,
		"example.com/bob/rules@" + c2:   mod2,
	}}
	prev := newDefaultFetcher
	t.Cleanup(func() { newDefaultFetcher = prev })
	newDefaultFetcher = func() (remote.Fetcher, error) { return f, nil }

	userDir := t.TempDir()
	manifest := `imports: {
	"example.com/alice/rules": "v1"
	"example.com/bob/rules": "v1"
}
`
	if err := os.WriteFile(filepath.Join(userDir, remote.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	h1, _ := remote.HashTree(mod1)
	h2, _ := remote.HashTree(mod2)
	lf := struct {
		Modules map[string]remote.LockedModule `json:"modules"`
	}{Modules: map[string]remote.LockedModule{
		"example.com/alice/rules": {Version: "v1", Commit: c1, Hash: h1},
		"example.com/bob/rules":   {Version: "v1", Commit: c2, Hash: h2},
	}}
	b, _ := json.MarshalIndent(lf, "", "  ")
	if err := os.WriteFile(filepath.Join(userDir, remote.LockFile), b, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadDir(userDir)
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "collision") {
		t.Errorf("expected error to mention collision; got %v", err)
	}
}

// TestLoadDirIgnoresPastaCueAsRule verifies that pasta.cue is not
// treated as a rule file even when it contains valid CUE that could
// otherwise look like an analyzer.
func TestLoadDirIgnoresPastaCueAsRule(t *testing.T) {
	dir := t.TempDir()
	// pasta.cue with an empty imports block — would parse fine but
	// must not be loaded by the rule pipeline.
	if err := os.WriteFile(filepath.Join(dir, remote.ManifestFile),
		[]byte("imports: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ruleSrc := `package r

import "github.com/imjasonh/pasta/schema"

r: schema.#Analyzer & {
	name: "r"
	version: "0.1.0"
	facts: {}
	rules: only: {
		name: "only"
		doc: "x"
		languages: ["go"]
		requires: []
		provides: []
		match: {node: "identifier"}
		diagnose: {message: "y", severity: "hint"}
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "r.cue"), []byte(ruleSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(res.Analyzers) != 1 {
		t.Fatalf("expected 1 analyzer, got %d", len(res.Analyzers))
	}
}
