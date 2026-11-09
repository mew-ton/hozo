package lsp

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/mew-ton/hozo/internal/lsp/jsonrpc"
	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/internal/vocab"
)

// Options は Serve の設定（docs/packages.md「internal/lsp」）。
type Options struct {
	// Root はクライアントが rootUri も workspaceFolders も送らなかったときのワークスペース。空なら作業ディレクトリ。
	Root string
	// ConfigPath は --config / HOZO_CONFIG の hozo.toml（docs/cli.md「lsp」）。空ならワークスペースから親へ探す
	// （docs/lsp.md「プロジェクト」）。相対パスならワークスペースからの相対。
	ConfigPath string
	// Log はログの書き出し先（--log）。nil なら捨てる。標準エラーには書かない（docs/lsp.md「通信」）。
	Log io.Writer
}

// debounceDelay は didChange から解析し直すまでの待ち時間（docs/lsp.md「プロジェクト」の 150 ms）。
// テストが短くするために変数。
var debounceDelay = 150 * time.Millisecond

// Serve は in から要求を読み、out に応答と通知を書く言語サーバーを、クライアントが exit を送るか in が
// 閉じるまで動かす（docs/lsp.md）。要求は一つの goroutine が順に処理し、didChange の待ち合わせの
// タイマーも同じ goroutine が見る。戻り値は常に nil（docs/lsp.md「ライフサイクル」: 終了コードは常に 0）で、
// 読み書きの誤りはログに残して終わる。
func Serve(in io.Reader, out io.Writer, opts Options) error {
	s := newServer(in, out, opts)
	s.run()
	return nil
}

// server は一つの接続の状態。run の goroutine だけが触る。
type server struct {
	reader *jsonrpc.Reader
	writer *jsonrpc.Writer
	log    *log.Logger
	logW   io.Writer
	opts   Options

	initialized bool
	shutdown    bool
	enc         encoding
	// root はワークスペースのディレクトリ（絶対パス）。hozo.toml を探し始める場所。
	root string
	// dynamicWatch はクライアントが didChangeWatchedFiles の動的な登録を受け付けるか。
	dynamicWatch bool
	nextID       int64

	// overlay は開いている文書の最新の内容。キーは絶対パス（project.Options.Overlay）。
	overlay map[string][]byte
	// docs は開いている文書の URI → overlay のキー。
	docs map[string]string
	// proj は直近の解析。initialized の前は nil。
	proj *project.Project
	// fileIDs は proj.Files のパス → FileID。
	fileIDs map[string]source.FileID
	// asts は要求のために構文解析し直した構文木（ドキュメントのシンボル、宣言の位置）。解析ごとに捨てる。
	asts map[source.FileID]*ast.File
	// declIdx は宣言の名前の位置の索引、pendingTags と docRefIdx はドキュメンテーションコメントのタグの索引
	// （locate.go）。解析ごとに捨てる。
	declIdx     map[source.FileID][]declEntry
	pendingTags map[source.FileID][]pendingTag
	docRefIdx   map[source.FileID][]declEntry
	// declNS は宣言の FQN → 宣言した名前空間（hover.go の修飾）。解析ごとに作り直す。
	declNS map[string]string
	// published は URI ごとに最後に送った診断の JSON。同じなら送らない（docs/lsp.md「プロジェクト」）。
	published map[string]string

	// timer は didChange の待ち合わせ。pending が true なら動いている。
	timer   *time.Timer
	pending bool
}

func newServer(in io.Reader, out io.Writer, opts Options) *server {
	logW := opts.Log
	if logW == nil {
		logW = io.Discard
	}
	return &server{
		reader:    jsonrpc.NewReader(in),
		writer:    jsonrpc.NewWriter(out),
		log:       log.New(logW, "", log.Ltime|log.Lmicroseconds),
		logW:      logW,
		opts:      opts,
		enc:       encUTF16,
		overlay:   map[string][]byte{},
		docs:      map[string]string{},
		published: map[string]string{},
	}
}

type incoming struct {
	msg *jsonrpc.Message
	err error
}

