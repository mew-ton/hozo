package shop

// このファイルは生成物ではない。internal/emit/golang の TestStateBehavior が、生成した shop のパッケージ
// （testdata/state/expected/gen/shop と同じ内容）の隣に置いて `go test` する。人が書く側（葉、States、StateCases、
// <C>Cases）を偽物で埋め、docs/emit-go.md「classify」「match」の振る舞いを確かめる。

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mew-ton/hozo/runtime"
)

// fakeLeaves は呼ばれた op の名前を記録する。
type fakeLeaves struct {
	calls []string
	me    MeInput
}

func (f *fakeLeaves) SubmitOrder(context.Context) (Order, error) {
	f.calls = append(f.calls, "submitOrder")
	return Order{Id: "o1"}, nil
}
func (f *fakeLeaves) EmptyCart(context.Context) error {
	f.calls = append(f.calls, "emptyCart")
	return nil
}
func (f *fakeLeaves) ShowLogin(context.Context) error {
	f.calls = append(f.calls, "showLogin")
	return nil
}
func (f *fakeLeaves) ShowCart(context.Context) error {
	f.calls = append(f.calls, "showCart")
	return nil
}
func (f *fakeLeaves) ShowEmpty(context.Context) error {
	f.calls = append(f.calls, "showEmpty")
	return nil
}
func (f *fakeLeaves) ShowOrders(context.Context) error {
	f.calls = append(f.calls, "showOrders")
	return nil
}
func (f *fakeLeaves) SampleLine(context.Context) (Line, error) {
	f.calls = append(f.calls, "sampleLine")
	return Line{Item: "sample"}, nil
}
func (f *fakeLeaves) FirstLine(context.Context) (Line, error) {
	f.calls = append(f.calls, "firstLine")
	return Line{Item: "first"}, nil
}
func (f *fakeLeaves) Me(_ context.Context, in MeInput) (Profile, error) {
	f.calls = append(f.calls, "me")
	f.me = in
	return Profile{Name: "me"}, nil
}

// fakeStates は States の偽物。
type fakeStates struct {
	token *Token
	err   error
}

func (f fakeStates) Token(context.Context) (*Token, error) { return f.token, f.err }

// fakeStateCases は StateCases の偽物。呼ばれた述語を記録する。
type fakeStateCases struct {
	cartsEmpty, ordersEmpty, tokenEmpty bool
	err                                 error
	calls                               []string
}

func (f *fakeStateCases) CartsIsEmpty(context.Context) (bool, error) {
	f.calls = append(f.calls, "carts")
	return f.cartsEmpty, f.err
}
func (f *fakeStateCases) OrdersIsEmpty(context.Context) (bool, error) {
	f.calls = append(f.calls, "orders")
	return f.ordersEmpty, f.err
}
func (f *fakeStateCases) TokenIsEmpty(context.Context) (bool, error) {
	f.calls = append(f.calls, "token")
	return f.tokenEmpty, f.err
}

// fakeCheckout は CheckoutCases の偽物。
type fakeCheckout struct {
	shopping, ordered       bool
	shoppingErr, orderedErr error
	calls                   []string
}

func (f *fakeCheckout) Shopping(context.Context) (bool, error) {
	f.calls = append(f.calls, "Shopping")
	return f.shopping, f.shoppingErr
}
func (f *fakeCheckout) Ordered(context.Context) (bool, error) {
	f.calls = append(f.calls, "Ordered")
	return f.ordered, f.orderedErr
}

// fakeLoggedIn は LoggedInCases の偽物。
type fakeLoggedIn struct{ in bool }

func (f fakeLoggedIn) LoggedIn(context.Context) (bool, error) { return f.in, nil }

type world struct {
	leaves     *fakeLeaves
	states     fakeStates
	stateCases *fakeStateCases
	checkout   *fakeCheckout
	loggedIn   fakeLoggedIn
}

func newWorld() *world {
	return &world{leaves: &fakeLeaves{}, stateCases: &fakeStateCases{}, checkout: &fakeCheckout{}}
}

