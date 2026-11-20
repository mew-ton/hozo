package sema

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

var update = flag.Bool("update", false, "ゴールデンを更新する（docs/testing.md「コーパス」）")

// normalizeSource は docs/ir.md「正規化」の表の各行を一つずつ含む入力。
var normalizeSource = project{
	"b.hozo": `
/** 名前空間 a の説明。二つ目のブロックの説明は使わない */
namespace a {
  model Later { id: string; }
}
namespace a.child { model Deep { id: string; } }
`,
	"a.hozo": `
/** 使われない説明（b.hozo の方が後） */
namespace a {
  namespace child { model Deeper { id: string; } }

  /** メールアドレス */
  scalar Email extends string;
  model User { id: string; email: Email; hash: string; nick: string??; }
  model PublicUser extends User { override hash: never; }

  decorator mark for field;
  model Member {
    /** 識別子 */
    @mark id: string;
    /** 名前 */
    name: string;
    /** 段階 */
    level: int;
    since: Time;
  }
  model Admin extends Member {
    override id: Email;
    /** 新しい名前 */
    @mark override name;
    override level: never;
    role: string;
  }
  model Shapes { a: list<string>; b: nullable<int>; c: single<User>; d: nullable<string?>; e: single<list<User>?>; }

  state users: list<User>;
  state token: nullable<string>;
  state title: single<string>;
  state plain: string;
  state maybe: single<string?>;
  state rows: User[];
  effect fn add(u: User): effects users.post;
  effect fn set(t: string): effects token.put;

  exception Unauthorized;
  exception Invalid(field: string);
  exception Wrapped(user: User, at: Time?);
  union Errors { Unauthorized; Invalid; }
  union AllErrors { Errors; Wrapped; }
  union Effects { add; set; }

  union Page {
    /** 準備できた */
    Ready: { name: string; };
    Missing: User;
  }

  classify IsActive of User { Active; Inactive; }
  classify Kind of User { A; /** B の説明 */ B; /** それ以外 */ C; }

  @external
  decorator get(path: string) throws Unauthorized;
  decorator fallback(error: any exception, view: any model) for op | field catches error recovers;
  decorator unavailable = fallback(AllErrors, User);
  decorator cache(key: string[], stale: Duration?);
  decorator tagged(tags: string | int) for model;

  /**
   * ユーザーを探す。
   *
   * 詳しい説明。
   *
   * @param id 識別子
   * @throws Unauthorized 認証がない
   * @see User
   */
  @get("/users")
  fn find(id: string): User throws Unauthorized prevents effects throws Invalid;
  fn noop();
  fn void2(): void;
  fn leaf(u: User): PublicUser throws AllErrors keeps effects;
  fn setToken(t: string) { set(t); }
  fn both(u: User): PublicUser effects Effects throws AllErrors {
    add(u);
    setToken("x");
    return leaf(u);
  }
  fn take(in: User): void;
  fn fromIn(in: User): User;

  read fn userById(id: string): User? reads users;
  export fn find2(id: string): User? reads users, token reads users {
    return match(token by nullable){ HasItems => userById(id); Empty => none; };
  }
  fn nameOf(u: User): string;
  fn gone(): never throws Unauthorized;
  classify Found of find2 { Hit: find2 as User; Miss: find2 as else; }
  classify Session of token { In: token as string; Out: token as else; }
  fn wire2(id: string): string throws Unauthorized {
    val   u = find2(id);
    return match(u by Found){
      Hit => nameOf(u);
      else => throw Unauthorized;
    };
  }
  fn wire3(): string? {
    return match(token by nullable){ HasItems => token; Empty => none; };
  }
  fn wire4(): string { return a.title; }
  fn wire5(): string {
    return match(by Session){ In => token; Out => gone(); };
  }

  @unavailable
  @cache(["users", id], 5m)
  fn wire(id: string): User {
    val   u = find(id);
    take({ id, email: u.email, hash: "x" });
    val   w = match(u by Kind){
      A, B => fromIn(u);
      else => none;
    };
    return u;
  }
}
`,
}

