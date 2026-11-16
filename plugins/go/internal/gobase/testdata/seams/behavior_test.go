// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 継ぎ目を通ることと、入れ子の順
// （docs/plugin-go.md「継ぎ目」）、内側のフォルダから内部の op を呼べること、入口の Ops が export の op だけを持つこと
// （同「内側のフォルダと入口」）。plugins/go の TestBehavior が、生成したコードのモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/front"
	plain "example.com/app/gen/plain" // パッケージ名は名前空間から決まる（shop）ので、名前を付ける
	"example.com/app/gen/shop"
	"example.com/app/gen/shop/inner"
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

// leaves は、人が書く葉。デコレータが reads / effects を運ぶ葉（Add、Count、Summary）も、ここ。
type leaves struct{ l *log }

func (x leaves) Add(context.Context, shop.Item) error    { x.l.note("leaf Add"); return nil }
func (x leaves) Count(context.Context) (int64, error)    { x.l.note("leaf Count"); return 3, nil }
func (x leaves) Summary(context.Context) (string, error) { x.l.note("leaf Summary"); return "s", nil }
func (x leaves) Notify(context.Context, shop.Item) error { x.l.note("leaf Notify"); return nil }
func (x leaves) Lookup(_ context.Context, id string) (shop.Item, error) {
	x.l.note("leaf Lookup")
	return shop.Item{Id: id}, nil
}
func (x leaves) Audit(context.Context, shop.Item) error { x.l.note("leaf Audit"); return nil }
func (x leaves) Post(_ context.Context, body string, body_ string) (string, error) {
	x.l.note("leaf Post")
	return body + body_, nil
}
func (x leaves) Plain(_ context.Context, id string) (shop.Item, error) {
	x.l.note("leaf Plain")
	return shop.Item{Id: id}, nil
}

// items は、状態 shop.items の単位: 形で決まる問い合わせ、read countById、効果 addItem。
type items struct{ l *log }

func (x items) IsEmpty(context.Context) (bool, error)    { return true, nil }
func (x items) Get(context.Context) ([]shop.Item, error) { return nil, nil }
func (x items) CountById(_ context.Context, id string) (int64, error) {
	x.l.note("state CountById " + id)
	return 2, nil
}
func (x items) AddItem(_ context.Context, item shop.Item) error {
	x.l.note("state AddItem " + item.Id)
	return nil
}

// title は、状態 shop.title の単位: 形で決まる問い合わせと、配線が直に呼ぶ、形で決まる命令 Put。
type title struct{ l *log }

func (title) Get(context.Context) (string, error) { return "shop", nil }
func (x title) Put(_ context.Context, value string) error {
	if x.l != nil {
		x.l.note("state title.Put " + value)
	}
	return nil
}

// seams は、継ぎ目の実装: 前後を log に残して、body をそのまま呼ぶ。deny に名前があれば、body を呼ばずにエラーを返す。
type seams struct {
	l    *log
	deny string
}

var errDenied = errors.New("denied")

func around[T any](s seams, name string, body func() (T, error)) (T, error) {
	if s.deny == name {
		s.l.note(name + " denied")
		var zero T
		return zero, errDenied
	}
	s.l.note(name + " before")
	v, err := body()
	s.l.note(name + " after")
	return v, err
}

func around0(s seams, name string, body func() error) error {
	_, err := around(s, name, func() (struct{}, error) { return struct{}{}, body() })
	return err
}

