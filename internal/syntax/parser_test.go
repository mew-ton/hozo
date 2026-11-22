package syntax

import (
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/internal/syntax/token"
)

func parseSrc(t *testing.T, src string) (*ast.File, []diag.Diagnostic) {
	t.Helper()
	fs := source.NewFileSet()
	id := fs.Add("t.hozo", []byte(src))
	file, diags := Parse(fs, id)
	for _, d := range diags {
		if (d.Code != "SYNTAX" && d.Code != "LIT-1") || d.Severity != diag.Error || d.Span == nil {
			t.Errorf("診断の形が違う: %+v", d)
		}
	}
	return file, diags
}

// mustParse は誤りなく読めることを確かめ、構文木を返す。
func mustParse(t *testing.T, src string) *ast.File {
	t.Helper()
	file, diags := parseSrc(t, src)
	for _, d := range diags {
		t.Errorf("予期しない診断: %s", d.Message)
	}
	return file
}

// mustFail は SYNTAX が一つ以上出て、最初のメッセージに want を含むことを確かめる。
func mustFail(t *testing.T, src, want string) []diag.Diagnostic {
	t.Helper()
	_, diags := parseSrc(t, src)
	if len(diags) == 0 {
		t.Fatalf("誤りが出ない: %s", src)
	}
	if !strings.Contains(diags[0].Message, want) {
		t.Errorf("message = %q, want contains %q", diags[0].Message, want)
	}
	return diags
}

// firstDecl は一つだけの宣言を T として返す。
func firstDecl[T ast.Decl](t *testing.T, src string) T {
	t.Helper()
	file := mustParse(t, src)
	if len(file.Decls) != 1 {
		t.Fatalf("宣言の数 = %d, want 1", len(file.Decls))
	}
	d, ok := file.Decls[0].(T)
	if !ok {
		t.Fatalf("宣言の型 = %T", file.Decls[0])
	}
	return d
}

func typeString(t ast.Type) string {
	switch x := t.(type) {
	case *ast.CoreType:
		return x.Name
	case *ast.ShapeType:
		return x.Shape + "<" + typeString(x.Elem) + ">"
	case *ast.NamedType:
		return x.Name.String()
	case *ast.OnType:
		names := make([]string, len(x.Cases))
		for i, c := range x.Cases {
			names[i] = c.String()
		}
		return typeString(x.Of) + " on " + strings.Join(names, " | ")
	case *ast.ArrayType:
		return typeString(x.Elem) + "[]"
	case *ast.OptionalType:
		return typeString(x.Elem) + "?"
	}
	return "?"
}

func names(qns []ast.QualifiedName) string {
	var s []string
	for _, q := range qns {
		s = append(s, q.String())
	}
	return strings.Join(s, "|")
}

// --- File / Decl（docs/grammar.md「ファイルと宣言」） ---

func TestFile(t *testing.T) {
	file := mustParse(t, "")
	if len(file.Decls) != 0 {
		t.Errorf("Decls = %d", len(file.Decls))
	}
	file = mustParse(t, "// コメントだけ\n/* ブロック */")
	if len(file.Decls) != 0 || file.Span.End != source.Offset(len("// コメントだけ\n/* ブロック */")) {
		t.Errorf("File = %+v", file)
	}
	file = mustParse(t, "model A {}\nscalar B extends string;\nbind x to a.*;")
	if len(file.Decls) != 3 {
		t.Errorf("Decls = %d", len(file.Decls))
	}
}

func TestDeclDocAndDecorators(t *testing.T) {
	m := firstDecl[*ast.ModelDecl](t, "/** 説明 */\n@db @http.status(401)\nmodel A {}")
	if m.Doc == nil || m.Doc.Body != "説明" {
		t.Errorf("Doc = %+v", m.Doc)
	}
	if len(m.Decorators) != 2 || m.Decorators[0].Name.String() != "db" || m.Decorators[1].Name.String() != "http.status" {
		t.Errorf("Decorators = %+v", m.Decorators)
	}
	// 宣言の Span は DocComment を含まず、デコレータから "}" まで。
	if m.Span.Start != source.Offset(len("/** 説明 */\n")) || m.Span.End != source.Offset(len("/** 説明 */\n@db @http.status(401)\nmodel A {}")) {
		t.Errorf("Span = %+v", m.Span)
	}
}

func TestDeclReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"dangling doc at EOF", "/** x */", "付く先のないドキュメンテーションコメント"},
		{"dangling doc before }", "namespace a { /** x */ }", "付く先のない"},
		{"two docs", "/** a */ /** b */ model A {}", "付く先のない"},
		{"doc after decorator", "@db /** x */ model A {}", "デコレータの前に置く"},
		{"doc on bind", "/** x */ bind a to b.*;", "bind に説明は付けられない"},
		{"decorator on namespace", "@db namespace a {}", "namespace にデコレータは付けられない"},
		{"decorator on bind", "@db bind a to b.*;", "bind にデコレータは付けられない"},
		{"not a decl", "foo;", "宣言が必要"},
		{"stray close brace", "model A {} }", "対応する「{」のない「}」"},
		{"entry on model", "@entry model A {}", "@entry はデコレータの宣言の直前にだけ書ける"},
		{"entry with args", "@entry() decorator fn x();", "@entry は引数を取らない"},
		{"entry before other decorator", "@entry @db decorator fn x();", "@entry はデコレータの宣言の直前にだけ書ける"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// --- namespace ---

func TestNamespace(t *testing.T) {
	ns := firstDecl[*ast.NamespaceDecl](t, "/** d */ namespace server.admin {\n model A {}\n namespace x { scalar S extends int; }\n}")
	if ns.Name.String() != "server.admin" || len(ns.Name.Segments) != 2 {
		t.Errorf("Name = %+v", ns.Name)
	}
	if ns.Doc == nil || ns.Doc.Body != "d" {
		t.Errorf("Doc = %+v", ns.Doc)
	}
	if len(ns.Decls) != 2 {
		t.Fatalf("Decls = %d", len(ns.Decls))
	}
	inner, ok := ns.Decls[1].(*ast.NamespaceDecl)
	if !ok || inner.Name.String() != "x" || len(inner.Decls) != 1 {
		t.Errorf("inner = %+v", ns.Decls[1])
	}
	mustFail(t, "namespace a { model A {}", "namespace a の「}」がない")
	mustFail(t, "namespace { }", "名前が必要")
	// デコレータは名前空間に付けられない（docs/grammar.md「ファイルと宣言」「決めたこと」）。入れ子でも同じ。
	mustFail(t, "/** d */ @ctx.bounded(\"注文\") namespace shop {}", "namespace にデコレータは付けられない")
	mustFail(t, "namespace shop { @inner namespace admin {} }", "namespace にデコレータは付けられない")
	mustFail(t, "@hint namespace a {}", "@hint はデコレータの宣言の直前にだけ書ける")
}

// --- デコレータの適用 ---

func TestDecoratorApplication(t *testing.T) {
	m := firstDecl[*ast.ModelDecl](t, `@a @b() @c(1, "s", true, 5m, -2, 1.5, x.y) @d(name: "n", items: ["o", q], set: A | b.C, nested: [[1], []]) model M {}`)
	if len(m.Decorators) != 4 {
		t.Fatalf("Decorators = %d", len(m.Decorators))
	}
	if m.Decorators[0].Args != nil {
		t.Errorf("@a の Args は nil のはず")
	}
	if m.Decorators[1].Args == nil || len(m.Decorators[1].Args) != 0 {
		t.Errorf("@b() の Args は空のはず: %v", m.Decorators[1].Args)
	}
	c := m.Decorators[2]
	if len(c.Args) != 7 {
		t.Fatalf("@c の Args = %d", len(c.Args))
	}
	kinds := []ast.LiteralKind{ast.IntLit, ast.StringLit, ast.BoolLit, ast.DurationLit, ast.IntLit, ast.FloatLit}
	for i, k := range kinds {
		lit := c.Args[i].Value.Terms[0].Literal
		if c.Args[i].Name != nil || lit == nil || lit.Kind != k {
			t.Errorf("@c の引数 %d = %+v", i, c.Args[i].Value.Terms[0])
		}
	}
	if c.Args[4].Value.Terms[0].Literal.Int != -2 || c.Args[3].Value.Terms[0].Literal.Duration != 300_000_000_000 {
		t.Errorf("値が違う: %+v %+v", c.Args[4].Value.Terms[0].Literal, c.Args[3].Value.Terms[0].Literal)
	}
	if n := c.Args[6].Value.Terms[0].Name; n == nil || n.String() != "x.y" {
		t.Errorf("名前の引数 = %+v", c.Args[6].Value.Terms[0])
	}
	d := m.Decorators[3]
	if len(d.Args) != 4 || d.Args[0].Name.Name != "name" || d.Args[1].Name.Name != "items" {
		t.Fatalf("@d の Args = %+v", d.Args)
	}
	list := d.Args[1].Value
	if !list.IsList || len(list.List) != 2 || list.List[1].Terms[0].Name.String() != "q" {
		t.Errorf("list = %+v", list)
	}
	set := d.Args[2].Value
	if set.IsList || len(set.Terms) != 2 || set.Terms[1].Name.String() != "b.C" {
		t.Errorf("set = %+v", set)
	}
	nested := d.Args[3].Value
	if !nested.IsList || len(nested.List) != 2 || !nested.List[0].IsList || len(nested.List[1].List) != 0 {
		t.Errorf("nested = %+v", nested)
	}
}

func TestDecoratorApplicationReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"positional after named", `@a(x: 1, 2) model M {}`, "名前で渡す引数の後ろに位置で渡す引数は書けない"},
		{"missing paren", `@a(1 model M {}`, "「)」が必要"},
		{"bad value", `@a(;) model M {}`, "引数の値（リテラルか名前）が必要"},
		{"no name", `@ (1) model M {}`, "デコレータの名前が必要"},
		// 予約語はデコレータの名前に書ける（DecoratorRef）ので、`@model` はデコレータ model の適用で、宣言が続かない
		{"keyword is the name", `@ model M {}`, "宣言が必要だが識別子 Mがある"},
		{"keyword in the middle", `@a.op.b model M {}`, "op は予約語で、名前には使えない"},
		{"keyword head", `@list.x model M {}`, "宣言が必要だが「.」がある"},
		{"unclosed list", `@a([1) model M {}`, "「]」が必要"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// --- model ---

func TestModel(t *testing.T) {
	m := firstDecl[*ast.ModelDecl](t, "model User {\n  /** 鍵 */\n  @db.key id: UserId;\n  name: string;\n  @http.default(1) page: PosInt;\n  field: string;\n  tags: string[]?;\n}")
	if m.Name.Name != "User" || m.Body == nil || m.Extends != nil {
		t.Fatalf("Model = %+v", m)
	}
	f := m.Body.Fields
	if len(f) != 5 {
		t.Fatalf("Fields = %d", len(f))
	}
	if f[0].Doc == nil || f[0].Doc.Body != "鍵" || len(f[0].Decorators) != 1 || f[0].Name.Name != "id" || typeString(f[0].Type) != "UserId" {
		t.Errorf("Field 0 = %+v", f[0])
	}
	if f[1].Name.Name != "name" || typeString(f[1].Type) != "string" || len(f[1].Decorators) != 0 {
		t.Errorf("Field 1 = %+v", f[1])
	}
	// 値がないときに入れる値は、境界のデコレータが言う（フィールドの既定値は書けない）
	if len(f[2].Decorators) != 1 || f[2].Decorators[0].Name.String() != "http.default" || f[2].Decorators[0].Args[0].Value.Terms[0].Literal.Int != 1 {
		t.Errorf("Field 2 = %+v", f[2])
	}
	if f[3].Name.Name != "field" {
		t.Errorf("文脈キーワード field がフィールド名にならない: %+v", f[3].Name)
	}
	if typeString(f[4].Type) != "string[]?" {
		t.Errorf("Field 4 type = %s", typeString(f[4].Type))
	}
	// Field の Span は DocComment を含まず、デコレータから ";" まで。
	if f[0].Span.Start != f[0].Decorators[0].Span.Start || f[0].Span.End <= f[0].Type.NodeSpan().End {
		t.Errorf("Field 0 Span = %+v", f[0].Span)
	}
	empty := firstDecl[*ast.ModelDecl](t, "model E {}")
	if empty.Body == nil || len(empty.Body.Fields) != 0 {
		t.Errorf("empty = %+v", empty.Body)
	}
}

// 継承した model（docs/grammar.md「model」の ModelDecl の extends と Override）。
func TestModelExtends(t *testing.T) {
	m := firstDecl[*ast.ModelDecl](t, `model PublicUser extends server.User {
  override hash: never;
  /** 鍵 */
  @db.key override id;
  override status: StatusOrPending;
  @db.unique override email: Email;
  nickname: string;
  override: string;
  override override: never;
}`)
	if m.Extends == nil || m.Extends.String() != "server.User" || m.Body == nil {
		t.Fatalf("Model = %+v", m)
	}
	o := m.Body.Overrides
	if len(o) != 5 || len(m.Body.Fields) != 2 {
		t.Fatalf("Overrides = %d, Fields = %d", len(o), len(m.Body.Fields))
	}
	if o[0].Name.Name != "hash" || !o[0].Never || o[0].Type != nil || o[0].NeverSpan.End-o[0].NeverSpan.Start != source.Offset(len("never")) {
		t.Errorf("Override 0 = %+v", o[0])
	}
	if o[1].Name.Name != "id" || o[1].Never || o[1].Type != nil || len(o[1].Decorators) != 1 || o[1].Doc == nil || o[1].Doc.Body != "鍵" {
		t.Errorf("Override 1 = %+v", o[1])
	}
	if o[2].Name.Name != "status" || typeString(o[2].Type) != "StatusOrPending" || o[2].Never {
		t.Errorf("Override 2 = %+v", o[2])
	}
	if o[3].Name.Name != "email" || typeString(o[3].Type) != "Email" || len(o[3].Decorators) != 1 {
		t.Errorf("Override 3 = %+v", o[3])
	}
	// メンバーの頭の override は、直後が ":" ならフィールドの名前。override override は、その名前のフィールドへの Override。
	if m.Body.Fields[0].Name.Name != "nickname" || m.Body.Fields[1].Name.Name != "override" {
		t.Errorf("Fields = %+v", m.Body.Fields)
	}
	if o[4].Name.Name != "override" || !o[4].Never {
		t.Errorf("Override 4 = %+v", o[4])
	}
	// Override の Span は DocComment を含まず、デコレータ（なければ override）から ";" まで。
	if o[1].Span.Start != o[1].Decorators[0].Span.Start || o[1].Span.End <= o[1].Name.Span.End {
		t.Errorf("Override 1 Span = %+v", o[1].Span)
	}
	// 本体が空でもよい。extends のない model の Override も構文としては読める（検査 TYPE-10）。
	empty := firstDecl[*ast.ModelDecl](t, "model Admin extends User { }")
	if empty.Extends == nil || empty.Body == nil || len(empty.Body.Fields) != 0 || len(empty.Body.Overrides) != 0 {
		t.Errorf("empty = %+v", empty)
	}
	plain := firstDecl[*ast.ModelDecl](t, "model A { override f; }")
	if plain.Extends != nil || len(plain.Body.Overrides) != 1 {
		t.Errorf("plain = %+v", plain)
	}
	// その場で書く model の本体も同じ Member。
	u := firstDecl[*ast.UnionDecl](t, "union U { Ready: { override f: never; }; }")
	if len(u.Members[0].Type.Body.Overrides) != 1 {
		t.Errorf("arm body = %+v", u.Members[0].Type.Body)
	}
}

func TestModelReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"is omit", "model A is B omit f;", "model A extends 元の model { override フィールド: never; } と書く"},
		{"is without omit", "model A is B;", "model A is ... omit ... は書けない"},
		{"extends without body", "model A extends B;", "「{」が必要だが「;」がある"},
		{"extends without name", "model A extends { }", "名前が必要"},
		// フィールドの既定値はなくした書き方（docs/grammar.md「model」「決めたこと」）
		{"field default", "model A { page: int = 1; }", "フィールド page に既定値は書けない"},
		{"field default string", "model A { name: string = \"n\"; }", "境界のデコレータで言う（@http.default(値)、@cli.default(値)"},
		{"field default on reserved name", "model A { default: int = 1; }", "フィールド default に既定値は書けない"},
		{"override default", "model A extends B { override f: int = 1; }", "フィールド f に既定値は書けない"},
		{"override default without type", "model A extends B { override f = 1; }", "フィールド f に既定値は書けない"},
		{"override without name", "model A extends B { override; }", "フィールド名が必要"},
		{"override qualified", "model A extends B { override a.b; }", "修飾しない識別子"},
		{"never field type", "model A { f: never; }", "never は op の戻り値の型と、override のなくす印にだけ書ける"},
		{"override never optional", "model A extends B { override f: never?; }", "「;」が必要だが「?」がある"},
		{"optional on name", "model A { x?: T; }", "「:」が必要だが「?」がある"},
		{"missing semicolon", "model A { x: T }", "「;」が必要だが「}」がある"},
		{"missing type", "model A { x: ; }", "型が必要"},
		{"field default, not a literal", "model A { x: int = y; }", "フィールド x に既定値は書けない"},
		{"no body", "model A;", "「{」か「extends」が必要"},
		{"qualified field name", "model A { a.b: string; }", "修飾しない識別子"},
		{"dangling doc in body", "model A { /** x */ }", "付く先のない"},
		{"hint on field", "model A { @hint x: int; }", "@hint はデコレータの宣言の直前にだけ書ける"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// TestModelIndex は、インデックスの model（docs/grammar.md「model」の Index）: `[<キーの名前>: <キーの型>]: <値の型>;`。
func TestModelIndex(t *testing.T) {
	m := firstDecl[*ast.ModelDecl](t, "model Attrs { [name: string]: string[]; }")
	idx := m.Body.Index
	if idx == nil || idx.Name.Name != "name" || typeString(idx.Key) != "string" || typeString(idx.Value) != "string[]" || len(m.Body.Fields) != 0 {
		t.Fatalf("Index = %+v, Fields = %+v", idx, m.Body.Fields)
	}
	src := "model Attrs { [name: string]: string[]; }"
	if idx.Span.Start != source.Offset(strings.Index(src, "[")) || idx.Span.End != source.Offset(strings.Index(src, ";")+1) {
		t.Errorf("Index の位置 = %+v", idx.Span)
	}
	// キーの型は scalar でも、値の型は、どの型でもよい（検査は、意味の解析: TYPE-11）
	m = firstDecl[*ast.ModelDecl](t, "/** 在庫 */ @mark model Stock { [sku: type.Sku]: nullable<Count>; }")
	if idx := m.Body.Index; idx == nil || idx.Name.Name != "sku" || typeString(idx.Key) != "type.Sku" || typeString(idx.Value) != "nullable<Count>" || m.Doc == nil || len(m.Decorators) != 1 {
		t.Errorf("Index = %+v", m.Body.Index)
	}
	// ふつうの model は、Index を持たない
	if m := firstDecl[*ast.ModelDecl](t, "model A { x: string; }"); m.Body.Index != nil {
		t.Errorf("Index = %+v, want nil", m.Body.Index)
	}
	rejects := []struct{ name, src, want string }{
		{"index after field", "model A { x: int; [k: string]: int; }", "インデックスと、フィールドは、一つの model に混ぜられない"},
		{"field after index", "model A { [k: string]: int; x: int; }", "インデックスと、フィールドは、一つの model に混ぜられない"},
		{"two indexes", "model A { [k: string]: int; [j: string]: int; }", "インデックスは、一つの model に一つだけ"},
		{"decorator on index", "model A { @mark [k: string]: int; }", "インデックスには、デコレータも、説明も、付けられない"},
		{"doc on index", "model A { /** x */ [k: string]: int; }", "インデックスには、デコレータも、説明も、付けられない"},
		{"index with extends", "model A extends B { [k: string]: int; }", "extends を持つ model に、インデックスは書けない"},
		{"index in inline model", "union U { A: { [k: string]: int; }; }", "その場で書く model に、インデックスは書けない"},
		{"missing key name", "model A { [string]: int; }", "string は予約語で、識別子には使えない"},
		{"missing key type", "model A { [k]: int; }", "「:」が必要だが「]」がある"},
		{"missing value type", "model A { [k: string]; }", "「:」が必要だが「;」がある"},
		{"missing semicolon", "model A { [k: string]: int }", "「;」が必要だが「}」がある"},
	}
	for _, tt := range rejects {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// --- scalar ---

func TestScalar(t *testing.T) {
	s := firstDecl[*ast.ScalarDecl](t, "/** d */ @valid.min(1) scalar PosInt extends int;")
	if s.Name.Name != "PosInt" || typeString(s.Base) != "int" || s.Doc == nil || len(s.Decorators) != 1 {
		t.Errorf("Scalar = %+v", s)
	}
	mustFail(t, "scalar A: int;", "「extends」が必要")
	mustFail(t, "scalar A extends int", "「;」が必要")
}

// --- union ---

func TestUnion(t *testing.T) {
	u := firstDecl[*ast.UnionDecl](t, `union OrderPage {
  /** a */ Ready: { userName: string; orders: OrderRow[]; };
  Unavailable: OrdersUnavailable;
  server.Unauthorized;
  transitionToLogin;
}`)
	if u.Name.Name != "OrderPage" || len(u.Members) != 4 {
		t.Fatalf("Union = %+v", u)
	}
	m0 := u.Members[0]
	if m0.Doc == nil || m0.Arm == nil || m0.Arm.Name != "Ready" || m0.Type == nil || m0.Type.Body == nil || len(m0.Type.Body.Fields) != 2 || m0.Target != nil {
		t.Errorf("Member 0 = %+v", m0)
	}
	m1 := u.Members[1]
	if m1.Arm == nil || m1.Type == nil || m1.Type.Type == nil || typeString(m1.Type.Type) != "OrdersUnavailable" {
		t.Errorf("Member 1 = %+v", m1)
	}
	m2 := u.Members[2]
	if m2.Arm != nil || m2.Target == nil || m2.Target.String() != "server.Unauthorized" {
		t.Errorf("Member 2 = %+v", m2)
	}
	if u.Members[3].Target == nil || u.Members[3].Target.String() != "transitionToLogin" {
		t.Errorf("Member 3 = %+v", u.Members[3])
	}
	mustFail(t, "union U { Ready: ; }", "型が必要")
	mustFail(t, "union U { 1; }", "ユニオンのメンバー")
	mustFail(t, "union U { A: { x: int } ; }", "「;」が必要だが「}」がある")
}

// --- state ---

func TestState(t *testing.T) {
	s := firstDecl[*ast.StateDecl](t, "@db state users: list<User>;")
	if s.Name.Name != "users" || typeString(s.Type) != "list<User>" || len(s.Decorators) != 1 {
		t.Errorf("State = %+v", s)
	}
	// 形の語は型の別の書き方で、状態の型は Type（docs/grammar.md「state」）。形は検査が型から決める。
	tests := []struct{ src, want string }{
		{"state query: single<Query>;", "single<Query>"},
		{"state token: nullable<server.Token>;", "nullable<server.Token>"},
		{"state token: single<Token?>;", "single<Token?>"},
		{"state token: Token?;", "Token?"},
		{"state title: string;", "string"},
		{"state users: User[];", "User[]"},
		{"state nested: list<list<User>>?;", "list<list<User>>?"},
	}
	for _, tt := range tests {
		s = firstDecl[*ast.StateDecl](t, tt.src)
		if got := typeString(s.Type); got != tt.want {
			t.Errorf("%s: Type = %s, want %s", tt.src, got, tt.want)
		}
	}
	mustFail(t, "state users list<User>;", "「:」が必要")
	mustFail(t, "state users: list<User;", "「>」が必要")
	mustFail(t, "state users: list;", "「<」が必要")
	mustFail(t, "state users: ;", "型が必要")
	mustFail(t, "state users: never;", "never は op の戻り値の型と")
}

// --- exception / effect ---

func TestException(t *testing.T) {
	e := firstDecl[*ast.ExceptionDecl](t, "@http.status(401) exception Unauthorized;")
	if e.Name.Name != "Unauthorized" || len(e.Params) != 0 || len(e.Decorators) != 1 {
		t.Errorf("Exception = %+v", e)
	}
	// 引数は例外が運ぶ値（docs/grammar.md「exception / effect / read」）
	e = firstDecl[*ast.ExceptionDecl](t, "/** 待つ */ exception LimitExceeded(wait: duration, items: Violation[]);")
	if len(e.Params) != 2 || e.Params[0].Name.Name != "wait" || typeString(e.Params[0].Type) != "duration" ||
		e.Params[1].Name.Name != "items" || typeString(e.Params[1].Type) != "Violation[]" || e.Doc == nil {
		t.Errorf("Exception = %+v", e)
	}
	mustFail(t, "exception X", "「;」が必要")
	// なくした書き方と、括弧だけの書き方は、今の書き方をメッセージで示す
	mustFail(t, "exception ClientError: Violations;", "例外が運ぶ値は引数で書く（exception ClientError(value: T);")
	mustFail(t, "exception X: { items: Violation[]; };", "exception X: T という書き方はなくなった")
	mustFail(t, "exception X();", "引数のない例外は括弧を書かない（exception X; と書く")
	mustFail(t, "exception X(a);", "「:」が必要")
	// 括弧だけの誤りの後ろの宣言は残る
	file, diags := parseSrc(t, "exception X();\nexception Y;")
	if len(diags) != 1 || len(file.Decls) != 2 {
		t.Errorf("回復: decls = %d、diags = %v", len(file.Decls), diags)
	}
}

func TestEffect(t *testing.T) {
	for _, tt := range []struct {
		src  string
		op   ast.StateOp
		stat string
	}{
		{"effect fn createSession(): effects sessions.post;", ast.Post, "sessions"},
		{"effect fn a(): effects location.put;", ast.Put, "location"},
		{"effect fn b(): effects server.users.delete;", ast.Delete, "server.users"},
	} {
		e := firstDecl[*ast.EffectDecl](t, tt.src)
		if e.On == nil || e.Transition != nil {
			t.Fatalf("%s: On = %+v, Transition = %+v", tt.src, e.On, e.Transition)
		}
		if e.On.Op != tt.op || e.On.State.String() != tt.stat {
			t.Errorf("%s: On = %+v", tt.src, e.On)
		}
		if e.On.OpSpan.End != e.On.Span.End || e.On.State.Span.Start != e.On.Span.Start || e.On.State.Span.End >= e.On.OpSpan.Start {
			t.Errorf("%s: Span = %+v", tt.src, e.On)
		}
	}
	mustFail(t, "effect fn a(): effects sessions.get;", "状態の操作は post / put / delete のどれかで、get ではない")
	mustFail(t, "effect fn a(): effects sessions;", "「状態.操作」")
	mustFail(t, "effect fn a() sessions.post;", "「: effects 宛先」（effect fn a(…): effects 状態.操作; と書く）が必要")
	mustFail(t, "effect fn a(): sessions.post;", "「: effects 宛先」")
	mustFail(t, "effect fn a(): effects sessions.post, sessions.put;", "effect fn a の宛先は、一つだけ")
	// 投げる例外（docs/grammar.md「exception / effect / read」の EffectThrows）: effects の前に、一つだけ。関係の節は書けない
	for _, tt := range []struct {
		src  string
		want string
		void bool
	}{
		{"effect fn a(): throws E effects sessions.post;", "E", false},
		{"effect fn a(): throws E, store.F effects sessions.put as Stage.B;", "E,store.F", false},
		{"effect fn a(): void throws E effects Checkout as Shopping -> Ordered;", "E", true},
	} {
		e := firstDecl[*ast.EffectDecl](t, tt.src)
		if e.Throws == nil || len(e.Throws.Relations) != 0 {
			t.Fatalf("%s: Throws = %+v", tt.src, e.Throws)
		}
		var names []string
		for _, x := range e.Throws.Exceptions {
			names = append(names, x.String())
		}
		if got := strings.Join(names, ","); got != tt.want {
			t.Errorf("%s: Throws = %s, want %s", tt.src, got, tt.want)
		}
		if (e.Return != nil && e.Return.Void) != tt.void {
			t.Errorf("%s: Return = %+v", tt.src, e.Return)
		}
	}
	if e := firstDecl[*ast.EffectDecl](t, "effect fn a(): effects sessions.post;"); e.Throws != nil {
		t.Errorf("throws のない効果の Throws = %+v", e.Throws)
	}
	mustFail(t, "effect fn a(): throws E prevents effects effects sessions.post;", "effect fn a の throws に、prevents の節は書けない（効果が例外を投げたとき、その効果は、いつも、起きていない")
	mustFail(t, "effect fn a(): throws E keeps effects effects sessions.post;", "effect fn a の throws に、keeps の節は書けない")
	mustFail(t, "effect fn a(): throws E throws F effects sessions.post;", "effect fn a の throws は、一つだけ（例外は「,」で並べる: throws A, B")
	mustFail(t, "effect fn a(): effects sessions.post throws E;", "effect fn a の throws は、effects の前に書く")
	mustFail(t, "effect fn a(): catches E recovers effects sessions.post;", "effect fn a に catches の節は書けない（効果に書ける節は、throws と effects")
	mustFail(t, "effect fn a() throws E effects sessions.post;", "「)」の後ろに、戻り値か節を書くなら、「:」で始める（effect fn a(…): throws … と書く")
	// 「)」の後ろに、戻り値か節を書くなら、「:」で始める（docs/grammar.md「関数の宣言」）
	mustFail(t, "effect fn a() effects sessions.post;", "「)」の後ろに、戻り値か節を書くなら、「:」で始める（effect fn a(…): effects … と書く")
	// なくした書き方（on）は、新しい書き方をメッセージで示す
	mustFail(t, "effect fn a() on sessions.post;", "effect fn a(…) on … という書き方はなくなった。状態との関係は、effects で書く（effect fn a(…): … effects … と書く")
	mustFail(t, "effect fn a(): void on sessions.post;", "effect fn a(…) on … という書き方はなくなった")
	// 引数（効果が受け取る値）。括弧は引数がなくても書く
	e := firstDecl[*ast.EffectDecl](t, "effect fn createOrder(order: Order, at: datetime): effects orders.post;")
	if len(e.Params) != 2 || e.Params[0].Name.Name != "order" || typeString(e.Params[1].Type) != "datetime" {
		t.Errorf("Params = %+v", e.Params)
	}
	// なくした書き方（括弧なし）は、今の書き方をメッセージで示す
	mustFail(t, "effect fn a: effects sessions.post;", "effect fn a の名前の後ろに、引数の括弧が要る（引数がなくても effect fn a(): effects … と書く")
	// 戻り値（docs/grammar.md「関数の宣言」）: 書いたものを、そのまま持つ。void のほかを弾くのは、意味の解析（TYPE-18）
	if e := firstDecl[*ast.EffectDecl](t, "effect fn a(): effects sessions.post;"); e.Return != nil {
		t.Errorf("省いた戻り値: Return = %+v", e.Return)
	}
	if e := firstDecl[*ast.EffectDecl](t, "effect fn a(): void effects sessions.post;"); e.Return == nil || !e.Return.Void {
		t.Errorf("void の戻り値: Return = %+v", e.Return)
	}
	if e := firstDecl[*ast.EffectDecl](t, "effect fn a(): T effects sessions.post;"); e.Return == nil || typeString(e.Return.Type) != "T" {
		t.Errorf("型の戻り値: Return = %+v", e.Return)
	}
	// なくした書き方（頭に fn のない宣言）は、新しい書き方をメッセージで示す
	mustFail(t, "effect a(): effects sessions.post;", "effect a(…) という書き方はなくなった。関数の宣言の頭は fn で、種類は、その前に書く（effect fn a(…) と書く")
	mustFail(t, "effect effects sessions.post;", "fn（effect fn 名前(…) と書く）が必要")
}

func idents(ids []ast.Ident) string {
	var s []string
	for _, id := range ids {
		s = append(s, id.Name)
	}
	return strings.Join(s, "|")
}

// 遷移（docs/grammar.md「exception / effect / read」の Transition と CaseSet）。
func TestEffectTransition(t *testing.T) {
	for _, tt := range []struct {
		src      string
		of       string
		from, to string // "any" か、ケース名を "|" でつないだもの（idents の書き方）
	}{
		{"effect fn placeOrder(): effects Checkout as Shopping -> Ordered;", "Checkout", "Shopping", "Ordered"},
		{"effect fn clearCart(): effects carts as HasItems -> Empty;", "carts", "HasItems", "Empty"},
		{"effect fn reset(): effects Checkout as any -> Shopping;", "Checkout", "any", "Shopping"},
		{"effect fn touch(): effects Checkout as Shopping, Ordered -> any;", "Checkout", "Shopping|Ordered", "any"},
		{"effect fn a(): effects server.Checkout as A,B->C,D;", "server.Checkout", "A|B", "C|D"},
	} {
		e := firstDecl[*ast.EffectDecl](t, tt.src)
		if e.On != nil || e.Transition == nil {
			t.Fatalf("%s: On = %+v, Transition = %+v", tt.src, e.On, e.Transition)
		}
		tr := e.Transition
		set := func(c *ast.CaseSet) string {
			if c.Any {
				if len(c.Cases) != 0 {
					t.Errorf("%s: any なのに Cases = %+v", tt.src, c.Cases)
				}
				return "any"
			}
			return idents(c.Cases)
		}
		if tr.Of.String() != tt.of || set(tr.From) != tt.from || set(tr.To) != tt.to {
			t.Errorf("%s: Transition = %s: %s -> %s", tt.src, tr.Of.String(), set(tr.From), set(tr.To))
		}
		// Transition の Span は名前から後のケースの終わりまで（";" は含まない）。
		if tr.Span.Start != tr.Of.Span.Start || tr.Span.End != tr.To.Span.End || tr.Span.End+1 != e.Span.End {
			t.Errorf("%s: Span = %+v（effect %+v）", tt.src, tr.Span, e.Span)
		}
	}
	// 最後の要素が post / put / delete の名前は、いつも「状態.操作」として読む（docs/grammar.md「exception / effect / read」）:
	// 後ろの as は、行の遷移で、ケースは、分類の名前から書く。
	mustFail(t, "effect fn a(): effects carts.post as A;", "行の遷移のケースは、分類の名前から書く（<分類>.A")
	// 最後の要素が操作でなく、直後が as でもなければ、「状態.操作」として読んで、誤りを示す
	mustFail(t, "effect fn a(): effects carts.clear;", "状態の操作は post / put / delete のどれかで、clear ではない")
	// なくした書き方（宛先の後ろの「:」）は、新しい書き方をメッセージで示す
	mustFail(t, "effect fn a(): effects Checkout: A -> B;", "状態の遷移を、宛先の後ろの「:」で書く形はなくなった。as で書く（effects C as A -> B と書く")
	tests := []struct{ name, src, want string }{
		{"missing arrow", "effect fn a(): effects C as A;", "「->」が必要"},
		{"fat arrow", "effect fn a(): effects C as A => B;", "「->」が必要"},
		{"missing to", "effect fn a(): effects C as A -> ;", "遷移のケース（any かケース名）が必要"},
		{"missing from", "effect fn a(): effects C as -> B;", "遷移のケース（any かケース名）が必要"},
		{"qualified case", "effect fn a(): effects C as x.A -> B;", "遷移のケース（any かケース名）が必要"},
		{"any in a set", "effect fn a(): effects C as A, any -> B;", "any は予約語"},
		{"two transitions", "effect fn a(): effects C as A -> B, D as E -> F;", "「;」が必要"},
		{"no target", "effect fn a(): effects as A -> B;", "「状態.操作」"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// 行の遷移（docs/grammar.md「exception / effect / read」の RowClause）: as の後ろは、効果の後のケース一つか、else。
// 前のケースは、行の引数の型（絞り込んだ型）が言う。
func TestEffectRowClause(t *testing.T) {
	rowString := func(r *ast.RowClause) string {
		switch {
		case r.After.Else && r.After.Case != nil:
			return "else なのに Case がある"
		case r.After.Else:
			return "else"
		case r.After.Case != nil:
			return r.After.Case.String()
		}
		return "?"
	}
	for _, tt := range []struct{ src, on, param, row string }{
		{"effect fn accept(order: Order): effects orders.post as Stage.Accepted;", "orders.post", "Order", "Stage.Accepted"},
		{"effect fn start(order: Order on Stage.Accepted): effects orders.put as Stage.InProduction;", "orders.put", "Order on Stage.Accepted", "Stage.InProduction"},
		{"effect fn cancel(order: Order on Stage.Draft | Stage.Accepted): effects orders.put as Stage.Cancelled;", "orders.put", "Order on Stage.Draft | Stage.Accepted", "Stage.Cancelled"},
		{"effect fn force(order: Order): effects orders.put as Stage.Shipped;", "orders.put", "Order", "Stage.Shipped"},
		{"effect fn revise(order: Order on Stage.Accepted): effects orders.put as else;", "orders.put", "Order on Stage.Accepted", "else"},
		{"effect fn patch(order: Order): effects orders.put as else;", "orders.put", "Order", "else"},
		{"effect fn drop(order: Order on ordering.Life.Draft): effects shop.orders.delete as ordering.Life.Absent;", "shop.orders.delete", "Order on ordering.Life.Draft", "ordering.Life.Absent"},
	} {
		e := firstDecl[*ast.EffectDecl](t, tt.src)
		if e.On == nil || e.Transition != nil || e.Row == nil {
			t.Fatalf("%s: On = %+v, Transition = %+v, Row = %+v", tt.src, e.On, e.Transition, e.Row)
		}
		if got := e.On.State.String() + "." + e.On.Op.String(); got != tt.on {
			t.Errorf("%s: On = %s", tt.src, got)
		}
		if got := typeString(e.Params[0].Type); got != tt.param {
			t.Errorf("%s: 引数の型 = %s, want %s", tt.src, got, tt.param)
		}
		if got := rowString(e.Row); got != tt.row {
			t.Errorf("%s: Row = %s, want %s", tt.src, got, tt.row)
		}
		// RowClause の Span は as から後の終わりまで（";" は含まない）。AsSpan は as の語
		r := e.Row
		if r.Span.Start != r.AsSpan.Start || r.AsSpan.End-r.AsSpan.Start != 2 || r.Span.End != r.After.Span.End || r.Span.End+1 != e.Span.End {
			t.Errorf("%s: Span = %+v（as %+v、後 %+v、effect %+v）", tt.src, r.Span, r.AsSpan, r.After.Span, e.Span)
		}
	}
	// as を書かない効果は Row が nil
	if e := firstDecl[*ast.EffectDecl](t, "effect fn rename(order: Order on Stage.Draft): effects orders.put;"); e.Row != nil {
		t.Errorf("Row = %+v", e.Row)
	}
	for _, tt := range []struct{ name, src, want string }{
		// なくした書き方は、新しい書き方をメッセージで示す
		{"arrow", "effect fn a(o: O): effects s.put as C.A -> C.B;", "行の遷移の矢印（as <前> -> <後>）はなくなった。行の遷移の、前のケースは、行の引数の型で言い（order: Order on C.A | C.B）"},
		{"arrow to else", "effect fn a(o: O): effects s.put as C.A -> else;", "行の遷移の矢印（as <前> -> <後>）はなくなった"},
		{"any", "effect fn a(o: O): effects s.post as any;", "行の遷移に any は書けない"},
		{"any before arrow", "effect fn a(o: O): effects s.put as any -> C.B;", "行の遷移に any は書けない"},
		{"two cases", "effect fn a(o: O): effects s.post as C.A, C.B;", "行の遷移の as に書けるケースは、一つだけ（効果の後のケース）"},
		{"two cases with pipe", "effect fn a(o: O): effects s.put as C.A | C.B;", "行の遷移の as に書けるケースは、一つだけ（効果の後のケース）"},
		{"bare case", "effect fn a(o: O): effects s.put as Shipped;", "行の遷移のケースは、分類の名前から書く（<分類>.Shipped"},
		{"missing everything", "effect fn a(o: O): effects s.put as ;", "行の遷移のケース（<分類>.<ケース> か、else）が必要"},
		{"after a transition", "effect fn a(): effects Checkout as A -> B as C.A;", "「;」が必要"},
		{"reserved word as a case", "effect fn a(o: O): effects s.put as C.else;", "else は予約語で、名前には使えない"},
	} {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// --- classify ---

func TestClassify(t *testing.T) {
	c := firstDecl[*ast.ClassifyDecl](t, "classify CampaignKind of server.Item {\n /** s */ Spring;\n Summer;\n /** d */ Other;\n}")
	if c.Name.Name != "CampaignKind" || len(c.Of) != 1 || typeString(c.Of[0]) != "server.Item" || len(c.Cases) != 3 {
		t.Fatalf("Classify = %+v", c)
	}
	if c.Cases[0].Doc == nil || c.Cases[0].Name.Name != "Spring" {
		t.Errorf("Case 0 = %+v", c.Cases[0])
	}
	if c.Cases[2].Name.Name != "Other" || c.Cases[2].Doc == nil || c.Cases[2].Doc.Body != "d" {
		t.Errorf("Case 2 = %+v", c.Cases[2])
	}
	// ケースの型（CaseTyping = Type "as" ( Type | "else" )）。
	c = firstDecl[*ast.ClassifyDecl](t, `classify Registration of findUser {
  /** 登録済み */
  Registered: findUser as type.User;
  Unknown;
}`)
	if as := c.Cases[0].As; len(as) != 1 || typeString(as[0].Subject) != "findUser" || typeString(as[0].Type) != "type.User" || c.Cases[0].Doc == nil {
		t.Errorf("Case 0 = %+v", c.Cases[0])
	}
	if len(c.Cases[1].As) != 0 || c.Cases[1].Name.Name != "Unknown" {
		t.Errorf("Case 1 = %+v", c.Cases[1])
	}
	c = firstDecl[*ast.ClassifyDecl](t, "classify Ready of token, profile { Ready: token as Token, profile as Profile; Half: token as nullable<Token>; Neither; }")
	if as := c.Cases[0].As; len(as) != 2 || typeString(as[1].Subject) != "profile" || typeString(as[1].Type) != "Profile" {
		t.Errorf("Case 0 = %+v", c.Cases[0])
	}
	if as := c.Cases[1].As; len(as) != 1 || typeString(as[0].Type) != "nullable<Token>" {
		t.Errorf("Case 1 = %+v", c.Cases[1])
	}
	c = firstDecl[*ast.ClassifyDecl](t, "classify Presence of nullable<User> { Present: nullable<User> as User; }")
	if typeString(c.Of[0]) != "nullable<User>" || typeString(c.Cases[0].As[0].Subject) != "nullable<User>" {
		t.Errorf("Classify = %+v", c)
	}
	// 残りのケース（CaseTyping = Type "as" ( Type | "else" )）。else は型ではなく、Type は nil
	c = firstDecl[*ast.ClassifyDecl](t, "classify Registration of findUser { Registered: findUser as type.User; /** 未登録 */ Unregistered: findUser as else; }")
	if as := c.Cases[1].As; len(as) != 1 || !as[0].Else || as[0].Type != nil || typeString(as[0].Subject) != "findUser" ||
		as[0].ElseSpan.End-as[0].ElseSpan.Start != 4 || c.Cases[1].Doc == nil {
		t.Errorf("Case 1 = %+v", c.Cases[1])
	}
	if as := c.Cases[0].As; len(as) != 1 || as[0].Else || as[0].Type == nil {
		t.Errorf("Case 0 = %+v", c.Cases[0])
	}
	// ない側のケース（CaseTyping = Type "as" ( Type | "else" | "none" )）。none は型ではなく、Type は nil
	c = firstDecl[*ast.ClassifyDecl](t, "classify Life of Order? { Draft: Order? as Order; Placed: Order? as Order; /** ない */ Absent: Order? as none; }")
	if as := c.Cases[2].As; len(as) != 1 || !as[0].None || as[0].Else || as[0].Type != nil || typeString(as[0].Subject) != "Order?" ||
		as[0].NoneSpan.End-as[0].NoneSpan.Start != 4 || c.Cases[2].Doc == nil {
		t.Errorf("Case 2 = %+v", c.Cases[2])
	}
	if as := c.Cases[0].As; len(as) != 1 || as[0].None || as[0].Type == nil {
		t.Errorf("Case 0 = %+v", c.Cases[0])
	}
	// none は予約語: as の後ろの none の後ろに、型の続きは書けない
	mustFail(t, "classify A of T? { X: T? as none?; }", "「;」が必要")
	mustFail(t, "classify A of T? { none: T? as T; }", "none は予約語で、識別子には使えない")
	// else は予約語（docs/grammar.md「予約語」）: 書けるのは as の後ろと、アームの頭と、フィールド名の位置だけ
	mustFail(t, "classify A of T { X: T as else?; }", "「;」が必要")
	mustFail(t, "classify A of T { X: else; }", "型が必要")
	mustFail(t, "classify A of T { else; }", "else は予約語で、識別子には使えない")
	mustFail(t, "model else { id: string; }", "else は予約語で、識別子には使えない")
	mustFail(t, "fn f(m: else);", "型が必要")
	firstDecl[*ast.ModelDecl](t, "model M { else: string; }")
	mustFail(t, "classify A of T { X: T; }", "「as」が必要")
	mustFail(t, "classify A of T { X: as U; }", "型が必要")
	mustFail(t, "classify A of T { X: T as; }", "型が必要")
	mustFail(t, "classify A of T { X: T as U,; }", "型が必要")
	mustFail(t, "classify A of T { x }", "「;」が必要")
	mustFail(t, "classify A T { }", "「of」が必要")
	mustFail(t, "classify A of T { /** x */ }", "付く先のない")
}

func typeStrings(ts []ast.Type) string {
	var s []string
	for _, t := range ts {
		s = append(s, typeString(t))
	}
	return strings.Join(s, ",")
}

// 対象が二つ以上（docs/grammar.md「classify」の ClassifySubject）。型か op か状態かは名前の解決が決める。
func TestClassifySubject(t *testing.T) {
	c := firstDecl[*ast.ClassifyDecl](t, "classify Checkout of carts, orders {\n Shopping;\n Ordered;\n}")
	if typeStrings(c.Of) != "carts,orders" || len(c.Cases) != 2 {
		t.Errorf("Classify = %+v（Of = %s）", c, typeStrings(c.Of))
	}
	c = firstDecl[*ast.ClassifyDecl](t, "classify Session of token { LoggedIn; LoggedOut; }")
	if typeStrings(c.Of) != "token" {
		t.Errorf("Of = %s", typeStrings(c.Of))
	}
	// 構文は Type の並びなので、型と状態が混ざっていても読める。弾くのは検査 CLS-3。
	c = firstDecl[*ast.ClassifyDecl](t, "classify X of server.Item[], carts { A; }")
	if typeStrings(c.Of) != "server.Item[],carts" {
		t.Errorf("Of = %s", typeStrings(c.Of))
	}
	mustFail(t, "classify A of { X; }", "分類の対象（型か op か状態の名前）が必要")
	mustFail(t, "classify A of carts, { X; }", "分類の対象（型か op か状態の名前）が必要")
	mustFail(t, "classify A of carts orders { X; }", "「{」が必要")
	// 形の語は型なので、対象にも書ける。
	c = firstDecl[*ast.ClassifyDecl](t, "classify A of list<T> { X; }")
	if typeStrings(c.Of) != "list<T>" {
		t.Errorf("Of = %s", typeStrings(c.Of))
	}
}

// 分類を閉じたときになくした書き方（docs/grammar.md「classify」「exception / effect / read」「配線」）: 新しい書き方を
// メッセージで示して SYNTAX にする。
func TestRemovedClassifyForms(t *testing.T) {
	for _, tt := range []struct{ name, src, want string }{
		{"default case", "classify A of T { X; default; }", "名前のないケース default は書けない。ケースには名前を付ける"},
		{"default case points to as else", "classify A of T { X: T as U; default; }", "<名前>: <対象> as else;"},
		{"default case with doc", "classify A of T { X; /** d */ default; }", "名前のないケース default は書けない"},
		{"default case with as", "classify A of T { X; default: T as U; }", "名前のないケース default は書けない"},
		{"condition", "condition A of T;", "condition の宣言は書けない。両側に名前を付けた classify で書く"},
		{"condition shows the form", "/** d */ condition A of carts, orders;", "classify C of T { <成り立つ側>; <成り立たない側>; }"},
		{"default arm", "fn a() { match (x by K) { A => f(); default => none; }; }", "default のアームは書けない。受けるケースを名前で並べるか、最後のアームを else => … と書く"},
		{"pipe in arm", "fn a() { match (x by K) { A | B => f(); }; }", "アームのケースの並びの区切りは「,」で、「|」ではない（A, B => … と書く"},
		{"pipe in transition, from", "effect fn e(): effects C as A | B -> any;", "遷移のケースの並びの区切りは「,」で、「|」ではない（effects C as A, B -> any と書く"},
		{"pipe in transition, to", "effect fn e(): effects C as any -> A | B;", "遷移のケースの並びの区切りは「,」で、「|」ではない"},
		{"else arm, then an arm", "fn a() { match (x by K) { A => f(); else => none; B => g(); }; }", "else のアームの後ろに、アームは書けない"},
		{"two else arms", "fn a() { match (x by K) { A => f(); else => none; else => g(); }; }", "else のアームの後ろに、アームは書けない"},
		{"else arm only", "fn a() { match (x by K) { else => f(); }; }", "else のアームは、ケースを名前で並べたアームの後ろに書く"},
		{"else arm first", "fn a() { match (x by K) { else => f(); A => g(); }; }", "else のアームは、ケースを名前で並べたアームの後ろに書く"},
		{"else in a case list", "fn a() { match (x by K) { A, else => f(); }; }", "else は予約語で、識別子には使えない"},
		{"pipe in decorator targets", "decorator d for model | state;", "for の対象の並びの区切りは「,」で、「|」ではない（for model, state, op と書く"},
	} {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
	// condition は予約語ではない（docs/grammar.md「予約語」）: 名前に使える。宣言の頭でだけ、なくした書き方として見分ける
	o := firstDecl[*ast.OpDecl](t, "fn check(condition: string): string { val condition2 = f(condition); return condition2; }")
	if o.Params[0].Name.Name != "condition" {
		t.Errorf("Params = %+v", o.Params)
	}
	// default は予約語のまま（書ける位置は FieldName と DecoratorName だけ）
	mustFail(t, "fn f(default: string);", "default は予約語で、識別子には使えない")
	// なくした区切り "|" は、"," として読み進める: 宣言は残り、ケースは二つとも読める
	file, diags := parseSrc(t, "effect fn e(): effects C as A | B -> any;\nfn a() { match (x by K) { A | B => f(); }; }")
	if len(diags) != 2 || len(file.Decls) != 2 {
		t.Fatalf("diags = %d, Decls = %d", len(diags), len(file.Decls))
	}
	if from := file.Decls[0].(*ast.EffectDecl).Transition.From; idents(from.Cases) != "A|B" {
		t.Errorf("From = %s", idents(from.Cases))
	}
	if arm := file.Decls[1].(*ast.OpDecl).Body.Stmts[0].(*ast.MatchStmt).Match.Arms[0]; idents(arm.Cases) != "A|B" {
		t.Errorf("Arm = %s", idents(arm.Cases))
	}
	// default のアームは、else のアームとして読み進める。else の後ろのアームは捨てる
	file, diags = parseSrc(t, "fn a() { match (x by K) { A => f(); default => none; }; }\nfn b() { match (x by K) { A => f(); else => none; B => g(); }; }")
	if len(diags) != 2 || len(file.Decls) != 2 {
		t.Fatalf("diags = %d, Decls = %d", len(diags), len(file.Decls))
	}
	if arms := file.Decls[0].(*ast.OpDecl).Body.Stmts[0].(*ast.MatchStmt).Match.Arms; len(arms) != 2 || !arms[1].Else {
		t.Errorf("default のアーム: Arms = %+v", arms)
	}
	if arms := file.Decls[1].(*ast.OpDecl).Body.Stmts[0].(*ast.MatchStmt).Match.Arms; len(arms) != 2 || !arms[1].Else {
		t.Errorf("else の後ろのアーム: Arms = %+v", arms)
	}
}

// --- decorator の宣言 ---

func TestDecoratorDecl(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, "decorator fn retry(times: int);")
	if d.Name.Name != "retry" || len(d.Params) != 1 || d.Params[0].Name.Name != "times" || len(d.Params[0].Type.Types) != 1 || len(d.For) != 0 || len(d.Clauses) != 0 || d.Entry || d.Hint || d.Alias != nil {
		t.Errorf("Decorator = %+v", d)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator body for field;")
	if len(d.Params) != 0 || len(d.For) != 1 || d.For[0].Kind != ast.TargetField {
		t.Errorf("Decorator = %+v", d)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator min(value: int | float) for scalar, field, op, state, exception, model;")
	if len(d.Params[0].Type.Types) != 2 || len(d.For) != 6 {
		t.Errorf("Decorator = %+v", d)
	}
	wantTargets := []ast.TargetKind{ast.TargetScalar, ast.TargetField, ast.TargetOp, ast.TargetState, ast.TargetException, ast.TargetModel}
	for i, w := range wantTargets {
		if d.For[i].Kind != w {
			t.Errorf("For[%d] = %s, want %s", i, d.For[i].Kind, w)
		}
	}
	d = firstDecl[*ast.DecoratorDecl](t, "@entry decorator fn get(path: string): outer throws ServerError, NetworkError;")
	if !d.Entry || d.EntrySpan.Start != 0 || d.EntrySpan.End != 6 || d.Hint || len(d.Clauses) != 1 {
		t.Fatalf("Decorator = %+v", d)
	}
	th := d.Clauses[0].(*ast.ThrowsClause)
	if !th.Outer || th.Span.Start != 39 {
		t.Errorf("outer throws = %+v", th)
	}
	if names(th.Exceptions) != "ServerError|NetworkError" || len(th.Relations) != 0 {
		t.Errorf("Throws = %+v", th)
	}
	// Span は @entry から ";" まで。
	if d.Span.Start != 0 {
		t.Errorf("Span = %+v", d.Span)
	}
}

// TestCoreMarks は核の印 CoreMark = "@repeatable" | "@entry" | "@hint"（docs/grammar.md「decorator の宣言」）と、なくした "@external"。
func TestCoreMarks(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, "@repeatable decorator where(column: string, value: string | int | float | boolean) for read;")
	if !d.Repeatable || d.Entry || d.Hint || d.RepeatableSpan.Start != 0 || d.RepeatableSpan.End != source.Offset(len("@repeatable")) || d.Span.Start != 0 {
		t.Fatalf("Decorator = %+v", d)
	}
	// 二つを並べてよく、順は問わない
	for _, src := range []string{"@entry @repeatable decorator fn x(): outer throws E;", "@repeatable @entry decorator fn x(): outer throws E;"} {
		d = firstDecl[*ast.DecoratorDecl](t, src)
		if !d.Repeatable || !d.Entry || d.Hint || len(d.Decorators) != 0 {
			t.Errorf("%s: Decorator = %+v", src, d)
		}
	}
	// 印のない宣言
	if d = firstDecl[*ast.DecoratorDecl](t, "decorator fn x();"); d.Repeatable || d.Entry || d.Hint {
		t.Errorf("Decorator = %+v", d)
	}
	// @hint。for の形にも、付けられる
	if d = firstDecl[*ast.DecoratorDecl](t, "@hint decorator tag(name: string) for model;"); !d.Hint || d.HintSpan.End != 5 || d.Entry {
		t.Errorf("Decorator = %+v", d)
	}
	// throws と outer throws を、一つの宣言に。outer は、直後が throws のときだけ、節の頭
	d = firstDecl[*ast.DecoratorDecl](t, "decorator fn post(): throws A prevents effects outer throws B, C keeps effects;")
	if len(d.Clauses) != 2 || d.Clauses[0].(*ast.ThrowsClause).Outer || !d.Clauses[1].(*ast.ThrowsClause).Outer || names(d.Clauses[1].(*ast.ThrowsClause).Exceptions) != "B|C" {
		t.Errorf("Clauses = %+v", d.Clauses)
	}
	tests := []struct{ name, src, want string }{
		{"on model", "@repeatable model A {}", "@repeatable はデコレータの宣言の直前にだけ書ける"},
		{"with args", "@repeatable() decorator fn x();", "@repeatable は引数を取らない"},
		{"before other decorator", "@repeatable @db decorator fn x();", "@repeatable はデコレータの宣言の直前にだけ書ける"},
		{"twice", "@repeatable @repeatable decorator fn x();", "@repeatable を二度書いている"},
		{"entry twice", "@entry @repeatable @entry decorator fn x();", "@entry を二度書いている"},
		{"entry on for form", "@entry decorator x for read;", "@entry を付けられるのは、関数に付けるデコレータ"},
		{"hint on alias", "@hint decorator a = b();", "名前付きのデコレータに @hint は付けられない"},
		{"removed external", "@external decorator fn x(): throws E;", "@external はなくなった。fn の外で起きる例外は、節 outer throws で書く"},
		{"outer on for form", "decorator x for read outer throws E;", "for で付ける先を書いたデコレータは、節も本体も持てない"},
		{"outer without colon", "decorator fn x() outer throws E;", "「)」の後ろに、節を書くなら、「:」で始める"},
		{"on alias", "@repeatable decorator a = b();", "名前付きのデコレータに @repeatable は付けられない"},
		{"on field", "model A { @repeatable x: int; }", "@repeatable はデコレータの宣言の直前にだけ書ける"},
		{"on reserved field", "model A { @repeatable decorator: string; }", "@repeatable はデコレータの宣言の直前にだけ書ける"},
		{"on op", "@repeatable fn f();", "@repeatable はデコレータの宣言の直前にだけ書ける"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

func TestDecoratorDeclClauses(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, `decorator fn fallbackToCache():
    catches http.NetworkError recovers
    throws OfflineCacheMiss prevents effects
    catches A, B ignores
    catches C halts transitionToLogin
    throws X, Y keeps effects;`)
	if len(d.Clauses) != 5 {
		t.Fatalf("Clauses = %d", len(d.Clauses))
	}
	c0 := d.Clauses[0].(*ast.CatchesClause)
	if names(c0.Exceptions) != "http.NetworkError" || c0.Outcome != ast.Recovers || c0.Halts != nil {
		t.Errorf("Clause 0 = %+v", c0)
	}
	c1 := d.Clauses[1].(*ast.ThrowsClause)
	if names(c1.Exceptions) != "OfflineCacheMiss" || len(c1.Relations) != 1 || c1.Relations[0].Kind != ast.Prevents {
		t.Errorf("Clause 1 = %+v", c1)
	}
	c2 := d.Clauses[2].(*ast.CatchesClause)
	if names(c2.Exceptions) != "A|B" || c2.Outcome != ast.Ignores {
		t.Errorf("Clause 2 = %+v", c2)
	}
	c3 := d.Clauses[3].(*ast.CatchesClause)
	if c3.Outcome != ast.Halts || c3.Halts == nil || c3.Halts.String() != "transitionToLogin" {
		t.Errorf("Clause 3 = %+v", c3)
	}
	c4 := d.Clauses[4].(*ast.ThrowsClause)
	if names(c4.Exceptions) != "X|Y" || len(c4.Relations) != 1 || c4.Relations[0].Kind != ast.Keeps {
		t.Errorf("Clause 4 = %+v", c4)
	}
	// Relation の Span は "keeps effects" の二語。
	if got := c4.Relations[0].Span; got.End-got.Start != source.Offset(len("keeps effects")) {
		t.Errorf("Relation.Span = %+v", got)
	}
}

// TestDecoratorName は DecoratorName / DecoratorRef（docs/grammar.md「識別子」）: デコレータの名前を書く位置
// （宣言の名前、適用の @ の後ろ、別名の = の後ろ、@see の後ろ）には予約語を書け、指す位置では最後の要素だけが予約語でよい。
func TestDecoratorName(t *testing.T) {
	for _, kw := range []string{"list", "effects", "int", "float", "delete", "op", "for", "decorator", "reads", "datetime"} {
		d := firstDecl[*ast.DecoratorDecl](t, "decorator fn "+kw+"(table: any state);")
		if d.Name.Name != kw || len(d.Params) != 1 || !d.Fn || len(d.For) != 0 {
			t.Errorf("decorator fn %s = %+v", kw, d)
		}
		if got := d.Name.Span; got.Start != source.Offset(len("decorator fn ")) || got.End != source.Offset(len("decorator fn "+kw)) {
			t.Errorf("decorator %s の名前の Span = %+v", kw, got)
		}
	}
	// 引数も対象も節もない宣言
	d := firstDecl[*ast.DecoratorDecl](t, "decorator int for field;")
	if d.Name.Name != "int" || d.For[0].Kind != ast.TargetField {
		t.Errorf("Decorator = %+v", d)
	}
	// 適用: 修飾していれば最後の要素だけ、していなければその予約語の字句そのもの
	o := firstDecl[*ast.OpDecl](t, `@db.list(orders) @db.effects(a, b) @effects(a) @list @a.b.int fn f();`)
	want := []string{"db.list", "db.effects", "effects", "list", "a.b.int"}
	if len(o.Decorators) != len(want) {
		t.Fatalf("Decorators = %d", len(o.Decorators))
	}
	for i, w := range want {
		if got := o.Decorators[i].Name.String(); got != w {
			t.Errorf("Decorators[%d] = %s, want %s", i, got, w)
		}
	}
	if segs := o.Decorators[0].Name.Segments; len(segs) != 2 || segs[1].Name != "list" || segs[1].Span.Start != 4 || segs[1].Span.End != 8 {
		t.Errorf("@db.list の要素 = %+v", segs)
	}
	if len(o.Decorators[1].Args) != 2 || o.Decorators[3].Args != nil {
		t.Errorf("Args = %+v / %+v", o.Decorators[1].Args, o.Decorators[3].Args)
	}
	// フィールド、効果、状態にも付く
	m := firstDecl[*ast.ModelDecl](t, "model M { @db.int @db.key id: int; @text name: string; }")
	if got := m.Body.Fields[0].Decorators[0].Name.String(); got != "db.int" {
		t.Errorf("field decorator = %s", got)
	}
	if got := m.Body.Fields[1].Decorators[0].Name.String(); got != "text" {
		t.Errorf("field decorator = %s", got)
	}
	e := firstDecl[*ast.EffectDecl](t, "@db.delete(sessions, \"token\") effect fn deleteSession(): effects sessions.delete;")
	if len(e.Decorators) != 1 || e.Decorators[0].Name.String() != "db.delete" || len(e.Decorators[0].Args) != 2 {
		t.Errorf("effect decorators = %+v", e.Decorators)
	}
	// 別名の先
	a := firstDecl[*ast.DecoratorDecl](t, "decorator allOrders = db.list(orders);")
	if a.Alias == nil || a.Alias.Name.String() != "db.list" {
		t.Errorf("Alias = %+v", a.Alias)
	}
	a = firstDecl[*ast.DecoratorDecl](t, "decorator float = int();")
	if a.Name.Name != "float" || a.Alias == nil || a.Alias.Name.String() != "int" {
		t.Errorf("Alias = %+v", a)
	}
	// @see は QualifiedName か DecoratorRef
	mm := firstDecl[*ast.ModelDecl](t, "/**\n * @see db.list\n * @see effects\n */ model A {}")
	if len(mm.Doc.Tags) != 2 || mm.Doc.Tags[0].Name.String() != "db.list" || mm.Doc.Tags[1].Name.String() != "effects" {
		t.Errorf("Tags = %+v", mm.Doc.Tags)
	}
	// 核の印は予約語のデコレータ名の宣言にも付く
	x := firstDecl[*ast.DecoratorDecl](t, "@entry decorator fn delete(): outer throws X;")
	if !x.Entry || x.Name.Name != "delete" {
		t.Errorf("Decorator = %+v", x)
	}
}

// TestDecoratorDeclCarryingClauses は DecoReadsClause / DecoEffectsClause と対象 effect、残りの引数
// （docs/grammar.md「decorator の宣言」）。
func TestDecoratorDeclCarryingClauses(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, "decorator fn find(table: any state, column: string): reads table;")
	if len(d.Clauses) != 1 {
		t.Fatalf("Clauses = %d", len(d.Clauses))
	}
	r := d.Clauses[0].(*ast.ReadsClause)
	if names(r.Reads) != "table" {
		t.Errorf("reads = %+v", r)
	}
	if got := r.Span; got.End-got.Start != source.Offset(len("reads table")) {
		t.Errorf("reads の Span = %+v", got)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator fn effects(...targets: any effect): effects targets;")
	if d.Name.Name != "effects" || len(d.Params) != 1 || !d.Params[0].Rest || d.Params[0].Name.Name != "targets" {
		t.Fatalf("Decorator = %+v", d)
	}
	if p := d.Params[0]; p.Type.Any == nil || p.Type.Any.Kind != ast.AnyEffect || p.Type.Optional {
		t.Errorf("Rest の型 = %+v", p.Type)
	}
	if got := d.Params[0].RestSpan; got.Start != source.Offset(len("decorator fn effects(")) || got.End-got.Start != 3 {
		t.Errorf("RestSpan = %+v", got)
	}
	if got := d.Params[0].Span; got.Start != d.Params[0].RestSpan.Start {
		t.Errorf("残りの引数の Span は ... から: %+v", got)
	}
	e := d.Clauses[0].(*ast.EffectsClause)
	if names(e.Effects) != "targets" {
		t.Errorf("effects = %+v", e)
	}
	// 節は何度でも、どの順でも書ける。要素と区切りは op の節と同じ（区切りは ","。effects には直に書いた「状態.操作」も）
	d = firstDecl[*ast.DecoratorDecl](t, `decorator fn both(a: any state, b: any state, ...es: any effect):
    reads a, b, store.users
    throws X prevents effects effects es, store.e, store.users.post
    catches Y recovers
    reads carts
    effects U;`)
	if len(d.Clauses) != 6 {
		t.Fatalf("Clauses = %d", len(d.Clauses))
	}
	if r := d.Clauses[0].(*ast.ReadsClause); names(r.Reads) != "a|b|store.users" {
		t.Errorf("Clause 0 = %+v", r)
	}
	// `throws X prevents effects effects e`: Relation の後ろに DecoEffectsClause
	th := d.Clauses[1].(*ast.ThrowsClause)
	if names(th.Exceptions) != "X" || len(th.Relations) != 1 || th.Relations[0].Kind != ast.Prevents {
		t.Errorf("Clause 1 = %+v", th)
	}
	if e := d.Clauses[2].(*ast.EffectsClause); names(e.Effects) != "es|store.e|store.users.post" {
		t.Errorf("Clause 2 = %+v", e)
	}
	if _, ok := d.Clauses[3].(*ast.CatchesClause); !ok {
		t.Errorf("Clause 3 = %T", d.Clauses[3])
	}
	if r := d.Clauses[4].(*ast.ReadsClause); names(r.Reads) != "carts" {
		t.Errorf("Clause 4 = %+v", r)
	}
	if e := d.Clauses[5].(*ast.EffectsClause); names(e.Effects) != "U" {
		t.Errorf("Clause 5 = %+v", e)
	}
	// 残りの引数は、ほかの引数の後ろに一つ、または単独で。型は any か、型（| も書ける）
	d = firstDecl[*ast.DecoratorDecl](t, "decorator fn tags(first: string, second: int?, ...rest: int | float);")
	if len(d.Params) != 3 || d.Params[0].Rest || d.Params[1].Rest || !d.Params[2].Rest || len(d.Params[2].Type.Types) != 2 {
		t.Errorf("Params = %+v", d.Params)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator fn on(...ss: any effect on carts.post);")
	if p := d.Params[0]; !p.Rest || p.Type.Any == nil || p.Type.Any.On == nil || p.Type.Any.On.Op != ast.Post {
		t.Errorf("Param = %+v", p)
	}
	// 対象 effect（予約語。その位置でも同じ字句）
	d = firstDecl[*ast.DecoratorDecl](t, "decorator insert(table: any state) for effect;")
	if len(d.For) != 1 || d.For[0].Kind != ast.TargetEffect || d.For[0].Kind.String() != "effect" {
		t.Errorf("For = %+v", d.For)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator mark for op, effect, field;")
	if len(d.For) != 3 || d.For[1].Kind != ast.TargetEffect {
		t.Errorf("For = %+v", d.For)
	}
}

// TestRestArguments は残りの引数への渡し方（docs/grammar.md「デコレータの適用」）: 適用の構文は変わらず、
// ほかの引数の数を超えて位置で渡した値が、書いた順に並ぶ（対応付けは internal/sema）。
func TestRestArguments(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, "@db.effects(createOrder, notifyUser, a.b | c) @db.effects() @db.effects fn f();")
	if len(o.Decorators) != 3 {
		t.Fatalf("Decorators = %d", len(o.Decorators))
	}
	args := o.Decorators[0].Args
	if len(args) != 3 || args[0].Name != nil || args[1].Value.Terms[0].Name.String() != "notifyUser" || len(args[2].Value.Terms) != 2 {
		t.Errorf("Args = %+v", args)
	}
	if o.Decorators[1].Args == nil || len(o.Decorators[1].Args) != 0 || o.Decorators[2].Args != nil {
		t.Errorf("0 個の渡し方: %+v / %+v", o.Decorators[1].Args, o.Decorators[2].Args)
	}
}

// TestRelationObject は Relation = ( "prevents" | "keeps" ) "effects"（docs/grammar.md「decorator の宣言」）:
// 目的語は `effects` だけで、効果の名前は書けない（docs/grammar.md「決めたこと」、docs/decisions.md S9）。
func TestRelationObject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"named effect on op", "fn f(): effects a throws X keeps a;", "目的語は「effects」だけ"},
		{"named effects on op", "fn f(): effects a throws X keeps a prevents b;", "keeps の目的語は「effects」だけ"},
		{"qualified effect", "fn f(): throws X prevents server.a;", "prevents の目的語は「effects」だけ"},
		{"named effect on decorator", "decorator fn d(): throws X prevents a | b;", "prevents の目的語は「effects」だけ"},
		{"missing object", "fn f(): throws X prevents;", "「effects」が必要だが「;」がある"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

func TestDecoratorDeclAlias(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, "decorator unavailableOnFailure = page.fallback(http.NetworkError | http.ServerError, Unavailable);")
	if d.Alias == nil || d.Alias.Name.String() != "page.fallback" || len(d.Alias.Args) != 2 || len(d.Alias.Args[0].Value.Terms) != 2 {
		t.Fatalf("Alias = %+v", d.Alias)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator a = b();")
	if d.Alias == nil || len(d.Alias.Args) != 0 {
		t.Errorf("Alias = %+v", d.Alias)
	}
}

func TestDecoratorDeclParamTypes(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, "decorator fn route(path: string, from: any state?, onInvalid: any effect on location.put, when: any variant, e: any exception, o: any op, m: any model);")
	if len(d.Params) != 7 {
		t.Fatalf("Params = %d", len(d.Params))
	}
	p1 := d.Params[1].Type
	if p1.Any == nil || p1.Any.Kind != ast.AnyState || !p1.Optional || p1.Any.On != nil {
		t.Errorf("Param 1 = %+v", p1)
	}
	p2 := d.Params[2].Type
	if p2.Any == nil || p2.Any.Kind != ast.AnyEffect || p2.Optional || p2.Any.On == nil || p2.Any.On.State.String() != "location" || p2.Any.On.Op != ast.Put {
		t.Errorf("Param 2 = %+v", p2)
	}
	kinds := []ast.AnyKind{ast.AnyVariant, ast.AnyException, ast.AnyOp, ast.AnyModel}
	for i, k := range kinds {
		if a := d.Params[3+i].Type.Any; a == nil || a.Kind != k {
			t.Errorf("Param %d = %+v, want %s", 3+i, d.Params[3+i].Type, k)
		}
	}
}

func TestDecoratorDeclReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"entry on alias", "@entry decorator a = b();", "名前付きのデコレータに @entry は付けられない"},
		{"catches without outcome", "decorator fn a(): catches X;", "catches の後の振る舞い"},
		{"halts without effect", "decorator fn a(): catches X halts;", "名前が必要"},
		{"bad any kind", "decorator fn a(x: any thing);", "any の種類"},
		{"bad target", "decorator a for thing;", "for の対象"},
		{"namespace target", "decorator a for namespace;", "for の対象"},
		{"param without type", "decorator fn a(x);", "「:」が必要"},
		{"alias without args", "decorator a = b;", "「(」が必要"},
		{"optional non-any type with pipe", "decorator fn a(x: int |);", "型が必要"},
		{"missing semicolon", "decorator a for state", "「;」が必要"},
		{"reads without name", "decorator fn a(): reads;", "名前が必要"},
		{"reads separated by pipe", "decorator fn a(): reads x | y;", "reads の並びの区切りは「,」で、「|」ではない"},
		{"effects separated by pipe", "decorator fn a(): effects x | y;", "effects の並びの区切りは「,」で、「|」ではない"},
		{"throws separated by pipe", "decorator fn a(): throws X | Y;", "throws の並びの区切りは「,」で、「|」ではない"},
		{"catches separated by pipe", "decorator fn a(): catches X | Y recovers;", "catches の並びの区切りは「,」で、「|」ではない"},
		{"effects trailing comma", "decorator fn a(): effects x,;", "名前が必要"},
		{"effects without name", "decorator fn a(): effects;", "名前が必要"},
		{"qualified name", "decorator a.b;", "修飾しない識別子"},
		{"no name", "decorator ;", "デコレータの名前が必要"},
		{"rest not last", "decorator fn a(...xs: any effect, y: string);", "残りの引数 ...xs は引数の並びの最後に一つだけ書ける"},
		{"two rests", "decorator fn a(...xs: any effect, ...ys: any state);", "残りの引数 ...xs は引数の並びの最後に一つだけ書ける"},
		{"optional rest any", "decorator fn a(...xs: any effect?);", "残りの引数 ...xs は省略可能にできない"},
		{"optional rest type", "decorator fn a(...xs: string?);", "残りの引数 ...xs は省略可能にできない"},
		{"array rest type", "decorator fn a(...xs: string[]);", "渡す値一つの型を書く"},
		{"rest without name", "decorator fn a(...: string);", "「識別子」が必要"},
		{"rest with keyword name", "decorator fn a(...to: string);", "to は予約語で、識別子には使えない"},
		{"two dots", "decorator fn a(..xs: string);", "「識別子」が必要だが「.」がある"},
		{"rest in op params", "fn f(...xs: string);", "「識別子」が必要だが「...」がある"},
		{"alias target keyword in the middle", "decorator a = b.op.c();", "op は予約語で、名前には使えない"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// --- op ---

func TestOp(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, `@http.post("/login") @validated
fn login(in: LoginInput, n: int): LoginOk
  effects createSession, notify
  throws InvalidCredentials, NotVerified prevents effects
  throws Other keeps effects;`)
	if o.Name.Name != "login" || len(o.Params) != 2 || o.Params[0].Name.Name != "in" || typeString(o.Params[1].Type) != "int" {
		t.Fatalf("Op = %+v", o)
	}
	if o.Return == nil || o.Return.Void || typeString(o.Return.Type) != "LoginOk" {
		t.Errorf("Return = %+v", o.Return)
	}
	if len(o.Clauses) != 3 || o.Body != nil {
		t.Fatalf("Clauses = %d, Body = %v", len(o.Clauses), o.Body)
	}
	ef := o.Clauses[0].(*ast.EffectsClause)
	if names(ef.Effects) != "createSession|notify" {
		t.Errorf("Effects = %+v", ef)
	}
	th := o.Clauses[1].(*ast.ThrowsClause)
	if names(th.Exceptions) != "InvalidCredentials|NotVerified" || len(th.Relations) != 1 {
		t.Errorf("Throws = %+v", th)
	}
	if o.Clauses[2].(*ast.ThrowsClause).Relations[0].Kind != ast.Keeps {
		t.Errorf("Throws 2 = %+v", o.Clauses[2])
	}

	o = firstDecl[*ast.OpDecl](t, "fn clearToken(): effects removeToken;")
	if o.Return != nil || len(o.Params) != 0 || len(o.Clauses) != 1 {
		t.Errorf("Op = %+v", o)
	}
	// 「:」の後ろは、関数の型: 戻り値と節。戻り値は省けて、省けば void（docs/grammar.md「関数の宣言」）
	o = firstDecl[*ast.OpDecl](t, "fn a(x: int): throws X { f(x); }")
	if o.Return != nil || len(o.Clauses) != 1 || o.Body == nil {
		t.Errorf("戻り値を省いて、節を書いた形: Op = %+v", o)
	}
	o = firstDecl[*ast.OpDecl](t, "fn a(): void throws X reads s;")
	if o.Return == nil || !o.Return.Void || len(o.Clauses) != 2 {
		t.Errorf("void と節: Op = %+v", o)
	}
	o = firstDecl[*ast.OpDecl](t, "fn a() { f(); }")
	if o.Return != nil || len(o.Clauses) != 0 || o.Body == nil {
		t.Errorf("何も書かない形: Op = %+v", o)
	}
	o = firstDecl[*ast.OpDecl](t, "fn a(): void;")
	if o.Return == nil || !o.Return.Void || o.Return.Type != nil {
		t.Errorf("Return = %+v", o.Return)
	}
	o = firstDecl[*ast.OpDecl](t, "fn a(x: int)\n  : T\n{ return x; }")
	if o.Body == nil || len(o.Body.Stmts) != 1 || typeString(o.Return.Type) != "T" {
		t.Errorf("Op = %+v", o)
	}
	if o.Span.End != o.Body.Span.End {
		t.Errorf("Span = %+v, Body = %+v", o.Span, o.Body.Span)
	}

	// export、reads、never（docs/grammar.md「op」）。
	o = firstDecl[*ast.OpDecl](t, "/** d */ @http.get(\"/x\") export fn findUser(email: Email): User? reads users, sessions reads store.orders throws X;")
	if !o.Export || o.ExportSpan.End-o.ExportSpan.Start != source.Offset(len("export")) || o.Doc == nil || len(o.Decorators) != 1 {
		t.Errorf("Op = %+v", o)
	}
	if len(o.Clauses) != 3 {
		t.Fatalf("Clauses = %d", len(o.Clauses))
	}
	if r := o.Clauses[0].(*ast.ReadsClause); names(r.Reads) != "users|sessions" {
		t.Errorf("Reads 0 = %+v", r)
	}
	if r := o.Clauses[1].(*ast.ReadsClause); names(r.Reads) != "store.orders" {
		t.Errorf("Reads 1 = %+v", r)
	}
	// Span は export から。
	if o.Span.Start != o.Decorators[0].Span.Start {
		t.Errorf("Span = %+v", o.Span)
	}
	o = firstDecl[*ast.OpDecl](t, "export fn a();")
	if !o.Export || o.Span.Start != o.ExportSpan.Start {
		t.Errorf("Op = %+v", o)
	}
	o = firstDecl[*ast.OpDecl](t, "fn unauthorized(): never throws Unauthorized;")
	if o.Export || o.Return == nil || !o.Return.Never || o.Return.Void || o.Return.Type != nil {
		t.Errorf("Op = %+v（Return = %+v）", o, o.Return)
	}
}

// TestClauseSeparators は節の並び（docs/grammar.md「op」）: 区切りは "," で、effects の要素には直に書いた「状態.操作」、
// reads の要素には read の名前か状態の名前を書ける（どちらかは名前の解決が決めるので、構文木では名前のまま）。
// なくした書き方 "|" は SYNTAX にするが、"," として読み進めるので、宣言は残る。
func TestClauseSeparators(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, "fn f(): effects addUser, users.post, store.users.delete reads userByEmail, users, store.sessions throws A, b.B;")
	if len(o.Clauses) != 3 {
		t.Fatalf("Clauses = %d", len(o.Clauses))
	}
	if e := o.Clauses[0].(*ast.EffectsClause); names(e.Effects) != "addUser|users.post|store.users.delete" {
		t.Errorf("effects = %+v", e)
	}
	if r := o.Clauses[1].(*ast.ReadsClause); names(r.Reads) != "userByEmail|users|store.sessions" {
		t.Errorf("reads = %+v", r)
	}
	if th := o.Clauses[2].(*ast.ThrowsClause); names(th.Exceptions) != "A|b.B" {
		t.Errorf("throws = %+v", th)
	}

	file, diags := parseSrc(t, "fn f(): throws A | B effects e | users.post reads s | t;\nfn g();")
	if len(diags) != 3 {
		t.Fatalf("診断 = %d 件, want 3（節ごとに一つ）: %v", len(diags), diags)
	}
	for i, clause := range []string{"throws", "effects", "reads"} {
		if !strings.Contains(diags[i].Message, clause+" の並びの区切りは「,」で、「|」ではない") {
			t.Errorf("診断 %d = %q", i, diags[i].Message)
		}
	}
	if len(file.Decls) != 2 {
		t.Fatalf("読めた宣言 = %d, want 2（\"|\" は \",\" として読み進める）", len(file.Decls))
	}
	f := file.Decls[0].(*ast.OpDecl)
	if th := f.Clauses[0].(*ast.ThrowsClause); names(th.Exceptions) != "A|B" {
		t.Errorf("throws = %+v", th)
	}
	if e := f.Clauses[1].(*ast.EffectsClause); names(e.Effects) != "e|users.post" {
		t.Errorf("effects = %+v", e)
	}
}

// TestReadDecl は ReadDecl = "read" Ident OpParams ":" Type "on" QualifiedName ";"（docs/grammar.md「exception / effect / read」）。
func TestReadDecl(t *testing.T) {
	r := firstDecl[*ast.ReadDecl](t, "/** メールで引く */ @db.find(users) read fn userByEmail(email: Email): User? reads users;")
	if r.Name.Name != "userByEmail" || r.On.String() != "users" || r.Doc == nil || len(r.Decorators) != 1 {
		t.Fatalf("Read = %+v", r)
	}
	if len(r.Params) != 1 || r.Params[0].Name.Name != "email" || typeString(r.Params[0].Type) != "Email" || typeString(r.Result) != "User?" {
		t.Errorf("Read の引数と結果 = %+v %v", r.Params, r.Result)
	}
	if r.Span.Start != r.Decorators[0].Span.Start {
		t.Errorf("Span はデコレータから: %+v", r.Span)
	}
	r = firstDecl[*ast.ReadDecl](t, "read fn all(): User[] reads store.users;")
	if r.On.String() != "store.users" || r.Doc != nil || len(r.Decorators) != 0 || len(r.Params) != 0 || typeString(r.Result) != "User[]" {
		t.Errorf("Read = %+v", r)
	}

	// read は予約語だが、フィールド名とデコレータの名前には書ける（docs/grammar.md「識別子」）
	m := firstDecl[*ast.ModelDecl](t, "model M { read: boolean; }")
	if len(m.Body.Fields) != 1 || m.Body.Fields[0].Name.Name != "read" {
		t.Errorf("Model = %+v", m)
	}
	d := firstDecl[*ast.DecoratorDecl](t, "decorator read(target: any state, ...rest: any state) for read, op;")
	if d.Name.Name != "read" || d.Params[0].Type.Any.Kind != ast.AnyState || !d.Params[1].Rest {
		t.Fatalf("Decorator = %+v", d)
	}
	if len(d.For) != 2 || d.For[0].Kind != ast.TargetRead || d.For[1].Kind != ast.TargetOp {
		t.Errorf("For = %+v", d.For)
	}

	tests := []struct{ name, src, want string }{
		{"no reads", "read fn r(): T;", "「reads 状態」（read fn r(…): T reads 状態; と書く）が必要"},
		{"no colon", "read fn r() reads users;", "「)」の後ろに、戻り値か節を書くなら、「:」で始める（read fn r(…): reads … と書く"},
		{"old on", "read fn r(): T on users;", "read fn r(…) on … という書き方はなくなった。状態との関係は、reads で書く（read fn r(…): … reads … と書く"},
		{"two states", "read fn r(): T reads users, orders;", "read fn r が読む状態は、一つだけ"},
		{"throws", "read fn r(): T throws E reads users;", "read fn r に throws の節は書けない"},
		{"no state", "read fn r(): T reads;", "読む状態の名前が必要"},
		{"with body", "read fn r(): T reads users {}", "「;」が必要"},
		{"export", "export read fn r(): T reads users;", "export を付けられるのは、種類を書かない fn（op）だけ"},
		{"qualified name", "read fn a.r(): T reads users;", "修飾しない識別子"},
		{"no parens", "read fn r: T reads users;", "read fn r の名前の後ろに、引数の括弧が要る"},
		{"old head", "read r(): T reads users;", "read r(…) という書き方はなくなった。関数の宣言の頭は fn で、種類は、その前に書く（read fn r(…) と書く"},
		{"any read", "decorator fn d(x: any read);", "any の種類（exception / effect / state / op / model / variant / case）が必要"},
		{"keyword as op name", "fn read();", "read は予約語で、識別子には使えない"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
	// 戻り値（docs/grammar.md「関数の宣言」）: 書いたものを、そのまま持つ。省いた形、void、never を弾くのは、意味の解析（TYPE-18）
	if r := firstDecl[*ast.ReadDecl](t, "read fn r(): reads users;"); r.Return != nil || r.Result != nil {
		t.Errorf("省いた戻り値: Return = %+v, Result = %+v", r.Return, r.Result)
	}
	if r := firstDecl[*ast.ReadDecl](t, "read fn r(): void reads users;"); r.Return == nil || !r.Return.Void || r.Result != nil {
		t.Errorf("void の戻り値: Return = %+v", r.Return)
	}
	if r := firstDecl[*ast.ReadDecl](t, "read fn r(): never reads users;"); r.Return == nil || !r.Return.Never || r.Result != nil {
		t.Errorf("never の戻り値: Return = %+v", r.Return)
	}
}

func TestOpReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"catches on op", "fn a() catches X recovers;", "catches はデコレータの宣言にだけ書ける"},
		{"no terminator", "fn a()", "「;」か配線の「{」が必要"},
		{"clause without colon", "fn a() throws X;", "「)」の後ろに、戻り値か節を書くなら、「:」で始める（fn a(…): throws … と書く"},
		{"clause without colon, body", "fn a() effects e { e(); }", "「)」の後ろに、戻り値か節を書くなら、「:」で始める（fn a(…): effects … と書く"},
		{"clause without colon, reads", "fn a()\n  reads s;", "「:」で始める（fn a(…): reads … と書く"},
		{"colon with nothing", "fn a():;", "「:」の後ろに、戻り値か節が要る。何も書かないなら、「:」も書かない（fn a(…); か fn a(…) { … } と書く"},
		{"colon with nothing, body", "fn a(): { f(); }", "「:」の後ろに、戻り値か節が要る"},
		{"on", "fn a(): T on users;", "種類を書かない fn に on は書けない。状態を読み書きするのは、read fn と effect fn"},
		{"no params", "fn a;", "「(」が必要"},
		{"old head", "op a();", "op a(…) という書き方はなくなった。関数の宣言の頭は fn（fn a(…) と書く"},
		{"old head with export", "export op a();", "op a(…) という書き方はなくなった"},
		{"op fn", "op fn a();", "op fn とは書かない。種類を書かない fn が op"},
		{"param without type", "fn a(x);", "「:」が必要"},
		{"void param", "fn a(x: void);", "void は op の戻り値の型にだけ書ける"},
		{"pipe type in param", "fn a(x: int | float);", "「,」か「)」"},
		{"for on op", "fn a() for op;", "「;」か配線の「{」が必要"},
		{"qualified name", "fn a.b();", "修飾しない識別子"},
		{"export on model", "export model A {}", "export を付けられるのは、種類を書かない fn（op）だけ"},
		{"export on state", "export state s: int;", "export を付けられるのは、種類を書かない fn（op）だけ"},
		{"export on namespace", "export namespace a {}", "export を付けられるのは、種類を書かない fn（op）だけ"},
		{"export before decorator", "export @d fn a();", "export を付けられるのは、種類を書かない fn（op）だけ"},
		{"export twice", "export export fn a();", "export を付けられるのは、種類を書かない fn（op）だけ"},
		{"reads with pipe", "fn a(): reads s | t;", "reads の並びの区切りは「,」で、「|」ではない"},
		{"effects with pipe", "fn a(): effects e | users.post;", "effects の並びの区切りは「,」で、「|」ではない"},
		{"throws with pipe", "fn a(): throws X | Y;", "throws の並びの区切りは「,」で、「|」ではない"},
		{"throws trailing comma", "fn a(): throws X,;", "名前が必要"},
		{"reads without name", "fn a(): reads;", "名前が必要"},
		{"reads trailing comma", "fn a(): reads s,;", "名前が必要"},
		{"never param", "fn a(x: never);", "never は op の戻り値の型と"},
		{"never optional", "fn a(): never?;", "「;」か配線の「{」が必要"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want == "「,」か「)」" {
				// 文法の誤りの位置だけを確かめる（どの字句が足りないかはメッセージの形で決まる）。
				mustFail(t, tt.src, "「)」が必要だが「|」がある")
				return
			}
			mustFail(t, tt.src, tt.want)
		})
	}
}

// --- bind ---

func TestBind(t *testing.T) {
	b := firstDecl[*ast.BindDecl](t, "bind token to server.*;")
	if b.Name.Name != "token" || b.Target.String() != "server" || b.Deep {
		t.Errorf("Bind = %+v", b)
	}
	b = firstDecl[*ast.BindDecl](t, "bind token to server.admin.**;")
	if b.Target.String() != "server.admin" || !b.Deep {
		t.Errorf("Bind = %+v", b)
	}
	mustFail(t, "bind token to server;", "「.」が必要")
	mustFail(t, "bind token to server.x;", "「.」が必要")
	mustFail(t, "bind token to server.***;", "「;」が必要")
	mustFail(t, "bind token server.*;", "「to」が必要")
}

// --- 型 ---

func TestType(t *testing.T) {
	tests := []struct{ src, want string }{
		{"string", "string"},
		{"boolean", "boolean"},
		{"int", "int"},
		{"float", "float"},
		{"datetime", "datetime"},
		{"duration", "duration"},
		{"User", "User"},
		{"server.User", "server.User"},
		{"User[]", "User[]"},
		{"User?", "User?"},
		{"User[]?", "User[]?"},
		{"User?[]", "User?[]"},
		{"User[][]", "User[][]"},
		{"Record", "Record"}, // Record は、予約語ではなく、ふつうの名前
		// 絞り込んだ型（docs/grammar.md「型」の OnType）: on は、[] と ? の後ろに、一度だけ。いくつかのケースは「|」
		{"server.Item on WarrantyKind.Yamada", "server.Item on WarrantyKind.Yamada"},
		{"Order on Stage.Draft | Stage.Accepted", "Order on Stage.Draft | Stage.Accepted"},
		{"Order on shop.Stage.Draft | shop.Stage.Accepted | shop.Stage.Shipped", "Order on shop.Stage.Draft | shop.Stage.Accepted | shop.Stage.Shipped"},
		{"Order? on Life.Absent", "Order? on Life.Absent"},
		{"Order[] on Batch.Full", "Order[] on Batch.Full"},
		{"list<Order on Stage.Draft>", "list<Order on Stage.Draft>"},
		{"nullable<Order on Stage.Draft | Stage.Accepted>[]", "nullable<Order on Stage.Draft | Stage.Accepted>[]"},
		{"list<A>", "list<A>"},
		{"nullable<A>", "nullable<A>"},
		{"single<A>", "single<A>"},
		{"list<nullable<a.B>>?", "list<nullable<a.B>>?"},
		{"single<A?>[]", "single<A?>[]"},
	}
	for _, tt := range tests {
		s := firstDecl[*ast.ScalarDecl](t, "scalar X extends "+tt.src+";")
		if got := typeString(s.Base); got != tt.want {
			t.Errorf("%s: type = %s, want %s", tt.src, got, tt.want)
		}
		if s.Base.NodeSpan().Start != source.Offset(len("scalar X extends ")) || s.Base.NodeSpan().End != source.Offset(len("scalar X extends ")+len(tt.src)) {
			t.Errorf("%s: Span = %+v", tt.src, s.Base.NodeSpan())
		}
	}
	rejects := []struct{ src, want string }{
		{"void", "void は op の戻り値の型にだけ書ける"},
		{"never", "never は op の戻り値の型と、override のなくす印にだけ書ける"},
		{"list<never>", "never は op の戻り値の型と"},
		{"list", "「<」が必要"},
		{"list<A, B>", "「>」が必要"},
		{"{ a: int; }", "その場で書く model はユニオンのアームと例外の値にだけ置ける"},
		// Record<K, V> は、なくした書き方（docs/grammar.md「model」「決めたこと」）
		{"Record<string, int>", "Record<K, V> という書き方はなくなった"},
		{"Record<string, int>", "model 名前 { [キーの名前: K]: V; }"},
		{"list<Record<string, int>>", "インデックスの model を宣言して、その名前を書く"},
		{"User[", "「]」が必要"},
		{"string<int>", "「;」が必要だが「<」がある"},
		// なくした書き方 C.K<T> は、新しい書き方をメッセージで示す
		{"Stage.Draft<Order>", "Stage.Draft<T> という書き方はなくなった。分類のケースで絞り込んだ型は、T on Stage.Draft と書く"},
		{"A<B<C>>?", "A<T> という書き方はなくなった"},
		// on の後ろ: ケースは、分類の名前から。区切りは「|」。[] と ? と、二つめの on は続けられない
		{"Order on Draft", "絞り込んだ型のケースは、分類の名前から書く（T on <分類>.Draft"},
		{"Order on", "絞り込んだ型のケース（T on <分類>.<ケース>）が必要"},
		{"Order on Stage.Draft |", "絞り込んだ型のケース（T on <分類>.<ケース>）が必要"},
		{"list<Order on Stage.Draft, Stage.Accepted>", "絞り込んだ型の、いくつかのケースの区切りは「|」で、「,」ではない"},
		{"Order on Stage.Draft?", "on の後ろに [] と ? は続けられない"},
		{"Order on Stage.Draft[]", "on の後ろに [] と ? は続けられない"},
		{"Order on Stage.Draft on Urgency.Rush", "on は、一つの型に、一度だけ書ける"},
		{"5", "型が必要"},
	}
	for _, tt := range rejects {
		mustFail(t, "scalar X extends "+tt.src+";", tt.want)
	}
}

// 絞り込んだ型を、引数と戻り値に書く（docs/grammar.md「型」の OnType）: 引数の並びの「,」は、次が「名前 :」なので、
// ケースの区切りと取り違えない。
func TestOnTypeInFn(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, "fn f(o: Order on Stage.Draft, p: Payment on Standing.Held | Standing.Prepaid, n: int): Order on Stage.Accepted;")
	var got []string
	for _, p := range o.Params {
		got = append(got, typeString(p.Type))
	}
	if want := "Order on Stage.Draft; Payment on Standing.Held | Standing.Prepaid; int"; strings.Join(got, "; ") != want {
		t.Errorf("引数の型 = %s, want %s", strings.Join(got, "; "), want)
	}
	if o.Return == nil || typeString(o.Return.Type) != "Order on Stage.Accepted" {
		t.Errorf("戻り値の型 = %+v", o.Return)
	}
	mustFail(t, "fn f(o: Order on Stage.Draft, Stage.Accepted): void;", "絞り込んだ型の、いくつかのケースの区切りは「|」で、「,」ではない（T on C.A | C.B と書く")
	// val と for の変数にも書ける
	b := firstDecl[*ast.OpDecl](t, "fn g(): void { val o: Order on Stage.Draft = find(); }").Body
	if v := b.Stmts[0].(*ast.ValStmt); v.Type == nil || typeString(v.Type) != "Order on Stage.Draft" {
		t.Errorf("val の型 = %+v", v.Type)
	}
}

// --- 配線 ---

func TestBlockStmts(t *testing.T) {
	src := `fn a(query: OrdersQuery): OrderPage {
  val me = fetchMe();
  val line = server.addToCart({ item });
  val v = query.page;
  val n = 5;
  val z = none;
  requestLogout();
  server.logout({});
  match (x by K) { A => f(); };
  return orderPageVm(me, line, "s", -1, 2.5, true, 30s, none, { page: query.page, status: s, });
}`
	o := firstDecl[*ast.OpDecl](t, src)
	st := o.Body.Stmts
	if len(st) != 9 {
		t.Fatalf("Stmts = %d", len(st))
	}
	c0 := st[0].(*ast.ValStmt)
	if c0.Name.Name != "me" {
		t.Errorf("Stmt 0 = %+v", c0)
	}
	if call, ok := c0.Value.(*ast.Call); !ok || call.Callee.String() != "fetchMe" || len(call.Args) != 0 {
		t.Errorf("Stmt 0 value = %+v", c0.Value)
	}
	c1 := st[1].(*ast.ValStmt).Value.(*ast.Call)
	if c1.Callee.String() != "server.addToCart" || len(c1.Args) != 1 {
		t.Fatalf("Stmt 1 = %+v", c1)
	}
	rec := c1.Args[0].(*ast.RecordLit)
	if len(rec.Fields) != 1 || rec.Fields[0].Name.Name != "item" || rec.Fields[0].Value != nil {
		t.Errorf("record = %+v", rec)
	}
	if n, ok := st[2].(*ast.ValStmt).Value.(*ast.NameAtom); !ok || n.Name.String() != "query.page" || len(n.Name.Segments) != 2 {
		t.Errorf("Stmt 2 = %+v", st[2].(*ast.ValStmt).Value)
	}
	if l, ok := st[3].(*ast.ValStmt).Value.(*ast.Literal); !ok || l.Int != 5 {
		t.Errorf("Stmt 3 = %+v", st[3].(*ast.ValStmt).Value)
	}
	if _, ok := st[4].(*ast.ValStmt).Value.(*ast.NoneAtom); !ok {
		t.Errorf("Stmt 4 = %+v", st[4].(*ast.ValStmt).Value)
	}
	if cs, ok := st[5].(*ast.CallStmt); !ok || cs.Call.Callee.String() != "requestLogout" {
		t.Errorf("Stmt 5 = %+v", st[5])
	}
	cs6 := st[6].(*ast.CallStmt)
	if cs6.Call.Callee.String() != "server.logout" || len(cs6.Call.Args) != 1 || len(cs6.Call.Args[0].(*ast.RecordLit).Fields) != 0 {
		t.Errorf("Stmt 6 = %+v", cs6)
	}
	if ms, ok := st[7].(*ast.MatchStmt); !ok || ms.Match.Classifier.String() != "K" {
		t.Errorf("Stmt 7 = %+v", st[7])
	}
	ret := st[8].(*ast.ReturnStmt).Value.(*ast.Call)
	if len(ret.Args) != 9 {
		t.Fatalf("return args = %d", len(ret.Args))
	}
	wantKinds := []ast.LiteralKind{ast.StringLit, ast.IntLit, ast.FloatLit, ast.BoolLit, ast.DurationLit}
	for i, k := range wantKinds {
		if l, ok := ret.Args[2+i].(*ast.Literal); !ok || l.Kind != k {
			t.Errorf("return arg %d = %+v", 2+i, ret.Args[2+i])
		}
	}
	if _, ok := ret.Args[7].(*ast.NoneAtom); !ok {
		t.Errorf("return arg 7 = %+v", ret.Args[7])
	}
	last := ret.Args[8].(*ast.RecordLit)
	if len(last.Fields) != 2 || last.Fields[0].Value.(*ast.NameAtom).Name.String() != "query.page" || last.Fields[1].Name.Name != "status" {
		t.Errorf("record = %+v", last)
	}
	// 文の Span は ";" まで。
	if end := st[8].NodeSpan().End; end >= o.Body.Span.End || src[end-1] != ';' {
		t.Errorf("return Span = %+v, Body = %+v", st[8].NodeSpan(), o.Body.Span)
	}
}

func TestReturnAtom(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, "fn a(me: M): M { return me; }")
	if _, ok := o.Body.Stmts[0].(*ast.ReturnStmt).Value.(*ast.NameAtom); !ok {
		t.Errorf("return = %+v", o.Body.Stmts[0])
	}
}

func TestMatch(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, `fn a(item: I) {
  val warranty = match (item by WarrantyKind) {
    Standard, NoWarranty => chooseStandardWarranty(item, line);
    Yamada     => server.chooseYamada(item);
    else => none;
  };
  match (item by Available) { A => f(); };
}`)
	m := o.Body.Stmts[0].(*ast.ValStmt).Value.(*ast.Match)
	if m.Subject.(*ast.NameAtom).Name.String() != "item" || m.Classifier.String() != "WarrantyKind" || m.Builtin || len(m.Arms) != 3 {
		t.Fatalf("Match = %+v", m)
	}
	a0 := m.Arms[0]
	if len(a0.Cases) != 2 || a0.Cases[1].Name != "NoWarranty" || a0.Else {
		t.Errorf("Arm 0 = %+v", a0)
	}
	if c, ok := a0.Body.(*ast.Call); !ok || c.Callee.String() != "chooseStandardWarranty" || len(c.Args) != 2 {
		t.Errorf("Arm 0 body = %+v", a0.Body)
	}
	if c, ok := m.Arms[1].Body.(*ast.Call); !ok || c.Callee.String() != "server.chooseYamada" {
		t.Errorf("Arm 1 body = %+v", m.Arms[1].Body)
	}
	// ElseArm = "else" "=>" ArmBody ";"。ケースの並びは空で、ElseSpan が else の位置
	a2 := m.Arms[2]
	if !a2.Else || len(a2.Cases) != 0 || a2.ElseSpan.Start != a2.Span.Start || a2.ElseSpan.End-a2.ElseSpan.Start != 4 {
		t.Errorf("Arm 2 = %+v", a2)
	}
	if _, ok := a2.Body.(*ast.NoneAtom); !ok {
		t.Errorf("Arm 2 body = %+v", a2.Body)
	}
	// match の Span は "}" まで。const の Span は ";" まで。
	if m.Span.End+1 != o.Body.Stmts[0].NodeSpan().End {
		t.Errorf("Match Span = %+v, Const Span = %+v", m.Span, o.Body.Stmts[0].NodeSpan())
	}
}

// アームの本体（docs/grammar.md「配線」の ArmBody = Call | Atom | Throw）と、修飾した分類の名前。
func TestMatchArmBody(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, `fn a(user: User?): Session {
  return match (user by store.Registration) {
    Registered => issueSession(user, in.body.password);
    LoggedIn => session;
    Field => session.token;
    Empty => none;
    Standard => 1;
    Named => "x";
    Rec => { a: 1, b };
    else => throw type.Unauthorized;
  };
}`)
	m := o.Body.Stmts[0].(*ast.ReturnStmt).Value.(*ast.Match)
	if m.Classifier.String() != "store.Registration" || len(m.Classifier.Segments) != 2 || m.Builtin {
		t.Fatalf("Classifier = %+v", m.Classifier)
	}
	if _, ok := m.Arms[0].Body.(*ast.Call); !ok {
		t.Errorf("Arm 0 body = %+v", m.Arms[0].Body)
	}
	if n, ok := m.Arms[1].Body.(*ast.NameAtom); !ok || n.Name.String() != "session" {
		t.Errorf("Arm 1 body = %+v", m.Arms[1].Body)
	}
	if n, ok := m.Arms[2].Body.(*ast.NameAtom); !ok || n.Name.String() != "session.token" {
		t.Errorf("Arm 2 body = %+v", m.Arms[2].Body)
	}
	if _, ok := m.Arms[3].Body.(*ast.NoneAtom); !ok {
		t.Errorf("Arm 3 body = %+v", m.Arms[3].Body)
	}
	if l, ok := m.Arms[4].Body.(*ast.Literal); !ok || l.Int != 1 {
		t.Errorf("Arm 4 body = %+v", m.Arms[4].Body)
	}
	if l, ok := m.Arms[5].Body.(*ast.Literal); !ok || l.Str != "x" {
		t.Errorf("Arm 5 body = %+v", m.Arms[5].Body)
	}
	if r, ok := m.Arms[6].Body.(*ast.RecordLit); !ok || len(r.Fields) != 2 {
		t.Errorf("Arm 6 body = %+v", m.Arms[6].Body)
	}
	th, ok := m.Arms[7].Body.(*ast.Throw)
	if !ok || th.Exception.String() != "type.Unauthorized" || !m.Arms[7].Else {
		t.Fatalf("Arm 7 body = %+v", m.Arms[7].Body)
	}
	// Throw の Span は throw から例外の名前まで。
	if th.Span.End != th.Exception.Span.End || th.Exception.Span.Start-th.Span.Start != source.Offset(len("throw ")) || len(th.Args) != 0 {
		t.Errorf("Throw Span = %+v, Exception = %+v", th.Span, th.Exception.Span)
	}
	// 引数を持つ throw（docs/grammar.md「配線」の Throw）。引数は Atom で、Span は閉じ括弧まで。
	tsrc := "fn a() { match (x by K) { A => throw rate.LimitExceeded(wait, 5, { a: b }); else => throw X; }; }"
	ta := firstDecl[*ast.OpDecl](t, tsrc).Body.Stmts[0].(*ast.MatchStmt).Match.Arms[0].Body.(*ast.Throw)
	if ta.Exception.String() != "rate.LimitExceeded" || len(ta.Args) != 3 {
		t.Fatalf("Throw = %+v", ta)
	}
	if _, ok := ta.Args[0].(*ast.NameAtom); !ok {
		t.Errorf("Args[0] = %+v", ta.Args[0])
	}
	if tsrc[ta.Span.End-1] != ')' {
		t.Errorf("Throw Span.End = %d", ta.Span.End)
	}
	// 値を対象にした標準の分類（対象は Atom のどれでも）。
	o = firstDecl[*ast.OpDecl](t, "fn b(in: In) { match (in.token by nullable) { None => f(); Some => g(in.token); }; }")
	if mt := o.Body.Stmts[0].(*ast.MatchStmt).Match; mt.Subject.(*ast.NameAtom).Name.String() != "in.token" || !mt.Builtin {
		t.Errorf("match = %+v", mt)
	}
}

// 状態での match（docs/grammar.md「配線」の表の二つ目と三つ目の形）。
func TestMatchOnState(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, `fn a() {
  match (carts by list) {
    Empty => showEmpty();
    HasItems => showCart();
  };
  val me = match (token by nullable) {
    None => none;
    Some => server.getMe();
  };
  match (by Checkout) {
    Shopping => f();
    Ordered, Shipped => g();
  };
  return match (by Session) { LoggedIn => h(); else => none; };
}`)
	byList := o.Body.Stmts[0].(*ast.MatchStmt).Match
	if byList.Subject.(*ast.NameAtom).Name.String() != "carts" || !byList.Builtin || byList.Classifier.String() != "list" || len(byList.Arms) != 2 {
		t.Errorf("by list = %+v", byList)
	}
	if byList.Arms[0].Cases[0].Name != "Empty" || byList.Arms[1].Cases[0].Name != "HasItems" {
		t.Errorf("by list arms = %+v", byList.Arms)
	}
	byNullable := o.Body.Stmts[1].(*ast.ValStmt).Value.(*ast.Match)
	if byNullable.Subject.(*ast.NameAtom).Name.String() != "token" || !byNullable.Builtin || byNullable.Classifier.String() != "nullable" {
		t.Errorf("by nullable = %+v", byNullable)
	}
	byClassify := o.Body.Stmts[2].(*ast.MatchStmt).Match
	if byClassify.Subject != nil || byClassify.Builtin || byClassify.Classifier.String() != "Checkout" || len(byClassify.Arms) != 2 {
		t.Errorf("by Checkout = %+v", byClassify)
	}
	ret := o.Body.Stmts[3].(*ast.ReturnStmt).Value.(*ast.Match)
	if ret.Subject != nil || ret.Classifier.String() != "Session" || !ret.Arms[1].Else {
		t.Errorf("return match = %+v", ret)
	}
	// by の後ろの予約語の Span は Classifier が持つ。
	if byList.Classifier.Span.End-byList.Classifier.Span.Start != source.Offset(len("list")) {
		t.Errorf("Classifier Span = %+v", byList.Classifier.Span)
	}
	// 構文は対象の有無と by の後ろを別々に読む。組み合わせが合うかは検査 MATCH-7。
	o = firstDecl[*ast.OpDecl](t, "fn b() { match (by list) { Empty => f(); HasItems => g(); }; match (carts by Checkout) { A => f(); }; }")
	if m := o.Body.Stmts[0].(*ast.MatchStmt).Match; m.Subject != nil || !m.Builtin {
		t.Errorf("match by list = %+v", m)
	}
	if m := o.Body.Stmts[1].(*ast.MatchStmt).Match; m.Subject == nil || m.Builtin {
		t.Errorf("match carts by Checkout = %+v", m)
	}
}

// TestStateConstructSpans は、状態を分類で表す構文の節（Transition、CaseSet、対象の並び、対象のない match）の Span が
// ファイルに収まり、親の中にあること（checkSpans。設計書の例の testdata にはこれらの構文がない）。
func TestStateConstructSpans(t *testing.T) {
	src := `namespace shop {
  state carts: list<Line>;
  state token: nullable<Token>;
  classify Checkout of carts, orders {
    Shopping;
    Ordered;
  }
  classify LoggedIn of token { Yes; No; }
  effect fn placeOrder(): effects Checkout as Shopping, Ordered -> any;
  effect fn clearCart(): effects shop.carts as any -> Empty;
  fn view(): Order? {
    match (token by nullable) { None => f(); Some => none; };
    val placed = match (by Checkout) { Shopping => submit(); Ordered => none; };
    return match (carts by list) { Empty, HasItems => g(); };
  }
}`
	fs := source.NewFileSet()
	id := fs.Add("t.hozo", []byte(src))
	file, diags := Parse(fs, id)
	for _, d := range diags {
		t.Errorf("予期しない診断: %s", d.Message)
	}
	checkSpans(t, fs, file)
	// 節の種類ごとに一つ以上たどれている
	seen := map[string]int{}
	walk(file, func(n ast.Node, _ ast.Node) {
		switch n.(type) {
		case *ast.Transition:
			seen["Transition"]++
		case *ast.CaseSet:
			seen["CaseSet"]++
		case *ast.Match:
			seen["Match"]++
		}
	})
	if seen["Transition"] != 2 || seen["CaseSet"] != 4 || seen["Match"] != 3 {
		t.Errorf("たどった節 = %v", seen)
	}
}

func TestBlockReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"nested call", "fn a() { val x = f(g()); }", "呼び出しの引数に呼び出しは書けない"},
		{"match without arms", "fn a() { match (x by K) { }; }", "match にはアームが一つ以上要る"},
		{"call without semicolon", "fn a() { f() g(); }", "「;」が必要"},
		{"arm body match", "fn a() { match (x by K) { A => match (y by L) { B => none; }; }; }", "アームの本体に match は書けない"},
		{"arm body missing", "fn a() { match (x by K) { A => ; }; }", "アームの本体（呼び出しか値か throw）"},
		{"arm throw without name", "fn a() { match (x by K) { A => throw; }; }", "名前が必要"},
		{"arm throw empty parens", "fn a() { match (x by K) { A => throw f(); }; }", "引数のない例外は括弧を書かない（throw f と書く"},
		{"arm throw call in args", "fn a() { match (x by K) { A => throw f(g()); }; }", "呼び出しの引数に呼び出しは書けない"},
		{"arm throw value", "fn a() { match (x by K) { A => throw 5; }; }", "名前が必要"},
		{"throw statement", "fn a() { throw X; }", "throw は match のアームの本体にだけ書ける"},
		{"return throw", "fn a(): T { return throw X; }", "throw は match のアームの本体にだけ書ける"},
		{"const throw", "fn a() { val x = throw X; }", "throw は match のアームの本体にだけ書ける"},
		{"arm without arrow", "fn a() { match (x by K) { A f(); }; }", "「=>」が必要"},
		{"if", "fn a() { if (x) { } }", "「;」が必要だが「{」がある"},
		{"literal statement", "fn a() { 5; }", "文（val / 値を入れる / 集める / return / 呼び出し / match / disjoint / for）が必要"},
		{"removed const", "fn a() { const x = g(); }", "const は、なくした書き方。val と書く"},
		{"removed match head", "fn a(x: M) { match x by C { A => f(); }; }", "match の頭は、全体を、丸括弧で囲む"},
		{"empty statement", "fn a() { f();; }", "「何もしない文」は、ない"},
		{"val with two values", "fn a() { val x = f(), y = g(); }", "値と一緒に宣言する val は、一つの文に、一つ"},
		{"disjoint with one arm", "fn a() { disjoint { f(); } }", "disjoint には、アームが二つ以上要る"},
		{"val as arm", "fn a() { disjoint { val x = f(); g(); } }", "文の一つのアームに、val の宣言は書けない"},
		{"return as arm", "fn a() { disjoint { return f(); g(); } }", "disjoint のアームに、return は書けない"},
		{"counting for", "fn a() { for (x of xs) { f(); } }", "for の頭は (val 名前 of 並び) の形だけ"},
		{"call in for head", "fn a() { for (val x of g()) { f(); } }", "for の並びに、呼び出しは書けない"},
		{"bare name statement", "fn a() { x; }", "「(」が必要"},
		{"record field value call", "fn a() { f({ x: g() }); }", "「,」か「}」"},
		{"record field qualified", "fn a() { f({ a.b }); }", "修飾しない識別子"},
		{"doc in block", "fn a() { /** d */ return x; }", "付く先のない"},
		{"return missing semicolon", "fn a() { return x }", "「;」が必要だが「}」がある"},
		{"match by single", "fn a() { match (s by single) { A => f(); }; }", "single は標準の分類ではない"},
		{"match by reserved segment", "fn a() { match (x by a.op) { A => f(); }; }", "op は予約語で、名前には使えない"},
		{"match by core type", "fn a() { match (s by string) { A => f(); }; }", "string は予約語"},
		{"match without by", "fn a() { match (x) { A => f(); }; }", "「by」が必要"},
		{"match two subjects", "fn a() { match (x, y by K) { A => f(); }; }", "「by」が必要"},
		{"match by nullable with type argument", "fn a() { match (s by nullable<T>) { None => f(); }; }", "「)」が必要"},
		{"match by without classifier", "fn a() { match (by) { A => f(); }; }", "名前が必要"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want == "「,」か「}」" {
				mustFail(t, tt.src, "「}」が必要だが「(」がある")
				return
			}
			mustFail(t, tt.src, tt.want)
		})
	}
}

