package lsp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/ir"
)

// docs/lsp.md「プラグインの診断」のテスト。プラグインは、sh で動く、にせもの（pluginProject）。

// pluginSpec は、hozoc 自身の診断が、warning 一つ（REL-2: 効果を持たない op の prevents effects）だけの仕様。error が
// ないので、プラグインの check は、走る。
const pluginSpec = "namespace a {\n  exception Idle;\n  fn f(): throws Idle prevents effects;\n}\n"

// pluginSpecEdited は、pluginSpec に、コメントを一行足したもの（診断は、同じ。中身は、違う）。
const pluginSpecEdited = "// 編集した\n" + pluginSpec

// pluginSpecBroken は、hozoc 自身の診断に、error（RES-1）がある仕様。プラグインの check は、走らない。
const pluginSpecBroken = "namespace a {\n  fn f(): Missing;\n}\n"

// pluginProject は、にせもののプラグイン chk（[check] enabled）を、単位に並べた、小さなプロジェクト。
//
// プラグインは、呼ばれるたびに、自分のディレクトリの calls に一行足し、何度目の呼び出しかを数える。gate があれば、open が
// できるまで、待つ。crash があれば、終了コード 3 で終わる。そうでなければ、診断 CHK-1（位置は a.f。メッセージは
// "run <何度目>"）を返す。
type pluginProject struct {
	t    *testing.T
	root string
	dir  string // プラグインのディレクトリ
	uri  string // a.hozo
}

func newPluginProject(t *testing.T, text string) *pluginProject {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh がない")
	}
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	p := &pluginProject{t: t, root: root, dir: filepath.Join(root, "plugins", "chk"), uri: fileURI(filepath.Join(root, "a.hozo"))}
	script := filepath.Join(p.dir, "run.sh")
	writeFile(t, script, `cat >/dev/null
d=$(dirname "$0")
echo x >> "$d/calls"
n=$(wc -l < "$d/calls" | tr -d ' ')
while [ -f "$d/gate" ] && [ ! -f "$d/open" ]; do sleep 0.01; done
if [ -f "$d/crash" ]; then exit 3; fi
printf '{"diagnostics":[{"severity":"error","code":"CHK-1","at":"a.f","message":"run %s"}]}' "$n"
`)
	writeFile(t, filepath.Join(p.dir, "hozo-plugin.toml"),
		"name = \"chk\"\ncommand = [\"sh\", "+strconv.Quote(script)+"]\nprotocol = \""+ir.Version+"\"\n\n[check]\nenabled = true\n\n[[transpiler]]\nname = \"chk\"\nlanguage = \"go\"\n")
	writeFile(t, filepath.Join(root, "hozo.toml"), "[hozo]\nplugin_dirs = [\"plugins\"]\n\n[[emit]]\nnamespace = \"a\"\ntranspilers = [\"chk\"]\nout = \"gen/a\"\n")
	writeFile(t, filepath.Join(root, "a.hozo"), text)
	return p
}

// calls は、プラグインが、これまでに起動した回数。
func (p *pluginProject) calls() int {
	data, err := os.ReadFile(filepath.Join(p.dir, "calls"))
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "x")
}

