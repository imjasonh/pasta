// Package runner is the programmatic entry point for both the CLI and
// Go tests. It loads CUE rule files, runs them against source files,
// validates `// want` markers, and compares fixed output against
// `.golden` files.
package runner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/imjasonh/pasta/internal/apply"
	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/effect"
	"github.com/imjasonh/pasta/internal/engine"
	"github.com/imjasonh/pasta/internal/lang"
	"github.com/imjasonh/pasta/internal/loader"
)

// FileResult holds the engine output for a single source file plus the
// fixed contents (when CheckGolden is on).
type FileResult struct {
	Path        string
	Diagnostics []effect.Diagnostic
	Ops         []effect.Op
	Fixed       []byte
}

// LoadRules loads every *.cue file in dir, returning all analyzers
// found. Any language declarations present in the dir are registered
// with internal/lang so subsequent file dispatch sees them.
func LoadRules(dir string) ([]*dsl.Analyzer, error) {
	res, err := loader.LoadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, ld := range res.Languages {
		if err := lang.Register(ld); err != nil {
			return nil, fmt.Errorf("register language %q: %w", ld.Name, err)
		}
	}
	if len(res.Analyzers) == 0 {
		return nil, fmt.Errorf("%s: no analyzers found", dir)
	}
	return res.Analyzers, nil
}

// LoadRule loads a single .cue file as an analyzer.
func LoadRule(path string) (*dsl.Analyzer, error) {
	return loader.LoadFile(path)
}

// RunFile parses src as the language inferred from path's extension,
// runs every analyzer over it, and returns a FileResult. If applyFixes
// is true, edits are applied and Fixed is populated.
func RunFile(ctx context.Context, path string, src []byte, analyzers []*dsl.Analyzer, applyFixes bool) (FileResult, error) {
	ext := filepath.Ext(path)
	l, ok := lang.ByExt(ext)
	if !ok {
		return FileResult{Path: path}, fmt.Errorf("no language registered for %q", ext)
	}
	res, err := engine.Run(ctx, src, l, analyzers)
	if err != nil {
		return FileResult{Path: path}, err
	}
	out := FileResult{
		Path:        path,
		Diagnostics: res.Diagnostics,
		Ops:         res.Ops,
	}
	if applyFixes {
		fixed, err := apply.Apply(src, res.Ops, dsl.RewriteOpts{})
		if err != nil {
			return out, fmt.Errorf("apply: %w", err)
		}
		out.Fixed = fixed
	}
	return out, nil
}

// TestReport is the outcome of running tests in a rule directory.
type TestReport struct {
	Dir      string
	NumFiles int
	Failures []string // human-readable failure messages
}

// Failed reports whether any failure was recorded.
func (r TestReport) Failed() bool { return len(r.Failures) > 0 }

// TestDir loads every *.cue rule in ruleDir, walks ruleDir/testdata
// recursively, and validates each source file's diagnostics against
// its `// want` markers and (if a `<file>.golden` exists) its fixed
// output against the golden.
//
// Fails if testdata/ is missing or contains no source files mappable
// to a registered language.
func TestDir(ctx context.Context, ruleDir string) (TestReport, error) {
	report := TestReport{Dir: ruleDir}
	analyzers, err := LoadRules(ruleDir)
	if err != nil {
		return report, err
	}

	testdata := filepath.Join(ruleDir, "testdata")
	info, err := os.Stat(testdata)
	if err != nil || !info.IsDir() {
		return report, fmt.Errorf("%s: testdata directory missing", ruleDir)
	}

	var sources []string
	err = filepath.WalkDir(testdata, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".golden") {
			return nil
		}
		ext := filepath.Ext(p)
		if _, ok := lang.ByExt(ext); !ok {
			return nil
		}
		sources = append(sources, p)
		return nil
	})
	if err != nil {
		return report, err
	}
	if len(sources) == 0 {
		return report, fmt.Errorf("%s: no source files in testdata", ruleDir)
	}
	sort.Strings(sources)
	report.NumFiles = len(sources)

	for _, p := range sources {
		src, err := os.ReadFile(p)
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		res, err := RunFile(ctx, p, src, analyzers, true)
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		if msg := checkDiagnostics(src, res.Diagnostics); msg != "" {
			report.Failures = append(report.Failures, fmt.Sprintf("%s: %s", p, msg))
		}
		goldenPath := p + ".golden"
		if _, err := os.Stat(goldenPath); err == nil {
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				report.Failures = append(report.Failures, fmt.Sprintf("%s: read golden: %v", goldenPath, err))
				continue
			}
			if string(res.Fixed) != string(want) {
				report.Failures = append(report.Failures, fmt.Sprintf("%s: fixed output does not match %s\n%s",
					p, filepath.Base(goldenPath), unifiedDiff(string(want), string(res.Fixed))))
			}
		}
	}
	return report, nil
}