// normalizeSource に足す、版 0.5 の行（docs/ir.md「正規化」の「残りの引数に渡した値」「デコレータの宣言の reads / effects」
// 「op に付いたデコレータの reads / effects の節」）と、対象 effect、予約語のデコレータ名。
func init() {
	normalizeSource["c.hozo"] = `
namespace store {
  model Row { @text id: string; }
  @table(Row) state rows: list<Row>;
  state logs: list<Row>;

  decorator table(element: any model) for state;
  decorator text for field;
  decorator find(table: any state, column: string) for op reads table;
  decorator list(table: any state, column: string?) for op reads table;
  decorator insert(table: any state) for effect;
  decorator effects(...targets: any effect) for op effects targets;
  decorator audited(extra: any state?) for op reads logs, extra reads rows effects Writes, log;
  decorator tags(first: string, ...more: string | int) for op | effect;
  decorator both = effects(addRow, Writes);

  @insert(rows) effect fn addRow(r: Row): effects rows.post;
  @insert(logs) @tags("a") effect fn log(l: Row): effects logs.post;
  effect fn touch(): effects rows.put;
  union Writes { addRow; touch; }

  /**
   * 一行を引く。
   *
   * @see list
   * @see store.effects
   */
  @find(rows, "id") fn findRow(id: string): Row?;
  @list(rows) fn allRows(): Row[];
  @effects(addRow, touch | log, Writes) fn place(r: Row, l: Row);
  @effects fn nothing();
  @effects() fn nothingEither();
  @both fn viaAlias(r: Row);
  @audited @find(logs, "id") fn mixed(id: string): Row?;
  @audited(logs) @tags("x", 1, "y") fn audit();
}
`
}

// normalizeSource に足す、版 0.6 の行（docs/ir.md「正規化」の「exception X;」「配線の呼び出し」「throw E(a, b)」「reads a, b」
// 「effects a, b」「デコレータの宣言の reads / effects」）: 引数を持つ exception / effect / read、配線からの read と効果の
// 呼び出し、直に呼ぶ組み込みの効果、引数を持つ throw、節に名前で書いた要素と直に書いた要素、対象 read、@repeatable。
func init() {
	normalizeSource["d.hozo"] = `
namespace lib {
  model Row { id: string; }
  state rows: list<Row>;
  state logs: list<Row>;
  state title: single<string>;
  state cursor: nullable<Row>;

  decorator find(table: any state) for read;
  @repeatable decorator where(column: string, value: string | int) for read;
  decorator scan(table: any state, via: any state?) for op
    reads via, table, rowById reads logs
    effects logs.post, touch;

  /**
   * id で一行を引く
   *
   * @param id 引く行の識別子
   */
  @find(rows) @where("id", "x") @where("kind", 1) read fn rowById(id: string): Row? reads rows;
  read fn allLogs(): Row[] reads logs;
  /** @param r 触る行 */
  effect fn touch(r: Row): effects rows.put;
  /** @param wait 待つ時間 */
  exception Busy(wait: Duration, note: string?);

  fn named(id: string): Row? reads rowById { return rowById(id); }
  fn direct(): Row[] reads rows { return rows; }
  fn pick(row: Row?, a: Row[], b: Row[]): Row[];
  fn mixed(id: string): Row[] reads rowById, rows, lib.allLogs reads rows, rowById {
    val   row = rowById(id);
    val   logged = allLogs();
    return pick(row, logged, rows);
  }
  fn write(r: Row): effects rows.post { rows.post(r); }
  fn rename(t: string): effects title.put { title.put(t); }
  fn forget() { cursor.delete(); }
  fn remember(r: Row) { cursor.post(r); lib.cursor.put(r); }
  fn touchIt(r: Row): effects touch { touch(r); }
  @scan(logs) fn scanned(): Row[];
  @scan(rows, logs) fn scanned2(): Row[];
  @scan(rows) fn scannedWired(): Row[] { return rows; }
  classify Load of Row { Heavy; Light; }
  fn wired(r: Row): Row? effects rows.post, touch, lib.title.put reads rowById throws Busy {
    write(r);
    touchIt(r);
    rename("t");
    return match(r by Load){
      Heavy => throw Busy(5m, "heavy");
      Light => named(r.id);
    };
  }
}
`
	// 版 0.9 の行（docs/ir.md「正規化」）: ケースの `X as none`、行の遷移（`effect fn …: effects s.op as …`）。入力は row_test.go
	normalizeSource["row.hozo"] = rowSource
}

