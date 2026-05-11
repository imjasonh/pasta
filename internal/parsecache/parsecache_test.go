package parsecache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/effect"
)

func TestHashFile_StableAndLanguageScoped(t *testing.T) {
	src := []byte("package x\nfunc f() {}\n")
	a := HashFile("go", src)
	b := HashFile("go", src)
	if a != b {
		t.Fatalf("HashFile not deterministic: %s != %s", a, b)
	}

	// Same bytes, different language → different hash. Otherwise a Go
	// file and a Python file with byte-identical contents would share
	// a cache entry and the rule output would be wrong.
	if HashFile("python", src) == a {
		t.Fatal("HashFile collided across languages")
	}

	// Different bytes → different hash.
	if HashFile("go", []byte("package y\n")) == a {
		t.Fatal("HashFile collided across distinct sources")
	}
}

func TestHashRules_OrderIndependent(t *testing.T) {
	a1 := &dsl.Analyzer{Name: "first", Version: "1", Rules: map[string]dsl.Rule{"r": {Name: "r"}}}
	a2 := &dsl.Analyzer{Name: "second", Version: "1", Rules: map[string]dsl.Rule{"r": {Name: "r"}}}

	h1 := HashRules([]*dsl.Analyzer{a1, a2})
	h2 := HashRules([]*dsl.Analyzer{a2, a1})
	if h1 != h2 {
		t.Fatalf("HashRules depends on input order: %s != %s", h1, h2)
	}

	// A rule edit must flip the hash.
	a1b := &dsl.Analyzer{Name: "first", Version: "2", Rules: map[string]dsl.Rule{"r": {Name: "r"}}}
	if HashRules([]*dsl.Analyzer{a1b, a2}) == h1 {
		t.Fatal("HashRules ignored a version bump")
	}
}

func TestPutGet_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	c := Open(dir, "ruleX", 0)
	if c == nil {
		t.Fatal("Open returned nil")
	}

	want := Entry{
		Diagnostics: []effect.Diagnostic{
			{Rule: "noop", Message: "hi", StartByte: 1, EndByte: 5, LineNum: 1},
		},
		Ops: []effect.Op{
			{Start: 0, End: 4, Text: "abc", Rule: "r"},
		},
	}
	c.Put("file1", want)

	got, ok := c.Get("file1")
	if !ok {
		t.Fatal("Get missed after Put")
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0] != want.Diagnostics[0] {
		t.Errorf("Diagnostics mismatch: got %+v want %+v", got.Diagnostics, want.Diagnostics)
	}
	if len(got.Ops) != 1 || got.Ops[0].Start != want.Ops[0].Start || got.Ops[0].Text != want.Ops[0].Text {
		t.Errorf("Ops mismatch: got %+v want %+v", got.Ops, want.Ops)
	}
}

func TestGet_MissOnEmpty(t *testing.T) {
	c := Open(t.TempDir(), "rule", 0)
	if _, ok := c.Get("nope"); ok {
		t.Fatal("Get hit on empty cache")
	}
	if c.Stats().Misses != 1 {
		t.Errorf("expected miss counter=1, got %d", c.Stats().Misses)
	}
}

func TestPrune_EvictsOldestUntilUnderLimit(t *testing.T) {
	dir := t.TempDir()
	c := Open(dir, "rule", 0) // open unbounded so we can measure first
	if c == nil {
		t.Fatal("Open returned nil")
	}

	mk := func(key, msg string, mtime time.Time) int64 {
		c.Put(key, Entry{Diagnostics: []effect.Diagnostic{{Rule: "r", Message: msg}}})
		path := filepath.Join(dir, schemaVersion, "rule", key+".gob")
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info.Size()
	}

	t0 := time.Now().Add(-3 * time.Hour)
	mk("oldest", "aaaa", t0)
	mk("middle", "bbbb", t0.Add(time.Hour))
	entrySize := mk("newest", "cccc", t0.Add(2*time.Hour))

	// Cap so two entries fit comfortably but three don't. Triggers
	// exactly one eviction — the oldest.
	c.sizeLimit = entrySize*2 + entrySize/2

	if err := c.Prune(); err != nil {
		t.Fatal(err)
	}

	if _, ok := c.Get("newest"); !ok {
		t.Error("newest entry was evicted; LRU policy is backwards")
	}
	if _, ok := c.Get("middle"); !ok {
		t.Error("middle entry was evicted with budget for two entries")
	}
	if _, ok := c.Get("oldest"); ok {
		t.Error("oldest entry survived a prune over the size limit")
	}
}

func TestNilCache_IsNoOp(t *testing.T) {
	var c *Cache
	// All public methods must tolerate nil so the engine can pass it
	// through unconditionally when caching is disabled.
	if got, ok := c.Get("key"); ok || got.Diagnostics != nil {
		t.Errorf("nil Get returned (%+v, %v)", got, ok)
	}
	c.Put("key", Entry{})
	if err := c.Prune(); err != nil {
		t.Errorf("nil Prune: %v", err)
	}
	if (c.Stats() != Stats{}) {
		t.Errorf("nil Stats: %+v", c.Stats())
	}
}

func BenchmarkPut(b *testing.B) {
	dir := b.TempDir()
	c := Open(dir, "r", 0)
	e := Entry{Diagnostics: []effect.Diagnostic{{Rule: "r", Message: "test diagnostic", StartByte: 100, EndByte: 200, LineNum: 5}}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put("key", e)
	}
}

func BenchmarkPutParallel(b *testing.B) {
	dir := b.TempDir()
	c := Open(dir, "r", 0)
	e := Entry{Diagnostics: []effect.Diagnostic{{Rule: "r", Message: "test", StartByte: 100, EndByte: 200, LineNum: 5}}}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Put(fmt.Sprintf("k%d", i), e)
			i++
		}
	})
}
