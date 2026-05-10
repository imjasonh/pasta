package engine

import (
	"reflect"
	"testing"

	"github.com/imjasonh/pasta/internal/tsutil"
	"github.com/odvcencio/gotreesitter/grammars"
)

// parseGo parses src as Go and returns the root + a default comment-
// types map. The grammar emits comment nodes as type "comment".
func parseGo(t *testing.T, src string) (tsutil.Node, func()) {
	t.Helper()
	tree, root, err := tsutil.Parse(t.Context(), grammars.GoLanguage(), []byte(src), "test.go")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return root, func() { tree.Release() }
}

var goCommentTypes = map[string]bool{"comment": true}

func TestParseSuppressions(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want map[int]suppression
	}{
		{
			name: "no directives",
			src:  "package p\n\nvar x = 1\n",
			want: nil,
		},
		{
			name: "all-rules form",
			src:  "package p\n\nvar x = 1 // pasta:ignore\n",
			want: map[int]suppression{3: {all: true}},
		},
		{
			name: "single rule",
			src:  "package p\n\nvar x = 1 // pasta:ignore go_iferr\n",
			want: map[int]suppression{3: {rules: map[string]bool{"go_iferr": true}}},
		},
		{
			name: "multiple rules comma",
			src:  "package p\n\nvar x = 1 // pasta:ignore go_iferr, go_negcmp\n",
			want: map[int]suppression{3: {rules: map[string]bool{"go_iferr": true, "go_negcmp": true}}},
		},
		{
			name: "multiple lines",
			src:  "package p\n\n// pasta:ignore foo\nvar x = 1\nvar y = 2 // pasta:ignore\n",
			want: map[int]suppression{
				3: {rules: map[string]bool{"foo": true}},
				5: {all: true},
			},
		},
		{
			name: "block comment terminator stripped",
			src:  "package p\n\nvar x = 1 /* pasta:ignore go_iferr */\n",
			want: map[int]suppression{3: {rules: map[string]bool{"go_iferr": true}}},
		},
		{
			name: "multiline block comment — directive line is what matters",
			src:  "package p\n\n/*\n * pasta:ignore foo\n */\nvar x = 1\n",
			want: map[int]suppression{4: {rules: map[string]bool{"foo": true}}},
		},
		{
			name: "ignored prefix is not a directive",
			src:  "package p\n\n// pasta:ignored go_iferr\n",
			want: nil,
		},
		{
			name: "string literal containing the directive does NOT trigger",
			// The whole point of using parsed comment nodes: a
			// string literal that happens to contain `pasta:ignore`
			// is a string node, not a comment node, so the scanner
			// never sees it.
			src:  "package p\n\nvar s = \"// pasta:ignore foo\"\n",
			want: nil,
		},
		{
			name: "string literal with bare directive — same",
			src:  "package p\n\nvar s = \"pasta:ignore foo\"\n",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, release := parseGo(t, tc.src)
			defer release()
			got := parseSuppressions(root, goCommentTypes)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseSuppressions:\nsrc:\n%s\n  got:  %#v\n  want: %#v", tc.src, got, tc.want)
			}
		})
	}
}

func TestParseSuppressions_emptyCommentTypes(t *testing.T) {
	// A language without declared comment types disables suppression
	// entirely — better to be inert than to scan source bytes
	// blindly.
	root, release := parseGo(t, "package p\n\nvar x = 1 // pasta:ignore foo\n")
	defer release()
	if got := parseSuppressions(root, nil); got != nil {
		t.Errorf("expected nil for empty commentTypes; got %#v", got)
	}
	if got := parseSuppressions(root, map[string]bool{}); got != nil {
		t.Errorf("expected nil for empty commentTypes; got %#v", got)
	}
}

func TestIsSuppressed(t *testing.T) {
	m := map[int]suppression{
		3: {all: true},
		5: {rules: map[string]bool{"foo": true}},
	}
	cases := []struct {
		rule string
		line int
		want bool
	}{
		{"foo", 3, true},
		{"bar", 3, true},
		{"foo", 5, true},
		{"bar", 5, false},
		{"foo", 99, false},
		{"foo", 0, false},
	}
	for _, tc := range cases {
		got := isSuppressed(m, tc.rule, tc.line)
		if got != tc.want {
			t.Errorf("isSuppressed(%q, line=%d) = %v, want %v", tc.rule, tc.line, got, tc.want)
		}
	}
	if isSuppressed(nil, "foo", 1) {
		t.Errorf("isSuppressed on nil map should be false")
	}
}