// --- リテラル ---

func TestLiteral(t *testing.T) {
	tests := []struct {
		src   string
		check func(l *ast.Literal) bool
	}{
		{`"a\"b\n"`, func(l *ast.Literal) bool { return l.Kind == ast.StringLit && l.Str == "a\"b\n" && l.Raw == `"a\"b\n"` }},
		{`"\u{3042}"`, func(l *ast.Literal) bool { return l.Str == "あ" }},
		{"0", func(l *ast.Literal) bool { return l.Kind == ast.IntLit && l.Int == 0 }},
		{"-42", func(l *ast.Literal) bool { return l.Kind == ast.IntLit && l.Int == -42 }},
		{"9223372036854775807", func(l *ast.Literal) bool { return l.Int == 9223372036854775807 }},
		{"1.5", func(l *ast.Literal) bool { return l.Kind == ast.FloatLit && l.Float == 1.5 }},
		{"-0.25", func(l *ast.Literal) bool { return l.Float == -0.25 }},
		{"true", func(l *ast.Literal) bool { return l.Kind == ast.BoolLit && l.Bool }},
		{"false", func(l *ast.Literal) bool { return l.Kind == ast.BoolLit && !l.Bool }},
		{"5m", func(l *ast.Literal) bool { return l.Kind == ast.DurationLit && l.Duration == 5*60*1_000_000_000 }},
		{"30s", func(l *ast.Literal) bool { return l.Duration == 30*1_000_000_000 }},
		{"100ms", func(l *ast.Literal) bool { return l.Duration == 100*1_000_000 }},
		{"2h", func(l *ast.Literal) bool { return l.Duration == 2*3600*1_000_000_000 }},
		{"1d", func(l *ast.Literal) bool { return l.Duration == 24*3600*1_000_000_000 }},
	}
	for _, tt := range tests {
		m := firstDecl[*ast.ModelDecl](t, "@d("+tt.src+") model M {}")
		l := m.Decorators[0].Args[0].Value.Terms[0].Literal
		if l == nil || !tt.check(l) {
			t.Errorf("%s: Literal = %+v", tt.src, l)
		}
	}
	mustFail(t, "@d(9223372036854775808) model M {}", "整数が 64 ビットに収まらない")
	mustFail(t, "@d(none) model M {}", "引数の値（リテラルか名前）が必要")
}

