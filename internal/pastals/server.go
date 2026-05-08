// Package pastals implements the pasta language server. Server.Run is
// the entry point; cmd/pastals just wires it up to stdio.
package pastals

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/pastals/jsonrpc"
	"github.com/imjasonh/pasta/internal/pastals/protocol"
	"github.com/imjasonh/pasta/internal/runner"
)

// Server holds all shared state for the LSP. One per process —
// pastals is invoked fresh per editor session.
type Server struct {
	conn *jsonrpc.Conn
	docs *docStore

	mu          sync.RWMutex
	cfg         Config
	root        string
	rules       []string // currently loaded rule files
	analyzerSet []*dsl.Analyzer

	// Per-document debouncer. Each in-flight diagnostic run can be
	// canceled when a newer didChange arrives.
	debounceMu sync.Mutex
	cancels    map[string]context.CancelFunc
	timers     map[string]*time.Timer

	shuttingDown bool
}

// New constructs a server reading from in and writing to out.
func New(in io.Reader, out io.Writer) *Server {
	return &Server{
		conn:    jsonrpc.NewConn(in, out),
		docs:    newDocStore(),
		cfg:     DefaultConfig(),
		cancels: map[string]context.CancelFunc{},
		timers:  map[string]*time.Timer{},
	}
}

// Run is the message-dispatch loop. It returns when the input stream
// closes (after `exit`) or on a fatal error.
func (s *Server) Run(ctx context.Context) error {
	for {
		msg, err := s.conn.Read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		s.dispatch(ctx, msg)
	}
}

// dispatch routes one message. Notifications execute inline (LSP
// requires in-order processing for state-modifying notifications like
// didChange). Requests run in goroutines so a slow request handler
// can't stall notifications.
func (s *Server) dispatch(ctx context.Context, m jsonrpc.Message) {
	if m.ID == nil {
		s.handleNotification(ctx, m)
		return
	}
	go s.handleRequest(ctx, m)
}

func (s *Server) handleNotification(ctx context.Context, m jsonrpc.Message) {
	switch m.Method {
	case "initialized":
		s.handleInitialized(ctx)
	case "textDocument/didOpen":
		var p protocol.DidOpenTextDocumentParams
		if err := json.Unmarshal(m.Params, &p); err == nil {
			s.handleDidOpen(ctx, p)
		}
	case "textDocument/didChange":
		var p protocol.DidChangeTextDocumentParams
		if err := json.Unmarshal(m.Params, &p); err == nil {
			s.handleDidChange(ctx, p)
		}
	case "textDocument/didSave":
		var p protocol.DidSaveTextDocumentParams
		if err := json.Unmarshal(m.Params, &p); err == nil {
			s.handleDidSave(ctx, p)
		}
	case "textDocument/didClose":
		var p protocol.DidCloseTextDocumentParams
		if err := json.Unmarshal(m.Params, &p); err == nil {
			s.handleDidClose(ctx, p)
		}
	case "workspace/didChangeWatchedFiles":
		var p protocol.DidChangeWatchedFilesParams
		if err := json.Unmarshal(m.Params, &p); err == nil {
			s.handleDidChangeWatched(ctx, p)
		}
	case "exit":
		// EOF closes the loop; nothing to do here.
	default:
		// Unknown notifications are ignored per LSP.
	}
}

func (s *Server) handleRequest(ctx context.Context, m jsonrpc.Message) {
	switch m.Method {
	case "initialize":
		var p protocol.InitializeParams
		if err := json.Unmarshal(m.Params, &p); err != nil {
			_ = s.conn.ReplyErr(m.ID, jsonrpc.CodeInvalidParams, err.Error())
			return
		}
		_ = s.conn.Reply(m.ID, s.handleInitialize(p))
	case "shutdown":
		s.shuttingDown = true
		_ = s.conn.Reply(m.ID, nil)
	case "textDocument/codeAction":
		var p protocol.CodeActionParams
		if err := json.Unmarshal(m.Params, &p); err != nil {
			_ = s.conn.ReplyErr(m.ID, jsonrpc.CodeInvalidParams, err.Error())
			return
		}
		_ = s.conn.Reply(m.ID, s.handleCodeAction(ctx, p))
	default:
		_ = s.conn.ReplyErr(m.ID, jsonrpc.CodeMethodNotFound, "unknown method "+m.Method)
	}
}

