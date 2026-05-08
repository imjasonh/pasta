package pastals

import (
	"sync"
	"testing"
)

func TestDocStore_PutGet(t *testing.T) {
	s := newDocStore()
	d := &document{uri: "file:///x.go"}
	d.update(1, "package x\n")
	s.put(d)

	got, ok := s.get("file:///x.go")
	if !ok {
		t.Fatal("get: missing")
	}
	if got != d {
		t.Errorf("got = %p, want %p", got, d)
	}
}

func TestDocStore_GetMissing(t *testing.T) {
	s := newDocStore()
	if _, ok := s.get("file:///nope.go"); ok {
		t.Fatal("expected miss")
	}
}

func TestDocStore_Delete(t *testing.T) {
	s := newDocStore()
	d := &document{uri: "file:///x.go"}
	s.put(d)
	s.delete("file:///x.go")
	if _, ok := s.get("file:///x.go"); ok {
		t.Fatal("delete didn't remove the entry")
	}
}

func TestDocStore_All(t *testing.T) {
	s := newDocStore()
	for _, u := range []string{"file:///a", "file:///b", "file:///c"} {
		s.put(&document{uri: u})
	}
	got := s.all()
	if len(got) != 3 {
		t.Errorf("len(all()) = %d, want 3", len(got))
	}
}

func TestDocStore_ConcurrentAccess(t *testing.T) {
	s := newDocStore()
	var wg sync.WaitGroup
	const N = 200
	for i := 0; i < N; i++ {
		wg.Add(2)
		uri := "file:///x" + itoa(i)
		go func() {
			defer wg.Done()
			s.put(&document{uri: uri})
		}()
		go func() {
			defer wg.Done()
			_, _ = s.get(uri)
		}()
	}
	wg.Wait()
	if got := len(s.all()); got != N {
		t.Errorf("len(all()) = %d, want %d", got, N)
	}
}

func TestDocument_Update(t *testing.T) {
	d := &document{uri: "file:///a.go"}
	d.update(1, "hello\n")
	if d.version != 1 {
		t.Errorf("version = %d, want 1", d.version)
	}
	if string(d.src) != "hello\n" {
		t.Errorf("src = %q, want hello", string(d.src))
	}
	if d.conv == nil {
		t.Fatal("conv not built")
	}

	// Re-updating preserves the conv-rebuild invariant.
	d.update(2, "different\ncontent\n")
	if d.version != 2 {
		t.Errorf("version = %d, want 2", d.version)
	}
	pos := d.conv.PositionFromByte(10) // "different\n"=10 chars, so byte 10 is 'c'
	if pos.Line != 1 || pos.Character != 0 {
		t.Errorf("position after update = %+v, want line 1 col 0", pos)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
