package check

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// serverBase は server の状態・効果・例外のひな形。e1..e4 は server の効果、X / Y / Z / W は server の例外。
func serverBase() []ir.Decl {
	return []ir.Decl{
		state("server.sA", ir.StateSingle, ir.String()),
		state("server.sB", ir.StateSingle, ir.String()),
		eff("server.e1", "server.sA", ir.OpPut),
		eff("server.e2", "server.sB", ir.OpPut),
		eff("server.e3", "server.sA", ir.OpPut),
		eff("server.e4", "server.sB", ir.OpPut),
		eff("server.h", "server.sA", ir.OpPut),
		exc("server.X"), exc("server.Y"), exc("server.Z"), exc("server.W"),
	}
}

func serverWith(decls ...ir.Decl) *ir.Program {
	return program(nil, namespace("server", append(serverBase(), decls...)...))
}

// TestSigLeaf は手順 0（葉の op）の表の各行: E = Declared、F = effects、rel は relations から、succ は kept。
// relations の目的語は常にすべての効果なので、rel は効果によらない。.hozo の葉の op は effects の節を書けない（STATE-5 は
// internal/sema が出す）が、手順 0 はデコレータが運ぶ効果を持つ葉と、循環に含まれる配線の op（本体を無視して葉として計算する）にも
// 使うので、効果が複数の IR で確かめる。
func TestSigLeaf(t *testing.T) {
	o := leaf("server.op", nil)
	o.Effects = ir.NamedEffects("server.e1", "server.e2", "server.e3")
	o.Throws = []ir.ThrowsClause{
		throws([]string{"server.X"}, prevents()),
		throws([]string{"server.Y"}, keeps()),
		throws([]string{"server.Z"}, prevents(), keeps()),
		throws([]string{"server.W"}),
	}
	p := serverWith(o)
	Compute(p)
	sig := sigOf(t, p, "server.op")
	wantStrings(t, "E", sig.Exceptions, []string{"server.X", "server.Y", "server.Z", "server.W"})
	wantStrings(t, "F", effectNames(sig.Effects), []string{"server.e1", "server.e2", "server.e3"})
	if len(sig.Relations) != 12 {
		t.Errorf("relations = %d, want 12 (E × F)", len(sig.Relations))
	}
	for _, e := range sig.Effects {
		if got := relOf(t, sig, "server.X", e); got != ir.Prevented {
			t.Errorf("rel(X, %s) = %s, want prevented (prevents effects)", e, got)
		}
		if got := relOf(t, sig, "server.Y", e); got != ir.Kept {
			t.Errorf("rel(Y, %s) = %s, want kept (keeps effects)", e, got)
		}
		if got := relOf(t, sig, "server.Z", e); got != ir.Unknown {
			t.Errorf("rel(Z, %s) = %s, want unknown (both prevents and keeps; REL-5)", e, got)
		}
		if got := relOf(t, sig, "server.W", e); got != ir.Unknown {
			t.Errorf("rel(W, %s) = %s, want unknown (no relation)", e, got)
		}
		if got := succOf(t, sig, e); got != ir.Kept {
			t.Errorf("succ(%s) = %s, want kept", e, got)
		}
	}
	// 配線でない op の Call.Bound 埋めは何もしない。Signature の順は E の順、F の順。
	if sig.Relations[0] != (ir.SignatureRelation{Exception: "server.X", Effect: ir.NamedEffect{Effect: "server.e1"}, State: ir.Prevented}) {
		t.Errorf("relations[0] = %+v", sig.Relations[0])
	}
}

// TestSigWiringRows は手順 0（配線の op）: F の連結と重複の寄せ、E の和と Declared、出どころの状態 1〜3、
// 出どころのない例外は Op.throws から。
func TestSigWiringRows(t *testing.T) {
	a := leaf("server.a", ir.String())
	a.Effects = ir.NamedEffects("server.e1", "server.e2")
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	b := leaf("server.b", ir.String())
	b.Effects = ir.NamedEffects("server.e2", "server.e3")
	b.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
	w := wire("server.w", ir.String(), nil,
		cst("x", call("server.a")),
		cst("y", call("server.b")),
		retAtom(nameAtom("y")),
	)
	w.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y"}), throws([]string{"server.Z"}, keeps())}
	p := serverWith(a, b, w)
	Compute(p)
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "F", effectNames(sig.Effects), []string{"server.e1", "server.e2", "server.e3"})
	wantStrings(t, "E", sig.Exceptions, []string{"server.X", "server.Y", "server.Z"})
	cases := []struct {
		x, e string
		want ir.EffectState
		why  string
	}{
		{"server.X", "server.e1", ir.Prevented, "出どころの行の効果は呼び出しの宣言（prevents effects）"},
		{"server.X", "server.e2", ir.Prevented, "同上"},
		{"server.X", "server.e3", ir.Prevented, "出どころより下の行の効果は起きていない"},
		{"server.Y", "server.e1", ir.Kept, "出どころより上の行の効果は起きている"},
		{"server.Y", "server.e2", ir.Kept, "上の行にもあれば規則 1 が先"},
		{"server.Y", "server.e3", ir.Unknown, "b は Y と e3 の関係を宣言していない"},
		{"server.Z", "server.e1", ir.Kept, "出どころのない例外は Op.throws の relations（keeps effects）"},
		{"server.Z", "server.e3", ir.Kept, "同上"},
	}
	for _, c := range cases {
		if got := relOf(t, sig, c.x, c.e); got != c.want {
			t.Errorf("rel(%s, %s) = %s, want %s: %s", c.x, c.e, got, c.want, c.why)
		}
	}
	for _, e := range sig.Effects {
		if got := succOf(t, sig, e); got != ir.Kept {
			t.Errorf("succ(%s) = %s, want kept", e, got)
		}
	}
	wantNoCode(t, Check(p, cfgFor("server")), "THROWS-1")
}

// TestSigMultipleSources は同じ例外が複数の出どころから出るとき、状態が同じならその状態、違えば unknown。
func TestSigMultipleSources(t *testing.T) {
	a := leaf("server.a", nil)
	a.Effects = ir.NamedEffects("server.e1")
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	b := leaf("server.b", nil)
	b.Effects = ir.NamedEffects("server.e2")
	b.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	c := leaf("server.c", nil)
	c.Effects = ir.NamedEffects("server.e3")
	c.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	w := wire("server.w", nil, nil, cs(call("server.a")), cs(call("server.b")), cs(call("server.c")))
	w.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	p := serverWith(a, b, c, w)
	Compute(p)
	sig := sigOf(t, p, "server.w")
	// e1: a では prevented、b と c では上の行なので kept → unknown
	if got := relOf(t, sig, "server.X", "server.e1"); got != ir.Unknown {
		t.Errorf("rel(X, e1) = %s, want unknown (differs by source)", got)
	}
	// e2: a では下の行 → prevented、b では宣言なし → unknown、c では上の行 → kept → unknown
	if got := relOf(t, sig, "server.X", "server.e2"); got != ir.Unknown {
		t.Errorf("rel(X, e2) = %s, want unknown", got)
	}
	// e3: a と b では下の行 → prevented、c では prevents effects → prevented → prevented
	if got := relOf(t, sig, "server.X", "server.e3"); got != ir.Prevented {
		t.Errorf("rel(X, e3) = %s, want prevented (same at every source)", got)
	}
}

