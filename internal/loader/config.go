package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/imjasonh/pasta/internal/dsl"
)

// ConfigFile is the well-known filename for a project config inside
// the rule directory. Sits alongside pasta.cue (the imports manifest)
// — kept separate so the manifest stays a small focused file the
// remote-imports machinery can parse without dragging in unrelated
// fields.
const ConfigFile = "config.cue"

// Config is project-level configuration loaded from
// `<rules_dir>/config.cue`. Every field is optional; an absent file
// is equivalent to an empty Config.
//
//	disabled_rules: ["go_iferr", "todo_format"]   // skip these rules entirely
//	severity:      {go_panic_empty: "error"}       // override per-rule severity
//	skip:          ["build", "dist"]               // extra ./... walk skip-dirs
type Config struct {
	DisabledRules []string          `json:"disabled_rules,omitempty"`
	Severity      map[string]string `json:"severity,omitempty"`
	Skip          []string          `json:"skip,omitempty"`
}

// LoadConfig reads `<dir>/config.cue` if present. Returns (nil, false,
// nil) when the file doesn't exist; callers treat that as "no
// project config". Validation errors (bad CUE, unknown severity
// value) come back as a non-nil error.
func LoadConfig(dir string) (*Config, bool, error) {
	path := filepath.Join(dir, ConfigFile)
	src, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileBytes(src, cue.Filename(path))
	if err := v.Err(); err != nil {
		return nil, false, fmt.Errorf("parse %s: %s", path, cueErrDetails(err))
	}
	cfg := &Config{}
	if err := decodeStringList(v, "disabled_rules", &cfg.DisabledRules); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	if err := decodeStringList(v, "skip", &cfg.Skip); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	sev := v.LookupPath(cue.ParsePath("severity"))
	if sev.Exists() {
		jb, err := sev.MarshalJSON()
		if err != nil {
			return nil, false, fmt.Errorf("%s: severity: %w", path, err)
		}
		if err := json.Unmarshal(jb, &cfg.Severity); err != nil {
			return nil, false, fmt.Errorf("%s: severity must be a string→string map: %w", path, err)
		}
		for rule, sev := range cfg.Severity {
			if !validSeverity(sev) {
				return nil, false, fmt.Errorf("%s: severity[%q]: %q is not a valid severity (error|warning|info|hint)", path, rule, sev)
			}
		}
	}
	return cfg, true, nil
}

func decodeStringList(v cue.Value, field string, dst *[]string) error {
	x := v.LookupPath(cue.ParsePath(field))
	if !x.Exists() {
		return nil
	}
	jb, err := x.MarshalJSON()
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	if err := json.Unmarshal(jb, dst); err != nil {
		return fmt.Errorf("%s must be a list of strings: %w", field, err)
	}
	return nil
}

func validSeverity(s string) bool {
	switch dsl.Severity(s) {
	case dsl.SeverityError, dsl.SeverityWarning, dsl.SeverityInfo, dsl.SeverityHint:
		return true
	}
	return false
}

// applyConfig mutates analyzers in place: drops rules whose name is
// in DisabledRules and overrides Diagnose.Severity for any rule named
// in Severity. Both are no-ops when the corresponding map/list is
// empty, so callers can pass a possibly-nil Config.
func applyConfig(cfg *Config, analyzers []*dsl.Analyzer) {
	if cfg == nil {
		return
	}
	disabled := map[string]bool{}
	for _, r := range cfg.DisabledRules {
		disabled[r] = true
	}
	for _, a := range analyzers {
		for name, rule := range a.Rules {
			if disabled[rule.Name] || disabled[name] {
				delete(a.Rules, name)
				continue
			}
			if sev, ok := cfg.Severity[rule.Name]; ok {
				if rule.Diagnose == nil {
					rule.Diagnose = &dsl.Diagnostic{}
				}
				rule.Diagnose.Severity = dsl.Severity(sev)
				a.Rules[name] = rule
			}
		}
	}
}
