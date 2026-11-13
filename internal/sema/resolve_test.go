package sema

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/ir"
)

// 解決の手続きの各段（docs/resolve.md「手続き」）に一つずつ（docs/testing.md「単体テスト」）。

// refStrings は Refs を「行:列 kind decl[.name]」の形にする。
func refStrings(res *Result, fs *source.FileSet) []string {
	var out []string
	for _, r := range res.Refs {
		pos := fs.Position(r.Span.File, r.Span.Start)
		s := fmt.Sprintf("%s:%d:%d %s %s", fs.Path(r.Span.File), pos.Line, pos.Col, r.Target.Kind, r.Target.Decl)
		if r.Target.Name != "" {
			s += "." + r.Target.Name
		}
		out = append(out, s)
	}
	return out
}

func hasRef(t *testing.T, res *Result, fs *source.FileSet, want string) {
	t.Helper()
	for _, s := range refStrings(res, fs) {
		if s == want {
			return
		}
	}
	t.Errorf("参照 %q がない。あるもの:\n%s", want, strings.Join(refStrings(res, fs), "\n"))
}

func noErrors(t *testing.T, res *Result, fs *source.FileSet) {
	t.Helper()
	if n, _ := diag.Count(res.Diags); n > 0 {
		var b strings.Builder
		_ = diag.WriteText(&b, res.Diags, fs)
		t.Fatalf("error がある:\n%s", b.String())
	}
}

func codes(res *Result) []string {
	var out []string
	for _, d := range res.Diags {
		out = append(out, d.Code)
	}
	return out
}

// TestResolveStep1_1 は 1.1: 配線の中では、上で束縛した名前と op の引数が値の参照になる。
func TestResolveStep1_1(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  model M { id: string; }
  op value(): M;
  op use(m: M, n: M): void;
  op f(m: M): void {
    const x = value();
    use(m, x);
  }
}`}, false)
	noErrors(t, res, fs)
	hasRef(t, res, fs, "a.hozo:8:9 opParam a.f.m")
	hasRef(t, res, fs, "a.hozo:8:12 binding a.f.x")
	hasRef(t, res, fs, "a.hozo:7:15 decl a.value")
}

// TestResolveStep1_2 は 1.2: デコレータの宣言の throws / catches では、その引数名が $param: になる。
func TestResolveStep1_2(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  state s: single<string>;
  effect e on s.put;
  decorator d(error: any exception, target: any effect) catches error halts target throws error;
}`}, false)
	noErrors(t, res, fs)
	d := res.Program.Lookup("a.d").(*ir.Decorator)
	if got := d.Catches[0].Exceptions[0]; got != "$param:error" {
		t.Errorf("catches = %q", got)
	}
	if got := d.Catches[0].Halts; got != "$param:target" {
		t.Errorf("halts = %q", got)
	}
	if got := d.Throws[0].Exceptions[0]; got != "$param:error" {
		t.Errorf("throws = %q", got)
	}
	hasRef(t, res, fs, "a.hozo:5:65 decoratorParam a.d.error")
	hasRef(t, res, fs, "a.hozo:5:77 decoratorParam a.d.target")
}

// TestResolveStep1_3 は 1.3: op に付いたデコレータの引数の値で、Param.type が type なら op の引数名が $param: になり、
// any なら飛ばして宣言（状態 query）を指す。
func TestResolveStep1_3(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  state query: single<string>;
  decorator cache(key: string[]);
  decorator route(path: string, from: any state?);
  @cache(["orders", query])
  @route("/orders", from: query)
  op f(query: string): void;
}`}, false)
	noErrors(t, res, fs)
	op := res.Program.Lookup("a.f").(*ir.Op)
	list := op.Decorators[0].Args[0].Value.(ir.ListValue)
	if got := list.Items[1].(ir.RefValue).Target; got != "$param:query" {
		t.Errorf("cache の key = %q", got)
	}
	if got := op.Decorators[1].Args[1].Value.(ir.RefValue).Target; got != "a.query" {
		t.Errorf("route の from = %q", got)
	}
	hasRef(t, res, fs, "a.hozo:6:21 opParam a.f.query")
	hasRef(t, res, fs, "a.hozo:7:27 decl a.query")
}

// TestResolveStep1_4 は 1.4: セグメントが二つ以上なら先頭はルートの名前空間。一つだけなら名前空間は候補にならない（@db）。
func TestResolveStep1_4(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace server {
  model User { id: string; }
  @db
  state users: list<User>;
  model Order { @db.ref(User) userId: string; }
}`}, true)
	noErrors(t, res, fs)
	st := res.Program.Lookup("server.users").(*ir.State)
	if st.Decorators[0].Decorator != "db" {
		t.Errorf("@db = %q", st.Decorators[0].Decorator)
	}
	m := res.Program.Lookup("server.Order").(*ir.Model)
	if m.Fields[0].Decorators[0].Decorator != "db.ref" {
		t.Errorf("@db.ref = %q", m.Fields[0].Decorators[0].Decorator)
	}
	hasRef(t, res, fs, "a.hozo:4:4 decl db")
	hasRef(t, res, fs, "a.hozo:6:18 namespace db")
	hasRef(t, res, fs, "a.hozo:6:21 decl db.ref")
}