// TestSigMatchArms は match の行: アームごとの呼び出しが出どころになり、一部のアームにしかない効果の succ は unknown、
// 別のアームの効果は出どころから見て prevented。
func TestSigMatchArms(t *testing.T) {
	a := leaf("server.a", ir.String())
	a.Effects = ir.NamedEffects("server.e1")
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	b := leaf("server.b", ir.String())
	b.Effects = ir.NamedEffects("server.e1", "server.e2")
	d := leaf("server.d", nil)
	d.Effects = ir.NamedEffects("server.e3")
	ca, cb := call("server.a"), call("server.b")
	cls := &ir.Classify{DeclBase: ir.DeclBase{Name: "server.C", Span: sp("server.C")}, Subject: ir.TypeSubject{Type: ir.String()},
		Cases: []ir.ClassifyCase{{Name: "A"}, {Name: "B"}, {Name: "C"}}}
	w := wire("server.w", nil, []ir.OpParam{param("x", ir.String())},
		matchConst("r", "server.C", arm(&ca, "A"), arm(&cb, "B"), arm(nil)),
		cs(call("server.d")),
	)
	w.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	p := serverWith(cls, a, b, d, w)
	Compute(p)
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "F", effectNames(sig.Effects), []string{"server.e1", "server.e2", "server.e3"})
	if got := succOf(t, sig, "server.e1"); got != ir.Unknown {
		t.Errorf("succ(e1) = %s, want unknown (none のアームにはない)", got)
	}
	if got := succOf(t, sig, "server.e2"); got != ir.Unknown {
		t.Errorf("succ(e2) = %s, want unknown (一部のアームだけ)", got)
	}
	if got := succOf(t, sig, "server.e3"); got != ir.Kept {
		t.Errorf("succ(e3) = %s, want kept", got)
	}
	if got := relOf(t, sig, "server.X", "server.e1"); got != ir.Unknown {
		t.Errorf("rel(X, e1) = %s, want unknown (a は宣言していない)", got)
	}
	if got := relOf(t, sig, "server.X", "server.e2"); got != ir.Prevented {
		t.Errorf("rel(X, e2) = %s, want prevented (別のアーム)", got)
	}
	if got := relOf(t, sig, "server.X", "server.e3"); got != ir.Prevented {
		t.Errorf("rel(X, e3) = %s, want prevented (下の行)", got)
	}

	// すべてのアームにある効果は kept のまま
	w2 := wire("server.w2", nil, []ir.OpParam{param("x", ir.String())},
		matchConst("r", "server.C", arm(&ca, "A"), arm(&cb, "B")))
	w2.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	p2 := serverWith(cls, a, b, w2)
	Compute(p2)
	if got := succOf(t, sigOf(t, p2, "server.w2"), "server.e1"); got != ir.Kept {
		t.Errorf("succ(e1) = %s, want kept (every arm)", got)
	}
}

// TestSigStateMatch は状態での match（docs/ir.md「配線」の Match.subject が state / classify）の計算が、値の match と
// 同じ規則（手順 0 の「match ならアームごとの呼び出し」）であること。検査器は遷移を行をまたいで追わない
// （docs/check.md「状態の形」の「検査器が遷移を使う範囲」）ので、効果の宛先が組み込みの効果でも遷移でも Sig は同じ。
func TestSigStateMatch(t *testing.T) {
	a := leaf("server.a", nil)
	a.Effects = ir.NamedEffects("server.e1")
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	b := leaf("server.b", nil)
	b.Effects = ir.NamedEffects("server.e1", "server.move")
	d := leaf("server.d", nil)
	d.Effects = ir.NamedEffects("server.drain")
	decls := []ir.Decl{
		state("server.items", ir.StateList, ir.String()),
		&ir.Classify{DeclBase: ir.DeclBase{Name: "server.Phase", Span: sp("server.Phase")},
			Subject: ir.StatesSubject{States: []string{"server.sA", "server.items"}},
			Cases:   []ir.ClassifyCase{{Name: "Idle"}, {Name: "Busy"}}},
		transition("server.move", "server.Phase", ir.Cases("Idle"), ir.Cases("Busy"), "server.sA", "server.items"),
		transition("server.drain", "server.items", ir.Cases(ir.CaseHasItems), ir.Cases(ir.CaseEmpty), "server.items"),
		a, b, d,
	}
	for _, tt := range []struct {
		name  string
		match func(arms ...ir.MatchArm) ir.Statement
		cases [2]string
	}{
		{"by list", func(arms ...ir.MatchArm) ir.Statement { return matchState("server.items", ir.StateList, arms...) }, [2]string{ir.CaseEmpty, ir.CaseHasItems}},
		{"by classify", func(arms ...ir.MatchArm) ir.Statement { return matchByClassify("server.Phase", arms...) }, [2]string{"Idle", "Busy"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ca, cb := call("server.a"), call("server.b")
			w := wire("server.w", nil, nil,
				tt.match(arm(&ca, tt.cases[0]), arm(&cb, tt.cases[1])),
				cs(call("server.d")),
			)
			w.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
			p := serverWith(append(append([]ir.Decl{}, decls...), w)...)
			Compute(p)
			sig := sigOf(t, p, "server.w")
			wantStrings(t, "F", effectNames(sig.Effects), []string{"server.e1", "server.move", "server.drain"})
			wantStrings(t, "E", sig.Exceptions, []string{"server.X"})
			if got := succOf(t, sig, "server.e1"); got != ir.Kept {
				t.Errorf("succ(e1) = %s, want kept (すべてのアームにある)", got)
			}
			if got := succOf(t, sig, "server.move"); got != ir.Unknown {
				t.Errorf("succ(move) = %s, want unknown (一部のアームだけ)", got)
			}
			if got := succOf(t, sig, "server.drain"); got != ir.Kept {
				t.Errorf("succ(drain) = %s, want kept", got)
			}
			if got := relOf(t, sig, "server.X", "server.move"); got != ir.Prevented {
				t.Errorf("rel(X, move) = %s, want prevented (別のアーム)", got)
			}
			if got := relOf(t, sig, "server.X", "server.drain"); got != ir.Prevented {
				t.Errorf("rel(X, drain) = %s, want prevented (下の行)", got)
			}
			// 規則の検査も通る（EFF-2 / THROWS-1 / STATE-2 は宛先の形によらない）
			for _, dg := range Check(p, cfgFor("server")) {
				if dg.Severity == diag.Error {
					t.Errorf("想定しない error: %s %s", dg.Code, dg.Message)
				}
			}
		})
	}
}

