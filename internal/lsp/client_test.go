package lsp

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mew-ton/hozo/internal/lsp/jsonrpc"
	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/project"
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
	if opts.PluginPath == "" {
		// コーパスの出力単位が指す基盤のトランスパイラ（go）は、リポジトリの plugins/ にある。サーバーは、文書を開いたときと、
		// 保存したときに、プラグインの check を、裏で起動する（docs/lsp.md「プラグインの診断」）ので、起動するのは、代役にする
		opts.PluginPath = standInPlugins(t)
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
	return c.initializeWith(root, caps, "")
}

// initializeWith は、initialize に、initializationOptions（JSON の文面。"" なら、付けない）を付けて送る
// （docs/lsp.md「プラグインの診断」の「許し」）。
func (c *testClient) initializeWith(root string, caps protocol.ClientCapabilities, options string) (protocol.InitializeResult, map[string][]protocol.Diagnostic) {
	c.t.Helper()
	rootURI := fileURI(root)
	params := protocol.InitializeParams{RootURI: &rootURI, Capabilities: caps}
	if options != "" {
		params.InitializationOptions = json.RawMessage(options)
	}
	var res protocol.InitializeResult
	c.result(protocol.MethodInitialize, params, &res)
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

// repoPlugins はリポジトリの plugins/（HOZO_PLUGIN_PATH に渡す場所。コーパスの hozo.toml が指す go はここにある）。
func repoPlugins(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// standInPlugins は、リポジトリの plugins/ の代役: マニフェストと語彙は、同じで、起動するのは、診断を返さない sh のスクリプト
// （本物は、開発中のマニフェストの go run で、go.mod のない一時のプロジェクトでは、異常終了する）。sh がなければ、本物の場所。
func standInPlugins(t *testing.T) string {
	t.Helper()
	repo := repoPlugins(t)
	if _, err := exec.LookPath("sh"); err != nil {
		return repo
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "ok.sh")
	writeFile(t, script, "cat >/dev/null\nprintf '{\"diagnostics\":[]}'\n")
	entries, err := os.ReadDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(repo, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if f.IsDir() || strings.HasSuffix(f.Name(), ".go") {
				continue
			}
			content := readFile(t, filepath.Join(repo, e.Name(), f.Name()))
			if f.Name() == "hozo-plugin.toml" {
				lines := strings.Split(content, "\n")
				for i, l := range lines {
					if strings.HasPrefix(l, "command") {
						lines[i] = "command = [\"sh\", " + strconv.Quote(script) + "]"
					}
				}
				content = strings.Join(lines, "\n")
			}
			writeFile(t, filepath.Join(dir, e.Name(), f.Name()), content)
		}
	}
	return dir
}

// loadCorpus は check と同じ結果（project.LoadWith）から、プラグインの check の診断を除いたもの（言語サーバーは、プラグインの check を
// 起動しない。docs/lsp.md「診断」）。プラグインの探索は、テストのサーバー（startServer）と同じ場所を見る。
func loadCorpus(t *testing.T, root string) *project.Project {
	t.Helper()
	return project.LoadWith(project.Options{Root: root, PluginPath: repoPlugins(t), NoPluginChecks: true})
}

// warnFile は corpusWithWarning が足すファイル。設計書のコーパスには診断がない（error も warning もない。
// tests/corpus/design/README.md の想定）ので、診断の発行と check との一致を空でない集合で
// 確かめるために、意図した warning を一つ持たせる: 効果を持たない op の prevents effects（REL-2。docs/check.md「throws」）。
const warnFile = `// internal/lsp のテストが足すファイル: 意図した warning（REL-2）を一つ出す
namespace server {
  /** 手が空いている */
  exception Idle;
  fn ping(): throws Idle prevents effects;
}
`

// corpusWithWarning は tests/corpus/design の直下のファイルを一時ディレクトリに写し、warn.hozo（warnFile）を足した
// プロジェクトのルート。
func corpusWithWarning(t *testing.T) string {
	t.Helper()
	src := corpusDir(t)
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		writeFile(t, filepath.Join(dir, rel), readFile(t, p))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "warn.hozo"), warnFile)
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
