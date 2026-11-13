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
  model PublicUser is User omit hash;

  state users: list<User>;
  state token: nullable<string>;
  effect add on users.post;
  effect set on token.put;

  exception Unauthorized;
  exception Invalid: { field: string; };
  exception Wrapped: User;
  union Errors { Unauthorized; Invalid; }
  union AllErrors { Errors; Wrapped; }
  union Effects { add; set; }

  union Page {
    /** 準備できた */
    Ready: { name: string; };
    Missing: User;
  }

  condition IsActive of User;
  classify Kind of User { A; /** B の説明 */ B; /** それ以外 */ default; }

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
  op find(id: string): User throws Unauthorized prevents effects throws Invalid;
  op noop();
  op void2(): void;
  op leaf(u: User): PublicUser effects add throws AllErrors keeps effects;
  op setToken() effects set;
  op both(u: User): PublicUser effects Effects throws AllErrors {
    setToken();
    return leaf(u);
  }
  op take(in: User): void;
  op fromIn(in: User): User;

  @unavailable
  @cache(["users", id], 5m)
  op wire(id: string): PublicUser {
    const u = find(id);
    take({ id, email: u.email, hash: "x" });
    const w = match u by Kind {
      A | B => fromIn(u);
      default => none;
    };
    return u;
  }
}
`,
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

	// is X omit
	pub := p.Lookup("a.PublicUser").(*ir.Model)
	eq("derivedFrom", pub.DerivedFrom, &ir.DerivedFrom{Base: "a.User", Omit: []string{"hash"}})
	eq("PublicUser のフィールド", fieldNames(pub), []string{"id", "email", "nick"})

	// 例外のその場の model、ユニオンのアームのその場の model（持ち主の直後）
	eq("Invalid の値", p.Lookup("a.Invalid").(*ir.Exception).Value, ir.Ref("a.Invalid.Value"))
	eq("Invalid.Value", p.Lookup("a.Invalid.Value").(*ir.Model).SynthesizedFrom, "a.Invalid")
	eq("Wrapped の値はそのまま ref", p.Lookup("a.Wrapped").(*ir.Exception).Value, ir.Ref("a.User"))
	page := p.Lookup("a.Page").(*ir.Union)
	eq("Page のアーム", []ir.Type{page.Arms[0].Type, page.Arms[1].Type}, []ir.Type{ir.Ref("a.Page.Ready"), ir.Ref("a.User")})
	eq("Ready の説明", page.Arms[0].Description.Summary, "準備できた")
	eq("Ready は Page の直後", declAfter(p, "a.Page"), "a.Page.Ready")

	// ユニオンのメンバーにユニオン → 展開
	eq("AllErrors のメンバー", memberNames(p.Lookup("a.AllErrors").(*ir.Union)), []string{"a.Unauthorized", "a.Invalid", "a.Wrapped"})

	// condition → classify
	cond := p.Lookup("a.IsActive").(*ir.Classify)
	eq("condition", []any{cond.FromCondition, cond.HasDefault, cond.Cases[0].Name}, []any{true, true, "IsActive"})
	kind := p.Lookup("a.Kind").(*ir.Classify)
	eq("default の説明", kind.DefaultDescription.Summary, "それ以外")

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
	eq("葉の effects", leaf.Effects, []string{"a.add"})
	eq("throws のユニオンと keeps effects", leaf.Throws[0], ir.ThrowsClause{
		Exceptions: []string{"a.Unauthorized", "a.Invalid", "a.Wrapped"},
		Relations:  []ir.Relation{{Kind: ir.Keeps}},
	})
	// 効果のユニオンを書けるのは、二つ以上の効果を持てる配線の op（葉は EFF-3。docs/decisions.md S8）
	eq("effects のユニオン", p.Lookup("a.both").(*ir.Op).Effects, []string{"a.add", "a.set"})
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
	m := stmts[2].(ir.ConstStmt).Expr.(ir.MatchExpr).Match
	eq("match", []any{m.Classify, m.Arms[0].Cases, m.Arms[1].Cases, m.Arms[1].Body}, []any{"a.Kind", ir.ArmCases{Cases: []string{"A", "B"}}, ir.DefaultArm(), ir.NoneBody{}})
	if !reflect.DeepEqual(stmts[3].(ir.ReturnStmt).Expr.(ir.AtomExpr).Atom.(ir.NameAtom).Path, []string{"u"}) {
		t.Errorf("return u のパス")
	}
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
