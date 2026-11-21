// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 行の遷移を持つ効果は、宛先の状態の単位の
// メソッドのままで、配線は、仕様の呼び出しを、そのまま写す（基底は、効果の後で、行が、言ったケースにあるかを確かめるコードを
// 足さない。docs/plugin-go.md「状態の単位」）。効果の前の、行のケース（絞り込んだ引数）は、効果の入口で確かめる（同「効果の入口の
// 確かめ」）。ない側のケースを持つ、述語で決まる分類は、述語を持たないケースを、判定関数が、値がないことで決める（同「分類」）。
// plugins/go の TestBehavior が、生成したコードのモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"example.com/app/gen/app"
	"github.com/mew-ton/hozo/runtime"
)

// orders は OrdersState の実装。効果のメソッドは、呼ばれたことと、受け取った行を覚えるだけ（行のケースは、見ない）。
type orders struct {
	rows  map[string]app.Order
	calls []string
}

func (s *orders) IsEmpty(context.Context) (bool, error) { return len(s.rows) == 0, nil }
func (s *orders) Get(context.Context) ([]app.Order, error) {
	var out []app.Order
	for _, o := range s.rows {
		out = append(out, o)
	}
	return out, nil
}
func (s *orders) OrderById(_ context.Context, id string) (*app.Order, error) {
	if o, ok := s.rows[id]; ok {
		return &o, nil
	}
	return nil, nil
}
func (s *orders) call(name string, order app.Order) error {
	s.calls = append(s.calls, name+" "+order.Id)
	return nil
}
func (s *orders) Accept(_ context.Context, order app.Order) error { return s.call("accept", order) }
func (s *orders) Open(_ context.Context, order app.Order) error   { return s.call("open", order) }
func (s *orders) Bring(_ context.Context, order app.Order) error  { return s.call("bring", order) }
func (s *orders) Start(_ context.Context, order app.Order, at time.Time) error {
	return s.call("start@"+at.Format("15:04"), order)
}
func (s *orders) Cancel(_ context.Context, order app.Order) error { return s.call("cancel", order) }
func (s *orders) ForceShip(_ context.Context, order app.Order) error {
	return s.call("forceShip", order)
}
func (s *orders) Revise(_ context.Context, order app.Order, note string) error {
	return s.call("revise("+note+")", order)
}
func (s *orders) Patch(_ context.Context, order app.Order) error   { return s.call("patch", order) }
func (s *orders) Rename(_ context.Context, order app.Order) error  { return s.call("rename", order) }
func (s *orders) Discard(_ context.Context, order app.Order) error { return s.call("discard", order) }
func (s *orders) Purge(_ context.Context, order app.Order) error   { return s.call("purge", order) }

// stage は StageCases の実装: 注文の note が、ケースの名前。
type stage struct{}

func (stage) Draft(_ context.Context, v app.Order) (bool, error)    { return v.Note == "draft", nil }
func (stage) Accepted(_ context.Context, v app.Order) (bool, error) { return v.Note == "accepted", nil }
func (stage) InProduction(_ context.Context, v app.Order) (bool, error) {
	return v.Note == "inProduction", nil
}
func (stage) Shipped(_ context.Context, v app.Order) (bool, error) { return v.Note == "shipped", nil }
func (stage) Cancelled(_ context.Context, v app.Order) (bool, error) {
	return v.Note == "cancelled", nil
}

// life は LifeCases の実装。メソッドは、ない側のケース Absent を除く、二つだけ: Absent のメソッドを持たないこの型を、
// app.New に渡せることが、ない側のケースが述語を持たないことを、コンパイルで確かめる。呼ばれた述語を覚える。
type life struct {
	asked      []string
	open, both bool
}

func (l *life) Open(_ context.Context, v *app.Order) (bool, error) {
	l.asked = append(l.asked, "Open")
	return l.both || (l.open && v.Note == "open"), nil
}
func (l *life) Closed(_ context.Context, v *app.Order) (bool, error) {
	l.asked = append(l.asked, "Closed")
	return l.both || v.Note == "closed", nil
}

func setup(rows ...app.Order) (*orders, *life, app.Ops) {
	s := &orders{rows: map[string]app.Order{}}
	for _, r := range rows {
		s.rows[r.Id] = r
	}
	l := &life{open: true}
	return s, l, app.New(s, l, stage{})
}

// LifeCases は、ない側のケースの述語を持たない（docs/plugin-go.md「分類」）
func TestAbsentCaseHasNoPredicate(t *testing.T) {
	typ := reflect.TypeOf((*app.LifeCases)(nil)).Elem()
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	if !reflect.DeepEqual(names, []string{"Closed", "Open"}) {
		t.Errorf("LifeCases のメソッド = %v, want [Closed Open]", names)
	}
}

