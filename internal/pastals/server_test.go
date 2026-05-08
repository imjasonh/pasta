package pastals

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/imjasonh/pasta/internal/pastals/jsonrpc"
	"github.com/imjasonh/pasta/internal/pastals/protocol"
)

// TestEndToEnd spawns a server in-process, drives it through
// initialize → didOpen → publishDiagnostics, and verifies the
// published diagnostics correspond to the iferr analyzer's findings.
func TestEndToEnd(t *testing.T) {
	// Two pipes: c2s carries client→server, s2c carries server→client.
	c2sR, c2sW := io.Pipe() // server reads c2sR, client writes c2sW
	s2cR, s2cW := io.Pipe() // client reads s2cR, server writes s2cW
	srv := New(c2sR, s2cW)

	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Run(context.Background()) }()

	client := newTestClient(t, c2sW, s2cR)
	defer client.close()

	// Resolve the iferr analyzer dir so the server picks it up.
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	rulesGlob := filepath.Join(repoRoot, "analyzers/go_iferr/*.cue")

	rootURI := pathToFileURI(repoRoot)
	client.request(1, "initialize", protocol.InitializeParams{
		RootURI: rootURI,
		InitializationOptions: rawJSON(map[string]any{
			"rules":      []string{rulesGlob},
			"debounceMs": 1,
		}),
	})
	resp := client.expectResponse(1)
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}

	client.notify("initialized", struct{}{})

	src := `package p

import "errors"

func f() error {
	err := errors.New("x")
	if err != nil {
		return err
	}
	return nil
}
`
	uri := pathToFileURI(filepath.Join(repoRoot, "tmp_lsp_test.go"))
	client.notify("textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "go",
			Version:    1,
			Text:       src,
		},
	})

	deadline := time.Now().Add(10 * time.Second)
	var diagsParams protocol.PublishDiagnosticsParams
	for {
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for publishDiagnostics")
		}
		msg := client.readNext(t)
		if msg.Method == "textDocument/publishDiagnostics" {
			if err := json.Unmarshal(msg.Params, &diagsParams); err != nil {
				t.Fatalf("decode diagnostics: %v", err)
			}
			if diagsParams.URI == uri {
				break
			}
		}
	}

	if len(diagsParams.Diagnostics) == 0 {
		t.Fatalf("expected at least one diagnostic, got 0")
	}
	got := diagsParams.Diagnostics[0]
	if !strings.Contains(got.Source, "inline_") {
		t.Errorf("source = %q, want substring 'inline_'", got.Source)
	}
	if got.Severity != protocol.SeverityWarning {
		t.Errorf("severity = %d, want warning(2)", got.Severity)
	}
	if got.Data == nil || len(got.Data.Ops) == 0 {
		t.Errorf("expected fix ops in diagnostic data, got %+v", got.Data)
	}

	// Code-action handshake: ask for quickfixes covering the diagnostic.
	client.request(2, "textDocument/codeAction", protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range:        got.Range,
		Context:      protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{got}},
	})
	caResp := client.expectResponse(2)
	if caResp.Error != nil {
		t.Fatalf("codeAction error: %v", caResp.Error)
	}
	var actions []protocol.CodeAction
	if err := json.Unmarshal(caResp.Result, &actions); err != nil {
		t.Fatalf("decode codeAction result: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("expected at least one code action")
	}
	hasQuickfix := false
	for _, a := range actions {
		if a.Kind == protocol.CodeActionKindQuickFix {
			hasQuickfix = true
			if a.Edit == nil || len(a.Edit.Changes[uri]) == 0 {
				t.Errorf("quickfix has no edits: %+v", a)
			}
		}
	}
	if !hasQuickfix {
		t.Errorf("no quickfix action returned; got: %+v", actions)
	}

	// Shut down.
	client.request(99, "shutdown", nil)
	client.expectResponse(99)
	client.notify("exit", struct{}{})
	c2sW.Close()
	s2cR.Close()

	select {
	case <-srvDone:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not exit")
	}
}

// ---------- test client (minimal LSP RPC over a pair of pipes) ----------

// The reader runs in a goroutine and pushes every inbound message into
// `incoming`. This mirrors what real LSP clients do — failure to drain
// the server's writes concurrently with sending requests deadlocks
// when the server emits a notification mid-handler (e.g.
// client/registerCapability fired from `initialized`).

type testClient struct {
	t        *testing.T
	w        io.WriteCloser
	r        *bufio.Reader
	incoming chan jsonrpc.Message
	closed   bool
}

func newTestClient(t *testing.T, w io.WriteCloser, r io.Reader) *testClient {
	c := &testClient{
		t:        t,
		w:        w,
		r:        bufio.NewReader(r),
		incoming: make(chan jsonrpc.Message, 64),
	}
	go c.readLoop()
	return c
}

func (c *testClient) readLoop() {
	for {
		m, err := readRPC(c.r)
		if err != nil {
			close(c.incoming)
			return
		}
		c.incoming <- m
	}
}

func (c *testClient) close() { c.closed = true }

func (c *testClient) write(m jsonrpc.Message) {
	if m.JSONRPC == "" {
		m.JSONRPC = "2.0"
	}
	body, err := json.Marshal(m)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		c.t.Fatal(err)
	}
	if _, err := c.w.Write(body); err != nil {
		c.t.Fatal(err)
	}
}

func (c *testClient) request(id int, method string, params any) {
	raw, err := json.Marshal(params)
	if err != nil {
		c.t.Fatal(err)
	}
	idRaw := json.RawMessage(strconv.Itoa(id))
	c.write(jsonrpc.Message{ID: idRaw, Method: method, Params: raw})
}

func (c *testClient) notify(method string, params any) {
	raw, err := json.Marshal(params)
	if err != nil {
		c.t.Fatal(err)
	}
	c.write(jsonrpc.Message{Method: method, Params: raw})
}

// readRPC parses one framed message from r.
func readRPC(r *bufio.Reader) (jsonrpc.Message, error) {
	contentLen := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return jsonrpc.Message{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return jsonrpc.Message{}, fmt.Errorf("bad Content-Length: %w", err)
			}
			contentLen = n
		}
	}
	if contentLen < 0 {
		return jsonrpc.Message{}, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return jsonrpc.Message{}, err
	}
	var m jsonrpc.Message
	if err := json.Unmarshal(body, &m); err != nil {
		return jsonrpc.Message{}, err
	}
	return m, nil
}

func (c *testClient) readNext(t *testing.T) jsonrpc.Message {
	select {
	case m, ok := <-c.incoming:
		if !ok {
			t.Fatal("server closed the stream")
		}
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("timed out reading from server")
	}
	return jsonrpc.Message{}
}

// expectResponse drains messages until a response matching id arrives.
// Notifications encountered along the way are discarded.
func (c *testClient) expectResponse(id int) jsonrpc.Message {
	want := strconv.Itoa(id)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		msg := c.readNext(c.t)
		if msg.ID != nil && string(msg.ID) == want {
			return msg
		}
	}
	c.t.Fatalf("timed out waiting for response id=%d", id)
	return jsonrpc.Message{}
}

// ---------- helpers ----------

func rawJSON(v any) *json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	r := json.RawMessage(b)
	return &r
}

func pathToFileURI(p string) string {
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err == nil {
			p = abs
		}
	}
	u := &url.URL{Scheme: "file", Path: p}
	return u.String()
}
