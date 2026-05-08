// Package apply applies compiled edit operations to source bytes.
package apply

import (
	"fmt"
	"sort"
	"strings"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/effect"
)

// Apply applies ops to src and returns the new bytes.
//
// Ops are sorted by start offset, validated for non-overlap, and
// emitted left-to-right into a builder: the bytes between the previous
// op's end and the current op's start, then the op's replacement text.
// Tail bytes after the last op are appended at the end.
func Apply(src []byte, ops []effect.Op, opts dsl.RewriteOpts) ([]byte, error) {
	if len(ops) == 0 {
		return src, nil
	}

	sorted := make([]effect.Op, len(ops))
	copy(sorted, ops)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		// Pure insertions (Start==End) at the same position go after
		// non-insertions at that position.
		iIns := sorted[i].Start == sorted[i].End
		jIns := sorted[j].Start == sorted[j].End
		if iIns != jIns {
			return !iIns
		}
		return sorted[i].End < sorted[j].End
	})

	for i := 1; i < len(sorted); i++ {
		prev := sorted[i-1]
		cur := sorted[i]
		// Allow exact same point (insert at the deletion's start = end).
		if cur.Start < prev.End {
			return nil, fmt.Errorf("conflicting edits: %q[%d-%d) overlaps %q[%d-%d)",
				prev.Rule, prev.Start, prev.End, cur.Rule, cur.Start, cur.End)
		}
	}

	var b strings.Builder
	b.Grow(len(src))
	cursor := uint32(0)
	for _, op := range sorted {
		b.Write(src[cursor:op.Start])
		b.WriteString(op.Text)
		cursor = op.End
	}
	b.Write(src[cursor:])
	return []byte(b.String()), nil
}