func (s seams) BuyTLimited(ctx context.Context, id string, body func(ctx context.Context, id string) (shop.Item, error)) (shop.Item, error) {
	return around(s, "BuyTLimited", func() (shop.Item, error) { return body(ctx, id) })
}
func (s seams) RestockTLimited(ctx context.Context, item shop.Item, body func(ctx context.Context, item shop.Item) (int64, error)) (int64, error) {
	return around(s, "RestockTLimited", func() (int64, error) { return body(ctx, item) })
}
func (s seams) ClosedTLimited(ctx context.Context, body func(ctx context.Context) error) error {
	return around0(s, "ClosedTLimited", func() error { return body(ctx) })
}
func (s seams) AddTLimited(ctx context.Context, item shop.Item, body func(ctx context.Context, item shop.Item) error) error {
	return around0(s, "AddTLimited", func() error { return body(ctx, item) })
}
func (s seams) CountTLimited(ctx context.Context, body func(ctx context.Context) (int64, error)) (int64, error) {
	return around(s, "CountTLimited", func() (int64, error) { return body(ctx) })
}
func (s seams) SummaryTLimited(ctx context.Context, body func(ctx context.Context) (string, error)) (string, error) {
	return around(s, "SummaryTLimited", func() (string, error) { return body(ctx) })
}
func (s seams) NotifyTLimited(ctx context.Context, item shop.Item, body func(ctx context.Context, item shop.Item) error) error {
	return around0(s, "NotifyTLimited", func() error { return body(ctx, item) })
}
func (s seams) FindTTraced(ctx context.Context, id string, body func(ctx context.Context, id string) (shop.Item, error)) (shop.Item, error) {
	return around(s, "FindTTraced", func() (shop.Item, error) { return body(ctx, id) })
}
func (s seams) FindTLimited(ctx context.Context, id string, body func(ctx context.Context, id string) (shop.Item, error)) (shop.Item, error) {
	return around(s, "FindTLimited", func() (shop.Item, error) { return body(ctx, id) })
}
func (s seams) AuditTTraced(ctx context.Context, item shop.Item, body func(ctx context.Context, item shop.Item) error) error {
	return around0(s, "AuditTTraced", func() error { return body(ctx, item) })
}
func (s seams) PostTLimited(ctx context.Context, body string, body_ string, body__ func(ctx context.Context, body string, body_ string) (string, error)) (string, error) {
	return around(s, "PostTLimited", func() (string, error) { return body__(ctx, body, body_) })
}

// 継ぎ目を持つ op は、継ぎ目を通して中身を呼ぶ。中身は、配線の本体・never を返す op・葉の実装（Leaves のメソッド）
func TestSeamsPassThrough(t *testing.T) {
	ctx := context.Background()
	l := &log{}
	o := shop.New(leaves{l}, items{l}, title{l}, seams{l: l})

	if err := o.Closed(ctx); !errors.Is(err, shop.Closed{}) {
		t.Errorf("Closed の err = %v", err)
	}
	l.expect(t, "ClosedTLimited before", "ClosedTLimited after")
	if err := o.Add(ctx, shop.Item{}); err != nil {
		t.Error(err)
	}
	l.expect(t, "AddTLimited before", "leaf Add", "AddTLimited after")
	if n, err := o.Count(ctx); err != nil || n != 3 {
		t.Errorf("Count = %d, %v", n, err)
	}
	l.expect(t, "CountTLimited before", "leaf Count", "CountTLimited after")
	if _, err := o.Summary(ctx); err != nil {
		t.Error(err)
	}
	l.expect(t, "SummaryTLimited before", "leaf Summary", "SummaryTLimited after")
	if err := o.Notify(ctx, shop.Item{}); err != nil {
		t.Error(err)
	}
	l.expect(t, "NotifyTLimited before", "leaf Notify", "NotifyTLimited after")
	if got, err := o.Post(ctx, "a", "b"); err != nil || got != "ab" {
		t.Errorf("Post = %q, %v", got, err)
	}
	l.expect(t, "PostTLimited before", "leaf Post", "PostTLimited after")
	// 継ぎ目が掛かるのは op だけ: 継ぎ目を持つ配線の中身が呼ぶ read と効果と、直に呼ぶ組み込みの効果は、状態の単位のメソッドを、
	// そのまま呼ぶ（継ぎ目は、op の外側の一度だけ）
	if n, err := o.Restock(ctx, shop.Item{Id: "i1"}); err != nil || n != 2 {
		t.Errorf("Restock = %d, %v", n, err)
	}
	l.expect(t, "RestockTLimited before", "state AddItem i1", "state title.Put restocked", "state CountById i1", "RestockTLimited after")
	// 継ぎ目にないデコレータだけの op は、継ぎ目を通らない
	if _, err := o.Plain(ctx, "p"); err != nil {
		t.Error(err)
	}
	l.expect(t, "leaf Plain")
}