// TestNormalizeRefs は、版 0.6 の行（docs/ir.md「正規化」）を、手で組んだ期待と比べる: 引数を持つ exception / effect / read、
// 配線の呼び出しの callee、throw の引数、reads と effects の要素（名前で書いた要素は named、直に書いた要素は direct）、
// @repeatable。
func TestNormalizeRefs(t *testing.T) {
	res, fs := analyzeProject(t, normalizeSource, false)
	noErrors(t, res, fs)
	p := res.Program
	eq := func(name string, got, want any) {
		t.Helper()
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		if !bytes.Equal(g, w) {
			t.Errorf("%s:\n got  %s\n want %s", name, g, w)
		}
	}
	op := func(name string) *ir.Op { return p.Lookup("lib." + name).(*ir.Op) }
	named := func(fqn string) ir.ReadRef { return ir.NamedRead{Read: fqn} }
	state := func(fqn string) ir.ReadRef { return ir.DirectRead{State: fqn} }
	post := ir.DirectEffect{State: "lib.rows", Operation: ir.OpPost}
	touch := ir.NamedEffect{Effect: "lib.touch"}
	row := ir.Ref("lib.Row")
	params := func(ps []ir.OpParam) []any {
		var out []any
		for _, x := range ps {
			out = append(out, []any{x.Name, x.Type})
		}
		return out
	}

	// read の宣言: 引数、結果の型、宛先の状態、説明（@param を含む）、付けたデコレータ（対象 read）
	read := p.Lookup("lib.rowById").(*ir.Read)
	eq("read の引数", params(read.Params), []any{[]any{"id", ir.String()}})
	eq("read の結果の型", read.Returns, ir.Optional(row))
	eq("read の宛先", read.State, "lib.rows")
	eq("read の説明", []any{read.Description.Summary, read.Description.Params}, []any{"id で一行を引く", []ir.DocParam{{Name: "id", Text: "引く行の識別子"}}})
	eq("引数のない read", []any{len(p.Lookup("lib.allLogs").(*ir.Read).Params), p.Lookup("lib.allLogs").(*ir.Read).Returns}, []any{0, ir.Array(row)})
	eq("対象 read", p.Lookup("lib.find").(*ir.Decorator).Targets, []ir.DecoratorTarget{ir.TargetRead})
	eq("read は、状態の後ろ、効果の前（出現順）", declAfter(p, "lib.allLogs"), "lib.touch")

	// @repeatable: 同じデコレータの適用が、付けた数だけ、書かれた順に並ぶ
	where := p.Lookup("lib.where").(*ir.Decorator)
	eq("repeatable", []bool{where.Repeatable, where.External, p.Lookup("lib.find").(*ir.Decorator).Repeatable}, []bool{true, false, false})
	var applied []any
	for _, app := range read.Decorators {
		applied = append(applied, []any{app.Decorator, app.Args[0].Value})
	}
	eq("何度も付けた適用", applied, []any{
		[]any{"lib.find", ir.RefValue{Target: "lib.rows"}},
		[]any{"lib.where", ir.LiteralValue{Literal: ir.StringLiteral{Value: "id"}}},
		[]any{"lib.where", ir.LiteralValue{Literal: ir.StringLiteral{Value: "kind"}}},
	})

	// effect と exception の引数
	eff := p.Lookup("lib.touch").(*ir.Effect)
	eq("effect の引数", params(eff.Params), []any{[]any{"r", row}})
	eq("effect の説明の @param", eff.Description.Params, []ir.DocParam{{Name: "r", Text: "触る行"}})
	busy := p.Lookup("lib.Busy").(*ir.Exception)
	eq("exception の引数", params(busy.Params), []any{[]any{"wait", ir.Duration()}, []any{"note", ir.Optional(ir.String())}})
	eq("exception の説明の @param", busy.Description.Params, []ir.DocParam{{Name: "wait", Text: "待つ時間"}})
	eq("値を運ばない例外の params は空", len(p.Lookup("a.Unauthorized").(*ir.Exception).Params), 0)
	eq("その場の model は、例外からは合成しない", p.Lookup("a.Invalid.Value"), nil)

	// 配線の呼び出しの callee: read、効果の宣言、直に呼ぶ組み込みの効果、op
	callOf := func(st ir.Statement) ir.Call {
		switch x := st.(type) {
		case ir.CallStmt:
			return x.Call
		case ir.ValStmt:
			return x.Expr.(ir.CallExpr).Call
		case ir.ReturnStmt:
			return x.Expr.(ir.CallExpr).Call
		}
		t.Fatalf("呼び出しでない文: %T", st)
		return ir.Call{}
	}
	argNames := func(c ir.Call) []string {
		var out []string
		for _, a := range c.Args {
			out = append(out, a.Param)
		}
		return out
	}
	rc := callOf(op("named").Body.Statements[0])
	eq("read の呼び出し", []any{rc.Callee, argNames(rc)}, []any{ir.ReadCallee{Read: "lib.rowById"}, []string{"id"}})
	ec := callOf(op("touchIt").Body.Statements[0])
	eq("効果の呼び出し", []any{ec.Callee, argNames(ec)}, []any{ir.EffectCallee{Effect: touch}, []string{"r"}})
	dc := callOf(op("write").Body.Statements[0])
	eq("直に呼ぶ組み込みの効果（list の post）", []any{dc.Callee, argNames(dc)}, []any{ir.EffectCallee{Effect: post}, []string{ir.DirectEffectParam}})
	eq("single の put", callOf(op("rename").Body.Statements[0]).Callee, ir.EffectCallee{Effect: ir.DirectEffect{State: "lib.title", Operation: ir.OpPut}})
	del := callOf(op("forget").Body.Statements[0])
	eq("nullable の delete は引数なし", []any{del.Callee, len(del.Args)}, []any{ir.EffectCallee{Effect: ir.DirectEffect{State: "lib.cursor", Operation: ir.OpDelete}}, 0})
	eq("nullable の post", callOf(op("remember").Body.Statements[0]).Callee, ir.EffectCallee{Effect: ir.DirectEffect{State: "lib.cursor", Operation: ir.OpPost}})
	eq("修飾して書いた状態の put", callOf(op("remember").Body.Statements[1]).Callee, ir.EffectCallee{Effect: ir.DirectEffect{State: "lib.cursor", Operation: ir.OpPut}})
	eq("op の呼び出し", callOf(op("wired").Body.Statements[0]).Callee, ir.OpCallee{Op: "lib.write"})

	// 引数を持つ throw: 例外の params の名前に対応付ける
	th := op("wired").Body.Statements[3].(ir.ReturnStmt).Expr.(ir.MatchExpr).Match.Arms[0].Body.(ir.ThrowBody)
	eq("throw の例外", th.Exception, "lib.Busy")
	eq("throw の引数", []any{th.Args[0].Param, th.Args[0].Value.(ir.LiteralAtom).Literal, th.Args[1].Param}, []any{"wait", ir.DurationLiteral{Value: 5 * 60 * 1e9}, "note"})

	// 配線の op の reads: 名前で書いた要素と、直に書いた要素。重複は最初の出現に寄せる（read と、その状態は別の要素）
	eq("名前で書いた reads", op("named").Reads, []ir.ReadRef{named("lib.rowById")})
	eq("直に書いた reads", op("direct").Reads, []ir.ReadRef{state("lib.rows")})
	eq("混ぜた reads", op("mixed").Reads, []ir.ReadRef{named("lib.rowById"), state("lib.rows"), named("lib.allLogs")})

	// 配線の op の effects: 直に書いた「状態.操作」と、効果の名前
	eq("直に書いた effects", op("write").Effects, []ir.EffectRef{post})
	eq("配線の effects", op("wired").Effects, []ir.EffectRef{post, touch, ir.DirectEffect{State: "lib.title", Operation: ir.OpPut}})
	eq("配線の reads", op("wired").Reads, []ir.ReadRef{named("lib.rowById")})
	eq("節を書いていない配線", []any{op("forget").Effects, op("forget").Reads}, []any{[]ir.EffectRef(nil), []ir.ReadRef(nil)})

	// デコレータの宣言の節: any state の引数は direct の位置に $param:。read と効果の名前、直に書いた要素も書ける
	scan := p.Lookup("lib.scan").(*ir.Decorator)
	eq("reads の節（引数、read、状態。節は書かれた順に連結）", scan.Reads,
		[]ir.ReadRef{state("$param:via"), state("$param:table"), named("lib.rowById"), state("lib.logs")})
	eq("effects の節（直に書いた効果と、効果の名前）", scan.Effects,
		[]ir.EffectRef{ir.DirectEffect{State: "lib.logs", Operation: ir.OpPost}, touch})

	// 運ばれるもの: $param: を適用の引数で置き換える。省いた省略可能な引数は足さない。重複は最初の出現に寄せる
	eq("carried は Op.reads に足さない", op("scanned").Reads, []ir.ReadRef(nil))
	eq("省いた any state? は足さない", op("scanned").Carried, ir.Carried{
		Reads:   []ir.ReadRef{state("lib.logs"), named("lib.rowById")},
		Effects: []ir.EffectRef{ir.DirectEffect{State: "lib.logs", Operation: ir.OpPost}, touch},
	})
	eq("any state? を渡した", op("scanned2").Carried.Reads,
		[]ir.ReadRef{state("lib.logs"), state("lib.rows"), named("lib.rowById")})
	// 配線の op にも運ばれる（docs/check.md「デコレータが運ぶ reads と effects」）。Op.reads / Op.effects は、書いたもの（空）のまま
	wiredScan := op("scannedWired")
	eq("配線の op の carried", wiredScan.Carried, ir.Carried{
		Reads:   []ir.ReadRef{state("lib.rows"), named("lib.rowById"), state("lib.logs")},
		Effects: []ir.EffectRef{ir.DirectEffect{State: "lib.logs", Operation: ir.OpPost}, touch},
	})
	eq("配線の op の carried は Op.reads / Op.effects に足さない", []any{wiredScan.Body != nil, wiredScan.Reads, wiredScan.Effects},
		[]any{true, []ir.ReadRef(nil), []ir.EffectRef(nil)})
}