// run はメッセージを読む goroutine を一つ立て、自分はメッセージとタイマーを一つずつ処理する。
func (s *server) run() {
	msgs := make(chan incoming)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			m, err := s.reader.ReadMessage()
			select {
			case msgs <- incoming{m, err}:
			case <-done:
				return
			}
			var rpcErr *jsonrpc.Error
			if err != nil && !errors.As(err, &rpcErr) {
				return // 入力の終わりか壊れたヘッダ
			}
		}
	}()
	for {
		select {
		case in := <-msgs:
			if in.err != nil {
				var rpcErr *jsonrpc.Error
				if errors.As(in.err, &rpcErr) {
					s.send(jsonrpc.NewErrorResponse(jsonrpc.ID{}, rpcErr))
					continue
				}
				if in.err != io.EOF {
					s.log.Printf("read: %v", in.err)
				}
				s.stopTimer()
				return
			}
			if s.handle(in.msg) {
				s.stopTimer()
				return
			}
		case <-s.timerC():
			s.pending = false
			s.reanalyze()
		}
	}
}

// send はメッセージを書く。書けなければログに残す。
func (s *server) send(m *jsonrpc.Message) {
	if err := s.writer.WriteMessage(m); err != nil {
		s.log.Printf("write: %v", err)
	}
}

// reply は要求に result で答える。
func (s *server) reply(id jsonrpc.ID, result any) {
	m, err := jsonrpc.NewResponse(id, result)
	if err != nil {
		s.replyError(id, jsonrpc.NewError(jsonrpc.CodeInternalError, "%v", err))
		return
	}
	s.send(m)
}

// replyError は要求に誤りで答える。
func (s *server) replyError(id jsonrpc.ID, e *jsonrpc.Error) {
	s.send(jsonrpc.NewErrorResponse(id, e))
}

// notify は通知を送る。
func (s *server) notify(method string, params any) {
	m, err := jsonrpc.NewNotification(method, params)
	if err != nil {
		s.log.Printf("notify %s: %v", method, err)
		return
	}
	s.send(m)
}

// request はクライアントへ要求を送る。応答は handle が受けてログに残すだけ。
func (s *server) request(method string, params any) {
	s.nextID++
	m, err := jsonrpc.NewRequest(jsonrpc.NumberID(s.nextID), method, params)
	if err != nil {
		s.log.Printf("request %s: %v", method, err)
		return
	}
	s.send(m)
}

// handle は一つのメッセージを処理する。exit なら true。
func (s *server) handle(m *jsonrpc.Message) (exit bool) {
	switch {
	case m.IsResponse():
		if m.Error != nil {
			s.log.Printf("response %s: error %d %s", m.ID, m.Error.Code, m.Error.Message)
		}
		return false
	case m.IsRequest():
		s.handleRequest(m)
		return false
	default:
		return s.handleNotification(m)
	}
}

func (s *server) handleRequest(m *jsonrpc.Message) {
	s.log.Printf("request %s %s", m.ID, m.Method)
	if m.Method == protocol.MethodInitialize {
		if s.initialized {
			s.replyError(m.ID, jsonrpc.NewError(jsonrpc.CodeInvalidRequest, "already initialized"))
			return
		}
		s.initialize(m)
		return
	}
	if !s.initialized {
		s.replyError(m.ID, jsonrpc.NewError(jsonrpc.CodeServerNotInitialized, "not initialized"))
		return
	}
	if s.shutdown && m.Method != protocol.MethodShutdown {
		s.replyError(m.ID, jsonrpc.NewError(jsonrpc.CodeInvalidRequest, "shutting down"))
		return
	}
	switch m.Method {
	case protocol.MethodShutdown:
		s.shutdown = true
		s.stopTimer()
		s.reply(m.ID, nil)
	case protocol.MethodDefinition:
		var params protocol.TextDocumentPositionParams
		if !s.params(m, &params) {
			return
		}
		s.reply(m.ID, s.definition(params))
	case protocol.MethodHover:
		var params protocol.TextDocumentPositionParams
		if !s.params(m, &params) {
			return
		}
		s.reply(m.ID, s.hover(params))
	case protocol.MethodDocumentSymbol:
		var params protocol.DocumentSymbolParams
		if !s.params(m, &params) {
			return
		}
		s.reply(m.ID, s.documentSymbols(params))
	case protocol.MethodVocabulary:
		var params protocol.VocabularyParams
		if !s.params(m, &params) {
			return
		}
		res, err := s.vocabulary(params)
		if err != nil {
			s.replyError(m.ID, err)
			return
		}
		s.reply(m.ID, res)
	default:
		s.replyError(m.ID, jsonrpc.NewError(jsonrpc.CodeMethodNotFound, "unknown method %s", m.Method))
	}
}

