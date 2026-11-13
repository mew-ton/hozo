package check

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/ir"
)

var update = flag.Bool("update", false, "update golden files (docs/testing.md「コーパス」)")

// TestViewsGolden は §9 の例に深い bind を足した小さなプログラムの View（browser から見たもの）がゴールデンと一致すること。
// 更新は go test ./internal/check -update。
func TestViewsGolden(t *testing.T) {
	lineCounter = 1000 // 位置を決定的にする
	p := logoutProgram()
	// 入れ子の位置: server.deep(in: Outer{ inner: Inner{ token: Token? }, note: string })
	server := p.Namespace("server")
	server.Decls = append(server.Decls,
		model("server.Inner", field("token", ir.Optional(ir.Ref("server.Token"))), field("n", ir.Int())),
		model("server.Outer", field("inner", ir.Ref("server.Inner")), field("note", ir.String())),
		leaf("server.deep", ir.String(), param("in", ir.Ref("server.Outer")), param("t", ir.Optional(ir.Ref("server.Token")))),
	)
	cfg := cfgFor("server", "browser")
	plugins := []string{"react-router"}
	nt := cfg.Namespaces["browser"]
	nt.Plugins = plugins
	cfg.Namespaces["browser"] = nt
	// server.admin はテーブルだけあって Program にはない（CFG-10 は check の別の規則）。Dir は server の out を継いで gen/server/admin
	cfg.Namespaces["server.admin"] = config.NamespaceTable{Plugins: []string{}}

	views := Views(p, cfg)
	if len(views) != 2 {
		t.Fatalf("views = %d, want 2 (server, browser)", len(views))
	}
	v := views["browser"]
	if v.Viewer != "browser" || v.Config.Language != "go" || v.Config.Out != "gen/browser" || v.Config.Dir != "gen/browser" || len(v.Config.Plugins) != 1 {
		t.Errorf("config = %+v", v.Config)
	}
	// namespaces は設定のあるすべての名前空間（viewer を含む）
	if len(v.Namespaces) != 3 {
		t.Errorf("namespaces = %+v", v.Namespaces)
	}
	if nc := v.Namespaces["server.admin"]; nc.Language != "go" || nc.Out != "gen/server" || nc.Dir != "gen/server/admin" || len(nc.Plugins) != 0 {
		t.Errorf("namespaces[server.admin] = %+v", nc)
	}
	if !reflect.DeepEqual(v.Namespaces["browser"], v.Config) {
		t.Errorf("namespaces[viewer] = %+v, config = %+v", v.Namespaces["browser"], v.Config)
	}
	if !reflect.DeepEqual(views["server"].Namespaces, v.Namespaces) {
		t.Error("every view must carry the same namespaces")
	}
	// 合成した model は browser の decls の末尾にだけ足され、元の Program は変わらない
	// logout$in（token を除いた LogoutInput）、deep$in、deep$in$inner の三つ
	if n := len(v.Program.Namespace("browser").Decls); n != len(p.Namespace("browser").Decls)+3 {
		t.Errorf("browser decls = %d, want +3 synthesized", n)
	}
	if m := v.Program.Lookup("browser.logout$in").(*ir.Model); len(m.Fields) != 0 || m.SynthesizedFrom != "server.logout" {
		t.Errorf("logout$in = %+v", m)
	}
	if len(views["server"].Program.Namespace("browser").Decls) != len(p.Namespace("browser").Decls) {
		t.Error("server view has browser's synthesized models")
	}
	deep := v.Ops["server.deep"]
	if len(deep.Params) != 1 || deep.Params[0].Type.String() != "browser.deep$in" {
		t.Errorf("deep params = %+v", deep.Params)
	}
	if len(deep.Bound) != 2 {
		t.Errorf("deep bound = %+v", deep.Bound)
	}
	synth := v.Program.Lookup("browser.deep$in$inner").(*ir.Model)
	if len(synth.Fields) != 1 || synth.Fields[0].Name != "n" || synth.SynthesizedFrom != "server.deep" {
		t.Errorf("synthesized inner = %+v", synth)
	}

	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "view.golden.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("view differs from golden (run with -update to refresh)\n%s", firstDiff(got, want))
	}
}

func firstDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := i - 80
			if lo < 0 {
				lo = 0
			}
			hi := i + 80
			if hi > len(a) {
				hi = len(a)
			}
			hj := i + 80
			if hj > len(b) {
				hj = len(b)
			}
			return "got:  …" + string(a[lo:hi]) + "…\nwant: …" + string(b[lo:hj]) + "…"
		}
	}
	return "lengths differ"
}

