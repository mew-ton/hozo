package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/lsp/jsonrpc"
	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// diagKey は docs/testing.md「言語サーバーと check の一致」の比較の単位（ファイル、行、列、ID、重さ）。
type diagKey struct {
	file      string
	line, col int
	code      string
	severity  int
}

func (k diagKey) String() string {
	return fmt.Sprintf("%s:%d:%d %s sev=%d", k.file, k.line, k.col, k.code, k.severity)
}

// checkKeys は hozoc check 側（internal/project.Load）の診断の集合。
func checkKeys(p *project.Project) map[diagKey]bool {
	out := map[diagKey]bool{}
	for _, d := range p.Diags {
		k := diagKey{code: d.Code, severity: int(d.Severity) + 1}
		if d.Span != nil {
			k.file = p.Files.Path(d.Span.File)
			pos := p.Files.Position(d.Span.File, d.Span.Start)
			k.line, k.col = pos.Line, pos.Col
		} else {
			k.file = d.File
			if k.file == "" {
				k.file = "hozo.toml"
			}
			k.line, k.col = 1, 1
		}
		out[k] = true
	}
	return out
}

// lspKeys は publishDiagnostics から戻した集合。LSP の位置（enc の単位）はファイルの内容でコードポイントの列に戻す。
// LSP-1 は言語サーバーにしか出ないので除く。
func lspKeys(t *testing.T, p *project.Project, enc encoding, byURI map[string][]protocol.Diagnostic) map[diagKey]bool {
	t.Helper()
	out := map[diagKey]bool{}
	ids := map[string]source.FileID{}
	for i := 0; i < p.Files.Len(); i++ {
		ids[p.Files.Path(source.FileID(i))] = source.FileID(i)
	}
	for uri, diags := range byURI {
		path, ok := uriToPath(p.Root, uri)
		if !ok {
			t.Fatalf("URI が読めない: %s", uri)
		}
		for _, d := range diags {
			if d.Code == codeNoConfig {
				continue
			}
			k := diagKey{file: path, code: d.Code, severity: int(d.Severity)}
			if id, ok := ids[path]; ok {
				off := enc.toOffset(p.Files.Content(id), d.Range.Start)
				pos := p.Files.Position(id, off)
				k.line, k.col = pos.Line, pos.Col
			} else {
				k.line, k.col = d.Range.Start.Line+1, d.Range.Start.Character+1
			}
			out[k] = true
		}
	}
	return out
}

func diffKeys(t *testing.T, want, got map[diagKey]bool) {
	t.Helper()
	var lines []string
	for k := range want {
		if !got[k] {
			lines = append(lines, "lsp にない: "+k.String())
		}
	}
	for k := range got {
		if !want[k] {
			lines = append(lines, "check にない: "+k.String())
		}
	}
	sort.Strings(lines)
	if len(lines) > 0 {
		t.Fatalf("診断の集合が違う:\n%s", strings.Join(lines, "\n"))
	}
}

// TestCheckAgreement は docs/testing.md「言語サーバーと check の一致」: 各コーパスについて project.Load の診断と
// publishDiagnostics の診断（LSP の形から戻したもの）が同じ集合であること。utf-16 と utf-8 の両方で。
// コーパスは tests/corpus の下のすべて（足せばここにも入る）と、設計書のコーパスに warning を一つ足したもの
// （corpusWithWarning。設計書のコーパスには診断がないので、空でない集合でも確かめる）。
func TestCheckAgreement(t *testing.T) {
	corpora, err := filepath.Abs(filepath.Join("..", "..", "tests", "corpus"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(corpora)
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"design+warning": corpusWithWarning(t)}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(corpora, e.Name(), "hozo.toml")); err == nil {
			roots[e.Name()] = filepath.Join(corpora, e.Name())
		}
	}
	// docs/testing.md「コーパス」に並ぶものがそろっている（名前を変えたり消したりしたら、ここで分かる）
	for _, name := range []string{"design", "view", "state", "model", "units"} {
		if roots[name] == "" {
			t.Errorf("コーパス %s がない", name)
		}
	}
	nonEmpty := 0
	for name, root := range roots {
		t.Run(name, func(t *testing.T) {
			want := checkKeys(loadCorpus(t, root))
			if name == "design+warning" && len(want) == 0 {
				t.Fatal("コーパスに診断がない（warn.hozo の REL-2 の warning があるはず）")
			}
			if len(want) > 0 {
				nonEmpty++
			}
			for _, encs := range [][]string{nil, {"utf-8", "utf-16"}} {
				c := startServer(t, Options{})
				res, byURI := c.initialize(root, protocol.ClientCapabilities{General: &protocol.GeneralClientCapabilities{PositionEncodings: encs}})
				enc := encoding(res.Capabilities.PositionEncoding)
				if (encs == nil && enc != encUTF16) || (encs != nil && enc != encUTF8) {
					t.Fatalf("positionEncoding = %q for %v", enc, encs)
				}
				p := loadCorpus(t, root)
				diffKeys(t, want, lspKeys(t, p, enc, byURI))
				c.shutdown()
			}
		})
	}
	if nonEmpty < 2 {
		t.Errorf("診断のあるコーパスが %d 個しかない（design+warning と view にはあるはず）", nonEmpty)
	}
}

