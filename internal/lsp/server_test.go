package lsp

import (
	"fmt"
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

// TestCheckAgreement は docs/testing.md「言語サーバーと check の一致」: コーパスについて project.Load の診断と
// publishDiagnostics の診断（LSP の形から戻したもの）が同じ集合であること。utf-16 と utf-8 の両方で。
func TestCheckAgreement(t *testing.T) {
	root := corpusWithWarning(t)
	want := checkKeys(project.Load(root, "", nil))
	if len(want) == 0 {
		t.Fatal("コーパスに診断がない（warn.hozo の REL-2 の warning があるはず）")
	}
	for _, encs := range [][]string{nil, {"utf-8", "utf-16"}} {
		c := startServer(t, Options{})
		res, byURI := c.initialize(root, protocol.ClientCapabilities{General: &protocol.GeneralClientCapabilities{PositionEncodings: encs}})
		enc := encoding(res.Capabilities.PositionEncoding)
		if (encs == nil && enc != encUTF16) || (encs != nil && enc != encUTF8) {
			t.Fatalf("positionEncoding = %q for %v", enc, encs)
		}
		p := project.Load(root, "", nil)
		diffKeys(t, want, lspKeys(t, p, enc, byURI))
		c.shutdown()
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
	for _, want := range []string{"```hozo\nop placeOrder(in: OrderInput): Order\n```", "throws:", "- `NotificationFailed` — prevents notifyUser, keeps createOrder", "effects:", "- `createOrder`", "- `notifyUser`"} {
		if !strings.Contains(hov.Contents.Value, want) {
			t.Errorf("hover(placeOrder) に %q がない:\n%s", want, hov.Contents.Value)
		}
	}
	// ホバー: 別の名前空間から見た op の参照（修飾あり）と説明
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: positionOf(t, enc, browserText, "addToCart({ item })", 0)}, &hov)
	if hov == nil {
		t.Fatal("hover(server.addToCart) = nil")
	}
	for _, want := range []string{"op server.addToCart(in: server.AddToCartInput): server.CartLine", "商品をカートに入れる", "- `server.addCartLine`", "throws: なし"} {
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
	if hov == nil || !strings.Contains(hov.Contents.Value, "```hozo\nmodel OrdersQuery\n  page: PosInt = 1\n```") {
		t.Errorf("hover(field page) = %+v", hov)
	}
	// 別の名前空間の model のフィールド（説明付き）: server.GetOrdersInput.page の宣言名
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(serverURI), Position: positionOf(t, enc, serverText, "page: int = 1", 0)}, &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "model GetOrdersInput\n  page: int = 1") || !strings.Contains(hov.Contents.Value, "1 から数えるページ番号") {
		t.Errorf("hover(field GetOrdersInput.page) = %+v", hov)
	}
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(browserURI), Position: positionOf(t, enc, browserText, "Yamada     =>", 0)}, &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "classify WarrantyKind of server.Item\n  Yamada") || !strings.Contains(hov.Contents.Value, "ヤマダ電機の専用保証プラン") {
		t.Errorf("hover(case Yamada) = %+v", hov)
	}
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(serverURI), Position: positionOf(t, enc, serverText, "email 再設定", 0)}, &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "op requestPasswordReset(email: Email)\n  email: Email") || !strings.Contains(hov.Contents.Value, "再設定するアカウントのメールアドレス") {
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
		"WarrantyAvailable":    protocol.SymbolEnum,      // condition
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
	if got := names(children["CampaignKind"]); strings.Join(got, ",") != "Spring,Summer" {
		t.Errorf("CampaignKind の子（default を含まない）= %v", got)
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
	writeFile(t, filepath.Join(root, "a.hozo"), "namespace a {\n  op f();\n}\n")
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
	writeFile(t, filepath.Join(root, "conf", "a.hozo"), "namespace a {\n  op f();\n}\n")
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
	writeFile(t, filepath.Join(root, "a.hozo"), "namespace a {\n  op f();\n}\n")
	uri := fileURI(filepath.Join(root, "a.hozo"))
	c := startServer(t, Options{})
	c.initialize(root, protocol.ClientCapabilities{})
	c.notify(protocol.MethodDidOpen, protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: uri, Text: "namespace a {\n  op f();\n}\n"}})
	// 三つの変更を続けて送る。最後のものだけが診断になる。
	for i, text := range []string{"namespace a {\n  op f(): Nope1;\n}\n", "namespace a {\n  op f(): Nope2;\n}\n", "namespace a {\n  op f(): Nope3;\n}\n"} {
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
	pos := positionOf(t, encUTF16, "namespace a {\n  op f(): Nope3;\n}\n", "Nope3", 0)
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

// TestStateClassification は、状態を分類で表す決定（docs/decisions.md「設計書を変える決定」）の構文に対する、
// 診断の一致・ホバーの見出し（docs/lsp.md「ホバー」）・定義へジャンプ・ドキュメントのシンボル。
// コーパスは tests/corpus/state（check だけのコーパス。README.md）。
func TestStateClassification(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "tests", "corpus", "state"))
	if err != nil {
		t.Fatal(err)
	}
	c := startServer(t, Options{})
	_, byURI := c.initialize(root, protocol.ClientCapabilities{})
	enc := encUTF16
	// 言語サーバーと check の一致（docs/testing.md）: このコーパスには診断がない
	p := project.Load(root, "", nil)
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
		{"effects addLine;", len("effects "), "```hozo\neffect addLine on carts.post\n```"},
		{"effects placeOrder throws", len("effects "), "```hozo\neffect placeOrder on Checkout: Shopping -> Ordered\n```"},
		{"effects startOver;", len("effects "), "```hozo\neffect startOver on Checkout: any -> Shopping\n```"},
		{"effects clearCart;", len("effects "), "```hozo\neffect clearCart on carts: HasItems -> Empty\n```"},
		{"touchCheckout on", 0, "```hozo\neffect touchCheckout on Checkout: Shopping | Ordered -> any\n```"},
		// 分類の対象: 状態の分類は状態の名前の並び、値の分類は型
		{"match by Checkout", len("match by "), "```hozo\nclassify Checkout of carts, orders\n```"},
		{"match by LoggedIn", len("match by "), "```hozo\ncondition LoggedIn of token\n```"},
		{"match line by LineKind", len("match line by "), "```hozo\nclassify LineKind of Line\n```"},
		// 状態の形
		{"match token by nullable", len("match "), "```hozo\nstate token: nullable<Token>\n```"},
		{"match carts by list", len("match "), "```hozo\nstate carts: list<Line>\n```"},
		// 遷移のケース名は分類のケース
		{"Checkout: Shopping -> Ordered", len("Checkout: "), "classify Checkout of carts, orders\n  Shopping"},
		// 束縛のホバーは式を短く書く: 状態の分類での match
		{"return placed;", len("return "), "match by Checkout { ... }"},
	} {
		if got := hover(tt.needle, 0, tt.shiftBy); !strings.Contains(got, tt.want) {
			t.Errorf("hover(%q) に %q がない:\n%s", tt.needle, tt.want, got)
		}
	}
	if got := hover("Checkout: Shopping -> Ordered", 0, len("Checkout: ")); !strings.Contains(got, "カートに入れている途中") {
		t.Errorf("hover(遷移のケース Shopping) にケースの説明がない:\n%s", got)
	}

	// 定義へジャンプ: 遷移の宛先（分類、基底の状態）、遷移のケース、match の対象の状態、match by の分類
	for _, tt := range []struct {
		what, needle string
		shiftBy      int
		target       string // 定義の位置（この文字列の先頭）
	}{
		{"遷移の宛先の分類", "placeOrder on Checkout", len("placeOrder on "), "Checkout of carts, orders"},
		{"遷移の後のケース", "Shopping -> Ordered;", len("Shopping -> "), "Ordered;\n  }"},
		{"遷移の宛先の基底の状態", "clearCart on carts", len("clearCart on "), "carts: list<Line>"},
		{"分類の対象の状態", "Checkout of carts, orders", len("Checkout of carts, "), "orders: list<Order>"},
		{"match の対象の状態", "match token by nullable", len("match "), "token: nullable<Token>"},
		{"match by の分類", "match by Checkout", len("match by "), "Checkout of carts, orders"},
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
		"addLine":       "on carts.post",
		"placeOrder":    "on Checkout: Shopping -> Ordered",
		"touchCheckout": "on Checkout: Shopping | Ordered -> any",
		"clearCart":     "on carts: HasItems -> Empty",
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