// waitCalls は、プラグインの起動が、n 回になるのを待つ。
func (p *pluginProject) waitCalls(n int) {
	p.t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for p.calls() < n {
		if time.Now().After(deadline) {
			p.t.Fatalf("プラグインの起動が %d 回にならない（今は %d 回）", n, p.calls())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// gate は、これからのプラグインの起動を、release まで、待たせる。
func (p *pluginProject) gate() {
	_ = os.Remove(filepath.Join(p.dir, "open"))
	writeFile(p.t, filepath.Join(p.dir, "gate"), "")
}

// release は、待っているプラグインを、進ませる。
func (p *pluginProject) release() { writeFile(p.t, filepath.Join(p.dir, "open"), "") }

func (p *pluginProject) open(c *testClient, text string) {
	c.notify(protocol.MethodDidOpen, protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: p.uri, LanguageID: "hozo", Version: 1, Text: text}})
}

func (p *pluginProject) change(c *testClient, text string) {
	c.notify(protocol.MethodDidChange, protocol.DidChangeTextDocumentParams{
		TextDocument:   protocol.VersionedTextDocumentIdentifier{URI: p.uri, Version: 2},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{{Text: text}},
	})
}

// save は、text をディスクに書いて、didSave を送る。
func (p *pluginProject) save(c *testClient, text string) {
	writeFile(p.t, filepath.Join(p.root, "a.hozo"), text)
	c.notify(protocol.MethodDidSave, protocol.DidSaveTextDocumentParams{TextDocument: textDoc(p.uri)})
}

// allowed は、プラグインを起動してよい、と言って、initialize する（docs/lsp.md「プラグインの診断」の「許し」）。
func (p *pluginProject) allowed(c *testClient) (protocol.InitializeResult, map[string][]protocol.Diagnostic) {
	c.t.Helper()
	return c.initializeWith(p.root, protocol.ClientCapabilities{}, `{"pluginChecks": true}`)
}

// allow は、動いている間に、許しを変える（workspace/didChangeConfiguration）。
func (p *pluginProject) allow(c *testClient, on bool) {
	c.notify(protocol.MethodDidChangeConfiguration, protocol.DidChangeConfigurationParams{
		Settings: json.RawMessage(`{"hozo": {"pluginChecks": ` + strconv.FormatBool(on) + `}}`),
	})
}

// watched は、a.hozo が、エディタの外で変わった、と知らせる（workspace/didChangeWatchedFiles）。
func (p *pluginProject) watched(c *testClient) {
	c.notify(protocol.MethodDidChangeWatchedFiles, protocol.DidChangeWatchedFilesParams{Changes: []protocol.FileEvent{{URI: p.uri, Type: protocol.FileChanged}}})
}

// pluginCheckStarts は、サーバーが、裏の check を始めた回数（ログの行を数える）。
func (c *testClient) pluginCheckStarts() int {
	return strings.Count(c.logBuf.String(), "plugin check: start")
}

// waitPlugin は、uri への publishDiagnostics を、CHK-1 を含むものが来るまで待ち、その CHK-1 のメッセージを返す。
// それまでに届いた、uri への発行は、seen に足す（古い答えが、出ていないかを見るため）。
func (c *testClient) waitPlugin(uri string, seen *[][]protocol.Diagnostic) (string, []protocol.Diagnostic) {
	c.t.Helper()
	for {
		ds := c.waitDiagnostics(uri)
		if seen != nil {
			*seen = append(*seen, ds)
		}
		for _, d := range ds {
			if d.Code == "CHK-1" {
				return d.Message, ds
			}
		}
	}
}

// waitNoPlugin は、uri への publishDiagnostics を、CHK-1 を含まないものが来るまで待つ。
func (c *testClient) waitNoPlugin(uri string) []protocol.Diagnostic {
	c.t.Helper()
	for {
		ds := c.waitDiagnostics(uri)
		if !hasCode(ds, "CHK-1") {
			return ds
		}
	}
}

// TestPluginDiagnosticsOnOpen は、文書を開いたときに、hozoc 自身の診断が、先に出て、プラグインの check が、裏で走り、終わったら、
// プラグインの診断が、足されて出ること。足した後の診断は、hozoc check（project.LoadWith）と同じ。initialized では、走らせない。
func TestPluginDiagnosticsOnOpen(t *testing.T) {
	p := newPluginProject(t, pluginSpec)
	p.gate()
	c := startServer(t, Options{})
	_, byURI := p.allowed(c)
	if !hasCode(byURI[p.uri], "REL-2") || hasCode(byURI[p.uri], "CHK-1") {
		t.Fatalf("initialized の診断 = %v（期待は、hozoc 自身の REL-2 だけ）", codes(byURI[p.uri]))
	}
	if n := c.pluginCheckStarts(); n != 0 || p.calls() != 0 {
		t.Fatalf("initialized で、プラグインの check が走った（始めた %d 回、起動 %d 回）", n, p.calls())
	}

	// 開く: プラグインは、裏で、待たされている。その間、診断は、hozoc 自身のものだけ
	p.open(c, pluginSpec)
	p.waitCalls(1)
	for u, ds := range c.syncDiagnostics() {
		if hasCode(ds, "CHK-1") {
			t.Errorf("%s: プラグインが終わる前に、プラグインの診断が出た: %v", u, codes(ds))
		}
	}

	// 進ませる: プラグインの診断が、足されて出る
	p.release()
	msg, ds := c.waitPlugin(p.uri, nil)
	if msg != "run 1" || !hasCode(ds, "REL-2") {
		t.Fatalf("プラグインが終わった後の診断 = %v（CHK-1 のメッセージ %q。期待は、REL-2 と、CHK-1 \"run 1\"）", codes(ds), msg)
	}
	if n := c.pluginCheckStarts(); n != 1 {
		t.Errorf("裏の check を始めた回数 = %d（期待は 1）", n)
	}

	// hozoc check と同じ診断（プラグインの check の分を含む）
	want := project.LoadWith(project.Options{Root: p.root})
	if want.RunErr != nil {
		t.Fatal(want.RunErr)
	}
	var wantCodes []string
	for _, d := range want.Diags {
		if d.Span != nil && want.Files.Path(d.Span.File) == "a.hozo" {
			sev := "warning"
			if d.Severity == diag.Error {
				sev = "error"
			}
			wantCodes = append(wantCodes, sev+" "+d.Code)
		}
	}
	var gotCodes []string
	for _, d := range ds {
		sev := "warning"
		if d.Severity == protocol.SeverityError {
			sev = "error"
		}
		gotCodes = append(gotCodes, sev+" "+d.Code)
	}
	if strings.Join(gotCodes, ",") != strings.Join(wantCodes, ",") {
		t.Errorf("言語サーバーの診断 = %v、hozoc check の診断 = %v", gotCodes, wantCodes)
	}
	c.shutdown()
}

// TestPluginDiagnosticsOnSave は、保存したときに、プラグインの check が、走り直すこと（中身が、同じでも）。
func TestPluginDiagnosticsOnSave(t *testing.T) {
	p := newPluginProject(t, pluginSpec)
	c := startServer(t, Options{})
	p.allowed(c)
	p.open(c, pluginSpec)
	if msg, _ := c.waitPlugin(p.uri, nil); msg != "run 1" {
		t.Fatalf("開いた後の CHK-1 = %q", msg)
	}
	p.save(c, pluginSpec)
	if msg, _ := c.waitPlugin(p.uri, nil); msg != "run 2" {
		t.Fatalf("保存の後の CHK-1 = %q（期待は、走り直した \"run 2\"）", msg)
	}
	if n := c.pluginCheckStarts(); n != 2 {
		t.Errorf("裏の check を始めた回数 = %d（期待は 2）", n)
	}
	c.shutdown()
}

// TestNoPluginCheckDiagnostics は、解析（initialized と、打鍵の didChange）では、プラグインの check を、起動しないこと
// （docs/lsp.md「プラグインの診断」: 起動は、打鍵ごとの解析には、遅い）。[check] enabled のプラグインが、診断を返す設定でも、
// その診断は出ず、プラグインは、呼ばれない。hozoc 自身の診断は、今までどおり出る。
func TestNoPluginCheckDiagnostics(t *testing.T) {
	p := newPluginProject(t, pluginSpec)
	c := startServer(t, Options{})
	_, byURI := p.allowed(c)
	for u, ds := range byURI {
		if hasCode(ds, "CHK-1") {
			t.Errorf("%s: プラグインの check の診断が出ている: %+v", u, ds)
		}
	}
	if !hasCode(byURI[p.uri], "REL-2") {
		t.Errorf("hozoc 自身の診断（REL-2）が出ていない: %+v", byURI[p.uri])
	}
	// 開かずに、編集する（didChange だけ）: 解析し直すが、プラグインは、起動しない
	p.change(c, pluginSpecBroken)
	if ds := c.waitDiagnostics(p.uri); !hasCode(ds, "RES-1") {
		t.Fatalf("didChange の後の診断 = %v（期待は RES-1）", codes(ds))
	}
	p.change(c, pluginSpecEdited)
	if ds := c.waitDiagnostics(p.uri); !hasCode(ds, "REL-2") || hasCode(ds, "CHK-1") {
		t.Fatalf("didChange の後の診断 = %v（期待は、REL-2 だけ）", codes(ds))
	}
	c.shutdown() // 裏の check が走っていれば、止まるのを待つ
	if n := c.pluginCheckStarts(); n != 0 || p.calls() != 0 {
		t.Errorf("解析だけで、プラグインの check が走った（始めた %d 回、起動 %d 回）", n, p.calls())
	}
}

// TestPluginCheckDoesNotBlock は、プラグインの check が走っている間も、要求（ホバー）と、解析（didChange の後の診断）が、
// 待たされないこと。
func TestPluginCheckDoesNotBlock(t *testing.T) {
	p := newPluginProject(t, pluginSpec)
	p.gate()
	c := startServer(t, Options{})
	p.allowed(c)
	p.open(c, pluginSpec)
	p.waitCalls(1) // プラグインは、起動して、待たされている

	var hov *protocol.Hover
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(p.uri), Position: positionOf(t, encUTF16, pluginSpec, "Idle", 1)}, &hov)
	if hov == nil || !strings.Contains(hov.Contents.Value, "exception") {
		t.Fatalf("プラグインが走っている間のホバー = %+v", hov)
	}
	p.change(c, pluginSpecBroken)
	if ds := c.waitDiagnostics(p.uri); !hasCode(ds, "RES-1") {
		t.Fatalf("プラグインが走っている間の、didChange の後の診断 = %v（期待は RES-1）", codes(ds))
	}
	if p.calls() != 1 {
		t.Errorf("プラグインの起動 = %d 回（期待は、待たされている 1 回だけ）", p.calls())
	}
	p.release()
	c.shutdown()
}