// --- 識別子 ---

func TestIdentAndQualifiedName(t *testing.T) {
	e := firstDecl[*ast.EffectDecl](t, "effect fn post(): effects a.b.put;")
	if e.Name.Name != "post" {
		t.Errorf("文脈キーワードが識別子にならない: %+v", e.Name)
	}
	segs := e.On.State.Segments
	if len(segs) != 2 || segs[0].Name != "a" || segs[1].Name != "b" {
		t.Fatalf("Segments = %+v", segs)
	}
	start := source.Offset(len("effect fn post(): effects "))
	if segs[0].Span.Start != start || segs[0].Span.End != start+1 || segs[1].Span.Start != start+2 || segs[1].Span.End != start+3 {
		t.Errorf("Segment Spans = %+v %+v", segs[0].Span, segs[1].Span)
	}
	mustFail(t, "model string {}", "string は予約語で、識別子には使えない")
	mustFail(t, "model to {}", "to は予約語")
	mustFail(t, "scalar A extends a.string;", "string は予約語で、名前には使えない")
}

// TestFieldName は FieldName = Ident | 予約語（docs/grammar.md「識別子」）: フィールド名を書く位置
// （model のフィールド、その場で書く model、override の後ろの名前、レコードリテラルのキー、値のパスの二つ目以降）には
// 予約語を書ける。
func TestFieldName(t *testing.T) {
	// 予約語はすべてフィールド名になる（宣言の頭、修飾、節の頭、文の頭と値、核の型と形、リテラル）。
	var words []string
	for w := range token.Keywords {
		words = append(words, w)
	}
	sort.Strings(words)
	var b strings.Builder
	b.WriteString("model All {\n")
	for _, w := range words {
		b.WriteString("  " + w + ": string;\n")
	}
	b.WriteString("}")
	m := firstDecl[*ast.ModelDecl](t, b.String())
	if len(m.Body.Fields) != len(words) {
		t.Fatalf("Fields = %d, want %d", len(m.Body.Fields), len(words))
	}
	for i, w := range words {
		if f := m.Body.Fields[i]; f.Name.Name != w || f.Name.Span.End-f.Name.Span.Start != source.Offset(len(w)) {
			t.Errorf("Fields[%d].Name = %+v, want %s", i, f.Name, w)
		}
	}

	// デコレータ・説明・型との組み合わせ。フィールド名と同じ語が型の位置にあっても読める。
	m = firstDecl[*ast.ModelDecl](t, `model Wrap {
  /** 包む op */
  @valid.length(min: 1) op: string;
  string: string;
  list: Item[];
  decorator: string?;
  default: int;
  true: boolean;
}`)
	if len(m.Body.Fields) != 6 {
		t.Fatalf("Fields = %d", len(m.Body.Fields))
	}
	op := m.Body.Fields[0]
	if op.Name.Name != "op" || op.Doc == nil || len(op.Decorators) != 1 {
		t.Errorf("Fields[0] = %+v", op)
	}
	if ct, ok := m.Body.Fields[1].Type.(*ast.CoreType); !ok || ct.Name != "string" || m.Body.Fields[1].Name.Name != "string" {
		t.Errorf("Fields[1] = %+v", m.Body.Fields[1])
	}

	// その場で書く model（ユニオンのアーム）も同じ Field。
	u := firstDecl[*ast.UnionDecl](t, "union U { Ready: { state: string; match: int; }; }")
	if fs := u.Members[0].Type.Body.Fields; len(fs) != 2 || fs[0].Name.Name != "state" || fs[1].Name.Name != "match" {
		t.Errorf("arm body = %+v", fs)
	}

	// override の後ろの名前。
	m = firstDecl[*ast.ModelDecl](t, "model P extends U { override op: never; @d override namespace; override never: never; override hash: string; }")
	if ov := m.Body.Overrides; len(ov) != 4 || ov[0].Name.Name != "op" || ov[1].Name.Name != "namespace" || ov[2].Name.Name != "never" || !ov[2].Never || ov[3].Name.Name != "hash" {
		t.Errorf("Overrides = %+v", ov)
	}

	// レコードリテラルのキーと、値のパス（Path = Ident { "." FieldName }）。
	o := firstDecl[*ast.OpDecl](t, `fn f(res: Wrap): Wrap {
  val a = g({ op: res.op, state: res.state.model, match: "m", namespace: none, item });
  val b = match (res.match by Kind) { A => h(res.by); else => none; };
  return res.decorator;
}`)
	call := o.Body.Stmts[0].(*ast.ValStmt).Value.(*ast.Call)
	rec := call.Args[0].(*ast.RecordLit)
	var keys []string
	for _, f := range rec.Fields {
		keys = append(keys, f.Name.Name)
	}
	if strings.Join(keys, ",") != "op,state,match,namespace,item" {
		t.Errorf("keys = %v", keys)
	}
	if rec.Fields[4].Value != nil {
		t.Errorf("{ item } は値を省いた形: %+v", rec.Fields[4])
	}
	path := rec.Fields[1].Value.(*ast.NameAtom).Name
	if path.String() != "res.state.model" || len(path.Segments) != 3 {
		t.Fatalf("path = %+v", path)
	}
	// 各要素の Span は元の字句の中の位置。
	src := "fn f(res: Wrap): Wrap {\n  val a = g({ op: res.op, state: "
	if s := path.Segments[1].Span; s.Start != source.Offset(len(src)+len("res.")) || s.End != source.Offset(len(src)+len("res.state")) {
		t.Errorf("Segments[1].Span = %+v", s)
	}
	mt := o.Body.Stmts[1].(*ast.ValStmt).Value.(*ast.Match)
	if mt.Subject.(*ast.NameAtom).Name.String() != "res.match" || mt.Classifier.String() != "Kind" {
		t.Errorf("match = %+v", mt)
	}
	if arg := mt.Arms[0].Body.(*ast.Call).Args[0].(*ast.NameAtom).Name.String(); arg != "res.by" {
		t.Errorf("arm arg = %s", arg)
	}
	if ret := o.Body.Stmts[2].(*ast.ReturnStmt).Value.(*ast.NameAtom).Name.String(); ret != "res.decorator" {
		t.Errorf("return = %s", ret)
	}
}