// 判定関数: 値がなければ、述語を一つも呼ばずに、ない側のケース。値があれば、ほかのケースの述語で決まる
func TestClassifyWithAbsentCase(t *testing.T) {
	ctx := context.Background()
	l := &life{open: true}
	if got, err := app.ClassifyLife(ctx, l, nil); err != nil || got != app.LifeAbsent {
		t.Errorf("ClassifyLife(nil) = %v, %v, want LifeAbsent", got, err)
	}
	if len(l.asked) != 0 {
		t.Errorf("値がないのに、述語を呼んだ: %v", l.asked)
	}
	if got, err := app.ClassifyLife(ctx, l, &app.Order{Id: "o1", Note: "open"}); err != nil || got != app.LifeOpen {
		t.Errorf("ClassifyLife(open) = %v, %v, want LifeOpen", got, err)
	}
	if !reflect.DeepEqual(l.asked, []string{"Open", "Closed"}) {
		t.Errorf("値があるとき、呼んだ述語 = %v, want [Open Closed]", l.asked)
	}
	if got, err := app.ClassifyLife(ctx, l, &app.Order{Id: "o2", Note: "closed"}); err != nil || got != app.LifeClosed {
		t.Errorf("ClassifyLife(closed) = %v, %v, want LifeClosed", got, err)
	}
	// 値があって、どの述語も真でない、二つが真: 分類の誤り。ない側のケースは、Hits に入らない
	var ce *runtime.ClassificationError
	if _, err := app.ClassifyLife(ctx, l, &app.Order{Id: "o3", Note: "?"}); !errors.As(err, &ce) || len(ce.Hits) != 0 {
		t.Errorf("どの述語も真でない: err = %v", err)
	}
	if _, err := app.ClassifyLife(ctx, &life{both: true}, &app.Order{Id: "o4"}); !errors.As(err, &ce) || !reflect.DeepEqual(ce.Hits, []string{"Open", "Closed"}) {
		t.Errorf("二つが真: err = %v", err)
	}
}

// 配線は、仕様の呼び出しを、そのまま写す: 受けたアームで、効果のメソッドを、渡した行と値で呼ぶ
func TestRowTransitionCalls(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 1, 2, 9, 30, 0, 0, time.UTC)
	s, _, o := setup(app.Order{Id: "a", Note: "accepted"}, app.Order{Id: "d", Note: "draft"})

	if err := o.Begin(ctx, "a", at); err != nil {
		t.Fatalf("Begin(a) = %v", err)
	}
	if err := o.Begin(ctx, "d", at); !errors.Is(err, app.NotAccepted{}) {
		t.Errorf("Begin(d) = %v, want NotAccepted", err)
	}
	if err := o.Begin(ctx, "x", at); !errors.Is(err, app.NoOrder{}) {
		t.Errorf("Begin(x) = %v, want NoOrder", err)
	}
	if err := o.Amend(ctx, app.Order{Id: "a", Note: "accepted"}, "hurry"); err != nil {
		t.Fatalf("Amend = %v", err)
	}
	if want := []string{"start@09:30 a", "revise(hurry) a"}; !reflect.DeepEqual(s.calls, want) {
		t.Errorf("呼んだ効果 = %v, want %v", s.calls, want)
	}
}

// ない側のケースを持つ分類での match: 値があって Open のアームだけが、効果を呼ぶ。値がなければ、ない側のケースのアーム
func TestDeleteWithAbsentCase(t *testing.T) {
	ctx := context.Background()
	s, l, o := setup(app.Order{Id: "o", Note: "open"}, app.Order{Id: "c", Note: "closed"})

	if err := o.Drop(ctx, "o"); err != nil {
		t.Fatalf("Drop(o) = %v", err)
	}
	if err := o.Drop(ctx, "c"); !errors.Is(err, app.NotOpen{}) {
		t.Errorf("Drop(c) = %v, want NotOpen", err)
	}
	l.asked = nil
	if err := o.Drop(ctx, "x"); !errors.Is(err, app.NoOrder{}) {
		t.Errorf("Drop(x) = %v, want NoOrder", err)
	}
	if len(l.asked) != 0 {
		t.Errorf("行がないのに、述語を呼んだ: %v", l.asked)
	}
	if want := []string{"discard o"}; !reflect.DeepEqual(s.calls, want) {
		t.Errorf("呼んだ効果 = %v, want %v", s.calls, want)
	}
	for _, tt := range []struct {
		found *app.Order
		want  string
	}{
		{&app.Order{Id: "o", Note: "open"}, "open"},
		{&app.Order{Id: "c", Note: "closed"}, "closed"},
		{nil, "absent"},
	} {
		if got, err := o.LifeName(ctx, tt.found); err != nil || got != tt.want {
			t.Errorf("LifeName(%v) = %q, %v, want %q", tt.found, got, err, tt.want)
		}
	}
}

// 行の引数を絞り込んだ効果は、入口で、行が、言ったケースにあるかを確かめる: 型から Accepted と分かっているはずの注文が、
// Accepted になければ、*runtime.CaseError が返り、単位のメソッドは、呼ばれない
func TestRowCaseIsCheckedAtEntry(t *testing.T) {
	ctx := context.Background()
	s, _, o := setup()
	err := o.Amend(ctx, app.Order{Id: "d", Note: "draft"}, "hurry")
	var ce *runtime.CaseError
	if !errors.As(err, &ce) {
		t.Fatalf("Amend(draft) = %v, want *runtime.CaseError", err)
	}
	want := runtime.CaseError{Effect: "app.revise", Param: "order", Classify: "app.Stage", Want: []string{"Accepted"}, Got: "Draft"}
	if !reflect.DeepEqual(*ce, want) {
		t.Errorf("CaseError = %+v, want %+v", *ce, want)
	}
	if len(s.calls) != 0 {
		t.Errorf("確かめが通らないのに、効果を呼んだ: %v", s.calls)
	}
}
