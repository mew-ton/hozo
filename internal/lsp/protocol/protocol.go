// Package protocol は Language Server Protocol 3.17 のうち hozoc lsp が使う型（docs/lsp.md「通信」の
// 「要求の型は使うものだけを手で定義する」）。フィールド名と JSON のキーは仕様のまま。
// 使わない省略可能なフィールドは持たない。
package protocol

import "encoding/json"

// メソッド名。
const (
	MethodInitialize             = "initialize"
	MethodInitialized            = "initialized"
	MethodShutdown               = "shutdown"
	MethodExit                   = "exit"
	MethodCancelRequest          = "$/cancelRequest"
	MethodDidOpen                = "textDocument/didOpen"
	MethodDidChange              = "textDocument/didChange"
	MethodDidClose               = "textDocument/didClose"
	MethodDidSave                = "textDocument/didSave"
	MethodDidChangeWatchedFiles  = "workspace/didChangeWatchedFiles"
	MethodDidChangeConfiguration = "workspace/didChangeConfiguration"
	MethodPublishDiagnostics     = "textDocument/publishDiagnostics"
	MethodDefinition             = "textDocument/definition"
	MethodHover                  = "textDocument/hover"
	MethodDocumentSymbol         = "textDocument/documentSymbol"
	MethodRegisterCapability     = "client/registerCapability"
	MethodLogMessage             = "window/logMessage"
	// MethodVocabulary は hozoc 独自の要求（docs/lsp.md「プロジェクト」）。`hozo:` の URI の内容を返す。
	MethodVocabulary = "hozo/vocabulary"
)

// 位置のエンコーディング（docs/lsp.md「通信」）。
const (
	PositionEncodingUTF8  = "utf-8"
	PositionEncodingUTF16 = "utf-16"
)

// Position は 0 始まりの行と、エンコーディングで数えた 0 始まりの桁。
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range は位置の範囲。End は含まない。
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location は文書の中の範囲。
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier は文書の URI。
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier は版付きの文書の URI。
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentItem は開いた文書。
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// TextDocumentPositionParams は文書の中の位置を指す要求の params。
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// WorkspaceFolder はワークスペースのフォルダ。
type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// ClientCapabilities はクライアントの capabilities のうち見るもの。
type ClientCapabilities struct {
	Workspace *WorkspaceClientCapabilities `json:"workspace,omitempty"`
	General   *GeneralClientCapabilities   `json:"general,omitempty"`
}

// WorkspaceClientCapabilities はワークスペースに関する capabilities のうち見るもの。
type WorkspaceClientCapabilities struct {
	DidChangeWatchedFiles *DynamicRegistration `json:"didChangeWatchedFiles,omitempty"`
}

// DynamicRegistration は client/registerCapability で登録できるか。
type DynamicRegistration struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// GeneralClientCapabilities は general の capabilities のうち見るもの。
type GeneralClientCapabilities struct {
	// PositionEncodings はクライアントが受け付ける位置のエンコーディング。省略は ["utf-16"]。
	PositionEncodings []string `json:"positionEncodings,omitempty"`
}

// InitializeParams は initialize の params。
type InitializeParams struct {
	ProcessID        *int               `json:"processId"`
	RootPath         *string            `json:"rootPath,omitempty"`
	RootURI          *string            `json:"rootUri"`
	Capabilities     ClientCapabilities `json:"capabilities"`
	WorkspaceFolders []WorkspaceFolder  `json:"workspaceFolders,omitempty"`
	// InitializationOptions は、サーバーごとの設定（docs/lsp.md「プラグインの診断」の「許し」）。形は、読む側が確かめる。
	InitializationOptions json.RawMessage `json:"initializationOptions,omitempty"`
}

// InitializeResult は initialize の result。
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerInfo はサーバーの名前と版。
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ServerCapabilities はサーバーの capabilities（docs/lsp.md「ライフサイクル」）。
type ServerCapabilities struct {
	PositionEncoding       string                   `json:"positionEncoding,omitempty"`
	TextDocumentSync       *TextDocumentSyncOptions `json:"textDocumentSync,omitempty"`
	DefinitionProvider     bool                     `json:"definitionProvider,omitempty"`
	HoverProvider          bool                     `json:"hoverProvider,omitempty"`
	DocumentSymbolProvider bool                     `json:"documentSymbolProvider,omitempty"`
}

// TextDocumentSyncKind は didChange で送られる内容の形。
type TextDocumentSyncKind int

// TextDocumentSyncKind の値。
const (
	SyncNone        TextDocumentSyncKind = 0
	SyncFull        TextDocumentSyncKind = 1
	SyncIncremental TextDocumentSyncKind = 2
)

// TextDocumentSyncOptions は textDocumentSync。
type TextDocumentSyncOptions struct {
	OpenClose bool                 `json:"openClose"`
	Change    TextDocumentSyncKind `json:"change"`
	Save      *SaveOptions         `json:"save,omitempty"`
}

// SaveOptions は didSave の設定。
type SaveOptions struct {
	IncludeText bool `json:"includeText"`
}