// TestSigSuccPrevented は succ の集約: 呼び出し先の succ がすべて prevented なら prevented、混ざれば unknown。
func TestSigSuccPrevented(t *testing.T) {
	stop := decorator("server.stop")
	stop.Catches = []ir.CatchesClause{catches(ir.Halts, "server.h", "server.X")}
	pp := leaf("server.p", nil)
	pp.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	pp.Decorators = []ir.Application{app("server.stop")}
	q := leaf("server.q", nil)
	q.Effects = ir.NamedEffects("server.h")
	w1 := wire("server.w1", nil, nil, cs(call("server.p")))
	w2 := wire("server.w2", nil, nil, cs(call("server.p")), cs(call("server.q")))
	p := serverWith(stop, pp, q, w1, w2)
	Compute(p)
	ps := sigOf(t, p, "server.p")
	wantStrings(t, "p.E", ps.Exceptions, nil)
	wantStrings(t, "p.F", effectNames(ps.Effects), []string{"server.h"})
	if got := succOf(t, ps, "server.h"); got != ir.Prevented {
		t.Errorf("succ(p, h) = %s, want prevented (halts)", got)
	}
	if got := succOf(t, sigOf(t, p, "server.w1"), "server.h"); got != ir.Prevented {
		t.Errorf("succ(w1, h) = %s, want prevented (all prevented)", got)
	}
	if got := succOf(t, sigOf(t, p, "server.w2"), "server.h"); got != ir.Unknown {
		t.Errorf("succ(w2, h) = %s, want unknown (mixed)", got)
	}
}

// TestSigCatchesRecovers は手順 i の catches recovers: rel(X, e) ≠ kept の e は succ が unknown になり（DEC-5）、X は E から消える。
func TestSigCatchesRecovers(t *testing.T) {
	r := decorator("server.recover")
	r.Catches = []ir.CatchesClause{catches(ir.Recovers, "", "server.X")}
	// 効果ごとの葉の op: first は e1、l は e2 と例外。w は first; l の順に呼ぶので、X のとき e1 は kept（上の行）、
	// e2 は l の宣言（relations なし）から unknown
	first := leaf("server.first", nil)
	first.Effects = ir.NamedEffects("server.e1")
	l := leaf("server.l", ir.String())
	l.Effects = ir.NamedEffects("server.e2")
	l.Throws = []ir.ThrowsClause{throws([]string{"server.X"}), throws([]string{"server.Y"})}
	w := wire("server.w", ir.String(), nil, cs(call("server.first")), ret(call("server.l")))
	w.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
	w.Decorators = []ir.Application{app("server.recover")}
	p := serverWith(r, first, l, w)
	diags := Check(p, cfgFor("server"))
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "E", sig.Exceptions, []string{"server.Y"})
	if got := succOf(t, sig, "server.e1"); got != ir.Kept {
		t.Errorf("succ(e1) = %s, want kept (X のとき e1 は上の行で起きている)", got)
	}
	if got := succOf(t, sig, "server.e2"); got != ir.Unknown {
		t.Errorf("succ(e2) = %s, want unknown (X と e2 の関係は不明)", got)
	}
	d := wantOneCode(t, diags, "DEC-5")
	if d.Severity.String() != "warning" {
		t.Errorf("DEC-5 severity = %s", d.Severity)
	}
	wantNoCode(t, diags, "THROWS-1")

	// すべて kept なら DEC-5 は出ない
	l.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps()), throws([]string{"server.Y"})}
	p2 := serverWith(r, first, l, w)
	wantNoCode(t, Check(p2, cfgFor("server")), "DEC-5")
	// F が空なら起きない
	first.Effects = nil
	l.Effects = nil
	p3 := serverWith(r, first, l, w)
	wantNoCode(t, Check(p3, cfgFor("server")), "DEC-5")
}

// TestSigCatchesIgnores は ignores: recovers と同じく succ は unknown になるが DEC-5 は出ない。値を返す op なら DEC-4。
func TestSigCatchesIgnores(t *testing.T) {
	ig := decorator("server.ignore")
	ig.Catches = []ir.CatchesClause{catches(ir.Ignores, "", "server.X")}
	l := leaf("server.l", nil)
	l.Effects = ir.NamedEffects("server.e1")
	l.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	w := wire("server.w", nil, nil, cs(call("server.l")))
	w.Decorators = []ir.Application{app("server.ignore")}
	p := serverWith(ig, l, w)
	diags := Check(p, cfgFor("server"))
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "E", sig.Exceptions, nil)
	if got := succOf(t, sig, "server.e1"); got != ir.Unknown {
		t.Errorf("succ(e1) = %s, want unknown", got)
	}
	wantNoCode(t, diags, "DEC-5")
	wantNoCode(t, diags, "DEC-4")

	w2 := wire("server.w2", ir.String(), nil, ret(call("server.l2")))
	l2 := leaf("server.l2", ir.String())
	l2.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	w2.Decorators = []ir.Application{app("server.ignore")}
	wantOneCode(t, Check(serverWith(ig, l2, w2), cfgFor("server")), "DEC-4")
}

// TestSigCatchesHalts は halts h: X が消え、h が F になければ末尾に足して残りの Y との rel と succ を prevented に、あればそのまま。
func TestSigCatchesHalts(t *testing.T) {
	h := decorator("server.halt")
	h.Catches = []ir.CatchesClause{catches(ir.Halts, "server.h", "server.X")}
	l := leaf("server.l", nil)
	l.Effects = ir.NamedEffects("server.e1")
	l.Throws = []ir.ThrowsClause{throws([]string{"server.X"}), throws([]string{"server.Y"}, prevents())}
	w := wire("server.w", nil, nil, cs(call("server.l")))
	w.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
	w.Decorators = []ir.Application{app("server.halt")}
	p := serverWith(h, l, w)
	Compute(p)
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "E", sig.Exceptions, []string{"server.Y"})
	wantStrings(t, "F", effectNames(sig.Effects), []string{"server.e1", "server.h"})
	if got := relOf(t, sig, "server.Y", "server.h"); got != ir.Prevented {
		t.Errorf("rel(Y, h) = %s, want prevented", got)
	}
	if got := relOf(t, sig, "server.Y", "server.e1"); got != ir.Prevented {
		t.Errorf("rel(Y, e1) = %s, want prevented (from l)", got)
	}
	if got := succOf(t, sig, "server.h"); got != ir.Prevented {
		t.Errorf("succ(h) = %s, want prevented", got)
	}
	if got := succOf(t, sig, "server.e1"); got != ir.Kept {
		t.Errorf("succ(e1) = %s, want kept", got)
	}

	// h が既に F にあればそのまま
	l.Effects = ir.NamedEffects("server.h", "server.e1")
	p2 := serverWith(h, l, w)
	Compute(p2)
	sig2 := sigOf(t, p2, "server.w")
	wantStrings(t, "F", effectNames(sig2.Effects), []string{"server.h", "server.e1"})
	if got := succOf(t, sig2, "server.h"); got != ir.Kept {
		t.Errorf("succ(h) = %s, want kept (already in F)", got)
	}
	if got := relOf(t, sig2, "server.Y", "server.h"); got != ir.Prevented {
		t.Errorf("rel(Y, h) = %s, want prevented (from l's prevents effects)", got)
	}
}

