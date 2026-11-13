package check

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
	// 単位は四つ: server、browser（トランスパイラ react-router）、同じ browser の二つ目（別のトランスパイラと out）、
	// Program にない server.admin（CFG-10 は check の別の規則。出力はない）
	cfg := cfgFor("server", "browser", "browser", "server.admin")
	cfg.Units[1].Transpilers = []string{"react-router"}
	cfg.Units[2].Transpilers = []string{"vue-router", "pinia"}
	cfg.Units[2].Out = "gen/browser2"

	all := Views(p, cfg, goUnits(p, cfg))
	if len(all) != 4 || len(all[0]) != 1 || len(all[1]) != 1 || len(all[2]) != 1 || len(all[3]) != 0 {
		t.Fatalf("views = %v, want one view each for the units of server, browser, browser, and none for server.admin", all)
	}
	views := map[string]*ir.View{"server": all[0]["server"], "browser": all[1]["browser"]}
	v := views["browser"]
	want1 := ir.NamespaceConfig{Language: "go", Out: "gen/browser", Dir: "gen/browser", Transpilers: []string{"react-router"}}
	if v.Viewer != "browser" || !reflect.DeepEqual(v.Config, want1) {
		t.Errorf("config = %+v", v.Config)
	}
	if !reflect.DeepEqual(v.Files, []string{"browser.hozo", "server.hozo"}) {
		t.Errorf("files = %v", v.Files)
	}
	// namespaces は出力のあるすべての名前空間（viewer を含む）。値は、その名前空間を出力する単位ごとの設定（設定の順）
	if len(v.Namespaces) != 2 || len(v.Namespaces["server"]) != 1 || len(v.Namespaces["browser"]) != 2 {
		t.Errorf("namespaces = %+v", v.Namespaces)
	}
	if !reflect.DeepEqual(v.Namespaces["browser"][0], v.Config) {
		t.Errorf("namespaces[viewer][0] = %+v, config = %+v", v.Namespaces["browser"][0], v.Config)
	}
	// 同じ名前空間の二つ目の単位の View は、config だけが違う
	second := all[2]["browser"]
	want2 := ir.NamespaceConfig{Language: "go", Out: "gen/browser2", Dir: "gen/browser2", Transpilers: []string{"vue-router", "pinia"}}
	if !reflect.DeepEqual(second.Config, want2) || !reflect.DeepEqual(v.Namespaces["browser"][1], want2) {
		t.Errorf("second unit: config = %+v, namespaces = %+v", second.Config, v.Namespaces["browser"])
	}
	if !reflect.DeepEqual(second.Ops, v.Ops) || !reflect.DeepEqual(second.Namespaces, v.Namespaces) {
		t.Error("views of the same namespace must differ only in config and files")
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

// TestViewsUncoveredNamespace は docs/config.md「単位の対象」: どの単位の対象でもない名前空間は View を
// 持たない（出力しない）が、検査と計算は同じに掛かる。単位の対象の名前空間が対象でない名前空間の op を呼んでも、
// その逆でも、Signature は普通に計算され、対象の側の View.ops には対象でない側の op も Sig(O, viewer) で載る。
func TestViewsUncoveredNamespace(t *testing.T) {
	// server（単位の対象）: @http.get の葉 getMe と、外のシステム ext（設定なし）の葉を呼ぶ配線 sync
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
	wantCodes(t, Check(p, cfg)) // 単位の対象でない名前空間に診断はなく、THROWS-1 も出ない

	// 単位の対象でない名前空間の op の Signature も計算されている
	wantStrings(t, "Sig(user.open).E", sigOf(t, p, "user.open").Exceptions, []string{"http.ServerError", "http.NetworkError"})
	pressSig := sigOf(t, p, "user.press")
	wantStrings(t, "Sig(user.press).F", pressSig.Effects, []string{"ext.record"})
	if got := relOf(t, pressSig, "ext.Down", "ext.record"); got != ir.Prevented {
		t.Errorf("rel(Down, record) = %s, want prevented", got)
	}
	wantStrings(t, "Sig(server.sync).F", sigOf(t, p, "server.sync").Effects, []string{"ext.record"})

	// View は単位の対象の server だけ。namespaces にも、対象でない名前空間は入らない
	views := viewsOf(p, cfg)
	if len(views) != 1 || views["server"] == nil {
		t.Fatalf("views = %v, want only server", reflect.ValueOf(views).MapKeys())
	}
	v := views["server"]
	if len(v.Namespaces) != 1 {
		t.Errorf("namespaces = %+v, want only server", v.Namespaces)
	}
	// ops はすべての op（単位の対象でない名前空間のものも）を server から見た Sig で持つ
	for _, name := range []string{"server.getMe", "server.sync", "ext.push", "user.open", "user.press"} {
		if v.Ops[name] == nil {
			t.Errorf("View(server).ops に %s がない", name)
		}
	}
	wantStrings(t, "View(server).ext.push.E", v.Ops["ext.push"].Exceptions, []string{"ext.Down"})
	wantStrings(t, "View(server).ext.push.F", v.Ops["ext.push"].Effects, []string{"ext.record"})
	// Program は全体そのまま（単位の対象でない名前空間の宣言も読める）
	if v.Program.Lookup("user.press") == nil || v.Program.Lookup("ext.log") == nil {
		t.Error("View.program に、単位の対象でない名前空間の宣言がない")
	}
	// 単位が一つもなければ View は一つもない（設定なしの検査）
	if n := len(Views(p, config.Default("/root"), nil)); n != 0 {
		t.Errorf("views without config = %d, want 0", n)
	}
	wantCodes(t, Check(p, config.Default("/root")))
}

// TestViewsCoverage は docs/config.md「単位の対象」と「in」: 単位は namespace とその子孫を出力し、別の単位が名指しした
// 子孫は外れる。in に合うファイルに宣言を持たない名前空間は、その単位では出力しない。
func TestViewsCoverage(t *testing.T) {
	inFile := func(file string, o *ir.Op) *ir.Op {
		o.Span.File = file
		return o
	}
	p := program(nil,
		namespace("shop", inFile("shop/api.hozo", leaf("shop.open", nil)), inFile("shop/batch.hozo", leaf("shop.nightly", nil))),
		namespace("shop.admin", inFile("shop/admin.hozo", leaf("shop.admin.ban", nil))),
		namespace("shop.admin.audit", inFile("shop/admin.hozo", leaf("shop.admin.audit.log", nil))),
		namespace("shop.cart", inFile("shop/batch.hozo", leaf("shop.cart.sweep", nil))),
		namespace("stock", inFile("stock.hozo", leaf("stock.count", nil))),
	)
	// 0: shop（in は api だけ）、1: shop（in なし）、2: shop.admin
	cfg := cfgFor("shop", "shop", "shop.admin")
	cfg.Units[1].Out = "gen/all"
	cfg.Units[2].Out = "gen/admin"
	units := goUnits(p, cfg)
	units[0].Files = []string{"shop/api.hozo"}
	all := Views(p, cfg, units)
	keys := func(m map[string]*ir.View) []string {
		out := []string{}
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	// 0: in に合うファイルに宣言を持つのは shop だけ。shop.cart は batch.hozo にしか宣言がないので出力しない
	wantStrings(t, "unit 0", keys(all[0]), []string{"shop"})
	// 1: shop と、別の単位が名指ししていない子孫 shop.cart。shop.admin とその下は外れる
	wantStrings(t, "unit 1", keys(all[1]), []string{"shop", "shop.cart"})
	// 2: shop.admin とその子孫
	wantStrings(t, "unit 2", keys(all[2]), []string{"shop.admin", "shop.admin.audit"})
	// Dir(U, N) は out に、単位の namespace からのセグメントを足す
	if d := all[1]["shop.cart"].Config.Dir; d != "gen/all/cart" {
		t.Errorf("Dir(1, shop.cart) = %q", d)
	}
	if d := all[2]["shop.admin.audit"].Config.Dir; d != "gen/admin/audit" {
		t.Errorf("Dir(2, shop.admin.audit) = %q", d)
	}
	v := all[0]["shop"]
	wantStrings(t, "files", v.Files, []string{"shop/api.hozo"})
	// View.InUnit: viewer の宣言のうち span.file が files にあるもの
	if !v.InUnit("shop.open") || v.InUnit("shop.nightly") || v.InUnit("stock.count") || v.InUnit("shop.nope") {
		t.Errorf("InUnit: open=%v nightly=%v", v.InUnit("shop.open"), v.InUnit("shop.nightly"))
	}
	if w := all[1]["shop"]; !w.InUnit("shop.open") || !w.InUnit("shop.nightly") {
		t.Error("the unit without in must have every declaration of the namespace")
	}
	// namespaces: shop は二つの単位が出力する。どの単位の対象でもない stock は入らない
	if len(v.Namespaces) != 4 || len(v.Namespaces["shop"]) != 2 || len(v.Namespaces["shop.cart"]) != 1 || v.Namespaces["stock"] != nil {
		t.Errorf("namespaces = %+v", v.Namespaces)
	}
	// Program は全体そのまま（in の外の宣言も読める）
	if v.Program.Lookup("shop.nightly") == nil || v.Ops["shop.nightly"] == nil {
		t.Error("declarations outside in must stay in View.program and View.ops")
	}
}
