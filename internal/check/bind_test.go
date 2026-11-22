package check

import (
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/ir"
)

// bindFixture は §5.3 の形: client.token（nullable<server.Token>。docs/decisions.md「設計書から離れた決定」S2）を
// server.* に bind する。
func bindFixture(serverOps []ir.Decl, browserOps []ir.Decl, binds ...ir.Bind) *ir.Program {
	server := []ir.Decl{
		scalar("server.Token", ir.String()),
		model("server.Body", field("email", ir.String())),
		model("server.LoginInput", field("token", ir.Optional(ir.Ref("server.Token"))), field("body", ir.Ref("server.Body"))),
		model("server.StrictInput", field("token", ir.Ref("server.Token"))),
		model("server.Node", field("next", ir.Optional(ir.Ref("server.Node"))), field("token", ir.Optional(ir.Ref("server.Token")))),
		// インデックスの model: 値の型が τ でも、その先は辿らない
		&ir.Model{DeclBase: ir.DeclBase{Name: "server.Tokens"}, Fields: []ir.Field{}, Index: &ir.ModelIndex{Name: "name", Key: ir.String(), Value: ir.Ref("server.Token")}},
	}
	client := []ir.Decl{
		state("client.token", ir.StateNullable, ir.Ref("server.Token")),
		state("client.token2", ir.StateNullable, ir.Ref("server.Token")),
		state("client.tokens", ir.StateList, ir.Ref("server.Token")),
		state("client.session", ir.StateSingle, ir.Ref("server.Token")),
	}
	return program(binds,
		namespace("server", append(server, serverOps...)...),
		namespace("server.admin", leaf("server.admin.x", nil, param("t", ir.Optional(ir.Ref("server.Token"))))),
		namespace("client", append(client, browserOps...)...))
}

func positionsOf(t *testing.T, p *ir.Program, bi int, op string) [][]string {
	t.Helper()
	ix := newIndex(p)
	tb := newBindTable(ix)
	var out [][]string
	for _, pos := range tb.positions(bi, ix.op(op)) {
		out = append(out, pos.path)
	}
	return out
}

func wantPaths(t *testing.T, what string, got [][]string, want ...string) {
	t.Helper()
	var gs []string
	for _, g := range got {
		gs = append(gs, joinPath(g))
	}
	wantStrings(t, what, gs, want)
}

// TestBindPositions は Pos(b, O): 0 / 1 / 2 の位置、model の再帰、array とインデックスの model は辿らない、同じ model に戻ったら止める、
// `*` は直下だけ、`**` は子孫も。
func TestBindPositions(t *testing.T) {
	ops := []ir.Decl{
		leaf("server.none", nil, param("n", ir.Int())),
		leaf("server.login", nil, param("in", ir.Ref("server.LoginInput"))),
		leaf("server.two", nil, param("a", ir.Ref("server.Token")), param("b", ir.Optional(ir.Ref("server.Token")))),
		leaf("server.list", nil, param("l", ir.Array(ir.Ref("server.Token"))), param("r", ir.Ref("server.Tokens"))),
		leaf("server.node", nil, param("n", ir.Ref("server.Node"))),
		leaf("server.opt", nil, param("in", ir.Optional(ir.Ref("server.LoginInput")))),
	}
	p := bindFixture(ops, nil, bind("client", "client.token", "server", false), bind("client", "client.token", "server", true))
	wantPaths(t, "none", positionsOf(t, p, 0, "server.none"))
	wantPaths(t, "login", positionsOf(t, p, 0, "server.login"), "in.token")
	wantPaths(t, "two", positionsOf(t, p, 0, "server.two"), "a", "b")
	wantPaths(t, "list/index model", positionsOf(t, p, 0, "server.list"))
	wantPaths(t, "node (cycle cut)", positionsOf(t, p, 0, "server.node"), "n.token")
	wantPaths(t, "optional model", positionsOf(t, p, 0, "server.opt"), "in.token")
	wantPaths(t, "server.* does not reach server.admin", positionsOf(t, p, 0, "server.admin.x"))
	wantPaths(t, "server.** reaches server.admin", positionsOf(t, p, 1, "server.admin.x"), "t")
}

