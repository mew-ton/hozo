// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 述語で決まる分類の判定
// （docs/plugin-go.md「分類」）と、それで分ける配線、型で決まる分類の「分類の誤りの枝」（同「match」）。
// plugins/go の TestBehavior が、生成したコードのモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/app"
	"github.com/mew-ton/hozo/runtime"
)

// sizes は SizeCases の実装。item.Price が、ケースごとの印（ビット）。述語は、op と同じに、ctx を取り、error を返す。
type sizes struct{}

func (sizes) Big(_ context.Context, v app.Item) (bool, error)   { return v.Price&1 != 0, nil }
func (sizes) Small(_ context.Context, v app.Item) (bool, error) { return v.Price&2 != 0, nil }
func (sizes) Tiny(_ context.Context, v app.Item) (bool, error)  { return v.Price&4 != 0, nil }

// failingSizes は、述語 Big が error を返す SizeCases。呼ばれた述語と、届いた ctx を覚える。
type failingSizes struct {
	err    error
	called []string
	seen   []context.Context
}

func (c *failingSizes) Big(ctx context.Context, _ app.Item) (bool, error) {
	c.called, c.seen = append(c.called, "Big"), append(c.seen, ctx)
	return false, c.err
}
func (c *failingSizes) Small(ctx context.Context, _ app.Item) (bool, error) {
	c.called, c.seen = append(c.called, "Small"), append(c.seen, ctx)
	return true, nil
}
func (c *failingSizes) Tiny(ctx context.Context, _ app.Item) (bool, error) {
	c.called, c.seen = append(c.called, "Tiny"), append(c.seen, ctx)
	return false, nil
}

// addresses は AddressCases の実装。二つに分かれる分類も、どちらのケースも述語を持つ。空の文字列は、どちらでもない
// （宣言したケースの外の値）
type addresses struct{}

func (addresses) Valid(_ context.Context, v string) (bool, error) { return v == "a@example.com", nil }
func (addresses) Invalid(_ context.Context, v string) (bool, error) {
	return v != "" && v != "a@example.com", nil
}

// contacts は ContactCases の実装。呼ばれた述語を覚える。
type contacts struct{ called []string }

func (c *contacts) Mail(_ context.Context, v *string) (bool, error) {
	c.called = append(c.called, "Mail")
	return v != nil && *v == "a@example.com", nil
}

func (c *contacts) Text(_ context.Context, v *string) (bool, error) {
	c.called = append(c.called, "Text")
	return *v == "plain", nil // v が nil なら、ここで落ちる（判定関数は、nil のときにこの述語を呼ばない）
}

func (c *contacts) Silent(_ context.Context, v *string) (bool, error) {
	c.called = append(c.called, "Silent")
	return v == nil, nil
}

// stocks は StockCases の実装。InStock と SoldOut は、値がないときには呼ばれない（型で決まる絞り込みの as を持つ）
type stocks struct{}

func (stocks) InStock(_ context.Context, v *app.Item) (bool, error) { return v.Price > 0, nil }
func (stocks) SoldOut(_ context.Context, v *app.Item) (bool, error) { return v.Price == 0, nil }
func (stocks) Unknown(_ context.Context, v *app.Item) (bool, error) { return v == nil, nil }

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

// 仕様の上で純粋な葉（デコレータも throws もない）も、ctx を取り、error を返す
func (*leaves) TitleOf(_ context.Context, ready app.Title) (string, error) { return ready.Title, nil }
func (*leaves) CodeText(_ context.Context, code int64) (string, error)     { return "code", nil }
func (*leaves) ReceiptText(context.Context, app.Receipt) (string, error)   { return "receipt", nil }
func (*leaves) Send(_ context.Context, addr app.Email) (string, error) {
	return "sent " + string(addr), nil
}
func (*leaves) PriceOf(_ context.Context, item app.Item) (int64, error) { return item.Price, nil }
func (*leaves) BigOnly(_ context.Context, item app.Item) (string, error) {
	return "big " + item.Name, nil
}
func (l *leaves) Show(_ context.Context, text string) error {
	l.shown = append(l.shown, text)
	return nil
}

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

