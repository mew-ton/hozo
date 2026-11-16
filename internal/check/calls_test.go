package check

import (
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// 配線からの read と効果の呼び出し（docs/check.md「状態に触れるもの」の「配線からの呼び出し」）: 手順 0 の表の各行、
// 読む状態、純粋さ、必須の入力。

// readCall は read の宣言の呼び出し（`r(a)`）。
func readCall(read string, args ...ir.CallArg) ir.Call {
	return ir.Call{Callee: ir.ReadCallee{Read: read}, Args: args, Span: sp("call." + read)}
}

// effectCall は効果の宣言の呼び出し（`e(a);`）。
func effectCall(effect string, args ...ir.CallArg) ir.Call {
	return ir.Call{Callee: ir.EffectCallee{Effect: ir.NamedEffect{Effect: effect}}, Args: args, Span: sp("call." + effect)}
}

// directCall は組み込みの効果を直に呼んだもの（`s.post(v);`）。
func directCall(state string, op ir.Operation, args ...ir.CallArg) ir.Call {
	return ir.Call{Callee: ir.EffectCallee{Effect: ir.DirectEffect{State: state, Operation: op}}, Args: args, Span: sp("call." + state)}
}

// callsProgram は、状態と read と効果を持つ store。配線 ops を足して返す。
func callsProgram(ops ...ir.Decl) *ir.Program {
	user := ir.Ref("store.User")
	create := eff("store.createUser", "store.users", ir.OpPost)
	create.Params = []ir.OpParam{param("user", user)}
	byEmail := readDecl("store.byEmail", "store.users")
	byEmail.Params = []ir.OpParam{param("email", ir.String())}
	byEmail.Returns = ir.Optional(user)
	limit := exc("store.LimitExceeded")
	limit.Params = []ir.OpParam{param("wait", ir.Duration()), param("note", ir.Optional(ir.String()))}
	check := leaf("store.check", nil)
	check.Throws = []ir.ThrowsClause{throws([]string{"store.Plain"})}
	decls := []ir.Decl{
		model("store.User", field("email", ir.String())),
		state("store.users", ir.StateList, user),
		state("store.token", ir.StateNullable, ir.String()),
		state("store.title", ir.StateSingle, ir.String()),
		create, byEmail, limit, exc("store.Plain"),
		&ir.Classify{DeclBase: ir.DeclBase{Name: "store.K", Span: sp("store.K")}, Subject: ir.TypeSubject{Type: ir.String()},
			Cases: []ir.ClassifyCase{{Name: "A"}, {Name: "B"}}, DecidedBy: ir.DecidedByPredicate},
		check,
	}
	return program(nil, namespace("store", append(decls, ops...)...))
}

// TestSigStep0Calls は手順 0 の表: 効果の呼び出しは、例外がなく、効果がその効果一つ（宣言なら named、直に呼んだものなら
// direct）で、succ は kept。read の呼び出しは、例外も効果も持たない。throw は、引数があっても、例外が X だけ。
func TestSigStep0Calls(t *testing.T) {
	post := ir.DirectEffect{State: "store.users", Operation: ir.OpPost}
	del := ir.DirectEffect{State: "store.token", Operation: ir.OpDelete}
	w := wire("store.register", nil, nil,
		cst("found", readCall("store.byEmail", carg("email", nameAtom("e")))),
		cs(effectCall("store.createUser", carg("user", nameAtom("u")))),
		cs(call("store.check")),
		cs(directCall("store.users", ir.OpPost, carg(ir.DirectEffectParam, nameAtom("u")))),
		cs(directCall("store.token", ir.OpDelete)),
	)
	w.Throws = []ir.ThrowsClause{throws([]string{"store.Plain"})}
	p := callsProgram(w)
	wantCodes(t, Check(p, cfgFor("store")))
	sig := sigOf(t, p, "store.register")
	wantStrings(t, "E（read と効果は例外を持たない）", sig.Exceptions, []string{"store.Plain"})
	wantEffects(t, "F（行の順。宣言と、直に呼んだ効果は別）", sig.Effects, "store.createUser", post, del)
	// check が Plain を投げるとき: 上の行の createUser は kept、下の行の直に呼んだ効果は prevented
	if got := relOf(t, sig, "store.Plain", "store.createUser"); got != ir.Kept {
		t.Errorf("rel(Plain, createUser) = %s", got)
	}
	if got := relOf(t, sig, "store.Plain", post); got != ir.Prevented {
		t.Errorf("rel(Plain, users.post) = %s", got)
	}
	for _, e := range []any{"store.createUser", post, del} {
		if got := succOf(t, sig, e); got != ir.Kept {
			t.Errorf("succ(%v) = %s", e, got)
		}
	}
	wantStrings(t, "R（read の宛先の状態。効果は読まない）", sig.Reads, []string{"store.users"})
	if sig.Pure {
		t.Error("read と効果を呼ぶ配線は純粋でない")
	}
}

// TestPureWithCalls は純粋さ: 効果だけ、read だけを呼ぶ配線は純粋でない。デコレータのない葉は、いつも純粋。
func TestPureWithCalls(t *testing.T) {
	onlyRead := wire("store.find", ir.Optional(ir.Ref("store.User")), nil, ret(readCall("store.byEmail", carg("email", nameAtom("e")))))
	onlyEffect := wire("store.add", nil, nil, cs(effectCall("store.createUser", carg("user", nameAtom("u")))))
	onlyDirect := wire("store.rename", nil, nil, cs(directCall("store.title", ir.OpPut, carg(ir.DirectEffectParam, nameAtom("t")))))
	viaOp := wire("store.outer", nil, nil, cs(call("store.add")))
	p := callsProgram(onlyRead, onlyEffect, onlyDirect, viaOp, leaf("store.plain", nil))
	Compute(p)
	for name, want := range map[string]bool{"store.find": false, "store.add": false, "store.rename": false, "store.outer": false, "store.plain": true, "store.check": true} {
		if got := sigOf(t, p, name).Pure; got != want {
			t.Errorf("pure(%s) = %v, want %v", name, got, want)
		}
	}
	wantStrings(t, "R(find)", sigOf(t, p, "store.find").Reads, []string{"store.users"})
	wantStrings(t, "R(add)", sigOf(t, p, "store.add").Reads, nil)
	wantEffects(t, "F(outer) は伝播", sigOf(t, p, "store.outer").Effects, "store.createUser")
	wantEffects(t, "F(rename)", sigOf(t, p, "store.rename").Effects, ir.DirectEffect{State: "store.title", Operation: ir.OpPut})
}

// TestThrowArgsReads は、throw の引数に書いた状態の名前が、読む状態に入ること（計算「reads」の 2.1）。
func TestThrowArgsReads(t *testing.T) {
	th := throwArm("store.LimitExceeded", "A")
	body := th.Body.(ir.ThrowBody)
	body.Args = []ir.CallArg{carg("wait", nameAtom("w")), carg("note", stateAtom("store.token"))}
	th.Body = body
	w := wire("store.guard", nil, nil, matchStmt("store.K", th, arm(nil)))
	w.Throws = []ir.ThrowsClause{throws([]string{"store.LimitExceeded"})}
	p := callsProgram(w)
	Compute(p)
	sig := sigOf(t, p, "store.guard")
	wantStrings(t, "E", sig.Exceptions, []string{"store.LimitExceeded"})
	wantStrings(t, "R", sig.Reads, []string{"store.token"})
}

// TestBind4Callees は BIND-4 が、read、効果（宣言と、直に呼ぶ組み込みの効果）、アームの throw にも掛かること
// （docs/check.md「必須の入力」）。bind は、op でない呼び出し先の引数を埋めない。
func TestBind4Callees(t *testing.T) {
	missing := throwArm("store.LimitExceeded", "A")
	given := throwArm("store.LimitExceeded", "B")
	gb := given.Body.(ir.ThrowBody)
	gb.Args = []ir.CallArg{carg("wait", nameAtom("w"))} // note は optional
	given.Body = gb
	w := wire("store.w", nil, nil,
		cst("a", readCall("store.byEmail")),                                                 // email がない
		cst("b", readCall("store.byEmail", carg("email", nameAtom("e")))),                   // ok
		cs(effectCall("store.createUser")),                                                  // user がない
		cs(effectCall("store.createUser", carg("user", rec()))),                             // user.email がない
		cs(directCall("store.users", ir.OpPost)),                                            // value がない
		cs(directCall("store.token", ir.OpPost, carg(ir.DirectEffectParam, nameAtom("t")))), // ok
		cs(directCall("store.token", ir.OpPut)),                                             // value がない
		cs(directCall("store.token", ir.OpDelete)),                                          // ok: 引数なし
		cs(directCall("store.title", ir.OpPut)),                                             // value がない
		matchStmt("store.K", missing, given, throwArm("store.Plain")),                       // wait がない / ok / ok
	)
	w.Throws = []ir.ThrowsClause{throws([]string{"store.LimitExceeded", "store.Plain"})}
	// bind token は string の位置に当たるが、read の引数は埋めない
	p := callsProgram(w)
	p.Binds = []ir.Bind{bind("store", "store.token", "store", false)}
	b4 := withCode(Check(p, cfgFor("store")), "BIND-4")
	want := []string{"email", "user", "user.email", "value", "value", "value", "wait"}
	if len(b4) != len(want) {
		t.Fatalf("BIND-4 count = %d, want %d\n%s", len(b4), len(want), messages(b4))
	}
	for i, d := range b4 {
		paths := d.Data["missing"].([][]string)
		var got []string
		for _, path := range paths {
			got = append(got, joinPath(path))
		}
		wantStrings(t, sprintf("BIND-4 #%d missing", i), got, []string{want[i]})
	}
	// op でない呼び出し先の Call.bound は、いつも空
	for _, st := range p.Lookup("store.w").(*ir.Op).Body.Statements {
		if c, ok := st.(ir.ConstStmt); ok {
			if b := c.Expr.(ir.CallExpr).Call.Bound; len(b) != 0 {
				t.Errorf("read の呼び出しの bound = %v", b)
			}
		}
	}
}

// wantEffects は効果の列の照合。文字列は宣言の FQN（名前で書いた効果）、EffectRef はそのまま。
func wantEffects(t *testing.T, what string, got []ir.EffectRef, want ...any) {
	t.Helper()
	ok := len(got) == len(want)
	for i := 0; ok && i < len(got); i++ {
		ok = got[i] == effectOf(want[i])
	}
	if !ok {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}