// TestSession は台本どおりのセッション（docs/testing.md「単体テスト」）: initialize → initialized → didOpen →
// didChange（誤りを入れる → 直す）→ 定義へジャンプ → ホバー → ドキュメントのシンボル → shutdown / exit。
func TestSession(t *testing.T) {
	root := corpusWithWarning(t)
	c := startServer(t, Options{})
	res, byURI := c.initialize(root, protocol.ClientCapabilities{
		Workspace: &protocol.WorkspaceClientCapabilities{DidChangeWatchedFiles: &protocol.DynamicRegistration{DynamicRegistration: true}},
	})
	enc := encUTF16

	// capabilities（docs/lsp.md「ライフサイクル」）
	caps := res.Capabilities
	if caps.PositionEncoding != "utf-16" || !caps.DefinitionProvider || !caps.HoverProvider || !caps.DocumentSymbolProvider {
		t.Fatalf("capabilities = %+v", caps)
	}
	if ts := caps.TextDocumentSync; ts == nil || !ts.OpenClose || ts.Change != protocol.SyncFull || ts.Save == nil || ts.Save.IncludeText {
		t.Fatalf("textDocumentSync = %+v", caps.TextDocumentSync)
	}

	// 監視の登録（docs/lsp.md「プロジェクト」）
	var registered bool
	for _, r := range c.requests {
		if r.Method == protocol.MethodRegisterCapability && strings.Contains(string(r.Params), "**/hozo.toml") && strings.Contains(string(r.Params), "**/*.hozo") {
			registered = true
		}
	}
	if !registered {
		t.Fatalf("client/registerCapability がない: %d requests", len(c.requests))
	}

	serverPath := filepath.Join(root, "server.hozo")
	browserPath := filepath.Join(root, "browser.hozo")
	serverURI, browserURI := fileURI(serverPath), fileURI(browserPath)
	serverText, browserText := readFile(t, serverPath), readFile(t, browserPath)

	// 最初の発行: warn.hozo の REL-2 だけ（corpusWithWarning）。診断のないファイルは最初は送らない。
	warnURI := fileURI(filepath.Join(root, "warn.hozo"))
	if got := codes(byURI[warnURI]); strings.Join(got, ",") != "REL-2" {
		t.Fatalf("warn.hozo の最初の診断 = %v", got)
	}
	for _, uri := range []string{serverURI, browserURI} {
		if _, ok := byURI[uri]; ok {
			t.Fatalf("%s に診断がないのに最初から送っている: %v", uri, byURI[uri])
		}
	}
	for uri := range byURI {
		if !strings.HasPrefix(uri, "file://") && !strings.HasPrefix(uri, "hozo:") {
			t.Fatalf("URI の形: %s", uri)
		}
	}

	// didOpen → 変わらないので発行なし。didChange で誤りを入れる → 新しい診断。
	c.notify(protocol.MethodDidOpen, protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: browserURI, LanguageID: "hozo", Version: 1, Text: browserText}})
	broken := strings.Replace(browserText, "scalar PosInt extends int;", "scalar PosInt extends Nope;", 1)
	if broken == browserText {
		t.Fatal("置き換えに失敗")
	}
	c.notify(protocol.MethodDidChange, protocol.DidChangeTextDocumentParams{
		TextDocument:   protocol.VersionedTextDocumentIdentifier{URI: browserURI, Version: 2},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{{Text: broken}},
	})
	diags := c.waitDiagnostics(browserURI)
	if !hasCode(diags, "RES-1") {
		t.Fatalf("didChange の後の browser.hozo の診断 = %v", codes(diags))
	}
	var res1 protocol.Diagnostic
	for _, d := range diags {
		if d.Code == "RES-1" {
			res1 = d
		}
	}
	wantPos := positionOf(t, enc, broken, "Nope", 0)
	if res1.Range.Start != wantPos || res1.Severity != protocol.SeverityError || res1.Source != diagnosticSource || res1.Message == "" {
		t.Fatalf("RES-1 = %+v, want start %+v", res1, wantPos)
	}
	if res1.Range.End.Character != wantPos.Character+len("Nope") {
		t.Fatalf("RES-1 end = %+v", res1.Range.End)
	}

	// 直す → 空の配列（docs/lsp.md「プロジェクト」: 診断がなくなったファイルには空の配列を送る）
	c.notify(protocol.MethodDidChange, protocol.DidChangeTextDocumentParams{
		TextDocument:   protocol.VersionedTextDocumentIdentifier{URI: browserURI, Version: 3},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{{Text: browserText}},
	})
	if diags = c.waitDiagnostics(browserURI); len(diags) != 0 {
		t.Fatalf("直した後の診断 = %v", codes(diags))
	}

	// 定義へジャンプ: 別の名前空間の宣言（browser の server.Token → server.hozo の scalar Token）
	var loc *protocol.Location
	c.result(protocol.MethodDefinition, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: positionOf(t, enc, browserText, "server.Token>", 0)}, &loc)
	// 位置は "server." の先頭なので名前空間 server のブロック
	if loc == nil || loc.URI != serverURI || loc.Range.Start != positionOf(t, enc, serverText, "server {", 0) {
		t.Fatalf("definition(server) = %+v", loc)
	}
	tokenPos := positionOf(t, enc, browserText, "Token>", 0)
	c.result(protocol.MethodDefinition, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: tokenPos}, &loc)
	if loc == nil || loc.URI != serverURI || loc.Range.Start != positionOf(t, enc, serverText, "Token extends", 0) {
		t.Fatalf("definition(Token) = %+v", loc)
	}
	if loc.Range.End.Character-loc.Range.Start.Character != len("Token") {
		t.Fatalf("definition(Token) range = %+v", loc.Range)
	}

	// 語彙への参照（http.NetworkError → hozo:vocab/http.hozo）と hozo/vocabulary
	c.result(protocol.MethodDefinition, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: positionOf(t, enc, browserText, "NetworkError recovers", 0)}, &loc)
	if loc == nil || loc.URI != "hozo:vocab/http.hozo" {
		t.Fatalf("definition(http.NetworkError) = %+v", loc)
	}
	var voc protocol.VocabularyResult
	c.result(protocol.MethodVocabulary, protocol.VocabularyParams{URI: loc.URI}, &voc)
	line := strings.Split(voc.Content, "\n")[loc.Range.Start.Line]
	if !strings.Contains(line, "exception NetworkError") || !strings.HasPrefix(line[offsetOf(enc, line, protocol.Position{Character: loc.Range.Start.Character})-0:], "NetworkError") {
		t.Fatalf("vocabulary の行 %d = %q", loc.Range.Start.Line, line)
	}
	if r := c.call(protocol.MethodVocabulary, protocol.VocabularyParams{URI: "hozo:vocab/nope.hozo"}); r.Error == nil || r.Error.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("知らない語彙: %+v", r)
	}

	// 解決できない位置は null（コメントの中）
	c.result(protocol.MethodDefinition, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: positionOf(t, enc, browserText, "設計書", 0)}, &loc)
	if loc != nil {
		t.Fatalf("definition(comment) = %+v", loc)
	}

	// ホバー: op の宣言名（throws / effects と relations）
	var hov *protocol.Hover
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(serverURI), Position: positionOf(t, enc, serverText, "placeOrder(in: OrderInput)", 0)}, &hov)
	if hov == nil || hov.Contents.Kind != protocol.Markdown {
		t.Fatalf("hover(placeOrder) = %+v", hov)
	}
	for _, want := range []string{"```hozo\nfn placeOrder(in: OrderInput): Order\n```", "throws:", "- `NotificationFailed` — prevents notifyUser, keeps createOrder", "effects:", "- `createOrder`", "- `notifyUser`"} {
		if !strings.Contains(hov.Contents.Value, want) {
			t.Errorf("hover(placeOrder) に %q がない:\n%s", want, hov.Contents.Value)
		}
	}
	// ホバー: 別の名前空間から見た op の参照（修飾あり）と説明
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: positionOf(t, enc, browserText, "addToCart({ item })", 0)}, &hov)
	if hov == nil {
		t.Fatal("hover(server.addToCart) = nil")
	}
	for _, want := range []string{"fn server.addToCart(in: server.AddToCartInput): server.CartLine", "商品をカートに入れる", "- `server.addCartLine`", "throws: なし"} {
		if !strings.Contains(hov.Contents.Value, want) {
			t.Errorf("hover(server.addToCart) に %q がない:\n%s", want, hov.Contents.Value)
		}
	}
	// ホバー: 名前付きデコレータの適用は、宣言と固定した引数（docs/lsp.md「ホバー」の 4）
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: positionOf(t, enc, browserText, "unavailableOnFailure\n  @page.route(\"/profile\")", 0)}, &hov)
	if hov == nil {
		t.Fatal("hover(@unavailableOnFailure) = nil")
	}
	for _, want := range []string{"decorator unavailableOnFailure = page.fallback(error: http.NetworkError | http.ServerError, view: Unavailable)", "decorator page.fallback(error: any exception, view: any model) catches error recovers", "固定した引数:", "- `error: http.NetworkError | http.ServerError`", "- `view: Unavailable`"} {
		if !strings.Contains(hov.Contents.Value, want) {
			t.Errorf("hover(@unavailableOnFailure) に %q がない:\n%s", want, hov.Contents.Value)
		}
	}
	// ホバー: フィールド、ケース、引数
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: positionOf(t, enc, browserText, "page, status", 0)}, &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "```hozo\nmodel OrdersQuery\n  page: PosInt\n```") {
		t.Errorf("hover(field page) = %+v", hov)
	}
	// 別の名前空間の model のフィールド（説明付き）: server.GetOrdersInput.page の宣言名
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(serverURI), Position: positionOf(t, enc, serverText, "page: int;", 0)}, &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "model GetOrdersInput\n  page: int\n") || !strings.Contains(hov.Contents.Value, "1 から数えるページ番号") {
		t.Errorf("hover(field GetOrdersInput.page) = %+v", hov)
	}
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: positionOf(t, enc, browserText, "Yamada     =>", 0)}, &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "classify WarrantyKind of server.Item\n  Yamada") || !strings.Contains(hov.Contents.Value, "ヤマダ電機の専用保証プラン") {
		t.Errorf("hover(case Yamada) = %+v", hov)
	}
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(serverURI), Position: positionOf(t, enc, serverText, "email 再設定", 0)}, &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "fn requestPasswordReset(email: Email)\n  email: Email") || !strings.Contains(hov.Contents.Value, "再設定するアカウントのメールアドレス") {
		t.Errorf("hover(@param email) = %+v", hov)
	}
	// @throws / @see の名前は宣言へ飛ぶ（docs/lsp.md「定義へジャンプ」の表）
	c.result(protocol.MethodDefinition, protocol.TextDocumentPositionParams{TextDocument: textDoc(serverURI), Position: positionOf(t, enc, serverText, "RateLimited 同じアドレス", 0)}, &loc)
	if loc == nil || loc.URI != serverURI || loc.Range.Start != positionOf(t, enc, serverText, "RateLimited;", 0) {
		t.Errorf("definition(@throws RateLimited) = %+v", loc)
	}
	c.result(protocol.MethodDefinition, protocol.TextDocumentPositionParams{TextDocument: textDoc(serverURI), Position: shift(positionOf(t, enc, serverText, "@see login", 0), len("@see "))}, &loc)
	if loc == nil || loc.URI != serverURI || loc.Range.Start != positionOf(t, enc, serverText, "login(in: LoginInput)", 0) {
		t.Errorf("definition(@see login) = %+v", loc)
	}
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(serverURI), Position: positionOf(t, enc, serverText, "Email extends string", 0)}, &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "```hozo\nscalar Email extends string\n```") || !strings.Contains(hov.Contents.Value, "メールアドレス") {
		t.Errorf("hover(scalar Email) = %+v", hov)
	}

	// ドキュメントのシンボル（docs/lsp.md「ドキュメントのシンボル」）
	var syms []protocol.DocumentSymbol
	c.result(protocol.MethodDocumentSymbol, protocol.DocumentSymbolParams{TextDocument: textDoc(browserURI)}, &syms)
	if len(syms) != 1 || syms[0].Name != "browser" || syms[0].Kind != protocol.SymbolNamespace {
		t.Fatalf("documentSymbol top = %+v", summarize(syms))
	}
	kinds := map[string]protocol.SymbolKind{}
	children := map[string][]protocol.DocumentSymbol{}
	for _, s := range syms[0].Children {
		kinds[s.Name+"/"+fmt.Sprint(s.Kind)] = s.Kind
		children[s.Name] = s.Children
	}
	wantKinds := map[string]protocol.SymbolKind{
		"token":                protocol.SymbolVariable, // state（bind の token は Property で別）
		"transitionToLogin":    protocol.SymbolEvent,    // effect
		"ApiError":             protocol.SymbolEnum,     // union
		"PosInt":               protocol.SymbolTypeParameter,
		"OfflineCacheMiss":     protocol.SymbolClass,     // exception
		"fallbackToCache":      protocol.SymbolInterface, // decorator
		"clearToken":           protocol.SymbolFunction,  // op
		"WarrantyKind":         protocol.SymbolEnum,      // classify
		"WarrantyAvailable":    protocol.SymbolEnum,      // classify（二つに分かれる分類も、両側がケース）
		"OrdersQuery":          protocol.SymbolStruct,    // model
		"unavailableOnFailure": protocol.SymbolInterface,
	}
	for name, kind := range wantKinds {
		if _, ok := kinds[name+"/"+fmt.Sprint(kind)]; !ok {
			t.Errorf("symbol %s kind %d がない", name, kind)
		}
	}
	if _, ok := kinds["token/"+fmt.Sprint(protocol.SymbolProperty)]; !ok {
		t.Errorf("bind token（Property）がない")
	}
	if got := names(children["WarrantyKind"]); strings.Join(got, ",") != "Standard,Yamada,NoWarranty" {
		t.Errorf("WarrantyKind の子 = %v", got)
	}
	for _, ch := range children["WarrantyKind"] {
		if ch.Kind != protocol.SymbolEnumMember {
			t.Errorf("case %s kind = %d", ch.Name, ch.Kind)
		}
	}
	if got := names(children["CampaignKind"]); strings.Join(got, ",") != "Spring,Summer,NoCampaign" {
		t.Errorf("CampaignKind の子 = %v", got)
	}
	if got := names(children["WarrantyAvailable"]); strings.Join(got, ",") != "Available,Unavailable" {
		t.Errorf("WarrantyAvailable の子 = %v", got)
	}
	if got := names(children["OrdersQuery"]); strings.Join(got, ",") != "page,status" || children["OrdersQuery"][0].Kind != protocol.SymbolField || children["OrdersQuery"][0].Detail != "PosInt" {
		t.Errorf("OrdersQuery の子 = %+v", summarize(children["OrdersQuery"]))
	}
	if got := names(children["OrderPage"]); strings.Join(got, ",") != "Ready,Empty,Unavailable" || children["OrderPage"][0].Kind != protocol.SymbolEnumMember {
		t.Errorf("OrderPage の子 = %+v", summarize(children["OrderPage"]))
	}
	for _, s := range syms[0].Children {
		if s.Name == "openOrders" && s.Detail != "(query: OrdersQuery): OrderPage" {
			t.Errorf("openOrders detail = %q", s.Detail)
		}
		if s.Range.Start.Line > s.SelectionRange.Start.Line || s.Range.End.Line < s.SelectionRange.End.Line {
			t.Errorf("%s: selectionRange %+v が range %+v の外", s.Name, s.SelectionRange, s.Range)
		}
	}
	// 語彙のファイルのシンボルも引ける
	c.result(protocol.MethodDocumentSymbol, protocol.DocumentSymbolParams{TextDocument: textDoc("hozo:vocab/http.hozo")}, &syms)
	if len(syms) != 1 || syms[0].Name != "http" {
		t.Fatalf("documentSymbol(hozo:vocab/http.hozo) = %+v", summarize(syms))
	}

	// didClose → ディスクの内容で解析し直す。didSave も解析し直す（発行は変わらないので何も届かない）。
	c.notify(protocol.MethodDidClose, protocol.DidCloseTextDocumentParams{TextDocument: textDoc(browserURI)})
	c.notify(protocol.MethodDidSave, protocol.DidSaveTextDocumentParams{TextDocument: textDoc(serverURI)})
	c.notify(protocol.MethodDidChangeWatchedFiles, protocol.DidChangeWatchedFilesParams{Changes: []protocol.FileEvent{{URI: fileURI(filepath.Join(root, "hozo.toml")), Type: protocol.FileChanged}}})
	if got := c.syncDiagnostics(); len(got) != 0 {
		t.Fatalf("変わらない解析で発行した: %v", got)
	}

	// $/cancelRequest は無視、知らないメソッドは MethodNotFound
	c.notify(protocol.MethodCancelRequest, map[string]any{"id": 1})
	if r := c.call("textDocument/nope", struct{}{}); r.Error == nil || r.Error.Code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("unknown method: %+v", r)
	}

	c.shutdown()
	if strings.Contains(c.logBuf.String(), "panic") {
		t.Fatalf("log:\n%s", c.logBuf.String())
	}
}

