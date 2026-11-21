// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 本体のあるデコレータの外側が、本体の順に
// 動くこと、proceed() が、内側の層を、op の引数のまま呼ぶこと、op の結果が、そのまま返ること、継ぎ目との入れ子の順
// （docs/plugin-go.md「本体のあるデコレータ」）。plugins/go の TestBehavior が、生成したコードのモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"example.com/app/gen/gate"
	"example.com/app/gen/shop"
	"example.com/app/gen/shop/shoptest"
	"example.com/app/gen/trace"
)

// log は、呼ばれた順。fail は、失敗させる葉の名前（"" なら、どれも、失敗しない）。
type log struct {
	lines []string
	fail  string
}

// errBroken は、葉が返す、仕様の例外でない失敗。
var errBroken = errors.New("broken")

func (l *log) note(s string) { l.lines = append(l.lines, s) }

func (l *log) expect(t *testing.T, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(l.lines, want) {
		t.Errorf("呼ばれた順:\n  got  %q\n  want %q", l.lines, want)
	}
	l.lines = nil
}

// shopLeaves は、shop の、人が書く葉。
type shopLeaves struct{ l *log }

func (x shopLeaves) Lookup(_ context.Context, id string) (shop.Item, error) {
	x.l.note("leaf Lookup " + id)
	if x.l.fail == "Lookup" {
		return shop.Item{}, errBroken
	}
	return shop.Item{Id: id}, nil
}
func (x shopLeaves) Ring(context.Context) error {
	x.l.note("leaf Ring")
	if x.l.fail == "Ring" {
		return errBroken
	}
	return nil
}
func (x shopLeaves) Save(_ context.Context, item shop.Item) (shop.Item, error) {
	x.l.note("leaf Save " + item.Id)
	return shop.Item{Id: item.Id + "!"}, nil
}
func (x shopLeaves) Open(context.Context) error  { x.l.note("leaf Open"); return nil }
func (x shopLeaves) Sweep(context.Context) error { x.l.note("leaf Sweep"); return nil }

// gateOps は、shop の配線（と、shop の op に付いた、gate のデコレータの本体）が呼ぶ、gate の op。
type gateOps struct {
	l  *log
	ok bool
}

func (x gateOps) Ticket(context.Context) (gate.Ticket, error) {
	x.l.note("gate Ticket")
	return gate.Ticket{Ok: x.ok}, nil
}
func (x gateOps) Note(_ context.Context, label string, times int64, memo *string) error {
	m := "nil"
	if memo != nil {
		m = *memo
	}
	x.l.note(fmt.Sprintf("gate Note %s %d %s", label, times, m))
	return nil
}

// gateCases は、分類 gate.Gate の述語: 切符の ok で決める。
type gateCases struct{}

func (gateCases) Open(_ context.Context, v gate.Ticket) (bool, error) { return v.Ok, nil }
func (gateCases) Shut(_ context.Context, v gate.Ticket) (bool, error) { return !v.Ok, nil }

// seams は、継ぎ目 trace.traced: 中身の前と後に、印を残す。
type seams struct{ l *log }

func (x seams) OpenTraceTraced(ctx context.Context, body func(ctx context.Context) error) error {
	x.l.note("seam Open before")
	err := body(ctx)
	x.l.note("seam Open after")
	return err
}
func (x seams) CloseTraceTraced(ctx context.Context, body func(ctx context.Context) error) error {
	x.l.note("seam Close before")
	err := body(ctx)
	x.l.note("seam Close after")
	return err
}

func newShop(l *log, open bool) shop.Ops {
	return shop.New(shopLeaves{l}, gateCases{}, gateOps{l, open}, seams{l})
}

// TestGuardBefore は、前で確かめるデコレータ: 開いていれば、中身を呼んで、その結果を、そのまま返す。閉まっていれば、
// 中身を呼ばずに、例外。
func TestGuardBefore(t *testing.T) {
	ctx, l := context.Background(), &log{}
	got, err := newShop(l, true).Find(ctx, "a")
	if err != nil || got != (shop.Item{Id: "a"}) {
		t.Fatalf("Find = %+v, %v", got, err)
	}
	l.expect(t, "gate Ticket", "leaf Lookup a")

	_, err = newShop(l, false).Find(ctx, "a")
	var closed gate.Closed
	if !errors.As(err, &closed) {
		t.Fatalf("閉まっているときの Find の失敗 = %v（期待は gate.Closed）", err)
	}
	l.expect(t, "gate Ticket")
}

