package check

import (
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
func TestSigLeaf(t *testing.T) {
	o := leaf("server.op", nil)
	o.Effects = []string{"server.e1", "server.e2", "server.e3"}
	o.Throws = []ir.ThrowsClause{
		throws([]string{"server.X"}, prevents()),
		throws([]string{"server.Y"}, keeps("server.e1")),
		throws([]string{"server.Z"}, prevents("server.e1"), keeps("server.e1")),
		throws([]string{"server.W"}),
	}
	p := serverWith(o)
	Compute(p)
	sig := sigOf(t, p, "server.op")
	wantStrings(t, "E", sig.Exceptions, []string{"server.X", "server.Y", "server.Z", "server.W"})
	wantStrings(t, "F", sig.Effects, []string{"server.e1", "server.e2", "server.e3"})
	if len(sig.Relations) != 12 {
		t.Errorf("relations = %d, want 12 (E × F)", len(sig.Relations))
	}
	for _, e := range sig.Effects {
		if got := relOf(t, sig, "server.X", e); got != ir.Prevented {
			t.Errorf("rel(X, %s) = %s, want prevented (prevents all)", e, got)
		}
		if got := relOf(t, sig, "server.W", e); got != ir.Unknown {
			t.Errorf("rel(W, %s) = %s, want unknown (no relation)", e, got)
		}
		if got := succOf(t, sig, e); got != ir.Kept {
			t.Errorf("succ(%s) = %s, want kept", e, got)
		}
	}
	if got := relOf(t, sig, "server.Y", "server.e1"); got != ir.Kept {
		t.Errorf("rel(Y, e1) = %s, want kept (keeps by name)", got)
	}
	if got := relOf(t, sig, "server.Y", "server.e2"); got != ir.Unknown {
		t.Errorf("rel(Y, e2) = %s, want unknown (not named)", got)
	}
	if got := relOf(t, sig, "server.Z", "server.e1"); got != ir.Unknown {
		t.Errorf("rel(Z, e1) = %s, want unknown (both prevents and keeps; REL-5)", got)
	}
	// 配線でない op の Call.Bound 埋めは何もしない。Signature の順は E の順、F の順。
	if sig.Relations[0] != (ir.SignatureRelation{Exception: "server.X", Effect: "server.e1", State: ir.Prevented}) {
		t.Errorf("relations[0] = %+v", sig.Relations[0])
	}
}

// TestSigWiringRows は手順 0（配線の op）: F の連結と重複の寄せ、E の和と Declared、出どころの状態 1〜3、
// 出どころのない例外は Op.throws から。
func TestSigWiringRows(t *testing.T) {
	a := leaf("server.a", ir.String())
	a.Effects = []string{"server.e1", "server.e2"}
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	b := leaf("server.b", ir.String())
	b.Effects = []string{"server.e2", "server.e3"}
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
	wantStrings(t, "F", sig.Effects, []string{"server.e1", "server.e2", "server.e3"})
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
	a.Effects = []string{"server.e1"}
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, prevents())}
	b := leaf("server.b", nil)
	b.Effects = []string{"server.e2"}
	b.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	c := leaf("server.c", nil)
	c.Effects = []string{"server.e3"}
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
	a.Effects = []string{"server.e1"}
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	b := leaf("server.b", ir.String())
	b.Effects = []string{"server.e1", "server.e2"}
	d := leaf("server.d", nil)
	d.Effects = []string{"server.e3"}
	ca, cb := call("server.a"), call("server.b")
	cls := &ir.Classify{DeclBase: ir.DeclBase{Name: "server.C", Span: sp("server.C")}, Subject: ir.TypeSubject{Type: ir.String()},
		Cases: []ir.ClassifyCase{{Name: "A"}, {Name: "B"}}, HasDefault: true}
	w := wire("server.w", nil, []ir.OpParam{param("x", ir.String())},
		matchConst("r", "server.C", arm(&ca, "A"), arm(&cb, "B"), arm(nil)),
		cs(call("server.d")),
	)
	w.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	p := serverWith(cls, a, b, d, w)
	Compute(p)
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "F", sig.Effects, []string{"server.e1", "server.e2", "server.e3"})
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
	a.Effects = []string{"server.e1"}
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	b := leaf("server.b", nil)
	b.Effects = []string{"server.e1", "server.move"}
	d := leaf("server.d", nil)
	d.Effects = []string{"server.drain"}
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
			wantStrings(t, "F", sig.Effects, []string{"server.e1", "server.move", "server.drain"})
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
	q.Effects = []string{"server.h"}
	w1 := wire("server.w1", nil, nil, cs(call("server.p")))
	w2 := wire("server.w2", nil, nil, cs(call("server.p")), cs(call("server.q")))
	p := serverWith(stop, pp, q, w1, w2)
	Compute(p)
	ps := sigOf(t, p, "server.p")
	wantStrings(t, "p.E", ps.Exceptions, nil)
	wantStrings(t, "p.F", ps.Effects, []string{"server.h"})
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
	l := leaf("server.l", ir.String())
	l.Effects = []string{"server.e1", "server.e2"}
	l.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps("server.e1")), throws([]string{"server.Y"})}
	w := wire("server.w", ir.String(), nil, ret(call("server.l")))
	w.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
	w.Decorators = []ir.Application{app("server.recover")}
	p := serverWith(r, l, w)
	diags := Check(p, cfgFor("server"))
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "E", sig.Exceptions, []string{"server.Y"})
	if got := succOf(t, sig, "server.e1"); got != ir.Kept {
		t.Errorf("succ(e1) = %s, want kept (X keeps e1)", got)
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
	p2 := serverWith(r, l, w)
	wantNoCode(t, Check(p2, cfgFor("server")), "DEC-5")
	// F が空なら起きない
	l.Effects = nil
	p3 := serverWith(r, l, w)
	wantNoCode(t, Check(p3, cfgFor("server")), "DEC-5")
}