// params は要求の params を読む。読めなければ InvalidParams で答えて false。
func (s *server) params(m *jsonrpc.Message, v any) bool {
	if err := m.UnmarshalParams(v); err != nil {
		s.replyError(m.ID, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "%s: %v", m.Method, err))
		return false
	}
	return true
}

func (s *server) handleNotification(m *jsonrpc.Message) (exit bool) {
	s.log.Printf("notification %s", m.Method)
	if m.Method == protocol.MethodExit {
		return true
	}
	if !s.initialized || m.Method == protocol.MethodCancelRequest {
		return false // docs/lsp.md「通信」: $/cancelRequest は受けるが無視する
	}
	var err error
	switch m.Method {
	case protocol.MethodInitialized:
		s.initializedNotification()
	case protocol.MethodDidOpen:
		var p protocol.DidOpenTextDocumentParams
		if err = m.UnmarshalParams(&p); err == nil {
			s.didOpen(p)
		}
	case protocol.MethodDidChange:
		var p protocol.DidChangeTextDocumentParams
		if err = m.UnmarshalParams(&p); err == nil {
			s.didChange(p)
		}
	case protocol.MethodDidClose:
		var p protocol.DidCloseTextDocumentParams
		if err = m.UnmarshalParams(&p); err == nil {
			s.didClose(p)
		}
	case protocol.MethodDidSave:
		var p protocol.DidSaveTextDocumentParams
		if err = m.UnmarshalParams(&p); err == nil {
			s.didSave(p)
		}
	case protocol.MethodDidChangeWatchedFiles:
		s.reanalyze()
	}
	if err != nil {
		s.log.Printf("%s: bad params: %v", m.Method, err)
	}
	return false
}

// initialize は docs/lsp.md「ライフサイクル」の initialize: rootUri（なければ workspaceFolders の先頭）を
// ワークスペースにし、位置のエンコーディングを決め、capabilities を返す。プロジェクトの解析と診断は initialized で。
func (s *server) initialize(m *jsonrpc.Message) {
	var params protocol.InitializeParams
	if !s.params(m, &params) {
		return
	}
	root := ""
	if params.RootURI != nil && *params.RootURI != "" {
		if p, ok := uriToFilePath(*params.RootURI); ok {
			root = p
		}
	}
	if root == "" && len(params.WorkspaceFolders) > 0 {
		if p, ok := uriToFilePath(params.WorkspaceFolders[0].URI); ok {
			root = p
		}
	}
	if root == "" && params.RootPath != nil {
		root = *params.RootPath
	}
	if root == "" {
		root = s.opts.Root
	}
	if root == "" {
		root = "."
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	s.root = root
	if g := params.Capabilities.General; g != nil {
		s.enc = negotiate(g.PositionEncodings)
	}
	if w := params.Capabilities.Workspace; w != nil && w.DidChangeWatchedFiles != nil {
		s.dynamicWatch = w.DidChangeWatchedFiles.DynamicRegistration
	}
	s.initialized = true
	s.log.Printf("initialize: root=%s encoding=%s", s.root, s.enc)
	s.reply(m.ID, protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			PositionEncoding: string(s.enc),
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.SyncFull,
				Save:      &protocol.SaveOptions{IncludeText: false},
			},
			DefinitionProvider:     true,
			HoverProvider:          true,
			DocumentSymbolProvider: true,
		},
		ServerInfo: &protocol.ServerInfo{Name: "hozoc"},
	})
}