// TestStalePluginDiagnosticsDropped は、古い check の答えが、新しい保存の後に、出ないこと: 一つ目の check が走っている間に、
// 中身を変えて保存すると、一つ目の答えは、出ず、二つ目の check の答えだけが出る。裏の check は、一度に一つ。
func TestStalePluginDiagnosticsDropped(t *testing.T) {
	p := newPluginProject(t, pluginSpec)
	p.gate()
	c := startServer(t, Options{})
	p.allowed(c)
	p.open(c, pluginSpec)
	p.waitCalls(1) // 一つ目が、待たされている

	p.change(c, pluginSpecEdited)
	p.save(c, pluginSpecEdited)
	c.syncDiagnostics()
	if p.calls() != 1 {
		t.Fatalf("一つ目が終わる前に、二つ目のプラグインが起動した（%d 回）", p.calls())
	}

	p.release()
	var seen [][]protocol.Diagnostic
	msg, ds := c.waitPlugin(p.uri, &seen)
	if msg != "run 2" {
		t.Fatalf("保存の後に出た CHK-1 = %q（期待は、二つ目の \"run 2\"。一つ目の答えは、古い中身のもの）", msg)
	}
	for _, pub := range seen {
		for _, d := range pub {
			if d.Code == "CHK-1" && d.Message != "run 2" {
				t.Errorf("古い check の答えが出た: %q", d.Message)
			}
		}
	}
	// 位置は、新しい中身のもの（コメントを一行足したので、a.f は、一行下）
	for _, d := range ds {
		if d.Code == "CHK-1" && d.Range.Start.Line != 3 {
			t.Errorf("CHK-1 の行 = %d（期待は 3: 新しい中身での a.f）", d.Range.Start.Line)
		}
	}
	c.shutdown()
	if p.calls() != 2 {
		t.Errorf("プラグインの起動 = %d 回（期待は 2）", p.calls())
	}
}

