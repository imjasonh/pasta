// Package protocol defines the subset of LSP types pastals uses on the
// wire. Field names match the LSP 3.17 spec exactly.
//
// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/.
package protocol

import "encoding/json"

// Position is an LSP zero-based (line, character) location. `Character`
// counts UTF-16 code units per the spec's default encoding.
type Position struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

// Range is a half-open [Start, End) span.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextDocumentIdentifier identifies an open document by URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier adds a monotonically-increasing version.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentItem is sent by the client on didOpen.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// ----------------------------------------------------------------------
// initialize / initialized
// ----------------------------------------------------------------------

// InitializeParams is what the client sends as the first request.
type InitializeParams struct {
	ProcessID             *int              `json:"processId"`
	RootURI               string            `json:"rootUri"`
	WorkspaceFolders      []WorkspaceFolder `json:"workspaceFolders,omitempty"`
	InitializationOptions *json.RawMessage  `json:"initializationOptions,omitempty"`
	Capabilities          json.RawMessage   `json:"capabilities"`
}

// WorkspaceFolder is one of the roots opened by the client.
type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// InitializeResult is the server's response.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerInfo identifies the server software/version.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ServerCapabilities advertises what features the server supports.
type ServerCapabilities struct {
	TextDocumentSync   int                    `json:"textDocumentSync"` // 1 = full
	CodeActionProvider *CodeActionOptions     `json:"codeActionProvider,omitempty"`
	Workspace          *WorkspaceCapabilities `json:"workspace,omitempty"`
}

// CodeActionOptions advertises which code-action kinds the server
// produces.
type CodeActionOptions struct {
	CodeActionKinds []string `json:"codeActionKinds,omitempty"`
}

// WorkspaceCapabilities is the workspace-related portion of server caps.
type WorkspaceCapabilities struct {
	WorkspaceFolders *WorkspaceFoldersOptions `json:"workspaceFolders,omitempty"`
	FileOperations   *FileOperationsOptions   `json:"fileOperations,omitempty"`
}

// WorkspaceFoldersOptions advertises workspace-folder support.
type WorkspaceFoldersOptions struct {
	Supported           bool `json:"supported"`
	ChangeNotifications bool `json:"changeNotifications,omitempty"`
}

// FileOperationsOptions is reserved for future expansion.
type FileOperationsOptions struct{}

// ----------------------------------------------------------------------
// textDocument/didOpen, didChange, didClose, didSave
// ----------------------------------------------------------------------

// DidOpenTextDocumentParams is the payload of textDocument/didOpen.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidChangeTextDocumentParams is the payload of textDocument/didChange.
// Pastals advertises full sync (TextDocumentSync=1), so each change
// event carries the entire new file text.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// TextDocumentContentChangeEvent is the full-sync flavor: just Text.
// Incremental sync (with Range) is not used.
type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

// DidCloseTextDocumentParams is the payload of textDocument/didClose.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidSaveTextDocumentParams is the payload of textDocument/didSave.
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

// ----------------------------------------------------------------------
// publishDiagnostics
// ----------------------------------------------------------------------

// PublishDiagnosticsParams is what the server pushes to the client.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Diagnostic is one published finding. `Data` carries pasta-specific
// context (rule name + fix ops) so the client can request a code
// action without us re-running.
type Diagnostic struct {
	Range    Range           `json:"range"`
	Severity int             `json:"severity"`
	Source   string          `json:"source,omitempty"`
	Message  string          `json:"message"`
	Data     *DiagnosticData `json:"data,omitempty"`
}

// LSP DiagnosticSeverity codes.
const (
	SeverityError       = 1
	SeverityWarning     = 2
	SeverityInformation = 3
	SeverityHint        = 4
)

// DiagnosticData is pasta's payload tucked into Diagnostic.Data.
type DiagnosticData struct {
	Rule string      `json:"rule"`
	Ops  []EncodedOp `json:"ops,omitempty"`
}

// EncodedOp is the JSON shape of effect.Op for transport.
type EncodedOp struct {
	Start uint32 `json:"start"`
	End   uint32 `json:"end"`
	Text  string `json:"text"`
}

// ----------------------------------------------------------------------
// textDocument/codeAction
// ----------------------------------------------------------------------

// CodeActionParams is the request payload.
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

// CodeActionContext narrows the request to specific diagnostics or
// kinds.
type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Only        []string     `json:"only,omitempty"`
}

// CodeAction is one offered action.
type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
}

// WorkspaceEdit is a set of TextEdits keyed by document URI.
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

// TextEdit replaces Range with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// CodeActionKind values pastals emits.
const (
	CodeActionKindQuickFix     = "quickfix"
	CodeActionKindSourceFixAll = "source.fixAll.pasta"
)

// ----------------------------------------------------------------------
// workspace/didChangeWatchedFiles
// ----------------------------------------------------------------------

// DidChangeWatchedFilesParams is the payload of the watched-files
// notification.
type DidChangeWatchedFilesParams struct {
	Changes []FileEvent `json:"changes"`
}

// FileEvent describes a single watched-file change.
type FileEvent struct {
	URI  string `json:"uri"`
	Type int    `json:"type"` // 1=Created, 2=Changed, 3=Deleted
}

// ----------------------------------------------------------------------
// client/registerCapability
// ----------------------------------------------------------------------

// RegistrationParams is the payload to register dynamic capabilities.
type RegistrationParams struct {
	Registrations []Registration `json:"registrations"`
}

// Registration is one dynamic capability registration.
type Registration struct {
	ID              string `json:"id"`
	Method          string `json:"method"`
	RegisterOptions any    `json:"registerOptions,omitempty"`
}

// DidChangeWatchedFilesRegistrationOptions is the options struct for
// the watched-files registration.
type DidChangeWatchedFilesRegistrationOptions struct {
	Watchers []FileSystemWatcher `json:"watchers"`
}

// FileSystemWatcher describes one file-watch glob pattern.
type FileSystemWatcher struct {
	GlobPattern string `json:"globPattern"`
	Kind        int    `json:"kind,omitempty"` // bitmask: 1=Create 2=Change 4=Delete; default = all
}

// ----------------------------------------------------------------------
// window/showMessage, window/logMessage
// ----------------------------------------------------------------------

// ShowMessageParams is the payload of window/showMessage and
// window/logMessage.
type ShowMessageParams struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}

// MessageType codes.
const (
	MessageTypeError   = 1
	MessageTypeWarning = 2
	MessageTypeInfo    = 3
	MessageTypeLog     = 4
)
