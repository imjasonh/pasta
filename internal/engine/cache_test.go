package engine

import (
	"context"
	"testing"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/lang"
	"github.com/imjasonh/pasta/internal/parsecache"
)

// TestRunGroup_CacheRoundTrip verifies that a second run with a warm
// cache returns the same diagnostics without re-running the analyzer.
// The first run populates the cache; the second run hits it and we
// confirm hit/miss counters match expectations.
func TestRunGroup_CacheRoundTrip(t *testing.T) {
	src := []byte(`package main

func main() {
	if true {
	} else {
	}
}
`)
	goLang, ok := lang.ByExt(".go")
	if !ok {
		t.Skip("Go language not registered in this test build")
	}

	analyzer := &dsl.Analyzer{
		Name:    "go_empty_else",
		Version: "1",
		Rules: map[string]dsl.Rule{
			"empty_else": {
				Name:      "empty_else",
				Languages: []string{"go"},
				Match: dsl.Pattern{
					Node: []string{"if_statement"},
					Fields: map[string]dsl.Child{
						"alternative": {
							Capture: "alt",
							Pattern: &dsl.Pattern{Node: []string{"block"}},
						},
					},
					Where: []dsl.Predicate{
						{Op: "empty", Args: []dsl.Arg{{Str: "@alt"}}},
					},
				},
				Diagnose: &dsl.Diagnostic{Message: "empty else", Severity: dsl.SeverityHint},
			},
		},
	}

	cache := parsecache.Open(t.TempDir(), parsecache.HashRules([]*dsl.Analyzer{analyzer}), 0)
	if cache == nil {
		t.Fatal("Open returned nil")
	}

	inputs := []FileInput{{FileID: "a.go", Src: src, Lang: goLang}}

	// First run: cache miss, expect to populate.
	r1, err := RunGroup(context.Background(), inputs, []*dsl.Analyzer{analyzer}, WithCache(cache))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(r1) != 1 || len(r1[0].Diagnostics) == 0 {
		t.Fatalf("first run produced no diagnostics: %+v", r1)
	}
	s := cache.Stats()
	if s.Misses != 1 || s.Writes != 1 {
		t.Errorf("first run cache stats: got %+v, want misses=1 writes=1", s)
	}

	// Second run: cache hit, same diagnostics, no rule eval needed.
	r2, err := RunGroup(context.Background(), inputs, []*dsl.Analyzer{analyzer}, WithCache(cache))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(r2) != 1 || len(r2[0].Diagnostics) != len(r1[0].Diagnostics) {
		t.Fatalf("second run diagnostics mismatch: got %d want %d", len(r2[0].Diagnostics), len(r1[0].Diagnostics))
	}
	for i := range r1[0].Diagnostics {
		if r1[0].Diagnostics[i] != r2[0].Diagnostics[i] {
			t.Errorf("diagnostic[%d] differs: %+v vs %+v", i, r1[0].Diagnostics[i], r2[0].Diagnostics[i])
		}
	}
	s = cache.Stats()
	if s.Hits != 1 {
		t.Errorf("second run cache stats: got hits=%d, want 1", s.Hits)
	}

	// Third run with the SAME file but a different rule set should
	// miss — rule_hash scopes entries, so changing the rule cannot
	// silently return stale diagnostics from a prior set.
	other := &dsl.Analyzer{Name: "other", Version: "1", Rules: map[string]dsl.Rule{
		"r": {Name: "r", Match: dsl.Pattern{Node: []string{"identifier"}}},
	}}
	cache2 := parsecache.Open(cache.Dir(), parsecache.HashRules([]*dsl.Analyzer{other}), 0)
	if _, ok := cache2.Get(parsecache.HashFile("go", src)); ok {
		t.Error("file hit cross-rule cache: rule_hash scoping broken")
	}
}