// TestPluginDiagnosticsFollowContent は、プラグインの診断を、それを求めた中身と、今の中身が同じ間だけ、出すこと: 保存の後に
// 編集すると、消え、編集を取り消すと、プラグインを起動し直さずに、また出る。
func TestPluginDiagnosticsFollowContent(t *testing.T) {
	p := newPluginProject(t, pluginSpec)
	c := startServer(t, Options{})
	p.allowed(c)
	p.open(c, pluginSpec)
	if msg, _ := c.waitPlugin(p.uri, nil); msg != "run 1" {
		t.Fatalf("開いた後の CHK-1 = %q", msg)
	}
	p.change(c, pluginSpecEdited)
	if ds := c.waitNoPlugin(p.uri); !hasCode(ds, "REL-2") {
		t.Fatalf("編集の後の診断 = %v（期待は、hozoc 自身の REL-2 だけ）", codes(ds))
	}
	p.change(c, pluginSpec)
	if msg, _ := c.waitPlugin(p.uri, nil); msg != "run 1" {
		t.Fatalf("編集を取り消した後の CHK-1 = %q（期待は、前の答え \"run 1\"）", msg)
	}
	c.shutdown()
	if n := c.pluginCheckStarts(); n != 1 || p.calls() != 1 {
		t.Errorf("編集と、取り消しで、プラグインの check が走った（始めた %d 回、起動 %d 回。期待は、どちらも 1）", n, p.calls())
	}
}