// TestNormalizeGolden は正規化の結果を testdata/normalize.golden.json と比べる。更新は go test ./internal/sema -update。
func TestNormalizeGolden(t *testing.T) {
	res, fs := analyzeProject(t, normalizeSource, false)
	noErrors(t, res, fs)
	for _, d := range res.Diags {
		if d.Code != "WIRE-8" { // w は使っていない（意図した warning）
			t.Errorf("想定しない診断: %s %s", d.Code, d.Message)
		}
	}
	got, err := json.MarshalIndent(res.Program, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "normalize.golden.json")
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
		t.Errorf("ゴールデンと違う（-update で更新）\n%s", firstDiff(got, want))
	}
	// 読み戻して同じ Program になる（docs/ir.md「JSON」）。
	var back ir.Program
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatal(err)
	}
	again, _ := json.MarshalIndent(&back, "", "  ")
	if !bytes.Equal(append(again, '\n'), got) {
		t.Error("JSON の往復で変わる")
	}
}

func firstDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := i - 200
			if lo < 0 {
				lo = 0
			}
			return "got:  " + string(a[lo:min(len(a), i+200)]) + "\nwant: " + string(b[lo:min(len(b), i+200)])
		}
	}
	return "length differs"
}

// TestNormalizeExpectations は正規化の各行（docs/ir.md「正規化」）を手で組んだ期待と比べる。
func TestNormalizeExpectations(t *testing.T) {
	res, fs := analyzeProject(t, normalizeSource, false)
	noErrors(t, res, fs)
	p := res.Program
	eq := func(name string, got, want any) {
		t.Helper()
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		if !bytes.Equal(g, w) {
			t.Errorf("%s:\n got  %s\n want %s", name, g, w)
		}
	}

	// namespace a { namespace child {} } と namespace a.child {} は一つ。宣言は出現順（a.hozo が先）。
	child := p.Namespace("a.child")
	eq("a.child の宣言", []string{child.Decls[0].Common().Name, child.Decls[1].Common().Name}, []string{"a.child.Deeper", "a.child.Deep"})
	eq("a の説明は最初のブロックのもの", p.Namespace("a").Description.Summary, "使われない説明（b.hozo の方が後）")

	// T?? → T?
	user := p.Lookup("a.User").(*ir.Model)
	eq("nick の型", user.Fields[3].Type, ir.Optional(ir.String()))

	// model D extends B { override ...; 足すフィールド }
	pub := p.Lookup("a.PublicUser").(*ir.Model)
	eq("extends", pub.Extends, &ir.ModelExtends{Base: "a.User", Removed: []string{"hash"}})
	eq("PublicUser のフィールド", fieldNames(pub), []string{"id", "email", "nick"})
	member := p.Lookup("a.Member").(*ir.Model)
	admin := p.Lookup("a.Admin").(*ir.Model)
	eq("Admin の extends", admin.Extends, &ir.ModelExtends{Base: "a.Member", Removed: []string{"level"}, Overridden: []string{"id", "name"}})
	eq("Admin のフィールド（元の並びのまま、足したものは末尾）", fieldNames(admin), []string{"id", "name", "since", "role"})
	eq("override した型（差し替え）", admin.Fields[0].Type, ir.Ref("a.Email"))
	eq("受け継いだデコレータ", len(admin.Fields[0].Decorators), 1)
	eq("override に足したデコレータ", len(admin.Fields[1].Decorators), 1)
	// 説明: override したフィールドは override に書いたもの（なければなし）。していないフィールドは受け継ぐ
	eq("override に説明がなければ説明なし", admin.Fields[0].Description, (*ir.Doc)(nil))
	eq("override に書いた説明", admin.Fields[1].Description.Summary, "新しい名前")
	eq("元の説明は変わらない", []string{member.Fields[0].Description.Summary, member.Fields[1].Description.Summary}, []string{"識別子", "名前"})
	eq("受け継いだままのフィールドの位置は元のもの", admin.Fields[2].Span, member.Fields[3].Span)
	if admin.Fields[0].Span == member.Fields[0].Span {
		t.Errorf("override したフィールドの位置は override のもの: %+v", admin.Fields[0].Span)
	}
	eq("元の model は変わらない", fieldNames(member), []string{"id", "name", "level", "since"})

	// 形の語は型の別の書き方（どの型の位置でも）
	shapes := p.Lookup("a.Shapes").(*ir.Model)
	eq("list<T> / nullable<T> / single<T>", []ir.Type{shapes.Fields[0].Type, shapes.Fields[1].Type, shapes.Fields[2].Type, shapes.Fields[3].Type, shapes.Fields[4].Type},
		[]ir.Type{ir.Array(ir.String()), ir.Optional(ir.Int()), ir.Ref("a.User"), ir.Optional(ir.String()), ir.Optional(ir.Array(ir.Ref("a.User")))})
	// state s: T は type に正規化した型を置き、shape と element をそこから決める
	state := func(name string) []any {
		st := p.Lookup(name).(*ir.State)
		return []any{st.Type, st.Shape, st.Element}
	}
	eq("state users", state("a.users"), []any{ir.Array(ir.Ref("a.User")), ir.StateList, ir.Ref("a.User")})
	eq("state rows（T[]）", state("a.rows"), []any{ir.Array(ir.Ref("a.User")), ir.StateList, ir.Ref("a.User")})
	eq("state token", state("a.token"), []any{ir.Optional(ir.String()), ir.StateNullable, ir.String()})
	eq("state title（single<T>）", state("a.title"), []any{ir.String(), ir.StateSingle, ir.String()})
	eq("state plain（T）", state("a.plain"), []any{ir.String(), ir.StateSingle, ir.String()})
	eq("state maybe（single<T?> は nullable<T>）", state("a.maybe"), []any{ir.Optional(ir.String()), ir.StateNullable, ir.String()})

	// 例外の引数、ユニオンのアームのその場の model（持ち主の直後）
	eq("Invalid の引数", p.Lookup("a.Invalid").(*ir.Exception).Params[0].Type, ir.String())
	eq("Wrapped の引数", []ir.Type{p.Lookup("a.Wrapped").(*ir.Exception).Params[0].Type, p.Lookup("a.Wrapped").(*ir.Exception).Params[1].Type},
		[]ir.Type{ir.Ref("a.User"), ir.Optional(ir.Time())})
	page := p.Lookup("a.Page").(*ir.Union)
	eq("Page のアーム", []ir.Type{page.Arms[0].Type, page.Arms[1].Type}, []ir.Type{ir.Ref("a.Page.Ready"), ir.Ref("a.User")})
	eq("Ready の説明", page.Arms[0].Description.Summary, "準備できた")
	eq("Ready は Page の直後", declAfter(p, "a.Page"), "a.Page.Ready")

	// ユニオンのメンバーにユニオン → 展開
	eq("AllErrors のメンバー", memberNames(p.Lookup("a.AllErrors").(*ir.Union)), []string{"a.Unauthorized", "a.Invalid", "a.Wrapped"})

	// 二つに分かれる分類も、両側が名前の付いたケース（docs/check.md「閉じた分類」）
	cond := p.Lookup("a.IsActive").(*ir.Classify)
	eq("二つのケース", []any{len(cond.Cases), cond.Cases[0].Name, cond.Cases[1].Name}, []any{2, "Active", "Inactive"})
	kind := p.Lookup("a.Kind").(*ir.Classify)
	eq("ケースの説明", []any{len(kind.Cases), kind.Cases[2].Description.Summary}, []any{3, "それ以外"})
	eq("as のないケースは as: []、述語で決まる", []any{len(kind.Cases[0].As), kind.DecidedBy, cond.DecidedBy}, []any{0, ir.DecidedByPredicate, ir.DecidedByPredicate})

	// op の分類、ケースの X as T、型で決まる分類
	found := p.Lookup("a.Found").(*ir.Classify)
	eq("op の分類の対象", found.Subject, ir.OpSubject{Op: "a.find2", Type: ir.Optional(ir.Ref("a.User"))})
	eq("ケースの as（op）", found.Cases[0].As, []ir.CaseTyping{{Type: ir.Ref("a.User"), By: ir.NarrowByPresence, Span: found.Cases[0].As[0].Span}})
	eq("型で決まる分類", found.DecidedBy, ir.DecidedByType)
	session := p.Lookup("a.Session").(*ir.Classify)
	eq("ケースの as（状態）", session.Cases[0].As, []ir.CaseTyping{{State: "a.token", Type: ir.String(), By: ir.NarrowByPresence, Span: session.Cases[0].As[0].Span}})
	eq("状態一つの、型で決まる分類", session.DecidedBy, ir.DecidedByType)

	// デコレータの宣言: targets の既定、external、alias
	get := p.Lookup("a.get").(*ir.Decorator)
	eq("targets の既定", get.Targets, []ir.DecoratorTarget{ir.TargetOp})
	eq("external", get.External, true)
	alias := p.Lookup("a.unavailable").(*ir.Decorator)
	eq("alias", alias.Alias, &ir.DecoratorAlias{Target: "a.fallback", Args: []ir.Arg{
		{Param: "error", Value: ir.RefsValue{Targets: []string{"a.Unauthorized", "a.Invalid", "a.Wrapped"}}},
		{Param: "view", Value: ir.RefValue{Target: "a.User"}},
	}})
	cache := p.Lookup("a.cache").(*ir.Decorator)
	eq("省略可能な引数", cache.Params[1], ir.Param{Name: "stale", Type: ir.TypeParam{Types: []ir.Type{ir.Duration()}}, Optional: true})
	eq("int | float の引数", p.Lookup("a.tagged").(*ir.Decorator).Params[0].Type, ir.TypeParam{Types: []ir.Type{ir.String(), ir.Int()}})

	// op: void、複数行の throws、effects のユニオン、Doc
	eq("returns の省略", p.Lookup("a.noop").(*ir.Op).Returns, nil)
	eq("void", p.Lookup("a.void2").(*ir.Op).Returns, nil)
	find := p.Lookup("a.find").(*ir.Op)
	eq("throws の節", find.Throws, []ir.ThrowsClause{
		{Exceptions: []string{"a.Unauthorized"}, Relations: []ir.Relation{{Kind: ir.Prevents}}},
		{Exceptions: []string{"a.Invalid"}},
	})
	eq("Doc", find.Description, &ir.Doc{
		Summary: "ユーザーを探す。",
		Body:    "ユーザーを探す。\n\n詳しい説明。",
		Params:  []ir.DocParam{{Name: "id", Text: "識別子"}},
		Throws:  []ir.DocThrows{{Exception: "a.Unauthorized", Text: "認証がない"}},
		See:     []string{"a.User"},
	})
	leaf := p.Lookup("a.leaf").(*ir.Op)
	eq("葉は effects も reads も持たない", []any{leaf.Effects, leaf.Reads, leaf.Carried}, []any{[]ir.EffectRef(nil), []ir.ReadRef(nil), ir.Carried{}})
	eq("throws のユニオンと keeps effects", leaf.Throws[0], ir.ThrowsClause{
		Exceptions: []string{"a.Unauthorized", "a.Invalid", "a.Wrapped"},
		Relations:  []ir.Relation{{Kind: ir.Keeps}},
	})
	// export、reads、never
	find2 := p.Lookup("a.find2").(*ir.Op)
	eq("export", []bool{find2.Exported, find.Exported}, []bool{true, false})
	eq("reads（複数の節、重複は最初の出現に寄せる）", find2.Reads, ir.StateReads("a.users", "a.token"))
	eq("reads を書いていなければ空", find.Reads, []ir.ReadRef(nil))
	eq("never", p.Lookup("a.gone").(*ir.Op).Returns, ir.Never())
	// 効果のユニオンを書けるのは、節を書ける配線の op（葉は STATE-5）
	eq("effects のユニオン", p.Lookup("a.both").(*ir.Op).Effects, ir.NamedEffects("a.add", "a.set"))
	eq("Signature は零値", find.Signature, ir.Signature{})

	// 適用: 名前付きの展開、位置→名前、$param:、Duration
	wire := p.Lookup("a.wire").(*ir.Op)
	eq("名前付きデコレータの展開", wire.Decorators[0], ir.Application{
		Decorator: "a.fallback", Via: "a.unavailable", Args: alias.Alias.Args, Span: wire.Decorators[0].Span,
	})
	eq("位置の引数と $param:", wire.Decorators[1].Args, []ir.Arg{
		{Param: "key", Value: ir.ListValue{Items: []ir.ArgValue{
			ir.LiteralValue{Literal: ir.StringLiteral{Value: "users"}},
			ir.RefValue{Target: "$param:id"},
		}}},
		{Param: "stale", Value: ir.LiteralValue{Literal: ir.DurationLiteral{Value: 5 * 60 * 1e9}}},
	})

	// 配線: { id } の展開、位置の引数、bound は空、match
	stmts := wire.Body.Statements
	call := stmts[1].(ir.CallStmt).Call
	eq("呼び出しの引数", call.Args[0].Param, "in")
	rec := call.Args[0].Value.(ir.RecordAtom)
	eq("{ id } の展開", rec.Fields[0], ir.RecordField{Name: "id", Value: ir.NameAtom{Path: []string{"id"}, Span: rec.Fields[0].Value.(ir.NameAtom).Span}})
	eq("bound は check が埋める", call.Bound, []ir.BoundPosition(nil))
	m := stmts[2].(ir.ValStmt).Expr.(ir.MatchExpr).Match
	// else のアームの cases は、それが受けるケース（ほかのアームが並べなかったもの。docs/ir.md「配線」）
	eq("match", []any{m.Classify, m.Arms[0].Cases, m.Arms[0].Else, m.Arms[1].Cases, m.Arms[1].Else, m.Arms[1].Body}, []any{"a.Kind", []string{"A", "B"}, false, []string{"C"}, true, ir.NoneBody{}})
	if !reflect.DeepEqual(stmts[3].(ir.ReturnStmt).Expr.(ir.AtomExpr).Atom.(ir.NameAtom).Path, []string{"u"}) {
		t.Errorf("return u のパス")
	}
	// 型を書かないケースの絞り込みは case(C, Ks, X)。受けるケースが一つの else のアームは、そのケースのアームと同じ
	eq("narrowed（型を書かないケース）", m.Arms[0].Narrowed, []ir.Narrowing{{Type: ir.Case("a.Kind", []string{"A", "B"}, ir.Ref("a.User"))}})
	eq("narrowed（else、ケース一つ）", m.Arms[1].Narrowed, []ir.Narrowing{{Type: ir.Case("a.Kind", []string{"C"}, ir.Ref("a.User"))}})

	// アームの本体の throw、as で絞り込んだ型
	m2 := p.Lookup("a.wire2").(*ir.Op).Body.Statements[1].(ir.ReturnStmt).Expr.(ir.MatchExpr).Match
	eq("as のケースのアームでは、対象は書いた型", m2.Arms[0].Narrowed, []ir.Narrowing{{Type: ir.Ref("a.User")}})
	eq("throw", m2.Arms[1].Body, ir.ThrowBody{Exception: "a.Unauthorized", Span: m2.Arms[1].Body.(ir.ThrowBody).Span})
	// 状態の名前は Atom の state。標準の分類の対象も value。アームの本体の値は atom、none は none
	m3 := p.Lookup("a.wire3").(*ir.Op).Body.Statements[0].(ir.ReturnStmt).Expr.(ir.MatchExpr).Match
	subject := m3.Subject.(ir.ValueSubject).Atom.(ir.StateAtom)
	eq("match token by nullable の対象", []any{subject.State, subject.Path, m3.Classify}, []any{"a.token", []string(nil), "nullable"})
	eq("HasItems だけのアームで T", m3.Arms[0].Narrowed, []ir.Narrowing{{Type: ir.String()}})
	body := m3.Arms[0].Body.(ir.AtomExpr).Atom.(ir.StateAtom)
	eq("アームの本体の値", []any{body.State, len(body.Path)}, []any{"a.token", 0})
	eq("アームの本体の none", m3.Arms[1].Body, ir.NoneBody{})
	eq("Empty のアームは外のまま", m3.Arms[1].Narrowed, []ir.Narrowing(nil))
	// 修飾して書いた状態も同じ
	ret := p.Lookup("a.wire4").(*ir.Op).Body.Statements[0].(ir.ReturnStmt).Expr.(ir.AtomExpr).Atom.(ir.StateAtom)
	eq("a.title", []any{ret.State, len(ret.Path)}, []any{"a.title", 0})
	// 状態の分類での match: 絞り込まれる状態ごとに narrowed
	m5 := p.Lookup("a.wire5").(*ir.Op).Body.Statements[0].(ir.ReturnStmt).Expr.(ir.MatchExpr).Match
	eq("match by C の対象", m5.Subject, ir.ClassifyOwnedSubject{})
	eq("状態の分類の narrowed", m5.Arms[0].Narrowed, []ir.Narrowing{{State: "a.token", Type: ir.String()}})
}

func fieldNames(m *ir.Model) []string {
	var out []string
	for _, f := range m.Fields {
		out = append(out, f.Name)
	}
	return out
}

func memberNames(u *ir.Union) []string {
	var out []string
	for _, m := range u.Members {
		out = append(out, m.Target)
	}
	return out
}

// declAfter は fqn の直後の宣言の名前。
func declAfter(p *ir.Program, fqn string) string {
	for _, ns := range p.Namespaces {
		for i, d := range ns.Decls {
			if d.Common().Name == fqn && i+1 < len(ns.Decls) {
				return ns.Decls[i+1].Common().Name
			}
		}
	}
	return ""
}