// TestBindShapes は docs/check.md「bind」2〜3: bind できるのは single と nullable で、どちらも τ は状態の element。
// 位置は型が τ か optional(τ) のところ。list は位置を持たない（BIND-6）。
func TestBindShapes(t *testing.T) {
	ops := []ir.Decl{
		leaf("server.login", nil, param("in", ir.Ref("server.LoginInput"))),
		leaf("server.strict", nil, param("in", ir.Ref("server.StrictInput"))),
		leaf("server.two", nil, param("a", ir.Ref("server.Token")), param("b", ir.Optional(ir.Ref("server.Token")))),
	}
	p := bindFixture(ops, nil,
		bind("client", "client.token", "server", false),   // nullable
		bind("client", "client.session", "server", false), // single
		bind("client", "client.tokens", "server", false),  // list
	)
	for bi, shape := range []string{"nullable", "single"} {
		wantPaths(t, shape+": optional(τ) の位置", positionsOf(t, p, bi, "server.login"), "in.token")
		wantPaths(t, shape+": τ の位置", positionsOf(t, p, bi, "server.strict"), "in.token")
		wantPaths(t, shape+": τ と optional(τ)", positionsOf(t, p, bi, "server.two"), "a", "b")
	}
	for _, op := range []string{"server.login", "server.strict", "server.two"} {
		wantPaths(t, "list: "+op, positionsOf(t, p, 2, op))
	}
}

// TestBindRules は BIND-1 / BIND-2 / BIND-5 / BIND-6 と、それぞれの通る例。
func TestBindRules(t *testing.T) {
	ops := []ir.Decl{
		leaf("server.login", nil, param("in", ir.Ref("server.LoginInput"))),
		leaf("server.two", nil, param("a", ir.Ref("server.Token")), param("b", ir.Ref("server.Token"))),
		leaf("server.strict", nil, param("in", ir.Ref("server.StrictInput"))),
	}
	t.Run("ok", func(t *testing.T) {
		p := bindFixture([]ir.Decl{ops[0]}, nil, bind("client", "client.token", "server", false))
		diags := Check(p, cfgFor("server", "client"))
		for _, c := range []string{"BIND-1", "BIND-2", "BIND-5", "BIND-6"} {
			wantNoCode(t, diags, c)
		}
	})
	t.Run("BIND-1", func(t *testing.T) {
		p := bindFixture([]ir.Decl{ops[1]}, nil, bind("client", "client.token", "server", false))
		d := wantOneCode(t, Checker{Locate: locatorFor()}.Check(p, cfgFor("server", "client")), "BIND-1")
		if len(d.Related) != 2 {
			t.Errorf("BIND-1 related = %d, want 2 positions", len(d.Related))
		}
		if d.Span == nil || d.Severity != diag.Error {
			t.Errorf("BIND-1 span/severity: %+v", d)
		}
	})
	t.Run("BIND-2", func(t *testing.T) {
		// 状態の形が nullable で、位置（StrictInput.token）が optional でない
		p := bindFixture([]ir.Decl{ops[2]}, nil, bind("client", "client.token", "server", false))
		wantOneCode(t, Check(p, cfgFor("server", "client")), "BIND-2")
		// 状態の形が single なら、同じ位置でも BIND-2 にならない（常に一列）
		p2 := bindFixture([]ir.Decl{ops[2]}, nil, bind("client", "client.session", "server", false))
		wantNoCode(t, Check(p2, cfgFor("server", "client")), "BIND-2")
		// nullable でも、位置が optional（LoginInput.token）なら通る
		p3 := bindFixture([]ir.Decl{ops[0]}, nil, bind("client", "client.token", "server", false))
		wantNoCode(t, Check(p3, cfgFor("server", "client")), "BIND-2")
	})
	t.Run("BIND-5", func(t *testing.T) {
		p := bindFixture([]ir.Decl{ops[0]}, nil,
			bind("client", "client.token", "server", false), bind("client", "client.token2", "server", false))
		d := wantOneCode(t, Check(p, cfgFor("server", "client")), "BIND-5")
		if d.File != "client.hozo" {
			t.Errorf("BIND-5 file = %q", d.File)
		}
		// 別の名前空間の bind は衝突しない
		p2 := bindFixture([]ir.Decl{ops[0]}, nil,
			bind("client", "client.token", "server", false), bind("other", "client.token2", "server", false))
		wantNoCode(t, Check(p2, cfgFor("server", "client")), "BIND-5")
	})
	t.Run("BIND-6", func(t *testing.T) {
		p := bindFixture([]ir.Decl{ops[0]}, nil, bind("client", "client.tokens", "server", false))
		wantOneCode(t, Check(p, cfgFor("server", "client")), "BIND-6")
		wantPaths(t, "list state has no positions", positionsOf(t, p, 0, "server.login"))
		// bind できるのは single と nullable
		for _, s := range []string{"client.token", "client.session"} {
			ok := bindFixture([]ir.Decl{ops[0]}, nil, bind("client", s, "server", false))
			wantNoCode(t, Check(ok, cfgFor("server", "client")), "BIND-6")
		}
	})
}