// TestPluginCheckSkippedOnErrors は、hozoc 自身の診断に、error があるときは、開いても、保存しても、プラグインを起動しないこと。
// error を直して保存すれば、走る。その後、error を足すと、前のプラグインの診断は、出ない。
func TestPluginCheckSkippedOnErrors(t *testing.T) {
	p := newPluginProject(t, pluginSpecBroken)
	c := startServer(t, Options{})
	_, byURI := p.allowed(c)
	if !hasCode(byURI[p.uri], "RES-1") {
		t.Fatalf("initialized の診断 = %v（期待は RES-1）", codes(byURI[p.uri]))
	}
	p.open(c, pluginSpecBroken)
	p.save(c, pluginSpecBroken)
	c.syncDiagnostics()
	if n := c.pluginCheckStarts(); n != 0 || p.calls() != 0 {
		t.Fatalf("error があるのに、プラグインの check が走った（始めた %d 回、起動 %d 回）", n, p.calls())
	}

	p.change(c, pluginSpec)
	p.save(c, pluginSpec)
	if msg, _ := c.waitPlugin(p.uri, nil); msg != "run 1" {
		t.Fatalf("error を直して保存した後の CHK-1 = %q", msg)
	}

	p.change(c, pluginSpecBroken)
	if ds := c.waitNoPlugin(p.uri); !hasCode(ds, "RES-1") {
		t.Fatalf("error を足した後の診断 = %v（期待は、RES-1 だけ）", codes(ds))
	}
	p.save(c, pluginSpecBroken)
	c.syncDiagnostics()
	c.shutdown()
	if n := c.pluginCheckStarts(); n != 1 || p.calls() != 1 {
		t.Errorf("error があるのに、保存で、プラグインの check が走った（始めた %d 回、起動 %d 回。期待は、どちらも 1）", n, p.calls())
	}
}

// TestPluginCheckOnWatchedFiles は、エディタの外で、ディスクが書き換わったとき（workspace/didChangeWatchedFiles）に、今の中身の
// プラグインの診断を、まだ持っていなければ、check を走らせること。同じ中身の check が、走っているか、済んでいれば、何もしない
// （保存の直後に届く、同じ中身の知らせ）。
func TestPluginCheckOnWatchedFiles(t *testing.T) {
	p := newPluginProject(t, pluginSpecBroken)
	c := startServer(t, Options{})
	p.allowed(c)
	watched := func() { p.watched(c) }

	// 開いていないファイルが、外で、直った
	p.gate()
	writeFile(t, filepath.Join(p.root, "a.hozo"), pluginSpec)
	watched()
	p.waitCalls(1)
	// 同じ中身の知らせが、もう一度: 走っている check を、待つだけ
	watched()
	c.syncDiagnostics()
	if n := c.pluginCheckStarts(); n != 1 {
		t.Fatalf("同じ中身の check が走っているのに、もう一つ始めた（%d 回）", n)
	}
	p.release()
	if msg, _ := c.waitPlugin(p.uri, nil); msg != "run 1" {
		t.Fatalf("外で直った後の CHK-1 = %q", msg)
	}
	// 済んだ後の、同じ中身の知らせ: 何もしない
	watched()
	c.syncDiagnostics()
	c.shutdown()
	if n := c.pluginCheckStarts(); n != 1 || p.calls() != 1 {
		t.Errorf("同じ中身の知らせで、プラグインの check が走った（始めた %d 回、起動 %d 回。期待は、どちらも 1）", n, p.calls())
	}
}