// is と omit は予約語ではなくなった（docs/grammar.md「予約語」）: 識別子として、どの位置にも書ける。
func TestIsAndOmitAreIdents(t *testing.T) {
	file := mustParse(t, `model is { omit: string; }
fn omit(is: is): is { val omit = f(is); return omit; }
namespace is.omit { scalar Omit extends string; }`)
	if got := strings.Join(declNames(file), ","); got != "model is,op omit,namespace is.omit" {
		t.Errorf("decls = %s", got)
	}
}

// TestReservedWordReject は、フィールド名の位置のほかでは予約語が識別子にも名前にもならないこと
// （docs/grammar.md「予約語」）。
func TestReservedWordReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"declaration name", "model op {}", "op は予約語で、識別子には使えない"},
		{"op name", "fn state();", "state は予約語で、識別子には使えない"},
		{"op parameter", "fn f(op: string);", "op は予約語で、識別子には使えない"},
		{"decorator parameter", "decorator fn d(to: string);", "to は予約語で、識別子には使えない"},
		{"named argument", "@d(to: 1) fn f();", "to は予約語で、識別子には使えない"},
		{"const name", "fn f() { val match = g(); }", "match は予約語で、識別子には使えない"},
		{"union arm", "union U { model: string; }", "ユニオンのメンバー"},
		{"classify case", "classify C of T { state; }", "state は予約語で、識別子には使えない"},
		{"match arm case", "fn f() { match (x by C) { op => g(); }; }", "op は予約語で、識別子には使えない"},
		{"namespace segment", "namespace a.op {}", "op は予約語で、名前には使えない"},
		{"namespace head", "namespace op {}", "op は予約語で、名前には使えない"},
		{"type name", "model A { x: a.model; }", "model は予約語で、名前には使えない"},
		{"decorator name, not the last segment", "@a.op.x model A {}", "op は予約語で、名前には使えない"},
		{"callee", "fn f() { res.op(); }", "op は予約語で、名前には使えない"},
		{"callee in expr", "fn f() { val x = res.match(1); }", "match は予約語で、名前には使えない"},
		{"throws", "fn f(): throws a.none;", "none は予約語で、名前には使えない"},
		{"effect state", "effect fn e(): effects s.default;", "default は予約語で、名前には使えない"},
		{"bind target", "bind s to a.model.*;", "model は予約語で、名前には使えない"},
		{"decorator argument", "@d(a.op) fn f();", "op は予約語で、名前には使えない"},
		{"path head", "fn f() { g(op.x); }", "値（名前・リテラル・none・レコード）が必要だが「op」がある"},
		{"shorthand record key", "fn f() { g({ op }); }", "op は予約語なので、値を省いた形（{ op }）では書けない"},
		{"shorthand record key after field", "fn f() { g({ a: 1, state, b }); }", "state は予約語なので、値を省いた形"},
		{"qualified record key", "fn f() { g({ a.op: 1 }); }", "修飾しない識別子"},
		{"export as name", "fn export();", "export は予約語で、識別子には使えない"},
		{"reads as param", "fn f(reads: string);", "reads は予約語で、識別子には使えない"},
		{"as as const", "fn f() { val as = g(); }", "as は予約語で、識別子には使えない"},
		{"never as type name", "model A { x: a.never; }", "never は予約語で、名前には使えない"},
		{"throw as callee", "fn f() { throw(); }", "throw は match のアームの本体にだけ書ける"},
		{"external on reserved field", "model A { @external decorator: string; }", "@external はデコレータの宣言の直前にだけ書ける"},
		{"member not a field", "model A { fn f(); }", "「:」が必要だが識別子 fがある"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// --- ドキュメンテーションコメント ---

// TestDocTagShortName は、タグの語（@param）の中にも現れる短い名前（r、a、am）を、タグの語の後ろから探すこと。
func TestDocTagShortName(t *testing.T) {
	src := "/** @param r 触る行 */\nfn f(r: T);"
	o := firstDecl[*ast.OpDecl](t, src)
	if len(o.Doc.Tags) != 1 {
		t.Fatalf("Tags = %+v", o.Doc.Tags)
	}
	tag := o.Doc.Tags[0]
	if tag.Name.String() != "r" || tag.Text != "触る行" || string(src[tag.Name.Span.Start:tag.Name.Span.End]) != "r" || src[tag.Name.Span.Start-1] != ' ' {
		t.Errorf("Tag = %+v（名前の位置 %q）", tag, src[tag.Name.Span.Start:tag.Name.Span.End])
	}
}

func TestDocComment(t *testing.T) {
	src := `/**
 * パスワードの再設定メールを送る。
 * 二行目。
 *
 * 二つ目の段落。@deprecated は本文。
 *
 * @param email 再設定するアカウントのメールアドレス
 *   説明の続き
 * @throws server.RateLimited 同じアドレスに短時間で送りすぎた
 * @throws Other
 * @see login
 */
fn requestPasswordReset(email: Email);`
	o := firstDecl[*ast.OpDecl](t, src)
	d := o.Doc
	if d == nil {
		t.Fatal("Doc がない")
	}
	wantBody := "パスワードの再設定メールを送る。\n二行目。\n\n二つ目の段落。@deprecated は本文。"
	if d.Body != wantBody {
		t.Errorf("Body = %q, want %q", d.Body, wantBody)
	}
	if len(d.Tags) != 4 {
		t.Fatalf("Tags = %d: %+v", len(d.Tags), d.Tags)
	}
	p := d.Tags[0]
	if p.Kind != ast.DocParam || p.Name.String() != "email" || p.Text != "再設定するアカウントのメールアドレス\n説明の続き" {
		t.Errorf("Tag 0 = %+v", p)
	}
	th := d.Tags[1]
	if th.Kind != ast.DocThrows || th.Name.String() != "server.RateLimited" || th.Text != "同じアドレスに短時間で送りすぎた" {
		t.Errorf("Tag 1 = %+v", th)
	}
	if d.Tags[2].Kind != ast.DocThrows || d.Tags[2].Name.String() != "Other" || d.Tags[2].Text != "" {
		t.Errorf("Tag 2 = %+v", d.Tags[2])
	}
	see := d.Tags[3]
	if see.Kind != ast.DocSee || see.Name.String() != "login" || see.Text != "" {
		t.Errorf("Tag 3 = %+v", see)
	}
	// タグの Span はその行の "@" から。名前の Span もファイルの位置。
	if string(src[see.Span.Start:see.Span.End]) != "@see login" {
		t.Errorf("@see Span = %q", src[see.Span.Start:see.Span.End])
	}
	if string(src[th.Name.Span.Start:th.Name.Span.End]) != "server.RateLimited" {
		t.Errorf("@throws Name Span = %q", src[th.Name.Span.Start:th.Name.Span.End])
	}
	if d.Span.Start != 0 || d.Span.End != source.Offset(strings.Index(src, "*/")+2) {
		t.Errorf("Doc Span = %+v", d.Span)
	}
	if d.Raw != src[3:strings.Index(src, "*/")] {
		t.Errorf("Raw = %q", d.Raw)
	}
}

func TestDocCommentOneLine(t *testing.T) {
	m := firstDecl[*ast.ModelDecl](t, "/** 要約だけ */ model A {}")
	if m.Doc.Body != "要約だけ" || len(m.Doc.Tags) != 0 {
		t.Errorf("Doc = %+v", m.Doc)
	}
	m = firstDecl[*ast.ModelDecl](t, "/***/ model A {}")
	if m.Doc == nil || m.Doc.Body != "" {
		t.Errorf("Doc = %+v", m.Doc)
	}
	m = firstDecl[*ast.ModelDecl](t, "/** @see a.b */ model A {}")
	if m.Doc.Body != "" || len(m.Doc.Tags) != 1 || m.Doc.Tags[0].Name.String() != "a.b" {
		t.Errorf("Doc = %+v", m.Doc)
	}
}

func TestDocCommentReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"param without name", "/** @param */ model A {}", "@param の後ろに名前がない"},
		{"param qualified", "/** @param a.b x */ model A {}", "@param の名前の形が正しくない"},
		{"see with text", "/** @see a b */ model A {}", "@see は名前だけを書く"},
		{"throws bad name", "/** @throws 1x */ model A {}", "@throws の名前の形が正しくない"},
		{"see keyword not last", "/** @see string.x */ model A {}", "@see の名前の形が正しくない"},
		{"throws keyword", "/** @throws db.list */ model A {}", "@throws の名前の形が正しくない"},
		{"param keyword", "/** @param list x */ model A {}", "@param の名前の形が正しくない"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := mustFail(t, tt.src, tt.want)
			if len(diags) != 1 {
				t.Errorf("診断の数 = %d（タグの誤りで宣言は落とさない）", len(diags))
			}
		})
	}
	// 誤ったタグの行があっても宣言は残る。
	file, _ := parseSrc(t, "/** @param */ model A {}")
	if len(file.Decls) != 1 {
		t.Errorf("Decls = %d", len(file.Decls))
	}
}