// TestSigThrowsClause は手順 i の throws: Y を E の末尾に足し、relations から状態を決める。
// 既に E にあって状態が違えば unknown、同じならそのまま。
func TestSigThrowsClause(t *testing.T) {
	tp := decorator("server.tp")
	tp.Throws = []ir.ThrowsClause{throws([]string{"server.Z"}, prevents())}
	tk := decorator("server.tk")
	tk.Throws = []ir.ThrowsClause{throws([]string{"server.W"}, keeps())}
	ty := decorator("server.ty")
	ty.Throws = []ir.ThrowsClause{throws([]string{"server.Y"}, prevents())}
	tx := decorator("server.tx")
	tx.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps())}
	l := leaf("server.l", nil)
	l.Effects = ir.NamedEffects("server.e1", "server.e2")
	l.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y"}, keeps())}
	l.Decorators = []ir.Application{app("server.tp"), app("server.tk"), app("server.ty"), app("server.tx")}
	p := serverWith(tp, tk, ty, tx, l)
	Compute(p)
	sig := sigOf(t, p, "server.l")
	wantStrings(t, "E", sig.Exceptions, []string{"server.X", "server.Y", "server.W", "server.Z"})
	for _, e := range sig.Effects {
		if got := relOf(t, sig, "server.Z", e); got != ir.Prevented {
			t.Errorf("rel(Z, %s) = %s, want prevented", e, got)
		}
		if got := relOf(t, sig, "server.Y", e); got != ir.Unknown {
			t.Errorf("rel(Y, %s) = %s, want unknown (kept from l, prevented from @ty)", e, got)
		}
		if got := relOf(t, sig, "server.X", e); got != ir.Kept {
			t.Errorf("rel(X, %s) = %s, want kept (same state twice)", e, got)
		}
	}
	for _, e := range sig.Effects {
		if got := relOf(t, sig, "server.W", e); got != ir.Kept {
			t.Errorf("rel(W, %s) = %s, want kept (@tk の keeps effects は F のすべて)", e, got)
		}
	}
}

// TestSigExternal は DEC-10: external なデコレータの throws は v = ns(O) なら飛ばし、別の名前空間から見れば加わる。
func TestSigExternal(t *testing.T) {
	getMe := leaf("server.getMe", ir.String())
	getMe.Decorators = []ir.Application{app("http.get", ir.Arg{Param: "path", Value: ir.LiteralValue{Literal: ir.StringLiteral{Value: "/me"}}})}
	inside := wire("server.inside", ir.String(), nil, ret(call("server.getMe")))
	outside := wire("browser.fetchMe", ir.String(), nil, ret(call("server.getMe")))
	p := program(nil, httpVocab(), namespace("server", getMe, inside), namespace("browser", outside))
	diags := Check(p, cfgFor("server", "browser"))
	wantStrings(t, "Sig(getMe, server).E", sigOf(t, p, "server.getMe").Exceptions, nil)
	wantStrings(t, "Sig(inside, server).E", sigOf(t, p, "server.inside").Exceptions, nil)
	wantStrings(t, "Sig(fetchMe, browser).E", sigOf(t, p, "browser.fetchMe").Exceptions, []string{"http.ServerError", "http.NetworkError"})
	// throws の節を書いていない配線には、THROWS-1 を出さない（docs/check.md「状態に触れるもの」の「節に書く要素」）
	wantNoCode(t, diags, "THROWS-1")
	// 節を書いた配線は、計算した例外を、節が覆うこと
	written := wire("browser.fetchMe", ir.String(), nil, ret(call("server.getMe")))
	written.Throws = []ir.ThrowsClause{throws([]string{"browser.Offline"})}
	pw := program(nil, httpVocab(), namespace("server", getMe, inside), namespace("browser", exc("browser.Offline"), written))
	d := wantOneCode(t, Check(pw, cfgFor("server", "browser")), "THROWS-1")
	if d.Message == "" || d.Data["missing"] == nil {
		t.Errorf("THROWS-1 without message or data: %+v", d)
	}
	wantStrings(t, "Data.missing", d.Data["missing"].([]string), []string{"http.ServerError", "http.NetworkError"})
	wantStrings(t, "Sig(fetchMe 節あり, browser).E", sigOf(t, pw, "browser.fetchMe").Exceptions, []string{"http.ServerError", "http.NetworkError", "browser.Offline"})
	views := viewsOf(p, cfgFor("server", "browser"))
	wantStrings(t, "View(browser).ops[server.getMe].E", views["browser"].Ops["server.getMe"].Exceptions, []string{"http.ServerError", "http.NetworkError"})
	wantStrings(t, "View(server).ops[server.getMe].E", views["server"].Ops["server.getMe"].Exceptions, nil)
}

