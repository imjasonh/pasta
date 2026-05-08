package pastals

import (
	"testing"

	"github.com/imjasonh/pasta/internal/pastals/protocol"
)

func TestShouldOfferFixAll(t *testing.T) {
	cases := []struct {
		name string
		only []string
		want bool
	}{
		{"empty Only -> all kinds offered", nil, true},
		{"explicit fixAll kind", []string{protocol.CodeActionKindSourceFixAll}, true},
		{"prefix 'source'", []string{"source"}, true},
		{"prefix 'source.fixAll'", []string{"source.fixAll"}, true},
		{"only quickfix -> no fixAll", []string{"quickfix"}, false},
		{"only refactor -> no fixAll", []string{"refactor"}, false},
		{"mixed: contains source", []string{"quickfix", "source"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldOfferFixAll(tc.only); got != tc.want {
				t.Errorf("got %v, want %v (only=%v)", got, tc.want, tc.only)
			}
		})
	}
}

func TestDropOverlapping(t *testing.T) {
	cases := []struct {
		name string
		in   []protocol.EncodedOp
		want []protocol.EncodedOp
	}{
		{
			"no overlap",
			[]protocol.EncodedOp{
				{Start: 0, End: 5, Text: "a"},
				{Start: 10, End: 15, Text: "b"},
			},
			[]protocol.EncodedOp{
				{Start: 0, End: 5, Text: "a"},
				{Start: 10, End: 15, Text: "b"},
			},
		},
		{
			"second op overlaps first",
			[]protocol.EncodedOp{
				{Start: 0, End: 10, Text: "a"},
				{Start: 5, End: 15, Text: "b"},
			},
			[]protocol.EncodedOp{
				{Start: 0, End: 10, Text: "a"},
			},
		},
		{
			"unsorted input is sorted then deduped",
			[]protocol.EncodedOp{
				{Start: 10, End: 15, Text: "b"},
				{Start: 0, End: 5, Text: "a"},
				{Start: 12, End: 13, Text: "c"}, // contained in b -> dropped
			},
			[]protocol.EncodedOp{
				{Start: 0, End: 5, Text: "a"},
				{Start: 10, End: 15, Text: "b"},
			},
		},
		{
			"identical ops -> first kept",
			[]protocol.EncodedOp{
				{Start: 0, End: 5, Text: "a"},
				{Start: 0, End: 5, Text: "duplicate"},
			},
			[]protocol.EncodedOp{
				{Start: 0, End: 5, Text: "a"},
			},
		},
		{
			"adjacent ops do NOT overlap",
			[]protocol.EncodedOp{
				{Start: 0, End: 5, Text: "a"},
				{Start: 5, End: 10, Text: "b"}, // touches but doesn't overlap
			},
			[]protocol.EncodedOp{
				{Start: 0, End: 5, Text: "a"},
				{Start: 5, End: 10, Text: "b"},
			},
		},
		{
			"empty",
			[]protocol.EncodedOp{},
			[]protocol.EncodedOp{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dropOverlapping(tc.in)
			if !equalOps(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestOpsToTextEdits(t *testing.T) {
	doc := &document{}
	doc.update(1, "hello\nworld\n")
	ops := []protocol.EncodedOp{
		{Start: 0, End: 5, Text: "HELLO"},
		{Start: 6, End: 11, Text: "WORLD"},
	}
	got := opsToTextEdits(doc, ops)
	if len(got) != 2 {
		t.Fatalf("got %d edits, want 2", len(got))
	}
	if got[0].NewText != "HELLO" {
		t.Errorf("edit 0 NewText = %q, want HELLO", got[0].NewText)
	}
	if got[0].Range.Start.Line != 0 || got[0].Range.Start.Character != 0 {
		t.Errorf("edit 0 start = %+v, want line 0 col 0", got[0].Range.Start)
	}
	if got[1].Range.Start.Line != 1 || got[1].Range.Start.Character != 0 {
		t.Errorf("edit 1 start = %+v, want line 1 col 0", got[1].Range.Start)
	}
}

func equalOps(a, b []protocol.EncodedOp) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Start != b[i].Start || a[i].End != b[i].End || a[i].Text != b[i].Text {
			return false
		}
	}
	return true
}
