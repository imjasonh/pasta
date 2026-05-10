package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLatestSemverTag covers the policy decisions: prefers highest
// semver, accepts both "v1.2.3" and "1.2.3", skips prereleases, and
// returns ok=false when no candidate is recognizable.
func TestLatestSemverTag(t *testing.T) {
	cases := []struct {
		name    string
		tags    []string
		wantTag string
		wantOK  bool
	}{
		{"prefers highest", []string{"v1.0.0", "v1.5.2", "v1.2.3"}, "v1.5.2", true},
		{"crosses major", []string{"v1.9.0", "v2.0.0"}, "v2.0.0", true},
		{"accepts unprefixed", []string{"1.0.0", "1.2.0"}, "1.2.0", true},
		{"mixed prefixes", []string{"v1.0.0", "1.5.0"}, "1.5.0", true},
		{"skips prereleases", []string{"v1.0.0", "v2.0.0-rc1", "v2.0.0-beta"}, "v1.0.0", true},
		{"skips garbage", []string{"latest", "main", "release"}, "", false},
		{"empty input", nil, "", false},
		{"only prereleases", []string{"v1.0.0-rc1", "v2.0.0-beta"}, "", false},
	}
	for _, tc := range cases {
		got, ok := LatestSemverTag(tc.tags)
		if got != tc.wantTag || ok != tc.wantOK {
			t.Errorf("%s: got=(%q,%v) want=(%q,%v)", tc.name, got, ok, tc.wantTag, tc.wantOK)
		}
	}
}