// TestNoConfig は hozo.toml のないワークスペースで LSP-1 が各 .hozo の 1 行 1 列に出ること
// （docs/testing.md「単体テスト」の internal/lsp、docs/lsp.md「プロジェクト」）。CFG-* は出ない。
func TestNoConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	writeFile(t, filepath.Join(root, "sub", "b.hozo"), "namespace b {\n  model M { x: string; }\n}\n")
	c := startServer(t, Options{})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	for _, rel := range []string{"a.hozo", "sub/b.hozo"} {
		uri := fileURI(filepath.Join(root, filepath.FromSlash(rel)))
		diags := byURI[uri]
		if len(diags) != 1 || diags[0].Code != codeNoConfig || diags[0].Severity != protocol.SeverityWarning || diags[0].Range != (protocol.Range{}) {
			t.Fatalf("%s: %+v", rel, diags)
		}
	}
	for uri, diags := range byURI {
		for _, d := range diags {
			if strings.HasPrefix(d.Code, "CFG-") {
				t.Fatalf("%s: CFG の診断 %+v", uri, d)
			}
		}
	}
	c.shutdown()
}

// TestConfigDiagnostic は位置のない診断（hozo.toml の誤り）が hozo.toml の URI の 1 行 1 列に付くこと
// （docs/lsp.md「診断」）。Options.ConfigPath で hozo.toml を指す。
func TestConfigDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "conf", "hozo.toml"), "[hozo]\nbogus = 1\n")
	writeFile(t, filepath.Join(root, "conf", "a.hozo"), "namespace a {\n  fn f();\n}\n")
	c := startServer(t, Options{ConfigPath: filepath.Join(root, "conf", "hozo.toml")})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	tomlURI := fileURI(filepath.Join(root, "conf", "hozo.toml"))
	diags := byURI[tomlURI]
	if len(diags) == 0 || !strings.HasPrefix(diags[0].Code, "CFG-") || diags[0].Range != (protocol.Range{}) {
		t.Fatalf("hozo.toml の診断 = %+v (all: %v)", diags, keysOf(byURI))
	}
	for uri, ds := range byURI {
		if hasCode(ds, codeNoConfig) {
			t.Fatalf("%s: 設定があるのに LSP-1", uri)
		}
	}
	c.shutdown()
}

// TestPositionEncoding はサロゲートペア（UTF-16 で 2 単位、UTF-8 で 4 バイト）を含む行で、診断の列が
// 交渉したエンコーディングの単位になること（docs/lsp.md「通信」）。
func TestPositionEncoding(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "hozo.toml"), "[hozo]\nsources = [\"**/*.hozo\"]\n")
	text := "namespace a {\n  @valid.pattern(\"😀\") scalar S extends Nope;\n}\n"
	writeFile(t, filepath.Join(root, "a.hozo"), text)
	uri := fileURI(filepath.Join(root, "a.hozo"))
	for _, tc := range []struct {
		offered []string
		enc     encoding
		char    int
	}{
		{nil, encUTF16, 1 + len(`@valid.pattern("`) + 2 + len(`") scalar S extends `) + 1},
		{[]string{"utf-8"}, encUTF8, 1 + len(`@valid.pattern("`) + 4 + len(`") scalar S extends `) + 1},
	} {
		c := startServer(t, Options{})
		res, byURI := c.initialize(root, protocol.ClientCapabilities{General: &protocol.GeneralClientCapabilities{PositionEncodings: tc.offered}})
		if res.Capabilities.PositionEncoding != string(tc.enc) {
			t.Fatalf("encoding = %s", res.Capabilities.PositionEncoding)
		}
		var res1 *protocol.Diagnostic
		for i := range byURI[uri] {
			if byURI[uri][i].Code == "RES-1" {
				res1 = &byURI[uri][i]
			}
		}
		if res1 == nil {
			t.Fatalf("%s: RES-1 がない: %v", tc.enc, byURI)
		}
		want := protocol.Position{Line: 1, Character: tc.char}
		if res1.Range.Start != want || res1.Range.End != (protocol.Position{Line: 1, Character: tc.char + 4}) {
			t.Fatalf("%s: RES-1 range = %+v, want start %+v", tc.enc, res1.Range, want)
		}
		if want != positionOf(t, tc.enc, text, "Nope", 0) {
			t.Fatalf("%s: positionOf = %+v", tc.enc, positionOf(t, tc.enc, text, "Nope", 0))
		}
		// 逆向き: その位置でのホバーは同じトークン（Nope は解決できないので null。pattern の valid はデコレータ）
		var hov *protocol.Hover
		c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(uri), Position: positionOf(t, tc.enc, text, "pattern", 0)}, &hov)
		if hov == nil || !strings.Contains(hov.Contents.Value, "decorator valid.pattern(") {
			t.Fatalf("%s: hover(pattern) = %+v", tc.enc, hov)
		}
		if hov.Range == nil || hov.Range.Start != positionOf(t, tc.enc, text, "pattern", 0) || hov.Range.End.Character-hov.Range.Start.Character != len("pattern") {
			t.Fatalf("%s: hover range = %+v", tc.enc, hov.Range)
		}
		c.shutdown()
	}
}

// TestLifecycleErrors は initialize の前の要求が ServerNotInitialized になり、initialize なしの exit でも
// Serve が戻ること。
func TestLifecycleErrors(t *testing.T) {
	c := startServer(t, Options{Root: t.TempDir()})
	if r := c.call(protocol.MethodHover, protocol.TextDocumentPositionParams{}); r.Error == nil || r.Error.Code != jsonrpc.CodeServerNotInitialized {
		t.Fatalf("before initialize: %+v", r)
	}
	c.notify(protocol.MethodDidOpen, protocol.DidOpenTextDocumentParams{}) // 無視される
	c.notify(protocol.MethodExit, nil)
	<-c.done
}

// TestDebounce は didChange が最後の変更の後にまとめて一度だけ解析されること（docs/lsp.md「プロジェクト」）。
func TestDebounce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "hozo.toml"), "[hozo]\n")
	writeFile(t, filepath.Join(root, "a.hozo"), "namespace a {\n  fn f();\n}\n")
	uri := fileURI(filepath.Join(root, "a.hozo"))
	c := startServer(t, Options{})
	c.initialize(root, protocol.ClientCapabilities{})
	c.notify(protocol.MethodDidOpen, protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: uri, Text: "namespace a {\n  fn f();\n}\n"}})
	// 三つの変更を続けて送る。最後のものだけが診断になる。
	for i, text := range []string{"namespace a {\n  fn f(): Nope1;\n}\n", "namespace a {\n  fn f(): Nope2;\n}\n", "namespace a {\n  fn f(): Nope3;\n}\n"} {
		c.notify(protocol.MethodDidChange, protocol.DidChangeTextDocumentParams{
			TextDocument:   protocol.VersionedTextDocumentIdentifier{URI: uri, Version: i + 2},
			ContentChanges: []protocol.TextDocumentContentChangeEvent{{Text: text}},
		})
	}
	diags := c.waitDiagnostics(uri)
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "Nope3") {
		t.Fatalf("diags = %+v", diags)
	}
	// 範囲付きの変更にも耐える（Nope3 → string で誤りが消える）
	pos := positionOf(t, encUTF16, "namespace a {\n  fn f(): Nope3;\n}\n", "Nope3", 0)
	c.notify(protocol.MethodDidChange, protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{URI: uri, Version: 5},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{{
			Range: &protocol.Range{Start: pos, End: protocol.Position{Line: pos.Line, Character: pos.Character + 5}},
			Text:  "string",
		}},
	})
	if diags = c.waitDiagnostics(uri); len(diags) != 0 {
		t.Fatalf("after fix: %+v", diags)
	}
	c.shutdown()
}

func summarize(syms []protocol.DocumentSymbol) []string {
	var out []string
	for _, s := range syms {
		out = append(out, fmt.Sprintf("%s/%d(%s)", s.Name, s.Kind, s.Detail))
	}
	return out
}

func names(syms []protocol.DocumentSymbol) []string {
	var out []string
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out
}

func keysOf(m map[string][]protocol.Diagnostic) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+"="+strings.Join(codes(v), ","))
	}
	sort.Strings(out)
	return out
}

// shift は同じ行で桁をずらす（ASCII の範囲）。
func shift(p protocol.Position, n int) protocol.Position {
	p.Character += n
	return p
}

