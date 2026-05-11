package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/imjasonh/pasta/internal/loader"
)

func TestWalkSources_filtersByMaxFileSize(t *testing.T) {
	dir := t.TempDir()
	small := []byte("package x\n")
	big := make([]byte, 5_000_000) // 5 MB
	for i := range big {
		big[i] = ' '
	}
	for name, data := range map[string][]byte{
		"small.go": small,
		"big.go":   big,
		"tiny.go":  small,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, skipped, err := walkSources(dir, map[string]bool{}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	// Strip the temp-dir prefix so the assertion is portable.
	rel := func(paths []string) []string {
		out := make([]string, len(paths))
		for i, p := range paths {
			out[i] = filepath.Base(p)
		}
		return out
	}
	wantKept := []string{"small.go", "tiny.go"}
	wantSkipped := []string{"big.go"}
	if got := rel(got); !sortedEqual(got, wantKept) {
		t.Errorf("kept: got %v, want %v", got, wantKept)
	}
	if got := rel(skipped); !sortedEqual(got, wantSkipped) {
		t.Errorf("skipped: got %v, want %v", got, wantSkipped)
	}
}

func TestWalkSources_zeroMaxDisablesCap(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, 5_000_000)
	if err := os.WriteFile(filepath.Join(dir, "huge.go"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	got, skipped, err := walkSources(dir, map[string]bool{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "huge.go" {
		t.Errorf("expected huge.go to be kept with maxFileSize=0; got %v", got)
	}
	if len(skipped) != 0 {
		t.Errorf("expected no skipped files; got %v", skipped)
	}
}

func TestResolveMaxFileSize(t *testing.T) {
	cases := []struct {
		name string
		cfg  *loader.Config
		want int64
	}{
		{name: "nil config", cfg: nil, want: defaultMaxFileSize},
		{name: "unset field", cfg: &loader.Config{}, want: defaultMaxFileSize},
		{name: "explicit zero", cfg: &loader.Config{MaxFileSize: int64Ptr(0)}, want: 0},
		{name: "explicit value", cfg: &loader.Config{MaxFileSize: int64Ptr(42)}, want: 42},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveMaxFileSize(tc.cfg); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// Local helpers — defined here rather than in main.go because they're
// test-only.

func int64Ptr(n int64) *int64 { return &n }

func sortedEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sortStrings(ac)
	sortStrings(bc)
	return reflect.DeepEqual(ac, bc)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
