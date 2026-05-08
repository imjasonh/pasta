// Package jsonrpc implements LSP-style JSON-RPC framing over a byte
// stream: `Content-Length: N\r\n\r\n<body>` repeated.
//
// The package knows about request, response, and notification framing
// but nothing about LSP types. Higher layers decode Params and Result
// as they see fit.
package jsonrpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Message is the union of JSON-RPC request, response, and notification.
// Which it is is decided by which fields are populated:
//
//	request:      ID + Method + Params
//	response:     ID + (Result or Error)
//	notification: Method + Params (no ID)
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is the JSON-RPC error response payload.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC / LSP error codes.
const (
	CodeParseError      = -32700
	CodeInvalidRequest  = -32600
	CodeMethodNotFound  = -32601
	CodeInvalidParams   = -32602
	CodeInternalError   = -32603
	CodeRequestCanceled = -32800
)

// Conn is a framed JSON-RPC connection over the given reader/writer.
// Concurrent writes from many goroutines are safe; reads must be
// serialized by the caller (typically the dispatch loop).
type Conn struct {
	in  *bufio.Reader
	out io.Writer
	wmu sync.Mutex
}

// NewConn wraps in/out as a JSON-RPC connection.
func NewConn(in io.Reader, out io.Writer) *Conn {
	return &Conn{in: bufio.NewReader(in), out: out}
}

// Read consumes one framed message. Returns io.EOF on clean stream end.
func (c *Conn) Read() (Message, error) {
	contentLen := -1
	for {
		line, err := c.in.ReadString('\n')
		if err != nil {
			return Message{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return Message{}, fmt.Errorf("bad Content-Length: %w", err)
			}
			contentLen = n
		}
	}
	if contentLen < 0 {
		return Message{}, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLen)
	if _, err := io.ReadFull(c.in, body); err != nil {
		return Message{}, err
	}
	var m Message
	if err := json.Unmarshal(body, &m); err != nil {
		return Message{}, fmt.Errorf("decode body: %w", err)
	}
	return m, nil
}

// Write frames and emits one message. JSONRPC defaults to "2.0".
func (c *Conn) Write(m Message) error {
	if m.JSONRPC == "" {
		m.JSONRPC = "2.0"
	}
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := fmt.Fprintf(c.out, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.out.Write(body)
	return err
}

// Reply sends a successful response to an inbound request.
func (c *Conn) Reply(id json.RawMessage, result any) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return c.Write(Message{ID: id, Result: raw})
}

// ReplyErr sends an error response.
func (c *Conn) ReplyErr(id json.RawMessage, code int, msg string) error {
	return c.Write(Message{ID: id, Error: &Error{Code: code, Message: msg}})
}

// Notify sends an outbound notification (no ID, no response expected).
func (c *Conn) Notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.Write(Message{Method: method, Params: raw})
}
