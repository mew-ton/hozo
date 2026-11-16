// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 述語で決まる分類の判定
// （docs/plugin-go.md「分類」）と、それで分ける配線。plugins/go の TestBehavior が、生成したコードのモジュールのルートに
// 置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/app"
	"github.com/mew-ton/hozo/runtime"
)

// sizes は SizeCases の実装。item.Price が、ケースごとの印（ビット）。
type sizes struct{}

func (sizes) Big(v app.Item) bool   { return v.Price&1 != 0 }
func (sizes) Small(v app.Item) bool { return v.Price&2 != 0 }
func (sizes) Tiny(v app.Item) bool  { return v.Price&4 != 0 }

type addresses struct{}

func (addresses) Valid(v string) bool { return v == "a@example.com" }

// contacts は ContactCases の実装。呼ばれた述語を覚える。
type contacts struct{ called []string }

func (c *contacts) Mail(v *string) bool {
	c.called = append(c.called, "Mail")
	return v != nil && *v == "a@example.com"
}

func (c *contacts) Text(v *string) bool {
	c.called = append(c.called, "Text")
	return *v == "plain" // v が nil なら、ここで落ちる（判定関数は、nil のときにこの述語を呼ばない）
}

type stocks struct{}

func (stocks) InStock(v *app.Item) bool { return v.Price > 0 }
func (stocks) SoldOut(v *app.Item) bool { return v.Price == 0 }

// checkout は CheckoutState の実装。
type checkout struct {
	shopping, ordered bool
	shoppingErr       error
	orderedCalled     bool
}

func (c *checkout) Shopping(context.Context) (bool, error) { return c.shopping, c.shoppingErr }
func (c *checkout) Ordered(context.Context) (bool, error) {
	c.orderedCalled = true
	return c.ordered, nil
}

type leaves struct{ shown []string }

func (*leaves) FindItem(_ context.Context, name string) (*app.Item, error) {
	switch name {
	case "none":
		return nil, nil
	case "sold":
		return &app.Item{Name: name}, nil
	}
	return &app.Item{Name: name, Price: 7}, nil
}
func (*leaves) Load(_ context.Context, name string) (string, error) { return "loaded " + name, nil }
func (*leaves) TitleOf(ready app.Title) string                      { return ready.Title }
func (*leaves) CodeText(code int64) string                          { return "code" }
func (*leaves) ReceiptText(app.Receipt) string                      { return "receipt" }
func (*leaves) Send(addr app.Email) string                          { return "sent " + string(addr) }
func (*leaves) PriceOf(item app.Item) int64                         { return item.Price }
func (*leaves) BigOnly(item app.Item) string                        { return "big " + item.Name }
func (l *leaves) Show(text string)                                  { l.shown = append(l.shown, text) }

type stage struct{ page app.Page }

func (s stage) Get(context.Context) (app.Page, error) { return s.page, nil }

func newOps(l *leaves, st stage, co *checkout, cs *contacts) app.Ops {
	return app.New(l, st, nil, nil, co, addresses{}, cs, sizes{}, stocks{})
}

func classificationError(t *testing.T, err error, classify string, hits []string) {
	t.Helper()
	var ce *runtime.ClassificationError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v（*runtime.ClassificationError でない）", err)
	}
	if ce.Classify != classify || !reflect.DeepEqual(append([]string{}, ce.Hits...), append([]string{}, hits...)) {
		t.Errorf("ClassificationError = %+v, want Classify=%s Hits=%v", ce, classify, hits)
	}
}

// 判定関数: 真がちょうど一つならそのケース。hasDefault が false なら、0 でも 2 以上でも ClassificationError
func TestClassifyWithoutDefault(t *testing.T) {
	for price, want := range map[int64]app.Size{1: app.SizeBig, 2: app.SizeSmall, 4: app.SizeTiny} {
		got, err := app.ClassifySize(sizes{}, app.Item{Price: price})
		if err != nil || got != want {
			t.Errorf("ClassifySize(%d) = %v, %v", price, got, err)
		}
	}
	_, err := app.ClassifySize(sizes{}, app.Item{Price: 0})
	classificationError(t, err, "app.Size", nil)
	_, err = app.ClassifySize(sizes{}, app.Item{Price: 5})
	classificationError(t, err, "app.Size", []string{"Big", "Tiny"})
}

// 判定関数: hasDefault が true なら、真が 0 で <C>Default
func TestClassifyWithDefault(t *testing.T) {
	if got, err := app.ClassifyAddress(addresses{}, "a@example.com"); err != nil || got != app.AddressValid {
		t.Errorf("Valid: %v, %v", got, err)
	}
	if got, err := app.ClassifyAddress(addresses{}, "x"); err != nil || got != app.AddressDefault {
		t.Errorf("default: %v, %v", got, err)
	}
}

