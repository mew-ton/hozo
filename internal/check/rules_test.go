package check

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/ir"
)

// TestThrows1 は THROWS-1: Missing = 呼び出し先の例外 − Declared − Catches − Throws。Related に出どころ、Data.missing に FQN。
func TestThrows1(t *testing.T) {
	a := leaf("server.a", nil)
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y"})}
	b := leaf("server.b", nil)
	b.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Z"})}
	catchY := decorator("server.catchY")
	catchY.Catches = []ir.CatchesClause{catches(ir.Recovers, "", "server.Y")}
	throwZ := decorator("server.throwZ")
	throwZ.External = true
	throwZ.Throws = []ir.ThrowsClause{throws([]string{"server.Z"})}

	t.Run("ng", func(t *testing.T) {
		w := wire("server.w", nil, nil, cs(call("server.a")), cs(call("server.b")))
		p := serverWith(a, b, w)
		d := wantOneCode(t, Checker{Locate: locatorFor()}.Check(p, cfgFor("server")), "THROWS-1")
		wantStrings(t, "missing", d.Data["missing"].([]string), []string{"server.X", "server.Y", "server.Z"})
		if len(d.Related) != 4 { // X from a and b, Y from a, Z from b
			t.Errorf("related = %d, want 4", len(d.Related))
		}
		if !strings.Contains(d.Message, "server.w") || !strings.Contains(d.Message, "server.X") {
			t.Errorf("message = %q", d.Message)
		}
	})
	t.Run("ok", func(t *testing.T) {
		w := wire("server.w", nil, nil, cs(call("server.a")), cs(call("server.b")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Z"})}
		w.Decorators = []ir.Application{app("server.throwZ"), app("server.catchY")}
		p := serverWith(a, b, catchY, throwZ, w)
		diags := Check(p, cfgFor("server"))
		wantNoCode(t, diags, "THROWS-1")
		wantStrings(t, "E", sigOf(t, p, "server.w").Exceptions, []string{"server.X", "server.Z"})
	})
	t.Run("external throws do not cover the body (docs/check.md「throws」)", func(t *testing.T) {
		// external なデコレータの throws は Sig(O, ns(O)) に入らない（DEC-10）ので、THROWS-1 の差し引きにも数えない。
		w := wire("server.w", nil, nil, cs(call("server.a")), cs(call("server.b")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
		w.Decorators = []ir.Application{app("server.throwZ"), app("server.catchY")}
		p := serverWith(a, b, catchY, throwZ, w)
		wantOneCode(t, Check(p, cfgFor("server")), "THROWS-1")
	})
	t.Run("over-declared is allowed (THROWS-2)", func(t *testing.T) {
		w := wire("server.w", nil, nil, cs(call("server.a")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y", "server.W"})}
		wantNoCode(t, Check(serverWith(a, w), cfgFor("server")), "THROWS-1")
	})
}

// TestRel2 は REL-2: 効果を持たない op の throws に relations。デコレータの throws は対象外。
func TestRel2(t *testing.T) {
	o := leaf("server.o", nil)
	o.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	wantOneCode(t, Check(serverWith(o), cfgFor("server")), "REL-2")
	o.Effects = []string{"server.e1"}
	wantNoCode(t, Check(serverWith(o), cfgFor("server")), "REL-2")
	d := decorator("server.d")
	d.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	wantNoCode(t, Check(serverWith(d), cfgFor("server")), "REL-2")
}

// TestRel3 は REL-3: relations.effects が名前の列（op とデコレータ）。all なら出ない。
func TestRel3(t *testing.T) {
	o := leaf("server.o", nil)
	o.Effects = []string{"server.e1"}
	o.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps("server.e1"))}
	d := decorator("server.d")
	d.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents("server.e1"))}
	diags := Check(serverWith(o, d), cfgFor("server"))
	if n := len(withCode(diags, "REL-3")); n != 2 {
		t.Errorf("REL-3 count = %d, want 2\n%s", n, messages(diags))
	}
	o.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps())}
	wantNoCode(t, Check(serverWith(o), cfgFor("server")), "REL-3")
}

