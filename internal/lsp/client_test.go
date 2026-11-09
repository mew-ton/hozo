package lsp

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mew-ton/hozo/internal/lsp/jsonrpc"
	"github.com/mew-ton/hozo/internal/lsp/protocol"
)

func init() {
	debounceDelay = 20 * time.Millisecond // テストを速くする（docs/lsp.md「プロジェクト」の 150 ms はテストでは待たない）
}

// waitTimeout はサーバーからの一つのメッセージを待つ上限。
const waitTimeout = 10 * time.Second

// testClient は in-memory のパイプで Serve とつながったクライアント（docs/testing.md「単体テスト」の
// 台本どおりのセッション）。サーバーからの要求（client/registerCapability）には null で答える。
type testClient struct {
	t        *testing.T
	w        *jsonrpc.Writer
	in       *io.PipeWriter
	msgs     chan *jsonrpc.Message
	done     chan struct{}
	nextID   int64
	pending  []*jsonrpc.Message // 応答を待つ間に届いた通知
	requests []*jsonrpc.Message // サーバーからの要求
	logBuf   *syncWriter
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func startServer(t *testing.T, opts Options) *testClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &testClient{t: t, w: jsonrpc.NewWriter(inW), in: inW, msgs: make(chan *jsonrpc.Message, 1024), done: make(chan struct{}), logBuf: &syncWriter{b: &strings.Builder{}}}
	if opts.Log == nil {
		opts.Log = c.logBuf
	}
	go func() {
		_ = Serve(inR, outW, opts)
		_ = outW.Close()
		close(c.done)
	}()
	go func() {
		r := jsonrpc.NewReader(outR)
		for {
			m, err := r.ReadMessage()
			if err != nil {
				close(c.msgs)
				return
			}
			if m.IsRequest() {
				resp, _ := jsonrpc.NewResponse(m.ID, nil)
				go func() { _ = c.w.WriteMessage(resp) }() // 書き込みで読みを止めない
			}
			c.msgs <- m
		}
	}()
	t.Cleanup(func() {
		_ = inW.Close()
		select {
		case <-c.done:
		case <-time.After(waitTimeout):
			t.Errorf("Serve が戻らない")
		}
	})
	return c
}