// TestStateClassification は、状態を分類で表す決定（docs/decisions.md「設計書から離れた決定」）の構文に対する、
// 診断の一致・ホバーの見出し（docs/lsp.md「ホバー」）・定義へジャンプ・ドキュメントのシンボル。
// コーパスは tests/corpus/state（README.md）。
func TestStateClassification(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "tests", "corpus", "state"))
	if err != nil {
		t.Fatal(err)
	}
	c := startServer(t, Options{})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	enc := encUTF16
	// 言語サーバーと check の一致（docs/testing.md）: このコーパスには診断がない
	p := loadCorpus(t, root)
	diffKeys(t, checkKeys(p), lspKeys(t, p, enc, byURI))
	if len(p.Diags) != 0 {
		t.Fatalf("tests/corpus/state に診断がある: %d", len(p.Diags))
	}

	path := filepath.Join(root, "shop.hozo")
	uri, text := fileURI(path), readFile(t, path)
	hover := func(needle string, n, shiftBy int) string {
		t.Helper()
		var hov *protocol.Hover
		c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(uri), Position: shift(positionOf(t, enc, text, needle, n), shiftBy)}, &hov)
		if hov == nil {
			t.Fatalf("hover(%q) = nil", needle)
		}
		return hov.Contents.Value
	}
	definition := func(needle string, n, shiftBy int) *protocol.Location {
		t.Helper()
		var loc *protocol.Location
		c.result(protocol.MethodDefinition, protocol.TextDocumentPositionParams{TextDocument: textDoc(uri), Position: shift(positionOf(t, enc, text, needle, n), shiftBy)}, &loc)
		return loc
	}

	// ホバーの見出し: Hozo の構文で書いた宣言（docs/lsp.md「ホバー」の 1）
	for _, tt := range []struct {
		needle  string
		shiftBy int
		want    string
	}{
		// 効果の宛先: 組み込みの効果と、分類の遷移
		// 引数を持つ効果（節の要素と、配線の呼び出しの名前のどちらからも）
		{"effects addLine {", len("effects "), "```hozo\neffect fn addLine(line: Line): effects carts.post\n```"},
		{"{ addLine(line); }", len("{ "), "```hozo\neffect fn addLine(line: Line): effects carts.post\n```"},
		{"effects placeOrder throws", len("effects "), "```hozo\neffect fn placeOrder(): effects Checkout as Shopping -> Ordered\n```"},
		{"effects startOver {", len("effects "), "```hozo\neffect fn startOver(): effects Checkout as any -> Shopping\n```"},
		{"effects clearCart {", len("effects "), "```hozo\neffect fn clearCart(): effects carts as HasItems -> Empty\n```"},
		{"touchCheckout(): effects", 0, "```hozo\neffect fn touchCheckout(): effects Checkout as Shopping, Ordered -> any\n```"},
		{"saveToken(token); }", 0, "```hozo\neffect fn saveToken(token: Token): effects token as Empty -> HasItems\n```"},
		// 分類の対象: 状態の分類は状態の名前の並び、値の分類は型
		{"match (by Checkout)", len("match (by "), "```hozo\nclassify Checkout of carts, orders\n```"},
		{"match (by LoggedIn)", len("match (by "), "```hozo\nclassify LoggedIn of token\n```"},
		{"match (line by LineKind", len("match (line by "), "```hozo\nclassify LineKind of Line\n```"},
		// 状態の形
		{"match (token by nullable", len("match ("), "```hozo\nstate token: nullable<Token>\n```"},
		{"match (carts by list", len("match ("), "```hozo\nstate carts: list<Line>\n```"},
		// 遷移のケース名は分類のケース
		{"Checkout as Shopping -> Ordered", len("Checkout as "), "classify Checkout of carts, orders\n  Shopping"},
		// 束縛のホバーは式を短く書く: 状態の分類での match
		{"return placed;", len("return "), "match (by Checkout) { ... }"},
	} {
		if got := hover(tt.needle, 0, tt.shiftBy); !strings.Contains(got, tt.want) {
			t.Errorf("hover(%q) に %q がない:\n%s", tt.needle, tt.want, got)
		}
	}
	if got := hover("Checkout as Shopping -> Ordered", 0, len("Checkout as ")); !strings.Contains(got, "カートに入れている途中") {
		t.Errorf("hover(遷移のケース Shopping) にケースの説明がない:\n%s", got)
	}

	// 定義へジャンプ: 遷移の宛先（分類、基底の状態）、遷移のケース、match の対象の状態、match by の分類
	for _, tt := range []struct {
		what, needle string
		shiftBy      int
		target       string // 定義の位置（この文字列の先頭）
	}{
		{"遷移の宛先の分類", "placeOrder(): effects Checkout", len("placeOrder(): effects "), "Checkout of carts, orders"},
		{"遷移の後のケース", "Shopping -> Ordered;", len("Shopping -> "), "Ordered;\n  }"},
		{"遷移の宛先の基底の状態", "clearCart(): effects carts", len("clearCart(): effects "), "carts: list<Line>"},
		{"効果の呼び出しの名前", "{ clearCart(); }", len("{ "), "clearCart(): effects carts"},
		{"効果の引数の型", "addLine(line: Line): effects", len("addLine(line: "), "Line { item"},
		{"分類の対象の状態", "Checkout of carts, orders", len("Checkout of carts, "), "orders: list<Order>"},
		{"match の対象の状態", "match (token by nullable", len("match ("), "token: nullable<Token>"},
		{"match by の分類", "match (by Checkout)", len("match (by "), "Checkout of carts, orders"},
		{"match by のアームのケース", "Shopping => submitOrder()", 0, "Shopping;\n"},
	} {
		loc := definition(tt.needle, 0, tt.shiftBy)
		if loc == nil || loc.URI != uri || loc.Range.Start != positionOf(t, enc, text, tt.target, 0) {
			t.Errorf("definition(%s) = %+v, want %q の位置 %+v", tt.what, loc, tt.target, positionOf(t, enc, text, tt.target, 0))
		}
	}
	// 組み込みの分類のケース（Empty / HasItems）には定義がない
	if loc := definition("HasItems -> Empty;", 0, 0); loc != nil {
		t.Errorf("definition(組み込みのケース HasItems) = %+v, want nil", loc)
	}

	// ドキュメントのシンボル: 効果の detail は宛先、分類の detail は対象
	var syms []protocol.DocumentSymbol
	c.result(protocol.MethodDocumentSymbol, protocol.DocumentSymbolParams{TextDocument: textDoc(uri)}, &syms)
	if len(syms) != 1 || syms[0].Name != "shop" {
		t.Fatalf("documentSymbol top = %+v", summarize(syms))
	}
	details := map[string]string{}
	for _, s := range syms[0].Children {
		details[s.Name] = s.Detail
	}
	for name, want := range map[string]string{
		"token":         "nullable<Token>",
		"addLine":       "(line: Line): effects carts.post",
		"placeOrder":    "(): effects Checkout as Shopping -> Ordered",
		"touchCheckout": "(): effects Checkout as Shopping, Ordered -> any",
		"clearCart":     "(): effects carts as HasItems -> Empty",
		"Checkout":      "of carts, orders",
		"LoggedIn":      "of token",
		"LineKind":      "of Line",
	} {
		if details[name] != want {
			t.Errorf("symbol %s の detail = %q, want %q", name, details[name], want)
		}
	}
	c.shutdown()
}

// TestReservedWordFields は、予約語をフィールド名に書いたとき（docs/grammar.md「識別子」の FieldName / Path）も、
// ホバーと定義へジャンプ（docs/lsp.md「ホバー」「定義へジャンプ」）がほかのフィールドと同じに働くこと。
func TestReservedWordFields(t *testing.T) {
	root := t.TempDir()
	text := `namespace a {
  model Wrap {
    /** 包む op の名前 */
    op: string;
    state: string;
  }
  fn fetch(): Wrap;
  fn store(w: Wrap): Wrap;
  fn run(): Wrap {
    val   res = fetch();
    return store({ op: res.op, state: res.state });
  }
}
`
	path := filepath.Join(root, "a.hozo")
	writeFile(t, path, text)
	c := startServer(t, Options{})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	uri, enc := fileURI(path), encUTF16
	for _, d := range byURI[uri] {
		if d.Code != codeNoConfig {
			t.Fatalf("診断が出た: %+v", d)
		}
	}
	at := func(needle string, shiftBy int) protocol.TextDocumentPositionParams {
		return protocol.TextDocumentPositionParams{TextDocument: textDoc(uri), Position: shift(positionOf(t, enc, text, needle, 0), shiftBy)}
	}
	for _, tt := range []struct {
		what    string
		pos     protocol.TextDocumentPositionParams
		target  string // 定義の位置（この文字列の先頭）
		summary string // ホバーに入る説明。なければ ""
	}{
		{"フィールドの宣言", at("op: string;", 0), "op: string;", "包む op の名前"},
		{"値のパスのフィールド", at("res.op", len("res.")), "op: string;", "包む op の名前"},
		{"値のパスのフィールド（state）", at("res.state", len("res.")), "state: string;", ""},
	} {
		var hov *protocol.Hover
		c.result(protocol.MethodHover, tt.pos, &hov)
		if hov == nil || !strings.Contains(hov.Contents.Value, "model Wrap") || !strings.Contains(hov.Contents.Value, tt.summary) {
			t.Errorf("hover(%s) = %+v", tt.what, hov)
		}
		var loc *protocol.Location
		c.result(protocol.MethodDefinition, tt.pos, &loc)
		if want := positionOf(t, enc, text, tt.target, 0); loc == nil || loc.URI != uri || loc.Range.Start != want {
			t.Errorf("definition(%s) = %+v, want %+v", tt.what, loc, want)
		}
	}
	c.shutdown()
}