func (w *world) ops() Ops {
	return New(w.leaves, w.states, w.stateCases, Classifiers{Checkout: w.checkout, LoggedIn: w.loggedIn})
}

func wantCalls(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestClassifyCheckout(t *testing.T) {
	ctx := context.Background()
	errA, errB := errors.New("a"), errors.New("b")
	tests := []struct {
		name string
		c    fakeCheckout
		want Checkout
		hits []string // ClassificationError の Hits。nil なら ClassificationError ではない
		err  error
	}{
		{name: "Shopping", c: fakeCheckout{shopping: true}, want: CheckoutShopping},
		{name: "Ordered", c: fakeCheckout{ordered: true}, want: CheckoutOrdered},
		{name: "none true", c: fakeCheckout{}, hits: []string{}},
		{name: "both true", c: fakeCheckout{shopping: true, ordered: true}, hits: []string{"Shopping", "Ordered"}},
		{name: "first predicate fails", c: fakeCheckout{shoppingErr: errA, ordered: true}, err: errA},
		{name: "second predicate fails", c: fakeCheckout{shopping: true, orderedErr: errB}, err: errB},
		{name: "both fail", c: fakeCheckout{shoppingErr: errA, orderedErr: errB}, err: errA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.c
			got, err := ClassifyCheckout(ctx, &c)
			// どの場合もすべての述語を評価する。
			wantCalls(t, "評価した述語", c.calls, "Shopping", "Ordered")
			var ce *runtime.ClassificationError
			switch {
			case tt.err != nil:
				if err != tt.err {
					t.Errorf("err = %v, want %v", err, tt.err)
				}
			case tt.hits != nil:
				if !errors.As(err, &ce) || ce.Classify != "shop.Checkout" || len(ce.Hits) != len(tt.hits) {
					t.Fatalf("err = %#v, want ClassificationError{shop.Checkout, %v}", err, tt.hits)
				}
				for i := range tt.hits {
					if ce.Hits[i] != tt.hits[i] {
						t.Errorf("Hits = %v, want %v", ce.Hits, tt.hits)
					}
				}
			default:
				if err != nil || got != tt.want {
					t.Errorf("= %v, %v, want %v, nil", got, err, tt.want)
				}
			}
		})
	}
}

func TestClassifyLoggedInDefault(t *testing.T) {
	ctx := context.Background()
	if got, err := ClassifyLoggedIn(ctx, fakeLoggedIn{in: true}); err != nil || got != LoggedInLoggedIn {
		t.Errorf("LoggedIn = %v, %v", got, err)
	}
	if got, err := ClassifyLoggedIn(ctx, fakeLoggedIn{}); err != nil || got != LoggedInDefault {
		t.Errorf("default = %v, %v", got, err)
	}
}

// match by C: 値の形（none のアームは nil）と、文の形（default のアーム）。
func TestMatchByClassify(t *testing.T) {
	ctx := context.Background()
	w := newWorld()
	w.checkout.shopping = true
	placed, err := w.ops().Checkout(ctx)
	if err != nil || placed == nil || placed.Id != "o1" {
		t.Errorf("Shopping: %v, %v", placed, err)
	}
	wantCalls(t, "Shopping の呼び出し", w.leaves.calls, "submitOrder")

	w = newWorld()
	w.checkout.ordered = true
	placed, err = w.ops().Checkout(ctx)
	if err != nil || placed != nil {
		t.Errorf("Ordered: %v, %v", placed, err)
	}
	wantCalls(t, "Ordered の呼び出し", w.leaves.calls)

	// 判定のエラー（真の数が合わない）はそのまま返り、アームは実行されない。
	w = newWorld()
	var ce *runtime.ClassificationError
	if _, err = w.ops().Checkout(ctx); !errors.As(err, &ce) {
		t.Errorf("どの述語も真でないのに err = %v", err)
	}
	wantCalls(t, "判定に失敗したときの呼び出し", w.leaves.calls)

	w = newWorld()
	w.checkout.ordered = true
	if err := w.ops().Open(ctx); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "ログインしていないときの Open", w.leaves.calls, "showLogin", "showOrders")
	w = newWorld()
	w.checkout.ordered = true
	w.loggedIn.in = true
	if err := w.ops().Open(ctx); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "ログインしているときの Open", w.leaves.calls, "showCart", "showOrders")
}

