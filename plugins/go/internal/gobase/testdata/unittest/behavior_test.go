// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 単体テストの口（plugins/go/spec/go.hozo の Requirement.UnitTestPort
// ）と、効果の入口の確かめ（同「効果の入口の確かめ」）。口を満たす型を、RunLeaves と Run<C> に渡して、すべての
// ケースが通ること。絞り込んだ引数を持つ効果は、引数が、言ったケースになければ、*runtime.CaseError を返して、単位を呼ばないこと。
// plugins/go の TestBehavior が、生成したコードのモジュールのルートに置いて走らせる。口が落とすこと（葉の結果が、ケースのものと
// 違う。例が、ケースに当たらない）は、plugins/go の TestUnitTestSeamFails が、このファイルの型を使って確かめる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/app"
	"example.com/app/gen/app/apptest"
	"example.com/app/gen/stock"
	"example.com/app/gen/stock/stocktest"
	"github.com/mew-ton/hozo/runtime"
)

// leaves は app.Leaves の実装。結果は、注文の note で決まる。wrongSend なら、send が、言ったケース（Sent）にない値を返す。
type leaves struct {
	wrongSend bool
}

func (leaves) Submit(_ context.Context, order app.Order, then string) (app.Order, error) {
	switch order.Note {
	case "reject":
		return app.Order{}, app.Rejected{Reason: "bad " + order.Id}
	case "busy":
		return app.Order{}, app.Busy{}
	case "shelf":
		return app.Order{}, stock.Busy{}
	}
	order.Note = then
	return order, nil
}
func (leaves) Log(context.Context, string) error { return nil }
func (l leaves) Send(_ context.Context, order app.Order) (app.Order, error) {
	if !l.wrongSend {
		order.Note = "sent"
	}
	return order, nil
}
func (leaves) Refill(_ context.Context, item stock.Item) (stock.Item, error) {
	item.Count += 3
	return item, nil
}
func (leaves) Reopen(_ context.Context, order app.Order) (app.Order, error) {
	order.Note = "open"
	return order, nil
}
func (leaves) Find(_ context.Context, id string) (*app.Order, error) {
	if id == "" {
		return nil, nil
	}
	return &app.Order{Id: id, Note: "hit"}, nil
}

// errStage は、stage の述語が返す、仕様の例外でない失敗。
var errStage = errors.New("stage: unreadable")

// stage は StageCases の実装: 注文の note が、ケースの名前。note が "!" なら、述語が失敗する。
type stage struct{}

func (stage) is(v app.Order, name string) (bool, error) {
	if v.Note == "!" {
		return false, errStage
	}
	return v.Note == name, nil
}
func (s stage) Draft(_ context.Context, v app.Order) (bool, error) { return s.is(v, "draft") }
func (s stage) Sent(_ context.Context, v app.Order) (bool, error)  { return s.is(v, "sent") }
func (s stage) Done(_ context.Context, v app.Order) (bool, error)  { return s.is(v, "done") }

// life は LifeCases の実装（ない側のケース Absent の述語は、ない）。
type life struct{}

func (life) Open(_ context.Context, v *app.Order) (bool, error)   { return v.Note == "open", nil }
func (life) Closed(_ context.Context, v *app.Order) (bool, error) { return v.Note == "closed", nil }

// lookup は LookupCases の実装。
type lookup struct{}

func (lookup) Hit(_ context.Context, v *app.Order) (bool, error) {
	return v != nil && v.Note == "hit", nil
}
func (lookup) Stale(_ context.Context, v *app.Order) (bool, error) {
	return v != nil && v.Note == "stale", nil
}
func (lookup) Miss(_ context.Context, v *app.Order) (bool, error) { return v == nil, nil }

// level は stock.LevelCases の実装: 数で決まる。
type level struct{}

func (level) Plenty(_ context.Context, v stock.Item) (bool, error) { return v.Count >= 10, nil }
func (level) Few(_ context.Context, v stock.Item) (bool, error) {
	return v.Count > 0 && v.Count < 10, nil
}
func (level) Out(_ context.Context, v stock.Item) (bool, error) { return v.Count <= 0, nil }

// backlog は BacklogState の実装。
type backlog struct{}

func (backlog) Calm(context.Context) (bool, error) { return true, nil }
func (backlog) Rush(context.Context) (bool, error) { return false, nil }

// units は OrdersState と ShelfState の実装。効果のメソッドは、呼ばれたことを覚えるだけ（引数のケースは、見ない）。
type units struct {
	calls []string
}

