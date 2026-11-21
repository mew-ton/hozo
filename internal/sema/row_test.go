package sema

import (
	"reflect"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、行の遷移（`effect fn …: effects s.op as …`）と、ない側のケース（`対象 as none`）の IR
// （docs/ir.md「state / exception / effect」の RowTransition、「classify」の absent、「正規化」。docs/check.md「行の遷移」
// 「ない側のケース」）。診断（STATE-7、STATE-8、CLS-7、CLS-8。呼ぶ側は TYPE-4、MATCH-4）は testdata/rules が確かめる。

const rowSource = `
namespace row {
  model Order { id: string; }
  state orders: list<Order>;
  state token: nullable<string>;
  fn find(): Order?;
  fn f();

  classify Stage of Order { Draft; Accepted; Shipped; }
  /** ない側のケースを持つ、述語で決まる分類 */
  classify Life of Order? { Open: Order? as Order; Closed: Order? as Order; /** ない */ Absent: Order? as none; }
  classify LifeElse of find { Open: find as Order; Closed: find as Order; Gone: find as else; }
  classify Known of Order? { Found: Order? as Order; Missing: Order? as none; }
  classify KnownElse of Order? { Found: Order? as Order; Missing: Order? as else; }
  classify Session of token { In: token as string; Out: token as else; }
  classify Present of Order? { Here: Order? as Order; }

  effect fn enter(order: Order): effects orders.post as Stage.Draft;
  effect fn enterOpen(order: Order): effects orders.post as Life.Open;
  effect fn plain(order: Order): effects orders.post;
  effect fn advance(at: datetime, order: Order on Stage.Draft | Stage.Accepted): effects orders.put as Stage.Shipped;
  effect fn force(order: Order): effects orders.put as row.Stage.Shipped;
  effect fn revise(order: Order on Stage.Accepted): effects orders.put as else;
  effect fn patch(order: Order): effects orders.put as else;
  effect fn rename(order: Order): effects orders.put;
  effect fn discard(order: Order on Life.Open): effects orders.delete as Life.Absent;
  effect fn purge(order: Order): effects orders.delete as Life.Absent;
  effect fn keep(order: Order on Stage.Draft): effects orders.put;

  fn wire(found: Order?) {
    match(found by Life){ Open => discard(found); Closed => f(); Absent => none; };
  }
}
`

func analyzeRow(t *testing.T) (*ir.Program, *Result) {
	t.Helper()
	res, fs := analyzeProject(t, project{"row.hozo": rowSource}, false)
	noErrors(t, res, fs)
	for _, d := range res.Diags {
		t.Errorf("想定しない診断: %s %s", d.Code, d.Message)
	}
	return res.Program, res
}

// TestRowTransition は、行の遷移の IR: 行の引数、分類、後のケース。前のケースは、行の引数の型（case）にある。
func TestRowTransition(t *testing.T) {
	p, _ := analyzeRow(t)
	order := ir.Ref("row.Order")
	tests := []struct {
		effect string
		want   *ir.RowTransition
		// param は、引数 order の型（前のケース）
		param ir.Type
	}{
		{"row.enter", &ir.RowTransition{Param: "order", Classify: "row.Stage", To: "Draft"}, order},
		{"row.enterOpen", &ir.RowTransition{Param: "order", Classify: "row.Life", To: "Open"}, order},
		{"row.plain", nil, order},
		{"row.advance", &ir.RowTransition{Param: "order", Classify: "row.Stage", To: "Shipped"}, ir.Case("row.Stage", []string{"Draft", "Accepted"}, order)},
		{"row.force", &ir.RowTransition{Param: "order", Classify: "row.Stage", To: "Shipped"}, order},
		{"row.revise", &ir.RowTransition{Param: "order"}, ir.Case("row.Stage", []string{"Accepted"}, order)},
		{"row.patch", &ir.RowTransition{Param: "order"}, order},
		{"row.rename", nil, order},
		{"row.discard", &ir.RowTransition{Param: "order", Classify: "row.Life", To: "Absent"}, ir.Case("row.Life", []string{"Open"}, order)},
		{"row.purge", &ir.RowTransition{Param: "order", Classify: "row.Life", To: "Absent"}, order},
		{"row.keep", nil, ir.Case("row.Stage", []string{"Draft"}, order)}, // as を書かない: ケースは変わらない。前は、型が言う
	}
	for _, tt := range tests {
		e := p.Lookup(tt.effect).(*ir.Effect)
		if !reflect.DeepEqual(e.Row, tt.want) {
			t.Errorf("%s の row = %+v、期待は %+v", tt.effect, e.Row, tt.want)
		}
		var got ir.Type
		for _, prm := range e.Params {
			if prm.Name == "order" {
				got = prm.Type
			}
		}
		if !ir.TypeEqual(got, tt.param) {
			t.Errorf("%s の引数 order の型 = %s、期待は %s", tt.effect, got, tt.param)
		}
		// 宛先と、書き込む状態は、行の遷移があっても変わらない
		if target, ok := e.Target.(ir.BuiltinTarget); !ok || target.State != "row.orders" || !reflect.DeepEqual(e.Writes, []string{"row.orders"}) {
			t.Errorf("%s の宛先 = %+v、writes = %v", tt.effect, e.Target, e.Writes)
		}
	}
}

// TestAbsentCase は、ない側のケース: Classify.absent、`as none` の CaseTyping（by は none、type は対象の型のまま）、
// 分類の判定（ほかのケースが一つなら type、二つ以上なら predicate）。
func TestAbsentCase(t *testing.T) {
	p, _ := analyzeRow(t)
	optOrder := ir.Optional(ir.Ref("row.Order"))
	tests := []struct {
		classify, absent string
		by               ir.NarrowBy
		decidedBy        ir.DecidedBy
	}{
		{"row.Life", "Absent", ir.NarrowByNone, ir.DecidedByPredicate},
		{"row.LifeElse", "Gone", ir.NarrowByElse, ir.DecidedByPredicate},
		{"row.Known", "Missing", ir.NarrowByNone, ir.DecidedByType},
		{"row.KnownElse", "Missing", ir.NarrowByElse, ir.DecidedByType},
	}
	for _, tt := range tests {
		c := p.Lookup(tt.classify).(*ir.Classify)
		if c.Absent != tt.absent || c.DecidedBy != tt.decidedBy {
			t.Errorf("%s: absent = %q、decidedBy = %s、期待は %q、%s", tt.classify, c.Absent, c.DecidedBy, tt.absent, tt.decidedBy)
		}
		cs := c.Case(tt.absent)
		if cs == nil || len(cs.As) != 1 {
			t.Fatalf("%s.%s の as = %+v", tt.classify, tt.absent, cs)
		}
		if as := cs.As[0]; as.By != tt.by || as.State != "" || !ir.TypeEqual(as.Type, optOrder) {
			t.Errorf("%s.%s の as = {state: %q, type: %s, by: %s}、期待は {type: %s, by: %s}", tt.classify, tt.absent, as.State, as.Type, as.By, optOrder, tt.by)
		}
	}
	// 対象の状態が一つの状態の分類の、値がない側も、ない側のケース
	if c := p.Lookup("row.Session").(*ir.Classify); c.Absent != "Out" || c.DecidedBy != ir.DecidedByType {
		t.Errorf("row.Session: absent = %q、decidedBy = %s", c.Absent, c.DecidedBy)
	}
	// ない側のケースを書かない分類と、対象の型が T の分類には、ない
	for _, name := range []string{"row.Present", "row.Stage"} {
		if c := p.Lookup(name).(*ir.Classify); c.Absent != "" {
			t.Errorf("%s: absent = %q、期待は、なし", name, c.Absent)
		}
	}
	// 説明は、ほかのケースと同じに付く
	if cs := p.Lookup("row.Life").(*ir.Classify).Case("Absent"); cs.Description == nil || cs.Description.Summary != "ない" {
		t.Errorf("row.Life.Absent の説明 = %+v", cs.Description)
	}
	// アームの中の対象の型: 型を書いたケースは、その型 T を、そのケースに絞り込んだ型。ない側のケースは、もとの型が外のまま
	arms := p.Lookup("row.wire").(*ir.Op).Body.Statements[0].(ir.MatchStmt).Match.Arms
	if n := arms[0].Narrowed; len(n) != 1 || !ir.TypeEqual(n[0].Type, ir.Case("row.Life", []string{"Open"}, ir.Ref("row.Order"))) {
		t.Errorf("Life.Open のアーム: narrowed = %+v", n)
	}
	if n := arms[2].Narrowed; len(n) != 1 || !ir.TypeEqual(n[0].Type, ir.Case("row.Life", []string{"Absent"}, ir.Optional(ir.Ref("row.Order")))) {
		t.Errorf("Life.Absent のアーム: narrowed = %+v", n)
	}
}

// TestRowCaseRefs は、行の遷移のケースと、絞り込んだ型のケース `C.K` の参照: C は classify、K はそのケース（docs/lsp.md「定義へジャンプ」）。
func TestRowCaseRefs(t *testing.T) {
	_, res := analyzeRow(t)
	want := map[string]int{"row.Stage": 0, "row.Life": 0}
	cases := map[string]int{}
	for _, r := range res.Refs {
		switch r.Target.Kind {
		case TargetDecl:
			if _, ok := want[r.Target.Decl]; ok {
				want[r.Target.Decl]++
			}
		case TargetCase:
			cases[r.Target.Decl+"."+r.Target.Name]++
		}
	}
	// Stage は、enter、advance（型に二つ、as に一つ）、force、revise（型）、keep（型）の七か所。Life は、enterOpen、
	// discard（型と as）、purge と、match の by の五か所
	if want["row.Stage"] != 7 || want["row.Life"] != 5 {
		t.Errorf("分類への参照 = %v", want)
	}
	for name, n := range map[string]int{"row.Stage.Draft": 3, "row.Stage.Accepted": 2, "row.Stage.Shipped": 2, "row.Life.Open": 3, "row.Life.Absent": 3} {
		if cases[name] != n {
			t.Errorf("ケース %s への参照 = %d、期待は %d", name, cases[name], n)
		}
	}
}