// 二つの継ぎ目は入れ子: 先に書かれたもの（@t.traced）が外側、op に近いもの（@t.limited）が内側で、中身を直に受け取る。
// 継ぎ目を持つ op を同じ名前空間の配線から呼んでも、外側を通る
func TestSeamsNesting(t *testing.T) {
	ctx := context.Background()
	l := &log{}
	o := shop.New(leaves{l}, items{l}, title{l}, seams{l: l})
	if got, err := o.Find(ctx, "x"); err != nil || got.Id != "x" {
		t.Errorf("Find = %v, %v", got, err)
	}
	l.expect(t, "FindTTraced before", "FindTLimited before", "leaf Lookup", "FindTLimited after", "FindTTraced after")

	if _, err := o.Buy(ctx, "x"); err != nil {
		t.Error(err)
	}
	l.expect(t,
		"BuyTLimited before",
		"FindTTraced before", "FindTLimited before", "leaf Lookup", "FindTLimited after", "FindTTraced after",
		"AddTLimited before", "leaf Add", "AddTLimited after",
		"BuyTLimited after")

	// 継ぎ目が body を呼ばなければ、中身は走らない
	denied := shop.New(leaves{l}, items{l}, title{l}, seams{l: l, deny: "FindTLimited"})
	if _, err := denied.Buy(ctx, "x"); err != errDenied {
		t.Errorf("Buy の err = %v, want denied", err)
	}
	l.expect(t, "BuyTLimited before", "FindTTraced before", "FindTLimited denied", "FindTTraced after", "BuyTLimited after")
}

// 内側の New の戻り値からは、内部の op も呼べる（継ぎ目も通る）。入口の Ops は export の op だけを持つ
func TestInnerAndEntry(t *testing.T) {
	ctx := context.Background()
	l := &log{}
	in := inner.New(leaves{l}, items{l}, title{l}, seams{l: l})
	if err := in.Audit(ctx, shop.Item{}); err != nil {
		t.Error(err)
	}
	l.expect(t, "AuditTTraced before", "leaf Audit", "AuditTTraced after")

	entry := reflect.TypeOf((*shop.Ops)(nil)).Elem()
	all := reflect.TypeOf((*inner.Ops)(nil)).Elem()
	if _, ok := entry.MethodByName("Audit"); ok {
		t.Error("入口の Ops が、内部の op Audit を持つ")
	}
	if _, ok := all.MethodByName("Audit"); !ok {
		t.Error("内側の Ops が、内部の op Audit を持たない")
	}
	if entry.NumMethod() != all.NumMethod()-1 {
		t.Errorf("入口の Ops のメソッドは %d、内側は %d（内部の op は Audit の一つ）", entry.NumMethod(), all.NumMethod())
	}
	// 入口の型は内側の型の別名
	var _ inner.Item = shop.Item{}
	var _ inner.Seams = shop.Seams(seams{})
}

// 継ぎ目を宣言したトランスパイラを並べていない単位の出力は、同じ名前空間でも継ぎ目を持たない（New に seams がない）。
// 別の名前空間の op を呼ぶ側は、相手の Ops を渡せば、相手の継ぎ目を通る
func TestOtherOutputs(t *testing.T) {
	ctx := context.Background()
	l := &log{}
	p := plain.New(plainLeaves{l}, plainItems{l}, title{})
	if _, err := p.Find(ctx, "x"); err != nil {
		t.Error(err)
	}
	l.expect(t, "plain Lookup")

	s := shop.New(leaves{l}, items{l}, title{l}, seams{l: l, deny: "BuyTLimited"})
	f := front.New(s)
	if _, err := f.Order(ctx, "x"); err != errDenied {
		t.Errorf("Order の err = %v, want denied", err)
	}
	l.expect(t, "BuyTLimited denied")
}

type plainLeaves struct{ l *log }

func (x plainLeaves) Add(context.Context, plain.Item) error    { return nil }
func (x plainLeaves) Count(context.Context) (int64, error)     { return 0, nil }
func (x plainLeaves) Summary(context.Context) (string, error)  { return "", nil }
func (x plainLeaves) Notify(context.Context, plain.Item) error { return nil }
func (x plainLeaves) Lookup(_ context.Context, id string) (plain.Item, error) {
	x.l.note("plain Lookup")
	return plain.Item{Id: id}, nil
}
func (x plainLeaves) Audit(context.Context, plain.Item) error { return nil }
func (x plainLeaves) Post(context.Context, string, string) (string, error) {
	return "", nil
}
func (x plainLeaves) Plain(context.Context, string) (plain.Item, error) { return plain.Item{}, nil }

type plainItems struct{ l *log }

func (x plainItems) IsEmpty(context.Context) (bool, error)            { return true, nil }
func (x plainItems) Get(context.Context) ([]plain.Item, error)        { return nil, nil }
func (x plainItems) CountById(context.Context, string) (int64, error) { return 0, nil }
func (x plainItems) AddItem(context.Context, plain.Item) error        { return nil }