// DidOpenTextDocumentParams は textDocument/didOpen の params。
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidChangeTextDocumentParams は textDocument/didChange の params。
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// TextDocumentContentChangeEvent は一つの変更。Range が nil なら全文。
type TextDocumentContentChangeEvent struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

// DidCloseTextDocumentParams は textDocument/didClose の params。
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidSaveTextDocumentParams は textDocument/didSave の params。
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

// FileChangeType は監視したファイルの変化の種類。
type FileChangeType int

// FileChangeType の値。
const (
	FileCreated FileChangeType = 1
	FileChanged FileChangeType = 2
	FileDeleted FileChangeType = 3
)

// FileEvent は監視したファイルの一つの変化。
type FileEvent struct {
	URI  string         `json:"uri"`
	Type FileChangeType `json:"type"`
}

// DidChangeConfigurationParams は workspace/didChangeConfiguration の params。settings の形は、読む側が確かめる
// （docs/lsp.md「プラグインの診断」の「許し」）。
type DidChangeConfigurationParams struct {
	Settings json.RawMessage `json:"settings"`
}

// DidChangeWatchedFilesParams は workspace/didChangeWatchedFiles の params。
type DidChangeWatchedFilesParams struct {
	Changes []FileEvent `json:"changes"`
}

// Registration は client/registerCapability の一つの登録。
type Registration struct {
	ID              string `json:"id"`
	Method          string `json:"method"`
	RegisterOptions any    `json:"registerOptions,omitempty"`
}

// RegistrationParams は client/registerCapability の params。
type RegistrationParams struct {
	Registrations []Registration `json:"registrations"`
}

// WatchKind は監視する変化の種類のビット。
type WatchKind int

// WatchKind の値。
const (
	WatchCreate WatchKind = 1
	WatchChange WatchKind = 2
	WatchDelete WatchKind = 4
)

// FileSystemWatcher は監視する glob。
type FileSystemWatcher struct {
	GlobPattern string    `json:"globPattern"`
	Kind        WatchKind `json:"kind,omitempty"`
}

// DidChangeWatchedFilesRegistrationOptions は didChangeWatchedFiles の登録の options。
type DidChangeWatchedFilesRegistrationOptions struct {
	Watchers []FileSystemWatcher `json:"watchers"`
}

// DiagnosticSeverity は診断の重さ（docs/lsp.md「診断」: Error → 1、Warning → 2）。
type DiagnosticSeverity int

// DiagnosticSeverity の値。
const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// Diagnostic は LSP の診断（docs/lsp.md「診断」の写し先）。
type Diagnostic struct {
	Range              Range                          `json:"range"`
	Severity           DiagnosticSeverity             `json:"severity,omitempty"`
	Code               string                         `json:"code,omitempty"`
	Source             string                         `json:"source,omitempty"`
	Message            string                         `json:"message"`
	RelatedInformation []DiagnosticRelatedInformation `json:"relatedInformation,omitempty"`
	Data               json.RawMessage                `json:"data,omitempty"`
}

// DiagnosticRelatedInformation は関連する位置（hozoc の Related）。
type DiagnosticRelatedInformation struct {
	Location Location `json:"location"`
	Message  string   `json:"message"`
}

// PublishDiagnosticsParams は textDocument/publishDiagnostics の params。Diagnostics は空でも配列。
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// MarkupKind は MarkupContent の種類。
type MarkupKind string

// MarkupKind の値。
const (
	PlainText MarkupKind = "plaintext"
	Markdown  MarkupKind = "markdown"
)

// MarkupContent はホバーの中身。
type MarkupContent struct {
	Kind  MarkupKind `json:"kind"`
	Value string     `json:"value"`
}

// Hover は textDocument/hover の result。
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// DocumentSymbolParams は textDocument/documentSymbol の params。
type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// SymbolKind はシンボルの種類（docs/lsp.md「ドキュメントのシンボル」の表が使うもの）。
type SymbolKind int

// SymbolKind の値（LSP 3.17 の番号）。
const (
	SymbolNamespace     SymbolKind = 3
	SymbolClass         SymbolKind = 5
	SymbolProperty      SymbolKind = 7
	SymbolField         SymbolKind = 8
	SymbolEnum          SymbolKind = 10
	SymbolInterface     SymbolKind = 11
	SymbolFunction      SymbolKind = 12
	SymbolVariable      SymbolKind = 13
	SymbolKey           SymbolKind = 20
	SymbolEnumMember    SymbolKind = 22
	SymbolStruct        SymbolKind = 23
	SymbolEvent         SymbolKind = 24
	SymbolTypeParameter SymbolKind = 26
)

// DocumentSymbol は入れ子のシンボル。
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// VocabularyParams は hozo/vocabulary の params（docs/lsp.md「プロジェクト」）。
type VocabularyParams struct {
	URI string `json:"uri"`
}

// VocabularyResult は hozo/vocabulary の result。
type VocabularyResult struct {
	Content string `json:"content"`
}
