package loader

import (
	"os"
	"path/filepath"
	"reflect"
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
	applyConfig(&Config{DisabledRules: []string{"drop"}}, []*dsl.Analyzer{a})
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
	applyConfig(&Config{Severity: map[string]string{"r": "error"}}, []*dsl.Analyzer{a})
	if got := a.Rules["r"].Diagnose.Severity; got != dsl.SeverityError {
		t.Errorf("severity: got %q, want error", got)
	}
}

func TestApplyConfig_nilNoop(t *testing.T) {
	a := &dsl.Analyzer{Name: "demo", Rules: map[string]dsl.Rule{"r": {Name: "r"}}}
	applyConfig(nil, []*dsl.Analyzer{a})
	if _, ok := a.Rules["r"]; !ok {
		t.Errorf("nil config should be no-op")
	}
}
