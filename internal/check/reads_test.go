package check

import (
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// 読む状態（R）、純粋さ、アームの throw、「返らない」アーム、export と bind
// （docs/check.md「計算」の「reads」「純粋」「手順 0」「bind」、「op の宣言と本体」）。

// storeProgram は、状態を持つ store と、それを呼ぶ server。store の op は export。
func storeProgram(serverOps ...ir.Decl) *ir.Program {
	user := ir.Ref("type.User")
	store := []ir.Decl{
		state("store.users", ir.StateList, user),
		state("store.sessions", ir.StateList, ir.Ref("type.Session")),
		state("store.token", ir.StateNullable, ir.String()),
		eff("store.createUser", "store.users", ir.OpPost),
		reads(leaf("store.findUser", ir.Optional(user), param("email", ir.String())), "store.users"),
		reads(leaf("store.findSession", ir.Optional(ir.Ref("type.Session"))), "store.sessions", "store.users", "store.sessions"),
		&ir.Classify{DeclBase: ir.DeclBase{Name: "store.Login", Span: sp("store.Login")},
			Subject: ir.StatesSubject{States: []string{"store.token", "store.sessions"}}, Cases: []ir.ClassifyCase{{Name: "In"}, {Name: "Out"}}},
	}
	add := leaf("store.addUser", nil, param("user", user))
	add.Effects = ir.NamedEffects("store.createUser")
	store = append(store, add)
	return program(nil,
		namespace("type", model("type.User", field("id", ir.String())), model("type.Session", field("token", ir.String())), exc("type.Unauthorized")),
		namespace("store", store...),
		namespace("server", serverOps...),
	)
}

// TestReadsLeaf は葉の op の R = Op.reads（書かれた順。重複は最初の出現に寄せる）。
func TestReadsLeaf(t *testing.T) {
	p := storeProgram()
	Compute(p)
	wantStrings(t, "R(findUser)", sigOf(t, p, "store.findUser").Reads, []string{"store.users"})
	wantStrings(t, "R(findSession)", sigOf(t, p, "store.findSession").Reads, []string{"store.sessions", "store.users"})
	wantStrings(t, "R(addUser)", sigOf(t, p, "store.addUser").Reads, nil)
}

// TestReadsWiring は配線の op の R: 行の順に、match の対象が読む状態、Atom が参照する状態、Call.bound の状態、
// 呼び出し先の R を出現順に集め、重複を最初の出現に寄せる。R は名前空間をまたいで伝わる。
func TestReadsWiring(t *testing.T) {
	user := ir.Ref("type.User")
	// 名前空間をまたいで伝わる
	across := wire("server.login", nil, nil,
		cst("s", call("store.findSession")),
		cst("u", call("store.findUser", carg("email", nameAtom("e")))),
	)
	p := storeProgram(across)
	Compute(p)
	wantStrings(t, "呼び出し先の R（行の順、重複は最初の出現）", sigOf(t, p, "server.login").Reads, []string{"store.sessions", "store.users"})

	// 一つの行の中の順: 1 match の対象、2.1 Atom、2.2 bound、2.3 呼び出し先の R
	take := leaf("store.take", nil, param("in", ir.Ref("store.In")), param("t", ir.Optional(ir.String())))
	inModel := model("store.In", field("who", ir.Optional(user)), field("n", ir.Array(ir.Ref("type.Session"))))
	byState := wire("store.w", nil, nil,
		// match(token by nullable){ Some => take({ n: sessions }, token); None => findUser(...) }
		matchState("store.token", ir.StateNullable,
			arm(ptr(call("store.take", carg("in", rec(rf("n", stateAtom("store.sessions")))), carg("t", stateAtom("store.token")))), "Some"),
			arm(ptr(call("store.findUser")), "None"),
		),
	)
	// match(by Login){ In => users; default => none }: C の対象の状態を宣言の順に、その後にアームの値
	byClassify := wire("store.c", ir.Optional(ir.Array(user)), nil,
		ir.ReturnStmt{Expr: ir.MatchExpr{Match: ir.Match{Subject: ir.ClassifyOwnedSubject{}, Classify: "store.Login", Arms: []ir.MatchArm{
			atomArm(stateAtom("store.users"), "In"),
			arm(nil),
		}}}},
	)
	// return の値、パスを持つ状態の参照
	direct := wire("store.d", ir.String(), nil, retAtom(stateAtom("store.token", "x")))
	p = storeProgram()
	p.Namespaces[1].Decls = append(p.Namespaces[1].Decls, inModel, take, byState, byClassify, direct)
	Compute(p)
	wantStrings(t, "match の対象 → アームの Atom → 呼び出し先の R", sigOf(t, p, "store.w").Reads, []string{"store.token", "store.sessions", "store.users"})
	wantStrings(t, "match by C は対象の状態を宣言の順に", sigOf(t, p, "store.c").Reads, []string{"store.token", "store.sessions", "store.users"})
	wantStrings(t, "return の値", sigOf(t, p, "store.d").Reads, []string{"store.token"})

	// bind で埋まる位置の状態（2.2）
	withBind := wire("store.b", nil, nil, cs(call("store.take", carg("in", rec()))))
	p = storeProgram()
	p.Namespaces[1].Decls = append(p.Namespaces[1].Decls, inModel, take, withBind)
	p.Binds = []ir.Bind{bind("store", "store.token", "store", false)}
	Compute(p)
	wantStrings(t, "Call.bound の状態", sigOf(t, p, "store.b").Reads, []string{"store.token"})
}

func ptr(c ir.Call) *ir.Call { return &c }

// TestReads2 は READS-2: 配線の op の Op.reads が空でなく、集合として R と等しくない。書かなければ出ない。
// 配線の reads には別の名前空間の状態も書ける。
func TestReads2(t *testing.T) {
	body := func() []ir.Statement {
		return []ir.Statement{cst("u", call("store.findUser", carg("email", nameAtom("e"))))}
	}
	omitted := wire("server.omitted", nil, nil, body()...)
	equal := reads(wire("server.equal", nil, nil, body()...), "store.users")
	wrong := reads(wire("server.wrong", nil, nil, body()...), "store.sessions")
	extra := reads(wire("server.extra", nil, nil, body()...), "store.users", "store.sessions")
	// 葉の op は READS-2 の対象外（Op.reads がそのまま R）
	diags := Checker{Locate: locatorFor()}.Check(storeProgram(omitted, equal, wrong, extra), cfgFor("server"))
	var got []string
	for _, d := range diags {
		if d.Code != "READS-2" {
			continue
		}
		got = append(got, d.Message)
		switch {
		case len(got) == 1:
			wantStrings(t, "missing", d.Data["missing"].([]string), []string{"store.users"})
			wantStrings(t, "extra", d.Data["extra"].([]string), []string{"store.sessions"})
		case len(got) == 2:
			wantStrings(t, "missing", d.Data["missing"].([]string), []string{})
			wantStrings(t, "extra", d.Data["extra"].([]string), []string{"store.sessions"})
		}
	}
	if len(got) != 2 {
		t.Fatalf("READS-2 = %d, want 2（wrong と extra）: %v", len(got), got)
	}
}

// TestPure は純粋さ（docs/check.md「純粋」）の四つの条件。
func TestPure(t *testing.T) {
	mark := decorator("server.mark")
	plain := leaf("server.plain", ir.String())
	marked := leaf("server.marked", ir.String())
	marked.Decorators = []ir.Application{app("server.mark")}
	thrower := leaf("server.thrower", nil)
	thrower.Throws = []ir.ThrowsClause{throws([]string{"type.Unauthorized"})}

	callsPure := wire("server.callsPure", nil, nil, cst("a", call("server.plain")), cs(call("server.thrower")))
	callsPure.Throws = thrower.Throws
	callsMarked := wire("server.callsMarked", nil, nil, cst("a", call("server.marked")))
	callsReader := wire("server.callsReader", nil, nil, cst("u", call("store.findUser")))
	callsEffect := wire("server.callsEffect", nil, nil, cs(call("store.addUser", carg("user", nameAtom("u")))))
	deep := wire("server.deep", nil, nil, cs(call("server.callsMarked")))
	throwing := wire("server.throwing", ir.String(), nil, ir.ReturnStmt{Expr: ir.MatchExpr{Match: ir.Match{
		Subject: ir.ValueSubject{Atom: nameAtom("x")}, Classify: "list",
		Arms: []ir.MatchArm{arm(ptr(call("server.plain")), "HasItems"), throwArm("type.Unauthorized", "Empty")},
	}}})
	throwing.Throws = thrower.Throws

	p := storeProgram(mark, plain, marked, thrower, callsPure, callsMarked, callsReader, callsEffect, deep, throwing)
	Compute(p)
	want := map[string]bool{
		"server.plain":       true,  // デコレータなし、R 空、F 空
		"server.thrower":     true,  // throws は純粋さを変えない
		"server.marked":      false, // 1. デコレータが付いている
		"store.findUser":     false, // 2. 状態を読む
		"store.addUser":      false, // 3. 効果を持つ
		"server.callsPure":   true,  // 4. 呼び出し先がすべて純粋
		"server.callsMarked": false, // 4. 呼び出し先が純粋でない
		"server.deep":        false, // 4. 呼び出し先の呼び出し先
		"server.callsReader": false, // 2. 状態を読む op を呼ぶ（R が伝わる）
		"server.callsEffect": false, // 3. 効果が伝わる
		"server.throwing":    true,  // アームの throw と match は純粋さを変えない
	}
	for name, w := range want {
		if got := sigOf(t, p, name).Pure; got != w {
			t.Errorf("pure(%s) = %v, want %v", name, got, w)
		}
	}
	// 状態を名前で参照する配線、bind で埋まる位置を持つ配線は純粋でない（R に入る）
	ref := wire("store.ref", ir.Optional(ir.String()), nil, retAtom(stateAtom("store.token")))
	p = storeProgram()
	p.Namespaces[1].Decls = append(p.Namespaces[1].Decls, ref)
	Compute(p)
	if sigOf(t, p, "store.ref").Pure {
		t.Error("状態を名前で参照する配線は純粋でない")
	}
	// 見る側によらない: View.ops の reads と pure は Program の signature と同じ
	p = storeProgram(plain, callsReader)
	views := viewsOf(p, cfgFor("server", "store"))
	for _, viewer := range []string{"server", "store"} {
		view := views[viewer]
		if view == nil {
			t.Fatalf("viewer %s の View がない", viewer)
		}
		if !view.Ops["server.plain"].Pure || view.Ops["server.callsReader"].Pure {
			t.Errorf("viewer %s: pure が見る側で変わっている", viewer)
		}
		wantStrings(t, "viewer "+viewer+" の reads", view.Ops["server.callsReader"].Reads, []string{"store.users"})
	}
}

// TestThrowArm は手順 0 の「アームの本体の throw X は、例外が X だけで効果を持たない呼び出しとして数える」と、
// THROWS-1、succ の「返らないアームは数えない」。
func TestThrowArm(t *testing.T) {
	issue := leaf("server.issue", ir.String())
	issue.Effects = ir.NamedEffects("server.e1")
	issue.Throws = []ir.ThrowsClause{throws([]string{"server.Y"}, prevents())}
	never := leaf("server.unauthorized", ir.Never())
	never.Throws = []ir.ThrowsClause{throws([]string{"server.Z"})}
	match := func(arms ...ir.MatchArm) ir.Statement {
		return ir.ReturnStmt{Expr: ir.MatchExpr{Match: ir.Match{Subject: ir.ValueSubject{Atom: nameAtom("x")}, Classify: "server.C", Arms: arms}}}
	}

	t.Run("E と rel と succ", func(t *testing.T) {
		w := wire("server.w", ir.String(), nil, match(arm(ptr(call("server.issue")), "A"), throwArm("server.X"), arm(ptr(call("server.unauthorized")), "B")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y", "server.Z"})}
		p := serverWith(issue, never, w)
		wantNoCode(t, Check(p, cfgFor("server")), "THROWS-1")
		sig := sigOf(t, p, "server.w")
		wantStrings(t, "E", sig.Exceptions, []string{"server.Y", "server.X", "server.Z"})
		wantStrings(t, "F", effectNames(sig.Effects), []string{"server.e1"})
		// throw の出どころでは、e1 は同じ行の別のアームにあるだけ → prevented
		if got := relOf(t, sig, "server.X", "server.e1"); got != ir.Prevented {
			t.Errorf("rel(X, e1) = %s, want prevented", got)
		}
		if got := relOf(t, sig, "server.Y", "server.e1"); got != ir.Prevented {
			t.Errorf("rel(Y, e1) = %s, want prevented（issue の relations）", got)
		}
		// 返らないアーム（throw、never を返す op の呼び出し）は数えない: e1 はすべてのアームにあるものとして扱う
		if got := succOf(t, sig, "server.e1"); got != ir.Kept {
			t.Errorf("succ(e1) = %s, want kept", got)
		}
	})
	t.Run("値のアームは成功する: 一部のアームにしかない効果は unknown", func(t *testing.T) {
		w := wire("server.w", ir.String(), nil, match(arm(ptr(call("server.issue")), "A"), atomArm(nameAtom("x"), "B"), throwArm("server.X")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.X", "server.Y"})}
		p := serverWith(issue, w)
		Compute(p)
		if got := succOf(t, sigOf(t, p, "server.w"), "server.e1"); got != ir.Unknown {
			t.Errorf("succ(e1) = %s, want unknown", got)
		}
	})
	t.Run("THROWS-1: throw の例外も throws に書く", func(t *testing.T) {
		w := wire("server.w", ir.String(), nil, match(arm(ptr(call("server.issue")), "A"), throwArm("server.X")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
		d := wantOneCode(t, Checker{Locate: locatorFor()}.Check(serverWith(issue, w), cfgFor("server")), "THROWS-1")
		wantStrings(t, "missing", d.Data["missing"].([]string), []string{"server.X"})
		if len(d.Related) != 1 {
			t.Errorf("related = %d, want 1（その throw）", len(d.Related))
		}
	})
	t.Run("デコレータが吸収する", func(t *testing.T) {
		catchX := decorator("server.catchX")
		catchX.Catches = []ir.CatchesClause{catches(ir.Recovers, "", "server.X")}
		w := wire("server.w", ir.String(), nil, match(arm(ptr(call("server.issue")), "A"), throwArm("server.X")))
		w.Throws = []ir.ThrowsClause{throws([]string{"server.Y"})}
		w.Decorators = []ir.Application{app("server.catchX")}
		p := serverWith(issue, catchX, w)
		diags := Check(p, cfgFor("server"))
		wantNoCode(t, diags, "THROWS-1")
		wantNoCode(t, diags, "DEC-16")
		wantStrings(t, "E", sigOf(t, p, "server.w").Exceptions, []string{"server.Y"})
	})
}

// TestBindExport は docs/check.md「bind」の 1: 別の名前空間の、export のない op には位置を作らない
// （呼べない op のために BIND-1 / BIND-2 を出さない）。同じ名前空間の op には export がなくても作る。
func TestBindExport(t *testing.T) {
	token := ir.Ref("browser.Token")
	exported := leaf("server.exported", nil, param("token", ir.Optional(token)))
	hidden := internal(leaf("server.hidden", nil, param("a", token), param("b", token))) // 位置が二つ（export なら BIND-1）
	own := internal(leaf("browser.own", nil, param("token", ir.Optional(token))))
	p := program([]ir.Bind{bind("browser", "browser.token", "server", false), bind("browser", "browser.token", "browser", false)},
		namespace("server", exported, hidden),
		namespace("browser", scalar("browser.Token", ir.String()), state("browser.token", ir.StateNullable, token), own),
	)
	diags := Check(p, cfgFor("server", "browser"))
	wantNoCode(t, diags, "BIND-1")
	wantNoCode(t, diags, "BIND-2")
	wantPaths(t, "export の付いた op", positionsOf(t, p, 0, "server.exported"), "token")
	wantPaths(t, "別の名前空間の内部の op", positionsOf(t, p, 0, "server.hidden"))
	wantPaths(t, "同じ名前空間の内部の op", positionsOf(t, p, 1, "browser.own"), "token")

	// export を付ければ BIND-1 が出る
	hidden.Exported = true
	wantOneCode(t, Check(p, cfgFor("server", "browser")), "BIND-1")
}

// TestCallBoundStateAtom は、引数に状態を名前で渡した位置は明示した位置で、bind は埋めないこと（docs/check.md「状態の参照」）。
func TestCallBoundStateAtom(t *testing.T) {
	token := ir.Ref("browser.Token")
	take := leaf("browser.take", nil, param("token", ir.Optional(token)))
	implicit := wire("browser.implicit", nil, nil, cs(call("browser.take")))
	explicit := wire("browser.explicit", nil, nil, cs(call("browser.take", carg("token", stateAtom("browser.token")))))
	p := program([]ir.Bind{bind("browser", "browser.token", "browser", false)},
		namespace("browser", scalar("browser.Token", ir.String()), state("browser.token", ir.StateNullable, token), take, implicit, explicit),
	)
	Compute(p)
	if b := p.Lookup("browser.implicit").(*ir.Op).Body.Statements[0].(ir.CallStmt).Call.Bound; len(b) != 1 {
		t.Errorf("書かなければ bind が埋める: %+v", b)
	}
	if b := p.Lookup("browser.explicit").(*ir.Op).Body.Statements[0].(ir.CallStmt).Call.Bound; len(b) != 0 {
		t.Errorf("状態を名前で渡した位置は明示: %+v", b)
	}
	// どちらも token を読む
	wantStrings(t, "R(implicit)", sigOf(t, p, "browser.implicit").Reads, []string{"browser.token"})
	wantStrings(t, "R(explicit)", sigOf(t, p, "browser.explicit").Reads, []string{"browser.token"})
}