// TestRel4 は REL-4: 配線の throws に書いた prevents / keeps が計算と食い違う。
func TestRel4(t *testing.T) {
	a := leaf("server.a", nil)
	a.Effects = []string{"server.e1"}
	b := leaf("server.b", nil)
	b.Effects = []string{"server.e2"}
	b.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	mk := func(cl ir.ThrowsClause) *ir.Program {
		w := wire("server.w", nil, nil, cs(call("server.a")), cs(call("server.b")))
		w.Throws = []ir.ThrowsClause{cl}
		return serverWith(a, b, w)
	}
	// 計算: rel(X, e1) = kept（上の行）、rel(X, e2) = prevented（b の宣言）
	wantNoCode(t, Check(mk(throws([]string{"server.X"}, keeps("server.e1"), prevents("server.e2"))), cfgFor("server")), "REL-4")
	d := wantOneCode(t, Check(mk(throws([]string{"server.X"}, prevents())), cfgFor("server")), "REL-4")
	if !strings.Contains(d.Message, "server.e1") || !strings.Contains(d.Message, "kept") {
		t.Errorf("REL-4 message = %q", d.Message)
	}
	wantOneCode(t, Check(mk(throws([]string{"server.X"}, keeps())), cfgFor("server")), "REL-4")
	// 関係を書かなければ比較しない
	wantNoCode(t, Check(mk(throws([]string{"server.X"})), cfgFor("server")), "REL-4")
}

// TestRel6 は REL-6: relations の効果名が Sig(O, ns(O)).effects にない。
func TestRel6(t *testing.T) {
	o := leaf("server.o", nil)
	o.Effects = []string{"server.e1"}
	o.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps("server.e1", "server.e2"))}
	d := wantOneCode(t, Check(serverWith(o), cfgFor("server")), "REL-6")
	if !strings.Contains(d.Message, "server.e2") || strings.Contains(d.Message, "server.e1") {
		t.Errorf("REL-6 message = %q", d.Message)
	}
	o.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps("server.e1"))}
	wantNoCode(t, Check(serverWith(o), cfgFor("server")), "REL-6")
}

// TestEff2 は EFF-2: 配線の effects が伝播した効果と集合として等しくない。Data.missing / Data.extra。
func TestEff2(t *testing.T) {
	a := leaf("server.a", nil)
	a.Effects = []string{"server.e1", "server.e2"}
	w := wire("server.w", nil, nil, cs(call("server.a")))
	w.Effects = []string{"server.e2", "server.e3"}
	d := wantOneCode(t, Check(serverWith(a, w), cfgFor("server")), "EFF-2")
	wantStrings(t, "missing", d.Data["missing"].([]string), []string{"server.e1"})
	wantStrings(t, "extra", d.Data["extra"].([]string), []string{"server.e3"})
	w.Effects = []string{"server.e2", "server.e1"} // 順序は問わない
	wantNoCode(t, Check(serverWith(a, w), cfgFor("server")), "EFF-2")
	w.Effects = nil // 省略は検査しない
	wantNoCode(t, Check(serverWith(a, w), cfgFor("server")), "EFF-2")
	// halts の効果も伝播した効果に含まれる
	h := decorator("server.halt")
	h.Catches = []ir.CatchesClause{catches(ir.Halts, "server.h", "server.X")}
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	w2 := wire("server.w2", nil, nil, cs(call("server.a")))
	w2.Decorators = []ir.Application{app("server.halt")}
	w2.Effects = []string{"server.e1", "server.e2"}
	d2 := wantOneCode(t, Check(serverWith(a, h, w2), cfgFor("server")), "EFF-2")
	wantStrings(t, "missing (halts)", d2.Data["missing"].([]string), []string{"server.h"})
}