// TestPluginCheckCrash は、プラグインの check が、最後まで走らなかったとき（異常終了）に、LSP-2 を、hozo.toml に出して、
// サーバーが、動き続けること。
func TestPluginCheckCrash(t *testing.T) {
	p := newPluginProject(t, pluginSpec)
	writeFile(t, filepath.Join(p.dir, "crash"), "")
	c := startServer(t, Options{})
	p.allowed(c)
	p.open(c, pluginSpec)
	configURI := fileURI(filepath.Join(p.root, "hozo.toml"))
	ds := c.waitDiagnostics(configURI)
	if len(ds) != 1 || ds[0].Code != "LSP-2" || ds[0].Severity != protocol.SeverityWarning || !strings.Contains(ds[0].Message, "chk") {
		t.Fatalf("hozo.toml の診断 = %+v（期待は、warning の LSP-2 一つで、メッセージに、プラグインの名前）", ds)
	}
	// 動き続ける: ホバーが返り、直して保存すれば、LSP-2 は消えて、プラグインの診断が出る
	var hov *protocol.Hover
	c.result(protocol.MethodHover, protocol.TextDocumentPositionParams{TextDocument: textDoc(p.uri), Position: positionOf(t, encUTF16, pluginSpec, "Idle", 1)}, &hov)
	if hov == nil {
		t.Fatal("異常終了の後に、ホバーが返らない")
	}
	if err := os.Remove(filepath.Join(p.dir, "crash")); err != nil {
		t.Fatal(err)
	}
	p.save(c, pluginSpec)
	if msg, _ := c.waitPlugin(p.uri, nil); msg != "run 2" {
		t.Fatalf("直して保存した後の CHK-1 = %q", msg)
	}
	if ds := c.waitDiagnostics(configURI); len(ds) != 0 {
		t.Fatalf("直して保存した後の、hozo.toml の診断 = %+v（期待は、なし）", ds)
	}
	c.shutdown()
}

// TestPluginCheckExit は、プラグインの check が走っている間に、終わるとき、サーバーが、check が止まるのを待つこと
// （プラグインが、進めば、すぐ終わる）と、待ちきれなければ、exitWait で、待つのをやめて、終わること。
func TestPluginCheckExit(t *testing.T) {
	// 進む場合: exit の後、プラグインが終わるまで、Serve は、戻らない
	p := newPluginProject(t, pluginSpec)
	p.gate()
	c := startServer(t, Options{})
	p.allowed(c)
	p.open(c, pluginSpec)
	p.waitCalls(1)
	if r := c.call(protocol.MethodShutdown, nil); r.Error != nil {
		t.Fatalf("shutdown: %+v", r)
	}
	c.notify(protocol.MethodExit, nil)
	select {
	case <-c.done:
		t.Fatal("プラグインが走っているのに、Serve が戻った")
	case <-time.After(50 * time.Millisecond):
	}
	p.release()
	select {
	case <-c.done:
	case <-time.After(waitTimeout):
		t.Fatal("プラグインが終わったのに、Serve が戻らない")
	}

	// 進まない場合: exitWait で、待つのをやめる
	saved := exitWait
	exitWait = 50 * time.Millisecond
	defer func() { exitWait = saved }()
	p = newPluginProject(t, pluginSpec)
	p.gate()
	t.Cleanup(p.release) // テストの後に、待っているプラグインを、終わらせる
	c = startServer(t, Options{})
	p.allowed(c)
	p.open(c, pluginSpec)
	p.waitCalls(1)
	c.shutdown() // exit の後、waitTimeout までに、Serve が戻らなければ、失敗
	if !strings.Contains(c.logBuf.String(), "待ちきれなかった") {
		t.Errorf("待つのをやめたことが、ログにない:\n%s", c.logBuf.String())
	}
}