func (u *units) IsEmpty(context.Context) (bool, error) { return true, nil }
func (u *units) call(name string) error {
	u.calls = append(u.calls, name)
	return nil
}

type orders struct{ *units }

func (s orders) Get(context.Context) ([]app.Order, error)       { return nil, nil }
func (s orders) Close(_ context.Context, order app.Order) error { return s.call("close " + order.Id) }
func (s orders) Discard(_ context.Context, order app.Order) error {
	return s.call("discard " + order.Id)
}
func (s orders) Keep(_ context.Context, order app.Order) error { return s.call("keep " + order.Id) }
func (s orders) CloseAll(_ context.Context, sent []app.Order) error {
	return s.call("closeAll")
}

type shelf struct{ *units }

func (s shelf) Get(context.Context) ([]stock.Item, error) { return nil, nil }
func (s shelf) Restock(_ context.Context, item stock.Item, editor app.Order, count int64) error {
	return s.call("restock " + item.Name + " by " + editor.Id)
}

func setup(l leaves) (*units, app.Ops) {
	u := &units{}
	return u, app.New(l, orders{u}, shelf{u}, backlog{}, life{}, stage{}, level{})
}

// ---- 単体テストの口: 葉の op ----

// thens は、Then が呼ばれたケース（<op>/<結果>）。
var thens []string

func then(name string) func(*testing.T, app.Order, error) {
	return func(*testing.T, app.Order, error) { thens = append(thens, name) }
}

// leafCases は apptest.LeafCases を満たす。
type leafCases struct{}

var _ apptest.LeafCases = leafCases{}

type submitCases struct{}

func (submitCases) OK() apptest.SubmitCase {
	return apptest.SubmitCase{Order: app.Order{Id: "o1", Note: "new"}, Then_: "kept", Then: func(t *testing.T, got app.Order, err error) {
		thens = append(thens, "submit/OK")
		if got.Note != "kept" || err != nil {
			t.Errorf("submit = %+v, %v（引数 then が、note に入るはず）", got, err)
		}
	}}
}
func (submitCases) Rejected() apptest.SubmitCase {
	return apptest.SubmitCase{Order: app.Order{Id: "o2", Note: "reject"}, Then: func(t *testing.T, _ app.Order, err error) {
		thens = append(thens, "submit/Rejected")
		// 例外が運ぶ値は、Then が、err から確かめる
		var ex app.Rejected
		if !errors.As(err, &ex) || ex.Reason != "bad o2" {
			t.Errorf("Rejected の値 = %+v", ex)
		}
	}}
}
func (submitCases) AppBusy() apptest.SubmitCase {
	return apptest.SubmitCase{Order: app.Order{Note: "busy"}, Then: then("submit/AppBusy")}
}
func (submitCases) StockBusy() apptest.SubmitCase {
	return apptest.SubmitCase{Order: app.Order{Note: "shelf"}, Then: then("submit/StockBusy")}
}
func (leafCases) Submit() apptest.SubmitCases { return submitCases{} }

type logCases struct{}

func (logCases) OK() apptest.LogCase {
	return apptest.LogCase{Text: "hello", Then: func(*testing.T, error) { thens = append(thens, "log/OK") }}
}
func (leafCases) Log() apptest.LogCases { return logCases{} }

type sendCases struct{}

func (sendCases) OK() apptest.SendCase {
	return apptest.SendCase{Order: app.Order{Id: "o3", Note: "draft"}, Then: then("send/OK")}
}
func (leafCases) Send() apptest.SendCases { return sendCases{} }

type refillCases struct{}

// Then を書かないケース（nil）: 結果の見分けだけで、足りる
func (refillCases) OK() apptest.RefillCase {
	return apptest.RefillCase{Item: stock.Item{Name: "pen", Count: 0}}
}
func (leafCases) Refill() apptest.RefillCases { return refillCases{} }

type reopenCases struct{}

func (reopenCases) OK() apptest.ReopenCase {
	return apptest.ReopenCase{Order: app.Order{Id: "o4", Note: "closed"}, Then: then("reopen/OK")}
}
func (leafCases) Reopen() apptest.ReopenCases { return reopenCases{} }

type findCases struct{}

func (findCases) OK() apptest.FindCase {
	return apptest.FindCase{Id: "o5", Then: func(t *testing.T, got *app.Order, _ error) {
		thens = append(thens, "find/OK")
		if got == nil || got.Id != "o5" {
			t.Errorf("find = %+v", got)
		}
	}}
}
func (leafCases) Find() apptest.FindCases { return findCases{} }