// TestState2 は STATE-2: 葉の op が持ち主でない名前空間の効果を宣言する。デコレータの halts の効果も同じ。
func TestState2(t *testing.T) {
	foreign := leaf("browser.l", nil)
	foreign.Effects = []string{"server.e1"}
	own := leaf("server.l", nil)
	own.Effects = []string{"server.e1"}
	h := decorator("server.halt")
	h.Catches = []ir.CatchesClause{catches(ir.Halts, "server.h", "server.X")}
	decorated := leaf("browser.m", nil)
	decorated.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	decorated.Decorators = []ir.Application{app("server.halt")}
	ok := leaf("server.m", nil)
	ok.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	ok.Decorators = []ir.Application{app("server.halt")}
	// 伝播した効果は別の名前空間の配線に現れてよい
	wiring := wire("browser.w", nil, nil, cs(call("server.l")))
	wiring.Effects = []string{"server.e1"}
	p := program(nil, namespace("server", append(serverBase(), own, h, ok)...), namespace("browser", foreign, decorated, wiring))
	diags := Check(p, cfgFor("server", "browser"))
	s2 := withCode(diags, "STATE-2")
	if len(s2) != 2 {
		t.Fatalf("STATE-2 count = %d, want 2\n%s", len(s2), messages(diags))
	}
	if !strings.Contains(s2[0].Message, "browser.l") || !strings.Contains(s2[1].Message, "browser.m") {
		t.Errorf("STATE-2 messages: %q / %q", s2[0].Message, s2[1].Message)
	}
	wantNoCode(t, diags, "EFF-2")
}

// TestDec6And14 は DEC-6（catches と throws に同じ例外）と DEC-14（external なのに throws がない）。別名の宣言は対象外。
func TestDec6And14(t *testing.T) {
	same := decorator("server.same")
	same.Catches = []ir.CatchesClause{catches(ir.Recovers, "", "server.X")}
	same.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	ext := decorator("server.ext")
	ext.External = true
	extOK := decorator("server.extOK")
	extOK.External = true
	extOK.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	alias := decorator("server.alias")
	alias.External = true
	alias.Alias = &ir.DecoratorAlias{Target: "server.extOK"}
	fallback := decorator("server.fallback")
	fallback.Catches = []ir.CatchesClause{catches(ir.Recovers, "", "server.X")}
	fallback.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
	diags := Check(serverWith(same, ext, extOK, alias, fallback), cfgFor("server"))
	wantOneCode(t, diags, "DEC-6")
	wantOneCode(t, diags, "DEC-14")
}

// TestWire6 は WIRE-6: const の数が [hozo].max_bindings を超える。
func TestWire6(t *testing.T) {
	a := leaf("server.a", ir.String())
	stmts := []ir.Statement{cst("x1", call("server.a")), cst("x2", call("server.a")), cst("x3", call("server.a"))}
	w := wire("server.w", nil, nil, stmts...)
	cfg := cfgFor("server")
	cfg.Hozo.MaxBindings = 2
	wantOneCode(t, Check(serverWith(a, w), cfg), "WIRE-6")
	cfg.Hozo.MaxBindings = 3
	wantNoCode(t, Check(serverWith(a, w), cfg), "WIRE-6")
	wantNoCode(t, Check(serverWith(a, w), nil), "WIRE-6") // 既定の 5
}

// TestDoc1Throws は DOC-1: @throws の例外が Signature.exceptions にない。
func TestDoc1Throws(t *testing.T) {
	o := leaf("server.o", nil)
	o.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	o.Description = &ir.Doc{Summary: "s", Body: "s", Throws: []ir.DocThrows{{Exception: "server.X", Text: "x"}}}
	wantNoCode(t, Check(serverWith(o), cfgFor("server")), "DOC-1")
	o.Description.Throws = append(o.Description.Throws, ir.DocThrows{Exception: "server.Y", Text: "y"})
	d := wantOneCode(t, Check(serverWith(o), cfgFor("server")), "DOC-1")
	if !strings.Contains(d.Message, "server.Y") {
		t.Errorf("DOC-1 message = %q", d.Message)
	}
}

