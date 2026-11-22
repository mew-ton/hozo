package govalid

import (
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは docs/plugin-go-valid.md「Validate を持つ型」の規則の各行を、手で組み立てた小さな IR で確かめる
// （docs/testing.md「単体テスト」の `plugin/govalid`）。

func decl(fqn string, decorators ...string) ir.DeclBase {
	b := ir.DeclBase{Name: fqn}
	for _, d := range decorators {
		b.Decorators = append(b.Decorators, ir.Application{Decorator: d})
	}
	return b
}

func scalar(fqn string, base ir.Type, decorators ...string) *ir.Scalar {
	return &ir.Scalar{DeclBase: decl(fqn, decorators...), Base: base}
}

func model(fqn string, fields ...ir.Field) *ir.Model {
	return &ir.Model{DeclBase: decl(fqn), Fields: fields}
}

// index は、インデックスの model（docs/types.md「インデックスの model」）。
func index(fqn string, key, value ir.Type) *ir.Model {
	return &ir.Model{DeclBase: decl(fqn), Index: &ir.ModelIndex{Name: "name", Key: key, Value: value}}
}

func field(name string, t ir.Type, decorators ...string) ir.Field {
	f := ir.Field{Name: name, Type: t}
	for _, d := range decorators {
		f.Decorators = append(f.Decorators, ir.Application{Decorator: d})
	}
	return f
}

func program(namespaces map[string][]ir.Decl, order ...string) *ir.Program {
	p := &ir.Program{}
	for _, name := range order {
		p.Namespaces = append(p.Namespaces, ir.Namespace{Name: name, Decls: namespaces[name]})
	}
	return p
}

func TestHasValidate(t *testing.T) {
	p := program(map[string][]ir.Decl{
		"type": {
			// 1: @valid.* が付いた scalar
			scalar("type.Email", ir.String(), "valid.format"),
			// 利用者が valid に足した制約も、@valid.* の一つ（使われていれば、go-valid が GOVALID-5 を返す）
			scalar("type.Code", ir.String(), "valid.checkDigit"),
			// valid の語彙でないデコレータは数えない（名前空間 valid.sub、valid で始まるだけの名前空間、グローバル）
			scalar("type.Plain", ir.String(), "db.key", "valid.sub.x", "validx.min", "valid"),
			// 2: 元の型が Validate を持つ scalar。その先も
			scalar("type.WorkEmail", ir.Ref("type.Email")),
			scalar("type.BossEmail", ir.Ref("type.WorkEmail")),
			scalar("type.PlainId", ir.Ref("type.Plain")),
		},
		"app": {
			// 3: @valid.* が付いたフィールドを持つ model
			model("app.Named", field("name", ir.String(), "valid.length")),
			model("app.Marked", field("id", ir.String(), "db.key")),
			// 4: Validate を持つ型をフィールドの型に持つ model。optional、array、インデックスの model の値を通して見る
			model("app.Direct", field("email", ir.Ref("type.Email"))),
			model("app.Opt", field("email", ir.Optional(ir.Ref("type.Email")))),
			model("app.List", field("emails", ir.Array(ir.Ref("type.Email")))),
			model("app.Map", field("emails", ir.Ref("app.Emails"))),
			model("app.Deep", field("x", ir.Optional(ir.Array(ir.Ref("app.NamedLists"))))),
			// インデックスのキーは見ない（値だけ）
			model("app.Keyed", field("byEmail", ir.Ref("app.ByEmail"))),
			// インデックスの model そのものは、Validate を持たない（値を回すのは、それをフィールドの型に持つ model）
			index("app.Emails", ir.String(), ir.Ref("type.Email")),
			index("app.NamedLists", ir.String(), ir.Array(ir.Optional(ir.Ref("app.Named")))),
			index("app.ByEmail", ir.Ref("type.Email"), ir.String()),
			// インデックスの model だけを辿って、自分に戻る（値を持たない循環）: 止まる
			index("app.Tree", ir.String(), ir.Ref("app.Tree")),
			model("app.Forest", field("root", ir.Ref("app.Tree"))),
			model("app.NoCheck", field("id", ir.Ref("type.PlainId")), field("at", ir.Datetime()), field("m", ir.Ref("app.Marked"))),
			// 当たるものがなくなるまで繰り返す: 順に辿る model と、互いを参照する model
			model("app.Outer", field("mid", ir.Ref("app.Mid"))),
			model("app.Mid", field("inner", ir.Array(ir.Ref("app.Named")))),
			model("app.Ping", field("pong", ir.Optional(ir.Ref("app.Pong")))),
			model("app.Pong", field("ping", ir.Optional(ir.Ref("app.Ping"))), field("n", ir.Int(), "valid.min")),
			model("app.Loop", field("self", ir.Optional(ir.Ref("app.Loop")))),
			// union と exception は Validate を持たない。
			// 5: union をフィールドの型（通して見たもの）に持つ model は、アームの型のどれかが、Validate を持つ型への ref なら、持つ
			&ir.Union{DeclBase: decl("app.Contact"), Variant: ir.UnionArms, Arms: []ir.UnionArm{{Name: "Mail", Type: ir.Ref("type.Email")}}},
			model("app.Holder", field("contact", ir.Ref("app.Contact"))),
			model("app.Holders", field("contacts", ir.Array(ir.Optional(ir.Ref("app.Contact"))))),
			// アームの型が、ref でない（配列）か、Validate を持たない型なら、それだけでは持たない
			&ir.Union{DeclBase: decl("app.Loose"), Variant: ir.UnionArms, Arms: []ir.UnionArm{
				{Name: "Many", Type: ir.Array(ir.Ref("type.Email"))}, {Name: "Plain", Type: ir.Ref("type.Plain")}}},
			model("app.LooseHolder", field("loose", ir.Ref("app.Loose"))),
			// 例外は、引数の型が Validate を持つ型でも（scalar、model、包まれた型）、持たない
			&ir.Exception{DeclBase: decl("app.Bad"), Params: []ir.OpParam{{Name: "value", Type: ir.Ref("app.Named")}}},
			&ir.Exception{DeclBase: decl("app.Bad3"), Params: []ir.OpParam{
				{Name: "email", Type: ir.Ref("type.Email")}, {Name: "named", Type: ir.Optional(ir.Ref("app.Named"))}, {Name: "list", Type: ir.Array(ir.Ref("app.Direct"))}}},
			// その場で書いた model（合成した model。ユニオンのアーム）は、普通の model
			&ir.Model{DeclBase: decl("app.Page.Ready"), SynthesizedFrom: "app.Page", Fields: []ir.Field{field("n", ir.Int(), "valid.max")}},
			&ir.Exception{DeclBase: decl("app.Bad2"), Params: []ir.OpParam{{Name: "n", Type: ir.Int()}}},
			// View が見る側のために合成した model（synthesizedFrom が op の FQN）は、Go の型にならないので持たない
			&ir.Op{DeclBase: decl("app.send")},
			&ir.Model{DeclBase: decl("app.send$in"), SynthesizedFrom: "app.send", Fields: []ir.Field{field("email", ir.Ref("type.Email"))}},
		},
	}, "type", "app")

	want := map[string]bool{
		"type.Email": true, "type.Code": true, "type.Plain": false,
		"type.WorkEmail": true, "type.BossEmail": true, "type.PlainId": false,
		"app.Named": true, "app.Marked": false,
		"app.Direct": true, "app.Opt": true, "app.List": true, "app.Map": true, "app.Deep": true,
		"app.Keyed": false, "app.NoCheck": false,
		"app.Emails": false, "app.NamedLists": false, "app.ByEmail": false, "app.Tree": false, "app.Forest": false,
		"app.Outer": true, "app.Mid": true, "app.Ping": true, "app.Pong": true, "app.Loop": false,
		"app.Contact": false, "app.Holder": true, "app.Holders": true, "app.Loose": false, "app.LooseHolder": false, "app.Bad": false,
		"app.Page.Ready": true, "app.Bad2": false, "app.Bad3": false,
		"app.send": false, "app.send$in": false,
		"app.Nowhere": false, "": false,
	}
	for fqn, w := range want {
		if got := HasValidate(p, fqn); got != w {
			t.Errorf("HasValidate(%q) = %v, want %v", fqn, got, w)
		}
	}
	if HasValidate(nil, "type.Email") {
		t.Error("Program が nil なのに true")
	}
}

// 継承した model は、展開済みのフィールド（受け継いだデコレータを含む）で判定する。
func TestHasValidateExtends(t *testing.T) {
	p := program(map[string][]ir.Decl{"app": {
		model("app.User", field("name", ir.String(), "valid.length"), field("hash", ir.String())),
		&ir.Model{DeclBase: decl("app.Public"), Extends: &ir.ModelExtends{Base: "app.User", Removed: []string{"hash"}},
			Fields: []ir.Field{field("name", ir.String(), "valid.length")}},
		// 制約の付いたフィールドをなくした model は、もう持たない
		&ir.Model{DeclBase: decl("app.Bare"), Extends: &ir.ModelExtends{Base: "app.User", Removed: []string{"name"}},
			Fields: []ir.Field{field("hash", ir.String())}},
	}}, "app")
	for fqn, w := range map[string]bool{"app.User": true, "app.Public": true, "app.Bare": false} {
		if got := HasValidate(p, fqn); got != w {
			t.Errorf("HasValidate(%q) = %v, want %v", fqn, got, w)
		}
	}
}

// docs/plugin-go-valid.md「出力」の GOVALID-3 と、それを前提にする相乗り（GOHTTP-11、CLI-9）が共有する規則: 別の名前空間の型の
// Validate を呼べるのは、その名前空間の型の置き場（docs/config.md「型の置き場」）の単位が go-valid を並べているときだけ。
func TestUnlisted(t *testing.T) {
	conf := func(index int, base string, transpilers ...string) ir.NamespaceConfig {
		return ir.NamespaceConfig{Index: index, Language: "go", Base: base, Transpilers: transpilers}
	}
	view := &ir.View{
		Viewer: "app",
		Config: conf(3, "go", "go", "go-valid", "go-http"),
		Namespaces: map[string][]ir.NamespaceConfig{
			"app": {conf(3, "go", "go", "go-valid", "go-http")},
			// 置き場は、基底が同じ最初の単位（2 番目。1 番目は基底が違う）
			"plain":   {conf(1, "ts", "ts", "go-valid"), conf(2, "go", "go"), conf(4, "go", "go", "go-valid")},
			"checked": {conf(2, "go", "go", "go-valid")},
			// 自分の単位も出力しているなら、置き場は自分の単位（設定の順で前に、並べていない単位があっても）
			"own": {conf(2, "go", "go"), conf(3, "go", "go", "go-valid", "go-http")},
			// 基底の違う単位だけ: 置き場がない
			"other": {conf(1, "ts", "ts")},
		},
	}
	for ns, want := range map[string]int{"plain": 2, "checked": 0, "own": 0, "other": 0, "nowhere": 0, "app": 0} {
		unit, ok := Unlisted(view, ns)
		if unit != want || ok != (want != 0) {
			t.Errorf("Unlisted(%s) = %d, %v, want %d", ns, unit, ok, want)
		}
	}
	// 見る側の名前空間は、View.config で見る
	view.Config = conf(3, "go", "go")
	if unit, ok := Unlisted(view, "app"); unit != 3 || !ok {
		t.Errorf("Unlisted(viewer) = %d, %v, want 3", unit, ok)
	}
	if unit, ok := Unlisted(nil, "app"); unit != 0 || ok {
		t.Errorf("Unlisted(nil) = %d, %v", unit, ok)
	}
}