// ---------------------------------------------------------------------
// Initialize / initialized
// ---------------------------------------------------------------------

func (s *Server) handleInitialize(p protocol.InitializeParams) protocol.InitializeResult {
	cfg, err := parseInitOptions(p.InitializationOptions)
	if err != nil {
		s.logf("initializationOptions: %v", err)
	}

	root := ""
	if len(p.WorkspaceFolders) > 0 {
		if rp, err := uriToPath(p.WorkspaceFolders[0].URI); err == nil {
			root = rp
		}
	}
	if root == "" && p.RootURI != "" {
		if rp, err := uriToPath(p.RootURI); err == nil {
			root = rp
		}
	}
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}

	s.mu.Lock()
	s.cfg = cfg
	s.root = root
	s.mu.Unlock()

	s.reloadRules()

	return protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: 1, // full
			CodeActionProvider: &protocol.CodeActionOptions{
				CodeActionKinds: []string{
					protocol.CodeActionKindQuickFix,
					protocol.CodeActionKindSourceFixAll,
				},
			},
			Workspace: &protocol.WorkspaceCapabilities{
				WorkspaceFolders: &protocol.WorkspaceFoldersOptions{
					Supported:           true,
					ChangeNotifications: true,
				},
			},
		},
		ServerInfo: &protocol.ServerInfo{Name: "pastals", Version: "0.1.0"},
	}
}

// handleInitialized fires after the client acks initialize. We use it
// to register file watchers for the rule globs.
func (s *Server) handleInitialized(ctx context.Context) {
	s.registerRuleWatcher()
}

// ---------------------------------------------------------------------
// Document lifecycle
// ---------------------------------------------------------------------

func (s *Server) handleDidOpen(ctx context.Context, p protocol.DidOpenTextDocumentParams) {
	path, err := uriToPath(p.TextDocument.URI)
	if err != nil {
		s.logf("didOpen: %v", err)
		return
	}
	d := &document{uri: p.TextDocument.URI, path: path}
	d.update(p.TextDocument.Version, p.TextDocument.Text)
	s.docs.put(d)
	s.scheduleDiagnostics(d, 0)
}

func (s *Server) handleDidChange(ctx context.Context, p protocol.DidChangeTextDocumentParams) {
	d, ok := s.docs.get(p.TextDocument.URI)
	if !ok {
		return
	}
	if len(p.ContentChanges) == 0 {
		return
	}
	// Full-sync: last entry's Text is the new full document.
	d.update(p.TextDocument.Version, p.ContentChanges[len(p.ContentChanges)-1].Text)
	s.scheduleDiagnostics(d, 0)
}

func (s *Server) handleDidSave(ctx context.Context, p protocol.DidSaveTextDocumentParams) {
	if !s.cfg.RunOnSave {
		return // already running on change
	}
	d, ok := s.docs.get(p.TextDocument.URI)
	if !ok {
		return
	}
	if p.Text != nil {
		d.update(d.version, *p.Text)
	}
	s.scheduleDiagnostics(d, 0)
}

func (s *Server) handleDidClose(ctx context.Context, p protocol.DidCloseTextDocumentParams) {
	s.stopDiagnostics(p.TextDocument.URI)
	s.docs.delete(p.TextDocument.URI)
	// Clear diagnostics for the closed document.
	s.publishDiagnostics(p.TextDocument.URI, 0, nil)
}

// ---------------------------------------------------------------------
// Debounced diagnostic dispatch
// ---------------------------------------------------------------------