// すべての葉の、すべての結果のケースが通り、結果の見分けの後で、Then が、書かれた順に呼ばれる
func TestLeafCases(t *testing.T) {
	thens = nil
	apptest.RunLeaves(t, leaves{}, life{}, stage{}, level{}, leafCases{})
	want := []string{"submit/OK", "submit/Rejected", "submit/AppBusy", "submit/StockBusy", "log/OK", "send/OK", "reopen/OK", "find/OK"}
	if !reflect.DeepEqual(thens, want) {
		t.Errorf("呼ばれた Then = %v, want %v", thens, want)
	}
}

// ---- 単体テストの口: 分類の判定 ----

type stageExamples struct{}

func (stageExamples) Draft() app.Order { return app.Order{Note: "draft"} }
func (stageExamples) Sent() app.Order  { return app.Order{Note: "sent"} }
func (stageExamples) Done() app.Order  { return app.Order{Note: "done"} }

// lifeExamples は、ない側のケース Absent のメソッドを持たない: この型を RunLife に渡せることが、ない側のケースが例を求めない
// ことを、コンパイルで確かめる。
type lifeExamples struct{}

func (lifeExamples) Open() *app.Order   { return &app.Order{Note: "open"} }
func (lifeExamples) Closed() *app.Order { return &app.Order{Note: "closed"} }

type lookupExamples struct{}

func (lookupExamples) Hit() *app.Order   { return &app.Order{Note: "hit"} }
func (lookupExamples) Stale() *app.Order { return &app.Order{Note: "stale"} }
func (lookupExamples) Miss() *app.Order  { return nil }

type levelExamples struct{}

func (levelExamples) Plenty() stock.Item { return stock.Item{Count: 10} }
func (levelExamples) Few() stock.Item    { return stock.Item{Count: 9} }
func (levelExamples) Out() stock.Item    { return stock.Item{} }

func TestStage(t *testing.T)  { apptest.RunStage(t, stage{}, stageExamples{}) }
func TestLife(t *testing.T)   { apptest.RunLife(t, life{}, lifeExamples{}) }
func TestLookup(t *testing.T) { apptest.RunLookup(t, lookup{}, lookupExamples{}) }
func TestLevel(t *testing.T)  { stocktest.RunLevel(t, level{}, levelExamples{}) }

// ---- 効果の入口の確かめ ----

// caseError は、err が *runtime.CaseError で、中身が want と同じことを確かめる。
func caseError(t *testing.T, what string, err error, want runtime.CaseError) {
	t.Helper()
	var ce *runtime.CaseError
	if !errors.As(err, &ce) {
		t.Errorf("%s: err = %v, want *runtime.CaseError", what, err)
		return
	}
	if !reflect.DeepEqual(*ce, want) {
		t.Errorf("%s: CaseError = %+v, want %+v", what, *ce, want)
	}
	// 確かめの失敗は、仕様の例外ではない
	var ex runtime.Exception
	if errors.As(err, &ex) {
		t.Errorf("%s: CaseError が、仕様の例外として読めた", what)
	}
}

// 引数が、言ったケースにあれば、単位のメソッドが呼ばれる。なければ、*runtime.CaseError が返り、単位は呼ばれない
func TestEntryCheck(t *testing.T) {
	ctx := context.Background()
	u, o := setup(leaves{})

	// ケースが二つ（Sent か Done）
	for _, note := range []string{"sent", "done"} {
		if err := o.Finish(ctx, app.Order{Id: note, Note: note}); err != nil {
			t.Errorf("Finish(%s) = %v", note, err)
		}
	}
	err := o.Finish(ctx, app.Order{Id: "d", Note: "draft"})
	caseError(t, "Finish(draft)", err, runtime.CaseError{Effect: "app.close", Param: "order", Classify: "app.Stage", Want: []string{"Sent", "Done"}, Got: "Draft"})
	if want := "effect app.close: argument order is in case Draft of app.Stage, want Sent, Done"; err == nil || err.Error() != want {
		t.Errorf("CaseError の文面 = %v, want %s", err, want)
	}

	// ない側のケースを持つ分類（判定関数には、引数のアドレスを渡す）
	if err := o.Drop(ctx, app.Order{Id: "op", Note: "open"}); err != nil {
		t.Errorf("Drop(open) = %v", err)
	}
	caseError(t, "Drop(closed)", o.Drop(ctx, app.Order{Id: "cl", Note: "closed"}),
		runtime.CaseError{Effect: "app.discard", Param: "order", Classify: "app.Life", Want: []string{"Open"}, Got: "Closed"})

	if want := []string{"close sent", "close done", "discard op"}; !reflect.DeepEqual(u.calls, want) {
		t.Errorf("呼ばれた効果 = %v, want %v", u.calls, want)
	}
}

