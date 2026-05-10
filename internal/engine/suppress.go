package engine

import (
	"regexp"
)

// suppression records the rules disabled on a single source line via a
// `pasta:ignore` directive. all=true means every rule is suppressed
// (`// pasta:ignore` with no rule list); otherwise only the names in
// rules are suppressed.
type suppression struct {
	all   bool
	rules map[string]bool
}

// ignoreRe finds `pasta:ignore` directives. The trailing capture group
// holds whatever followed the directive on the same line; rule names
// are extracted from it via nameRe so junk like comment terminators
// (`*/`, `-->`) is naturally filtered out.
var ignoreRe = regexp.MustCompile(`pasta:ignore\b([^\n]*)`)

// nameRe matches an identifier — used to pluck rule names from the
// directive's tail, which is a comma- or whitespace-separated list.
var nameRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// parseSuppressions scans src for `pasta:ignore` comments and returns
// a map from 1-based line number to the rules suppressed on that line.
//
// Forms recognized (any comment style — pasta only looks at the text):
//
//	x := foo() // pasta:ignore                  — suppress every rule on this line
//	x := foo() // pasta:ignore go_iferr         — suppress one rule
//	x := foo() // pasta:ignore go_iferr, go_negcmp  — suppress several
//
// The directive applies to diagnostics whose anchor falls on the same
// line as the directive itself. Both the diagnostic and any rewrite
// the rule would have produced are dropped; fact emission still
// happens, since facts are internal state and dropping them would
// change other rules' behavior in surprising ways.
func parseSuppressions(src []byte) map[int]suppression {
	if len(src) == 0 {
		return nil
	}
	var out map[int]suppression
	line := 1
	lineStart := 0
	for i := 0; i <= len(src); i++ {
		if i == len(src) || src[i] == '\n' {
			ln := src[lineStart:i]
			if entry, ok := parseSuppressionLine(ln); ok {
				if out == nil {
					out = map[int]suppression{}
				}
				out[line] = entry
			}
			line++
			lineStart = i + 1
		}
	}
	return out
}

func parseSuppressionLine(ln []byte) (suppression, bool) {
	m := ignoreRe.FindSubmatchIndex(ln)
	if m == nil {
		return suppression{}, false
	}
	tail := ln[m[2]:m[3]]
	names := nameRe.FindAll(tail, -1)
	if len(names) == 0 {
		return suppression{all: true}, true
	}
	rules := make(map[string]bool, len(names))
	for _, n := range names {
		rules[string(n)] = true
	}
	return suppression{rules: rules}, true
}

// isSuppressed reports whether rule is suppressed at the given 1-based
// line by suppress.
func isSuppressed(suppress map[int]suppression, rule string, line int) bool {
	if len(suppress) == 0 {
		return false
	}
	e, ok := suppress[line]
	if !ok {
		return false
	}
	return e.all || e.rules[rule]
}