// match s by nullable の値の形。HasItems のアームでは bind の token を States から取って埋める。
func TestMatchByNullable(t *testing.T) {
	ctx := context.Background()
	w := newWorld()
	w.stateCases.tokenEmpty = true
	profile, err := w.ops().Who(ctx)
	if err != nil || profile != nil {
		t.Errorf("Empty: %v, %v", profile, err)
	}
	wantCalls(t, "Empty の呼び出し", w.leaves.calls)

	w = newWorld()
	tok := Token("t1")
	w.states.token = &tok
	profile, err = w.ops().Who(ctx)
	if err != nil || profile == nil || profile.Name != "me" {
		t.Errorf("HasItems: %v, %v", profile, err)
	}
	wantCalls(t, "HasItems の呼び出し", w.leaves.calls, "me")
	if want := (MeInput{Token: "t1", Verbose: true}); w.leaves.me != want {
		t.Errorf("me の引数 = %+v, want %+v", w.leaves.me, want)
	}

	// 述語のエラーはそのまま返る。
	w = newWorld()
	w.stateCases.err = errors.New("store down")
	if _, err = w.ops().Who(ctx); err != w.stateCases.err {
		t.Errorf("述語のエラー: err = %v", err)
	}
	wantCalls(t, "述語が失敗したときの呼び出し", w.leaves.calls)

	// アームの中で状態の値を取れなければ、呼び出しの前に返る。
	w = newWorld()
	w.states.err = errors.New("no token")
	if _, err = w.ops().Who(ctx); err != w.states.err {
		t.Errorf("States のエラー: err = %v", err)
	}
	wantCalls(t, "States が失敗したときの呼び出し", w.leaves.calls)
}

// match s by list の文の形と値の形。アームの書かれた順によらず、Empty のアームは空のときに動く。
func TestMatchByList(t *testing.T) {
	ctx := context.Background()
	w := newWorld()
	w.stateCases.cartsEmpty = true
	if err := w.ops().OpenCart(ctx); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "空のカートを開く", w.leaves.calls, "showEmpty")
	line, err := w.ops().Pick(ctx)
	if err != nil || line.Item != "sample" {
		t.Errorf("空のときの Pick = %v, %v", line, err)
	}

	w = newWorld()
	if err := w.ops().OpenCart(ctx); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "行のあるカートを開く", w.leaves.calls, "emptyCart")
	line, err = w.ops().Pick(ctx)
	if err != nil || line.Item != "first" {
		t.Errorf("行があるときの Pick = %v, %v", line, err)
	}
}

// none のアームは何もしない。両方のケースを並べたアームは分岐しない。どの形でも述語は呼ぶ。
func TestMatchArmsWithoutCode(t *testing.T) {
	ctx := context.Background()
	w := newWorld() // carts は行あり、token はあり
	if err := w.ops().Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "行あり・トークンありの Refresh", w.leaves.calls, "showCart", "showOrders", "firstLine")
	wantCalls(t, "呼んだ述語", w.stateCases.calls, "carts", "token", "orders", "orders", "carts")

	w = newWorld()
	w.stateCases.cartsEmpty, w.stateCases.tokenEmpty = true, true
	if err := w.ops().Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "空・トークンなしの Refresh", w.leaves.calls, "showLogin", "showOrders", "firstLine")

	w = newWorld()
	w.stateCases.err = errors.New("store down")
	if err := w.ops().Refresh(ctx); err != w.stateCases.err {
		t.Errorf("述語のエラー: err = %v", err)
	}
	wantCalls(t, "述語が失敗したときの呼び出し", w.leaves.calls)
}