// initializedNotification は docs/lsp.md「ライフサイクル」の initialized: 開いているファイルがなくても
// プロジェクト全体の診断を一度発行し、hozo.toml と .hozo の監視を登録する（docs/lsp.md「プロジェクト」）。
func (s *server) initializedNotification() {
	if s.dynamicWatch {
		s.request(protocol.MethodRegisterCapability, protocol.RegistrationParams{
			Registrations: []protocol.Registration{{
				ID:     "hozo-watched-files",
				Method: protocol.MethodDidChangeWatchedFiles,
				RegisterOptions: protocol.DidChangeWatchedFilesRegistrationOptions{
					Watchers: []protocol.FileSystemWatcher{
						{GlobPattern: "**/hozo.toml", Kind: protocol.WatchCreate | protocol.WatchChange | protocol.WatchDelete},
						{GlobPattern: "**/*.hozo", Kind: protocol.WatchCreate | protocol.WatchChange | protocol.WatchDelete},
					},
				},
			}},
		})
	}
	s.reanalyze()
}

// 文書の同期（docs/lsp.md「プロジェクト」: overlay は開いている文書の最新の内容）。

func (s *server) didOpen(p protocol.DidOpenTextDocumentParams) {
	key, ok := s.overlayKey(p.TextDocument.URI)
	if !ok {
		return
	}
	s.overlay[key] = []byte(p.TextDocument.Text)
	s.docs[p.TextDocument.URI] = key
	s.reanalyze()
}

func (s *server) didChange(p protocol.DidChangeTextDocumentParams) {
	key, ok := s.docs[p.TextDocument.URI]
	if !ok {
		if key, ok = s.overlayKey(p.TextDocument.URI); !ok {
			return
		}
		s.docs[p.TextDocument.URI] = key
	}
	text := s.overlay[key]
	if text == nil {
		if id, ok := s.fileOf(p.TextDocument.URI); ok {
			text = s.proj.Files.Content(id)
		}
	}
	for _, c := range p.ContentChanges {
		text = s.enc.applyChange(text, c)
	}
	s.overlay[key] = text
	s.schedule()
}

func (s *server) didClose(p protocol.DidCloseTextDocumentParams) {
	key, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return
	}
	delete(s.docs, p.TextDocument.URI)
	delete(s.overlay, key)
	s.reanalyze()
}

func (s *server) didSave(p protocol.DidSaveTextDocumentParams) {
	if p.Text != nil {
		if key, ok := s.docs[p.TextDocument.URI]; ok {
			s.overlay[key] = []byte(*p.Text)
		}
	}
	s.reanalyze()
}

// overlayKey は文書の URI を overlay のキー（絶対パス）にする。語彙の文書（読み取り専用）は持たない。
func (s *server) overlayKey(uri string) (string, bool) {
	if vocab.IsVocab(uri) {
		return "", false
	}
	return uriToFilePath(uri)
}

// didChange の待ち合わせ。タイマーは run の goroutine だけが触る。

func (s *server) schedule() {
	if s.timer == nil {
		s.timer = time.NewTimer(debounceDelay)
	} else {
		if !s.timer.Stop() && s.pending {
			select {
			case <-s.timer.C:
			default:
			}
		}
		s.timer.Reset(debounceDelay)
	}
	s.pending = true
}

func (s *server) timerC() <-chan time.Time {
	if s.timer == nil || !s.pending {
		return nil
	}
	return s.timer.C
}

func (s *server) stopTimer() {
	if s.timer != nil && s.pending {
		if !s.timer.Stop() {
			select {
			case <-s.timer.C:
			default:
			}
		}
	}
	s.pending = false
}