// TestResolveStep1_5_6 は 1.5 / 1.6: 現在の名前空間の宣言がグローバルを隠す。隠すこと自体は診断しない。
func TestResolveStep1_5_6(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
model M { g: string; }
namespace a {
  model M { l: string; }
  op f(): M;
}
namespace b {
  op g(): M;
}`}, false)
	noErrors(t, res, fs)
	if got := res.Program.Lookup("a.f").(*ir.Op).Returns; !ir.TypeEqual(got, ir.Ref("a.M")) {
		t.Errorf("a.f の returns = %v", got)
	}
	if got := res.Program.Lookup("b.g").(*ir.Op).Returns; !ir.TypeEqual(got, ir.Ref("M")) {
		t.Errorf("b.g の returns = %v", got)
	}
}

// TestResolveStep1_7Candidates は 1.7: RES-1 の Data.candidates（同じ種類で編集距離 2 以内、別の名前空間の同じ名前の FQN）。
func TestResolveStep1_7Candidates(t *testing.T) {
	res, _ := analyzeProject(t, project{"a.hozo": `
namespace a {
  model User { id: string; }
  model Usr { id: string; }
  op user(): User;
  op f(): Usre;
}
namespace b {
  model User { id: string; }
  op g(): Usr;
}`}, false)
	var found int
	for _, d := range res.Diags {
		if d.Code != "RES-1" {
			continue
		}
		found++
		cands := d.Data["candidates"].([]string)
		got := strings.Join(cands, " ")
		switch {
		case strings.Contains(d.Message, "Usre"):
			if got != "a.User a.Usr" { // op user は種類が違うので候補にしない
				t.Errorf("Usre の候補 = %q", got)
			}
		case strings.Contains(d.Message, "Usr"):
			// 別の名前空間の同じ名前（a.Usr）と、現在の名前空間の編集距離 1 の名前（b.User）
			if got != "a.Usr b.User" {
				t.Errorf("Usr の候補 = %q", got)
			}
		}
	}
	if found != 2 {
		t.Errorf("RES-1 が %d 件", found)
	}
}

// TestResolveStep2 は 2: 直前のものの種類で次のセグメントが決まる（値→フィールド、classify→ケース、union→アーム、state→操作）。
func TestResolveStep2(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  model Q { page: int; inner: Q?; }
  classify C of Q { A; }
  union U { Ready: Q; }
  state s: list<Q>;
  decorator when(v: any variant);
  op takeInt(n: int): void;
  op takeOpt(n: int?): void;
  op f(q: Q): void {
    takeInt(q.page);
    takeOpt(q.inner.page);
  }
  op g(q: C.A<Q>): void;
  @when(U.Ready)
  op h(): void;
  decorator d(e: any effect);
  @d(s.post)
  op bad(): void;
}`}, false)
	if got := strings.Join(codes(res), ","); got != "RES-4" {
		var b strings.Builder
		_ = diag.WriteText(&b, res.Diags, fs)
		t.Fatalf("診断 = %s\n%s", got, b.String())
	}
	hasRef(t, res, fs, "a.hozo:11:15 field a.Q.page")
	hasRef(t, res, fs, "a.hozo:12:21 field a.Q.page")
	hasRef(t, res, fs, "a.hozo:14:13 case a.C.A")
	hasRef(t, res, fs, "a.hozo:15:11 arm a.U.Ready")
	v := res.Program.Lookup("a.h").(*ir.Op).Decorators[0].Args[0].Value.(ir.VariantValue)
	if v.Union != "a.U" || v.Arm != "Ready" {
		t.Errorf("variant = %+v", v)
	}
	g := res.Program.Lookup("a.g").(*ir.Op)
	if !ir.TypeEqual(g.Params[0].Type, ir.Case("a.C", []string{"A"}, ir.Ref("a.Q"))) {
		t.Errorf("C.A<Q> = %v", g.Params[0].Type)
	}
}

