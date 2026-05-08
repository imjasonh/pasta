package loader

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/imjasonh/pasta/internal/dsl"
)

// rootCapture is bound by the engine on every match to the matched node
// itself. Always considered "in scope" for capture validation.
const rootCapture = "_root"

// validateCaptures verifies that every capture reference in a's rules
// resolves to a capture actually bound by the rule's match pattern.
// References can appear in:
//
//   - Where predicate args (positional Arg.Str or Arg.List items)
//   - Pre-condition check args (named map of Arg)
//   - Rewrite edits (Target, Anchor, DeleteFrom, DeleteTo, Within are
//     bare capture names; Replacement, Text, ReplaceWith carry @name
//     interpolations within string content)
//   - Emit-fact Attach (bare capture name) and Payload string values
//     (@name interpolations)
//
// On success returns nil. On failure returns one error per offending
// reference, joined with "; ".
func validateCaptures(a *dsl.Analyzer) error {
	var errs []string
	for _, ruleKey := range sortedKeys(a.Rules) {
		r := a.Rules[ruleKey]
		caps := map[string]bool{rootCapture: true}
		collectCaptures(&r.Match, caps)
		for _, ref := range gatherCaptureRefs(&r) {
			if !captureBound(ref.name, caps) {
				errs = append(errs,
					fmt.Sprintf("rule %q: %s references unknown capture %q (have: %s)",
						r.Name, ref.where, ref.name, sortedCapList(caps)))
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

// captureBound reports whether name resolves. `name` may be a bare
// capture name OR a `|`-delimited fallback chain like "a|b|c" or
// "@a|b|@c"; alternatives may carry redundant "@" prefixes (the engine
// strips them in splitOr). We require *every* alternative to be
// defined — a typo'd alternative is always a bug, even though the
// engine falls back at runtime.
func captureBound(name string, caps map[string]bool) bool {
	for _, alt := range strings.Split(name, "|") {
		alt = strings.TrimSpace(alt)
		alt = strings.TrimPrefix(alt, "@")
		if !caps[alt] {
			return false
		}
	}
	return true
}

// captureRef is a discovered reference: the capture name and a human
// description of where it appeared (used in the error message).
type captureRef struct {
	name  string
	where string
}

// collectCaptures walks p recursively, recording every capture name it
// binds.
func collectCaptures(p *dsl.Pattern, out map[string]bool) {
	if p == nil {
		return
	}
	for _, c := range p.Fields {
		collectChildCaptures(&c, out)
	}
	for i := range p.Children {
		collectChildCaptures(&p.Children[i], out)
	}
	for i := range p.Adjacent {
		collectChildCaptures(&p.Adjacent[i], out)
	}
	collectChildCaptures(p.Preceding, out)
}

func collectChildCaptures(c *dsl.Child, out map[string]bool) {
	if c == nil {
		return
	}
	if c.Capture != "" {
		out[c.Capture] = true
	}
	collectCaptures(c.AsPattern(), out)
	collectChildCaptures(c.Preceding, out)
}

// gatherCaptureRefs scans every place in r that can carry a capture
// reference, returning each one with a label describing its location.
func gatherCaptureRefs(r *dsl.Rule) []captureRef {
	var refs []captureRef

	// where predicates inside the pattern (recursive)
	collectWhereRefs(&r.Match, "match", &refs)

	// pre_conditions
	for i, pc := range r.PreConditions {
		base := fmt.Sprintf("pre_conditions[%d:%s]", i, pc.Check)
		for k, v := range pc.Args {
			collectArgRefs(v, base+"."+k, &refs)
		}
	}

	// rewrite edits
	if r.Rewrite != nil {
		for i, e := range r.Rewrite.Edits {
			base := fmt.Sprintf("rewrite.edits[%d]", i)
			collectEditRefs(e, base, &refs)
		}
	}

	// emit
	for i, em := range r.Emit {
		base := fmt.Sprintf("emit[%d]", i)
		if em.Attach != "" {
			refs = append(refs, captureRef{name: em.Attach, where: base + ".attach"})
		}
		collectPayloadRefs(em.Payload, base+".payload", &refs)
	}
	return refs
}

// predicateCaptureArgs lists positional arg indices that are *capture
// references* for known predicates. Other positions are literals (regex
// patterns, fact names, type names, etc.) and may legitimately start
// with "@" — we don't try to interpret them as captures.
//
// Predicates not in this map fall through to "best-effort": every
// @-prefixed arg is treated as a capture reference. This matches the
// usual convention but won't catch false positives in user-defined
// predicates that take @-leading literals.
var predicateCaptureArgs = map[string][]int{
	"eq":                   {0},
	"neq":                  {0},
	"matches":              {0},
	"not_matches":          {0},
	"token_eq":             {0},
	"nil_comparison":       {0, 1, 2},
	"last_non_blank":       {0, 1},
	"same_ident":           {0, 1},
	"has_fact":             {0},
	"not_has_fact":         {0},
	"ancestor_is":          {0},
	"field_absent":         {0},
	"empty":                {0},
	"named_child_count":    {0},
	"prev_sibling_matches": {0},
	"stmt_index_delta":     {0, 1},
	"type_is":              {0},
	"all_match":            {0},
}

func collectWhereRefs(p *dsl.Pattern, path string, out *[]captureRef) {
	if p == nil {
		return
	}
	for i, pred := range p.Where {
		base := fmt.Sprintf("%s.where[%d:%s]", path, i, pred.Op)
		capPositions, known := predicateCaptureArgs[pred.Op]
		for j, arg := range pred.Args {
			if known {
				if !intIn(capPositions, j) {
					continue // literal arg; not a capture ref
				}
				collectArgRefs(arg, fmt.Sprintf("%s.args[%d]", base, j), out)
			} else {
				// Unknown op — best-effort: validate every @-prefixed arg.
				collectArgRefs(arg, fmt.Sprintf("%s.args[%d]", base, j), out)
			}
		}
	}
	for k, c := range p.Fields {
		collectWhereChildRefs(&c, fmt.Sprintf("%s.fields.%s", path, k), out)
	}
	for i := range p.Children {
		collectWhereChildRefs(&p.Children[i], fmt.Sprintf("%s.children[%d]", path, i), out)
	}
	for i := range p.Adjacent {
		collectWhereChildRefs(&p.Adjacent[i], fmt.Sprintf("%s.adjacent[%d]", path, i), out)
	}
	collectWhereChildRefs(p.Preceding, path+".preceding", out)
}

func collectWhereChildRefs(c *dsl.Child, path string, out *[]captureRef) {
	if c == nil {
		return
	}
	collectWhereRefs(c.AsPattern(), path, out)
	collectWhereChildRefs(c.Preceding, path+".preceding", out)
}

// collectArgRefs interprets an Arg as carrying capture references.
// Strings and list items beginning with "@" are treated as capture
// references (with leading "@" stripped). Other values are literals.
func collectArgRefs(a dsl.Arg, where string, out *[]captureRef) {
	if a.IsList() {
		for i, s := range a.List {
			if name, ok := atRef(s); ok {
				*out = append(*out, captureRef{name: name, where: fmt.Sprintf("%s[%d]", where, i)})
			}
		}
		return
	}
	if name, ok := atRef(a.Str); ok {
		*out = append(*out, captureRef{name: name, where: where})
	}
}

func atRef(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "@") {
		return "", false
	}
	return s[1:], true
}

func collectEditRefs(e dsl.Edit, base string, out *[]captureRef) {
	// Bare-capture-name fields.
	if e.Target != "" {
		*out = append(*out, captureRef{name: e.Target, where: base + ".target"})
	}
	if e.Anchor != "" {
		*out = append(*out, captureRef{name: e.Anchor, where: base + ".anchor"})
	}
	if e.DeleteFrom != "" {
		*out = append(*out, captureRef{name: e.DeleteFrom, where: base + ".delete_from"})
	}
	if e.DeleteTo != "" {
		*out = append(*out, captureRef{name: e.DeleteTo, where: base + ".delete_to"})
	}
	if e.Within != "" {
		*out = append(*out, captureRef{name: e.Within, where: base + ".within"})
	}
	// Strings carrying inline @name interpolations.
	for _, name := range scanAtRefs(e.Replacement) {
		*out = append(*out, captureRef{name: name, where: base + ".replacement"})
	}
	for _, name := range scanAtRefs(e.Text) {
		*out = append(*out, captureRef{name: name, where: base + ".text"})
	}
	for _, name := range scanAtRefs(e.ReplaceWith) {
		*out = append(*out, captureRef{name: name, where: base + ".replace_with"})
	}
}

// atRefRe matches an @-prefixed identifier embedded in arbitrary text
// (e.g. "!@expr" or "f(@a, @b)"). Allows alphanumerics + underscore;
// supports the leading "_" in "_root".
var atRefRe = regexp.MustCompile(`@([A-Za-z_][A-Za-z0-9_]*)`)

func scanAtRefs(s string) []string {
	matches := atRefRe.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

func collectPayloadRefs(p any, base string, out *[]captureRef) {
	m, ok := p.(map[string]any)
	if !ok {
		return
	}
	for k, v := range m {
		if s, ok := v.(string); ok {
			for _, name := range scanAtRefs(s) {
				*out = append(*out, captureRef{name: name, where: base + "." + k})
			}
		}
	}
}

func intIn(xs []int, want int) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCapList(caps map[string]bool) string {
	out := make([]string, 0, len(caps))
	for k := range caps {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
