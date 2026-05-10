package engine

import (
	"regexp"

	"github.com/imjasonh/pasta/internal/effect"
	"github.com/imjasonh/pasta/internal/tsutil"
)

// suppression records the rules disabled on a single source line via a
// `pasta:ignore` directive. all=true means every rule is suppressed
// (`// pasta:ignore` with no rule list); otherwise only the names in
// rules are suppressed.
type suppression struct {
	all   bool
	rules map[string]bool
}

// ignoreRe finds `pasta:ignore` directives within a comment node's
// text. We don't need to require a comment leader in the regex —
// callers only invoke us against text already known to be a comment
// (per the language's `comment_types`), so a string literal like
// `log("user typed pasta:ignore go_iferr")` can never reach us. The
// trailing capture holds whatever followed the directive on the same
// line; rule names are extracted from it via nameRe so junk like
// comment terminators (`*/`, `-->`) is naturally filtered out.
var ignoreRe = regexp.MustCompile(`pasta:ignore\b([^\n]*)`)

// nameRe matches an identifier — used to pluck rule names from the
// directive's tail, which is a comma- or whitespace-separated list.
var nameRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// parseSuppressions walks the parsed tree and returns a map from
// 1-based source line number to the rules suppressed on that line by
// a `pasta:ignore` directive. Only nodes whose Type() is in
// commentTypes are scanned, so the directive can never fire from
// inside a string literal, regex, or other source text — eliminating
// the obvious false-positive class of a plain text scanner.
//
// Forms recognized:
//
//	x := foo() // pasta:ignore                  — suppress every rule on this line
//	x := foo() // pasta:ignore go_iferr         — suppress one rule
//	x := foo() // pasta:ignore go_iferr, go_negcmp  — suppress several
//
// In a multi-line block comment the directive applies to whichever
// line the directive itself sits on, not to the whole comment range.
//
// Suppression skips both the diagnostic and the rewrite for matching
// matches; fact emission still happens, since facts are internal
// state and dropping them would change other rules' behavior in
// surprising ways.
//
// Returns nil when the tree has no root, the language declares no
// comment types, or no directives are found — `isSuppressed` handles
// the nil case.
func parseSuppressions(root tsutil.Node, commentTypes map[string]bool) map[int]suppression {
	if !root.IsValid() || len(commentTypes) == 0 {
		return nil
	}
	var out map[int]suppression
	tsutil.Walk(root, func(n tsutil.Node) bool {
		if !commentTypes[n.Type()] {
			return true
		}
		scanComment(n, &out)
		// Comments don't nest meaningfully; skipping descendants
		// would also be fine, but Walk's default of "visit
		// children" is harmless here (a comment's named children,
		// if any, are not themselves comment nodes per any grammar
		// we ship).
		return true
	})
	return out
}

func scanComment(n tsutil.Node, out *map[int]suppression) {
	text := n.Text()
	for _, m := range ignoreRe.FindAllStringSubmatchIndex(text, -1) {
		// m[0] is the directive's start within `text`; absolute
		// byte position lets us compute the source line.
		line := effect.ComputeLine(n.Src, n.StartByte()+uint32(m[0]))
		entry := parseSuppressionTail(text[m[2]:m[3]])
		if *out == nil {
			*out = map[int]suppression{}
		}
		(*out)[line] = entry
	}
}

func parseSuppressionTail(tail string) suppression {
	names := nameRe.FindAllString(tail, -1)
	if len(names) == 0 {
		return suppression{all: true}
	}
	rules := make(map[string]bool, len(names))
	for _, n := range names {
		rules[n] = true
	}
	return suppression{rules: rules}
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
