package sema

import (
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// 型の互換の各規則（docs/types.md「互換」）に一つずつ（docs/testing.md「単体テスト」）。

// compatAnalyzer は互換の判定に使う宣言を持つ analyzer。
func compatAnalyzer(t *testing.T) *analyzer {
	t.Helper()
	fs, files := parseProject(t, project{"a.hozo": `
namespace a {
  scalar Email extends string;
  scalar Token extends string;
  scalar Mail2 extends Email;
  model User { id: string; email: Email; hash: string; nick: string?; page: int; }
  model PublicUser extends User { override hash: never; }
  model Other { name: string; }
  model Same { name: string; }
  model Admin extends User { }
  model SuperAdmin extends Admin { level: int; }
  model HasDefault { id: string; limit: int; }
  union Page { Ready: User; Missing: Other; }
  classify C of User { A; B; }
  classify D of User { A; }
  model ByName { [name: string]: User; }
  model ByName2 { [name: string]: User; }
}`}, false)
	a := analyze(fs, files)
	if n := len(a.diags); n > 0 {
		t.Fatalf("診断がある: %+v", a.diags)
	}
	return a
}

var (
	user       = ir.Ref("a.User")
	publicUser = ir.Ref("a.PublicUser")
	other      = ir.Ref("a.Other")
	email      = ir.Ref("a.Email")
	token      = ir.Ref("a.Token")
	mail2      = ir.Ref("a.Mail2")
)

func TestCompatRule1Equal(t *testing.T) {
	a := compatAnalyzer(t)
	if !a.assignType(ir.String(), ir.String()) || !a.assignType(user, user) || !a.assignType(ir.Array(email), ir.Array(email)) {
		t.Error("等しい型は渡せる")
	}
	if a.assignType(ir.Int(), ir.String()) {
		t.Error("int は string に渡せない")
	}
}

func TestCompatRule2Optional(t *testing.T) {
	a := compatAnalyzer(t)
	if !a.assignType(ir.String(), ir.Optional(ir.String())) {
		t.Error("T は T? に渡せる")
	}
	if a.assignType(ir.Optional(ir.String()), ir.String()) {
		t.Error("T? は T に渡せない")
	}
}

func TestCompatRule3None(t *testing.T) {
	a := compatAnalyzer(t)
	if a.assign(noneType(), ir.Optional(user)) != assignOK {
		t.Error("none は optional に渡せる")
	}
	if a.assign(noneType(), user) == assignOK {
		t.Error("none は必須の位置に渡せない")
	}
}

func TestCompatRule4CaseToOf(t *testing.T) {
	a := compatAnalyzer(t)
	narrowed := ir.Case("a.C", []string{"A"}, user)
	if !a.assignType(narrowed, user) {
		t.Error("絞り込まれた型は元の型に渡せる")
	}
	if !a.assignType(narrowed, ir.Optional(user)) {
		t.Error("絞り込みを捨てた後も再帰的に適用する（T は T? に）")
	}
	if a.assignType(narrowed, publicUser) {
		t.Error("絞り込みを捨てても、別の model には渡せない")
	}
	if a.assignType(user, narrowed) {
		t.Error("元の型は絞り込まれた型に渡せない")
	}
}

func TestCompatRule5CaseSubset(t *testing.T) {
	a := compatAnalyzer(t)
	narrow := ir.Case("a.C", []string{"A"}, user)
	wide := ir.Case("a.C", []string{"A", "B"}, user)
	if !a.assignType(narrow, wide) {
		t.Error("狭い方は広い方に渡せる")
	}
	if a.assignType(wide, narrow) {
		t.Error("広い方は狭い方に渡せない")
	}
	if a.assignType(narrow, ir.Case("a.D", []string{"A"}, user)) {
		t.Error("別の classify の絞り込みには渡せない")
	}
}

func TestCompatRule6Array(t *testing.T) {
	a := compatAnalyzer(t)
	if !a.assignType(ir.Array(email), ir.Array(ir.String())) {
		t.Error("要素が渡せれば配列も渡せる")
	}
	if a.assignType(ir.Array(user), ir.Array(publicUser)) {
		t.Error("別の model の配列には渡せない")
	}
	if a.assignType(ir.Array(other), ir.Array(user)) {
		t.Error("要素が渡せなければ配列も渡せない")
	}
}

// TestCompatIndexModel は、インデックスの model（docs/types.md「インデックスの model」）: ほかの model と同じに、名前で比べる。
// キーと値の型が同じでも、別の model には渡せない（規則 7 は欠番）。
func TestCompatIndexModel(t *testing.T) {
	a := compatAnalyzer(t)
	byName, byName2 := ir.Ref("a.ByName"), ir.Ref("a.ByName2")
	if m := a.models["a.ByName"]; m == nil || m.Index == nil || m.Index.Name != "name" || !ir.TypeEqual(m.Index.Key, ir.String()) || !ir.TypeEqual(m.Index.Value, user) || len(m.Fields) != 0 {
		t.Fatalf("a.ByName のインデックス = %+v", m)
	}
	if !a.assignType(byName, byName) {
		t.Error("同じインデックスの model は渡せる")
	}
	if !a.assignType(byName, ir.Optional(byName)) {
		t.Error("インデックスの model は、その optional に渡せる")
	}
	if a.assignType(byName, byName2) {
		t.Error("キーと値の型が同じでも、別のインデックスの model には渡せない")
	}
}

// 規則 8（model から model への射影）は欠番（docs/types.md「互換」）。model の値は、別の model の位置には渡せない:
// フィールドが同じでも、一方がもう一方を継承していても、どちらの向きにも。
func TestCompatRule8Gone(t *testing.T) {
	a := compatAnalyzer(t)
	pairs := [][2]ir.Type{
		{user, publicUser}, {publicUser, user}, // 継承（フィールドをなくした）
		{ir.Ref("a.Admin"), user}, {user, ir.Ref("a.Admin")}, // 継承（本体が空で、フィールドは同じ）
		{ir.Ref("a.SuperAdmin"), ir.Ref("a.Admin")},
		{other, ir.Ref("a.Same")}, {ir.Ref("a.Same"), other}, // フィールドが同じ
		{user, ir.Ref("a.HasDefault")},
	}
	for _, p := range pairs {
		if a.assignType(p[0], p[1]) {
			t.Errorf("%s は %s に渡せない（別の型）", p[0], p[1])
		}
		if a.assign(valueType(p[0]), p[1]) != failType {
			t.Errorf("%s → %s は TYPE-4 / TYPE-3", p[0], p[1])
		}
	}
	// union（arms）に、アームの型の値を直接渡せない
	if a.assignType(user, ir.Ref("a.Page")) {
		t.Error("アームの型の値は union の位置に渡せない")
	}
}

// 「返らない」式は、どの型の位置にも書ける（docs/types.md「式の型」）。
func TestCompatNever(t *testing.T) {
	a := compatAnalyzer(t)
	for _, to := range []ir.Type{user, ir.String(), ir.Optional(email), ir.Array(user)} {
		if a.assign(neverType(), to) != assignOK {
			t.Errorf("「返らない」は %s に渡せる", to)
		}
	}
	if callType(ir.Never()).kind != etNever || callType(nil).kind != etNoValue || callType(user).kind != etValue {
		t.Error("呼び出しの式の型")
	}
}

// docs/types.md「変えられる型」の規則 1〜6。
func TestChangeable(t *testing.T) {
	a := compatAnalyzer(t)
	admin, super, page := ir.Ref("a.Admin"), ir.Ref("a.SuperAdmin"), ir.Ref("a.Page")
	cases := []struct {
		name    string
		o, n    ir.Type
		ok      bool
		widened bool
	}{
		{"1 等しい", user, user, true, false},
		{"2 広げる（union のアームに元の型）", user, page, true, true},
		{"2 アームにない型", email, page, false, false},
		{"3 差し替える（model を継承した model）", user, admin, true, false},
		{"3 二段の継承", user, super, true, false},
		{"3 逆向きは変えられない", admin, user, false, false},
		{"3 scalar を継承した scalar", email, mail2, true, false},
		{"3 核の型を継承した scalar", ir.String(), email, true, false},
		{"3 核の型を、二段で継承した scalar", ir.String(), mail2, true, false},
		{"3 別の scalar", email, token, false, false},
		{"3 scalar から核の型には戻せない", email, ir.String(), false, false},
		{"4 optional の中", ir.Optional(user), ir.Optional(admin), true, false},
		{"4 optional の中を広げる", ir.Optional(user), ir.Optional(page), true, true},
		{"5 array の中", ir.Array(ir.String()), ir.Array(email), true, false},
		{"必須を省略可能にできない", user, ir.Optional(user), false, false},
		{"省略可能を必須にできない", ir.Optional(user), user, false, false},
		{"関係のない型", user, other, false, false},
		{"union から元の型には戻せない", page, user, false, false},
		{"array を外せない", ir.Array(user), user, false, false},
	}
	for _, c := range cases {
		widened, ok := a.changeable(c.o, c.n)
		if ok != c.ok || widened != c.widened {
			t.Errorf("%s: %s → %s = (widened %v, ok %v), want (%v, %v)", c.name, c.o, c.n, widened, ok, c.widened, c.ok)
		}
	}
	// 継承している: 等しいときは含まない
	if a.inherits(user, user) || a.inherits(email, email) || !a.inherits(super, user) || a.inherits(user, super) {
		t.Error("inherits")
	}
}

// docs/types.md「絞り込める型」と「型で決まる絞り込み」。
func TestNarrowable(t *testing.T) {
	a := compatAnalyzer(t)
	page := ir.Ref("a.Page")
	cases := []struct {
		name string
		s, t ir.Type
		ok   bool
		by   ir.NarrowBy
	}{
		{"optional(T) を T に", ir.Optional(user), user, true, ir.NarrowByPresence},
		{"union をアームの型に", page, user, true, ir.NarrowByVariant},
		{"optional(union) をアームの型に（型では決まらない）", ir.Optional(page), other, true, ir.NarrowByPredicate},
		{"string を scalar に", ir.String(), email, true, ir.NarrowByPredicate},
		{"string? を scalar に", ir.Optional(ir.String()), email, true, ir.NarrowByPredicate},
		{"対象と同じ型に", user, user, true, ir.NarrowByPredicate},
		{"絞り込まれた型（case）に", user, ir.Case("a.C", []string{"A"}, user), true, ir.NarrowByPredicate},
		{"関係のない型には絞り込めない", user, other, false, ""},
		{"union のアームでない型", page, email, false, ""},
		{"広い型には絞り込めない", user, ir.Optional(user), false, ""},
	}
	for _, c := range cases {
		if got := a.narrowable(c.s, c.t); got != c.ok {
			t.Errorf("%s: narrowable(%s, %s) = %v, want %v", c.name, c.s, c.t, got, c.ok)
			continue
		}
		if c.ok {
			if got := a.narrowBy(c.s, c.t); got != c.by {
				t.Errorf("%s: narrowBy(%s, %s) = %s, want %s", c.name, c.s, c.t, got, c.by)
			}
		}
	}
}

func TestCompatRule9RecordLiteral(t *testing.T) {
	a := compatAnalyzer(t)
	rec := &recordVal{fields: []recordFieldVal{
		{name: "id", typ: literalType(0, ir.StringLiteral{Value: "x"})},
	}}
	// 当てはめの違反は fitRecord が TYPE-5 / TYPE-6 として報告し、呼び手には渡せた扱いで返す（二重に報告しないため）。
	a.assign(exprType{kind: etRecord, rec: rec}, ir.Optional(other))
	if len(a.diags) != 1 || a.diags[0].Code != "TYPE-5" {
		t.Errorf("TYPE-5 が出る: %+v", a.diags)
	}
	a.diags = nil
	if a.assign(exprType{kind: etRecord, rec: rec}, ir.Ref("a.HasDefault")) != assignOK || len(a.diags) != 0 {
		t.Errorf("当てはまるリテラル: %+v", a.diags)
	}
	if a.assign(exprType{kind: etRecord, rec: rec}, ir.String()) == assignOK {
		t.Error("model でない位置にレコードリテラルは渡せない")
	}
}

func TestCompatRule10Literal(t *testing.T) {
	a := compatAnalyzer(t)
	cases := []struct {
		lit  ir.Type
		to   ir.Type
		want assignFailure
	}{
		{ir.String(), ir.String(), assignOK},
		{ir.Int(), ir.Int(), assignOK},
		{ir.Int(), ir.Float(), assignOK},
		{ir.Float(), ir.Int(), failType},
		{ir.Boolean(), ir.Boolean(), assignOK},
		{ir.Duration(), ir.Duration(), assignOK},
		{ir.Duration(), ir.Int(), failType},
		{ir.String(), ir.Optional(ir.String()), assignOK},
		{ir.String(), email, failScalar},
		{ir.String(), ir.Optional(email), failScalar},
	}
	for _, c := range cases {
		if got := a.assign(exprType{kind: etLiteral, typ: c.lit}, c.to); got != c.want {
			t.Errorf("%s のリテラル → %s: %q, want %q", c.lit, c.to, got, c.want)
		}
	}
}

func TestCompatRule11ScalarBase(t *testing.T) {
	a := compatAnalyzer(t)
	if !a.assignType(email, ir.String()) {
		t.Error("Email は string に渡せる")
	}
	if !a.assignType(mail2, ir.String()) {
		t.Error("継承を辿った元の型に渡せる")
	}
	if a.assignType(token, email) {
		t.Error("scalar を別の scalar に渡せない")
	}
	if a.assign(valueType(ir.String()), email) != failScalar {
		t.Error("核の型の値を scalar に渡すのは TYPE-1")
	}
}