// TestSigParamSubstitution は `$param:` の置き換え（ref / refs / list）と、別名のデコレータを通した適用。
func TestSigParamSubstitution(t *testing.T) {
	redirectOn := decorator("browser.redirectOn",
		ir.Param{Name: "error", Type: ir.AnyParam{Of: ir.AnyException}},
		ir.Param{Name: "to", Type: ir.AnyParam{Of: ir.AnyEffect}})
	redirectOn.Catches = []ir.CatchesClause{catches(ir.Halts, "$param:to", "$param:error")}
	alias := decorator("browser.redirectToLogin")
	alias.Alias = &ir.DecoratorAlias{Target: "browser.redirectOn", Args: []ir.Arg{
		refArg("error", "server.X"), refArg("to", "browser.go")}}
	l := leaf("browser.l", nil)
	l.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y", "server.Z"})}
	l.Decorators = []ir.Application{app("browser.redirectOn", refsArg("error", "server.X", "server.Y"), refArg("to", "browser.go"))}
	viaAlias := leaf("browser.m", nil)
	viaAlias.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y"})}
	viaAlias.Decorators = []ir.Application{{Decorator: "browser.redirectToLogin", Span: sp("@alias")}}
	expanded := leaf("browser.n", nil)
	expanded.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y"})}
	expanded.Decorators = []ir.Application{{Decorator: "browser.redirectOn", Via: "browser.redirectToLogin",
		Args: []ir.Arg{refArg("error", "server.X"), refArg("to", "browser.go")}, Span: sp("@expanded")}}
	listed := leaf("browser.o", nil)
	listed.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y"})}
	listed.Decorators = []ir.Application{app("browser.redirectOn",
		ir.Arg{Param: "error", Value: ir.ListValue{Items: []ir.ArgValue{ir.RefValue{Target: "server.X"}, ir.RefValue{Target: "server.Y"}}}},
		refArg("to", "browser.go"))}
	p := program(nil,
		namespace("server", exc("server.X"), exc("server.Y"), exc("server.Z")),
		namespace("browser", state("browser.loc", ir.StateSingle, ir.String()), eff("browser.go", "browser.loc", ir.OpPut),
			redirectOn, alias, l, viaAlias, expanded, listed))
	Compute(p)
	sig := sigOf(t, p, "browser.l")
	wantStrings(t, "E", sig.Exceptions, []string{"server.Z"})
	wantStrings(t, "F", effectNames(sig.Effects), []string{"browser.go"})
	if got := relOf(t, sig, "server.Z", "browser.go"); got != ir.Prevented {
		t.Errorf("rel(Z, go) = %s, want prevented", got)
	}
	wantStrings(t, "alias E", sigOf(t, p, "browser.m").Exceptions, []string{"server.Y"})
	wantStrings(t, "expanded E", sigOf(t, p, "browser.n").Exceptions, []string{"server.Y"})
	wantStrings(t, "list E", sigOf(t, p, "browser.o").Exceptions, nil)
}

// TestSigDecoratorOrder は Op.decorators の末尾が最も内側で最初に適用されること。
func TestSigDecoratorOrder(t *testing.T) {
	addX := decorator("server.addX")
	addX.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	catchX := decorator("server.catchX")
	catchX.Catches = []ir.CatchesClause{catches(ir.Recovers, "", "server.X")}
	// 書いた順: @addX @catchX → catchX が内側。葉は X を投げないので DEC-16、その後 addX が足す → E = [X]
	a := leaf("server.a", ir.String())
	a.Decorators = []ir.Application{app("server.addX"), app("server.catchX")}
	// 書いた順: @catchX @addX → addX が内側で X を足し、catchX が吸収 → E = []
	b := leaf("server.b", ir.String())
	b.Decorators = []ir.Application{app("server.catchX"), app("server.addX")}
	p := serverWith(addX, catchX, a, b)
	diags := Check(p, cfgFor("server"))
	wantStrings(t, "a.E", sigOf(t, p, "server.a").Exceptions, []string{"server.X"})
	wantStrings(t, "b.E", sigOf(t, p, "server.b").Exceptions, nil)
	d := wantOneCode(t, diags, "DEC-16")
	if d.File != "server.hozo" {
		t.Errorf("DEC-16 file = %q", d.File)
	}
}

// TestSigCycle は WIRE-7: 配線の呼び出しの循環。循環上の op は葉として計算し、Related に循環上のすべての op を示す。
func TestSigCycle(t *testing.T) {
	a := wire("server.a", nil, nil, cs(call("server.b")))
	a.Effects = ir.NamedEffects("server.e1")
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	b := wire("server.b", nil, nil, cs(call("server.c")))
	c := wire("server.c", nil, nil, cs(call("server.a")))
	self := wire("server.self", nil, nil, cs(call("server.self")))
	d := wire("server.d", nil, nil, cs(call("server.a")))
	d.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
	p := serverWith(a, b, c, self, d)
	diags := Checker{Locate: locatorFor()}.Check(p, cfgFor("server"))
	w7 := withCode(diags, "WIRE-7")
	if len(w7) != 2 {
		t.Fatalf("WIRE-7 count = %d, want 2 (a→b→c→a and self)\n%s", len(w7), messages(diags))
	}
	if len(w7[0].Related) != 3 {
		t.Errorf("WIRE-7 related = %d, want 3", len(w7[0].Related))
	}
	if w7[0].Span == nil {
		t.Errorf("WIRE-7 has no span with locator")
	}
	sa := sigOf(t, p, "server.a")
	wantStrings(t, "a.E (as leaf)", sa.Exceptions, []string{"server.X"})
	wantStrings(t, "a.F (as leaf)", effectNames(sa.Effects), []string{"server.e1"})
	wantStrings(t, "b.E", sigOf(t, p, "server.b").Exceptions, nil)
	wantStrings(t, "d.E", sigOf(t, p, "server.d").Exceptions, []string{"server.X", "server.Y"})
	wantNoCode(t, diags, "EFF-2") // 循環上の配線の effects は自分の宣言と一致する
	if n := len(withCode(diags, "THROWS-1")); n != 1 {
		t.Errorf("THROWS-1 count = %d, want 1 (throws の節を書いた d が、a の X を覆わない)", n)
	}
}

// TestComputeIdempotent は Compute を二度呼んでも同じ結果になること（Check は Compute を含む）。
func TestComputeIdempotent(t *testing.T) {
	a := leaf("server.a", nil)
	a.Effects = ir.NamedEffects("server.e1")
	w := wire("server.w", nil, nil, cs(call("server.a")))
	p := serverWith(a, w)
	Compute(p)
	first := sigOf(t, p, "server.w")
	Compute(p)
	Check(p, cfgFor("server"))
	second := sigOf(t, p, "server.w")
	wantStrings(t, "F", effectNames(second.Effects), effectNames(first.Effects))
	if len(second.Relations) != len(first.Relations) || len(second.SuccessEffects) != len(first.SuccessEffects) {
		t.Errorf("signature changed on recompute")
	}
}