// scheduleDiagnostics schedules a diagnostic run for d, debounced by
// cfg.DebounceMS. If a run is already pending or in flight for d.uri,
// it is canceled and replaced. The replacement's ctx is fresh; the old
// run drops its publish when it sees ctx.Err.
func (s *Server) scheduleDiagnostics(d *document, delay time.Duration) {
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()

	if cancel, ok := s.cancels[d.uri]; ok {
		cancel()
	}
	if t, ok := s.timers[d.uri]; ok {
		t.Stop()
	}

	if delay == 0 {
		delay = time.Duration(s.cfg.DebounceMS) * time.Millisecond
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancels[d.uri] = cancel
	s.timers[d.uri] = time.AfterFunc(delay, func() {
		s.runDiagnostics(ctx, d)
	})
}

// stopDiagnostics cancels any pending or in-flight run for uri.
// Called from handleDidClose so closed documents leave no live
// goroutines behind.
func (s *Server) stopDiagnostics(uri string) {
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()
	if cancel, ok := s.cancels[uri]; ok {
		cancel()
		delete(s.cancels, uri)
	}
	if t, ok := s.timers[uri]; ok {
		t.Stop()
		delete(s.timers, uri)
	}
}

// ---------------------------------------------------------------------
// Rule loading + file-watch reload
// ---------------------------------------------------------------------

// reloadRules re-resolves the configured rule globs and reloads the
// analyzer set. On error, posts a window/showMessage warning and
// leaves the previous analyzer set intact.
func (s *Server) reloadRules() {
	s.mu.RLock()
	cfg := s.cfg
	root := s.root
	s.mu.RUnlock()

	files := resolveRuleFiles(cfg, root)
	if len(files) == 0 {
		s.logf("no rule files matched; serving with empty analyzer set")
		s.mu.Lock()
		s.rules = nil
		s.analyzerSet = nil
		s.mu.Unlock()
		return
	}

	// internal/runner.LoadRules expects a directory containing *.cue files,
	// but our globs may span multiple directories. Group by directory
	// and load each separately, then merge.
	byDir := map[string][]string{}
	for _, f := range files {
		dir := filepath.Dir(f)
		byDir[dir] = append(byDir[dir], f)
	}

	var allAnalyzers []*dsl.Analyzer
	for dir := range byDir {
		analyzers, err := runner.LoadRules(dir)
		if err != nil {
			s.showMessagef(protocol.MessageTypeWarning, "pasta: %v", err)
			continue
		}
		allAnalyzers = append(allAnalyzers, analyzers...)
	}

	s.mu.Lock()
	s.rules = files
	s.analyzerSet = allAnalyzers
	s.mu.Unlock()

	s.logf("loaded %d analyzer(s) from %d rule file(s)", len(allAnalyzers), len(files))

	// Re-run diagnostics on every open document.
	for _, d := range s.docs.all() {
		s.scheduleDiagnostics(d, 0)
	}
}

// analyzers returns a snapshot of the currently-loaded analyzer set.
func (s *Server) analyzers() []*dsl.Analyzer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*dsl.Analyzer, len(s.analyzerSet))
	copy(out, s.analyzerSet)
	return out
}

// registerRuleWatcher asks the client to notify us of changes to any
// configured rule glob via workspace/didChangeWatchedFiles.
func (s *Server) registerRuleWatcher() {
	s.mu.RLock()
	patterns := make([]string, len(s.cfg.Rules))
	copy(patterns, s.cfg.Rules)
	s.mu.RUnlock()

	watchers := make([]protocol.FileSystemWatcher, len(patterns))
	for i, p := range patterns {
		watchers[i] = protocol.FileSystemWatcher{GlobPattern: p}
	}
	if err := s.conn.Notify("client/registerCapability", protocol.RegistrationParams{
		Registrations: []protocol.Registration{{
			ID:     "pasta-rule-watcher",
			Method: "workspace/didChangeWatchedFiles",
			RegisterOptions: protocol.DidChangeWatchedFilesRegistrationOptions{
				Watchers: watchers,
			},
		}},
	}); err != nil {
		s.logf("registerCapability: %v", err)
	}
}

func (s *Server) handleDidChangeWatched(ctx context.Context, p protocol.DidChangeWatchedFilesParams) {
	if len(p.Changes) == 0 {
		return
	}
	s.reloadRules()
}

// ---------------------------------------------------------------------
// Logging helpers
// ---------------------------------------------------------------------

func (s *Server) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, "pastals:", msg)
	if s.conn == nil {
		return
	}
	_ = s.conn.Notify("window/logMessage", protocol.ShowMessageParams{
		Type:    protocol.MessageTypeLog,
		Message: msg,
	})
}

func (s *Server) showMessagef(typ int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if s.conn == nil {
		return
	}
	_ = s.conn.Notify("window/showMessage", protocol.ShowMessageParams{
		Type:    typ,
		Message: msg,
	})
}
