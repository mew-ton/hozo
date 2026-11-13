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
  model User { id: string; email: Email; hash: string; nick: string?; page: int = 1; }
  model PublicUser is User omit hash;
  model Other { name: string; }
  model NeedsNick { id: string; nick: string; }
  model HasDefault { id: string; limit: int = 10; }
  classify C of User { A; B; }
  classify D of User { A; }
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
	if !a.assignType(narrowed, publicUser) {
		t.Error("絞り込みを捨てた後は射影もできる（再帰的に適用）")
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
	if !a.assignType(ir.Array(user), ir.Array(publicUser)) {
		t.Error("要素が渡せれば配列も渡せる")
	}
	if a.assignType(ir.Array(other), ir.Array(user)) {
		t.Error("要素が渡せなければ配列も渡せない")
	}
}

func TestCompatRule7Record(t *testing.T) {
	a := compatAnalyzer(t)
	if !a.assignType(ir.Record(ir.String(), user), ir.Record(ir.String(), publicUser)) {
		t.Error("キーが等しく値が渡せれば Record も渡せる")
	}
	if a.assignType(ir.Record(email, user), ir.Record(ir.String(), user)) {
		t.Error("キーが違えば渡せない（Email と string は別）")
	}
}

func TestCompatRule8Projection(t *testing.T) {
	a := compatAnalyzer(t)
	if !a.assignType(user, publicUser) {
		t.Error("User は PublicUser に射影できる")
	}
	if !a.assignType(user, ir.Ref("a.HasDefault")) {
		t.Error("S にないフィールドは default で埋める")
	}
	if a.assignType(user, ir.Ref("a.NeedsNick")) {
		t.Error("T の必須のフィールドが S で省略可能なら射影できない")
	}
	if a.assignType(publicUser, user) {
		t.Error("hash がないので PublicUser は User に射影できない")
	}
	if a.assignType(other, user) {
		t.Error("フィールドが合わなければ射影できない")
	}
}

func TestCompatRule9RecordLiteral(t *testing.T) {
	a := compatAnalyzer(t)
	rec := &recordVal{fields: []recordFieldVal{
		{name: "id", typ: literalType(0)},
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
