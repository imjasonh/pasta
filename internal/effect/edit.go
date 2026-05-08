package effect

import (
	"fmt"
	"strings"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/match"
	"github.com/imjasonh/pasta/internal/tsutil"
)

// Op is a compiled edit: replace bytes [Start, End) with Text.
type Op struct {
	Start, End uint32
	Text       string

	// IsInsertion is true when Start == End. Used by post-processing for
	// comment preservation (we prepend floated comments to insertion texts).
	IsInsertion bool

	// Source rule name (for diagnostics on conflicts).
	Rule string

	// Originating Edit (for downstream post-processing).
	Edit dsl.Edit
}

// BuildContext groups inputs for BuildOps that go beyond the per-rewrite
// metadata.
type BuildContext struct {
	Captures    match.Captures
	AdjacentSeq []string // ordered capture names from rule.Match.Adjacent
	RewriteOpts dsl.RewriteOpts
	Root        tsutil.Node // for comment preservation
}

// BuildOps compiles a Rewrite's edits into a list of Ops.
//
// `within` edits are folded into the source-text of their target captures
// at interpolation time, rather than emitted as overlapping byte-range
// edits. Comments inside delete ranges are preserved (when
// PreserveComments is set) by prepending them to the next insertion at
// the delete's end point.
func BuildOps(rule string, rw *dsl.Rewrite, ctx BuildContext) ([]Op, error) {
	if rw == nil {
		return nil, nil
	}

	// Split edits into within-edits (interpolation transforms) and primary
	// edits (real byte-range mutations).
	withinByCap := map[string][]dsl.Edit{}
	var primary []dsl.Edit
	for _, e := range rw.Edits {
		if e.Within != "" {
			withinByCap[e.Within] = append(withinByCap[e.Within], e)
		} else {
			primary = append(primary, e)
		}
	}

	// Compute effective text for each capture (applying within-edits in
	// declared order).
	captureText := map[string]string{}
	for name, n := range ctx.Captures {
		if !n.IsValid() {
			continue
		}
		captureText[name] = applyWithin(n.Text(), withinByCap[name])
	}

	// Build primary ops.
	var ops []Op
	for _, e := range primary {
		op, err := buildPrimaryOp(rule, e, ctx, captureText)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}

	// Comment preservation: for every delete op (with preserve_comments
	// in this rewrite's opts), gather comments and prepend to the next
	// insertion at delete.End. Skip comments that fall inside captures
	// re-inserted by @-interpolation in this rewrite (those are
	// preserved verbatim via interpolation).
	if ctx.RewriteOpts.PreserveComments {
		preservedRanges := capturesPreservedByInterp(rw, ctx.Captures)
		applyCommentPreservation(ops, ctx.Root, preservedRanges)
	}

	return ops, nil
}

// capturesPreservedByInterp returns the byte ranges of captures referenced
// via @name interpolation in any primary edit's text. Comments inside
// those ranges are preserved by the interpolation itself and must not be
// floated separately.
func capturesPreservedByInterp(rw *dsl.Rewrite, caps match.Captures) [][2]uint32 {
	seen := map[string]bool{}
	for _, e := range rw.Edits {
		for _, name := range collectAtRefs(e.Replacement) {
			seen[name] = true
		}
		for _, name := range collectAtRefs(e.Text) {
			seen[name] = true
		}
	}
	var out [][2]uint32
	for name := range seen {
		if n, ok := caps[name]; ok {
			out = append(out, [2]uint32{n.StartByte(), n.EndByte()})
		}
	}
	return out
}

