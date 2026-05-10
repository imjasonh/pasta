package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestMissing(t *testing.T) {
	dir := t.TempDir()
	m, ok, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false for missing manifest")
	}
	if m != nil {
		t.Errorf("expected nil manifest; got %+v", m)
	}
}

func TestLoadManifestParse(t *testing.T) {
	dir := t.TempDir()
	src := `imports: {
	"github.com/alice/lint-rules": "v1.2.3"
	"gitlab.com/bob/checks":       "v0.5.0"
}
`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	m, ok, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got := m.Modules["github.com/alice/lint-rules"]; got != "v1.2.3" {
		t.Errorf("alice version = %q, want v1.2.3", got)
	}
	if got := m.Modules["gitlab.com/bob/checks"]; got != "v0.5.0" {
		t.Errorf("bob version = %q, want v0.5.0", got)
	}
}

func TestLoadManifestEmptyImports(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`// no imports yet`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, ok, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if !ok || m == nil {
		t.Fatalf("ok=%v m=%v", ok, m)
	}
	if len(m.Modules) != 0 {
		t.Errorf("expected zero modules, got %v", m.Modules)
	}
}

func TestLoadManifestRejectsBadPath(t *testing.T) {
	cases := map[string]string{
		"empty":      `imports: {"": "v1"}`,
		"absolute":   `imports: {"/etc/passwd": "v1"}`,
		"dotdot":     `imports: {"foo/../bar": "v1"}`,
		"no-slash":   `imports: {"justaname": "v1"}`,
		"whitespace": `imports: {"github.com/x y": "v1"}`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(src+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, _, err := LoadManifest(dir)
			if err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestLockfileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := &Lockfile{
		Modules: map[string]LockedModule{
			"github.com/alice/lint-rules": {Version: "v1.2.3", Commit: strings.Repeat("a", 40), Hash: "sha256:deadbeef"},
		},
	}
	if err := WriteLockfile(dir, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, ok, err := LoadLockfile(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true after write")
	}
	if len(out.Modules) != 1 {
		t.Errorf("round-trip mismatch: %+v", out)
	}
	got := out.Modules["github.com/alice/lint-rules"]
	if got.Version != "v1.2.3" || got.Commit != strings.Repeat("a", 40) || got.Hash != "sha256:deadbeef" {
		t.Errorf("round-trip: %+v", got)
	}
}

func TestLoadLockfileMissing(t *testing.T) {
	_, ok, err := LoadLockfile(t.TempDir())
	if err != nil {
		t.Fatalf("LoadLockfile: %v", err)
	}
	if ok {
		t.Error("expected ok=false")
	}
}

func TestHashTreeStable(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	for _, sub := range []string{"a/b.cue", "c.cue"} {
		full1 := filepath.Join(d1, sub)
		full2 := filepath.Join(d2, sub)
		if err := os.MkdirAll(filepath.Dir(full1), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(full2), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full1, []byte("hello "+sub), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full2, []byte("hello "+sub), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h1, err := HashTree(d1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashTree(d2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("hashes differ for identical trees: %s vs %s", h1, h2)
	}
	// Modify one file: hashes must diverge.
	if err := os.WriteFile(filepath.Join(d2, "c.cue"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	h2b, err := HashTree(d2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2b {
		t.Errorf("hash didn't change after content edit")
	}
}

// fakeFetcher is the in-memory test stand-in for GitFetcher: pretends
// to clone modules into a per-test temp dir without ever going to the
// network. The contents map keys are "<modulePath>@<version-or-commit>"
// pointing at a directory of files to "serve".
type fakeFetcher struct {
	root      string                        // where fake modules get materialized
	resolveFn func(path, ver string) string // tag/branch → commit
	files     map[string]map[string][]byte  // "<path>@<commit>" → relpath → content
	calls     []string
}

func (f *fakeFetcher) Fetch(path, ver string) (string, string, error) {
	commit := f.resolveFn(path, ver)
	f.calls = append(f.calls, "Fetch:"+path+"@"+ver+"->"+commit)
	dir, err := f.FetchCommit(path, commit)
	return dir, commit, err
}

func (f *fakeFetcher) FetchCommit(path, commit string) (string, error) {
	f.calls = append(f.calls, "FetchCommit:"+path+"@"+commit)
	target := filepath.Join(f.root, filepath.FromSlash(path)+"@"+commit)
	// Mirror the real fetcher: an existing target dir is the cache
	// hit — return it without rewriting. This is what makes
	// tamper-detection tests realistic; without it the fake would
	// helpfully un-tamper any cached file every time it's called.
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	files, ok := f.files[path+"@"+commit]
	if !ok {
		return "", os.ErrNotExist
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	for rel, content := range files {
		full := filepath.Join(target, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			return "", err
		}
	}
	return target, nil
}

func TestSyncWritesLockfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`imports: {"github.com/alice/rules": "v1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeFetcher{
		root:      t.TempDir(),
		resolveFn: func(_, _ string) string { return strings.Repeat("a", 40) },
		files: map[string]map[string][]byte{
			"github.com/alice/rules@" + strings.Repeat("a", 40): {
				"sample/sample.cue": []byte("package sample\n"),
			},
		},
	}
	m, _, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	lf, err := Sync(dir, m, f)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	got := lf.Modules["github.com/alice/rules"]
	if got.Version != "v1.0.0" || got.Commit != strings.Repeat("a", 40) {
		t.Errorf("locked entry = %+v", got)
	}
	if got.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if _, err := os.Stat(filepath.Join(dir, LockFile)); err != nil {
		t.Errorf("pasta.lock not written: %v", err)
	}
}

func TestSyncRejectsTransitiveImports(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`imports: {"github.com/alice/rules": "v1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	f := &fakeFetcher{
		root:      t.TempDir(),
		resolveFn: func(_, _ string) string { return commit },
		files: map[string]map[string][]byte{
			"github.com/alice/rules@" + commit: {
				"pasta.cue": []byte(`imports: {"github.com/charlie/transitive": "v1"}` + "\n"),
				"r.cue":     []byte("package r\n"),
			},
		},
	}
	m, _, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(dir, m, f); err == nil {
		t.Fatal("expected error for transitive imports")
	} else if !strings.Contains(err.Error(), "transitive") {
		t.Errorf("error doesn't mention transitive: %v", err)
	}
}

func TestSyncReusesCachedEntry(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`imports: {"github.com/alice/rules": "v1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("b", 40)
	f := &fakeFetcher{
		root:      t.TempDir(),
		resolveFn: func(_, _ string) string { return commit },
		files: map[string]map[string][]byte{
			"github.com/alice/rules@" + commit: {"r.cue": []byte("package r\n")},
		},
	}
	m, _, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(dir, m, f); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	prevCalls := len(f.calls)
	// Second sync at the same version should hit the cache
	// (FetchCommit only — no re-resolve via Fetch).
	if _, err := Sync(dir, m, f); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	for _, c := range f.calls[prevCalls:] {
		if strings.HasPrefix(c, "Fetch:") {
			t.Errorf("expected no Fetch call on reused entry; got %s", c)
		}
	}
}

func TestSyncRejectsHashMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`imports: {"github.com/alice/rules": "v1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("c", 40)
	f := &fakeFetcher{
		root:      t.TempDir(),
		resolveFn: func(_, _ string) string { return commit },
		files: map[string]map[string][]byte{
			"github.com/alice/rules@" + commit: {"r.cue": []byte("package r\n")},
		},
	}
	m, _, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(dir, m, f); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	// Tamper with the cached file. The second Sync should refuse
	// to reuse the stale entry and surface a clear error rather
	// than silently overwriting it (which could mask an attack).
	cached := filepath.Join(f.root, "github.com/alice/rules@"+commit, "r.cue")
	if err := os.WriteFile(cached, []byte("package r // tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Sync(dir, m, f)
	if err == nil {
		t.Fatal("expected hash-mismatch error on tampered cache")
	}
	if !strings.Contains(err.Error(), "hash") {
		t.Errorf("expected error to mention hash; got %v", err)
	}
}

func TestVendorDirsRejectsHashMismatch(t *testing.T) {
	served := t.TempDir()
	if err := os.WriteFile(filepath.Join(served, "r.cue"), []byte("package r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("d", 40)
	f := &fakeFetcher{
		root:      t.TempDir(),
		resolveFn: func(_, _ string) string { return commit },
		files:     map[string]map[string][]byte{"github.com/alice/rules@" + commit: {"r.cue": []byte("package r\n")}},
	}
	m := &Manifest{Modules: map[string]string{"github.com/alice/rules": "v1.0.0"}}
	lf := &Lockfile{Modules: map[string]LockedModule{
		"github.com/alice/rules": {Version: "v1.0.0", Commit: commit, Hash: "sha256:wrong"},
	}}
	_, err := VendorDirs(m, lf, f)
	if err == nil {
		t.Fatal("expected hash-mismatch error")
	}
	if !strings.Contains(err.Error(), "hash") {
		t.Errorf("expected error to mention hash; got %v", err)
	}
}

func TestVendorDirsManifestDrift(t *testing.T) {
	m := &Manifest{Modules: map[string]string{"github.com/alice/rules": "v2.0.0"}}
	lf := &Lockfile{Modules: map[string]LockedModule{
		"github.com/alice/rules": {Version: "v1.0.0", Commit: strings.Repeat("a", 40)},
	}}
	_, err := VendorDirs(m, lf, &fakeFetcher{root: t.TempDir()})
	if err == nil {
		t.Fatal("expected drift error")
	}
	if !strings.Contains(err.Error(), "pasta sync") {
		t.Errorf("expected hint to run pasta sync; got %v", err)
	}
}

func TestVendorDirsMissingFromLock(t *testing.T) {
	m := &Manifest{Modules: map[string]string{"github.com/alice/rules": "v1"}}
	lf := &Lockfile{Modules: map[string]LockedModule{}}
	_, err := VendorDirs(m, lf, &fakeFetcher{root: t.TempDir()})
	if err == nil {
		t.Fatal("expected error for module missing from lockfile")
	}
}

// TestIsInSync covers the four shapes the loader cares about: nil
// lockfile, missing entry, version drift, and a stale entry the
// manifest no longer asks for. A clean match returns (true, "").
func TestIsInSync(t *testing.T) {
	m := &Manifest{Modules: map[string]string{"a": "v1", "b": "v2"}}
	clean := &Lockfile{Modules: map[string]LockedModule{
		"a": {Version: "v1", Commit: "deadbeef"},
		"b": {Version: "v2", Commit: "cafef00d"},
	}}

	cases := []struct {
		name string
		lf   *Lockfile
		want bool
	}{
		{"clean", clean, true},
		{"nil lockfile", nil, false},
		{"missing entry", &Lockfile{Modules: map[string]LockedModule{
			"a": {Version: "v1", Commit: "deadbeef"},
		}}, false},
		{"version drift", &Lockfile{Modules: map[string]LockedModule{
			"a": {Version: "v1", Commit: "deadbeef"},
			"b": {Version: "v3", Commit: "cafef00d"},
		}}, false},
		{"extra entry", &Lockfile{Modules: map[string]LockedModule{
			"a": {Version: "v1", Commit: "deadbeef"},
			"b": {Version: "v2", Commit: "cafef00d"},
			"c": {Version: "v1", Commit: "12345678"},
		}}, false},
	}
	for _, tc := range cases {
		got, reason := IsInSync(m, tc.lf)
		if got != tc.want {
			t.Errorf("%s: got=%v want=%v reason=%q", tc.name, got, tc.want, reason)
		}
		if !got && reason == "" {
			t.Errorf("%s: out-of-sync result should include a reason", tc.name)
		}
	}
}