// TestExtendsReadsAsThrow は、op の export / reads / never、継承した model、op の分類とケースの as、状態の参照、
// アームの throw について、ホバーの見出し（docs/lsp.md「ホバー」）と定義へジャンプ（同「定義へジャンプ」の表）を確かめる。
func TestExtendsReadsAsThrow(t *testing.T) {
	root := t.TempDir()
	text := `namespace type {
  model User {
    /** 識別子 */
    @db.key id: string;
    /** 表示名 */
    name: string;
    /** パスワードのハッシュ */
    hash: string;
  }
  model PublicUser extends User {
    override hash: never;
    /** 公開する識別子 */
    @db.unique override id;
    @db.unique override name;
  }
  exception Unauthorized;
}
namespace store {
  state users: list<type.User>;
  state current: type.User?;
  state title: string;
  read fn userById(id: string): type.User? reads users;
  export fn findUser(id: string): type.User? reads users { return userById(id); }
  fn unauthorized(): never throws type.Unauthorized;
  fn nameOf(user: type.User): string;
  classify Registration of findUser {
    Registered: findUser as type.User;
    Unregistered: findUser as else;
  }
  classify Session of current {
    LoggedIn: current as type.User;
    LoggedOut: current as else;
  }
  classify Account of findUser {
    Known: findUser as type.User;
    Unknown: findUser as else;
  }
  fn whoAmI(): type.User? {
    return match(current by nullable){
      HasItems => current;
      Empty    => none;
    };
  }
  fn heading(): string {
    return title;
  }
}
namespace server {
  export fn login(id: string): string
    throws type.Unauthorized
  {
    val   user = store.findUser(id);
    return match(user by store.Registration){
      Registered => store.nameOf2(user);
      else       => throw type.Unauthorized;
    };
  }
  export fn lookup(id: string): string
    throws type.Unauthorized
  {
    val   user = store.findUser(id);
    return match(user by store.Account){
      Known   => store.nameOf2(user);
      Unknown => throw type.Unauthorized;
    };
  }
}
namespace store {
  export fn nameOf2(user: type.User): string;
}
`
	path := filepath.Join(root, "a.hozo")
	writeFile(t, path, text)
	c := startServer(t, Options{})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	uri, enc := fileURI(path), encUTF16
	for _, d := range byURI[uri] {
		if d.Code != codeNoConfig {
			t.Fatalf("診断が出た: %+v", d)
		}
	}
	at := func(needle string, shiftBy int) protocol.TextDocumentPositionParams {
		return protocol.TextDocumentPositionParams{TextDocument: textDoc(uri), Position: shift(positionOf(t, enc, text, needle, 0), shiftBy)}
	}
	hover := func(needle string, shiftBy int) string {
		t.Helper()
		var hov *protocol.Hover
		c.result(protocol.MethodHover, at(needle, shiftBy), &hov)
		if hov == nil {
			t.Fatalf("hover(%q) = nil", needle)
		}
		return hov.Contents.Value
	}

	// ホバーの見出しと、op の reads / 純粋
	for _, tt := range []struct {
		needle  string
		shiftBy int
		wants   []string
		nots    []string
	}{
		// export、never、reads、純粋
		{"store.findUser(id)", len("store."), []string{"```hozo\nexport fn store.findUser(id: string): type.User?\n```", "\nreads:\n- `store.users`\n"}, []string{"純粋"}},
		{"unauthorized(): never", 0, []string{"```hozo\nfn unauthorized(): never\n```", "throws:\n- `type.Unauthorized`", "\n純粋\n"}, []string{"reads:"}},
		{"nameOf(user: type.User)", 0, []string{"```hozo\nfn nameOf(user: type.User): string\n```", "\n純粋\n"}, []string{"export"}},
		// 配線の op の reads は、呼び出し先から伝わったもの
		{"login(id: string)", 0, []string{"```hozo\nexport fn login(id: string): string\n```", "\nreads:\n- `store.users`\n"}, []string{"純粋"}},
		// 継承した model
		{"PublicUser extends User", 0, []string{"```hozo\nmodel PublicUser extends User\n```"}, nil},
		{"PublicUser extends User", len("PublicUser extends "), []string{"```hozo\nmodel User\n```"}, []string{"extends"}},
		// op の分類と、as を持つケース
		{"by store.Registration", len("by store."), []string{"```hozo\nclassify store.Registration of store.findUser\n```"}, nil},
		{"Registered => store.nameOf2", 0, []string{"classify store.Registration of store.findUser\n  Registered: store.findUser as type.User"}, nil},
		{"Registration of findUser", len("Registration of "), []string{"```hozo\nexport fn findUser(id: string): type.User?\n```"}, nil},
		{"Registered: findUser as", len("Registered: "), []string{"```hozo\nexport fn findUser(id: string): type.User?\n```"}, nil},
		{"LoggedIn: current as", len("LoggedIn: "), []string{"```hozo\nstate current: nullable<type.User>\n```"}, nil},
		// 残りのケース（as else）: 見出しは、書いたとおり `対象 as else`。as の前は、ほかのケースと同じ
		{"Unknown => throw", 0, []string{"classify store.Account of store.findUser\n  Unknown: store.findUser as else"}, []string{"type.User?"}},
		{"Unknown: findUser as else", 0, []string{"classify Account of findUser\n  Unknown: findUser as else"}, nil},
		{"Unknown: findUser as else", len("Unknown: "), []string{"```hozo\nexport fn findUser(id: string): type.User?\n```"}, nil},
		// 状態の見出しだけは形の語で書く（T? と書いた状態も nullable<T>、T と書いた状態は single<T>）。アームの中で
		// 絞り込まれていても、見出しは宣言のまま
		{"match(current by nullable)", len("match "), []string{"```hozo\nstate current: nullable<type.User>\n```"}, nil},
		{"HasItems => current;", len("HasItems => "), []string{"```hozo\nstate current: nullable<type.User>\n```"}, nil},
		{"return title;", len("return "), []string{"```hozo\nstate title: single<string>\n```"}, nil},
		{"reads users {", len("reads "), []string{"```hozo\nstate users: list<type.User>\n```"}, nil},
		// read の宣言と、配線からの read の呼び出し
		{"read fn userById(", len("read fn "), []string{"```hozo\nread fn userById(id: string): type.User? reads users\n```"}, nil},
		{"return userById(id); }", len("return "), []string{"```hozo\nread fn userById(id: string): type.User? reads users\n```"}, nil},
		{"type.User? reads users;", len("type.User? reads "), []string{"```hozo\nstate users: list<type.User>\n```"}, nil},
		// throw の例外
		{"throw type.Unauthorized", len("throw type."), []string{"```hozo\nexception type.Unauthorized\n```"}, nil},
	} {
		got := hover(tt.needle, tt.shiftBy)
		for _, want := range tt.wants {
			if !strings.Contains(got, want) {
				t.Errorf("hover(%q) に %q がない:\n%s", tt.needle, want, got)
			}
		}
		for _, not := range tt.nots {
			if strings.Contains(got, not) {
				t.Errorf("hover(%q) に %q がある:\n%s", tt.needle, not, got)
			}
		}
	}
	// 継承した model のフィールドの説明: override したフィールドは override に書いたもの（なければなし）。
	// override の名前の上では、元の model のフィールド（飛び先と同じ）
	if got := hover("override id;", len("override ")); !strings.Contains(got, "model User") || !strings.Contains(got, "識別子") {
		t.Errorf("hover(override id) は元の model のフィールド:\n%s", got)
	}

	// 定義へジャンプ
	for _, tt := range []struct {
		what, needle string
		shiftBy      int
		target       string
	}{
		{"extends の元の model", "PublicUser extends User", len("PublicUser extends "), "User {\n    /** 識別子 */"},
		{"override のフィールド", "override hash: never", len("override "), "hash: string;"},
		{"override のフィールド（デコレータを足す）", "@db.unique override id;", len("@db.unique override "), "id: string;"},
		{"分類の対象の op", "Registration of findUser", len("Registration of "), "findUser(id: string)"},
		{"ケースの op as T の op", "Registered: findUser as", len("Registered: "), "findUser(id: string)"},
		{"ケースの X as T の T", "findUser as type.User;", len("findUser as type."), "User {\n    /** 識別子 */"},
		{"ケースの X as T の X（状態）", "LoggedIn: current as", len("LoggedIn: "), "current: type.User?;"},
		{"reads の状態", "reads users {", len("reads "), "users: list<type.User>;"},
		{"read fn r(…): T reads s の s", "type.User? reads users;", len("type.User? reads "), "users: list<type.User>;"},
		{"read の呼び出し", "return userById(id); }", len("return "), "userById(id: string): type.User? reads"},
		{"read の結果の型", "userById(id: string): type.User? reads", len("userById(id: string): type."), "User {\n    /** 識別子 */"},
		{"配線の値の位置の状態（match の対象）", "match(current by nullable)", len("match "), "current: type.User?;"},
		{"配線の値の位置の状態（アームの値）", "HasItems => current;", len("HasItems => "), "current: type.User?;"},
		{"配線の値の位置の状態（return）", "return title;", len("return "), "title: string;"},
		{"match x by ns.C の C", "by store.Registration", len("by store."), "Registration of findUser"},
		{"アームのケース名", "Registered => store.nameOf2", 0, "Registered: findUser as"},
		{"残りのケースの X as else の X", "Unknown: findUser as else", len("Unknown: "), "findUser(id: string)"},
		{"アームのケース名（残りのケース）", "Unknown => throw", 0, "Unknown: findUser as else"},
		{"throw E の E", "throw type.Unauthorized", len("throw type."), "Unauthorized;\n}"},
	} {
		var loc *protocol.Location
		c.result(protocol.MethodDefinition, at(tt.needle, tt.shiftBy), &loc)
		if want := positionOf(t, enc, text, tt.target, 0); loc == nil || loc.URI != uri || loc.Range.Start != want {
			t.Errorf("definition(%s) = %+v, want %q の位置 %+v", tt.what, loc, tt.target, want)
		}
	}
	// 標準の分類の名前（by の後ろの nullable）とケース（Empty / HasItems）は宣言を持たないので null
	for _, tt := range []struct {
		needle  string
		shiftBy int
	}{{"by nullable){", len("by ")}, {"HasItems => current;", 0}, {"Empty    => none;", 0},
		// 残りのケースの `X as else` の else も、宣言を持たない（docs/lsp.md「定義へジャンプ」）
		{"Unknown: findUser as else", len("Unknown: findUser as ")},
		// アームの頭の else も、一つのケースを指さない
		{"else       => throw type.Unauthorized;", 0}} {
		var loc *protocol.Location
		c.result(protocol.MethodDefinition, at(tt.needle, tt.shiftBy), &loc)
		if loc != nil {
			t.Errorf("definition(%q) = %+v, want nil", tt.needle, loc)
		}
	}

	// ドキュメントのシンボル: 継承した model は、本体に書いたメンバー（Field と Override）だけが子になる
	var syms []protocol.DocumentSymbol
	c.result(protocol.MethodDocumentSymbol, protocol.DocumentSymbolParams{TextDocument: textDoc(uri)}, &syms)
	var public *protocol.DocumentSymbol
	details := map[string]string{}
	for i := range syms {
		for j := range syms[i].Children {
			s := &syms[i].Children[j]
			details[s.Name] = s.Detail
			if s.Name == "PublicUser" {
				public = s
			}
		}
	}
	if public == nil || public.Detail != "extends User" || strings.Join(names(public.Children), ",") != "hash,id,name" {
		t.Errorf("PublicUser のシンボル = %+v", public)
	}
	for name, want := range map[string]string{"users": "list<type.User>", "current": "type.User?", "title": "string", "Registration": "of findUser"} {
		if details[name] != want {
			t.Errorf("symbol %s の detail = %q, want %q", name, details[name], want)
		}
	}
	c.shutdown()
}