// TestNoteAfter は、後で記録するデコレータ: 中身（葉）の結果を、そのまま返し、その後に、値の引数のリテラルで、記録する。
// 渡していない省略可能な引数は nil。
func TestNoteAfter(t *testing.T) {
	ctx, l := context.Background(), &log{}
	got, err := newShop(l, true).Save(ctx, shop.Item{Id: "b"})
	if err != nil || got != (shop.Item{Id: "b!"}) {
		t.Fatalf("Save = %+v, %v（期待は、葉が返した値のまま）", got, err)
	}
	l.expect(t, "leaf Save b", "gate Note save 2 nil")
}

// TestLayers は、継ぎ目と、本体のあるデコレータを重ねた op: 書かれた順に、外から内へ。
func TestLayers(t *testing.T) {
	ctx, l := context.Background(), &log{}
	// open: 継ぎ目 trace.traced → gate.guarded → gate.noted → 葉
	if err := newShop(l, true).Open(ctx); err != nil {
		t.Fatal(err)
	}
	l.expect(t, "seam Open before", "gate Ticket", "leaf Open", "gate Note open 1 朝", "seam Open after")
	err := newShop(l, false).Open(ctx)
	var closed gate.Closed
	if !errors.As(err, &closed) {
		t.Fatalf("閉まっているときの Open の失敗 = %v（期待は gate.Closed）", err)
	}
	l.expect(t, "seam Open before", "gate Ticket", "seam Open after")

	// close: gate.guarded → 継ぎ目 trace.traced → 配線の中身
	if err := newShop(l, true).Close(ctx); err != nil {
		t.Fatal(err)
	}
	l.expect(t, "gate Ticket", "seam Close before", "leaf Sweep", "seam Close after")
	if err := newShop(l, false).Close(ctx); !errors.As(err, &closed) {
		t.Fatalf("閉まっているときの Close の失敗 = %v（期待は gate.Closed）", err)
	}
	l.expect(t, "gate Ticket")
}

// gateLeaves は、gate の、人が書く葉。
type gateLeaves struct{ l *log }

func (x gateLeaves) Ticket(context.Context) (gate.Ticket, error)        { return gate.Ticket{Ok: true}, nil }
func (x gateLeaves) Note(context.Context, string, int64, *string) error { return nil }
func (x gateLeaves) Local(context.Context) error                        { x.l.note("leaf Local"); return nil }
func (x gateLeaves) Peek(context.Context) (gate.Ticket, error) {
	x.l.note("leaf Peek")
	return gate.Ticket{Ok: true}, nil
}

// TestSameNamespace は、宣言した名前空間の、葉の op に付けたデコレータ: 本体が、内部の op を呼び、中身は、Leaves のメソッド。
func TestSameNamespace(t *testing.T) {
	ctx, l := context.Background(), &log{}
	got, err := gate.New(gateLeaves{l}).Peek(ctx)
	if err != nil || !got.Ok {
		t.Fatalf("Peek = %+v, %v", got, err)
	}
	l.expect(t, "leaf Local", "leaf Peek")
}

// outcome は、継ぎ目 trace.traced の、結果一つを起こす実装: fail が nil なら、中身を呼んで、その結果を返す。nil でなければ、
// before なら、中身を呼ばずに、fail を返し、そうでなければ、中身を呼んでから、fail を返す。
type outcome struct {
	fail   error
	before bool
}

func (x outcome) run(ctx context.Context, body func(ctx context.Context) error) error {
	if x.fail != nil && x.before {
		return x.fail
	}
	if err := body(ctx); err != nil {
		return err
	}
	return x.fail
}
func (x outcome) OpenTraceTraced(ctx context.Context, body func(ctx context.Context) error) error {
	return x.run(ctx, body)
}
func (x outcome) CloseTraceTraced(ctx context.Context, body func(ctx context.Context) error) error {
	return x.run(ctx, body)
}

// traceCases は、デコレータ trace.traced の、結果ごとのケース（docs/plugin-go.md「デコレータのテストの口」）。
type traceCases struct{}

