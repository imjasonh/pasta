// Package effect builds diagnostics and edit operations from rule effects
// and capture maps.
package effect

import (
	"strings"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/match"
	"github.com/imjasonh/pasta/internal/tsutil"
)

// Diagnostic is what a rule emits when it matches. It is self-contained
// — no live tree-sitter node references — because the underlying tree
// may be released back to its pool after the engine returns.
type Diagnostic struct {
	Rule     string
	Message  string
	Severity dsl.Severity

	// Byte range of the anchor in the source.
	StartByte, EndByte uint32

	// 1-based line of StartByte. Pre-computed at build time.
	LineNum int
}

// Line returns the 1-based line number.
func (d Diagnostic) Line() int { return d.LineNum }

// BuildDiagnostic interpolates @captures into the message and snapshots
// the anchor's byte range and line number.
func BuildDiagnostic(rule string, dg *dsl.Diagnostic, anchor tsutil.Node, caps match.Captures) Diagnostic {
	return Diagnostic{
		Rule:      rule,
		Message:   Interpolate(dg.Message, caps),
		Severity:  dg.Severity,
		StartByte: anchor.StartByte(),
		EndByte:   anchor.EndByte(),
		LineNum:   computeLine(anchor.Src, anchor.StartByte()),
	}
}

func computeLine(src []byte, pos uint32) int {
	line := 1
	for i := uint32(0); i < pos && int(i) < len(src); i++ {
		if src[i] == '\n' {
			line++
		}
	}
	return line
}

// Interpolate replaces @capture tokens in s with the captured node's text.
func Interpolate(s string, caps match.Captures) string {
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
		if n, ok := caps[name]; ok {
			b.WriteString(n.Text())
		} else {
			b.WriteString(s[i:j])
		}
		i = j
	}
	return b.String()
}

func isIdentRune(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
