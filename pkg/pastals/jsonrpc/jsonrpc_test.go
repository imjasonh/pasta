package jsonrpc

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestReadFraming(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"x":1}}`
	stream := "Content-Length: " + itoa(len(body)) + "\r\n\r\n" + body
	c := NewConn(strings.NewReader(stream), io.Discard)
	m, err := c.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if m.Method != "hello" {
		t.Errorf("Method = %q, want hello", m.Method)
	}
	if string(m.ID) != "1" {
		t.Errorf("ID = %s, want 1", string(m.ID))
	}
	var p struct{ X int }
	if err := json.Unmarshal(m.Params, &p); err != nil || p.X != 1 {
		t.Errorf("Params decode: p=%+v err=%v", p, err)
	}
}

func TestReadMultipleMessages(t *testing.T) {
	a := `{"jsonrpc":"2.0","method":"a"}`
	b := `{"jsonrpc":"2.0","method":"b"}`
	stream := "Content-Length: " + itoa(len(a)) + "\r\n\r\n" + a +
		"Content-Length: " + itoa(len(b)) + "\r\n\r\n" + b
	c := NewConn(strings.NewReader(stream), io.Discard)
	first, err := c.Read()
	if err != nil || first.Method != "a" {
		t.Fatalf("first: method=%q err=%v", first.Method, err)
	}
	second, err := c.Read()
	if err != nil || second.Method != "b" {
		t.Fatalf("second: method=%q err=%v", second.Method, err)
	}
	if _, err := c.Read(); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestReadMissingHeader(t *testing.T) {
	c := NewConn(strings.NewReader("\r\n\r\n{}"), io.Discard)
	_, err := c.Read()
	if err == nil {
		t.Fatal("expected error for missing Content-Length")
	}
	if !strings.Contains(err.Error(), "Content-Length") {
		t.Errorf("error doesn't mention Content-Length: %v", err)
	}
}

func TestReadBadContentLength(t *testing.T) {
	c := NewConn(strings.NewReader("Content-Length: not-a-number\r\n\r\n{}"), io.Discard)
	_, err := c.Read()
	if err == nil {
		t.Fatal("expected error for bad Content-Length")
	}
}

func TestWriteFraming(t *testing.T) {
	var buf bytes.Buffer
	c := NewConn(strings.NewReader(""), &buf)
	if err := c.Write(Message{Method: "ping"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "Content-Length: ") {
		t.Errorf("missing Content-Length prefix: %q", out)
	}
	if !strings.Contains(out, "\r\n\r\n") {
		t.Errorf("missing header/body separator: %q", out)
	}
	if !strings.Contains(out, `"method":"ping"`) {
		t.Errorf("body missing method: %q", out)
	}
	if !strings.Contains(out, `"jsonrpc":"2.0"`) {
		t.Errorf("default jsonrpc=2.0 not applied: %q", out)
	}
}

func TestRoundTripVariousIDTypes(t *testing.T) {
	cases := []struct {
		name string
		id   json.RawMessage
	}{
		{"int", json.RawMessage(`42`)},
		{"string", json.RawMessage(`"abc"`)},
		{"null", json.RawMessage(`null`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			c := NewConn(strings.NewReader(""), &buf)
			if err := c.Write(Message{ID: tc.id, Method: "test"}); err != nil {
				t.Fatal(err)
			}
			r := NewConn(&buf, io.Discard)
			m, err := r.Read()
			if err != nil {
				t.Fatal(err)
			}
			if string(m.ID) != string(tc.id) {
				t.Errorf("ID = %s, want %s", string(m.ID), string(tc.id))
			}
		})
	}
}

func TestReplyAndError(t *testing.T) {
	var buf bytes.Buffer
	c := NewConn(strings.NewReader(""), &buf)

	if err := c.Reply(json.RawMessage(`5`), map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if err := c.ReplyErr(json.RawMessage(`6`), CodeMethodNotFound, "nope"); err != nil {
		t.Fatal(err)
	}

	r := NewConn(&buf, io.Discard)
	good, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(good.ID) != "5" || good.Error != nil {
		t.Errorf("good response shape wrong: %+v", good)
	}
	if !strings.Contains(string(good.Result), `"n":1`) {
		t.Errorf("result missing: %s", string(good.Result))
	}

	bad, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if bad.Error == nil || bad.Error.Code != CodeMethodNotFound || bad.Error.Message != "nope" {
		t.Errorf("error response wrong: %+v", bad.Error)
	}
}

func TestNotifyHasNoID(t *testing.T) {
	var buf bytes.Buffer
	c := NewConn(strings.NewReader(""), &buf)
	if err := c.Notify("did/something", nil); err != nil {
		t.Fatal(err)
	}
	r := NewConn(&buf, io.Discard)
	m, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.ID) != 0 {
		t.Errorf("notification carried an ID: %s", string(m.ID))
	}
	if m.Method != "did/something" {
		t.Errorf("Method = %q, want did/something", m.Method)
	}
}

// itoa avoids strconv import for one usage.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