// TestRewriteManifestVersionsPreservesFormatting is the contract
// `pasta bump` relies on: a surgical version replace must not
// disturb the surrounding text. Comments, alignment, key order all
// have to round-trip.
func TestRewriteManifestVersionsPreservesFormatting(t *testing.T) {
	dir := t.TempDir()
	src := `// project rules
imports: {
	// kept on v1 deliberately for now
	"github.com/alice/rules": "v1.0.0"
	"github.com/bob/lint":    "v0.5.2"
}
`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RewriteManifestVersions(dir, map[string]string{
		"github.com/alice/rules": "v1.4.0",
		"github.com/bob/lint":    "v0.6.0",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	want := `// project rules
imports: {
	// kept on v1 deliberately for now
	"github.com/alice/rules": "v1.4.0"
	"github.com/bob/lint":    "v0.6.0"
}
`
	if string(got) != want {
		t.Errorf("manifest mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestRewriteManifestVersionsErrorsOnMissingPath proves that asking
// to bump a module the manifest doesn't list is treated as a real
// error rather than a silent no-op. Catches stale callers and
// concurrent edits.
func TestRewriteManifestVersionsErrorsOnMissingPath(t *testing.T) {
	dir := t.TempDir()
	src := `imports: {"github.com/alice/rules": "v1.0.0"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	err := RewriteManifestVersions(dir, map[string]string{
		"github.com/nope/missing": "v1.0.0",
	})
	if err == nil {
		t.Fatal("expected error for module not in manifest")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention 'not found'", err.Error())
	}
}

// TestBumpEndToEnd exercises the full pipeline: list tags, pick
// latest, rewrite manifest, sync. Uses the package's existing fake
// fetcher (which knows how to "serve" prefab module contents and
// advertise a tag list) so no network is involved.
//
// One module gets bumped, one is already at the latest tag, and one
// has no semver tags — proving each branch of the per-module
// switch works.
func TestBumpEndToEnd(t *testing.T) {
	cacheDir := t.TempDir()
	f := &fakeFetcher{
		root: cacheDir,
		// Simple tag → fake commit map: "v1.4.0" → "v1.4.0sha".
		// Sync calls Fetch (which calls resolveFn) for new pins,
		// and FetchCommit for already-locked entries.
		resolveFn: func(path, ver string) string { return ver + "sha" + path },
		files:     map[string]map[string][]byte{},
		tags: map[string][]string{
			"github.com/alice/rules": {"v1.0.0", "v1.4.0", "v1.2.0"},
			"github.com/bob/lint":    {"v0.6.0", "v0.6.0-rc1"},
			"github.com/carol/exp":   {"main", "release", "nightly"},
		},
	}
	// Pre-stage module contents for every (path@commit) Sync might
	// reach for: Sync calls Fetch with the user-facing version, the
	// fake's resolveFn maps that to a commit, and we materialize a
	// minimal tree. Without this the post-bump sync errors with
	// "module not found".
	stage := func(path, ver string) {
		commit := f.resolveFn(path, ver)
		f.files[path+"@"+commit] = map[string][]byte{
			"placeholder.cue": []byte("package placeholder\n"),
		}
	}
	stage("github.com/alice/rules", "v1.0.0")
	stage("github.com/alice/rules", "v1.4.0")
	stage("github.com/bob/lint", "v0.6.0")
	stage("github.com/carol/exp", "main")

	dir := t.TempDir()
	manifest := `// project rules
imports: {
	"github.com/alice/rules": "v1.0.0"
	"github.com/bob/lint":    "v0.6.0"
	"github.com/carol/exp":   "main"
}
`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := Bump(dir, nil, f)
	if err != nil {
		t.Fatalf("Bump: %v", err)
	}

	// Build a lookup so order doesn't matter.
	got := map[string]BumpResult{}
	for _, r := range results {
		got[r.Module] = r
	}
	if r := got["github.com/alice/rules"]; r.NewVersion != "v1.4.0" || r.Skipped != "" {
		t.Errorf("alice/rules: %+v (want NewVersion=v1.4.0, Skipped=\"\")", r)
	}
	if r := got["github.com/bob/lint"]; r.Skipped != "already up to date" {
		t.Errorf("bob/lint: %+v (want Skipped=already up to date)", r)
	}
	if r := got["github.com/carol/exp"]; r.Skipped != "no semver tags" {
		t.Errorf("carol/exp: %+v (want Skipped=no semver tags)", r)
	}

	// Manifest should record the new pin only for alice; the others
	// are unchanged.
	out, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"github.com/alice/rules": "v1.4.0"`) {
		t.Errorf("manifest didn't get bumped pin for alice/rules:\n%s", out)
	}
	if !strings.Contains(string(out), `"github.com/bob/lint":    "v0.6.0"`) {
		t.Errorf("bob/lint pin (no bump) was disturbed:\n%s", out)
	}
	if !strings.Contains(string(out), `"github.com/carol/exp":   "main"`) {
		t.Errorf("carol/exp pin (no bump) was disturbed:\n%s", out)
	}
	// Comment from the original file should survive — the regex
	// rewriter is supposed to be surgical.
	if !strings.Contains(string(out), "// project rules") {
		t.Errorf("comment lost during bump:\n%s", out)
	}

	// Lockfile should now exist and pin alice@v1.4.0.
	lf, ok, err := LoadLockfile(dir)
	if err != nil || !ok {
		t.Fatalf("LoadLockfile: ok=%v err=%v", ok, err)
	}
	if lf.Modules["github.com/alice/rules"].Version != "v1.4.0" {
		t.Errorf("lockfile alice/rules version = %q, want v1.4.0",
			lf.Modules["github.com/alice/rules"].Version)
	}
}

// TestBumpRespectsFilter verifies that a non-empty filter narrows
// the bump to exactly those modules — important so a project with
// many imports can iterate one at a time.
func TestBumpRespectsFilter(t *testing.T) {
	cacheDir := t.TempDir()
	f := &fakeFetcher{
		root:      cacheDir,
		resolveFn: func(path, ver string) string { return ver + "sha" + path },
		files:     map[string]map[string][]byte{},
		tags: map[string][]string{
			"github.com/alice/rules": {"v1.0.0", "v1.4.0"},
			"github.com/bob/lint":    {"v0.5.0", "v0.9.0"},
		},
	}
	stage := func(path, ver string) {
		commit := f.resolveFn(path, ver)
		f.files[path+"@"+commit] = map[string][]byte{"x.cue": []byte("package x\n")}
	}
	stage("github.com/alice/rules", "v1.0.0")
	stage("github.com/alice/rules", "v1.4.0")
	stage("github.com/bob/lint", "v0.5.0")

	dir := t.TempDir()
	manifest := `imports: {
	"github.com/alice/rules": "v1.0.0"
	"github.com/bob/lint":    "v0.5.0"
}
`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := Bump(dir, map[string]bool{"github.com/alice/rules": true}, f)
	if err != nil {
		t.Fatalf("Bump: %v", err)
	}
	if len(results) != 1 || results[0].Module != "github.com/alice/rules" {
		t.Fatalf("filter ignored: results=%+v", results)
	}
	if results[0].NewVersion != "v1.4.0" {
		t.Errorf("alice/rules NewVersion = %q, want v1.4.0", results[0].NewVersion)
	}
	out, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	if !strings.Contains(string(out), `"github.com/bob/lint":    "v0.5.0"`) {
		t.Errorf("bob/lint pin was bumped despite filter excluding it:\n%s", out)
	}
}

// TestRewriteManifestVersionsTolerantOfWhitespace verifies the
// regex matches both single-line and oddly-spaced manifests, which
// is what makes it safe to advertise "rewrite preserves formatting"
// as a guarantee.
func TestRewriteManifestVersionsTolerantOfWhitespace(t *testing.T) {
	dir := t.TempDir()
	src := `imports : {  "github.com/x/y"  :   "v1.0.0"  }` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RewriteManifestVersions(dir, map[string]string{
		"github.com/x/y": "v2.0.0",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"v2.0.0"`) {
		t.Errorf("manifest didn't get the new version; got %q", got)
	}
	if strings.Contains(string(got), `"v1.0.0"`) {
		t.Errorf("manifest still has old version; got %q", got)
	}
}