// 絞り込んだ引数が二つの効果は、宣言の順に、一つずつ確かめる。別の名前空間の分類の述語は、New に渡したものが使われる
func TestEntryCheckOfTwoParams(t *testing.T) {
	ctx := context.Background()
	u, o := setup(leaves{})
	out, few := stock.Item{Name: "pen", Count: 0}, stock.Item{Name: "ink", Count: 2}
	draft, sent := app.Order{Id: "d", Note: "draft"}, app.Order{Id: "s", Note: "sent"}

	if err := o.Fill(ctx, out, draft, 5); err != nil {
		t.Errorf("Fill(out, draft) = %v", err)
	}
	caseError(t, "Fill(few, draft)", o.Fill(ctx, few, draft, 5),
		runtime.CaseError{Effect: "app.restock", Param: "item", Classify: "stock.Level", Want: []string{"Out"}, Got: "Few"})
	caseError(t, "Fill(out, sent)", o.Fill(ctx, out, sent, 5),
		runtime.CaseError{Effect: "app.restock", Param: "editor", Classify: "app.Stage", Want: []string{"Draft"}, Got: "Sent"})
	// 二つとも違えば、先の引数の失敗
	caseError(t, "Fill(few, sent)", o.Fill(ctx, few, sent, 5),
		runtime.CaseError{Effect: "app.restock", Param: "item", Classify: "stock.Level", Want: []string{"Out"}, Got: "Few"})
	if want := []string{"restock pen by d"}; !reflect.DeepEqual(u.calls, want) {
		t.Errorf("呼ばれた効果 = %v, want %v", u.calls, want)
	}
}

// 判定関数の失敗（述語の error と、分類の誤り）は、そのまま返り、単位は呼ばれない
func TestEntryCheckPassesClassifyErrors(t *testing.T) {
	ctx := context.Background()
	u, o := setup(leaves{})
	if err := o.Finish(ctx, app.Order{Note: "!"}); !errors.Is(err, errStage) {
		t.Errorf("述語の失敗: err = %v, want %v", err, errStage)
	}
	var ce *runtime.ClassificationError
	if err := o.Finish(ctx, app.Order{Note: "?"}); !errors.As(err, &ce) {
		t.Errorf("どの述語も真でない: err = %v, want *runtime.ClassificationError", err)
	}
	if len(u.calls) != 0 {
		t.Errorf("確かめが通らないのに、効果を呼んだ: %v", u.calls)
	}
}

// 絞り込んだ引数を持たない効果と、並びの中の絞り込みは、確かめない
func TestUncheckedEffects(t *testing.T) {
	ctx := context.Background()
	u, o := setup(leaves{})
	if err := o.Store(ctx, app.Order{Id: "x", Note: "?"}); err != nil {
		t.Errorf("Store = %v", err)
	}
	if err := o.FinishAll(ctx, []app.Order{{Id: "y", Note: "draft"}}); err != nil {
		t.Errorf("FinishAll = %v", err)
	}
	if want := []string{"keep x", "closeAll"}; !reflect.DeepEqual(u.calls, want) {
		t.Errorf("呼ばれた効果 = %v, want %v", u.calls, want)
	}
}

// 葉が、絞り込んだ型で返した値が、言ったケースになければ、それを受ける効果の入口で止まる
func TestEntryCheckStopsWrongLeaf(t *testing.T) {
	ctx := context.Background()
	u, o := setup(leaves{})
	if err := o.SendAndFinish(ctx, app.Order{Id: "a", Note: "draft"}); err != nil {
		t.Errorf("SendAndFinish = %v", err)
	}
	wrong, o2 := setup(leaves{wrongSend: true})
	caseError(t, "SendAndFinish（send が Draft のまま返す）", o2.SendAndFinish(ctx, app.Order{Id: "b", Note: "draft"}),
		runtime.CaseError{Effect: "app.close", Param: "order", Classify: "app.Stage", Want: []string{"Sent", "Done"}, Got: "Draft"})
	if want := []string{"close a"}; !reflect.DeepEqual(u.calls, want) || len(wrong.calls) != 0 {
		t.Errorf("呼ばれた効果 = %v と %v, want %v と、なし", u.calls, wrong.calls, want)
	}
}