// --- 回復（docs/packages.md「internal/syntax/token、internal/syntax/ast、internal/syntax」） ---

func declNames(file *ast.File) []string {
	var out []string
	for _, d := range file.Decls {
		switch x := d.(type) {
		case *ast.ModelDecl:
			out = append(out, "model "+x.Name.Name)
		case *ast.ScalarDecl:
			out = append(out, "scalar "+x.Name.Name)
		case *ast.OpDecl:
			out = append(out, "op "+x.Name.Name)
		case *ast.NamespaceDecl:
			out = append(out, "namespace "+x.Name.String())
		case *ast.ExceptionDecl:
			out = append(out, "exception "+x.Name.Name)
		case *ast.EffectDecl:
			out = append(out, "effect "+x.Name.Name)
		default:
			out = append(out, "?")
		}
	}
	return out
}

func TestRecoverDecls(t *testing.T) {
	src := `model A {}
scalar B extends;
model C {}
fn D(x int) { return x; }
model E {}
exception F
model G {}
`
	file, diags := parseSrc(t, src)
	// exception F の ";" がないので、次の ";" か "}" までの model G は読み飛ばされる。
	if got := strings.Join(declNames(file), ", "); got != "model A, model C, model E" {
		t.Errorf("Decls = %s", got)
	}
	if len(diags) != 3 {
		t.Errorf("診断の数 = %d: %+v", len(diags), diags)
	}
	want := []string{"型が必要だが「;」がある", "「:」が必要だが「int」がある", "「;」が必要だが「model」がある"}
	for i, w := range want {
		if i < len(diags) && !strings.Contains(diags[i].Message, w) {
			t.Errorf("diags[%d] = %q, want contains %q", i, diags[i].Message, w)
		}
	}
}