// TestRowTransitionAndAbsentCase は、版 0.9 の構文（行の遷移 `effect fn …: effects s.op as …`、ない側のケース `対象 as none`）について、
// ホバーの見出し（docs/lsp.md「ホバー」）、定義へジャンプ（同「定義へジャンプ」の表）、ドキュメントのシンボル、診断（STATE-9）を確かめる。
func TestRowTransitionAndAbsentCase(t *testing.T) {
	root := t.TempDir()
	text := `namespace kind {
  /** 注文の段階 */
  classify Stage of shop.Order {
    Draft;
    /** 受け付けた */
    Accepted;
    Shipped;
  }
}
namespace shop {
  model Order { id: string; }
  state orders: list<Order>;
  classify Life of Order? {
    Open: Order? as Order;
    Closed: Order? as Order;
    /** ない */
    Absent: Order? as none;
  }
  effect fn accept(order: Order): effects orders.post as kind.Stage.Accepted;
  effect fn open(order: Order): effects orders.post as Life.Absent -> Life.Open;
  effect fn bring(order: Order): effects orders.post as any;
  effect fn ship(order: Order, at: datetime): effects orders.put as kind.Stage.Draft, kind.Stage.Accepted -> kind.Stage.Shipped;
  effect fn revise(order: Order): effects orders.put as kind.Stage.Accepted -> else;
  effect fn patch(order: Order): effects orders.put as any -> else;
  effect fn discard(order: Order): effects orders.delete as Life.Open -> Life.Absent;
  effect fn purge(order: Order): effects orders.delete as any -> Life.Absent;
  fn send(order: Order, at: datetime) {
    match(order by kind.Stage){
      Accepted => ship(order, at);
      else     => none;
    };
  }
  fn wrong(order: Order, at: datetime) {
    ship(order, at);
  }
}
`
	path := filepath.Join(root, "a.hozo")
	writeFile(t, path, text)
	c := startServer(t, Options{})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	uri, enc := fileURI(path), encUTF16
	// 診断は、分かっていない行で呼んだ STATE-9 が一つ（位置は、渡した値）
	var state9 []protocol.Diagnostic
	for _, d := range byURI[uri] {
		switch d.Code {
		case codeNoConfig:
		case "STATE-9":
			state9 = append(state9, d)
		default:
			t.Fatalf("診断が出た: %+v", d)
		}
	}
	if want := shift(positionOf(t, enc, text, "ship(order, at);\n  }\n}", 0), len("ship(")); len(state9) != 1 || state9[0].Range.Start != want {
		t.Errorf("STATE-9 = %+v, want 一つ（位置 %+v）", state9, want)
	}
	at := func(needle string, shiftBy int) protocol.TextDocumentPositionParams {
		return protocol.TextDocumentPositionParams{TextDocument: textDoc(uri), Position: shift(positionOf(t, enc, text, needle, 0), shiftBy)}
	}
	hover := func(needle string, shiftBy int) string {
		t.Helper()
		var h *protocol.Hover
		c.result(protocol.MethodHover, at(needle, shiftBy), &h)
		if h == nil {
			t.Fatalf("hover(%q) が null", needle)
		}
		return h.Contents.Value
	}
	// ホバーの見出し: 行の遷移は、IR から、仕様の書き方で組み立てる（同じ意味の二つの書き方は、一つの形）。分類の名前は、
	// 見ている名前空間から見た修飾
	for _, tt := range []struct {
		needle  string
		shiftBy int
		want    string
	}{
		{"effect fn accept(", len("effect fn "), "```hozo\neffect fn accept(order: Order): effects orders.post as kind.Stage.Accepted\n```"},
		{"effect fn open(", len("effect fn "), "```hozo\neffect fn open(order: Order): effects orders.post as Life.Open\n```"},
		// post の as any は、as を書かないのと同じ形（IR に row がない）
		{"effect fn bring(", len("effect fn "), "```hozo\neffect fn bring(order: Order): effects orders.post\n```"},
		{"effect fn ship(", len("effect fn "), "```hozo\neffect fn ship(order: Order, at: datetime): effects orders.put as kind.Stage.Draft, kind.Stage.Accepted -> kind.Stage.Shipped\n```"},
		{"effect fn revise(", len("effect fn "), "```hozo\neffect fn revise(order: Order): effects orders.put as kind.Stage.Accepted -> else\n```"},
		{"effect fn patch(", len("effect fn "), "```hozo\neffect fn patch(order: Order): effects orders.put as any -> else\n```"},
		{"effect fn discard(", len("effect fn "), "```hozo\neffect fn discard(order: Order): effects orders.delete as Life.Open -> Life.Absent\n```"},
		{"effect fn purge(", len("effect fn "), "```hozo\neffect fn purge(order: Order): effects orders.delete as Life.Absent\n```"},
		// 配線の呼び出しの名前は、呼び出し先の効果の見出し
		{"Accepted => ship(order, at);", len("Accepted => "), "```hozo\neffect fn ship(order: Order, at: datetime): effects orders.put as kind.Stage.Draft, kind.Stage.Accepted -> kind.Stage.Shipped\n```"},
		// 行の遷移のケース: 分類のセグメントは分類、ケースのセグメントは、そのケース
		{"orders.post as kind.Stage.Accepted;", len("orders.post as kind."), "```hozo\nclassify kind.Stage of Order\n```"},
		{"orders.post as kind.Stage.Accepted;", len("orders.post as kind.Stage."), "classify kind.Stage of Order\n  Accepted"},
		{"as Life.Open -> Life.Absent;", len("as Life.Open -> Life."), "classify Life of Order?\n  Absent: Order? as none"},
		// ない側のケース（as none）: 見出しは、書いたとおり `対象 as none`
		{"Absent: Order? as none;", 0, "classify Life of Order?\n  Absent: Order? as none"},
	} {
		if got := hover(tt.needle, tt.shiftBy); !strings.Contains(got, tt.want) {
			t.Errorf("hover(%q) に %q がない:\n%s", tt.needle, tt.want, got)
		}
	}
	if got := hover("orders.post as kind.Stage.Accepted;", len("orders.post as kind.Stage.")); !strings.Contains(got, "受け付けた") {
		t.Errorf("行の遷移のケースのホバーに、ケースの説明がない:\n%s", got)
	}

	// 定義へジャンプ
	for _, tt := range []struct {
		what, needle string
		shiftBy      int
		target       string
	}{
		{"行の遷移のケースの分類（別の名前空間）", "orders.post as kind.Stage.Accepted;", len("orders.post as kind."), "Stage of shop.Order"},
		{"行の遷移のケース（矢印のない形）", "orders.post as kind.Stage.Accepted;", len("orders.post as kind.Stage."), "Accepted;\n    Shipped;"},
		{"行の遷移の前のケース（二つ目）", "kind.Stage.Draft, kind.Stage.Accepted -> ", len("kind.Stage.Draft, kind.Stage."), "Accepted;\n    Shipped;"},
		{"行の遷移の後のケース", "-> kind.Stage.Shipped;", len("-> kind.Stage."), "Shipped;\n  }"},
		{"行の遷移のケースの分類（同じ名前空間）", "as Life.Open -> Life.Absent;", len("as "), "Life of Order?"},
		{"行の遷移の、ない側のケース", "as Life.Open -> Life.Absent;", len("as Life.Open -> Life."), "Absent: Order? as none;"},
		{"ケースの X as none の X", "Absent: Order? as none;", len("Absent: "), "Order { id: string; }"},
	} {
		var loc *protocol.Location
		c.result(protocol.MethodDefinition, at(tt.needle, tt.shiftBy), &loc)
		if want := positionOf(t, enc, text, tt.target, 0); loc == nil || loc.URI != uri || loc.Range.Start != want {
			t.Errorf("definition(%s) = %+v, want %q の位置 %+v", tt.what, loc, tt.target, want)
		}
	}
	// 行の遷移の any と else、ケースの as none の none は、宣言を持たないので null
	for _, tt := range []struct {
		needle  string
		shiftBy int
	}{{"orders.post as any;", len("orders.post as ")}, {"as any -> else;", len("as ")}, {"as any -> else;", len("as any -> ")},
		{"Absent: Order? as none;", len("Absent: Order? as ")}} {
		var loc *protocol.Location
		c.result(protocol.MethodDefinition, at(tt.needle, tt.shiftBy), &loc)
		if loc != nil {
			t.Errorf("definition(%q) = %+v, want nil", tt.needle, loc)
		}
	}

	// ドキュメントのシンボル: 効果の detail は、引数と、宛先と、書いたままの行の遷移
	var syms []protocol.DocumentSymbol
	c.result(protocol.MethodDocumentSymbol, protocol.DocumentSymbolParams{TextDocument: textDoc(uri)}, &syms)
	details := map[string]string{}
	for i := range syms {
		for j := range syms[i].Children {
			details[syms[i].Children[j].Name] = syms[i].Children[j].Detail
		}
	}
	for name, want := range map[string]string{
		"accept": "(order: Order): effects orders.post as kind.Stage.Accepted",
		"open":   "(order: Order): effects orders.post as Life.Absent -> Life.Open",
		"ship":   "(order: Order, at: datetime): effects orders.put as kind.Stage.Draft, kind.Stage.Accepted -> kind.Stage.Shipped",
		"patch":  "(order: Order): effects orders.put as any -> else",
	} {
		if details[name] != want {
			t.Errorf("symbol %s の detail = %q, want %q", name, details[name], want)
		}
	}
	c.shutdown()
}

