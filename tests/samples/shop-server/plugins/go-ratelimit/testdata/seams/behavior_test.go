// 生成したコードの振る舞いのテスト: go-ratelimit が出した継ぎ目の中身（../../../../README.md「サンプルの中のプラグイン」の
// 「出力の規則」）。go-ratelimit の TestBehavior が、基底の出力と go-ratelimit の出力を置いたモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"example.com/app/gen/mixed"
	"example.com/app/gen/rate"
	"example.com/app/gen/shop"
	"example.com/app/gen/shop/inner"
	pkgtype "example.com/app/gen/type"
)

// limiter は、人が書く数え方の代わり: allow を返し、聞かれた op を覚える。
type limiter struct {
	allow bool
	asked []string
}

func (l *limiter) Allow(_ context.Context, op string) bool {
	l.asked = append(l.asked, op)
	return l.allow
}

// leaves は shop の葉の実装。呼ばれたものを log に残す。葉のメソッドは、すべてここに書く（インターフェースを埋め込まない:
// 一つでも足りなければ、New に渡すところでコンパイルできない）。
type leaves struct {
	log *[]string
}

func (l leaves) Ping(context.Context) error { *l.log = append(*l.log, "ping"); return nil }
func (l leaves) Audit(_ context.Context, note string) error {
	*l.log = append(*l.log, "audit "+note)
	return nil
}
func (l leaves) Find(_ context.Context, id pkgtype.ItemId) (pkgtype.Item, error) {
	*l.log = append(*l.log, "find "+string(id))
	return pkgtype.Item{Id: id, Name: "apple"}, nil
}
func (l leaves) Names(context.Context) ([]string, error)         { return []string{"a"}, nil }
func (l leaves) Title(context.Context) (string, error)           { return "title", nil }
func (l leaves) Total(context.Context) (int64, error)            { return 7, nil }
func (l leaves) Open(context.Context) (bool, error)              { return true, nil }
func (l leaves) Since(context.Context) (time.Time, error)        { return time.Unix(1, 0), nil }
func (l leaves) FirstId(context.Context) (pkgtype.ItemId, error) { return "first", nil }
func (l leaves) Clash(_ context.Context, s, ctx, body, context_ string, id pkgtype.ItemId) (string, error) {
	return s + ctx + body + context_ + string(id), nil
}

// items は状態 shop.items の単位の実装: 形で決まる問い合わせ（IsEmpty、Get）と、read shop.itemById と、効果 shop.addItem
// （docs/plugin-go.md「状態の単位」）。単位のメソッドは、すべてここに書く（インターフェースを埋め込まない: 一つでも足りないか、
// 名前が違えば、New に渡すところでコンパイルできない）。
type items struct {
	log *[]string
}

func (s items) IsEmpty(context.Context) (bool, error)       { return true, nil }
func (s items) Get(context.Context) ([]pkgtype.Item, error) { return nil, nil }
func (s items) ItemById(_ context.Context, id pkgtype.ItemId) (*pkgtype.Item, error) {
	*s.log = append(*s.log, "itemById "+string(id))
	return &pkgtype.Item{Id: id}, nil
}
func (s items) AddItem(_ context.Context, item pkgtype.Item) error {
	*s.log = append(*s.log, "addItem "+string(item.Id))
	return nil
}

// newShop は、RateSeams をそのまま基底の New に渡した shop（RateSeams だけで Seams を満たす。満たさなければ、ここで
// コンパイルできない）。内側の Ops を返す（内部の op audit も呼べる）。
func newShop(allow bool) (inner.Ops, *limiter, *[]string) {
	lim, log := &limiter{allow: allow}, &[]string{}
	return inner.New(leaves{log: log}, items{log: log}, shop.NewRateSeams(lim)), lim, log
}