func seamsOf(x outcome) shoptest.TraceTracedCase {
	return shoptest.TraceTracedCase{Seams: func() shop.Seams { return x }}
}
func (traceCases) OK() shoptest.TraceTracedCase { return seamsOf(outcome{}) }
func (traceCases) Busy() shoptest.TraceTracedCase {
	return seamsOf(outcome{fail: trace.Busy{}, before: true})
}
func (traceCases) Late() shoptest.TraceTracedCase { return seamsOf(outcome{fail: trace.Late{}}) }
func (traceCases) Odd() shoptest.TraceTracedCase {
	return seamsOf(outcome{fail: trace.Odd{}, before: true})
}

type seamCases struct{}

func (seamCases) TraceTraced() shoptest.TraceTracedCases { return traceCases{} }

// TestSeamCases は、生成した RunSeams が、付けた op ごと（open、close）、結果ごとに、継ぎ目を呼び、関係ごとの確かめ
// （prevents effects: 中身を呼ばない。keeps effects: 中身を呼んでいる。関係なし: 例外だけ）を通すこと。
func TestSeamCases(t *testing.T) {
	shoptest.RunSeams(t, seamCases{})
}

// TestMatchThenStatement は、proceed() のアームを持つ match の後ろに、文が続くデコレータ（gate.checked）: proceed() のアームに
// 入ったとき、op の結果が、そのまま返り、後ろの文が、動く。throw のアームに入ったとき、中身も、後ろの文も、動かない。中身が
// 失敗したとき、後ろの文が、動かない。値を返す op（price）と、値を返さない op（ring）の、両方。
func TestMatchThenStatement(t *testing.T) {
	ctx, l := context.Background(), &log{}
	var closed gate.Closed

	// 値を返す op
	got, err := newShop(l, true).Price(ctx, "a")
	if err != nil || got != (shop.Item{Id: "a"}) {
		t.Fatalf("Price = %+v, %v（期待は、中身が返した値のまま）", got, err)
	}
	l.expect(t, "gate Ticket", "leaf Lookup a", "gate Note price 1 nil")
	if _, err := newShop(l, false).Price(ctx, "a"); !errors.As(err, &closed) {
		t.Fatalf("閉まっているときの Price の失敗 = %v（期待は gate.Closed）", err)
	}
	l.expect(t, "gate Ticket")
	l.fail = "Lookup"
	if _, err := newShop(l, true).Price(ctx, "a"); !errors.Is(err, errBroken) {
		t.Fatalf("中身が失敗したときの Price の失敗 = %v（期待は、中身の失敗のまま）", err)
	}
	l.expect(t, "gate Ticket", "leaf Lookup a")
	l.fail = ""

	// 値を返さない op
	if err := newShop(l, true).Ring(ctx); err != nil {
		t.Fatal(err)
	}
	l.expect(t, "gate Ticket", "leaf Ring", "gate Note ring 1 nil")
	if err := newShop(l, false).Ring(ctx); !errors.As(err, &closed) {
		t.Fatalf("閉まっているときの Ring の失敗 = %v（期待は gate.Closed）", err)
	}
	l.expect(t, "gate Ticket")
	l.fail = "Ring"
	if err := newShop(l, true).Ring(ctx); !errors.Is(err, errBroken) {
		t.Fatalf("中身が失敗したときの Ring の失敗 = %v（期待は、中身の失敗のまま）", err)
	}
	l.expect(t, "gate Ticket", "leaf Ring")
}

// TestNeverOp は、never を返す op に付けた、前で確かめるだけのデコレータ: 開いていれば、中身（いつも、shop.Denied を投げる）へ進み、
// 閉まっていれば、中身へ進まずに gate.Closed。
func TestNeverOp(t *testing.T) {
	ctx, l := context.Background(), &log{}
	var denied shop.Denied
	if err := newShop(l, true).Refuse(ctx); !errors.As(err, &denied) {
		t.Fatalf("開いているときの Refuse の失敗 = %v（期待は shop.Denied）", err)
	}
	l.expect(t, "gate Ticket")
	var closed gate.Closed
	if err := newShop(l, false).Refuse(ctx); !errors.As(err, &closed) {
		t.Fatalf("閉まっているときの Refuse の失敗 = %v（期待は gate.Closed）", err)
	}
	l.expect(t, "gate Ticket")
}