// TestSigCarried は docs/check.md「デコレータが運ぶ reads と effects」: 葉の op の F は直に持つ効果（Op.effects の後ろに
// Op.carried.effects。重複は最初の出現）、R は直に読む状態（Op.reads の後ろに Op.carried.reads）。呼び出し元へは、op が
// 書いたものと同じに伝わる（EFF-1）。手順 i（catches / throws）は、運ばれた効果を op の中身の分として扱う。
// 配線の op も、運ばれたものを持つ（本体が行き着くものの後ろ。細かい場合は TestSigCarriedWiring）。循環していて葉として
// 計算する op は、節を書いた葉と同じ: 書いたものの後ろに、運ばれたもの。
func TestSigCarried(t *testing.T) {
	doing := decorator("server.doing")
	doing.Reads = ir.StateReads("$param:table")
	doing.Effects = ir.NamedEffects("$param:targets")
	doing.Throws = []ir.ThrowsClause{throws([]string{"server.Y"}, prevents())}

	carried := leaf("server.carried", nil)
	carried.Decorators = []ir.Application{app("server.doing")}
	carried.Effects = ir.NamedEffects("server.e2")
	carried.Reads = ir.StateReads("server.sB")
	carried.Carried = ir.Carried{Reads: ir.StateReads("server.sA", "server.sB"), Effects: ir.NamedEffects("server.e1", "server.e2", "server.e3")}
	carried.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps())}

	caller := wire("server.caller", nil, nil, cs(call("server.carried")))
	caller.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y"})}

	// 配線に運ばれたものも、その配線の F と R に入る
	wired := wire("server.wired", nil, nil, cs(call("server.plain")))
	wired.Carried = ir.Carried{Reads: ir.StateReads("server.sA"), Effects: ir.NamedEffects("server.e4")}
	plain := leaf("server.plain", nil)

	// 循環上の op は葉として計算する: 書いたものの後ろに、運ばれたもの
	loop := wire("server.loop", nil, nil, cs(call("server.loop")))
	loop.Effects = ir.NamedEffects("server.e1")
	loop.Reads = ir.StateReads("server.sA")
	loop.Carried = ir.Carried{Reads: ir.StateReads("server.sB"), Effects: ir.NamedEffects("server.e2")}

	p := serverWith(doing, carried, caller, wired, plain, loop)
	Compute(p)

	sig := sigOf(t, p, "server.carried")
	wantStrings(t, "葉の F（書いたもの、運ばれたもの）", effectNames(sig.Effects), []string{"server.e2", "server.e1", "server.e3"})
	wantStrings(t, "葉の R（書いたもの、運ばれたもの）", sig.Reads, []string{"server.sB", "server.sA"})
	wantStrings(t, "葉の E", sig.Exceptions, []string{"server.X", "server.Y"})
	for _, e := range sig.Effects {
		if got := relOf(t, sig, "server.X", e); got != ir.Kept {
			t.Errorf("rel(X, %s) = %s, want kept（op の throws の relations は、運ばれた効果にも掛かる）", e, got)
		}
		if got := relOf(t, sig, "server.Y", e); got != ir.Prevented {
			t.Errorf("rel(Y, %s) = %s, want prevented（デコレータの throws の relations も同じ）", e, got)
		}
		if got := succOf(t, sig, e); got != ir.Kept {
			t.Errorf("succ(%s) = %s, want kept", e, got)
		}
	}
	if sig.Pure {
		t.Error("デコレータの付いた op は純粋でない")
	}
	// Op.reads / Op.effects / Op.carried は書き換えない
	wantStrings(t, "Op.effects", effectNames(carried.Effects), []string{"server.e2"})
	wantStrings(t, "Op.carried.effects", effectNames(carried.Carried.Effects), []string{"server.e1", "server.e2", "server.e3"})

	up := sigOf(t, p, "server.caller")
	wantStrings(t, "呼び出し元の F", effectNames(up.Effects), []string{"server.e2", "server.e1", "server.e3"})
	wantStrings(t, "呼び出し元の R", up.Reads, []string{"server.sB", "server.sA"})

	w := sigOf(t, p, "server.wired")
	wantStrings(t, "配線の F（運ばれたもの）", effectNames(w.Effects), []string{"server.e4"})
	wantStrings(t, "配線の R（運ばれたもの）", w.Reads, []string{"server.sA"})
	if w.Pure {
		t.Error("運ばれたものを持つ配線は純粋でない")
	}

	l := sigOf(t, p, "server.loop")
	wantStrings(t, "循環上の op の F は Op.effects の後ろに Op.carried.effects", effectNames(l.Effects), []string{"server.e1", "server.e2"})
	wantStrings(t, "循環上の op の R は Op.reads の後ろに Op.carried.reads", l.Reads, []string{"server.sA", "server.sB"})
}