// TestViewsUnconfiguredNamespace は docs/config.md「出力する名前空間」: hozo.toml にテーブルのない名前空間は View を
// 持たない（出力しない）が、検査と計算は同じに掛かる。設定のある名前空間が設定のない名前空間の op を呼んでも、
// その逆でも、Signature は普通に計算され、設定のある側の View.ops には設定のない側の op も Sig(O, viewer) で載る。
func TestViewsUnconfiguredNamespace(t *testing.T) {
	// server（設定あり）: @http.get の葉 getMe と、外のシステム ext（設定なし）の葉を呼ぶ配線 sync
	getMe := leaf("server.getMe", ir.String())
	getMe.Decorators = []ir.Application{app("http.get", ir.Arg{Param: "path", Value: ir.LiteralValue{Literal: ir.StringLiteral{Value: "/me"}}})}
	sync := wire("server.sync", nil, nil, cs(call("ext.push")))
	sync.Throws = []ir.ThrowsClause{throws([]string{"ext.Down"})}
	// ext（設定なし）: 状態と効果と例外を持つ葉
	push := leaf("ext.push", nil)
	push.Effects = []string{"ext.record"}
	push.Throws = []ir.ThrowsClause{throws([]string{"ext.Down"}, prevents())}
	// user（設定なし）: 人の操作。server の op を呼ぶ配線
	open := wire("user.open", ir.String(), nil, ret(call("server.getMe")))
	open.Throws = []ir.ThrowsClause{throws([]string{"http.ServerError", "http.NetworkError"})}
	press := wire("user.press", nil, nil, cs(call("server.sync")))
	press.Throws = []ir.ThrowsClause{throws([]string{"ext.Down"})}
	p := program(nil,
		httpVocab(),
		namespace("server", getMe, sync),
		namespace("ext", state("ext.log", ir.StateList, ir.String()), eff("ext.record", "ext.log", ir.OpPost), exc("ext.Down"), push),
		namespace("user", open, press),
	)
	cfg := cfgFor("server")
	wantCodes(t, Check(p, cfg)) // CFG-1 も THROWS-1 も出ない

	// 設定のない名前空間の op の Signature も計算されている
	wantStrings(t, "Sig(user.open).E", sigOf(t, p, "user.open").Exceptions, []string{"http.ServerError", "http.NetworkError"})
	pressSig := sigOf(t, p, "user.press")
	wantStrings(t, "Sig(user.press).F", pressSig.Effects, []string{"ext.record"})
	if got := relOf(t, pressSig, "ext.Down", "ext.record"); got != ir.Prevented {
		t.Errorf("rel(Down, record) = %s, want prevented", got)
	}
	wantStrings(t, "Sig(server.sync).F", sigOf(t, p, "server.sync").Effects, []string{"ext.record"})

	// View は設定のある server だけ。namespaces にも設定のない名前空間は入らない
	views := Views(p, cfg)
	if len(views) != 1 || views["server"] == nil {
		t.Fatalf("views = %v, want only server", reflect.ValueOf(views).MapKeys())
	}
	v := views["server"]
	if len(v.Namespaces) != 1 {
		t.Errorf("namespaces = %+v, want only server", v.Namespaces)
	}
	// ops はすべての op（設定のない名前空間のものも）を server から見た Sig で持つ
	for _, name := range []string{"server.getMe", "server.sync", "ext.push", "user.open", "user.press"} {
		if v.Ops[name] == nil {
			t.Errorf("View(server).ops に %s がない", name)
		}
	}
	wantStrings(t, "View(server).ext.push.E", v.Ops["ext.push"].Exceptions, []string{"ext.Down"})
	wantStrings(t, "View(server).ext.push.F", v.Ops["ext.push"].Effects, []string{"ext.record"})
	// Program は全体そのまま（設定のない名前空間の宣言も読める）
	if v.Program.Lookup("user.press") == nil || v.Program.Lookup("ext.log") == nil {
		t.Error("View.program に設定のない名前空間の宣言がない")
	}
	// 設定がまったくなければ View は一つもない（設定なしの検査）
	if n := len(Views(p, config.Default("/root"))); n != 0 {
		t.Errorf("views without config = %d, want 0", n)
	}
	wantCodes(t, Check(p, config.Default("/root")))
}
