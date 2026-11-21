package check

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/ir"
)

// TestWrittenThrows は、配線の op に書いた throws の節（docs/check.md「状態に触れるもの」の「節に書く要素」、「throws」の THROWS-2）:
// 節に書いた例外は、計算した例外（呼び出し先の例外から、デコレータが吸収したものを除き、足したものを加えたもの）に、足される。
// 計算した例外を、節に書いていなくても、節に、本体から出ない例外を書いていても、診断は出ない。
func TestWrittenThrows(t *testing.T) {
	a := leaf("server.a", nil)
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y"})}
	b := leaf("server.b", nil)
	b.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Z"})}
	catchY := decorator("server.catchY")
	catchY.Catches = []ir.CatchesClause{catches(ir.Recovers, "", "server.Y")}
	throwZ := decorator("server.throwZ")
	throwZ.External = true
	throwZ.Throws = []ir.ThrowsClause{throws([]string{"server.Z"})}

	t.Run("節に書いていない例外も、E に入る。書いた例外は、足される", func(t *testing.T) {
		w := wire("server.w", nil, nil, cs(call("server.a")), cs(call("server.b")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.W"})}
		p := serverWith(a, b, w)
		wantCodes(t, Checker{Locate: locatorFor()}.Check(p, cfgFor("server")))
		wantStrings(t, "w.E", sigOf(t, p, "server.w").Exceptions, []string{"server.X", "server.Y", "server.Z", "server.W"})
	})
	t.Run("節を書かない配線", func(t *testing.T) {
		w := wire("server.w", nil, nil, cs(call("server.a")), cs(call("server.b")))
		outer := wire("server.outer", nil, nil, cs(call("server.w")))
		p := serverWith(a, b, w, outer)
		wantCodes(t, Check(p, cfgFor("server")))
		wantStrings(t, "w.E", sigOf(t, p, "server.w").Exceptions, []string{"server.X", "server.Y", "server.Z"})
		wantStrings(t, "outer.E", sigOf(t, p, "server.outer").Exceptions, []string{"server.X", "server.Y", "server.Z"})
	})
	t.Run("節を書かない配線の例外を、節を書いた呼び出し元が、書いていなくてよい", func(t *testing.T) {
		w := wire("server.w", nil, nil, cs(call("server.a")))
		outer := wire("server.outer", nil, nil, cs(call("server.w")))
		outer.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
		p := serverWith(a, w, outer)
		wantCodes(t, Check(p, cfgFor("server")))
		wantStrings(t, "outer.E", sigOf(t, p, "server.outer").Exceptions, []string{"server.X", "server.Y"})
	})
	t.Run("デコレータが吸収した例外は、E から除かれる", func(t *testing.T) {
		w := wire("server.w", nil, nil, cs(call("server.a")), cs(call("server.b")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Z"})}
		w.Decorators = []ir.Application{app("server.throwZ"), app("server.catchY")}
		p := serverWith(a, b, catchY, throwZ, w)
		wantCodes(t, Check(p, cfgFor("server")))
		wantStrings(t, "E", sigOf(t, p, "server.w").Exceptions, []string{"server.X", "server.Z"})
	})
	t.Run("external throws are not added inside (docs/check.md「throws」)", func(t *testing.T) {
		// external なデコレータの throws は Sig(O, ns(O)) に入らない（DEC-10）。本体から出る Z は、節に書いていなくても、E に入る
		w := wire("server.w", nil, nil, cs(call("server.a")), cs(call("server.b")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
		w.Decorators = []ir.Application{app("server.throwZ"), app("server.catchY")}
		p := serverWith(a, b, catchY, throwZ, w)
		wantCodes(t, Check(p, cfgFor("server")))
		wantStrings(t, "E", sigOf(t, p, "server.w").Exceptions, []string{"server.X", "server.Z"})
	})
	t.Run("over-declared is allowed (THROWS-2)", func(t *testing.T) {
		w := wire("server.w", nil, nil, cs(call("server.a")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y", "server.W"})}
		p := serverWith(a, w)
		wantCodes(t, Check(p, cfgFor("server")))
		wantStrings(t, "E", sigOf(t, p, "server.w").Exceptions, []string{"server.X", "server.Y", "server.W"})
	})
}

// TestRel2 は REL-2: 効果を持たない op の throws に relations。デコレータの throws は対象外。
func TestRel2(t *testing.T) {
	o := leaf("server.o", nil)
	o.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	wantOneCode(t, Check(serverWith(o), cfgFor("server")), "REL-2")
	o.Effects = ir.NamedEffects("server.e1")
	wantNoCode(t, Check(serverWith(o), cfgFor("server")), "REL-2")
	d := decorator("server.d")
	d.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	wantNoCode(t, Check(serverWith(d), cfgFor("server")), "REL-2")
}

// TestRel4 は REL-4: 配線の throws に書いた prevents / keeps が計算と食い違う。目的語は常にすべての効果。
func TestRel4(t *testing.T) {
	a := leaf("server.a", nil)
	a.Effects = ir.NamedEffects("server.e1")
	b := leaf("server.b", nil)
	b.Effects = ir.NamedEffects("server.e2")
	b.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	k := leaf("server.k", nil)
	k.Effects = ir.NamedEffects("server.e2")
	k.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps())}
	mk := func(cl ir.ThrowsClause, callees ...string) *ir.Program {
		var stmts []ir.Statement
		for _, c := range callees {
			stmts = append(stmts, cs(call(c)))
		}
		w := wire("server.w", nil, nil, stmts...)
		w.Throws = []ir.ThrowsClause{cl}
		return serverWith(a, b, k, w)
	}
	// a; b の計算: rel(X, e1) = kept（上の行）、rel(X, e2) = prevented（b の宣言）。どちらを書いても片方が食い違う
	d := wantOneCode(t, Check(mk(throws([]string{"server.X"}, prevents()), "server.a", "server.b"), cfgFor("server")), "REL-4")
	if !strings.Contains(d.Message, "server.e1") || !strings.Contains(d.Message, "kept") {
		t.Errorf("REL-4 message = %q", d.Message)
	}
	d = wantOneCode(t, Check(mk(throws([]string{"server.X"}, keeps()), "server.a", "server.b"), cfgFor("server")), "REL-4")
	if !strings.Contains(d.Message, "server.e2") || !strings.Contains(d.Message, "prevented") {
		t.Errorf("REL-4 message = %q", d.Message)
	}
	// 計算と一致すれば出ない: b だけなら e2 は prevented、a; k なら e1 も e2 も kept
	wantNoCode(t, Check(mk(throws([]string{"server.X"}, prevents()), "server.b"), cfgFor("server")), "REL-4")
	wantNoCode(t, Check(mk(throws([]string{"server.X"}, keeps()), "server.a", "server.k"), cfgFor("server")), "REL-4")
	wantOneCode(t, Check(mk(throws([]string{"server.X"}, keeps()), "server.b"), cfgFor("server")), "REL-4")
	// 関係を書かなければ比較しない
	wantNoCode(t, Check(mk(throws([]string{"server.X"}), "server.a", "server.b"), cfgFor("server")), "REL-4")
}

// TestWrittenEffects は、配線の op に書いた effects の節（docs/check.md「Sig(O, v)」の手順 0 の「節に書いた効果」）: 本体が行き着く
// 効果を、書いていなくてよく、本体が行き着かない効果を書けば、F の末尾に足される。足した効果の rel は、葉と同じに、
// throws の relations から決まり、succ は kept。本体が、もう行き着いている効果を書いても、何も変わらない。
func TestWrittenEffects(t *testing.T) {
	a := leaf("server.a", nil)
	a.Effects = ir.NamedEffects("server.e1", "server.e2")
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	w := wire("server.w", nil, nil, cs(call("server.a")))
	w.Effects = ir.NamedEffects("server.e2", "server.e3")
	p := serverWith(a, w)
	wantCodes(t, Check(p, cfgFor("server")))
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "F（本体の e1、e2 の後ろに、書いた e3）", effectNames(sig.Effects), []string{"server.e1", "server.e2", "server.e3"})
	// 本体が行き着く e2 は、書いても、本体から決めた状態のまま。足した e3 は、関係を書いていないので unknown
	for e, want := range map[string]ir.EffectState{"server.e1": ir.Prevented, "server.e2": ir.Prevented, "server.e3": ir.Unknown} {
		if got := relOf(t, sig, "server.X", e); got != want {
			t.Errorf("rel(X, %s) = %s, want %s", e, got, want)
		}
	}
	if got := succOf(t, sig, "server.e3"); got != ir.Kept {
		t.Errorf("succ(e3) = %s, want kept", got)
	}
	// 関係を付けて書けば、足した効果の rel は、書いたとおり（REL-4 にも当たらない: 本体の e1、e2 も prevented）
	w.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	p = serverWith(a, w)
	wantCodes(t, Check(p, cfgFor("server")))
	if got := relOf(t, sigOf(t, p, "server.w"), "server.X", "server.e3"); got != ir.Prevented {
		t.Errorf("rel(X, e3) = %s, want prevented（書いた prevents）", got)
	}
	// 呼び出し元へも、伝わる
	outer := wire("server.outer", nil, nil, cs(call("server.w")))
	p = serverWith(a, w, outer)
	wantCodes(t, Check(p, cfgFor("server")))
	wantStrings(t, "outer.F", effectNames(sigOf(t, p, "server.outer").Effects), []string{"server.e1", "server.e2", "server.e3"})
	// 節を書かなければ、本体の分だけ
	w.Effects, w.Throws = nil, nil
	p = serverWith(a, w)
	wantCodes(t, Check(p, cfgFor("server")))
	wantStrings(t, "F（節なし）", effectNames(sigOf(t, p, "server.w").Effects), []string{"server.e1", "server.e2"})
	// halts の効果は、デコレータが足す: 節に書いていなくてよい
	h := decorator("server.halt")
	h.Catches = []ir.CatchesClause{catches(ir.Halts, "server.h", "server.X")}
	w2 := wire("server.w2", nil, nil, cs(call("server.a")))
	w2.Decorators = []ir.Application{app("server.halt")}
	w2.Effects = ir.NamedEffects("server.e1", "server.e2")
	p = serverWith(a, h, w2)
	wantCodes(t, Check(p, cfgFor("server")))
	wantStrings(t, "F（halts）", effectNames(sigOf(t, p, "server.w2").Effects), []string{"server.e1", "server.e2", "server.h"})
}

// TestState2 は STATE-2 のうち、このパッケージが出す分: デコレータの halts の効果が、付けた op の名前空間と違う。
// 葉の op が直に持つ効果（自分で書いたものと、デコレータが運ぶもの）の分は、名前の位置を持つ internal/sema が出す
// （internal/sema の testdata/rules/STATE-2.hozo と tests/rules/STATE-2 が確かめる）ので、ここでは重ねて出さない。
func TestState2(t *testing.T) {
	foreign := leaf("browser.l", nil)
	foreign.Effects = ir.NamedEffects("server.e1")
	own := leaf("server.l", nil)
	own.Effects = ir.NamedEffects("server.e1")
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
	wiring.Effects = ir.NamedEffects("server.e1")
	p := program(nil, namespace("server", append(serverBase(), own, h, ok)...), namespace("browser", foreign, decorated, wiring))
	diags := Check(p, cfgFor("server", "browser"))
	s2 := withCode(diags, "STATE-2")
	if len(s2) != 1 {
		t.Fatalf("STATE-2 count = %d, want 1\n%s", len(s2), messages(diags))
	}
	if !strings.Contains(s2[0].Message, "browser.m") {
		t.Errorf("STATE-2 message: %q", s2[0].Message)
	}
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

// TestCfg13 は docs/config.md「単位の対象」（CFG-13。診断なし）: どの単位の対象でもない名前空間（グローバルを含む）は
// 出力しないだけで、state や op を持っていても診断は出ない。語彙も対象外。
func TestCfg13(t *testing.T) {
	// 単位の対象でない名前空間は、state や op を持っていても診断なし（対象の名前空間と並んでいても、単位が一つもなくても）
	mixed := program(nil, namespace("", leaf("g", nil)), httpVocab(), namespace("server", leaf("server.o", nil)),
		namespace("server.admin", leaf("server.admin.o", nil)), namespace("types", model("types.M")),
		namespace("actor", state("actor.s", ir.StateList, ir.String()), leaf("actor.o", nil)))
	wantCodes(t, Check(mixed, cfgFor("server")))
	wantCodes(t, Check(mixed, config.Default("/root")))
	// 同じ名前空間の単位が二つあっても、子孫を名指しする単位があっても同じ
	wantCodes(t, Check(mixed, cfgFor("server", "server", "server.admin")))
}

// TestCfg10 は CFG-10（.hozo にも語彙のファイルにもない名前空間を単位の namespace に書いた）。語彙の名前空間と、
// state も op も持たない型だけの名前空間は、単位の namespace に書ける（CFG-8 は欠番。docs/config.md「単位の対象」）。
func TestCfg10(t *testing.T) {
	p := program(nil,
		httpVocab(),
		namespace("server.admin", leaf("server.admin.o", nil)),
		namespace("types", model("types.M")),
	)
	cfg := cfgFor("http", "server", "server.admin", "types", "nowhere", "nowhere")
	cfg.Path = "/root/hozo.toml"
	diags := Check(p, cfg)
	wantNoCode(t, diags, "CFG-8")
	// 同じ namespace の単位が二つあっても一度だけ。設定ファイルのパスが File になる
	d10 := wantOneCode(t, diags, "CFG-10")
	if !strings.Contains(d10.Message, "nowhere") || d10.File != "/root/hozo.toml" || d10.Span != nil {
		t.Errorf("CFG-10 = %+v", d10)
	}
	// server の単位は、子孫 server.admin があるのでよい。語彙の http、型だけの types もよい
	wantCodes(t, Check(p, cfgFor("http", "server", "server.admin", "types")))
}

// TestCfg22 は CFG-22（docs/config.md「型の置き場」）: 出力が使う宣言の名前空間が、その単位の基底の出力を持たない。
// 表の行（何を数えるか）ごとに一つ。
func TestCfg22(t *testing.T) {
	const T = "types.T"
	ref := ir.Ref(T)
	types := func() ir.Namespace {
		return namespace("types", model(T), exc("types.E"), &ir.Classify{DeclBase: ir.DeclBase{Name: "types.K", Span: sp("types.K")},
			Subject: ir.TypeSubject{Type: ir.String()}, DecidedBy: ir.DecidedByPredicate, Cases: []ir.ClassifyCase{{Name: "A"}}},
			&ir.Classify{DeclBase: ir.DeclBase{Name: "types.ByType", Span: sp("types.ByType")},
				Subject: ir.TypeSubject{Type: ir.Optional(ir.String())}, DecidedBy: ir.DecidedByType, Cases: []ir.ClassifyCase{{Name: "A"}}})
	}
	// on は、絞り込んだ型 string on <分類>.A
	on := func(classify string) ir.Type {
		return ir.CaseType{Classify: classify, Cases: []string{"A"}, Of: ir.String()}
	}
	effectOf := func(t ir.Type) ir.Decl {
		return &ir.Effect{DeclBase: ir.DeclBase{Name: "server.e", Span: sp("server.e")}, Params: []ir.OpParam{param("row", t)}}
	}
	classifyOf := func(fqn string, subject ir.ClassifySubject, as ...ir.CaseTyping) *ir.Classify {
		return &ir.Classify{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)}, Subject: subject,
			Cases: []ir.ClassifyCase{{Name: "A", As: as}}, DecidedBy: ir.DecidedByPredicate}
	}
	withThrows := func(o *ir.Op, x string) *ir.Op {
		o.Throws = []ir.ThrowsClause{throws([]string{x})}
		return o
	}
	outOfUnit := func(o *ir.Op) *ir.Op {
		o.Span.File = "batch.hozo"
		return o
	}
	cases := []struct {
		name  string
		decls []ir.Decl
		want  bool // CFG-22 が出るか
	}{
		// 1: N のすべての model / scalar / union / exception
		{"フィールドの型", []ir.Decl{model("server.M", field("f", ref))}, true},
		{"型の中の型（array、optional）は辿る", []ir.Decl{model("server.M", field("f", ir.Array(ir.Optional(ref))))}, true},
		{"インデックスの値の型", []ir.Decl{&ir.Model{DeclBase: ir.DeclBase{Name: "server.I", Span: sp("server.I")}, Index: &ir.ModelIndex{Name: "name", Key: ir.String(), Value: ir.Array(ref)}}}, true},
		{"インデックスのキーの型", []ir.Decl{&ir.Model{DeclBase: ir.DeclBase{Name: "server.K", Span: sp("server.K")}, Index: &ir.ModelIndex{Name: "name", Key: ir.Ref("types.T"), Value: ir.String()}}}, true},
		{"scalar の元の型", []ir.Decl{scalar("server.S", ir.Ref("types.T"))}, true},
		{"union のアームの型", []ir.Decl{&ir.Union{DeclBase: ir.DeclBase{Name: "server.U", Span: sp("server.U")}, Variant: ir.UnionArms,
			Arms: []ir.UnionArm{{Name: "A", Type: ref}}}}, true},
		{"例外の引数の型", []ir.Decl{&ir.Exception{DeclBase: ir.DeclBase{Name: "server.X", Span: sp("server.X")}, Params: []ir.OpParam{param("value", ref)}}}, true},
		// 2: N のすべての状態、分類、read、効果
		{"read の引数の型", []ir.Decl{&ir.Read{DeclBase: ir.DeclBase{Name: "server.r", Span: sp("server.r")}, Params: []ir.OpParam{param("key", ref)}, Returns: ir.String()}}, true},
		{"read の結果の型", []ir.Decl{&ir.Read{DeclBase: ir.DeclBase{Name: "server.r", Span: sp("server.r")}, Returns: ir.Optional(ref)}}, true},
		{"効果の引数の型", []ir.Decl{&ir.Effect{DeclBase: ir.DeclBase{Name: "server.e", Span: sp("server.e")}, Params: []ir.OpParam{param("row", ref)}}}, true},
		{"状態の型", []ir.Decl{state("server.s", ir.StateList, ref)}, true},
		{"分類の対象の型", []ir.Decl{classifyOf("server.C", ir.TypeSubject{Type: ref})}, true},
		{"ケースの as の型", []ir.Decl{classifyOf("server.C", ir.TypeSubject{Type: ir.String()}, ir.CaseTyping{Type: ref, By: ir.NarrowByPredicate})}, true},
		// 3: 単位の対象の op
		{"op の引数の型", []ir.Decl{leaf("server.o", nil, param("p", ref))}, true},
		{"op の戻り値の型", []ir.Decl{leaf("server.o", ref)}, true},
		{"op の signature の例外", []ir.Decl{withThrows(leaf("server.o", nil), "types.E")}, true},
		{"単位の対象でない op は数えない", []ir.Decl{leaf("server.keep", nil), outOfUnit(leaf("server.o", ref))}, false},
		// 4: 単位の対象の配線が呼ぶ op
		{"配線が呼ぶ、単位の対象でない op の型", []ir.Decl{outOfUnit(leaf("server.o", ref)), wire("server.w", nil, nil, cs(call("server.o")))}, true},
		{"配線が呼ぶ、別の名前空間の op の型", []ir.Decl{wire("server.w", nil, nil, cs(call("other.o")))}, true},
		// 5: 単位の対象の配線の match
		{"述語で決まる、別の名前空間の分類", []ir.Decl{wire("server.w", nil, []ir.OpParam{param("v", ir.String())},
			ir.MatchStmt{Match: ir.Match{Subject: ir.ValueSubject{Atom: nameAtom("v")}, Classify: "types.K",
				Arms: []ir.MatchArm{{Cases: []string{"A"}, Body: ir.NoneBody{}}}}})}, true},
		// 6: N の効果の引数の、絞り込みの分類
		{"効果の引数を絞り込む、述語で決まる、別の名前空間の分類", []ir.Decl{effectOf(on("types.K"))}, true},
		{"効果の引数を絞り込む、型で決まる分類は数えない", []ir.Decl{effectOf(on("types.ByType"))}, false},
		{"効果の引数の、並びの中の絞り込みは数えない", []ir.Decl{effectOf(ir.Array(on("types.K")))}, false},
		// 7: 単位の対象の、本体のない op の戻り値の、絞り込みの分類
		{"本体のない op の戻り値を絞り込む、述語で決まる、別の名前空間の分類", []ir.Decl{leaf("server.o", on("types.K"))}, true},
		{"本体のない op の引数の絞り込みは数えない", []ir.Decl{leaf("server.o", nil, param("p", on("types.K")))}, false},
		{"配線の戻り値の絞り込みは数えない", []ir.Decl{wire("server.w", on("types.K"), []ir.OpParam{param("v", on("types.K"))}, retAtom(nameAtom("v")))}, false},
		{"単位の対象でない、本体のない op の戻り値の絞り込みは数えない", []ir.Decl{leaf("server.keep", nil), outOfUnit(leaf("server.o", on("types.K")))}, false},
		// 数えないもの
		{"デコレータの適用とその引数", []ir.Decl{func() ir.Decl {
			o := leaf("server.o", nil)
			o.Decorators = []ir.Application{app("types.dec", refArg("x", "types.E"))}
			return o
		}()}, false},
		{"別の名前空間の op そのもの（型を持たない）", []ir.Decl{wire("server.w", nil, nil, cs(call("other.plain")))}, false},
		{"同じ名前空間の宣言", []ir.Decl{model("server.A"), model("server.M", field("f", ir.Ref("server.A")))}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lineCounter = 2000
			p := program(nil, namespace("server", c.decls...), types(),
				namespace("other", leaf("other.o", ref), leaf("other.plain", nil)))
			Compute(p)
			cfg := cfgFor("server")
			cfg.Path = "/root/hozo.toml"
			units := []Unit{{Language: "go", Base: "go", Files: []string{"server.hozo", "types.hozo", "other.hozo"}}}
			diags := Checker{}.CheckHomes(p, cfg, units)
			if !c.want {
				wantCodes(t, diags)
				return
			}
			d := wantOneCode(t, diags, "CFG-22")
			if d.Severity != diag.Error || d.File != "/root/hozo.toml" || d.Span != nil ||
				!strings.Contains(d.Message, "1 番目の [[emit]]") || !strings.Contains(d.Message, `namespace = "types"`) {
				t.Errorf("CFG-22 = %+v", d)
			}
			// types を出力する同じ基底の単位を足せば出ない。基底が違う単位では足りない
			cfg2 := cfgFor("server", "types")
			both := []Unit{units[0], {Language: "go", Base: "go", Files: units[0].Files}}
			wantCodes(t, Checker{}.CheckHomes(p, cfg2, both))
			both[1].Base = "go-b"
			wantOneCode(t, Checker{}.CheckHomes(p, cfg2, both), "CFG-22")
		})
	}
}

// TestCfg22Units は CFG-22 の数え方: 単位と、足りない名前空間の組ごとに一つ。基底の決まらない単位（CFG-20）では出さない。
// 祖先の単位が対象にしている名前空間は、出力を持つ。
func TestCfg22Units(t *testing.T) {
	lineCounter = 3000
	p := program(nil,
		namespace("server", model("server.M", field("a", ir.Ref("shop.types.T")), field("b", ir.Ref("shop.types.T")), field("c", ir.Ref("lib.L")))),
		namespace("server.admin", model("server.admin.M", field("a", ir.Ref("shop.types.T")))),
		namespace("shop.types", model("shop.types.T")),
		namespace("lib", model("lib.L")),
	)
	files := []string{"server.hozo", "shop.hozo", "lib.hozo"}
	unit := Unit{Language: "go", Base: "go", Files: files}
	// server の単位は server と server.admin を出力し、どちらも shop.types を使う: shop.types について一つ、lib について一つ
	diags := Checker{}.CheckHomes(p, cfgFor("server"), []Unit{unit})
	if got := withCode(diags, "CFG-22"); len(got) != 2 || len(diags) != 2 {
		t.Fatalf("CFG-22 = %d\n%s", len(got), messages(diags))
	}
	// 祖先の単位（shop）が shop.types を出力する
	diags = Checker{}.CheckHomes(p, cfgFor("server", "shop", "lib"), []Unit{unit, unit, unit})
	wantCodes(t, diags)
	// 基底の決まらない単位では出さない
	wantCodes(t, Checker{}.CheckHomes(p, cfgFor("server"), []Unit{{Language: "go", Files: files}}))
	// in のせいで出力でない名前空間は、置き場にならない
	diags = Checker{}.CheckHomes(p, cfgFor("server", "shop", "lib"), []Unit{unit, {Language: "go", Base: "go", Files: []string{"server.hozo"}}, unit})
	if d := wantOneCode(t, diags, "CFG-22"); !strings.Contains(d.Message, "shop.types") {
		t.Errorf("CFG-22 = %+v", d)
	}
}

// TestCfg6 は CFG-6: unsupported にあるデコレータが、名前空間の宣言（フィールドを含む）のうち単位の対象のものに付いている。
// 付いた位置に出す。
func TestCfg6(t *testing.T) {
	m := model("server.M", field("f", ir.String()))
	m.Fields[0].Decorators = []ir.Application{app("valid.min")}
	o := leaf("server.o", nil)
	o.Decorators = []ir.Application{app("http.get"), {Decorator: "browser.redirectOn", Via: "browser.redirectToLogin", Span: sp("@via")}}
	batch := leaf("server.batch", nil)
	batch.Decorators = []ir.Application{app("http.get")}
	batch.Span.File = "batch.hozo"
	other := leaf("browser.o", nil)
	other.Decorators = []ir.Application{app("http.get")}
	p := program(nil, namespace("server", m, o, batch), namespace("browser", other))
	unsupported := []string{"http.get", "valid.min", "browser.redirectToLogin"}
	// files が nil なら名前空間のすべての宣言
	diags := CheckUnsupported(p, "server", nil, unsupported)
	if n := len(withCode(diags, "CFG-6")); n != 4 {
		t.Fatalf("CFG-6 count = %d, want 4\n%s", n, messages(diags))
	}
	for _, d := range diags {
		if d.Severity != diag.Warning {
			t.Errorf("CFG-6 severity = %s", d.Severity)
		}
	}
	// 単位の in に合うファイルの宣言だけ（batch.hozo の server.batch は単位の対象でない）
	diags = CheckUnsupported(p, "server", []string{"server.hozo"}, unsupported)
	if n := len(withCode(diags, "CFG-6")); n != 3 {
		t.Fatalf("CFG-6 count with in = %d, want 3\n%s", n, messages(diags))
	}
	if n := len(CheckUnsupported(p, "server", []string{}, unsupported)); n != 0 {
		t.Errorf("no file in the unit → %d diagnostics", n)
	}
	if n := len(CheckUnsupported(p, "server", nil, nil)); n != 0 {
		t.Errorf("no unsupported → %d diagnostics", n)
	}
	if n := len(CheckUnsupported(p, "nowhere", nil, []string{"http.get"})); n != 0 {
		t.Errorf("unknown namespace → %d diagnostics", n)
	}
}

// TestFileSetLocator は IR の Span（行・列はコードポイント）を FileSet のバイトのオフセットに戻すこと。
func TestFileSetLocator(t *testing.T) {
	fs := source.NewFileSet()
	id := fs.Add("a.hozo", []byte("namespace a {\n  fn 日本語(x: string);\n}\n"))
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
	if n := len(Check(nil, nil)) + len(CheckUnsupported(nil, "x", nil, []string{"y"})) + len(Views(nil, nil, nil)); n != 0 {
		t.Errorf("nil program → %d results", n)
	}
	if n := len(Check(&ir.Program{}, nil)); n != 0 {
		t.Errorf("empty program → %d diagnostics", n)
	}
	if v := Views(&ir.Program{}, nil, nil); len(v) != 0 {
		t.Errorf("empty program → %d views", len(v))
	}
}
