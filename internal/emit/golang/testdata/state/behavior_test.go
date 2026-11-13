package shop

// このファイルは生成物ではない。internal/emit/golang の TestBehavior が、生成した shop のパッケージ
// （testdata/state/expected/gen/shop と同じ内容）の隣に置いて `go test` する。人やプラグインが実装する側
// （Leaves のフィールド、States の問い合わせと命令）を偽物で埋め、docs/emit-go.md「実装の単位」「state」「classify」
// 「match」の振る舞いを確かめる。生成したコードは context を取らないので、このテストも context を使わない。

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mew-ton/hozo/runtime"
)

// world は偽物の一式。calls は呼ばれた葉の op と命令の名前、queries は呼ばれた問い合わせの名前。
type world struct {
	calls   []string
	queries []string
	me      MeInput

	cartsEmpty, ordersEmpty, tokenEmpty bool
	queryErr                            error // 組み込みの分類の問い合わせが返すエラー
	token                               *Token
	tokenErr                            error
	shopping, ordered                   bool
	shoppingErr, orderedErr             error
	loggedIn                            bool
}

// install は w を States に入れ、w の葉の実装を持つ Ops を返す。States はパッケージの変数なので、
// テストごとに入れ直す。
func (w *world) install() Ops {
	call := func(name string) func() error {
		return func() error { w.calls = append(w.calls, name); return nil }
	}
	query := func(name string, v *bool) func() (bool, error) {
		return func() (bool, error) { w.queries = append(w.queries, name); return *v, w.queryErr }
	}
	States = StateSet{
		Carts:  CartsState{IsEmpty: query("carts", &w.cartsEmpty), EmptyCart: call("emptyCart")},
		Orders: OrdersState{IsEmpty: query("orders", &w.ordersEmpty)},
		Token: TokenState{
			IsEmpty: query("token", &w.tokenEmpty),
			Get:     func() (*Token, error) { return w.token, w.tokenErr },
		},
		Title: TitleState{Get: func() (string, error) { return "店", nil }},
		Checkout: CheckoutState{
			Shopping: func() (bool, error) { w.queries = append(w.queries, "Shopping"); return w.shopping, w.shoppingErr },
			Ordered:  func() (bool, error) { w.queries = append(w.queries, "Ordered"); return w.ordered, w.orderedErr },
			SubmitOrder: func() (Order, error) {
				w.calls = append(w.calls, "submitOrder")
				return Order{Id: "o1"}, nil
			},
		},
		LoggedIn: LoggedInState{LoggedIn: func() (bool, error) { return w.loggedIn, nil }},
	}
	return New(Leaves{
		ShowLogin:  call("showLogin"),
		ShowCart:   call("showCart"),
		ShowEmpty:  call("showEmpty"),
		ShowOrders: call("showOrders"),
		SampleLine: func() (Line, error) { w.calls = append(w.calls, "sampleLine"); return Line{Item: "sample"}, nil },
		FirstLine:  func() (Line, error) { w.calls = append(w.calls, "firstLine"); return Line{Item: "first"}, nil },
		Me: func(in MeInput) (Profile, error) {
			w.calls = append(w.calls, "me")
			w.me = in
			return Profile{Name: "me"}, nil
		},
	})
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

// wantMissing は err が *runtime.MissingError{kind, name} であることを確かめる。
func wantMissing(t *testing.T, what string, err error, kind, name string) {
	t.Helper()
	var miss *runtime.MissingError
	if !errors.As(err, &miss) || miss.Kind != kind || miss.Name != name {
		t.Errorf("%s: err = %#v, want *runtime.MissingError{Kind: %q, Name: %q}", what, err, kind, name)
	}
}

func TestClassifyCheckout(t *testing.T) {
	errA, errB := errors.New("a"), errors.New("b")
	tests := []struct {
		name string
		w    world
		want Checkout
		hits []string // ClassificationError の Hits。nil なら ClassificationError ではない
		err  error
	}{
		{name: "Shopping", w: world{shopping: true}, want: CheckoutShopping},
		{name: "Ordered", w: world{ordered: true}, want: CheckoutOrdered},
		{name: "none true", w: world{}, hits: []string{}},
		{name: "both true", w: world{shopping: true, ordered: true}, hits: []string{"Shopping", "Ordered"}},
		{name: "first predicate fails", w: world{shoppingErr: errA, ordered: true}, err: errA},
		{name: "second predicate fails", w: world{shopping: true, orderedErr: errB}, err: errB},
		{name: "both fail", w: world{shoppingErr: errA, orderedErr: errB}, err: errA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := tt.w
			w.install()
			got, err := ClassifyCheckout()
			// どの場合もすべての述語を呼ぶ。
			wantCalls(t, "呼んだ述語", w.queries, "Shopping", "Ordered")
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
	w := &world{loggedIn: true}
	w.install()
	if got, err := ClassifyLoggedIn(); err != nil || got != LoggedInLoggedIn {
		t.Errorf("LoggedIn = %v, %v", got, err)
	}
	w.loggedIn = false
	if got, err := ClassifyLoggedIn(); err != nil || got != LoggedInDefault {
		t.Errorf("default = %v, %v", got, err)
	}
}

// match by C: 値の形（none のアームは nil）と、文の形（default のアーム）。命令（submitOrder）は States を通る。
func TestMatchByClassify(t *testing.T) {
	w := &world{shopping: true}
	placed, err := w.install().Checkout()
	if err != nil || placed == nil || placed.Id != "o1" {
		t.Errorf("Shopping: %v, %v", placed, err)
	}
	wantCalls(t, "Shopping の呼び出し", w.calls, "submitOrder")

	w = &world{ordered: true}
	placed, err = w.install().Checkout()
	if err != nil || placed != nil {
		t.Errorf("Ordered: %v, %v", placed, err)
	}
	wantCalls(t, "Ordered の呼び出し", w.calls)

	// 判定のエラー（真の数が合わない）はそのまま返り、アームは実行されない。
	w = &world{}
	var ce *runtime.ClassificationError
	if _, err = w.install().Checkout(); !errors.As(err, &ce) {
		t.Errorf("どの述語も真でないのに err = %v", err)
	}
	wantCalls(t, "判定に失敗したときの呼び出し", w.calls)

	w = &world{ordered: true}
	if err := w.install().Open(); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "ログインしていないときの Open", w.calls, "showLogin", "showOrders")
	w = &world{ordered: true, loggedIn: true}
	if err := w.install().Open(); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "ログインしているときの Open", w.calls, "showCart", "showOrders")
}

// match s by nullable の値の形。HasItems のアームでは bind の token を States.Token.Get で取って埋める。
func TestMatchByNullable(t *testing.T) {
	w := &world{tokenEmpty: true}
	profile, err := w.install().Who()
	if err != nil || profile != nil {
		t.Errorf("Empty: %v, %v", profile, err)
	}
	wantCalls(t, "Empty の呼び出し", w.calls)

	tok := Token("t1")
	w = &world{token: &tok}
	profile, err = w.install().Who()
	if err != nil || profile == nil || profile.Name != "me" {
		t.Errorf("HasItems: %v, %v", profile, err)
	}
	wantCalls(t, "HasItems の呼び出し", w.calls, "me")
	if want := (MeInput{Token: "t1", Verbose: true}); w.me != want {
		t.Errorf("me の引数 = %+v, want %+v", w.me, want)
	}

	// 述語のエラーはそのまま返る。
	w = &world{queryErr: errors.New("store down")}
	if _, err = w.install().Who(); err != w.queryErr {
		t.Errorf("述語のエラー: err = %v", err)
	}
	wantCalls(t, "述語が失敗したときの呼び出し", w.calls)

	// アームの中で状態の値を取れなければ、呼び出しの前に返る。
	w = &world{tokenErr: errors.New("no token")}
	if _, err = w.install().Who(); err != w.tokenErr {
		t.Errorf("Get のエラー: err = %v", err)
	}
	wantCalls(t, "Get が失敗したときの呼び出し", w.calls)
}

// match s by list の文の形と値の形。アームの書かれた順によらず、Empty のアームは空のときに動く。
func TestMatchByList(t *testing.T) {
	w := &world{cartsEmpty: true}
	ops := w.install()
	if err := ops.OpenCart(); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "空のカートを開く", w.calls, "showEmpty")
	line, err := ops.Pick()
	if err != nil || line.Item != "sample" {
		t.Errorf("空のときの Pick = %v, %v", line, err)
	}

	w = &world{}
	ops = w.install()
	if err := ops.OpenCart(); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "行のあるカートを開く", w.calls, "emptyCart") // 命令は States.Carts.EmptyCart
	line, err = ops.Pick()
	if err != nil || line.Item != "first" {
		t.Errorf("行があるときの Pick = %v, %v", line, err)
	}
}

