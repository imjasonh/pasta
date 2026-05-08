// Package apply applies compiled edit operations to source bytes.
package apply

import (
	"fmt"
	"sort"

	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/effect"
)

// Apply applies ops to src and returns the new bytes.
//
// Ops are sorted by start offset and applied right-to-left so byte
// offsets remain stable. Overlapping ops are an error.
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

	// Apply right-to-left so earlier offsets stay valid.
	out := make([]byte, len(src))
	copy(out, src)
	for i := len(sorted) - 1; i >= 0; i-- {
		op := sorted[i]
		out = append(out[:op.Start], append([]byte(op.Text), out[op.End:]...)...)
	}
	return out, nil
}