// TestCarryingDecorators は、版 0.5 の構文（予約語のデコレータ名、reads / effects の節、残りの引数、対象 effect）について、
// 定義へジャンプ（docs/lsp.md「定義へジャンプ」の表）、ホバーの見出し（同「ホバー」）、ドキュメントのシンボル、診断を確かめる。
// 運ぶ節を持つデコレータは、配線の op にも付けられる（docs/check.md「デコレータが運ぶ reads と effects」）: 付けただけでは診断は出ず、
// 運ばれたものは、その配線と、それを呼ぶ配線のホバーに出る。書いた節が運ばれたものを落としていれば EFF-2。
func TestCarryingDecorators(t *testing.T) {
	root := t.TempDir()
	text := `namespace store {
  model Row { @db.text id: string; }
  @db.table
  state rows: list<Row>;
  /** 表から引く */
  decorator list(table: any state, column: string?) for op reads table;
  /**
   * 並べた効果を行う。
   *
   * @param targets 行う効果
   */
  decorator effects(...targets: any effect) for op effects targets reads rows;
  decorator into(table: any state) for effect;
  decorator everything = list(rows);
  @into(rows)
  effect fn add(row: Row): effects rows.post;
  /**
   * 忙しい
   *
   * @param wait 待つ時間
   */
  exception Busy(wait: duration, note: string?);
  /**
   * id で引く
   *
   * @param id 引く行の識別子
   */
  @db.find @db.where("id", "x") @db.where("id", "y")
  read fn byId(id: string): Row? reads rows;
  classify Size of Row { Big; Small; }
  fn save(row: Row): Row? throws Busy {
    add(row);
    rows.post(row);
    return match(row by Size){
      Big     => throw Busy(5m, "big");
      else    => byId(row.id);
    };
  }
  /**
   * 全部を引く。
   *
   * @see list
   * @see store.effects
   */
  @list(rows)
  fn all(): Row[];
  @effects(add)
  fn insert(row: Row);
  @store.effects
  fn idle();
  @everything
  fn viaAlias(): Row[];
  @list(rows)
  fn wired(): Row[] {
    return all();
  }
  @effects(add)
  fn wiredWrite(): Row[] {
    return all();
  }
  fn viaWired(): Row[] {
    return wiredWrite();
  }
  @effects(add)
  fn wiredShort(row: Row): effects rows.post {
    rows.post(row);
  }
}
`
	path := filepath.Join(root, "a.hozo")
	writeFile(t, path, text)
	c := startServer(t, Options{})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	uri, enc := fileURI(path), encUTF16
	// 診断: 運ぶ節を持つデコレータを配線に付けた wired と wiredWrite には、何も出ない。出るのは、書いた effects が、運ばれた add を
	// 落としている wiredShort の EFF-2 だけ（位置は op。デコレータの適用から始まる）
	var codes []string
	for _, d := range byURI[uri] {
		if d.Code == codeNoConfig {
			continue
		}
		codes = append(codes, d.Code)
		if want := positionOf(t, enc, text, "@effects(add)\n  fn wiredShort", 0); d.Code == "EFF-2" && d.Range.Start != want {
			t.Errorf("EFF-2 の位置 = %+v, want %+v", d.Range.Start, want)
		}
		if d.Code == "EFF-2" && !strings.Contains(d.Message, "足りない: store.add") {
			t.Errorf("EFF-2 のメッセージ = %q, want 運ばれた store.add が足りないこと", d.Message)
		}
	}
	if strings.Join(codes, ",") != "EFF-2" {
		t.Fatalf("診断 = %v, want [EFF-2]", codes)
	}
	at := func(needle string, shiftBy int) protocol.TextDocumentPositionParams {
		return protocol.TextDocumentPositionParams{TextDocument: textDoc(uri), Position: shift(positionOf(t, enc, text, needle, 0), shiftBy)}
	}
	const (
		listHead    = "decorator list(table: any state, column: string?) reads table"
		effectsHead = "decorator effects(...targets: any effect) reads rows effects targets"
	)
	for _, tt := range []struct {
		what   string
		pos    protocol.TextDocumentPositionParams
		target string // 定義の位置（この文字列の先頭から skip 文字目）
		skip   int
		hover  []string
	}{
		{"予約語のデコレータ名の宣言", at("decorator list(", len("decorator ")), "decorator list(", len("decorator "), []string{listHead, "表から引く"}},
		{"予約語のデコレータ名の適用", at("@list(rows)\n  fn all", 1), "decorator list(", len("decorator "), []string{listHead, "表から引く"}},
		{"修飾しない予約語の適用（@effects）", at("@effects(add)", 1), "decorator effects(", len("decorator "), []string{effectsHead, "並べた効果を行う。"}},
		{"修飾した予約語の適用（@store.effects）", at("@store.effects\n", len("@store.")), "decorator effects(", len("decorator "), []string{effectsHead}},
		{"reads の節の引数名", at("for op reads table;", len("for op reads ")), "table: any state, column", 0, []string{listHead, "  table: any state"}},
		{"effects の節の、残りの引数の名前", at("for op effects targets", len("for op effects ")), "targets: any effect) for op", 0, []string{effectsHead, "  ...targets: any effect", "行う効果"}},
		{"節に書いた状態の名前", at("effects targets reads rows;", len("effects targets reads ")), "rows: list<Row>;", 0, []string{"state rows: list<Row>"}},
		{"残りの引数の宣言", at("...targets: any effect", len("...")), "targets: any effect) for op", 0, []string{"  ...targets: any effect"}},
		{"残りの引数に渡した効果", at("@effects(add)", len("@effects(")), "add(row: Row): effects rows.post;", 0, []string{"effect fn add(row: Row): effects rows.post"}},
		// 版 0.6: 引数を持つ exception / effect / read、配線からの呼び出し、引数を持つ throw
		{"効果の呼び出し", at("    add(row);", len("    ")), "add(row: Row): effects rows.post;", 0, []string{"effect fn add(row: Row): effects rows.post"}},
		{"直に呼ぶ「状態.操作」の状態", at("    rows.post(row);", len("    ")), "rows: list<Row>;", 0, []string{"state rows: list<Row>"}},
		{"read の呼び出し", at("else    => byId(row.id);", len("else    => ")), "byId(id: string): Row?", 0, []string{"read fn byId(id: string): Row? reads rows", "id で引く"}},
		{"throw E(…) の E", at("throw Busy(5m", len("throw ")), "Busy(wait: duration", 0, []string{"exception Busy(wait: duration, note: string?)", "忙しい"}},
		{"read の引数", at("byId(id: string): Row?", len("byId(")), "id: string): Row? reads", 0, []string{"read fn byId(id: string): Row? reads rows", "  id: string", "引く行の識別子"}},
		{"例外の @param", at("@param wait", len("@param ")), "wait: duration, note", 0, []string{"exception Busy(wait: duration, note: string?)", "  wait: duration", "待つ時間"}},
		{"read に付けた、何度も付けられるデコレータ", at("@db.where(\"id\", \"y\")", len("@db.")), "", -1, []string{"@repeatable decorator db.where(column: string, value: string | int | float | boolean) for read"}},
		{"効果の宣言に付けたデコレータ", at("@into(rows)", 1), "decorator into(", len("decorator "), []string{"decorator into(table: any state) for effect"}},
		{"別名の先の予約語", at("= list(rows);", len("= ")), "decorator list(", len("decorator "), []string{listHead}},
		{"@see の予約語のデコレータ名", at("@see list", len("@see ")), "decorator list(", len("decorator "), []string{listHead}},
		{"@see の修飾した予約語のデコレータ名", at("@see store.effects", len("@see store.")), "decorator effects(", len("decorator "), []string{effectsHead}},
		{"@param の残りの引数", at("@param targets", len("@param ")), "targets: any effect) for op", 0, []string{"  ...targets: any effect"}},
	} {
		var hov *protocol.Hover
		c.result(protocol.MethodHover, tt.pos, &hov)
		if hov == nil {
			t.Errorf("hover(%s) = nil", tt.what)
		} else {
			for _, want := range tt.hover {
				if !strings.Contains(hov.Contents.Value, want) {
					t.Errorf("hover(%s) に %q がない:\n%s", tt.what, want, hov.Contents.Value)
				}
			}
		}
		if tt.skip < 0 {
			continue // 語彙の宣言（飛び先は下で確かめる）
		}
		var loc *protocol.Location
		c.result(protocol.MethodDefinition, tt.pos, &loc)
		if want := shift(positionOf(t, enc, text, tt.target, 0), tt.skip); loc == nil || loc.URI != uri || loc.Range.Start != want {
			t.Errorf("definition(%s) = %+v, want %+v", tt.what, loc, want)
		}
	}
	// 直に呼ぶ「状態.操作」の操作の語は宣言を持たないので null（docs/lsp.md「定義へジャンプ」）
	var none *protocol.Location
	c.result(protocol.MethodDefinition, at("    rows.post(row);", len("    rows.")), &none)
	if none != nil {
		t.Errorf("definition(直に呼ぶ操作の語 post) = %+v, want nil", none)
	}
	// 語彙の予約語のデコレータ名（@db.find）は、語彙のファイルへ飛ぶ
	var loc *protocol.Location
	c.result(protocol.MethodDefinition, at("@db.find @db.where", len("@db.")), &loc)
	if loc == nil || loc.URI != "hozo:vocab/db.hozo" {
		t.Errorf("definition(@db.find) = %+v", loc)
	}
	var hov *protocol.Hover
	c.result(protocol.MethodHover, at("@db.find @db.where", len("@db.")), &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "decorator db.find for read") {
		t.Errorf("hover(@db.find) = %+v", hov)
	}
	// 名前付きデコレータの適用は、宣言と、固定した引数
	c.result(protocol.MethodHover, at("@everything", 1), &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "decorator everything = list(table: rows)") || !strings.Contains(hov.Contents.Value, listHead) {
		t.Errorf("hover(@everything) = %+v", hov)
	}
	// op のホバー: 運ばれた状態と効果は、Signature の一覧に出る
	c.result(protocol.MethodHover, at("fn insert(", len("fn ")), &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "effects:\n- `add`") || !strings.Contains(hov.Contents.Value, "reads:\n- `rows`") {
		t.Errorf("hover(op insert) = %+v", hov)
	}
	// 配線の op のホバー: 本体（all を呼ぶだけ）は効果を持たないので、effects は、デコレータが運んだ add。それを呼ぶ配線にも伝わる。
	// reads の節だけを運ぶデコレータの付いた配線 wired は、効果を持たない
	for _, name := range []string{"wiredWrite", "viaWired"} {
		c.result(protocol.MethodHover, at("fn "+name+"(", len("fn ")), &hov)
		if hov == nil || !strings.Contains(hov.Contents.Value, "effects:\n- `add`") || !strings.Contains(hov.Contents.Value, "reads:\n- `rows`") {
			t.Errorf("hover(op %s) = %+v", name, hov)
		}
	}
	c.result(protocol.MethodHover, at("fn wired(", len("fn ")), &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "effects: なし") || !strings.Contains(hov.Contents.Value, "reads:\n- `rows`") {
		t.Errorf("hover(op wired) = %+v", hov)
	}
	// ドキュメントのシンボル: 予約語の名前のデコレータも Interface、効果は Event
	var syms []protocol.DocumentSymbol
	c.result(protocol.MethodDocumentSymbol, protocol.DocumentSymbolParams{TextDocument: textDoc(uri)}, &syms)
	if len(syms) != 1 {
		t.Fatalf("documentSymbol = %+v", summarize(syms))
	}
	kinds := map[string]protocol.SymbolKind{}
	for _, s := range syms[0].Children {
		kinds[s.Name] = s.Kind
	}
	for _, name := range []string{"list", "effects", "into", "everything"} {
		if kinds[name] != protocol.SymbolInterface {
			t.Errorf("symbol %s の種類 = %v, want Interface", name, kinds[name])
		}
	}
	if kinds["add"] != protocol.SymbolEvent {
		t.Errorf("symbol add の種類 = %v, want Event", kinds["add"])
	}
	// read は Key、exception は Class。detail は、引数の並び（と、read の結果の型、宛先）
	if kinds["byId"] != protocol.SymbolKey || kinds["Busy"] != protocol.SymbolClass {
		t.Errorf("symbol byId / Busy の種類 = %v / %v, want Key / Class", kinds["byId"], kinds["Busy"])
	}
	details := map[string]string{}
	for _, s := range syms[0].Children {
		details[s.Name] = s.Detail
	}
	for name, want := range map[string]string{
		"byId": "(id: string): Row? reads rows",
		"add":  "(row: Row): effects rows.post",
		"Busy": "(wait: duration, note: string?)",
	} {
		if details[name] != want {
			t.Errorf("symbol %s の detail = %q, want %q", name, details[name], want)
		}
	}
	c.shutdown()
}