// TestResolveBindNamespace は bind の ns: セグメントが一つでも名前空間として解決する。
func TestResolveBindNamespace(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace server { model Token { v: string; } }
namespace browser {
  state token: nullable<server.Token>;
  bind token to server.*;
  bind token to server.**;
}`}, false)
	noErrors(t, res, fs)
	if len(res.Program.Binds) != 2 || res.Program.Binds[0].Target.Namespace != "server" || res.Program.Binds[0].Target.Deep || !res.Program.Binds[1].Target.Deep {
		t.Errorf("binds = %+v", res.Program.Binds)
	}
	hasRef(t, res, fs, "a.hozo:5:17 namespace server")
	hasRef(t, res, fs, "a.hozo:5:8 decl browser.token")
}

// TestDuplicatePosition は RES-6 の位置: ユーザーのファイルの側、両方ユーザーなら原則の順で後の方。NS-3 は両方が語彙。
func TestDuplicatePosition(t *testing.T) {
	res, fs := analyzeProject(t, project{
		"b.hozo":                   "namespace a { model X { id: string; } }\nnamespace http { exception NetworkError; }\n",
		"a.hozo":                   "namespace a { model X { id: string; } }\n",
		"hozo:plugin/p/vocab.hozo": "decorator db for state;\n",
	}, true)
	got := map[string]string{}
	for _, d := range res.Diags {
		got[d.Code] = fmt.Sprintf("%s:%d", fs.Path(d.Span.File), fs.Position(d.Span.File, d.Span.Start).Line)
		if len(d.Related) != 1 {
			t.Errorf("%s に Related がない", d.Code)
		}
	}
	if got["RES-6"] == "" {
		t.Fatalf("RES-6 がない: %v", codes(res))
	}
	if got["NS-3"] != "hozo:vocab/db.hozo:2" { // 語彙どうしは順で後の方（hozo:vocab は hozo:plugin の後）
		t.Errorf("NS-3 の位置 = %s", got["NS-3"])
	}
	// b.hozo の二つ: a.X（a.hozo が先なので b.hozo 側）と http.NetworkError（ユーザーの側）
	n := 0
	for _, d := range res.Diags {
		if d.Code == "RES-6" && fs.Path(d.Span.File) == "b.hozo" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("b.hozo の RES-6 が %d 件", n)
	}
	// 表に残るのは語彙の側: http.NetworkError は語彙の宣言
	if d := res.Program.Lookup("http.NetworkError"); d == nil || !strings.HasPrefix(d.Common().Span.File, "hozo:") {
		t.Errorf("http.NetworkError が語彙の宣言でない")
	}
}

// TestFileOrder はファイルの順（docs/ir.md「原則」）: 名前空間の出現順と宣言の順は、ユーザーの辞書順、次に語彙。
func TestFileOrder(t *testing.T) {
	res, _ := analyzeProject(t, project{
		"z.hozo": "namespace z { model Z { id: string; } }\nnamespace shared { model FromZ { id: string; } }\n",
		"a.hozo": "namespace shared { model FromA { id: string; } }\nnamespace a { model A { id: string; } }\n",
	}, true)
	var names []string
	for _, ns := range res.Program.Namespaces[:4] {
		names = append(names, ns.Name)
	}
	if got := strings.Join(names, ","); got != ",shared,a,z" {
		t.Errorf("名前空間の順 = %s", got)
	}
	shared := res.Program.Namespace("shared")
	if shared.Decls[0].Common().Name != "shared.FromA" || shared.Decls[1].Common().Name != "shared.FromZ" {
		t.Errorf("併合した宣言の順が違う: %s, %s", shared.Decls[0].Common().Name, shared.Decls[1].Common().Name)
	}
	last := res.Program.Namespaces[len(res.Program.Namespaces)-1]
	if !strings.HasPrefix(last.Decls[0].Common().Span.File, "hozo:") {
		t.Errorf("語彙が最後に来ていない: %s", last.Name)
	}
}

// TestErrorTolerance は解決の誤りがあっても Program を作り、解決できた部分を残すこと（docs/packages.md「internal/sema」）。
func TestErrorTolerance(t *testing.T) {
	res, _ := analyzeProject(t, project{"a.hozo": `
namespace a {
  model M { id: string; bad: Nope; }
  op f(m: M): Nope throws Missing;
  op g(): M {
    const x = nope();
    return other(x);
  }
}`}, false)
	m := res.Program.Lookup("a.M").(*ir.Model)
	if len(m.Fields) != 2 || m.Fields[1].Type != nil {
		t.Errorf("解決できなかったフィールドの型は省く: %+v", m.Fields)
	}
	f := res.Program.Lookup("a.f").(*ir.Op)
	if f.Returns != nil || len(f.Throws) != 1 || len(f.Throws[0].Exceptions) != 0 {
		t.Errorf("f = %+v", f)
	}
	g := res.Program.Lookup("a.g").(*ir.Op)
	if g.Body == nil || len(g.Body.Statements) != 0 {
		t.Errorf("解決できなかった文は落とす: %+v", g.Body)
	}
	for _, d := range res.Diags {
		if d.Span == nil {
			t.Errorf("Span のない診断: %+v", d)
		}
	}
}