// 判定関数: 真がちょうど一つならそのケース。0 でも 2 以上でも ClassificationError
func TestClassifyExactlyOne(t *testing.T) {
	ctx := context.Background()
	for price, want := range map[int64]app.Size{1: app.SizeBig, 2: app.SizeSmall, 4: app.SizeTiny} {
		got, err := app.ClassifySize(ctx, sizes{}, app.Item{Price: price})
		if err != nil || got != want {
			t.Errorf("ClassifySize(%d) = %v, %v", price, got, err)
		}
	}
	_, err := app.ClassifySize(ctx, sizes{}, app.Item{Price: 0})
	classificationError(t, err, "app.Size", nil)
	_, err = app.ClassifySize(ctx, sizes{}, app.Item{Price: 5})
	classificationError(t, err, "app.Size", []string{"Big", "Tiny"})
}

// 値の分類の判定関数: 述語が error を返したら、残りも呼んだうえで、最初の error を返す（真の数は確かめない）。
// 述語には、判定関数が受け取った ctx が、そのまま届く
func TestClassifyPredicateError(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "request")
	boom := errors.New("boom")
	cs := &failingSizes{err: boom}
	if _, err := app.ClassifySize(ctx, cs, app.Item{}); err != boom {
		t.Errorf("err = %v, want boom", err)
	}
	if !reflect.DeepEqual(cs.called, []string{"Big", "Small", "Tiny"}) {
		t.Errorf("呼ばれた述語 = %v（error の後の述語も呼ぶ）", cs.called)
	}
	for i, seen := range cs.seen {
		if seen.Value(key{}) != "request" {
			t.Errorf("述語 %s に届いた ctx が、判定関数に渡したものでない", cs.called[i])
		}
	}
}

// 二つに分かれる分類の判定関数: どちらのケースも、その述語が真のとき。基底は「ほかのどれでもない」ケースを導かないので、
// どの述語も真でない値は、ClassificationError（宣言したケースの外の値。docs/plugin-go.md「分類」）
func TestClassifyTwoSided(t *testing.T) {
	if got, err := app.ClassifyAddress(context.Background(), addresses{}, "a@example.com"); err != nil || got != app.AddressValid {
		t.Errorf("Valid: %v, %v", got, err)
	}
	if got, err := app.ClassifyAddress(context.Background(), addresses{}, "x"); err != nil || got != app.AddressInvalid {
		t.Errorf("Invalid: %v, %v", got, err)
	}
	_, err := app.ClassifyAddress(context.Background(), addresses{}, "")
	classificationError(t, err, "app.Address", nil)
}