// TestAllowed は、limiter が許したとき: 中身を、受け取った引数のまま呼び、戻り値をそのまま返す。聞くのは、op の FQN で一度。
func TestAllowed(t *testing.T) {
	s, lim, log := newShop(true)
	ctx := context.Background()
	got, err := s.Buy(ctx, "i1", 3)
	if err != nil || got != (shop.Receipt{Item: pkgtype.Item{Id: "i1", Name: "apple"}, Count: 3}) {
		t.Errorf("Buy = %+v, %v", got, err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Errorf("Ping = %v", err)
	}
	if err := s.Add(ctx, pkgtype.Item{Id: "i2"}); err != nil {
		t.Errorf("Add = %v", err)
	}
	if item, err := s.Lookup(ctx, "i3"); err != nil || item == nil || item.Id != "i3" {
		t.Errorf("Lookup = %+v, %v", item, err)
	}
	if err := s.Audit(ctx, "n"); err != nil {
		t.Errorf("Audit = %v", err)
	}
	if got, err := s.Clash(ctx, "a", "b", "c", "d", "e"); err != nil || got != "abcde" {
		t.Errorf("Clash = %q, %v（期待は abcde。引数を、順のまま中身に渡す）", got, err)
	}
	if want := []string{"shop.buy", "shop.ping", "shop.add", "shop.lookup", "shop.audit", "shop.clash"}; !reflect.DeepEqual(lim.asked, want) {
		t.Errorf("limiter が聞かれた op が %v（期待は %v）", lim.asked, want)
	}
	if want := []string{"find i1", "ping", "addItem i2", "itemById i3", "audit n"}; !reflect.DeepEqual(*log, want) {
		t.Errorf("呼ばれた中身が %v（期待は %v）", *log, want)
	}
}

// TestDenied は、limiter が許さなかったとき: 中身を呼ばずに、戻り値の型の零値と rate.LimitExceeded（値）を返す。
func TestDenied(t *testing.T) {
	s, lim, log := newShop(false)
	ctx := context.Background()
	limited := func(name string, err error) {
		t.Helper()
		var e rate.LimitExceeded
		if !errors.As(err, &e) {
			t.Errorf("%s のエラーが %v（期待は rate.LimitExceeded）", name, err)
		}
	}
	receipt, err := s.Buy(ctx, "i1", 3)
	limited("Buy", err)
	if receipt != (shop.Receipt{}) {
		t.Errorf("Buy の戻り値が %+v（期待は零値）", receipt)
	}
	limited("Ping", s.Ping(ctx))
	limited("Add", s.Add(ctx, pkgtype.Item{Id: "i2"}))
	item, err := s.Lookup(ctx, "i3")
	limited("Lookup", err)
	names, err := s.Names(ctx)
	limited("Names", err)
	title, err := s.Title(ctx)
	limited("Title", err)
	total, err := s.Total(ctx)
	limited("Total", err)
	open, err := s.Open(ctx)
	limited("Open", err)
	since, err := s.Since(ctx)
	limited("Since", err)
	first, err := s.FirstId(ctx)
	limited("FirstId", err)
	limited("Audit", s.Audit(ctx, "n"))
	if item != nil || names != nil || title != "" || total != 0 || open || !since.IsZero() || first != "" {
		t.Errorf("零値でない戻り値がある: %v %v %q %d %v %v %q", item, names, title, total, open, since, first)
	}
	if len(*log) != 0 {
		t.Errorf("許されなかったのに、中身が呼ばれた: %v", *log)
	}
	if len(lim.asked) != 11 {
		t.Errorf("limiter が聞かれた回数が %d（期待は 11）", len(lim.asked))
	}
}

// 以下は、継ぎ目を宣言したトランスパイラが二つある単位（mixed）: RateSeams だけでは Seams を満たさないので、人が、もう一方の
// 実装と一つの構造体に埋め込む（docs/plugin-go.md「継ぎ目の実装」）。

// traceSeams は、もう一方のトランスパイラ（go-trace）の継ぎ目の、人が書いた代わり。
type traceSeams struct{ log *[]string }

func (s traceSeams) BothTTraced(ctx context.Context, n int64, body func(ctx context.Context, n int64) (int64, error)) (int64, error) {
	*s.log = append(*s.log, "trace both")
	return body(ctx, n)
}
func (s traceSeams) TracedTTraced(ctx context.Context, n int64, body func(ctx context.Context, n int64) (int64, error)) (int64, error) {
	*s.log = append(*s.log, "trace traced")
	return body(ctx, n)
}

type mixedSeams struct {
	mixed.RateSeams
	traceSeams
}

var _ mixed.Seams = mixedSeams{}

type mixedLeaves struct{}

func (mixedLeaves) Both(_ context.Context, n int64) (int64, error)   { return n + 1, nil }
func (mixedLeaves) Traced(_ context.Context, n int64) (int64, error) { return n + 2, nil }

// TestMixed は、RateSeams だけでは mixed.Seams を満たさないことと、埋め込んだ構造体なら、二つの継ぎ目を通ること。
func TestMixed(t *testing.T) {
	if reflect.TypeOf(mixed.RateSeams{}).Implements(reflect.TypeOf((*mixed.Seams)(nil)).Elem()) {
		t.Error("RateSeams だけで mixed.Seams を満たす（ほかのトランスパイラの継ぎ目のメソッドを持っている）")
	}
	lim, log := &limiter{allow: true}, &[]string{}
	ops := mixed.New(mixedLeaves{}, mixedSeams{mixed.NewRateSeams(lim), traceSeams{log}})
	ctx := context.Background()
	if got, err := ops.Both(ctx, 1); err != nil || got != 2 {
		t.Errorf("Both = %d, %v", got, err)
	}
	if got, err := ops.Traced(ctx, 1); err != nil || got != 3 {
		t.Errorf("Traced = %d, %v", got, err)
	}
	if !reflect.DeepEqual(lim.asked, []string{"mixed.both"}) || !reflect.DeepEqual(*log, []string{"trace both", "trace traced"}) {
		t.Errorf("limiter = %v、trace = %v", lim.asked, *log)
	}
	// 制限を超えれば、外側の継ぎ目（@t.traced）は通るが、中身は呼ばれない
	lim.allow = false
	var e rate.LimitExceeded
	if got, err := ops.Both(ctx, 1); !errors.As(err, &e) || got != 0 {
		t.Errorf("Both = %d, %v（期待は 0, rate.LimitExceeded）", got, err)
	}
}