// TestCfg1 は CFG-1: state か op を持つユーザーの名前空間で language か out が未設定。グローバルに state / op があれば CFG-1。語彙は対象外。
func TestCfg1(t *testing.T) {
	p := program(nil,
		namespace("", leaf("g", nil)),
		httpVocab(),
		namespace("server", leaf("server.o", nil)),
		namespace("server.admin", leaf("server.admin.o", nil)),
		namespace("types", model("types.M")),
	)
	cfg := config.Default("/root")
	lang := "go"
	cfg.Namespaces["server"] = config.NamespaceTable{Language: &lang}
	diags := Check(p, cfg)
	c1 := withCode(diags, "CFG-1")
	if len(c1) != 3 { // global, server (out), server.admin (out, inherited language)
		t.Fatalf("CFG-1 count = %d, want 3\n%s", len(c1), messages(diags))
	}
	if c1[0].File != config.FileName || c1[0].Span != nil {
		t.Errorf("CFG-1 position: file=%q span=%v", c1[0].File, c1[0].Span)
	}
	if !strings.Contains(c1[1].Message, "server") || !strings.Contains(c1[1].Message, "out") || strings.Contains(c1[1].Message, "language") {
		t.Errorf("CFG-1 message = %q", c1[1].Message)
	}
	ok := program(nil, httpVocab(), namespace("server", leaf("server.o", nil)), namespace("server.admin", leaf("server.admin.o", nil)))
	wantNoCode(t, Check(ok, cfgFor("server")), "CFG-1") // 子は親の設定を引き継ぐ
	// 設定ファイルのパスが File になる
	cfg2 := cfgFor("server")
	cfg2.Path = "/root/hozo.toml"
	delete(cfg2.Namespaces, "server")
	if d := wantOneCode(t, Check(program(nil, namespace("server", leaf("server.o", nil))), cfg2), "CFG-1"); d.File != "/root/hozo.toml" {
		t.Errorf("CFG-1 file = %q", d.File)
	}
}

// TestCfg2 は CFG-2（.hozo 側）: 名前空間の名前のどの階層かが language / plugins / out / hozo。
func TestCfg2(t *testing.T) {
	p := program(nil,
		namespace("server", leaf("server.o", nil)),
		namespace("server.out", leaf("server.out.o", nil)),
		namespace("hozo", model("hozo.M")),
	)
	diags := Check(p, cfgFor("server", "server.out"))
	c2 := withCode(diags, "CFG-2")
	if len(c2) != 2 {
		t.Fatalf("CFG-2 count = %d, want 2\n%s", len(c2), messages(diags))
	}
	if c2[0].File != "server.hozo" {
		t.Errorf("CFG-2 file = %q (first decl of the namespace)", c2[0].File)
	}
}

// TestCfg8And10 は CFG-8（語彙の名前空間、state も op もない名前空間にテーブル）と CFG-10（.hozo にない名前空間のテーブル）。
func TestCfg8And10(t *testing.T) {
	p := program(nil,
		httpVocab(),
		namespace("server.admin", leaf("server.admin.o", nil)),
		namespace("types", model("types.M")),
	)
	cfg := cfgFor("http", "server", "server.admin", "types", "nowhere")
	diags := Check(p, cfg)
	c8 := withCode(diags, "CFG-8")
	if len(c8) != 2 {
		t.Fatalf("CFG-8 count = %d, want 2 (http, types)\n%s", len(c8), messages(diags))
	}
	d10 := wantOneCode(t, diags, "CFG-10")
	if !strings.Contains(d10.Message, "nowhere") {
		t.Errorf("CFG-10 message = %q", d10.Message)
	}
	// [server] は子孫 server.admin が op を持つので、親のテーブルとして許す
	for _, d := range c8 {
		if strings.Contains(d.Message, "名前空間 server ") {
			t.Errorf("unexpected CFG-8 for parent table: %q", d.Message)
		}
	}
	wantNoCode(t, Check(p, cfgFor("server.admin")), "CFG-8")
	wantNoCode(t, Check(p, cfgFor("server.admin")), "CFG-10")
}