// 型で決まる絞り込み（presence）の as を持つケースは、値がその型に当たらなければ、述語を呼ばずに偽とする
func TestClassifyPresenceGuard(t *testing.T) {
	cs := &contacts{}
	got, err := app.ClassifyContact(context.Background(), cs, nil)
	if err != nil || got != app.ContactSilent {
		t.Errorf("nil: %v, %v", got, err)
	}
	if !reflect.DeepEqual(cs.called, []string{"Mail", "Silent"}) {
		t.Errorf("呼ばれた述語 = %v（Text は、値がないので呼ばない）", cs.called)
	}
	plain := "plain"
	if got, err := app.ClassifyContact(context.Background(), &contacts{}, &plain); err != nil || got != app.ContactText {
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
	if got, err := o.Mail(ctx, "a@example.com", &mail); err != nil || got != "sent a@example.com" {
		t.Errorf("Mail(mail) = %q, %v", got, err)
	}
	if got, err := o.Mail(ctx, "x", &plain); err != nil || got != "plain" {
		t.Errorf("Mail(plain) = %q, %v", got, err)
	}
	if _, err := o.Mail(ctx, "x", nil); !errors.Is(err, app.Bad{}) {
		t.Errorf("Mail(nil) の err = %v, want app.Bad", err)
	}
	if !reflect.DeepEqual(l.shown, []string{"sent a@example.com", "invalid", "invalid"}) {
		t.Errorf("shown = %v", l.shown)
	}
	// const の match で、どの述語も真でない値: 通り抜けずに、判定関数の ClassificationError が返る
	_, err = o.Mail(ctx, "", &plain)
	classificationError(t, err, "app.Address", nil)

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
	if got, err := o.Describe(ctx, app.PageCode{Value: 3}); err != nil || got != "code" {
		t.Errorf("Describe(Code) = %q, %v", got, err)
	}
	if got, err := o.Describe(ctx, app.Problem{Reason: "r"}); err != nil || got != "r" {
		t.Errorf("Describe(Failed) = %q, %v", got, err)
	}
	if got, err := o.Describe(ctx, app.PageNote{}); err != nil || got != "other" {
		t.Errorf("Describe(Note) = %q, %v", got, err)
	}
	if got, err := o.StageText(ctx); err != nil || got != "top" {
		t.Errorf("StageText = %q, %v", got, err)
	}
	// else のアームは、受けるケースを名前で並べたアームと同じに動く（describeElse の else は Rest を受ける）
	for _, page := range []app.Page{app.PageCode{Value: 3}, app.Title{Title: "t"}, app.Problem{Reason: "r"}, app.PageNote{}, app.PageBlank{}} {
		want, wantErr := o.Describe(ctx, page)
		got, err := o.DescribeElse(ctx, page)
		if got != want || err != wantErr {
			t.Errorf("DescribeElse(%T) = %q, %v。Describe は %q, %v", page, got, err, want, wantErr)
		}
	}
	// 述語で決まる分類の else のアーム（pureLabel の else は Big と Small を受ける）
	if got, err := o.PureLabel(ctx, app.Item{Price: 1}); err != nil || got != "normal" {
		t.Errorf("PureLabel(Big) = %q, %v", got, err)
	}
	if got, err := o.PureLabel(ctx, app.Item{Price: 2}); err != nil || got != "normal" {
		t.Errorf("PureLabel(Small) = %q, %v", got, err)
	}
	// 型の switch の else のアーム（variants の三つの match）
	l.shown = nil
	if got, err := o.Variants(ctx, app.Title{Title: "t"}); err != nil || got != "done" || !reflect.DeepEqual(l.shown, []string{"ready", "not code"}) {
		t.Errorf("Variants(Ready) = %q, %v, shown = %v", got, err, l.shown)
	}
	l.shown = nil
	if got, err := o.Variants(ctx, app.PageCode{Value: 1}); err != nil || got != "done" || len(l.shown) != 0 {
		t.Errorf("Variants(Code) = %q, %v, shown = %v", got, err, l.shown)
	}
	l.shown = nil
	if got, err := o.Variants(ctx, app.PageNote{}); err != nil || got != "done" || !reflect.DeepEqual(l.shown, []string{"not code"}) {
		t.Errorf("Variants(Note) = %q, %v, shown = %v", got, err, l.shown)
	}

	// 状態の分類: 文の match
	l.shown = nil
	if err := o.CheckoutNote(ctx); err != nil || !reflect.DeepEqual(l.shown, []string{"shopping"}) {
		t.Errorf("CheckoutNote: %v, shown = %v", err, l.shown)
	}
}

// 純粋で throws のない配線（pureLabel、outcome）も、error を返す形を持つ（docs/plugin-go.md「op の形」）。どのケースにも
// 当たらない判定の誤りは、panic ではなく、その error として返る（同「分類」）
func TestPureWiringReturnsClassificationError(t *testing.T) {
	ctx := context.Background()
	o := newOps(&leaves{}, stage{}, &checkout{}, &contacts{})
	if got, err := o.PureLabel(ctx, app.Item{Price: 4}); err != nil || got != "tiny" {
		t.Errorf("PureLabel = %q, %v", got, err)
	}
	// 判定関数の誤り: どの述語も真でない、二つが真
	for price, hits := range map[int64][]string{0: nil, 3: {"Big", "Small"}} {
		got, err := returned(t, func() (string, error) { return o.PureLabel(ctx, app.Item{Price: price}) })
		if got != "" {
			t.Errorf("PureLabel(%d) = %q（期待は零値）", price, got)
		}
		classificationError(t, err, "app.Size", hits)
	}
	// 判定関数を持たない分類（variant）の switch の default: の節: どのアームの型でもない値（nil）
	got, err := returned(t, func() (string, error) { return o.Outcome(ctx, nil) })
	if got != "" {
		t.Errorf("Outcome(nil) = %q（期待は零値）", got)
	}
	classificationError(t, err, "app.Outcome", nil)
	if got, err := o.Outcome(ctx, app.Problem{Reason: "r"}); err != nil || got != "r" {
		t.Errorf("Outcome(Err) = %q, %v", got, err)
	}
}

// 型で決まる分類は判定関数を持たないので、宣言したケースの外の値は、match のコードが分類の誤りにする（docs/plugin-go.md
// 「match」の「分類の誤りの枝」）: 型の switch の default: の節。return / const / 文のどの match でも、通り抜けない
func TestTypeSwitchClassificationError(t *testing.T) {
	ctx := context.Background()
	l := &leaves{}
	o := newOps(l, stage{page: app.PageBlank{}}, &checkout{}, &contacts{})
	// 残りのケースのない分類 Shown: Title と int のアームだけがケース
	if got, err := o.Shown(ctx, app.Title{Title: "t"}); err != nil || got != "t" {
		t.Errorf("Shown(Ready) = %q, %v", got, err)
	}
	if got, err := o.Shown(ctx, app.PageCode{Value: 1}); err != nil || got != "code" {
		t.Errorf("Shown(Code) = %q, %v", got, err)
	}
	for _, page := range []app.Page{app.PageBlank{}, app.Problem{}, app.PageNote{}, nil} {
		got, err := returned(t, func() (string, error) { return o.Shown(ctx, page) })
		if got != "" {
			t.Errorf("Shown(%T) = %q（期待は零値）", page, got)
		}
		classificationError(t, err, "app.Shown", nil)
		// const の match でも、文の match でも、同じ
		l.shown = nil
		got, err = returned(t, func() (string, error) { return o.ShownNote(ctx, page) })
		if got != "" || len(l.shown) != 0 {
			t.Errorf("ShownNote(%T) = %q, shown = %v（期待は、零値と、何も呼ばないこと）", page, got, l.shown)
		}
		classificationError(t, err, "app.Shown", nil)
	}
	l.shown = nil
	if got, err := o.ShownNote(ctx, app.Title{Title: "t"}); err != nil || got != "noted" || !reflect.DeepEqual(l.shown, []string{"title"}) {
		t.Errorf("ShownNote(Ready) = %q, %v, shown = %v", got, err, l.shown)
	}
	// 残りのケースを持つ分類 PageKind: 残りのアーム（Blank、Note）は Rest。union の値がない（nil）のは、Rest でもなく、分類の誤り。
	// else のアームも、nil を受けない
	if got, err := o.Describe(ctx, app.PageBlank{}); err != nil || got != "other" {
		t.Errorf("Describe(Blank) = %q, %v", got, err)
	}
	for name, call := range map[string]func() (string, error){
		"Describe":     func() (string, error) { return o.Describe(ctx, nil) },
		"DescribeElse": func() (string, error) { return o.DescribeElse(ctx, nil) },
		"Variants":     func() (string, error) { return o.Variants(ctx, nil) },
	} {
		got, err := returned(t, call)
		if got != "" {
			t.Errorf("%s(nil) = %q（期待は零値）", name, got)
		}
		classificationError(t, err, "app.PageKind", nil)
	}
	// 型で決まる状態の分類: 残りのケース Hidden が残りのアームを受け、値がない（nil）のは分類の誤り
	if got, err := o.StageText(ctx); err != nil || got != "none" {
		t.Errorf("StageText(Blank) = %q, %v", got, err)
	}
	empty := newOps(l, stage{}, &checkout{}, &contacts{})
	got, err := returned(t, func() (string, error) { return empty.StageText(ctx) })
	if got != "" {
		t.Errorf("StageText(nil) = %q（期待は零値）", got)
	}
	classificationError(t, err, "app.Stage", nil)
}

// 述語が返した error も、純粋な配線から、包まれずに返る
func TestPureWiringReturnsPredicateError(t *testing.T) {
	boom := errors.New("boom")
	o := app.New(&leaves{}, stage{}, nil, nil, &checkout{}, addresses{}, &contacts{}, &failingSizes{err: boom}, stocks{})
	if _, err := returned(t, func() (string, error) { return o.PureLabel(context.Background(), app.Item{}) }); err != boom {
		t.Errorf("err = %v, want boom", err)
	}
}

// returned は call の戻り値。call が panic したら、テストを落とす（生成したコードは、判定の誤りで panic しない）。
func returned(t *testing.T, call func() (string, error)) (got string, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic した: %v（期待は、error として返ること）", r)
		}
	}()
	return call()
}