func TestRecoverInsideNamespace(t *testing.T) {
	src := `namespace server {
  model A {}
  state B list<X>;
  model C {}
}
model D {}
`
	file, diags := parseSrc(t, src)
	if got := strings.Join(declNames(file), ", "); got != "namespace server, model D" {
		t.Fatalf("Decls = %s", got)
	}
	ns := file.Decls[0].(*ast.NamespaceDecl)
	if got := strings.Join(declNames(&ast.File{Decls: ns.Decls}), ", "); got != "model A, model C" {
		t.Errorf("namespace の Decls = %s", got)
	}
	if len(diags) != 1 {
		t.Errorf("診断の数 = %d", len(diags))
	}
}

func TestRecoverMembers(t *testing.T) {
	src := `model M {
  a: string;
  b string;
  c: int;
  /** doc */ d;
  e: T? = ;
  f: bool;
}
model N {}
`
	file, diags := parseSrc(t, src)
	if got := strings.Join(declNames(file), ", "); got != "model M, model N" {
		t.Fatalf("Decls = %s", got)
	}
	m := file.Decls[0].(*ast.ModelDecl)
	var fields []string
	for _, f := range m.Body.Fields {
		fields = append(fields, f.Name.Name)
	}
	if got := strings.Join(fields, ","); got != "a,c,f" {
		t.Errorf("Fields = %s", got)
	}
	if len(diags) != 3 {
		t.Errorf("診断の数 = %d: %+v", len(diags), diags)
	}
}

func TestRecoverStmts(t *testing.T) {
	src := `fn a(x: X): Y {
  val a = f();
  val = g();
  h(x;
  val c = match (x by K) {
    A => f();
    B x;
    C => none;
  };
  return c;
}
model N {}
`
	file, diags := parseSrc(t, src)
	if got := strings.Join(declNames(file), ", "); got != "op a, model N" {
		t.Fatalf("Decls = %s", got)
	}
	o := file.Decls[0].(*ast.OpDecl)
	if len(o.Body.Stmts) != 3 {
		t.Errorf("Stmts = %d", len(o.Body.Stmts))
	}
	m := o.Body.Stmts[1].(*ast.ValStmt).Value.(*ast.Match)
	if len(m.Arms) != 2 {
		t.Errorf("Arms = %d", len(m.Arms))
	}
	if len(diags) != 3 {
		t.Errorf("診断の数 = %d: %+v", len(diags), diags)
	}
}