// TestSigCarriedWiring は docs/check.md「デコレータが運ぶ reads と effects」と、手順 0（配線の op）の「運ばれた効果」:
// 配線の op の F と R は、本体が行き着くものの後ろに、運ばれたもの（重複は最初の出現）。運ばれた効果のデコレータの分は、
// 葉と同じ（rel は op の throws の relations から、succ は kept）で、本体も行き着く効果は、本体から決めた状態と違えば unknown。
// 呼び出し元へ伝わり、書いた節は、運ばれたものを含む Sig と照合する（READS-2 / EFF-2）。
func TestSigCarriedWiring(t *testing.T) {
	doing := decorator("server.doing")
	doing.Reads = ir.StateReads("$param:table")
	doing.Effects = ir.NamedEffects("$param:targets")
	fail := leaf("server.fail", nil)
	fail.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	write := leaf("server.write", nil)
	write.Effects = ir.NamedEffects("server.e1")
	look := reads(leaf("server.look", nil), "server.sA")
	body := func(name string) *ir.Op {
		o := wire(name, nil, nil, cs(call("server.fail")), cs(call("server.write")), cs(call("server.look")))
		o.Decorators = []ir.Application{app("server.doing")}
		// e1 は本体も行き着く（X のとき prevented）。e2 は運ばれただけ。sA は本体も読む、sB は運ばれただけ
		o.Carried = ir.Carried{Reads: ir.StateReads("server.sB", "server.sA"), Effects: ir.NamedEffects("server.e2", "server.e1")}
		return o
	}
	// 関係を書かない: 運ばれた効果のデコレータの分は unknown
	plain := body("server.plain")
	plain.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	// prevents: 本体から決めた e1（prevented）と同じ。運ばれただけの e2 は、書いたとおり
	prevented := body("server.prevented")
	prevented.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	// keeps: 本体から決めた e1（prevented）と違うので unknown になり、REL-4
	kept := body("server.kept")
	kept.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps())}
	// 節を書く: 運ばれたものを含めば通り、落とせば READS-2 / EFF-2
	written := body("server.written")
	written.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	written.Reads = ir.StateReads("server.sA", "server.sB")
	written.Effects = ir.NamedEffects("server.e1", "server.e2")
	dropped := body("server.dropped")
	dropped.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	dropped.Reads = ir.StateReads("server.sA")
	dropped.Effects = ir.NamedEffects("server.e1")
	// 呼び出し元: 運ばれたものも伝わる。節が落とせば READS-2 / EFF-2
	caller := wire("server.caller", nil, nil, cs(call("server.prevented")))
	caller.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	callerDropped := wire("server.callerDropped", nil, nil, cs(call("server.prevented")))
	callerDropped.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	callerDropped.Reads = ir.StateReads("server.sA")
	callerDropped.Effects = ir.NamedEffects("server.e1")

	p := serverWith(doing, fail, write, look, plain, prevented, kept, written, dropped, caller, callerDropped)
	diags := Check(p, cfgFor("server"))

	for _, name := range []string{"server.plain", "server.prevented", "server.kept", "server.written", "server.dropped", "server.caller", "server.callerDropped"} {
		sig := sigOf(t, p, name)
		wantStrings(t, name+" の F（本体、運ばれたもの）", effectNames(sig.Effects), []string{"server.e1", "server.e2"})
		wantStrings(t, name+" の R（本体、運ばれたもの）", sig.Reads, []string{"server.sA", "server.sB"})
		if sig.Pure {
			t.Errorf("%s は純粋でない", name)
		}
	}
	rel := func(op string, e string, want ir.EffectState, why string) {
		t.Helper()
		if got := relOf(t, sigOf(t, p, op), "server.X", e); got != want {
			t.Errorf("%s: rel(X, %s) = %s, want %s（%s）", op, e, got, want, why)
		}
	}
	rel("server.plain", "server.e1", ir.Unknown, "本体は prevented、デコレータの分は unknown")
	rel("server.plain", "server.e2", ir.Unknown, "運ばれただけ。関係を書いていない")
	rel("server.prevented", "server.e1", ir.Prevented, "本体もデコレータの分も prevented")
	rel("server.prevented", "server.e2", ir.Prevented, "運ばれただけ。書いた prevents")
	rel("server.kept", "server.e1", ir.Unknown, "本体は prevented、デコレータの分は kept")
	rel("server.kept", "server.e2", ir.Kept, "運ばれただけ。書いた keeps")
	rel("server.caller", "server.e1", ir.Prevented, "呼び出し先の Sig のまま")
	rel("server.caller", "server.e2", ir.Prevented, "呼び出し先に運ばれた効果も、呼び出し先の Sig のまま")
	for _, name := range []string{"server.plain", "server.prevented", "server.kept"} {
		for _, e := range []string{"server.e1", "server.e2"} {
			if got := succOf(t, sigOf(t, p, name), e); got != ir.Kept {
				t.Errorf("%s: succ(%s) = %s, want kept", name, e, got)
			}
		}
	}
	// Op.reads / Op.effects / Op.carried は書き換えない
	wantStrings(t, "Op.effects", effectNames(plain.Effects), nil)
	wantStrings(t, "Op.carried.effects", effectNames(plain.Carried.Effects), []string{"server.e2", "server.e1"})

	byOp := func(code string) []string {
		var out []string
		for _, d := range withCode(diags, code) {
			out = append(out, strings.Fields(d.Message)[0]) // メッセージは op の FQN で始まる
		}
		return out
	}
	wantStrings(t, "REL-4（keeps と書いたが、本体も行き着く e1 は unknown）", byOp("REL-4"), []string{"server.kept"})
	wantStrings(t, "READS-2（運ばれた sB を落とした節）", byOp("READS-2"), []string{"server.dropped", "server.callerDropped"})
	wantStrings(t, "EFF-2（運ばれた e2 を落とした節）", byOp("EFF-2"), []string{"server.dropped", "server.callerDropped"})
	for _, d := range append(withCode(diags, "READS-2"), withCode(diags, "EFF-2")...) {
		missing, _ := d.Data["missing"].([]string)
		if len(missing) != 1 || (missing[0] != "server.sB" && missing[0] != "server.e2") {
			t.Errorf("%s の Data.missing = %v, want 運ばれたもの一つ", d.Code, d.Data["missing"])
		}
	}
	wantNoCode(t, diags, "THROWS-1")
}

// TestSigCarriedSucc は手順 0（配線の op）の「運ばれた効果」の succ: 本体も行き着く効果は、本体から決めた状態（一部のアームに
// しかなければ unknown）とデコレータの分（kept）が違えば unknown。
func TestSigCarriedSucc(t *testing.T) {
	write := leaf("server.write", nil)
	write.Effects = ir.NamedEffects("server.e1")
	k := &ir.Classify{DeclBase: ir.DeclBase{Name: "server.K", Span: sp("server.K")}, Subject: ir.TypeSubject{Type: ir.String()},
		Cases: []ir.ClassifyCase{{Name: "A"}, {Name: "B"}}, DecidedBy: ir.DecidedByPredicate}
	c := call("server.write")
	w := wire("server.w", nil, nil, matchStmt("server.K", arm(&c, "A"), arm(nil, "B")))
	w.Decorators = []ir.Application{app("server.doing")}
	w.Carried = ir.Carried{Effects: ir.NamedEffects("server.e1", "server.e2")}
	doing := decorator("server.doing")
	doing.Effects = ir.NamedEffects("$param:targets")
	p := serverWith(doing, k, write, w)
	Compute(p)
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "F", effectNames(sig.Effects), []string{"server.e1", "server.e2"})
	if got := succOf(t, sig, "server.e1"); got != ir.Unknown {
		t.Errorf("succ(e1) = %s, want unknown（本体は一部のアームだけ、デコレータの分は kept）", got)
	}
	if got := succOf(t, sig, "server.e2"); got != ir.Kept {
		t.Errorf("succ(e2) = %s, want kept（運ばれただけ）", got)
	}
}

// TestCarriedReadsOnly はデコレータが運ぶ状態だけを読む op（読む op）: R が空でなく、効果を持たない。
func TestCarriedReadsOnly(t *testing.T) {
	find := leaf("server.find", ir.Optional(ir.String()))
	find.Carried = ir.Carried{Reads: ir.StateReads("server.sA")}
	find.Decorators = []ir.Application{app("server.finder")}
	finder := decorator("server.finder")
	finder.Reads = ir.StateReads("server.sA")
	w := wire("server.w", ir.Optional(ir.String()), nil, ret(call("server.find")))
	w.Reads = ir.StateReads("server.sA")
	p := serverWith(finder, find, w)
	diags := Check(p, cfgFor("server"))
	wantStrings(t, "R(find)", sigOf(t, p, "server.find").Reads, []string{"server.sA"})
	wantStrings(t, "F(find)", effectNames(sigOf(t, p, "server.find").Effects), nil)
	wantStrings(t, "R(w)", sigOf(t, p, "server.w").Reads, []string{"server.sA"})
	wantNoCode(t, diags, "READS-2") // 配線に書いた reads は、運ばれた状態を含む計算と等しい
}

// throwing は、例外を投げる効果（Effect.throws。docs/check.md「read と効果」）。
func throwing(fqn, state string, exceptions ...string) *ir.Effect {
	e := eff(fqn, state, ir.OpPut)
	e.Throws = exceptions
	return e
}