// 型で決まる絞り込み（presence）の as を持つケースは、値がその型に当たらなければ、述語を呼ばずに偽とする
func TestClassifyPresenceGuard(t *testing.T) {
	cs := &contacts{}
	got, err := app.ClassifyContact(cs, nil)
	if err != nil || got != app.ContactDefault {
		t.Errorf("nil: %v, %v", got, err)
	}
	if !reflect.DeepEqual(cs.called, []string{"Mail"}) {
		t.Errorf("呼ばれた述語 = %v（Text は、値がないので呼ばない）", cs.called)
	}
	plain := "plain"
	if got, err := app.ClassifyContact(&contacts{}, &plain); err != nil || got != app.ContactText {
		t.Errorf("plain: %v, %v", got, err)
	}
}

// 状態の分類の判定関数: 述語がエラーを返したら、残りも呼んだうえで最初のエラーを返す
func TestClassifyState(t *testing.T) {
	ctx := context.Background()
	if got, err := app.ClassifyCheckout(ctx, &checkout{ordered: true}); err != nil || got != app.CheckoutOrdered {
		t.Errorf("Ordered: %v, %v", got, err)
	}
	boom := errors.New("boom")
	co := &checkout{shoppingErr: boom, ordered: true}
	if _, err := app.ClassifyCheckout(ctx, co); err != boom {
		t.Errorf("err = %v, want boom", err)
	}
	if !co.orderedCalled {
		t.Error("エラーの後の述語 Ordered を呼んでいない")
	}
	_, err := app.ClassifyCheckout(ctx, &checkout{shopping: true, ordered: true})
	classificationError(t, err, "app.Checkout", []string{"Shopping", "Ordered"})
}

// 配線: 述語で決まる分類で分ける。判定の誤りは、配線の error になる
func TestWiring(t *testing.T) {
	ctx := context.Background()
	l := &leaves{}
	o := newOps(l, stage{page: app.Title{Title: "top"}}, &checkout{shopping: true}, &contacts{})
	for price, want := range map[int64]string{1: "big box", 2: "small", 4: "loaded box"} {
		got, err := o.SizeLabel(ctx, app.Item{Name: "box", Price: price})
		if err != nil || got != want {
			t.Errorf("SizeLabel(%d) = %q, %v", price, got, err)
		}
	}
	_, err := o.SizeLabel(ctx, app.Item{Price: 3})
	classificationError(t, err, "app.Size", []string{"Big", "Small"})

	// 絞り込みが predicate: string を Email に、string? を Email / string に
	mail, plain := "a@example.com", "plain"
	if got, err := o.Mail("a@example.com", &mail); err != nil || got != "sent a@example.com" {
		t.Errorf("Mail(mail) = %q, %v", got, err)
	}
	if got, err := o.Mail("x", &plain); err != nil || got != "plain" {
		t.Errorf("Mail(plain) = %q, %v", got, err)
	}
	if _, err := o.Mail("x", nil); !errors.Is(err, app.Bad{}) {
		t.Errorf("Mail(nil) の err = %v, want app.Bad", err)
	}
	if !reflect.DeepEqual(l.shown, []string{"sent a@example.com", "invalid", "invalid"}) {
		t.Errorf("shown = %v", l.shown)
	}

	// op の分類: const、throw、「返らない」呼び出し
	if got, err := o.Price(ctx, "box"); err != nil || got != 7 {
		t.Errorf("Price(box) = %d, %v", got, err)
	}
	if _, err := o.Price(ctx, "sold"); !errors.Is(err, app.Bad{}) {
		t.Errorf("Price(sold) の err = %v", err)
	}
	if _, err := o.Price(ctx, "none"); !errors.Is(err, app.Bad{}) {
		t.Errorf("Price(none) の err = %v", err)
	}

	// 型で決まる分類（variant）: 値と、状態
	if got := o.Describe(app.PageCode{Value: 3}); got != "code" {
		t.Errorf("Describe(Code) = %q", got)
	}
	if got := o.Describe(app.Problem{Reason: "r"}); got != "r" {
		t.Errorf("Describe(Failed) = %q", got)
	}
	if got := o.Describe(app.PageNote{}); got != "other" {
		t.Errorf("Describe(Note) = %q", got)
	}
	if got, err := o.StageText(ctx); err != nil || got != "top" {
		t.Errorf("StageText = %q, %v", got, err)
	}

	// 状態の分類: 文の match
	l.shown = nil
	if err := o.CheckoutNote(ctx); err != nil || !reflect.DeepEqual(l.shown, []string{"shopping"}) {
		t.Errorf("CheckoutNote: %v, shown = %v", err, l.shown)
	}
}

// 純粋で失敗しない配線は、判定の誤りを返す先がないので、panic する
func TestPureWiringPanics(t *testing.T) {
	o := newOps(&leaves{}, stage{}, &checkout{}, &contacts{})
	if got := o.PureLabel(app.Item{Price: 4}); got != "tiny" {
		t.Errorf("PureLabel = %q", got)
	}
	defer func() {
		r := recover()
		err, _ := r.(error)
		if err == nil {
			t.Fatalf("recover() = %v（error でない）", r)
		}
		classificationError(t, err, "app.Size", nil)
	}()
	o.PureLabel(app.Item{Price: 0})
	t.Error("panic しなかった")
}
