package engine

import (
	"regexp"
	"strings"

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

// directiveRe matches the `pasta:ignore` token itself (no tail). We
// use FindAll positions and slice the tail manually — that way two
// directives on the same line (`// pasta:ignore foo pasta:ignore bar`)
// each get their own tail, instead of the first match greedily
// consuming the rest of the line and leaking the literal words
// `pasta` and `ignore` into the rule-name list.
//
// No comment-leader requirement: callers only invoke us against text
// already known to be a comment (per the language's `comment_types`),
// so a string literal like
// `log("user typed pasta:ignore go_iferr")` can never reach us.
var directiveRe = regexp.MustCompile(`pasta:ignore\b`)

// nameRe matches an identifier — used to pluck rule names from a
// directive's tail, which is a comma- or whitespace-separated list.
// Junk like comment terminators (`*/`, `-->`) is naturally filtered
// out because it doesn't match the identifier pattern.
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
		// A comment's named children (if any) cannot themselves be
		// comments under any grammar we ship, but skipping descent
		// is the defensive choice — guarantees we never double-
		// scan the same span.
		return false
	})
	return out
}

func scanComment(n tsutil.Node, out *map[int]suppression) {
	text := n.Text()
	starts := directiveRe.FindAllStringIndex(text, -1)
	for i, m := range starts {
		// Tail runs from the end of THIS directive to the start of
		// the NEXT one, capped at the next newline. Slicing this
		// way (instead of letting the regex's `.*` greedily eat
		// the rest of the line) prevents a second `pasta:ignore`
		// on the same line from being parsed as rule names.
		tailStart := m[1]
		tailEnd := len(text)
		if i+1 < len(starts) {
			tailEnd = starts[i+1][0]
		}
		if nl := strings.IndexByte(text[tailStart:tailEnd], '\n'); nl >= 0 {
			tailEnd = tailStart + nl
		}
		line := effect.ComputeLine(n.Src, n.StartByte()+uint32(m[0]))
		addSuppression(out, line, parseSuppressionTail(text[tailStart:tailEnd]))
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

// addSuppression merges incoming into out[line] rather than
// overwriting. Two comment nodes on the same line each carrying a
// directive — `/* pasta:ignore foo */ /* pasta:ignore bar */` — both
// take effect after the merge.
func addSuppression(out *map[int]suppression, line int, incoming suppression) {
	if *out == nil {
		*out = map[int]suppression{}
	}
	existing, ok := (*out)[line]
	if !ok {
		(*out)[line] = incoming
		return
	}
	(*out)[line] = mergeSuppression(existing, incoming)
}

// mergeSuppression unions two suppression entries. `all` wins over
// any rule list — once every rule on the line is suppressed, naming
// individual ones adds nothing.
func mergeSuppression(a, b suppression) suppression {
	if a.all || b.all {
		return suppression{all: true}
	}
	out := suppression{rules: make(map[string]bool, len(a.rules)+len(b.rules))}
	for n := range a.rules {
		out.rules[n] = true
	}
	for n := range b.rules {
		out.rules[n] = true
	}
	return out
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