// TestSigThrowingEffectCall は手順 0 の表の、効果の呼び出し: 宣言した効果の throws が、配線の例外に加わり、その効果との組は
// prevented（投げたとき、その効果は、起きていない）。先に動いた効果は kept、後ろの効果は prevented。配線が throws の節を
// 書いていなければ、例外は計算されるだけで、THROWS-1 は出ない。節を書いて覆わなければ THROWS-1。直に呼んだ組み込みの効果は、
// 例外を増やさない。
func TestSigThrowingEffectCall(t *testing.T) {
	t1 := throwing("server.t1", "server.sA", "server.X")
	w := wire("server.w", nil, nil, cs(effectCall("server.e1")), cs(effectCall("server.t1")), cs(effectCall("server.e2")), cs(directCall("server.sB", ir.OpPut)))
	w.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	p := serverWith(t1, w)
	Compute(p)
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "E", sig.Exceptions, []string{"server.X"})
	wantStrings(t, "F", effectNames(sig.Effects), []string{"server.e1", "server.t1", "server.e2", "server.sB.put"})
	for e, want := range map[string]ir.EffectState{"server.e1": ir.Kept, "server.t1": ir.Prevented, "server.e2": ir.Prevented} {
		if got := relOf(t, sig, "server.X", e); got != want {
			t.Errorf("rel(X, %s) = %s, want %s", e, got, want)
		}
	}
	if got := succOf(t, sig, "server.t1"); got != ir.Kept {
		t.Errorf("succ(t1) = %s, want kept", got)
	}

	// 配線が throws の節を書いていなければ、例外は計算されるだけ
	w2 := wire("server.w2", nil, nil, cs(effectCall("server.t1")))
	p2 := serverWith(t1, w2)
	wantNoCode(t, Check(p2, nil), "THROWS-1")
	wantStrings(t, "w2.E", sigOf(t, p2, "server.w2").Exceptions, []string{"server.X"})

	// 節を書いて、効果の例外を覆わなければ THROWS-1
	w3 := wire("server.w3", nil, nil, cs(effectCall("server.t1")))
	w3.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
	d := wantOneCode(t, Check(serverWith(t1, w3), nil), "THROWS-1")
	if !strings.Contains(d.Message, "server.X") {
		t.Errorf("THROWS-1 のメッセージに server.X がない: %s", d.Message)
	}
}

// TestSigCarriedThrows は「運ばれた効果が投げる例外」: 葉でも配線でも、op が throws に書かなくても、E に加わる。
// この出どころでの状態は、運ばれた効果そのものが prevented、ほかの効果は unknown。op も同じ例外を書いていれば、
// 同じならその状態、違えば unknown。
func TestSigCarriedThrows(t *testing.T) {
	t1 := throwing("server.t1", "server.sA", "server.X")

	// 葉: 書いていない例外 X が加わる
	l := leaf("server.l", nil)
	l.Carried = ir.Carried{Effects: ir.NamedEffects("server.e1", "server.t1")}
	l.Throws = []ir.ThrowsClause{throws([]string{"server.Y"}, prevents())}
	// 葉: 同じ例外 X を、prevents effects と書いている
	l2 := leaf("server.l2", nil)
	l2.Carried = ir.Carried{Effects: ir.NamedEffects("server.e1", "server.t1")}
	l2.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	// 配線: 本体の効果 e2 と、運ばれた t1
	w := wire("server.w", nil, nil, cs(effectCall("server.e2")))
	w.Carried = ir.Carried{Effects: ir.NamedEffects("server.t1")}
	p := serverWith(t1, l, l2, w)
	Compute(p)

	sig := sigOf(t, p, "server.l")
	wantStrings(t, "E(l)", sig.Exceptions, []string{"server.Y", "server.X"})
	wantStrings(t, "F(l)", effectNames(sig.Effects), []string{"server.e1", "server.t1"})
	if got := relOf(t, sig, "server.X", "server.t1"); got != ir.Prevented {
		t.Errorf("l: rel(X, t1) = %s, want prevented", got)
	}
	if got := relOf(t, sig, "server.X", "server.e1"); got != ir.Unknown {
		t.Errorf("l: rel(X, e1) = %s, want unknown", got)
	}
	if got := relOf(t, sig, "server.Y", "server.t1"); got != ir.Prevented {
		t.Errorf("l: rel(Y, t1) = %s, want prevented (prevents effects)", got)
	}

	sig = sigOf(t, p, "server.l2")
	wantStrings(t, "E(l2)", sig.Exceptions, []string{"server.X"})
	if got := relOf(t, sig, "server.X", "server.t1"); got != ir.Prevented {
		t.Errorf("l2: rel(X, t1) = %s, want prevented (同じ)", got)
	}
	if got := relOf(t, sig, "server.X", "server.e1"); got != ir.Unknown {
		t.Errorf("l2: rel(X, e1) = %s, want unknown (prevents と、運ばれた出どころの unknown が違う)", got)
	}

	sig = sigOf(t, p, "server.w")
	wantStrings(t, "E(w)", sig.Exceptions, []string{"server.X"})
	wantStrings(t, "F(w)", effectNames(sig.Effects), []string{"server.e2", "server.t1"})
	if got := relOf(t, sig, "server.X", "server.t1"); got != ir.Prevented {
		t.Errorf("w: rel(X, t1) = %s, want prevented", got)
	}
	if got := relOf(t, sig, "server.X", "server.e2"); got != ir.Unknown {
		t.Errorf("w: rel(X, e2) = %s, want unknown", got)
	}
	// 配線は、運ばれた効果の例外を、throws に書かなくてよい（デコレータの throws と同じ）
	wantNoCode(t, Check(p, nil), "THROWS-1")
}

// TestSigHaltsThrows は halts の項: halts に渡した効果 h が throws を持てば、その例外が E に加わる。h を、ここで F に
// 足したなら、h は prevented、ほかの効果は unknown。
func TestSigHaltsThrows(t *testing.T) {
	h := decorator("server.halt")
	h.Catches = []ir.CatchesClause{catches(ir.Halts, "server.t1", "server.X")}
	t1 := throwing("server.t1", "server.sA", "server.Z")
	l := leaf("server.l", nil)
	l.Effects = ir.NamedEffects("server.e1")
	l.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	w := wire("server.w", nil, nil, cs(call("server.l")))
	w.Decorators = []ir.Application{app("server.halt")}
	p := serverWith(h, t1, l, w)
	Compute(p)
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "E", sig.Exceptions, []string{"server.Z"})
	wantStrings(t, "F", effectNames(sig.Effects), []string{"server.e1", "server.t1"})
	if got := relOf(t, sig, "server.Z", "server.t1"); got != ir.Prevented {
		t.Errorf("rel(Z, t1) = %s, want prevented", got)
	}
	if got := relOf(t, sig, "server.Z", "server.e1"); got != ir.Unknown {
		t.Errorf("rel(Z, e1) = %s, want unknown", got)
	}
}
