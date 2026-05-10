package engine

import (
	"reflect"
	"testing"
)

func TestParseSuppressions(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want map[int]suppression
	}{
		{
			name: "no directives",
			src:  "x := 1\ny := 2\n",
			want: nil,
		},
		{
			name: "all-rules form",
			src:  "x := 1 // pasta:ignore\n",
			want: map[int]suppression{1: {all: true}},
		},
		{
			name: "single rule",
			src:  "x := 1 // pasta:ignore go_iferr\n",
			want: map[int]suppression{1: {rules: map[string]bool{"go_iferr": true}}},
		},
		{
			name: "multiple rules comma",
			src:  "x := 1 // pasta:ignore go_iferr, go_negcmp\n",
			want: map[int]suppression{1: {rules: map[string]bool{"go_iferr": true, "go_negcmp": true}}},
		},
		{
			name: "multiple lines",
			src:  "a // pasta:ignore foo\nb\nc // pasta:ignore\n",
			want: map[int]suppression{
				1: {rules: map[string]bool{"foo": true}},
				3: {all: true},
			},
		},
		{
			name: "block comment terminator stripped",
			src:  "x /* pasta:ignore go_iferr */\n",
			want: map[int]suppression{1: {rules: map[string]bool{"go_iferr": true}}},
		},
		{
			name: "html comment terminator stripped",
			src:  "<x/> <!-- pasta:ignore some_rule -->\n",
			want: map[int]suppression{1: {rules: map[string]bool{"some_rule": true}}},
		},
		{
			name: "ignored prefix is not a directive",
			src:  "// pasta:ignored go_iferr\n",
			want: nil,
		},
		{
			name: "no trailing newline",
			src:  "x // pasta:ignore foo",
			want: map[int]suppression{1: {rules: map[string]bool{"foo": true}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSuppressions([]byte(tc.src))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseSuppressions(%q):\n  got:  %#v\n  want: %#v", tc.src, got, tc.want)
			}
		})
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
		{"foo", 3, true},   // all wins
		{"bar", 3, true},   // all wins
		{"foo", 5, true},   // matches name
		{"bar", 5, false},  // not in name set
		{"foo", 99, false}, // line not present
		{"foo", 0, false},  // edge
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