// reanalyze はプロジェクト全体を解析し直し（internal/project.Load。docs/lsp.md「プロジェクト」）、診断を発行する。
// 待ち合わせ中の didChange もここでまとめて反映する。
func (s *server) reanalyze() {
	s.stopTimer()
	if !s.initialized {
		return
	}
	overlay := make(map[string][]byte, len(s.overlay))
	for k, v := range s.overlay {
		overlay[k] = v
	}
	start := time.Now()
	s.proj = project.LoadWith(project.Options{
		Root:       s.root,
		ConfigPath: s.opts.ConfigPath,
		Overlay:    overlay,
		Stderr:     s.logW, // 標準エラーには書かない（docs/lsp.md「通信」）
		Env:        os.Environ(),
	})
	s.fileIDs = make(map[string]source.FileID, s.proj.Files.Len())
	for i := 0; i < s.proj.Files.Len(); i++ {
		s.fileIDs[s.proj.Files.Path(source.FileID(i))] = source.FileID(i)
	}
	s.asts = map[source.FileID]*ast.File{}
	s.declIdx = map[source.FileID][]declEntry{}
	s.pendingTags = map[source.FileID][]pendingTag{}
	s.docRefIdx = map[source.FileID][]declEntry{}
	s.declNS = namespaceIndex(s.proj.Sema)
	if s.proj.RunErr != nil {
		s.log.Printf("plugin check: %v", s.proj.RunErr)
	}
	s.log.Printf("analyzed %d files, %d diagnostics in %s", s.proj.Files.Len(), len(s.proj.Diags), time.Since(start))
	s.publishAll()
}

// publishAll は docs/lsp.md「プロジェクト」の発行: プロジェクトのすべてのファイルについて publishDiagnostics を送る。
// 診断がなくなったファイルには空の配列を送り、前回と同じ内容なら送らない。
func (s *server) publishAll() {
	byURI := s.lspDiagnostics()
	uris := make([]string, 0, len(byURI)+len(s.published))
	seen := map[string]bool{}
	for uri := range byURI {
		seen[uri] = true
		uris = append(uris, uri)
	}
	for uri := range s.published {
		if !seen[uri] {
			uris = append(uris, uri)
		}
	}
	sort.Strings(uris)
	for _, uri := range uris {
		diags := byURI[uri]
		if diags == nil {
			diags = []protocol.Diagnostic{}
		}
		b, err := json.Marshal(diags)
		if err != nil {
			s.log.Printf("diagnostics %s: %v", uri, err)
			continue
		}
		prev, ok := s.published[uri]
		if !ok && len(diags) == 0 {
			continue // 一度も送っていないファイルの「なし」は送らない
		}
		if ok && prev == string(b) {
			continue
		}
		s.published[uri] = string(b)
		s.notify(protocol.MethodPublishDiagnostics, protocol.PublishDiagnosticsParams{URI: uri, Diagnostics: diags})
	}
}

// fileOf は URI のファイルの FileID。プロジェクトにないファイルなら false。
func (s *server) fileOf(uri string) (source.FileID, bool) {
	if s.proj == nil {
		return 0, false
	}
	path, ok := uriToPath(s.proj.Root, uri)
	if !ok {
		return 0, false
	}
	id, ok := s.fileIDs[path]
	return id, ok
}

// location はバイトの範囲を Location にする。
func (s *server) location(sp source.Span) protocol.Location {
	fs := s.proj.Files
	return protocol.Location{URI: pathToURI(s.proj.Root, fs.Path(sp.File)), Range: s.enc.toRange(fs, sp)}
}

// vocabulary は要求 hozo/vocabulary（docs/lsp.md「プロジェクト」）: `hozo:` の URI の内容を返す。
// プロジェクトに読み込んだ語彙（標準とプラグイン）から引き、なければ同梱の標準の語彙から引く。
func (s *server) vocabulary(p protocol.VocabularyParams) (protocol.VocabularyResult, *jsonrpc.Error) {
	if !vocab.IsVocab(p.URI) {
		return protocol.VocabularyResult{}, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "not a hozo: URI: %s", p.URI)
	}
	if id, ok := s.fileOf(p.URI); ok {
		return protocol.VocabularyResult{Content: string(s.proj.Files.Content(id))}, nil
	}
	for _, f := range vocab.Files() {
		if f.Path == p.URI {
			return protocol.VocabularyResult{Content: string(f.Content)}, nil
		}
	}
	return protocol.VocabularyResult{}, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "unknown vocabulary: %s", p.URI)
}

// configURI は位置のない診断を付ける hozo.toml の URI（docs/lsp.md「診断」）。
func (s *server) configURI() string {
	if s.proj.Config != nil && s.proj.Config.Path != "" {
		return fileURI(s.proj.Config.Path)
	}
	return fileURI(filepath.Join(s.proj.Root, "hozo.toml"))
}

// String はログ用。
func (e encoding) String() string { return string(e) }
