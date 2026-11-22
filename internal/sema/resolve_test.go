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
  fn value(): M;
  fn use(m: M, n: M): void;
  fn f(m: M): void {
    val   x = value();
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
  effect fn e(): effects s.put;
  decorator fn d(error: any exception, target: any effect): catches error halts target throws error;
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
	hasRef(t, res, fs, "a.hozo:5:69 decoratorParam a.d.error")
	hasRef(t, res, fs, "a.hozo:5:81 decoratorParam a.d.target")
}

// TestResolveStep1_3 は 1.3: op に付いたデコレータの引数の値で、Param.type が type なら op の引数名が $param: になり、
// any なら飛ばして宣言（状態 query）を指す。
func TestResolveStep1_3(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  state query: single<string>;
  decorator fn cache(key: string[]);
  decorator fn route(path: string, from: any state?);
  @cache(["orders", query])
  @route("/orders", from: query)
  fn f(query: string): void;
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
// 名前空間の名前と、グローバルの宣言の名前は別の空間（docs/resolve.md「名前空間の併合と重複」）: グローバルのデコレータ db と、
// 語彙の名前空間 db の db.ref は共存する。
func TestResolveStep1_4(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace server {
  model User { id: string; }
  @db
  state users: list<User>;
  model Order { @db.ref(User) userId: string; }
}
decorator db for state;`}, true)
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
  fn f(): M;
}
namespace b {
  fn g(): M;
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
  fn user(): User;
  fn f(): Usre;
}
namespace b {
  model User { id: string; }
  fn g(): Usr;
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

// TestResolveRemovedCoreTypes は 1.7 の、なくした核の型の名前（docs/resolve.md「手続き」）: 宣言がなければ、RES-1 のメッセージに、
// 今の語を足す。同じ名前の宣言があれば、それに解決する（もう、予約語ではない）。
func TestResolveRemovedCoreTypes(t *testing.T) {
	res, _ := analyzeProject(t, project{"a.hozo": `
namespace a {
  model M { at: Time; span: Duration; }
}`}, false)
	want := map[string]string{"Time": "datetime", "Duration": "duration"}
	for _, d := range res.Diags {
		if d.Code != "RES-1" {
			t.Errorf("予期しない診断 %s: %s", d.Code, d.Message)
			continue
		}
		for old, now := range want {
			if strings.Contains(d.Message, old+" が見つからない") {
				if !strings.Contains(d.Message, now+" に名前が変わった") {
					t.Errorf("%s のメッセージに、今の語 %s がない: %s", old, now, d.Message)
				}
				delete(want, old)
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("RES-1 が出なかった名前: %v", want)
	}
	res, _ = analyzeProject(t, project{"a.hozo": `
namespace a {
  model Time { hour: int; }
  model M { at: Time; }
}`}, false)
	if len(res.Diags) != 0 {
		t.Errorf("宣言した Time を指して、診断が出た: %v", res.Diags)
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
  decorator fn when(v: any variant);
  fn takeInt(n: int): void;
  fn takeOpt(n: int?): void;
  fn f(q: Q): void {
    takeInt(q.page);
    takeOpt(q.inner.page);
  }
  fn g(q: Q on C.A): void;
  @when(U.Ready)
  fn h(): void;
  decorator fn d(e: any effect);
  @d(s.post)
  fn bad(): void;
}`}, false)
	if got := strings.Join(codes(res), ","); got != "RES-4" {
		var b strings.Builder
		_ = diag.WriteText(&b, res.Diags, fs)
		t.Fatalf("診断 = %s\n%s", got, b.String())
	}
	hasRef(t, res, fs, "a.hozo:11:15 field a.Q.page")
	hasRef(t, res, fs, "a.hozo:12:21 field a.Q.page")
	hasRef(t, res, fs, "a.hozo:14:16 decl a.C")
	hasRef(t, res, fs, "a.hozo:14:18 case a.C.A")
	hasRef(t, res, fs, "a.hozo:15:11 arm a.U.Ready")
	v := res.Program.Lookup("a.h").(*ir.Op).Decorators[0].Args[0].Value.(ir.VariantValue)
	if v.Union != "a.U" || v.Arm != "Ready" {
		t.Errorf("variant = %+v", v)
	}
	g := res.Program.Lookup("a.g").(*ir.Op)
	if !ir.TypeEqual(g.Params[0].Type, ir.Case("a.C", []string{"A"}, ir.Ref("a.Q"))) {
		t.Errorf("Q on C.A = %v", g.Params[0].Type)
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
		"hozo:plugin/p/vocab.hozo": "decorator localStorage for state;\n",
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
	if got["NS-3"] != "hozo:vocab/localstorage.hozo:2" { // 語彙どうしは順で後の方（hozo:vocab は hozo:plugin の後）
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
  fn f(m: M): Nope throws Missing;
  fn g(): M {
    val   x = nope();
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

// TestResolveStateValue は docs/resolve.md「手続き」の 1 と 2 の、配線の値の位置の状態: 束縛と引数が先（状態を隠す）、
// なければ今の名前空間の状態（修飾して書いても同じ）。状態から先のセグメントはその値のフィールド。
// 別の名前空間（かグローバル）の状態は RES-5、それ以外の宣言は RES-4。
func TestResolveStateValue(t *testing.T) {
	p := project{"a.hozo": `
state shared: list<string>;
namespace a {
  model Profile { nick: string; }
  model User { id: string; profile: Profile; }
  state token: nullable<string>;
  state me: single<User>;
  state maybe: User?;
  fn takeToken(t: string?);
  fn takeString(s: string);
  fn takeOptString(s: string?);
  fn f(token: string) {
    takeString(token);
  }
  fn g() {
    takeToken(token);
    takeToken(a.token);
    takeString(me.profile.nick);
    takeOptString(maybe.id);
  }
  fn ng() {
    takeToken(b.remote);                // ERROR: RES-5
    takeToken(shared);                  // ERROR: RES-5
    takeToken(Profile);                 // ERROR: RES-4
    takeString(me.nope);                // ERROR: RES-3
    takeString(token.value);            // ERROR: RES-3
  }
}
namespace b {
  state remote: nullable<string>;
}`}
	res, fs := analyzeProject(t, p, false)
	checkAnnotations(t, fs, p, res.Diags)
	// 引数が同じ名前なら、引数を指す（状態を隠す）
	hasRef(t, res, fs, "a.hozo:13:16 opParam a.f.token")
	// 状態の名前は、状態の宣言への参照。修飾して書けば、名前空間と状態
	hasRef(t, res, fs, "a.hozo:16:15 decl a.token")
	hasRef(t, res, fs, "a.hozo:17:15 namespace a")
	hasRef(t, res, fs, "a.hozo:17:17 decl a.token")
	// 状態から先はフィールド
	hasRef(t, res, fs, "a.hozo:18:16 decl a.me")
	hasRef(t, res, fs, "a.hozo:18:19 field a.User.profile")
	hasRef(t, res, fs, "a.hozo:18:27 field a.Profile.nick")
	// IR: 束縛と引数は Atom.name、状態は Atom.state（修飾して書いても同じ。path は状態の値のフィールド）
	g := res.Program.Lookup("a.g").(*ir.Op).Body.Statements
	atom := func(i int) ir.Atom { return g[i].(ir.CallStmt).Call.Args[0].Value }
	for i, want := range []ir.StateAtom{
		{State: "a.token"}, {State: "a.token"}, {State: "a.me", Path: []string{"profile", "nick"}}, {State: "a.maybe", Path: []string{"id"}},
	} {
		got, ok := atom(i).(ir.StateAtom)
		if !ok || got.State != want.State || strings.Join(got.Path, ".") != strings.Join(want.Path, ".") {
			t.Errorf("g の %d 行目の引数 = %#v, want %#v", i+1, atom(i), want)
		}
	}
	if _, ok := res.Program.Lookup("a.f").(*ir.Op).Body.Statements[0].(ir.CallStmt).Call.Args[0].Value.(ir.NameAtom); !ok {
		t.Errorf("引数と同じ名前は Atom.name")
	}
}

// TestRefsForNewPositions は、定義へジャンプとホバーが使う参照（docs/lsp.md「定義へジャンプ」の表）のうち、
// extends の元の model、override のフィールド、分類の対象の op、ケースの X as T、reads の状態、修飾した分類、throw の例外。
func TestRefsForNewPositions(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace t {
  model User { id: string; hash: string; }
  exception Gone;
}
namespace a {
  model PublicUser extends t.User { override hash: never; }
  model Admin extends PublicUser { override id: Id; }
  scalar Id extends string;
  state users: list<t.User>;
  state current: t.User?;
  export fn find(id: string): t.User? reads users, a.current { return first(users, current); } fn first(us: t.User[], c: t.User?): t.User?;
  classify Found of find { Hit: find as t.User; Miss: find as else; }
  classify Session of current { In: current as t.User; Out: current as else; }
}
namespace b {
  fn keep(u: t.User): t.User;
  fn f(id: string): t.User throws t.Gone {
    val   u = a.find(id);
    return match(u by a.Found){
      Hit     => keep(u);
      Miss    => throw t.Gone;
    };
  }
}`}, false)
	noErrors(t, res, fs)
	for _, want := range []string{
		"a.hozo:7:30 decl t.User",           // extends B
		"a.hozo:7:46 field t.User.hash",     // override f → 元の model のフィールド
		"a.hozo:8:23 decl a.PublicUser",     // 元の model が継承した model
		"a.hozo:8:45 field a.PublicUser.id", // 元の model の fields の、そのフィールド
		"a.hozo:12:45 decl a.users",         // reads
		"a.hozo:12:54 decl a.current",       // reads（修飾）
		"a.hozo:13:21 decl a.find",          // classify C of op
		"a.hozo:13:33 decl a.find",          // ケースの op as T の op
		"a.hozo:13:43 decl t.User",          // ケースの X as T の T
		"a.hozo:14:23 decl a.current",       // 状態の分類の対象
		"a.hozo:14:37 decl a.current",       // ケースの X as T の X（状態）
		"a.hozo:20:25 decl a.Found",         // match x by ns.C
		"a.hozo:21:7 case a.Found.Hit",      // アームのケース名
		"a.hozo:22:26 decl t.Gone",          // throw E
	} {
		hasRef(t, res, fs, want)
	}
	// override f の飛び先: 元の model が受け継いだままのフィールドなら、さらに元の宣言の位置
	for _, r := range res.Refs {
		pos := fs.Position(r.Span.File, r.Span.Start)
		if pos.Line == 8 && pos.Col == 45 {
			def := fs.Position(r.Target.Span.File, r.Target.Span.Start)
			if def.Line != 3 || def.Col != 16 {
				t.Errorf("override id の飛び先 = %d:%d, want 3:16（t.User の id）", def.Line, def.Col)
			}
		}
	}
}

// TestCallResolution は docs/resolve.md「呼び出しの読み分け」の表の各行と、その参照（定義へのジャンプ）: 行き着いたものの
// 種類で、op、read、効果（宣言）、直に呼ぶ組み込みの効果のどれを呼ぶかが決まる。
func TestCallResolution(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  model M { id: string; }
  exception Busy(wait: duration);
  state rows: list<M>;
  read fn byId(id: string): M? reads rows;
  effect fn add(m: M): effects rows.post;
  fn keep(m: M?): M?;
  classify K of M { A; B; }
  fn f(m: M): M? throws Busy {
    add(m);
    rows.post(m);
    a.rows.post(m);
    val   found = a.byId(m.id);
    return match(m by K){
      A => throw Busy(5m);
      else    => keep(found);
    };
  }
}`}, false)
	noErrors(t, res, fs)
	for _, want := range []string{
		"a.hozo:6:38 decl a.rows",  // read fn r(…): T reads s の s
		"a.hozo:6:29 decl a.M",     // read の結果の型
		"a.hozo:7:20 decl a.M",     // effect の引数の型
		"a.hozo:11:5 decl a.add",   // 効果の呼び出し
		"a.hozo:12:5 decl a.rows",  // 直に呼ぶ「状態.操作」の状態
		"a.hozo:13:7 decl a.rows",  // 修飾して書いた状態
		"a.hozo:14:21 decl a.byId", // read の呼び出し（修飾）
		"a.hozo:16:18 decl a.Busy", // throw E(…) の E
		"a.hozo:17:18 decl a.keep", // op の呼び出し
	} {
		hasRef(t, res, fs, want)
	}
	f := res.Program.Lookup("a.f").(*ir.Op)
	var kinds []string
	for _, st := range f.Body.Statements {
		switch x := st.(type) {
		case ir.CallStmt:
			kinds = append(kinds, x.Call.Callee.Kind()+" "+x.Call.Callee.String())
		case ir.ValStmt:
			kinds = append(kinds, x.Expr.(ir.CallExpr).Call.Callee.Kind()+" "+x.Expr.(ir.CallExpr).Call.Callee.String())
		}
	}
	want := []string{"effect a.add", "effect a.rows.post", "effect a.rows.post", "read a.byId"}
	if strings.Join(kinds, "; ") != strings.Join(want, "; ") {
		t.Errorf("callee = %v, want %v", kinds, want)
	}
}

// TestCallShadowedByBinding は、呼び出しの名前の先頭が束縛か引数の名前と同じなら、手続き 1.1 が先で、値を指すこと
// （docs/resolve.md「呼び出しの読み分け」）。状態の操作を呼ぶには、修飾して書く。
func TestCallShadowedByBinding(t *testing.T) {
	res, _ := analyzeProject(t, project{"a.hozo": `
namespace a {
  model M { id: string; }
  state rows: list<M>;
  fn f(rows: M[], m: M) {
    rows.post(m);
    a.rows.post(m);
  }
}`}, false)
	var got []string
	for _, d := range res.Diags {
		got = append(got, d.Code)
	}
	if strings.Join(got, ",") != "RES-3" {
		t.Errorf("診断 = %v, want [RES-3]（引数 rows のフィールド post は辿れない）", got)
	}
}

// TestResolveDecoratorBody は、デコレータの本体（docs/grammar.md「デコレータの本体」、docs/resolve.md「手続き」の 1.1）:
// 本体の中の名前は、デコレータを宣言した名前空間で解決し、値の引数は、そのデコレータの引数を指す。包んだ関数の引数の呼び出しは、
// 包んだ op の中身の呼び出し（Callee の proceed）になり、その引数への参照を残す（同じ名前の op を隠す）。IR は、Decorator.fn と
// Decorator.body で、包んだ関数の引数は、Decorator.params に入らない。
func TestResolveDecoratorBody(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `
namespace a {
  fn note(label: string);
  fn next();
  decorator fn noted(next: fn, label: string) {
    next();
    note(label);
  }
  @noted("x")
  fn f();
}`}, false)
	noErrors(t, res, fs)
	d := res.Program.Lookup("a.noted").(*ir.Decorator)
	if !d.Fn || d.Body == nil || len(d.Body.Statements) != 2 {
		t.Fatalf("Decorator = %+v", d)
	}
	if len(d.Params) != 1 || d.Params[0].Name != "label" {
		t.Errorf("Decorator.params = %+v（期待は、label だけ: 包んだ関数の引数は、入らない）", d.Params)
	}
	first, ok := d.Body.Statements[0].(ir.CallStmt)
	if !ok || !first.Call.IsProceed() || len(first.Call.Args) != 0 {
		t.Errorf("一つ目の文 = %+v（期待は、包んだ関数の呼び出し。同じ名前の op a.next では、ない）", d.Body.Statements[0])
	}
	second, ok := d.Body.Statements[1].(ir.CallStmt)
	if !ok || second.Call.CalledOp() != "a.note" || len(second.Call.Args) != 1 {
		t.Fatalf("二つ目の文 = %+v（期待は a.note の呼び出し）", d.Body.Statements[1])
	}
	if atom, ok := second.Call.Args[0].Value.(ir.NameAtom); !ok || len(atom.Path) != 1 || atom.Path[0] != "label" {
		t.Errorf("note の引数 = %+v（期待は、デコレータの引数 label）", second.Call.Args[0].Value)
	}
	hasRef(t, res, fs, "a.hozo:6:5 decoratorParam a.noted.next")
	hasRef(t, res, fs, "a.hozo:7:5 decl a.note")
	hasRef(t, res, fs, "a.hozo:7:10 decoratorParam a.noted.label")
	// 付けた op の Signature は、本体を広げたもの: 本体が呼ぶ op の分を含む（ここでは、例外も効果もない）
	if f := res.Program.Lookup("a.f").(*ir.Op); len(f.Decorators) != 1 || f.Decorators[0].Decorator != "a.noted" {
		t.Errorf("f のデコレータ = %+v", f.Decorators)
	}
}