// TestPluginChecksNeedPermission は、クライアントが許していなければ（既定）、プラグインの check の、どのきっかけ（開く、保存、
// 外でファイルが変わる）でも、プラグインを起動しないこと（docs/lsp.md「プラグインの診断」の「許し」）。出るのは、hozoc 自身の
// 診断だけ。異常終了するプラグインでも、LSP-2 は、出ない（起動しないので）。許しとして読めない initializationOptions も、
// 許していない、と読む。
func TestPluginChecksNeedPermission(t *testing.T) {
	for _, tc := range []struct{ name, options string }{
		{"なし", ""},
		{"偽", `{"pluginChecks": false}`},
		{"項目がない", `{"other": true}`},
		{"真偽値でない", `{"pluginChecks": "yes"}`},
		{"オブジェクトでない", `7`},
		{"null", `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPluginProject(t, pluginSpec)
			writeFile(t, filepath.Join(p.dir, "crash"), "") // 起動すれば、LSP-2 が出る
			c := startServer(t, Options{})
			res, byURI := c.initializeWith(p.root, protocol.ClientCapabilities{}, tc.options)
			if !res.Capabilities.HoverProvider {
				t.Fatalf("initialize の答えが、おかしい: %+v", res)
			}
			if !hasCode(byURI[p.uri], "REL-2") {
				t.Fatalf("initialized の診断 = %v（期待は、hozoc 自身の REL-2）", codes(byURI[p.uri]))
			}
			p.open(c, pluginSpec)
			c.syncDiagnostics()
			p.save(c, pluginSpecEdited)
			if ds := c.syncDiagnostics()[p.uri]; ds != nil && (!hasCode(ds, "REL-2") || len(ds) != 1) {
				t.Errorf("保存の後の診断 = %v（期待は、REL-2 だけ）", codes(ds))
			}
			writeFile(t, filepath.Join(p.root, "a.hozo"), pluginSpec)
			p.watched(c)
			all := c.syncDiagnostics()
			c.shutdown()
			if n := c.pluginCheckStarts(); n != 0 || p.calls() != 0 {
				t.Errorf("許していないのに、プラグインの check が走った（始めた %d 回、起動 %d 回）", n, p.calls())
			}
			for u, ds := range all {
				if hasCode(ds, "CHK-1") || hasCode(ds, "LSP-2") {
					t.Errorf("%s: 許していないのに、プラグインの診断が出た: %v", u, codes(ds))
				}
			}
			if strings.Contains(c.logBuf.String(), "LSP-2") {
				t.Errorf("許していないのに、LSP-2 が、ログにある")
			}
		})
	}
}

// TestPluginChecksRevoked は、動いている間に、許しがなくなったら（workspace/didChangeConfiguration）、プラグインの診断が消え、
// その後は、どのきっかけでも、プラグインを起動しないこと。
func TestPluginChecksRevoked(t *testing.T) {
	p := newPluginProject(t, pluginSpec)
	c := startServer(t, Options{})
	p.allowed(c)
	p.open(c, pluginSpec)
	if msg, _ := c.waitPlugin(p.uri, nil); msg != "run 1" {
		t.Fatalf("開いた後の CHK-1 = %q", msg)
	}
	p.allow(c, false)
	if ds := c.waitNoPlugin(p.uri); !hasCode(ds, "REL-2") {
		t.Fatalf("許しがなくなった後の診断 = %v（期待は、hozoc 自身の REL-2 だけ）", codes(ds))
	}
	p.save(c, pluginSpec)
	p.watched(c)
	for u, ds := range c.syncDiagnostics() {
		if hasCode(ds, "CHK-1") {
			t.Errorf("%s: 許しがなくなった後に、プラグインの診断が出た: %v", u, codes(ds))
		}
	}
	c.shutdown()
	if n := c.pluginCheckStarts(); n != 1 || p.calls() != 1 {
		t.Errorf("許しがなくなった後に、プラグインの check が走った（始めた %d 回、起動 %d 回。期待は、どちらも 1）", n, p.calls())
	}
}

// TestPluginChecksRevokedWhileRunning は、裏の check が走っている間に、許しがなくなったら、その答えを、出さないこと。
// 異常終了する check でも、LSP-2 は、出ない。
func TestPluginChecksRevokedWhileRunning(t *testing.T) {
	for _, crash := range []bool{false, true} {
		p := newPluginProject(t, pluginSpec)
		if crash {
			writeFile(t, filepath.Join(p.dir, "crash"), "")
		}
		p.gate()
		c := startServer(t, Options{})
		p.allowed(c)
		p.open(c, pluginSpec)
		p.waitCalls(1)
		p.allow(c, false)
		c.syncDiagnostics() // 許しがなくなったことを、サーバーが、読み終えた
		p.release()
		// 走っていた check が終わるのを、ログで待つ（答えは、使わない）
		deadline := time.Now().Add(waitTimeout)
		for !strings.Contains(c.logBuf.String(), "plugin check: canceled") && !strings.Contains(c.logBuf.String(), "plugin check: 許されていない") {
			if time.Now().After(deadline) {
				t.Fatalf("crash=%v: 走っていた check が、終わらない:\n%s", crash, c.logBuf.String())
			}
			time.Sleep(2 * time.Millisecond)
		}
		for u, ds := range c.syncDiagnostics() {
			if hasCode(ds, "CHK-1") || hasCode(ds, "LSP-2") {
				t.Errorf("crash=%v %s: 許しがなくなった後に、プラグインの診断が出た: %v", crash, u, codes(ds))
			}
		}
		c.shutdown()
	}
}

// TestPluginChecksGranted は、動いている間に、許しが与えられたら（後から、ワークスペースを信頼した）、今の中身について、
// プラグインの check が走ること。値のない知らせと、ほかの項目だけの知らせでは、許しは、変わらない。
func TestPluginChecksGranted(t *testing.T) {
	p := newPluginProject(t, pluginSpec)
	c := startServer(t, Options{})
	c.initialize(p.root, protocol.ClientCapabilities{})
	p.open(c, pluginSpec)
	c.syncDiagnostics()
	// 許しを言わない知らせ: 変わらない
	for _, settings := range []string{`null`, `{}`, `{"hozo": {}}`, `{"hozo": {"pluginChecks": "yes"}}`, `{"pluginChecks": true}`, `{"other": {"pluginChecks": true}}`} {
		c.notify(protocol.MethodDidChangeConfiguration, protocol.DidChangeConfigurationParams{Settings: json.RawMessage(settings)})
	}
	c.syncDiagnostics()
	if n := c.pluginCheckStarts(); n != 0 || p.calls() != 0 {
		t.Fatalf("許しを言わない知らせで、プラグインの check が走った（始めた %d 回、起動 %d 回）", n, p.calls())
	}
	p.allow(c, true)
	if msg, ds := c.waitPlugin(p.uri, nil); msg != "run 1" || !hasCode(ds, "REL-2") {
		t.Fatalf("許しが与えられた後の診断 = %v（CHK-1 のメッセージ %q。期待は、REL-2 と、CHK-1 \"run 1\"）", codes(ds), msg)
	}
	// 同じ許しを、もう一度: 今の中身の診断を、もう持っているので、走らせない
	p.allow(c, true)
	c.syncDiagnostics()
	// 許しをなくして、また与える: 答えは、捨ててあるので、走り直す
	p.allow(c, false)
	c.waitNoPlugin(p.uri)
	p.allow(c, true)
	if msg, _ := c.waitPlugin(p.uri, nil); msg != "run 2" {
		t.Fatalf("許しを、与え直した後の CHK-1 = %q（期待は、走り直した \"run 2\"）", msg)
	}
	c.shutdown()
	if n := c.pluginCheckStarts(); n != 2 || p.calls() != 2 {
		t.Errorf("裏の check を始めた %d 回、起動 %d 回（期待は、どちらも 2）", n, p.calls())
	}
}