// TestCfg6 は CFG-6: unsupported にあるデコレータが名前空間の宣言（フィールドを含む）に付いている。付いた位置に出す。
func TestCfg6(t *testing.T) {
	m := model("server.M", field("f", ir.String()))
	m.Fields[0].Decorators = []ir.Application{app("valid.min")}
	o := leaf("server.o", nil)
	o.Decorators = []ir.Application{app("http.get"), {Decorator: "browser.redirectOn", Via: "browser.redirectToLogin", Span: sp("@via")}}
	other := leaf("browser.o", nil)
	other.Decorators = []ir.Application{app("http.get")}
	p := program(nil, namespace("server", m, o), namespace("browser", other))
	diags := CheckUnsupported(p, "server", []string{"http.get", "valid.min", "browser.redirectToLogin"})
	if n := len(withCode(diags, "CFG-6")); n != 3 {
		t.Fatalf("CFG-6 count = %d, want 3\n%s", n, messages(diags))
	}
	for _, d := range diags {
		if d.Severity != diag.Warning {
			t.Errorf("CFG-6 severity = %s", d.Severity)
		}
	}
	if n := len(CheckUnsupported(p, "server", nil)); n != 0 {
		t.Errorf("no unsupported → %d diagnostics", n)
	}
	if n := len(CheckUnsupported(p, "nowhere", []string{"http.get"})); n != 0 {
		t.Errorf("unknown namespace → %d diagnostics", n)
	}
}

// TestFileSetLocator は IR の Span（行・列はコードポイント）を FileSet のバイトのオフセットに戻すこと。
func TestFileSetLocator(t *testing.T) {
	fs := source.NewFileSet()
	id := fs.Add("a.hozo", []byte("namespace a {\n  op 日本語(x: string);\n}\n"))
	loc := FileSetLocator(fs)
	got, ok := loc(ir.Span{File: "a.hozo", Start: ir.Pos{Line: 2, Col: 6}, End: ir.Pos{Line: 2, Col: 9}})
	if !ok {
		t.Fatal("not located")
	}
	if got.File != id {
		t.Errorf("file = %d", got.File)
	}
	if pos := fs.Position(id, got.Start); pos != (source.Pos{Line: 2, Col: 6}) {
		t.Errorf("start → %+v", pos)
	}
	if pos := fs.Position(id, got.End); pos != (source.Pos{Line: 2, Col: 9}) {
		t.Errorf("end → %+v", pos)
	}
	if _, ok := loc(ir.Span{File: "missing.hozo"}); ok {
		t.Error("unknown file located")
	}
	// Checker に渡すと Span が付き、File は空
	o := leaf("server.o", nil)
	o.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	d := wantOneCode(t, Checker{Locate: locatorFor()}.Check(serverWith(o), cfgFor("server")), "REL-2")
	if d.Span == nil || d.File != "" {
		t.Errorf("with locator: span=%v file=%q", d.Span, d.File)
	}
}

// TestNilInputs は nil の Program や Config で落ちないこと。
func TestNilInputs(t *testing.T) {
	Compute(nil)
	if n := len(Check(nil, nil)) + len(CheckUnsupported(nil, "x", []string{"y"})) + len(Views(nil, nil)); n != 0 {
		t.Errorf("nil program → %d results", n)
	}
	if n := len(Check(&ir.Program{}, nil)); n != 0 {
		t.Errorf("empty program → %d diagnostics", n)
	}
	if v := Views(&ir.Program{}, nil); len(v) != 0 {
		t.Errorf("empty program → %d views", len(v))
	}
}