func collectAtRefs(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	i := 0
	for i < len(s) {
		if s[i] != '@' {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && isIdentRune(s[j]) {
			j++
		}
		out = append(out, s[i+1:j])
		i = j
	}
	return out
}

// applyTrim drops `start` bytes from the front of s and `end` bytes
// from the back. If the trim exceeds s's length, the result is empty.
func applyTrim(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start+end >= len(s) {
		return ""
	}
	return s[start : len(s)-end]
}

func applyWithin(src string, edits []dsl.Edit) string {
	cur := src
	for _, e := range edits {
		idx := strings.Index(cur, e.Token)
		if idx < 0 {
			continue
		}
		cur = cur[:idx] + e.ReplaceWith + cur[idx+len(e.Token):]
	}
	return cur
}

func buildPrimaryOp(rule string, e dsl.Edit, ctx BuildContext, captureText map[string]string) (Op, error) {
	switch {
	case e.Target != "":
		n, ok := ctx.Captures[e.Target]
		if !ok {
			return Op{}, fmt.Errorf("edit.target %q: capture missing", e.Target)
		}
		text := interpolateWithCaptures(e.Replacement, ctx.Captures, captureText)
		if e.TrimStart > 0 || e.TrimEnd > 0 {
			text = applyTrim(text, e.TrimStart, e.TrimEnd)
		}
		return Op{
			Start: n.StartByte(),
			End:   n.EndByte(),
			Text:  text,
			Rule:  rule,
			Edit:  e,
		}, nil

	case e.Position != "":
		n, ok := ctx.Captures[e.Anchor]
		if !ok {
			return Op{}, fmt.Errorf("edit.anchor %q: capture missing", e.Anchor)
		}
		var pos uint32
		if e.Position == "before" {
			pos = n.StartByte()
		} else {
			pos = n.EndByte()
		}
		return Op{
			Start:       pos,
			End:         pos,
			Text:        interpolateWithCaptures(e.Text, ctx.Captures, captureText),
			IsInsertion: true,
			Rule:        rule,
			Edit:        e,
		}, nil

	case e.DeleteFrom != "":
		from, ok := ctx.Captures[e.DeleteFrom]
		if !ok {
			_, fromN, fok := nextBoundInSeq(e.DeleteFrom, ctx.AdjacentSeq, ctx.Captures)
			if !fok {
				return Op{}, fmt.Errorf("edit.delete_from %q: no bound capture", e.DeleteFrom)
			}
			from = fromN
		}
		to, ok := ctx.Captures[e.DeleteTo]
		if !ok {
			return Op{}, fmt.Errorf("edit.delete_to %q: capture missing", e.DeleteTo)
		}
		start := from.StartByte()
		if e.DeleteFromEnd {
			start = from.EndByte()
		}
		end := to.StartByte()
		if e.DeleteToEnd {
			end = to.EndByte()
		}
		if end < start {
			return Op{}, fmt.Errorf("delete range inverted: from=%d to=%d", start, end)
		}
		return Op{
			Start: start,
			End:   end,
			Text:  "",
			Rule:  rule,
			Edit:  e,
		}, nil
	}
	return Op{}, fmt.Errorf("edit shape not recognized: %+v", e)
}

// interpolateWithCaptures replaces @name in s with effective capture text.
func interpolateWithCaptures(s string, caps match.Captures, captureText map[string]string) string {
	if !strings.Contains(s, "@") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '@' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i + 1
		for j < len(s) && isIdentRune(s[j]) {
			j++
		}
		name := s[i+1 : j]
		if t, ok := captureText[name]; ok {
			b.WriteString(t)
		} else if n, ok := caps[name]; ok {
			b.WriteString(n.Text())
		} else {
			b.WriteString(s[i:j])
		}
		i = j
	}
	return b.String()
}

// applyCommentPreservation: for each delete-style op, find comment nodes
// inside its byte range (but outside any capture preserved by
// @-interpolation) and prepend them (with proper indent) to the nearest
// subsequent insertion op at the same end point.
func applyCommentPreservation(ops []Op, root tsutil.Node, preserved [][2]uint32) {
	if !root.IsValid() {
		return
	}
	src := root.Src
	for i := range ops {
		op := ops[i]
		if op.IsInsertion || op.End <= op.Start {
			continue
		}
		var comments []tsutil.Node
		tsutil.Walk(root, func(n tsutil.Node) bool {
			s, e := n.Range()
			if e <= op.Start || s >= op.End {
				return false
			}
			if n.Type() == "comment" && s >= op.Start && e <= op.End {
				if !insideAny(s, e, preserved) {
					comments = append(comments, n)
				}
				return false
			}
			return true
		})
		if len(comments) == 0 {
			continue
		}
		indent := leadingIndent(src, op.Start)
		var floated strings.Builder
		for _, c := range comments {
			floated.WriteString(c.Text())
			floated.WriteByte('\n')
			floated.WriteString(indent)
		}
		for j := range ops {
			if ops[j].IsInsertion && ops[j].Start == op.End {
				ops[j].Text = floated.String() + ops[j].Text
				break
			}
		}
	}
}

// insideAny reports whether [s, e) lies fully inside any of the given
// preserved byte ranges.
func insideAny(s, e uint32, ranges [][2]uint32) bool {
	for _, r := range ranges {
		if s >= r[0] && e <= r[1] {
			return true
		}
	}
	return false
}

// leadingIndent returns the whitespace at the start of the line that
// contains `pos` in src (everything from the previous \n+1 to the first
// non-whitespace character).
func leadingIndent(src []byte, pos uint32) string {
	if int(pos) > len(src) {
		pos = uint32(len(src))
	}
	// Find the start of the line.
	start := int(pos)
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	end := start
	for end < int(pos) && (src[end] == ' ' || src[end] == '\t') {
		end++
	}
	return string(src[start:end])
}

// nextBoundInSeq finds the next bound capture in seq starting after the
// given name's position.
func nextBoundInSeq(name string, seq []string, caps match.Captures) (string, tsutil.Node, bool) {
	idx := -1
	for i, s := range seq {
		if s == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", tsutil.Node{}, false
	}
	for i := idx + 1; i < len(seq); i++ {
		if n, ok := caps[seq[i]]; ok {
			return seq[i], n, true
		}
	}
	return "", tsutil.Node{}, false
}
