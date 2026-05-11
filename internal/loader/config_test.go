package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/remote"
)

func TestLoadConfig_missing(t *testing.T) {
	dir := t.TempDir()
	cfg, ok, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok || cfg != nil {
		t.Fatalf("expected (nil, false, nil), got (%v, %v, nil)", cfg, ok)
	}
}

func TestLoadConfig_fields(t *testing.T) {
	dir := t.TempDir()
	src := `
disabled_rules: ["go_iferr", "todo_format"]
severity: {go_panic_empty: "error", python_eq_none: "info"}
skip: ["build", "dist"]
`
	if err := os.WriteFile(filepath.Join(dir, remote.ManifestFile), []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, ok, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok || cfg == nil {
		t.Fatalf("expected config, got ok=%v cfg=%v", ok, cfg)
	}
	if !reflect.DeepEqual(cfg.DisabledRules, []string{"go_iferr", "todo_format"}) {
		t.Errorf("disabled_rules: %v", cfg.DisabledRules)
	}
	if !reflect.DeepEqual(cfg.Severity, map[string]string{"go_panic_empty": "error", "python_eq_none": "info"}) {
		t.Errorf("severity: %v", cfg.Severity)
	}
	if !reflect.DeepEqual(cfg.Skip, []string{"build", "dist"}) {
		t.Errorf("skip: %v", cfg.Skip)
	}
}

func TestLoadConfig_importsOnly(t *testing.T) {
	dir := t.TempDir()
	src := `imports: {"github.com/foo/bar": "v1.0.0"}`
	if err := os.WriteFile(filepath.Join(dir, remote.ManifestFile), []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, ok, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok || cfg == nil {
		t.Fatalf("imports-only manifest should yield ok=true, empty Config; got ok=%v cfg=%v", ok, cfg)
	}
	if len(cfg.DisabledRules) != 0 || len(cfg.Severity) != 0 || len(cfg.Skip) != 0 {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestLoadConfig_combined(t *testing.T) {
	dir := t.TempDir()
	src := `
imports: {"github.com/foo/bar": "v1.0.0"}
disabled_rules: ["go_iferr"]
skip: ["build"]
`
	if err := os.WriteFile(filepath.Join(dir, remote.ManifestFile), []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, ok, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if !reflect.DeepEqual(cfg.DisabledRules, []string{"go_iferr"}) {
		t.Errorf("disabled_rules: %v", cfg.DisabledRules)
	}
	if !reflect.DeepEqual(cfg.Skip, []string{"build"}) {
		t.Errorf("skip: %v", cfg.Skip)
	}
}

func TestLoadConfig_maxFileSize(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want *int64
	}{
		{
			name: "absent",
			src:  `disabled_rules: ["x"]`,
			want: nil,
		},
		{
			name: "positive",
			src:  `max_file_size: 2_000_000`,
			want: int64Ptr(2_000_000),
		},
		{
			name: "explicit zero",
			src:  `max_file_size: 0`,
			want: int64Ptr(0),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, remote.ManifestFile), []byte(tc.src), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			cfg, _, err := LoadConfig(dir)
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if !reflect.DeepEqual(cfg.MaxFileSize, tc.want) {
				gv := "nil"
				if cfg.MaxFileSize != nil {
					gv = fmt.Sprintf("%d", *cfg.MaxFileSize)
				}
				wv := "nil"
				if tc.want != nil {
					wv = fmt.Sprintf("%d", *tc.want)
				}
				t.Errorf("MaxFileSize: got %s, want %s", gv, wv)
			}
		})
	}
}

func TestLoadConfig_maxFileSizeNegative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, remote.ManifestFile), []byte(`max_file_size: -1`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := LoadConfig(dir); err == nil {
		t.Fatal("expected error for negative max_file_size")
	}
}

func int64Ptr(n int64) *int64 { return &n }

func TestLoadConfig_invalidSeverity(t *testing.T) {
	dir := t.TempDir()
	src := `severity: {bad_rule: "fatal"}`
	if err := os.WriteFile(filepath.Join(dir, remote.ManifestFile), []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err := LoadConfig(dir)
	if err == nil {
		t.Fatalf("expected error for invalid severity")
	}
}

func TestApplyConfig_disable(t *testing.T) {
	a := &dsl.Analyzer{
		Name: "demo",
		Rules: map[string]dsl.Rule{
			"keep": {Name: "keep"},
			"drop": {Name: "drop"},
		},
	}
	warns := applyConfig(&Config{DisabledRules: []string{"drop"}}, []*dsl.Analyzer{a})
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if _, ok := a.Rules["drop"]; ok {
		t.Errorf("drop should have been removed")
	}
	if _, ok := a.Rules["keep"]; !ok {
		t.Errorf("keep should remain")
	}
}

func TestApplyConfig_severityOverride(t *testing.T) {
	a := &dsl.Analyzer{
		Name: "demo",
		Rules: map[string]dsl.Rule{
			"r": {Name: "r", Diagnose: &dsl.Diagnostic{Message: "hi", Severity: dsl.SeverityWarning}},
		},
	}
	warns := applyConfig(&Config{Severity: map[string]string{"r": "error"}}, []*dsl.Analyzer{a})
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if got := a.Rules["r"].Diagnose.Severity; got != dsl.SeverityError {
		t.Errorf("severity: got %q, want error", got)
	}
}

func TestApplyConfig_severityOnRewriteOnlyRule(t *testing.T) {
	// Regression: severity override on a rule with no Diagnose used
	// to synthesize an empty-message Diagnostic, causing the engine
	// to emit blank diagnostics on every match. Now it leaves the
	// rule alone and emits a warning instead.
	a := &dsl.Analyzer{
		Name: "demo",
		Rules: map[string]dsl.Rule{
			"rewrite_only": {Name: "rewrite_only", Rewrite: &dsl.Rewrite{}},
		},
	}
	warns := applyConfig(&Config{Severity: map[string]string{"rewrite_only": "error"}}, []*dsl.Analyzer{a})
	if a.Rules["rewrite_only"].Diagnose != nil {
		t.Errorf("rewrite-only rule should still have nil Diagnose; got %+v", a.Rules["rewrite_only"].Diagnose)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "rewrite_only") || !strings.Contains(warns[0], "no diagnose block") {
		t.Errorf("expected one warning about no diagnose block; got %v", warns)
	}
}

func TestApplyConfig_typoWarnings(t *testing.T) {
	a := &dsl.Analyzer{
		Name: "demo",
		Rules: map[string]dsl.Rule{
			"real_rule": {Name: "real_rule", Diagnose: &dsl.Diagnostic{Message: "x"}},
		},
	}
	cfg := &Config{
		DisabledRules: []string{"typo_rule"},
		Severity:      map[string]string{"another_typo": "error"},
	}
	warns := applyConfig(cfg, []*dsl.Analyzer{a})
	joined := strings.Join(warns, "\n")
	if !strings.Contains(joined, "typo_rule") {
		t.Errorf("expected warning mentioning typo_rule; got %v", warns)
	}
	if !strings.Contains(joined, "another_typo") {
		t.Errorf("expected warning mentioning another_typo; got %v", warns)
	}
}

func TestApplyConfig_disableSilencesSeverityWarning(t *testing.T) {
	// When a rule is disabled AND has a severity override, we don't
	// want a "severity has no effect" warning — the user's intent
	// (disable) is clearly the dominant signal.
	a := &dsl.Analyzer{
		Name: "demo",
		Rules: map[string]dsl.Rule{
			"r": {Name: "r", Rewrite: &dsl.Rewrite{}},
		},
	}
	cfg := &Config{
		DisabledRules: []string{"r"},
		Severity:      map[string]string{"r": "error"},
	}
	warns := applyConfig(cfg, []*dsl.Analyzer{a})
	if len(warns) != 0 {
		t.Errorf("expected no warnings; got %v", warns)
	}
}

func TestValidateFileMatch(t *testing.T) {
	good := &dsl.Analyzer{
		Name:  "demo",
		Rules: map[string]dsl.Rule{"r": {Name: "r", FileMatch: []string{"*_test.go", "*.h", "[abc]*.cue"}}},
	}
	if err := validateFileMatch(good); err != nil {
		t.Errorf("valid patterns rejected: %v", err)
	}
	bad := &dsl.Analyzer{
		Name:  "demo",
		Rules: map[string]dsl.Rule{"r": {Name: "r", FileMatch: []string{"[unclosed"}}},
	}
	err := validateFileMatch(bad)
	if err == nil {
		t.Fatalf("expected error for unclosed character class")
	}
	if !strings.Contains(err.Error(), "[unclosed") {
		t.Errorf("error should mention the bad pattern; got %v", err)
	}
}

func TestApplyConfig_nilNoop(t *testing.T) {
	a := &dsl.Analyzer{Name: "demo", Rules: map[string]dsl.Rule{"r": {Name: "r"}}}
	if warns := applyConfig(nil, []*dsl.Analyzer{a}); len(warns) != 0 {
		t.Errorf("nil config should produce no warnings; got %v", warns)
	}
	if _, ok := a.Rules["r"]; !ok {
		t.Errorf("nil config should be no-op")
	}
}
