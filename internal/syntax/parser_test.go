package syntax

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
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
	case *ast.RecordType:
		return "Record<" + typeString(x.Key) + ", " + typeString(x.Value) + ">"
	case *ast.NamedType:
		if x.Arg != nil {
			return x.Name.String() + "<" + typeString(x.Arg) + ">"
		}
		return x.Name.String()
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
		{"decorator on bind", "@db bind a to b.*;", "bind にデコレータは付けられない"},
		{"not a decl", "foo;", "宣言が必要"},
		{"stray close brace", "model A {} }", "対応する「{」のない「}」"},
		{"external on model", "@external model A {}", "@external はデコレータの宣言の直前にだけ書ける"},
		{"external with args", "@external() decorator x;", "@external は引数を取らない"},
		{"external before other decorator", "@external @db decorator x;", "@external はデコレータの宣言の直前にだけ書ける"},
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
	if len(ns.Decorators) != 0 {
		t.Errorf("Decorators = %+v", ns.Decorators)
	}
	mustFail(t, "namespace a { model A {}", "namespace a の「}」がない")
	mustFail(t, "namespace { }", "名前が必要")
}

// TestNamespaceDecorators は namespace ブロックの前のデコレータ（docs/grammar.md「ファイルと宣言」の
// Decl = [ DocComment ] { Decorator } NamespaceDecl）。
func TestNamespaceDecorators(t *testing.T) {
	src := "/** 注文の文脈 */\n@ctx.bounded(\"注文\") @core\nnamespace shop {\n @inner namespace admin { }\n}"
	ns := firstDecl[*ast.NamespaceDecl](t, src)
	if ns.Doc == nil || ns.Doc.Body != "注文の文脈" {
		t.Errorf("Doc = %+v", ns.Doc)
	}
	if len(ns.Decorators) != 2 || ns.Decorators[0].Name.String() != "ctx.bounded" || len(ns.Decorators[0].Args) != 1 || ns.Decorators[1].Name.String() != "core" {
		t.Fatalf("Decorators = %+v", ns.Decorators)
	}
	// 宣言の Span は DocComment を含まず、最初のデコレータから "}" まで。
	if ns.Span.Start != source.Offset(len("/** 注文の文脈 */\n")) || ns.Span.End != source.Offset(len(src)) {
		t.Errorf("Span = %+v", ns.Span)
	}
	inner, ok := ns.Decls[0].(*ast.NamespaceDecl)
	if !ok || len(inner.Decorators) != 1 || inner.Decorators[0].Name.String() != "inner" {
		t.Errorf("inner = %+v", ns.Decls[0])
	}
	mustFail(t, "@db /** x */ namespace a {}", "デコレータの前に置く")
	mustFail(t, "@external namespace a {}", "@external はデコレータの宣言の直前にだけ書ける")
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
		{"no name", `@ model M {}`, "予約語で、名前には使えない"},
		{"unclosed list", `@a([1) model M {}`, "「]」が必要"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// --- model ---

func TestModel(t *testing.T) {
	m := firstDecl[*ast.ModelDecl](t, "model User {\n  /** 鍵 */\n  @db.key id: UserId;\n  name: string = \"n\";\n  page: PosInt = 1;\n  field: string;\n  tags: string[]?;\n}")
	if m.Name.Name != "User" || m.Body == nil || m.Base != nil {
		t.Fatalf("Model = %+v", m)
	}
	f := m.Body.Fields
	if len(f) != 5 {
		t.Fatalf("Fields = %d", len(f))
	}
	if f[0].Doc == nil || f[0].Doc.Body != "鍵" || len(f[0].Decorators) != 1 || f[0].Name.Name != "id" || typeString(f[0].Type) != "UserId" || f[0].Default != nil {
		t.Errorf("Field 0 = %+v", f[0])
	}
	if f[1].Default == nil || f[1].Default.Kind != ast.StringLit || f[1].Default.Str != "n" {
		t.Errorf("Field 1 default = %+v", f[1].Default)
	}
	if f[2].Default == nil || f[2].Default.Int != 1 {
		t.Errorf("Field 2 default = %+v", f[2].Default)
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

func TestModelIsOmit(t *testing.T) {
	m := firstDecl[*ast.ModelDecl](t, "model PublicUser is server.User omit hash, salt;")
	if m.Body != nil || m.Base == nil || m.Base.String() != "server.User" {
		t.Fatalf("Model = %+v", m)
	}
	if len(m.Omit) != 2 || m.Omit[0].Name != "hash" || m.Omit[1].Name != "salt" {
		t.Errorf("Omit = %+v", m.Omit)
	}
}

func TestModelReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"is without omit", "model A is B;", "omit のない is は書けない"},
		{"optional on name", "model A { x?: T; }", "「:」が必要だが「?」がある"},
		{"missing semicolon", "model A { x: T }", "「;」が必要だが「}」がある"},
		{"missing type", "model A { x: ; }", "型が必要"},
		{"default not literal", "model A { x: int = y; }", "リテラルが必要"},
		{"no body", "model A;", "「{」か「is」が必要"},
		{"reserved field name", "model A { namespace: string; }", "予約語で、識別子には使えない"},
		{"qualified field name", "model A { a.b: string; }", "修飾しない識別子"},
		{"dangling doc in body", "model A { /** x */ }", "付く先のない"},
		{"external on field", "model A { @external x: int; }", "@external はデコレータの宣言の直前にだけ書ける"},
	}
	for _, tt := range tests {
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
	if s.Name.Name != "users" || s.Shape != ast.StateList || typeString(s.Elem) != "User" {
		t.Errorf("State = %+v", s)
	}
	s = firstDecl[*ast.StateDecl](t, "state query: single<Record<string, string[]>>;")
	if s.Shape != ast.StateSingle || typeString(s.Elem) != "Record<string, string[]>" {
		t.Errorf("State = %+v %s", s, typeString(s.Elem))
	}
	s = firstDecl[*ast.StateDecl](t, "state token: nullable<server.Token>;")
	if s.Shape != ast.StateNullable || s.Shape.String() != "nullable" || typeString(s.Elem) != "server.Token" {
		t.Errorf("State = %+v %s", s, typeString(s.Elem))
	}
	// single<T?> は構文としては読める。書けないことは検査 STATE-3 が決める（docs/grammar.md「state」）。
	s = firstDecl[*ast.StateDecl](t, "state token: single<Token?>;")
	if typeString(s.Elem) != "Token?" {
		t.Errorf("Elem = %s", typeString(s.Elem))
	}
	mustFail(t, "state users: User[];", "「list」か「nullable」か「single」が必要")
	mustFail(t, "state users: list<User;", "「>」が必要")
	mustFail(t, "state token: nullable Token;", "「<」が必要")
}

// --- exception / effect ---

func TestException(t *testing.T) {
	e := firstDecl[*ast.ExceptionDecl](t, "@http.status(401) exception Unauthorized;")
	if e.Name.Name != "Unauthorized" || e.Value != nil {
		t.Errorf("Exception = %+v", e)
	}
	e = firstDecl[*ast.ExceptionDecl](t, "exception ClientError: Violations;")
	if e.Value == nil || e.Value.Type == nil || typeString(e.Value.Type) != "Violations" {
		t.Errorf("Exception = %+v", e)
	}
	e = firstDecl[*ast.ExceptionDecl](t, "exception X: { items: Violation[]; };")
	if e.Value == nil || e.Value.Body == nil || len(e.Value.Body.Fields) != 1 {
		t.Errorf("Exception = %+v", e)
	}
	mustFail(t, "exception X: ;", "型が必要")
	mustFail(t, "exception X", "「;」が必要")
}

func TestEffect(t *testing.T) {
	for _, tt := range []struct {
		src  string
		op   ast.StateOp
		stat string
	}{
		{"effect createSession on sessions.post;", ast.Post, "sessions"},
		{"effect a on location.put;", ast.Put, "location"},
		{"effect b on server.users.delete;", ast.Delete, "server.users"},
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
	mustFail(t, "effect a on sessions.get;", "状態の操作は post / put / delete のどれかで、get ではない")
	mustFail(t, "effect a on sessions;", "「状態.操作」")
	mustFail(t, "effect a sessions.post;", "「on」が必要")
}

func idents(ids []ast.Ident) string {
	var s []string
	for _, id := range ids {
		s = append(s, id.Name)
	}
	return strings.Join(s, "|")
}

// 遷移（docs/grammar.md「exception / effect」の Transition と CaseSet）。
func TestEffectTransition(t *testing.T) {
	for _, tt := range []struct {
		src      string
		of       string
		from, to string // "any" か、"|" でつないだケース名
	}{
		{"effect placeOrder on Checkout: Shopping -> Ordered;", "Checkout", "Shopping", "Ordered"},
		{"effect clearCart on carts: HasItems -> Empty;", "carts", "HasItems", "Empty"},
		{"effect reset on Checkout: any -> Shopping;", "Checkout", "any", "Shopping"},
		{"effect touch on Checkout: Shopping | Ordered -> any;", "Checkout", "Shopping|Ordered", "any"},
		{"effect a on server.Checkout:A|B->C|D;", "server.Checkout", "A|B", "C|D"},
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
	// 「QualifiedName の直後が ":" なら Transition」なので、これは carts.post という名前の遷移として読める。
	// 名前が指すものの誤りは名前の解決（RES-4）が出す。
	if e := firstDecl[*ast.EffectDecl](t, "effect a on carts.post: A -> B;"); e.Transition == nil || e.Transition.Of.String() != "carts.post" {
		t.Errorf("Transition = %+v", e.Transition)
	}
	tests := []struct{ name, src, want string }{
		{"missing arrow", "effect a on C: A;", "「->」が必要"},
		{"fat arrow", "effect a on C: A => B;", "「->」が必要"},
		{"missing to", "effect a on C: A -> ;", "遷移のケース（any かケース名）が必要"},
		{"missing from", "effect a on C: -> B;", "遷移のケース（any かケース名）が必要"},
		{"qualified case", "effect a on C: x.A -> B;", "遷移のケース（any かケース名）が必要"},
		{"any in a set", "effect a on C: A | any -> B;", "any は予約語"},
		{"two transitions", "effect a on C: A -> B, D: E -> F;", "「;」が必要"},
		{"no target", "effect a on : A -> B;", "「状態.操作」"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// --- classify / condition ---

func TestClassify(t *testing.T) {
	c := firstDecl[*ast.ClassifyDecl](t, "classify CampaignKind of server.Item {\n /** s */ Spring;\n Summer;\n /** d */ default;\n}")
	if c.Name.Name != "CampaignKind" || len(c.Of) != 1 || typeString(c.Of[0]) != "server.Item" || len(c.Cases) != 3 {
		t.Fatalf("Classify = %+v", c)
	}
	if c.Cases[0].Doc == nil || c.Cases[0].Name == nil || c.Cases[0].Name.Name != "Spring" || c.Cases[0].Default {
		t.Errorf("Case 0 = %+v", c.Cases[0])
	}
	if !c.Cases[2].Default || c.Cases[2].Name != nil || c.Cases[2].Doc == nil || c.Cases[2].Doc.Body != "d" {
		t.Errorf("Case 2 = %+v", c.Cases[2])
	}
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

// 対象が二つ以上（docs/grammar.md「classify / condition」の ClassifySubject）。状態か型かは名前の解決が決める。
func TestClassifySubject(t *testing.T) {
	c := firstDecl[*ast.ClassifyDecl](t, "classify Checkout of carts, orders {\n Shopping;\n Ordered;\n}")
	if typeStrings(c.Of) != "carts,orders" || len(c.Cases) != 2 {
		t.Errorf("Classify = %+v（Of = %s）", c, typeStrings(c.Of))
	}
	c = firstDecl[*ast.ClassifyDecl](t, "classify Session of token { LoggedIn; default; }")
	if typeStrings(c.Of) != "token" {
		t.Errorf("Of = %s", typeStrings(c.Of))
	}
	// 構文は Type の並びなので、型と状態が混ざっていても読める。弾くのは検査 CLS-3。
	c = firstDecl[*ast.ClassifyDecl](t, "classify X of server.Item[], carts { A; }")
	if typeStrings(c.Of) != "server.Item[],carts" {
		t.Errorf("Of = %s", typeStrings(c.Of))
	}
	cond := firstDecl[*ast.ConditionDecl](t, "condition CartFilled of carts, orders;")
	if cond.Name.Name != "CartFilled" || typeStrings(cond.Of) != "carts,orders" {
		t.Errorf("Condition = %+v（Of = %s）", cond, typeStrings(cond.Of))
	}
	mustFail(t, "classify A of { X; }", "分類の対象（型か状態の名前）が必要")
	mustFail(t, "classify A of carts, { X; }", "分類の対象（型か状態の名前）が必要")
	mustFail(t, "classify A of carts orders { X; }", "「{」が必要")
	mustFail(t, "classify A of list<T> { X; }", "list / nullable / single は state と match の by の後ろにだけ書ける")
	mustFail(t, "condition A of carts,;", "分類の対象（型か状態の名前）が必要")
	mustFail(t, "condition A of;", "分類の対象（型か状態の名前）が必要")
}

func TestCondition(t *testing.T) {
	c := firstDecl[*ast.ConditionDecl](t, "condition WarrantyAvailable of server.Item;")
	if c.Name.Name != "WarrantyAvailable" || len(c.Of) != 1 || typeString(c.Of[0]) != "server.Item" {
		t.Errorf("Condition = %+v", c)
	}
	mustFail(t, "condition A of T { }", "「;」が必要")
}

// --- decorator の宣言 ---

func TestDecoratorDecl(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, "decorator retry(times: int);")
	if d.Name.Name != "retry" || len(d.Params) != 1 || d.Params[0].Name.Name != "times" || len(d.Params[0].Type.Types) != 1 || len(d.For) != 0 || len(d.Clauses) != 0 || d.External || d.Alias != nil {
		t.Errorf("Decorator = %+v", d)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator body for field;")
	if len(d.Params) != 0 || len(d.For) != 1 || d.For[0].Kind != ast.TargetField {
		t.Errorf("Decorator = %+v", d)
	}
	d = firstDecl[*ast.DecoratorDecl](t, "decorator min(value: int | float) for scalar | field | op | state | exception | model | namespace;")
	if len(d.Params[0].Type.Types) != 2 || len(d.For) != 7 {
		t.Errorf("Decorator = %+v", d)
	}
	wantTargets := []ast.TargetKind{ast.TargetScalar, ast.TargetField, ast.TargetOp, ast.TargetState, ast.TargetException, ast.TargetModel, ast.TargetNamespace}
	for i, w := range wantTargets {
		if d.For[i].Kind != w {
			t.Errorf("For[%d] = %s, want %s", i, d.For[i].Kind, w)
		}
	}
	d = firstDecl[*ast.DecoratorDecl](t, "@external decorator get(path: string) throws ServerError | NetworkError;")
	if !d.External || d.ExternalSpan.Start != 0 || d.ExternalSpan.End != 9 || len(d.Clauses) != 1 {
		t.Fatalf("Decorator = %+v", d)
	}
	th := d.Clauses[0].(*ast.ThrowsClause)
	if names(th.Exceptions) != "ServerError|NetworkError" || len(th.Relations) != 0 {
		t.Errorf("Throws = %+v", th)
	}
	// Span は @external から ";" まで。
	if d.Span.Start != 0 {
		t.Errorf("Span = %+v", d.Span)
	}
}

func TestDecoratorDeclClauses(t *testing.T) {
	d := firstDecl[*ast.DecoratorDecl](t, `decorator fallbackToCache
    catches http.NetworkError recovers
    throws OfflineCacheMiss prevents effects
    catches A | B ignores
    catches C halts transitionToLogin
    throws X | Y keeps effects;`)
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

// TestRelationObject は Relation = ( "prevents" | "keeps" ) "effects"（docs/grammar.md「decorator の宣言」）:
// 目的語は `effects` だけで、効果の名前は書けない（docs/grammar.md「決めたこと」、docs/decisions.md S9）。
func TestRelationObject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"named effect on op", "op f() effects a throws X keeps a;", "目的語は「effects」だけ"},
		{"named effects on op", "op f() effects a throws X keeps a prevents b;", "keeps の目的語は「effects」だけ"},
		{"qualified effect", "op f() throws X prevents server.a;", "prevents の目的語は「effects」だけ"},
		{"named effect on decorator", "decorator d throws X prevents a | b;", "prevents の目的語は「effects」だけ"},
		{"missing object", "op f() throws X prevents;", "「effects」が必要だが「;」がある"},
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
	d := firstDecl[*ast.DecoratorDecl](t, "decorator route(path: string, from: any state?, onInvalid: any effect on location.put, when: any variant, e: any exception, o: any op, m: any model);")
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
		{"external on alias", "@external decorator a = b();", "名前付きのデコレータに @external は付けられない"},
		{"catches without outcome", "decorator a catches X;", "catches の後の振る舞い"},
		{"halts without effect", "decorator a catches X halts;", "名前が必要"},
		{"bad any kind", "decorator a(x: any thing);", "any の種類"},
		{"bad target", "decorator a for thing;", "for の対象"},
		{"param without type", "decorator a(x);", "「:」が必要"},
		{"alias without args", "decorator a = b;", "「(」が必要"},
		{"optional non-any type with pipe", "decorator a(x: int |);", "型が必要"},
		{"missing semicolon", "decorator a", "「;」が必要"},
		{"effects clause", "decorator a effects x;", "「;」が必要だが「effects」がある"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustFail(t, tt.src, tt.want) })
	}
}

// --- op ---

func TestOp(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, `@http.post("/login") @validated
op login(in: LoginInput, n: int): LoginOk
  effects createSession | notify
  throws InvalidCredentials | NotVerified prevents effects
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

	o = firstDecl[*ast.OpDecl](t, "op clearToken() effects removeToken;")
	if o.Return != nil || len(o.Params) != 0 || len(o.Clauses) != 1 {
		t.Errorf("Op = %+v", o)
	}
	o = firstDecl[*ast.OpDecl](t, "op a(): void;")
	if o.Return == nil || !o.Return.Void || o.Return.Type != nil {
		t.Errorf("Return = %+v", o.Return)
	}
	o = firstDecl[*ast.OpDecl](t, "op a(x: int)\n  : T\n{ return x; }")
	if o.Body == nil || len(o.Body.Stmts) != 1 || typeString(o.Return.Type) != "T" {
		t.Errorf("Op = %+v", o)
	}
	if o.Span.End != o.Body.Span.End {
		t.Errorf("Span = %+v, Body = %+v", o.Span, o.Body.Span)
	}
}

func TestOpReject(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"catches on op", "op a() catches X recovers;", "catches はデコレータの宣言にだけ書ける"},
		{"no terminator", "op a()", "「;」か配線の「{」が必要"},
		{"no params", "op a;", "「(」が必要"},
		{"param without type", "op a(x);", "「:」が必要"},
		{"void param", "op a(x: void);", "void は op の戻り値の型にだけ書ける"},
		{"pipe type in param", "op a(x: int | float);", "「,」か「)」"},
		{"for on op", "op a() for op;", "「;」か配線の「{」が必要"},
		{"qualified name", "op a.b();", "修飾しない識別子"},
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
		{"Time", "Time"},
		{"Duration", "Duration"},
		{"User", "User"},
		{"server.User", "server.User"},
		{"User[]", "User[]"},
		{"User?", "User?"},
		{"User[]?", "User[]?"},
		{"User?[]", "User?[]"},
		{"User[][]", "User[][]"},
		{"Record<string, string[]>", "Record<string, string[]>"},
		{"Record<string, Record<string, int>>", "Record<string, Record<string, int>>"},
		{"WarrantyKind.Yamada<server.Item>", "WarrantyKind.Yamada<server.Item>"},
		{"A<B<C>>?", "A<B<C>>?"},
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
		{"list<A>", "list / nullable / single は state と match の by の後ろにだけ書ける"},
		{"nullable<A>", "list / nullable / single は state と match の by の後ろにだけ書ける"},
		{"single<A>", "list / nullable / single は state と match の by の後ろにだけ書ける"},
		{"{ a: int; }", "その場で書く model はユニオンのアームと例外の値にだけ置ける"},
		{"Record<string>", "「,」が必要"},
		{"Record<string, int", "「>」が必要"},
		{"User[", "「]」が必要"},
		{"string<int>", "「;」が必要だが「<」がある"},
		{"5", "型が必要"},
	}
	for _, tt := range rejects {
		mustFail(t, "scalar X extends "+tt.src+";", tt.want)
	}
}

// --- 配線 ---

func TestBlockStmts(t *testing.T) {
	src := `op a(query: OrdersQuery): OrderPage {
  const me = fetchMe();
  const line = server.addToCart({ item });
  const v = query.page;
  const n = 5;
  const z = none;
  requestLogout();
  server.logout({});
  match x by K { A => f(); };
  return orderPageVm(me, line, "s", -1, 2.5, true, 30s, none, { page: query.page, status: s, });
}`
	o := firstDecl[*ast.OpDecl](t, src)
	st := o.Body.Stmts
	if len(st) != 9 {
		t.Fatalf("Stmts = %d", len(st))
	}
	c0 := st[0].(*ast.ConstStmt)
	if c0.Name.Name != "me" {
		t.Errorf("Stmt 0 = %+v", c0)
	}
	if call, ok := c0.Value.(*ast.Call); !ok || call.Callee.String() != "fetchMe" || len(call.Args) != 0 {
		t.Errorf("Stmt 0 value = %+v", c0.Value)
	}
	c1 := st[1].(*ast.ConstStmt).Value.(*ast.Call)
	if c1.Callee.String() != "server.addToCart" || len(c1.Args) != 1 {
		t.Fatalf("Stmt 1 = %+v", c1)
	}
	rec := c1.Args[0].(*ast.RecordLit)
	if len(rec.Fields) != 1 || rec.Fields[0].Name.Name != "item" || rec.Fields[0].Value != nil {
		t.Errorf("record = %+v", rec)
	}
	if n, ok := st[2].(*ast.ConstStmt).Value.(*ast.NameAtom); !ok || n.Name.String() != "query.page" || len(n.Name.Segments) != 2 {
		t.Errorf("Stmt 2 = %+v", st[2].(*ast.ConstStmt).Value)
	}
	if l, ok := st[3].(*ast.ConstStmt).Value.(*ast.Literal); !ok || l.Int != 5 {
		t.Errorf("Stmt 3 = %+v", st[3].(*ast.ConstStmt).Value)
	}
	if _, ok := st[4].(*ast.ConstStmt).Value.(*ast.NoneAtom); !ok {
		t.Errorf("Stmt 4 = %+v", st[4].(*ast.ConstStmt).Value)
	}
	if cs, ok := st[5].(*ast.CallStmt); !ok || cs.Call.Callee.String() != "requestLogout" {
		t.Errorf("Stmt 5 = %+v", st[5])
	}
	cs6 := st[6].(*ast.CallStmt)
	if cs6.Call.Callee.String() != "server.logout" || len(cs6.Call.Args) != 1 || len(cs6.Call.Args[0].(*ast.RecordLit).Fields) != 0 {
		t.Errorf("Stmt 6 = %+v", cs6)
	}
	if ms, ok := st[7].(*ast.MatchStmt); !ok || ms.Match.Classifier.Name != "K" {
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
	o := firstDecl[*ast.OpDecl](t, "op a(me: M): M { return me; }")
	if _, ok := o.Body.Stmts[0].(*ast.ReturnStmt).Value.(*ast.NameAtom); !ok {
		t.Errorf("return = %+v", o.Body.Stmts[0])
	}
}

func TestMatch(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, `op a(item: I) {
  const warranty = match item by WarrantyKind {
    Standard | NoWarranty => chooseStandardWarranty(item, line);
    Yamada     => server.chooseYamada(item);
    default => none;
  };
  match item by Available { A => f(); };
}`)
	m := o.Body.Stmts[0].(*ast.ConstStmt).Value.(*ast.Match)
	if m.Subject.(*ast.NameAtom).Name.String() != "item" || m.Classifier.Name != "WarrantyKind" || m.Builtin || len(m.Arms) != 3 {
		t.Fatalf("Match = %+v", m)
	}
	a0 := m.Arms[0]
	if len(a0.Cases) != 2 || a0.Cases[1].Name != "NoWarranty" || a0.Default {
		t.Errorf("Arm 0 = %+v", a0)
	}
	if c, ok := a0.Body.(*ast.Call); !ok || c.Callee.String() != "chooseStandardWarranty" || len(c.Args) != 2 {
		t.Errorf("Arm 0 body = %+v", a0.Body)
	}
	if c, ok := m.Arms[1].Body.(*ast.Call); !ok || c.Callee.String() != "server.chooseYamada" {
		t.Errorf("Arm 1 body = %+v", m.Arms[1].Body)
	}
	a2 := m.Arms[2]
	if !a2.Default || len(a2.Cases) != 0 {
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

// 状態での match（docs/grammar.md「配線」の表の二つ目と三つ目の形）。
func TestMatchOnState(t *testing.T) {
	o := firstDecl[*ast.OpDecl](t, `op a() {
  match carts by list {
    Empty => showEmpty();
    HasItems => showCart();
  };
  const me = match token by nullable {
    Empty => none;
    HasItems => server.getMe();
  };
  match by Checkout {
    Shopping => f();
    Ordered | Shipped => g();
  };
  return match by Session { LoggedIn => h(); default => none; };
}`)
	byList := o.Body.Stmts[0].(*ast.MatchStmt).Match
	if byList.Subject.(*ast.NameAtom).Name.String() != "carts" || !byList.Builtin || byList.Classifier.Name != "list" || len(byList.Arms) != 2 {
		t.Errorf("by list = %+v", byList)
	}
	if byList.Arms[0].Cases[0].Name != "Empty" || byList.Arms[1].Cases[0].Name != "HasItems" {
		t.Errorf("by list arms = %+v", byList.Arms)
	}
	byNullable := o.Body.Stmts[1].(*ast.ConstStmt).Value.(*ast.Match)
	if byNullable.Subject.(*ast.NameAtom).Name.String() != "token" || !byNullable.Builtin || byNullable.Classifier.Name != "nullable" {
		t.Errorf("by nullable = %+v", byNullable)
	}
	byClassify := o.Body.Stmts[2].(*ast.MatchStmt).Match
	if byClassify.Subject != nil || byClassify.Builtin || byClassify.Classifier.Name != "Checkout" || len(byClassify.Arms) != 2 {
		t.Errorf("by Checkout = %+v", byClassify)
	}
	ret := o.Body.Stmts[3].(*ast.ReturnStmt).Value.(*ast.Match)
	if ret.Subject != nil || ret.Classifier.Name != "Session" || !ret.Arms[1].Default {
		t.Errorf("return match = %+v", ret)
	}
	// by の後ろの予約語の Span は Classifier が持つ。
	if byList.Classifier.Span.End-byList.Classifier.Span.Start != source.Offset(len("list")) {
		t.Errorf("Classifier Span = %+v", byList.Classifier.Span)
	}
	// 構文は対象の有無と by の後ろを別々に読む。組み合わせが合うかは検査 MATCH-7。
	o = firstDecl[*ast.OpDecl](t, "op b() { match by list { Empty => f(); HasItems => g(); }; match carts by Checkout { A => f(); }; }")
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
  condition LoggedIn of token;
  effect placeOrder on Checkout: Shopping | Ordered -> any;
  effect clearCart on shop.carts: any -> Empty;
  op view(): Order? {
    match token by nullable { Empty => f(); HasItems => none; };
    const placed = match by Checkout { Shopping => submit(); Ordered => none; };
    return match carts by list { Empty | HasItems => g(); };
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
		{"nested call", "op a() { const x = f(g()); }", "呼び出しの引数に呼び出しは書けない"},
		{"match without arms", "op a() { match x by K { }; }", "match にはアームが一つ以上要る"},
		{"match without semicolon", "op a() { match x by K { A => f(); } }", "「;」が必要だが「}」がある"},
		{"arm body atom", "op a() { match x by K { A => x; }; }", "アームの本体（呼び出しか none）"},
		{"arm body match", "op a() { match x by K { A => match y by L { B => none; }; }; }", "アームの本体（呼び出しか none）"},
		{"arm without arrow", "op a() { match x by K { A f(); }; }", "「=>」が必要"},
		{"if", "op a() { if (x) { } }", "「;」が必要だが「{」がある"},
		{"literal statement", "op a() { 5; }", "文（const / return / 呼び出し / match）が必要"},
		{"const without value", "op a() { const x; }", "「=」が必要"},
		{"bare name statement", "op a() { x; }", "「(」が必要"},
		{"record field value call", "op a() { f({ x: g() }); }", "「,」か「}」"},
		{"record field qualified", "op a() { f({ a.b }); }", "修飾しない識別子"},
		{"doc in block", "op a() { /** d */ return x; }", "付く先のない"},
		{"return missing semicolon", "op a() { return x }", "「;」が必要だが「}」がある"},
		{"match by qualified", "op a() { match x by a.K { A => f(); }; }", "修飾しない識別子"},
		{"match by single", "op a() { match s by single { A => f(); }; }", "single の状態には組み込みの分類がない"},
		{"match by core type", "op a() { match s by string { A => f(); }; }", "string は予約語"},
		{"match without by", "op a() { match x { A => f(); }; }", "「by」が必要"},
		{"match two subjects", "op a() { match x, y by K { A => f(); }; }", "「by」が必要"},
		{"match by nullable with type argument", "op a() { match s by nullable<T> { Empty => f(); }; }", "「{」が必要"},
		{"match by without classifier", "op a() { match by { A => f(); }; }", "識別子"},
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
		m := firstDecl[*ast.ModelDecl](t, "model M { x: T = "+tt.src+"; }")
		l := m.Body.Fields[0].Default
		if l == nil || !tt.check(l) {
			t.Errorf("%s: Literal = %+v", tt.src, l)
		}
	}
	mustFail(t, "model M { x: T = 9223372036854775808; }", "整数が 64 ビットに収まらない")
	mustFail(t, "model M { x: T = none; }", "リテラルが必要")
	mustFail(t, "model M { x: T = y; }", "リテラルが必要")
}

// --- 識別子 ---

func TestIdentAndQualifiedName(t *testing.T) {
	e := firstDecl[*ast.EffectDecl](t, "effect post on a.b.put;")
	if e.Name.Name != "post" {
		t.Errorf("文脈キーワードが識別子にならない: %+v", e.Name)
	}
	segs := e.On.State.Segments
	if len(segs) != 2 || segs[0].Name != "a" || segs[1].Name != "b" {
		t.Fatalf("Segments = %+v", segs)
	}
	start := source.Offset(len("effect post on "))
	if segs[0].Span.Start != start || segs[0].Span.End != start+1 || segs[1].Span.Start != start+2 || segs[1].Span.End != start+3 {
		t.Errorf("Segment Spans = %+v %+v", segs[0].Span, segs[1].Span)
	}
	mustFail(t, "model string {}", "string は予約語で、識別子には使えない")
	mustFail(t, "model to {}", "to は予約語")
	mustFail(t, "scalar A extends a.string;", "「;」が必要だが「.」がある")
}

// --- ドキュメンテーションコメント ---

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
op requestPasswordReset(email: Email);`
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
		{"see keyword", "/** @see string */ model A {}", "@see の名前の形が正しくない"},
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
op D(x int) { return x; }
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
	src := `op a(x: X): Y {
  const a = f();
  const = g();
  h(x;
  const c = match x by K {
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
	m := o.Body.Stmts[1].(*ast.ConstStmt).Value.(*ast.Match)
	if len(m.Arms) != 2 {
		t.Errorf("Arms = %d", len(m.Arms))
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
	src := "model A { x: string = \"a\\qb\"; }\nmodel B {}\nmodel C { y: int = 5w; }\nmodel D {}"
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