// TestValAndCaseTargets は、R6 の言語の部分の、飛び先とホバー（docs/lsp.md「定義へ移動」の表）: val と for の変数（値を入れる文と、
// 集める文の左を含む）、デコレータの引数の分類のケース、デコレータの宣言の分類のケース。
func TestValAndCaseTargets(t *testing.T) {
	root := t.TempDir()
	text := `namespace a {
  classify Actor of never {
    @human Customer;
    Clerk;
  }
  decorator human for case of Actor;
  decorator both(until: date?, from: time) for model, case of never;
  decorator of(who: any case of never) for op;
  decorator seenBy(what: any case of Order?, any1: any case?) for model, case;
  model Order { id: string; }
  model Note { id: string; }
  state orders: list<Order>;
  read fn all(): Order[] reads orders;
  fn note(order: Order): Note;
  fn count(notes: Note[], first: Order[]): int;
  @of(Actor.Clerk)
  fn run(): int {
    val found, again;
    disjoint {
      found = all();
      again = all();
    }
    val notes: Note[];
    disjoint for (val each of found) {
      notes collect note(each);
    }
    return count(notes, again);
  }
}
`
	path := filepath.Join(root, "a.hozo")
	writeFile(t, path, text)
	c := startServer(t, Options{})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	uri, enc := fileURI(path), encUTF16
	for _, d := range byURI[uri] {
		if d.Code != codeNoConfig {
			t.Errorf("診断 %s: %s", d.Code, d.Message)
		}
	}
	at := func(needle string, shiftBy int) protocol.TextDocumentPositionParams {
		return protocol.TextDocumentPositionParams{TextDocument: textDoc(uri), Position: shift(positionOf(t, enc, text, needle, 0), shiftBy)}
	}
	for _, tt := range []struct {
		what   string
		pos    protocol.TextDocumentPositionParams
		target string // 定義の位置（この文字列の先頭から skip 文字目）
		skip   int
		hover  []string
	}{
		{"名前を並べた val の、二つめの名前", at("val found, again;", len("val found, ")), "again;\n    disjoint", 0, []string{"val again: Order[]"}},
		{"値を入れる文の左", at("found = all();", 0), "found, again;", 0, []string{"val found: Order[]"}},
		{"for の頭の、並び", at("of found) {", len("of ")), "found, again;", 0, []string{"val found: Order[]"}},
		{"for の変数の宣言", at("val each of", len("val ")), "each of found", 0, []string{"val each: Order"}},
		{"for の変数を読む所", at("note(each);", len("note(")), "each of found", 0, []string{"val each: Order"}},
		{"集める文の左", at("notes collect", 0), "notes: Note[];", 0, []string{"val notes: Note[]"}},
		{"集めた val を読む所", at("count(notes, again);", len("count(")), "notes: Note[];", 0, []string{"val notes: Note[]"}},
		{"デコレータの引数の、分類", at("@of(Actor.Clerk)", len("@of(")), "Actor of never {", 0, []string{"classify Actor of never"}},
		{"デコレータの引数の、ケース", at("@of(Actor.Clerk)", len("@of(Actor.")), "Clerk;", 0, []string{"Clerk"}},
		{"デコレータの宣言の、of の後ろの、分類の名前", at("for case of Actor;", len("for case of ")), "Actor of never {", 0, []string{"classify Actor of never"}},
		{"デコレータの宣言の、of の後ろの、型の名前", at("any case of Order?", len("any case of ")), "Order { id: string; }", 0, []string{"model Order"}},
		{"ケースの前のデコレータ", at("@human Customer;", 1), "decorator human", len("decorator "), []string{"decorator human for case of Actor"}},
		{"付ける先の並び（区切りは ,）と、日付と時刻の核の型", at("decorator both(", len("decorator ")), "decorator both(", len("decorator "), []string{"decorator both(until: date?, from: time) for model, case of never"}},
		{"分類のケースの絞りの見出し（もとの形）", at("decorator seenBy(", len("decorator ")), "seenBy(what", 0, []string{"decorator seenBy(what: any case of Order?, any1: any case of any?) for model, case of any"}},
	} {
		var hov *protocol.Hover
		c.result(protocol.MethodHover, tt.pos, &hov)
		if hov == nil {
			t.Errorf("hover(%s) = nil", tt.what)
		} else {
			for _, want := range tt.hover {
				if !strings.Contains(hov.Contents.Value, want) {
					t.Errorf("hover(%s) に %q がない:\n%s", tt.what, want, hov.Contents.Value)
				}
			}
		}
		var loc *protocol.Location
		c.result(protocol.MethodDefinition, tt.pos, &loc)
		if want := shift(positionOf(t, enc, text, tt.target, 0), tt.skip); loc == nil || loc.URI != uri || loc.Range.Start != want {
			t.Errorf("definition(%s) = %+v, want %+v", tt.what, loc, want)
		}
	}
	// 宣言を持たない語は null（docs/lsp.md「定義へ移動」の表）
	for _, tt := range []struct {
		what string
		pos  protocol.TextDocumentPositionParams
	}{
		{"デコレータの宣言の case", at("for case of Actor;", len("for "))},
		{"デコレータの宣言の of", at("for case of Actor;", len("for case "))},
		{"引数の種類の case", at("any case of never", len("any "))},
		{"引数の種類の never", at("any case of never", len("any case of "))},
	} {
		var none *protocol.Location
		c.result(protocol.MethodDefinition, tt.pos, &none)
		if none != nil {
			t.Errorf("definition(%s) = %+v, want nil", tt.what, none)
		}
	}
}

// TestEffectThrows は、例外を投げる効果（docs/grammar.md「関数の宣言」。docs/lsp.md「定義へ移動」の表と「ホバー」）: 効果の throws の
// 例外は、その exception の名前へ飛ぶ。ホバーの見出しは、throws を effects の前に出す。アウトラインの詳細は、書いたまま。
func TestEffectThrows(t *testing.T) {
	root := t.TempDir()
	text := `namespace type {
  exception Taken;
}
namespace store {
  model Row { id: string; }
  state rows: list<Row>;
  exception Full;
  effect fn add(row: Row): throws type.Taken, Full effects rows.post;
  fn put(row: Row): throws type.Taken, Full {
    add(row);
  }
}
`
	path := filepath.Join(root, "a.hozo")
	writeFile(t, path, text)
	c := startServer(t, Options{})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	uri, enc := fileURI(path), encUTF16
	for _, d := range byURI[uri] {
		if d.Code != codeNoConfig {
			t.Errorf("診断 %s: %s", d.Code, d.Message)
		}
	}
	at := func(needle string, shiftBy int) protocol.TextDocumentPositionParams {
		return protocol.TextDocumentPositionParams{TextDocument: textDoc(uri), Position: shift(positionOf(t, enc, text, needle, 0), shiftBy)}
	}
	const head = "effect fn add(row: Row): throws type.Taken, Full effects rows.post"
	for _, tt := range []struct {
		what   string
		pos    protocol.TextDocumentPositionParams
		target string
		skip   int
		hover  string
	}{
		{"効果の宣言の名前", at("add(row: Row): throws", 0), "add(row: Row): throws", 0, head},
		{"効果の呼び出し", at("    add(row);", len("    ")), "add(row: Row): throws", 0, head},
		{"効果の throws の、別の名前空間の例外", at("throws type.Taken, Full effects", len("throws type.")), "Taken;", 0, "exception type.Taken"},
		{"効果の throws の、同じ名前空間の例外", at("throws type.Taken, Full effects", len("throws type.Taken, ")), "Full;", 0, "exception Full"},
	} {
		var hov *protocol.Hover
		c.result(protocol.MethodHover, tt.pos, &hov)
		if hov == nil || !strings.Contains(hov.Contents.Value, tt.hover) {
			t.Errorf("hover(%s) に %q がない: %+v", tt.what, tt.hover, hov)
		}
		var loc *protocol.Location
		c.result(protocol.MethodDefinition, tt.pos, &loc)
		if want := shift(positionOf(t, enc, text, tt.target, 0), tt.skip); loc == nil || loc.URI != uri || loc.Range.Start != want {
			t.Errorf("definition(%s) = %+v, want %+v", tt.what, loc, want)
		}
	}
	// アウトラインの詳細: 効果の throws は、書いたまま
	var symbols []protocol.DocumentSymbol
	c.result(protocol.MethodDocumentSymbol, protocol.DocumentSymbolParams{TextDocument: textDoc(uri)}, &symbols)
	found := false
	var walk func([]protocol.DocumentSymbol)
	walk = func(list []protocol.DocumentSymbol) {
		for _, s := range list {
			if s.Name == "add" {
				found = true
				if want := "(row: Row): throws type.Taken, Full effects rows.post"; s.Detail != want {
					t.Errorf("アウトラインの詳細 = %q, want %q", s.Detail, want)
				}
			}
			walk(s.Children)
		}
	}
	walk(symbols)
	if !found {
		t.Error("アウトラインに、効果 add がない")
	}
}
