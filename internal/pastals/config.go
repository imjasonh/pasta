package pastals

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config is parsed from initializationOptions, with defaults supplied
// for any missing field.
//
// On-save fixing is a client concern: editors configure
// `editor.codeActionsOnSave` with the kind `source.fixAll.pasta`,
// which the server already advertises. There is no FixOnSave knob.
type Config struct {
	Rules      []string `json:"rules"`
	RunOnSave  bool     `json:"runOnSave"`
	DebounceMS int      `json:"debounceMs"`
}

// DefaultConfig returns the default config: rule globs that look for
// `pasta.cue`, a `.pasta/` directory, or an `analyzers/` directory in
// the workspace root, and a 200ms diagnostic debounce.
func DefaultConfig() Config {
	return Config{
		Rules:      []string{"./pasta.cue", "./.pasta/**/*.cue", "./analyzers/**/*.cue"},
		DebounceMS: 200,
	}
}

// parseInitOptions parses an LSP initializationOptions field into a
// Config, filling defaults for any unset values. nil yields the
// defaults.
func parseInitOptions(raw *json.RawMessage) (Config, error) {
	cfg := DefaultConfig()
	if raw == nil || len(*raw) == 0 {
		return cfg, nil
	}
	var got Config
	if err := json.Unmarshal(*raw, &got); err != nil {
		return cfg, fmt.Errorf("decode initializationOptions: %w", err)
	}
	if got.Rules != nil {
		cfg.Rules = got.Rules
	}
	cfg.RunOnSave = got.RunOnSave
	if got.DebounceMS != 0 {
		cfg.DebounceMS = got.DebounceMS
	}
	return cfg, nil
}

// uriToPath converts a `file://` URI to a local filesystem path.
// Returns an error for non-file schemes.
func uriToPath(u string) (string, error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("parse uri: %w", err)
	}
	if parsed.Scheme != "file" {
		return "", fmt.Errorf("only file:// URIs are supported, got %q", parsed.Scheme)
	}
	p, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", fmt.Errorf("unescape uri path: %w", err)
	}
	if runtime.GOOS == "windows" {
		// "file:///C:/foo" -> "C:/foo"
		p = strings.TrimPrefix(p, "/")
	}
	return p, nil
}

// resolveRuleFiles expands cfg.Rules relative to root and returns the
// list of existing CUE files. Globs (** and *) are expanded with
// filepath.Glob (which doesn't natively support `**`); we implement a
// minimal `**` walker.
func resolveRuleFiles(cfg Config, root string) []string {
	seen := map[string]bool{}
	var out []string
	for _, pat := range cfg.Rules {
		if !filepath.IsAbs(pat) {
			pat = filepath.Join(root, pat)
		}
		matches := expandGlob(pat)
		for _, m := range matches {
			if !strings.HasSuffix(m, ".cue") {
				continue
			}
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			if seen[m] {
				continue
			}
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

// expandGlob expands a path pattern that may contain `**` (any number
// of path segments) and `*` (any segment). Single `*` is delegated to
// filepath.Glob.
func expandGlob(pat string) []string {
	// Split on the first `**`. Recurse into each match.
	idx := strings.Index(pat, "**")
	if idx < 0 {
		matches, _ := filepath.Glob(pat)
		return matches
	}
	prefix := strings.TrimSuffix(pat[:idx], string(filepath.Separator))
	suffix := strings.TrimPrefix(pat[idx+2:], string(filepath.Separator))
	if prefix == "" {
		prefix = "."
	}
	// Walk prefix; for every directory, append suffix and recurse
	// (suffix itself may contain `**` or `*`).
	var out []string
	roots, _ := filepath.Glob(prefix)
	if len(roots) == 0 {
		roots = []string{prefix}
	}
	for _, r := range roots {
		_ = filepath.Walk(r, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() {
				return nil
			}
			candidate := filepath.Join(p, suffix)
			out = append(out, expandGlob(candidate)...)
			return nil
		})
	}
	return out
}