// TestRecoverInsideRecord は、レコードリテラルの中の誤りからの回復: 読みかけのレコードリテラルを閉じてから
// 次の ";" まで読み飛ばす。レコードの "}" をブロックの終わりと取り違えず、SYNTAX は一つで、後ろの文も宣言も残る。
func TestRecoverInsideRecord(t *testing.T) {
	src := `fn a(x: X): Y {
  val a = f({ op, state: x.state });
  val b = f({ in: { op: g() }, rest: x });
  val c = match (x by K) {
    A => f({ a: { state } });
    B => none;
  };
  return f({ op: x.op });
}
model N {}
`
	file, diags := parseSrc(t, src)
	if got := strings.Join(declNames(file), ", "); got != "op a, model N" {
		t.Fatalf("Decls = %s", got)
	}
	o := file.Decls[0].(*ast.OpDecl)
	if len(o.Body.Stmts) != 2 {
		t.Fatalf("Stmts = %d", len(o.Body.Stmts))
	}
	if m := o.Body.Stmts[0].(*ast.ValStmt).Value.(*ast.Match); len(m.Arms) != 1 {
		t.Errorf("Arms = %d", len(m.Arms))
	}
	if _, ok := o.Body.Stmts[1].(*ast.ReturnStmt); !ok {
		t.Errorf("Stmts[1] = %T", o.Body.Stmts[1])
	}
	if len(diags) != 3 {
		t.Errorf("診断の数 = %d: %+v", len(diags), diags)
	}
}

func TestRecoverUnionAndClassify(t *testing.T) {
	src := `union U { A: X; B; C: ; D; }
classify K of T { A; 5; B; }
`
	file, diags := parseSrc(t, src)
	if len(file.Decls) != 2 {
		t.Fatalf("Decls = %d", len(file.Decls))
	}
	if u := file.Decls[0].(*ast.UnionDecl); len(u.Members) != 3 {
		t.Errorf("Members = %d", len(u.Members))
	}
	if c := file.Decls[1].(*ast.ClassifyDecl); len(c.Cases) != 2 {
		t.Errorf("Cases = %d", len(c.Cases))
	}
	if len(diags) != 2 {
		t.Errorf("診断の数 = %d", len(diags))
	}
}

func TestRecoverLexError(t *testing.T) {
	// 字句の誤りは字句解析器が一つ出し、構文解析器は重ねて出さない。
	src := "model A { @d(\"a\\qb\") x: string; }\nmodel B {}\nmodel C { @d(5w) y: int; }\nmodel D {}"
	file, diags := parseSrc(t, src)
	if got := strings.Join(declNames(file), ", "); got != "model A, model B, model C, model D" {
		t.Errorf("Decls = %s", got)
	}
	if len(diags) != 2 {
		t.Errorf("診断の数 = %d: %+v", len(diags), diags)
	}
	for _, d := range diags {
		if !strings.Contains(d.Message, "エスケープ") && !strings.Contains(d.Message, "数値のリテラルではない") {
			t.Errorf("予期しない診断: %s", d.Message)
		}
	}
	// 誤ったフィールドだけが落ち、model は残る。
	if a := file.Decls[0].(*ast.ModelDecl); len(a.Body.Fields) != 0 {
		t.Errorf("model A の Fields = %d", len(a.Body.Fields))
	}
}

func TestRecoverUnclosedAtEOF(t *testing.T) {
	file, diags := parseSrc(t, "model A {\n  x: int;\n")
	if len(file.Decls) != 0 || len(diags) != 1 || !strings.Contains(diags[0].Message, "「}」が必要だがファイルの終わり") {
		t.Errorf("Decls = %d, diags = %+v", len(file.Decls), diags)
	}
	file, diags = parseSrc(t, "namespace a {\n model B {}\n")
	if len(file.Decls) != 1 || len(diags) != 1 {
		t.Errorf("Decls = %d, diags = %+v", len(file.Decls), diags)
	}
}

func TestDiagnosticSpan(t *testing.T) {
	src := "model A {\n  x string;\n}"
	_, diags := parseSrc(t, src)
	if len(diags) != 1 {
		t.Fatalf("diags = %+v", diags)
	}
	s := diags[0].Span
	if string(src[s.Start:s.End]) != "string" {
		t.Errorf("Span は誤った字句を指すはず: %q", src[s.Start:s.End])
	}
}

// TestWiringBlocks は、配線の、R6 で足した文（docs/grammar.md「配線」）: 名前を並べた空の val、値を入れる文、集める文、
// disjoint（ブロックのアームと、文の一つのアーム）、for、disjoint for。"}" で終わる文の ";" は、省ける。
func TestWiringBlocks(t *testing.T) {
	src := `fn page(in: In): Page {
  val session = authenticate(in.token);
  val user, latest: Order?, orders;
  disjoint {
    {
      val found = findUser(session.userId);
      user = match (found by Account) {
        Found   => toPublic(found);
        Missing => throw Unauthorized;
      }
    }
    latest = latestOrder(session.userId);
    orders = listOrders(session.userId);
  }
  val summaries: Summary[];
  disjoint for (val order of orders) {
    summaries collect summarize(order);
  }
  for (val o of orders) {
    notify(o);
  };
  return page(user, latest, summaries);
}`
	o := firstDecl[*ast.OpDecl](t, src)
	st := o.Body.Stmts
	if len(st) != 9 {
		t.Fatalf("Stmts = %d", len(st))
	}
	for i, name := range []string{"user", "latest", "orders"} {
		v, ok := st[1+i].(*ast.ValStmt)
		if !ok || v.Name.Name != name || v.Value != nil || (v.Type != nil) != (name == "latest") {
			t.Errorf("Stmt %d = %+v（期待は、空の val %s）", 1+i, st[1+i], name)
		}
	}
	d, ok := st[4].(*ast.DisjointStmt)
	if !ok || len(d.Arms) != 3 || !d.Arms[0].Block || d.Arms[1].Block || len(d.Arms[0].Stmts) != 2 {
		t.Fatalf("Stmt 4 = %+v", st[4])
	}
	if a, ok := d.Arms[0].Stmts[1].(*ast.AssignStmt); !ok || a.Name.Name != "user" {
		t.Errorf("アーム 0 の文 1 = %+v", d.Arms[0].Stmts[1])
	} else if _, isMatch := a.Value.(*ast.Match); !isMatch {
		t.Errorf("user に入れる式 = %+v", a.Value)
	}
	if a, ok := d.Arms[2].Stmts[0].(*ast.AssignStmt); !ok || a.Name.Name != "orders" {
		t.Errorf("アーム 2 = %+v", d.Arms[2].Stmts[0])
	}
	if v, ok := st[5].(*ast.ValStmt); !ok || v.Type == nil || v.Value != nil {
		t.Errorf("Stmt 5 = %+v", st[5])
	}
	f, ok := st[6].(*ast.ForStmt)
	if !ok || !f.Disjoint || f.Var.Name != "order" || len(f.Body.Stmts) != 1 {
		t.Fatalf("Stmt 6 = %+v", st[6])
	}
	if c, ok := f.Body.Stmts[0].(*ast.CollectStmt); !ok || c.Name.Name != "summaries" {
		t.Errorf("for の中 = %+v", f.Body.Stmts[0])
	}
	if f2, ok := st[7].(*ast.ForStmt); !ok || f2.Disjoint || f2.Var.Name != "o" {
		t.Errorf("Stmt 7 = %+v", st[7])
	}
	if _, ok := st[8].(*ast.ReturnStmt); !ok {
		t.Errorf("Stmt 8 = %+v", st[8])
	}
}

// TestClassifyNeverAndCaseKind は、対象のない分類（of never）、ケースの前のデコレータ、デコレータの宣言の、分類のケース
// （docs/grammar.md「classify」「decorator の宣言」の CaseKind）。
func TestClassifyNeverAndCaseKind(t *testing.T) {
	c := firstDecl[*ast.ClassifyDecl](t, "classify Actor of never { /** 客 */ @human Customer; Staff; }")
	if !c.Never || len(c.Of) != 0 || len(c.Cases) != 2 || len(c.Cases[0].Decorators) != 1 || c.Cases[0].Doc == nil || len(c.Cases[1].Decorators) != 0 {
		t.Fatalf("classify = %+v", c)
	}
	d := firstDecl[*ast.DecoratorDecl](t, "decorator of(actor: any case of never, other: any case of Actor, free: any case?, all: any case of any?, opt: any case of M?, inner: any case of M?[], both: any case of M?[]?) for op, case, case of x.Actor, case of never, field, case of any, case of M[];")
	if len(d.Params) != 7 || len(d.For) != 7 {
		t.Fatalf("decorator = %+v", d)
	}
	named := func(k *ast.CaseKind) string {
		if k == nil || k.Of == nil {
			return ""
		}
		return typeString(k.Of)
	}
	p0 := d.Params[0].Type.Any
	if p0 == nil || p0.Kind != ast.AnyCase || p0.Case == nil || !p0.Case.Never || p0.Case.Of != nil {
		t.Errorf("Param 0 = %+v", d.Params[0].Type)
	}
	p1 := d.Params[1].Type.Any
	if p1 == nil || p1.Kind != ast.AnyCase || p1.Case == nil || p1.Case.Never || named(p1.Case) != "Actor" || d.Params[1].Type.Optional {
		t.Errorf("Param 1 = %+v", d.Params[1].Type)
	}
	// case は case of any の省略: どちらも、絞りを持たない。後ろの ? は、引数を省ける印
	for _, i := range []int{2, 3} {
		p := d.Params[i].Type.Any
		if p == nil || p.Kind != ast.AnyCase || p.Case == nil || p.Case.Never || p.Case.Of != nil || !d.Params[i].Type.Optional {
			t.Errorf("Param %d = %+v", i, d.Params[i].Type)
		}
	}
	// いちばん後ろの ? は、引数を省ける印（docs/decisions.md「決めたこと」354）: 対象の型は M で、引数は省ける。
	// 内側の ? は、型の一部
	for _, tt := range []struct {
		i        int
		subject  string
		optional bool
	}{{4, "M", true}, {5, "M?[]", false}, {6, "M?[]", true}} {
		p := d.Params[tt.i].Type
		if p.Any == nil || p.Any.Kind != ast.AnyCase || named(p.Any.Case) != tt.subject || p.Optional != tt.optional {
			t.Errorf("Param %d = %+v（対象 %s、省ける %v のはず）", tt.i, p, tt.subject, tt.optional)
		}
	}
	if k := d.For[1]; k.Kind != ast.TargetCase || k.Case == nil || k.Case.Never || k.Case.Of != nil {
		t.Errorf("Target 1 = %+v", k)
	}
	if k := d.For[2]; k.Kind != ast.TargetCase || k.Case == nil || k.Case.Never || named(k.Case) != "x.Actor" {
		t.Errorf("Target 2 = %+v", k)
	}
	if k := d.For[3]; k.Kind != ast.TargetCase || k.Case == nil || !k.Case.Never || k.Case.Of != nil {
		t.Errorf("Target 3 = %+v", k)
	}
	if k := d.For[4]; k.Kind != ast.TargetField || k.Case != nil {
		t.Errorf("Target 4 = %+v", k)
	}
	if k := d.For[5]; k.Kind != ast.TargetCase || k.Case == nil || k.Case.Never || k.Case.Of != nil {
		t.Errorf("Target 5 = %+v", k)
	}
	if k := d.For[6]; k.Kind != ast.TargetCase || named(k.Case) != "M[]" {
		t.Errorf("Target 6 = %+v", k)
	}

	const removed = "分類のケースは、case of any（省略して case）、case of never、case of 分類の名前 と書く"
	tests := []struct{ name, src, want string }{
		{"of without a type", "decorator d for case of;", "型"},
		{"name before case", "decorator d for Actor case;", "for の対象"},
		// 型で絞る形の対象に、省略可能な型は、書けない（名前で絞る形を示す）
		{"optional subject on target", "decorator d for case of M?;", "型で絞る形（case of 型）の対象に、省略可能な型は書けない"},
		{"nullable subject on target", "decorator d for case of nullable<M>;", "省略可能な型を対象にする分類のケースは、名前で絞る（case of 分類の名前）"},
		{"nullable subject on param", "decorator fn d(x: any case of nullable<M>);", "型で絞る形（case of 型）の対象に、省略可能な型は書けない"},
		{"doubly optional subject on param", "decorator fn d(x: any case of M??);", "型で絞る形（case of 型）の対象に、省略可能な型は書けない"},
		{"optional rest", "decorator fn d(...xs: any case of M?);", "残りの引数 ...xs は省略可能にできない"},
		{"removed: classify case", "decorator d for classify case;", removed},
		{"removed: classify of never case", "decorator d for classify of never case;", removed},
		{"removed: any classify Actor case", "decorator fn d(x: any classify Actor case);", removed},
		{"classify itself", "decorator d for classify;", "分類そのもの（classify）は、まだ、書けない"},
		{"any classify itself", "decorator fn d(x: any classify);", "分類そのもの（classify）は、まだ、書けない"},
		{"name without case", "decorator d for Actor;", "for の対象"},
		{"any name without case", "decorator fn d(x: any Actor);", "any の種類"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// TestDecoratorTargets は、付ける先の並び（docs/grammar.md「decorator の宣言」）: 区切りは ","。なくした区切り "|" は、
// "," として読み進める（対象は、どちらも読める）。核の型の語（datetime、date、time、duration）は、型の位置で読める。
func TestDecoratorTargets(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, "decorator context(name: string) for model, state, op;")
	if len(d.For) != 3 || d.For[0].Kind != ast.TargetModel || d.For[1].Kind != ast.TargetState || d.For[2].Kind != ast.TargetOp || d.Fn {
		t.Errorf("For = %+v, Fn = %v", d.For, d.Fn)
	}
	file, diags := parseSrc(t, "decorator d for model | state;")
	if len(diags) != 1 || len(file.Decls) != 1 || len(file.Decls[0].(*ast.DecoratorDecl).For) != 2 {
		t.Errorf("なくした区切りの後: 診断 %d 件、宣言 %d 件", len(diags), len(file.Decls))
	}
	m := firstDecl[*ast.ModelDecl](t, "model Slot { at: datetime; on: date; from: time?; span: duration[]; }")
	for i, want := range []string{"datetime", "date", "time?", "duration[]"} {
		if got := typeString(m.Body.Fields[i].Type); got != want {
			t.Errorf("フィールド %d の型 = %q, want %q", i, got, want)
		}
	}
	// 核の型の語は、予約語: 関数、引数、val の名前には、使えない。フィールドの名前には、書ける
	mustFail(t, "fn date();", "date は予約語で、識別子には使えない")
	mustFail(t, "fn f(time: string);", "time は予約語で、識別子には使えない")
	firstDecl[*ast.ModelDecl](t, "model M { time: time; date: date; datetime: datetime; duration: duration; }")
}

// TestDecoratorFn は、関数に付けるデコレータ（docs/grammar.md「decorator の宣言」「デコレータの本体」）: decorator fn の頭、
// 「)」の後ろの形（節は「:」の後ろ。節がなければ「:」も書かない）、本体、なくした書き方からの読み進め。
func TestDecoratorFn(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, "decorator fn post(path: string);")
	if !d.Fn || d.Name.Name != "post" || len(d.Params) != 1 || len(d.For) != 0 || len(d.Clauses) != 0 || d.Body != nil {
		t.Errorf("decorator fn post = %+v", d)
	}
	if got := d.FnSpan; got.Start != source.Offset(len("decorator ")) || got.End-got.Start != 2 {
		t.Errorf("FnSpan = %+v", got)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator fn limited(): throws LimitExceeded prevents effects;")
	if !d.Fn || len(d.Params) != 0 || len(d.Clauses) != 1 || d.Body != nil {
		t.Errorf("decorator fn limited = %+v", d)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator fn limited(): throws LimitExceeded { val a = allowance(); match (a by Allowance) { Allowed => proceed(); Denied => throw LimitExceeded; } }")
	if !d.Fn || len(d.Clauses) != 1 || d.Body == nil || len(d.Body.Stmts) != 2 {
		t.Fatalf("本体のある decorator fn = %+v", d)
	}
	arm := d.Body.Stmts[1].(*ast.MatchStmt).Match.Arms[0]
	if call, ok := arm.Body.(*ast.Call); !ok || call.Callee.String() != "proceed" || len(call.Args) != 0 {
		t.Errorf("アームの本体 = %+v", arm.Body)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator fn audited() { proceed(); record(); }")
	if d.Body == nil || len(d.Body.Stmts) != 2 || len(d.Clauses) != 0 {
		t.Errorf("節のない、本体のある decorator fn = %+v", d)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "@entry @repeatable decorator fn get(path: string): outer throws ServerError;")
	if !d.Fn || !d.Entry || !d.Repeatable {
		t.Errorf("核の印の付いた decorator fn = %+v", d)
	}

	// なくした書き方は、新しい書き方を示して SYNTAX にし、decorator fn として読み進める
	for _, tt := range []struct{ src, want string }{
		{"decorator limited for op throws X prevents effects;", "decorator limited for op … という書き方はなくなった。関数に付けるデコレータは decorator fn limited(…) と書く"},
		{"decorator limited throws X;", "付ける先を書かない decorator limited … という書き方はなくなった"},
		{"decorator traced;", "付ける先を書かない decorator traced … という書き方はなくなった"},
	} {
		file, diags := parseSrc(t, tt.src)
		if len(diags) != 1 || !strings.Contains(diags[0].Message, tt.want) {
			t.Errorf("%s: 診断 = %+v, want %q", tt.src, diags, tt.want)
			continue
		}
		if len(file.Decls) != 1 || !file.Decls[0].(*ast.DecoratorDecl).Fn {
			t.Errorf("%s: 読み進めた宣言が decorator fn でない", tt.src)
		}
	}

	rejects := []struct{ name, src, want string }{
		{"no parens", "decorator fn a;", "decorator fn の引数の「(」が要る。引数がなくても書く（decorator fn a() と書く"},
		{"clause without colon", "decorator fn a() throws X;", "「)」の後ろに、節を書くなら、「:」で始める（decorator fn a(…): throws … と書く"},
		{"colon without clause", "decorator fn a():;", "「:」の後ろに、節が要る。何も書かないなら、「:」も書かない"},
		{"colon before body", "decorator fn a(): { proceed(); }", "「:」の後ろに、節が要る。何も書かないなら、「:」も書かない"},
		{"return type", "decorator fn a(): string;", "decorator fn に、戻り値は書かない"},
		{"for after fn", "decorator fn a() for op;", "decorator fn に for は書かない"},
		{"fn alias", "decorator fn a = b();", "名前付きのデコレータに fn は書かない"},
		{"clause on for form", "decorator a for model, op throws X;", "for で付ける先を書いたデコレータは、節も本体も持てない"},
		{"body on for form", "decorator a for state { proceed(); }", "for で付ける先を書いたデコレータは、節も本体も持てない"},
		{"no end", "decorator fn a()", "「;」かデコレータの本体の「{」が必要"},
	}
	for _, tt := range rejects {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}