// TestCallBound は Call.bound: A のすべての Bind の Pos から、明示した位置（name の Atom、レコードリテラルのフィールド）を除く。
func TestCallBound(t *testing.T) {
	login := leaf("server.login", nil, param("in", ir.Ref("server.LoginInput")))
	two := leaf("server.two", nil, param("a", ir.Ref("server.Token")), param("b", ir.Optional(ir.Ref("server.Token"))))
	w := wire("client.w", nil, []ir.OpParam{param("t", ir.Optional(ir.Ref("server.Token")))},
		cs(call("server.login", carg("in", rec()))),
		cs(call("server.login", carg("in", rec(rf("token", nameAtom("t")))))),
		cs(call("server.login", carg("in", nameAtom("t")))),
		cs(call("server.login", carg("in", rec(rf("body", rec(rf("email", ir.LiteralAtom{Literal: ir.StringLiteral{Value: "a"}}))))))),
		cs(call("server.login")),
		cs(call("server.two", carg("a", nameAtom("t")))),
	)
	p := bindFixture([]ir.Decl{login, two}, []ir.Decl{w}, bind("client", "client.token", "server", false))
	Compute(p)
	rows := rowsOf(p.Lookup("client.w").(*ir.Op).Body)
	bound := func(i int) []ir.BoundPosition { return rows[i].calls[0].call.Bound }
	wantBound := func(i int, want ...string) {
		t.Helper()
		var got []string
		for _, b := range bound(i) {
			got = append(got, joinPath(b.Path)+"="+b.State)
		}
		wantStrings(t, sprintf("row %d bound", i), got, want)
	}
	wantBound(0, "in.token=client.token")
	wantBound(1)
	wantBound(2)
	wantBound(3, "in.token=client.token")
	wantBound(4, "in.token=client.token")
	wantBound(5, "b=client.token")

	// 別の名前空間（server の中）からの呼び出しには client の bind は効かない
	inner := wire("server.inner", nil, nil, cs(call("server.login", carg("in", rec()))))
	p2 := bindFixture([]ir.Decl{login, inner}, nil, bind("client", "client.token", "server", false))
	Compute(p2)
	if b := rowsOf(p2.Lookup("server.inner").(*ir.Op).Body)[0].calls[0].call.Bound; len(b) != 0 {
		t.Errorf("server.inner bound = %v, want none", b)
	}
}

// TestBind4 は BIND-4: 必須の引数が args にも bound にもない、レコードリテラルの必須のフィールドが入れ子でも埋まっていない。
func TestBind4(t *testing.T) {
	login := leaf("server.login", nil, param("in", ir.Ref("server.LoginInput")))
	tok := leaf("server.tok", nil, param("t", ir.Ref("server.Token")), param("o", ir.Optional(ir.Int())))
	deep := leaf("server.deep", nil, param("in", ir.Ref("server.Outer")))
	outer := model("server.Outer", field("inner", ir.Ref("server.Inner")), field("opt", ir.Optional(ir.String())))
	inner := model("server.Inner", field("token", ir.Optional(ir.Ref("server.Token"))), field("email", ir.String()), field("page", ir.Optional(ir.Int())))
	w := wire("client.w", nil, nil,
		cs(call("server.login", carg("in", rec()))),                                            // body がない
		cs(call("server.login", carg("in", rec(rf("body", rec()))))),                           // body.email がない
		cs(call("server.login")),                                                               // in がない
		cs(call("server.tok")),                                                                 // t は bind で埋まる。o は optional
		cs(call("server.deep", carg("in", rec(rf("inner", rec()))))),                           // inner.email がない（token は bind、page は optional）
		cs(call("server.deep", carg("in", rec(rf("inner", rec(rf("email", nameAtom("x")))))))), // ok
		cs(call("server.login", carg("in", rec(rf("body", nameAtom("b")))))),                   // name は中身が分からないので ok
	)
	p := bindFixture([]ir.Decl{login, tok, deep, outer, inner}, []ir.Decl{w}, bind("client", "client.token", "server", false))
	diags := Check(p, cfgFor("server", "client"))
	b4 := withCode(diags, "BIND-4")
	if len(b4) != 4 {
		t.Fatalf("BIND-4 count = %d, want 4\n%s", len(b4), messages(diags))
	}
	want := [][]string{{"in.body"}, {"in.body.email"}, {"in"}, {"in.inner.email"}}
	for i, d := range b4 {
		paths := d.Data["missing"].([][]string)
		var got []string
		for _, p := range paths {
			got = append(got, joinPath(p))
		}
		wantStrings(t, sprintf("BIND-4 #%d missing", i), got, want[i])
		if d.File != "server.hozo" { // 呼び出しの位置
			t.Errorf("BIND-4 #%d file = %q", i, d.File)
		}
	}
}