// syncWriter はログを複数の goroutine から書けるようにする。
type syncWriter struct {
	mu sync.Mutex
	b  *strings.Builder
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (c *testClient) notify(method string, params any) {
	c.t.Helper()
	m, err := jsonrpc.NewNotification(method, params)
	if err != nil {
		c.t.Fatal(err)
	}
	if err := c.w.WriteMessage(m); err != nil {
		c.t.Fatal(err)
	}
}

// next はサーバーからの次のメッセージ（応答を待つ間に取り置いた通知を先に）。
func (c *testClient) next() *jsonrpc.Message {
	c.t.Helper()
	if len(c.pending) > 0 {
		m := c.pending[0]
		c.pending = c.pending[1:]
		return m
	}
	select {
	case m, ok := <-c.msgs:
		if !ok {
			c.t.Fatalf("サーバーが出力を閉じた\nlog:\n%s", c.logBuf.String())
		}
		if m.IsRequest() {
			c.requests = append(c.requests, m)
		}
		return m
	case <-time.After(waitTimeout):
		c.t.Fatalf("サーバーからのメッセージを待ちきれない\nlog:\n%s", c.logBuf.String())
	}
	return nil
}

// call は要求を送って応答を待つ。応答が来るまでの通知は pending に取り置く。
func (c *testClient) call(method string, params any) *jsonrpc.Message {
	c.t.Helper()
	c.nextID++
	id := jsonrpc.NumberID(c.nextID)
	m, err := jsonrpc.NewRequest(id, method, params)
	if err != nil {
		c.t.Fatal(err)
	}
	if err := c.w.WriteMessage(m); err != nil {
		c.t.Fatal(err)
	}
	var held []*jsonrpc.Message
	for {
		r := c.next()
		if r.IsResponse() && r.ID.Equal(id) {
			c.pending = append(held, c.pending...)
			return r
		}
		held = append(held, r)
	}
}

// result は要求の結果を v に読む。誤りの応答なら失敗。
func (c *testClient) result(method string, params any, v any) {
	c.t.Helper()
	r := c.call(method, params)
	if r.Error != nil {
		c.t.Fatalf("%s: error %d %s", method, r.Error.Code, r.Error.Message)
	}
	if v != nil {
		if err := json.Unmarshal(r.Result, v); err != nil {
			c.t.Fatalf("%s: result %s: %v", method, r.Result, err)
		}
	}
}

// initialize は initialize と initialized を送り、最初の診断の発行をすべて集める（docs/lsp.md「ライフサイクル」）。
// 発行は initialized の処理の中で同期的に送られるので、次の要求の応答が来るまでの通知がそのすべて。
func (c *testClient) initialize(root string, caps protocol.ClientCapabilities) (protocol.InitializeResult, map[string][]protocol.Diagnostic) {
	c.t.Helper()
	rootURI := fileURI(root)
	var res protocol.InitializeResult
	c.result(protocol.MethodInitialize, protocol.InitializeParams{RootURI: &rootURI, Capabilities: caps}, &res)
	c.notify(protocol.MethodInitialized, struct{}{})
	return res, c.syncDiagnostics()
}

// syncDiagnostics は、軽い要求を一つ送り、その応答までに届いた publishDiagnostics を URI ごとに集める。
func (c *testClient) syncDiagnostics() map[string][]protocol.Diagnostic {
	c.t.Helper()
	r := c.call(protocol.MethodVocabulary, protocol.VocabularyParams{URI: "hozo:vocab/valid.hozo"})
	if r.Error != nil {
		c.t.Fatalf("sync: %v", r.Error)
	}
	out := map[string][]protocol.Diagnostic{}
	for len(c.pending) > 0 {
		m := c.pending[0]
		c.pending = c.pending[1:]
		if m.Method != protocol.MethodPublishDiagnostics {
			continue
		}
		var p protocol.PublishDiagnosticsParams
		if err := json.Unmarshal(m.Params, &p); err != nil {
			c.t.Fatal(err)
		}
		out[p.URI] = p.Diagnostics
	}
	return out
}

// waitDiagnostics は uri への次の publishDiagnostics を待つ。ほかの通知は捨てる。
func (c *testClient) waitDiagnostics(uri string) []protocol.Diagnostic {
	c.t.Helper()
	for {
		m := c.next()
		if m.Method != protocol.MethodPublishDiagnostics {
			continue
		}
		var p protocol.PublishDiagnosticsParams
		if err := json.Unmarshal(m.Params, &p); err != nil {
			c.t.Fatal(err)
		}
		if p.URI == uri {
			return p.Diagnostics
		}
	}
}

// shutdown は shutdown と exit を送り、Serve が戻るのを待つ。
func (c *testClient) shutdown() {
	c.t.Helper()
	r := c.call(protocol.MethodShutdown, nil)
	if r.Error != nil || string(r.Result) != "null" {
		c.t.Fatalf("shutdown: %+v", r)
	}
	c.notify(protocol.MethodExit, nil)
	select {
	case <-c.done:
	case <-time.After(waitTimeout):
		c.t.Fatal("exit の後に Serve が戻らない")
	}
}

// corpusDir は tests/corpus/design の絶対パス。
func corpusDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "tests", "corpus", "design"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hozo.toml")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// positionOf は content の中の needle（n 番目。0 始まり）の位置を enc の Position にする。
func positionOf(t *testing.T, enc encoding, content, needle string, n int) protocol.Position {
	t.Helper()
	off := -1
	from := 0
	for i := 0; i <= n; i++ {
		j := strings.Index(content[from:], needle)
		if j < 0 {
			t.Fatalf("%q が %d 回見つからない", needle, n+1)
		}
		off = from + j
		from = off + len(needle)
	}
	line := strings.Count(content[:off], "\n")
	ls := lineStart([]byte(content), off)
	return protocol.Position{Line: line, Character: enc.units([]byte(content[ls:off]))}
}

// offsetOf は Position をバイトのオフセットに戻す。
func offsetOf(enc encoding, content string, pos protocol.Position) int {
	return int(enc.toOffset([]byte(content), pos))
}

func textDoc(uri string) protocol.TextDocumentIdentifier {
	return protocol.TextDocumentIdentifier{URI: uri}
}

func hasCode(diags []protocol.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func codes(diags []protocol.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return out
}