// wantRe matches a `want` directive in a comment of any common style.
// An optional `:+N`, `:-N`, or `:N` between `want` and the substring
// shifts the expected diagnostic line — `// want:+1 "..."` says "the
// diag is expected on the next line", which lets test data keep want
// markers off the line being rewritten.
//
// The content between the delimiters is matched as a LITERAL
// substring (not a regex). Pick the delimiter — `"..."` or `\`...\“
// — based on which character (if either) you need to appear literally
// inside the want text.
//
//	// want "fragment of message"
//	# want "fragment"
//	-- want "fragment"             (SQL/Lua)
//	// want:+1 "fragment"          — diagnostic expected on the next line
//	// want:-1 "fragment"          — diagnostic expected on the previous line
//
// Comment leaders we recognize as want-marker prefixes. Each is
// matched as a literal string before optional whitespace + `want`.
//
//	//, #, --                — line comments (Go/C/JS/Python/Ruby/SQL/Lua)
//	/*                       — block-comment start (CSS, C/Java/JS block style)
//	<!--                     — HTML/XML comment start
//
// We only need the LEAD-IN; the trailing `*/` or `-->` comes after
// the want body and isn't consumed here.
//
// RE2 doesn't support backreferences, so the body is matched as
// either a backtick-delimited or double-quote-delimited literal —
// pick whichever delimiter (if any) you need to appear inside.
var wantRe = regexp.MustCompile("(?://|#|--|/\\*|<!--)\\s*want(?::([+\\-]?\\d+))?\\s+(?:`([^`]*)`|\"([^\"]*)\")")

// checkDiagnostics returns "" on success, or a multi-line failure
// message describing missing/extra diagnostics.
func checkDiagnostics(src []byte, diags []effect.Diagnostic) string {
	wants := extractWantMarkers(src)
	got := byLine(diags)

	all := map[int]bool{}
	for ln := range wants {
		all[ln] = true
	}
	for ln := range got {
		all[ln] = true
	}
	var keys []int
	for k := range all {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	var msgs []string
	for _, ln := range keys {
		w := wants[ln]
		g := got[ln]
		matched := make([]bool, len(g))
		for _, want := range w {
			ok := false
			for i, d := range g {
				if matched[i] {
					continue
				}
				if strings.Contains(d.Message, want) {
					matched[i] = true
					ok = true
					break
				}
			}
			if !ok {
				msgs = append(msgs, fmt.Sprintf("line %d: no diagnostic contained %q; got %v", ln, want, msgList(g)))
			}
		}
		for i, m := range matched {
			if !m {
				msgs = append(msgs, fmt.Sprintf("line %d: unexpected diagnostic: %q", ln, g[i].Message))
			}
		}
	}
	return strings.Join(msgs, "\n")
}

func extractWantMarkers(src []byte) map[int][]string {
	out := map[int][]string{}
	for i, line := range strings.Split(string(src), "\n") {
		for _, m := range wantRe.FindAllStringSubmatch(line, -1) {
			// m[1] = optional offset; m[2] = backtick-delimited content
			// (empty if quote form); m[3] = quote-delimited content.
			line := i + 1
			if m[1] != "" {
				off, err := parseOffset(m[1])
				if err == nil {
					line += off
				}
			}
			content := m[2]
			if content == "" {
				content = m[3]
			}
			out[line] = append(out[line], content)
		}
	}
	return out
}

// parseOffset parses "+1", "-2", "5" — the optional colon-separated
// offset on a want marker.
func parseOffset(s string) (int, error) {
	sign := 1
	if len(s) > 0 && s[0] == '+' {
		s = s[1:]
	} else if len(s) > 0 && s[0] == '-' {
		sign = -1
		s = s[1:]
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad offset")
		}
		n = n*10 + int(c-'0')
	}
	return sign * n, nil
}

func byLine(diags []effect.Diagnostic) map[int][]effect.Diagnostic {
	out := map[int][]effect.Diagnostic{}
	for _, d := range diags {
		out[d.Line()] = append(out[d.Line()], d)
	}
	return out
}

func msgList(d []effect.Diagnostic) []string {
	out := make([]string, len(d))
	for i, x := range d {
		out[i] = x.Message
	}
	return out
}

func unifiedDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	var b strings.Builder
	max := len(wl)
	if len(gl) > max {
		max = len(gl)
	}
	for i := 0; i < max; i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			fmt.Fprintf(&b, "  L%d-: %s\n  L%d+: %s\n", i+1, w, i+1, g)
		}
	}
	return b.String()
}