// TestSigCatchesIgnores は ignores: recovers と同じく succ は unknown になるが DEC-5 は出ない。値を返す op なら DEC-4。
func TestSigCatchesIgnores(t *testing.T) {
	ig := decorator("server.ignore")
	ig.Catches = []ir.CatchesClause{catches(ir.Ignores, "", "server.X")}
	l := leaf("server.l", nil)
	l.Effects = []string{"server.e1"}
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
	l.Effects = []string{"server.e1"}
	l.Throws = []ir.ThrowsClause{throws([]string{"server.X"}), throws([]string{"server.Y"}, prevents())}
	w := wire("server.w", nil, nil, cs(call("server.l")))
	w.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
	w.Decorators = []ir.Application{app("server.halt")}
	p := serverWith(h, l, w)
	Compute(p)
	sig := sigOf(t, p, "server.w")
	wantStrings(t, "E", sig.Exceptions, []string{"server.Y"})
	wantStrings(t, "F", sig.Effects, []string{"server.e1", "server.h"})
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
	l.Effects = []string{"server.h", "server.e1"}
	p2 := serverWith(h, l, w)
	Compute(p2)
	sig2 := sigOf(t, p2, "server.w")
	wantStrings(t, "F", sig2.Effects, []string{"server.h", "server.e1"})
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
	tk.Throws = []ir.ThrowsClause{throws([]string{"server.W"}, keeps("server.e1"))}
	ty := decorator("server.ty")
	ty.Throws = []ir.ThrowsClause{throws([]string{"server.Y"}, prevents())}
	tx := decorator("server.tx")
	tx.Throws = []ir.ThrowsClause{throws([]string{"server.X"}, keeps())}
	l := leaf("server.l", nil)
	l.Effects = []string{"server.e1", "server.e2"}
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
	if got := relOf(t, sig, "server.W", "server.e1"); got != ir.Kept {
		t.Errorf("rel(W, e1) = %s, want kept (named)", got)
	}
	if got := relOf(t, sig, "server.W", "server.e2"); got != ir.Unknown {
		t.Errorf("rel(W, e2) = %s, want unknown (not named)", got)
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
	d := wantOneCode(t, diags, "THROWS-1")
	if d.Message == "" || d.Data["missing"] == nil {
		t.Errorf("THROWS-1 without message or data: %+v", d)
	}
	wantStrings(t, "Data.missing", d.Data["missing"].([]string), []string{"http.ServerError", "http.NetworkError"})
	views := Views(p, cfgFor("server", "browser"))
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
	wantStrings(t, "F", sig.Effects, []string{"browser.go"})
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
	a.Effects = []string{"server.e1"}
	a.Throws = []ir.ThrowsClause{throws([]string{"server.X"})}
	b := wire("server.b", nil, nil, cs(call("server.c")))
	c := wire("server.c", nil, nil, cs(call("server.a")))
	self := wire("server.self", nil, nil, cs(call("server.self")))
	d := wire("server.d", nil, nil, cs(call("server.a")))
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
	wantStrings(t, "a.F (as leaf)", sa.Effects, []string{"server.e1"})
	wantStrings(t, "b.E", sigOf(t, p, "server.b").Exceptions, nil)
	wantStrings(t, "d.E", sigOf(t, p, "server.d").Exceptions, []string{"server.X"})
	wantNoCode(t, diags, "EFF-2") // 循環上の配線の effects は自分の宣言と一致する
	if n := len(withCode(diags, "THROWS-1")); n != 1 {
		t.Errorf("THROWS-1 count = %d, want 1 (d calls a)", n)
	}
}

// TestComputeIdempotent は Compute を二度呼んでも同じ結果になること（Check は Compute を含む）。
func TestComputeIdempotent(t *testing.T) {
	a := leaf("server.a", nil)
	a.Effects = []string{"server.e1"}
	w := wire("server.w", nil, nil, cs(call("server.a")))
	p := serverWith(a, w)
	Compute(p)
	first := sigOf(t, p, "server.w")
	Compute(p)
	Check(p, cfgFor("server"))
	second := sigOf(t, p, "server.w")
	wantStrings(t, "F", second.Effects, first.Effects)
	if len(second.Relations) != len(first.Relations) || len(second.SuccessEffects) != len(first.SuccessEffects) {
		t.Errorf("signature changed on recompute")
	}
}
