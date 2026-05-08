package pastals

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseInitOptions_Defaults(t *testing.T) {
	cfg, err := parseInitOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) == 0 {
		t.Errorf("expected default rule globs, got empty")
	}
	if cfg.DebounceMS != 200 {
		t.Errorf("DebounceMS = %d, want 200", cfg.DebounceMS)
	}
}

func TestParseInitOptions_Override(t *testing.T) {
	raw := json.RawMessage(`{"rules":["./foo.cue"],"debounceMs":50,"runOnSave":true}`)
	cfg, err := parseInitOptions(&raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 1 || cfg.Rules[0] != "./foo.cue" {
		t.Errorf("Rules = %v, want [./foo.cue]", cfg.Rules)
	}
	if cfg.DebounceMS != 50 {
		t.Errorf("DebounceMS = %d, want 50", cfg.DebounceMS)
	}
	if !cfg.RunOnSave {
		t.Errorf("RunOnSave = false, want true")
	}
}

func TestParseInitOptions_PartialOverride(t *testing.T) {
	// Only rules supplied — other fields should fall back to defaults.
	raw := json.RawMessage(`{"rules":["x.cue"]}`)
	cfg, err := parseInitOptions(&raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DebounceMS != 200 {
		t.Errorf("DebounceMS = %d, want default 200", cfg.DebounceMS)
	}
	if len(cfg.Rules) != 1 {
		t.Errorf("Rules = %v, want [x.cue]", cfg.Rules)
	}
}

func TestParseInitOptions_Malformed(t *testing.T) {
	raw := json.RawMessage(`{not json`)
	_, err := parseInitOptions(&raw)
	if err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

func TestUriToPath_FileScheme(t *testing.T) {
	p, err := uriToPath("file:///tmp/foo.go")
	if err != nil {
		t.Fatal(err)
	}
	if p != "/tmp/foo.go" {
		t.Errorf("got %q, want /tmp/foo.go", p)
	}
}

func TestUriToPath_Escaped(t *testing.T) {
	p, err := uriToPath("file:///tmp/with%20space.go")
	if err != nil {
		t.Fatal(err)
	}
	if p != "/tmp/with space.go" {
		t.Errorf("got %q, want /tmp/with space.go", p)
	}
}

func TestUriToPath_NonFileScheme(t *testing.T) {
	_, err := uriToPath("http://example.com/foo")
	if err == nil {
		t.Fatal("expected error for non-file scheme")
	}
}

func TestPathURIRoundTrip(t *testing.T) {
	// On Unix, /a/b/c -> file:///a/b/c -> /a/b/c.
	cases := map[string]string{
		"file:///a/b/c.go":              "/a/b/c.go",
		"file:///tmp/x.cue":             "/tmp/x.cue",
		"file:///some%20path/file.go":   "/some path/file.go",
	}
	for uri, p := range cases {
		got, err := uriToPath(uri)
		if err != nil {
			t.Errorf("uriToPath(%q): %v", uri, err)
			continue
		}
		if got != p {
			t.Errorf("round-trip: %q -> %q (want %q)", uri, got, p)
		}
	}
}

func TestResolveRuleFiles_SimpleGlob(t *testing.T) {
	tmp := t.TempDir()
	mkfile(t, filepath.Join(tmp, "a.cue"), "")
	mkfile(t, filepath.Join(tmp, "b.cue"), "")
	mkfile(t, filepath.Join(tmp, "c.txt"), "") // shouldn't match

	cfg := Config{Rules: []string{"./*.cue"}}
	got := resolveRuleFiles(cfg, tmp)
	if len(got) != 2 {
		t.Errorf("got %d files, want 2: %v", len(got), got)
	}
	for _, p := range got {
		if filepath.Ext(p) != ".cue" {
			t.Errorf("non-.cue file in result: %s", p)
		}
	}
}

func TestResolveRuleFiles_DoubleStar(t *testing.T) {
	tmp := t.TempDir()
	mkfile(t, filepath.Join(tmp, "a.cue"), "")
	mkfile(t, filepath.Join(tmp, "sub", "b.cue"), "")
	mkfile(t, filepath.Join(tmp, "sub", "deep", "c.cue"), "")

	cfg := Config{Rules: []string{"./**/*.cue"}}
	got := resolveRuleFiles(cfg, tmp)
	if len(got) < 2 {
		t.Errorf("expected at least 2 files (b.cue, c.cue), got %d: %v", len(got), got)
	}
}

func TestResolveRuleFiles_AbsoluteOverride(t *testing.T) {
	tmp := t.TempDir()
	mkfile(t, filepath.Join(tmp, "x.cue"), "")
	cfg := Config{Rules: []string{filepath.Join(tmp, "x.cue")}}
	got := resolveRuleFiles(cfg, "/some/other/root")
	if len(got) != 1 {
		t.Errorf("got %d files, want 1: %v", len(got), got)
	}
}

func TestResolveRuleFiles_Dedup(t *testing.T) {
	tmp := t.TempDir()
	mkfile(t, filepath.Join(tmp, "a.cue"), "")
	cfg := Config{Rules: []string{"./*.cue", "./a.cue"}}
	got := resolveRuleFiles(cfg, tmp)
	if len(got) != 1 {
		t.Errorf("got %d files, want 1 (deduped): %v", len(got), got)
	}
}

func mkfile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