// none のアームは何もしない。両方のケースを並べたアームは分岐しない。どの形でも述語は呼ぶ。
func TestMatchArmsWithoutCode(t *testing.T) {
	w := &world{} // carts は行あり、token はあり
	if err := w.install().Refresh(); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "行あり・トークンありの Refresh", w.calls, "showCart", "showOrders", "firstLine")
	wantCalls(t, "呼んだ述語", w.queries, "carts", "token", "orders", "orders", "carts")

	w = &world{cartsEmpty: true, tokenEmpty: true}
	if err := w.install().Refresh(); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, "空・トークンなしの Refresh", w.calls, "showLogin", "showOrders", "firstLine")

	w = &world{queryErr: errors.New("store down")}
	if err := w.install().Refresh(); err != w.queryErr {
		t.Errorf("述語のエラー: err = %v", err)
	}
	wantCalls(t, "述語が失敗したときの呼び出し", w.calls)
}

// docs/emit-go.md「実装の単位」: フィールドは使わなければ nil のままでよく、nil のフィールドが要るところへ来たら
// panic せず *runtime.MissingError{Kind, Name} を返す。
func TestMissing(t *testing.T) {
	t.Run("leaf", func(t *testing.T) {
		// 何も埋めていない Ops でも、葉を直に呼べば MissingError
		States = StateSet{}
		ops := New(Leaves{})
		wantMissing(t, "ShowCart", ops.ShowCart(), "leaf", "shop.showCart")
		_, err := ops.Me(MeInput{})
		wantMissing(t, "Me", err, "leaf", "shop.me")

		// 配線の途中で当たった葉。それまでの行は実行されている
		w := &world{loggedIn: true, ordered: true}
		w.install()
		ops = New(Leaves{ShowCart: func() error { w.calls = append(w.calls, "showCart"); return nil }})
		wantMissing(t, "Open", ops.Open(), "leaf", "shop.showOrders")
		wantCalls(t, "当たるまでの呼び出し", w.calls, "showCart")
	})
	t.Run("state query", func(t *testing.T) {
		w := &world{}
		ops := w.install()
		States.Carts.IsEmpty = nil
		wantMissing(t, "OpenCart", ops.OpenCart(), "state", "shop.carts")
		_, err := ops.Pick()
		wantMissing(t, "Pick", err, "state", "shop.carts")
		wantCalls(t, "問い合わせがないときの呼び出し", w.calls)

		// bind の値の取得（Get）。IsEmpty は入っているので、当たるのは HasItems のアームの中
		w = &world{}
		ops = w.install()
		States.Token.Get = nil
		_, err = ops.Who()
		wantMissing(t, "Who", err, "state", "shop.token")
		wantCalls(t, "Get がないときの呼び出し", w.calls)
	})
	t.Run("state classification predicate", func(t *testing.T) {
		w := &world{ordered: true}
		ops := w.install()
		States.Checkout.Shopping = nil
		_, err := ClassifyCheckout()
		wantMissing(t, "ClassifyCheckout", err, "state", "shop.Checkout")
		wantCalls(t, "残りの述語は呼ぶ", w.queries, "Ordered")
		_, err = ops.Checkout()
		wantMissing(t, "Checkout", err, "state", "shop.Checkout")
	})
	t.Run("state command", func(t *testing.T) {
		w := &world{shopping: true}
		ops := w.install()
		States.Checkout.SubmitOrder = nil
		_, err := ops.SubmitOrder()
		wantMissing(t, "SubmitOrder", err, "state", "shop.submitOrder")
		_, err = ops.Checkout()
		wantMissing(t, "Checkout", err, "state", "shop.submitOrder")

		States.Carts.EmptyCart = nil
		wantMissing(t, "OpenCart", ops.OpenCart(), "state", "shop.emptyCart")
	})
	t.Run("zero values everywhere", func(t *testing.T) {
		// 何も入れていなくても、どの op も panic しない
		States = StateSet{}
		ops := New(Leaves{})
		_, err := ops.Checkout()
		wantMissing(t, "Checkout", err, "state", "shop.Checkout")
		wantMissing(t, "Open", ops.Open(), "state", "shop.LoggedIn")
		_, err = ops.Who()
		wantMissing(t, "Who", err, "state", "shop.token")
		wantMissing(t, "OpenCart", ops.OpenCart(), "state", "shop.carts")
		wantMissing(t, "Refresh", ops.Refresh(), "state", "shop.carts")
		wantMissing(t, "EmptyCart", ops.EmptyCart(), "state", "shop.emptyCart")
	})
}
