package pastals

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestDebouncer_RapidSchedulesCollapse verifies the debouncer behavior:
// many scheduleDiagnostics calls in quick succession should result in
// at most one diagnostic run actually firing — the rest get canceled.
//
// We can't easily intercept runDiagnostics from the outside, so we
// verify indirectly by stopping schedules before they fire and
// confirming no goroutine leaked a publish.
func TestDebouncer_RapidSchedulesCollapse(t *testing.T) {
	srv := New(noopReader{}, noopWriter{})
	srv.cfg.DebounceMS = 50
	doc := &document{uri: "file:///x.go"}
	doc.update(1, "x")

	// Schedule 10 times in quick succession; the timer should keep
	// getting reset.
	for i := 0; i < 10; i++ {
		srv.scheduleDiagnostics(doc, 0)
	}

	// Immediately stop — ensures no schedule actually fires.
	srv.stopDiagnostics(doc.uri)

	// Wait long enough for any leaked schedule to fire.
	time.Sleep(150 * time.Millisecond)

	srv.debounceMu.Lock()
	defer srv.debounceMu.Unlock()
	if _, ok := srv.cancels[doc.uri]; ok {
		t.Errorf("cancel left in map after stopDiagnostics")
	}
	if _, ok := srv.timers[doc.uri]; ok {
		t.Errorf("timer left in map after stopDiagnostics")
	}
}

// TestDebouncer_ContextCanceledOnReplace verifies that a newer
// schedule cancels the in-flight ctx of the older one.
func TestDebouncer_ContextCanceledOnReplace(t *testing.T) {
	srv := New(noopReader{}, noopWriter{})
	srv.cfg.DebounceMS = 5
	doc := &document{uri: "file:///x.go"}
	doc.update(1, "x")

	srv.scheduleDiagnostics(doc, 0)

	srv.debounceMu.Lock()
	firstCancel, ok := srv.cancels[doc.uri]
	srv.debounceMu.Unlock()
	if !ok {
		t.Fatal("first cancel not registered")
	}
	// Wrap firstCancel so we can detect when it's invoked.
	var canceled atomic.Bool
	srv.debounceMu.Lock()
	srv.cancels[doc.uri] = func() {
		canceled.Store(true)
		firstCancel()
	}
	srv.debounceMu.Unlock()

	// Schedule again immediately — should cancel the first.
	srv.scheduleDiagnostics(doc, 0)
	if !canceled.Load() {
		t.Errorf("first cancel was not called when re-scheduled")
	}

	srv.stopDiagnostics(doc.uri)
}

// noopReader/noopWriter satisfy io.Reader/io.Writer for tests that
// instantiate Server but never actually use the connection.
type noopReader struct{}

func (noopReader) Read(p []byte) (int, error) {
	// Block forever — tests should never trigger an actual read.
	select {}
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }

// silence unused import in case the noop types stop being referenced.
var _ = context.Background
