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
)

// log は、呼ばれた順。
type log struct{ lines []string }

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
	return shop.Item{Id: id}, nil
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

// seams は、継ぎ目 t.traced: 中身の前と後に、印を残す。
type seams struct{ l *log }

func (x seams) OpenTTraced(ctx context.Context, body func(ctx context.Context) error) error {
	x.l.note("seam Open before")
	err := body(ctx)
	x.l.note("seam Open after")
	return err
}
func (x seams) CloseTTraced(ctx context.Context, body func(ctx context.Context) error) error {
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
	// open: 継ぎ目 t.traced → gate.guarded → gate.noted → 葉
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

	// close: gate.guarded → 継ぎ目 t.traced → 配線の中身
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
