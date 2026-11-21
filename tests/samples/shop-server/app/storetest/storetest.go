// Package storetest は、名前空間 store の op の振る舞いのテスト。人が書いたもので、走らせるのは、gen/store（read と効果の中身は
// go-sqlite が出したもの）の上の組み立て（../onsqlite）。
//
// ここで確かめるのは、仕様（store.hozo の read と効果と、その説明と、db の語彙のデコレータ）と、go-sqlite の規則
// （../../README.md「go-sqlite が従う規則」）から、保存先によらずに言えることだけ。仕様が言わないこと（@db.ref の先がないときなど）と、
// SQLite にだけ言えること（@db.transaction。onsqlite のテスト）は確かめない。
// 効果の結果は、状態の単位の、形で決まる問い合わせ（IsEmpty、Get）で確かめる: read のない状態（notifications）でも読める。
package storetest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	pkgtype "example.com/shop-server/gen/type"
)

// Ops は名前空間 store の op のうち、ここで試す九つ（recordNotice のほか。どれも export を付けてある）。gen/store の Ops が満たす
// （別のパッケージの別のインターフェースだが、型は同じ gen/type のものなので、メソッドの型が一致する）。
type Ops interface {
	FindUser(ctx context.Context, email pkgtype.Email) (*pkgtype.User, error)
	FindUserById(ctx context.Context, id pkgtype.UserId) (*pkgtype.User, error)
	FindSession(ctx context.Context, token pkgtype.Token) (*pkgtype.Session, error)
	ListOrders(ctx context.Context, userId pkgtype.UserId) ([]pkgtype.Order, error)
	LatestOrder(ctx context.Context, userId pkgtype.UserId) (*pkgtype.Order, error)
	AddUser(ctx context.Context, user pkgtype.User) error
	AddSession(ctx context.Context, session pkgtype.Session) error
	RemoveSession(ctx context.Context, token pkgtype.Token) error
	PlaceOrder(ctx context.Context, order pkgtype.Order, notification pkgtype.Notification) error
}

// List は、list の状態の単位が形から持つ問い合わせ（<状態>State の IsEmpty と Get）。
// 状態の単位の実装（go-sqlite の store.UsersTable など）は、どれもそのまま満たす。
type List[T any] interface {
	IsEmpty(ctx context.Context) (bool, error)
	Get(ctx context.Context) ([]T, error)
}

// States は、名前空間 store の四つの状態の、形で決まる問い合わせ。store.New に渡したのと同じ、状態の単位の実装を入れる。
type States struct {
	Users         List[pkgtype.User]
	Sessions      List[pkgtype.Session]
	Orders        List[pkgtype.Order]
	Notifications List[pkgtype.Notification]
}

// Store は試す相手: store の op と、その状態の、形で決まる問い合わせ。
type Store struct {
	Ops
	States States
}

var (
	alice = pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h1"}
	bob   = pkgtype.User{Id: "u2", Email: "bob@example.com", Name: "Bob", Hash: "h2"}
)

// Run は newStore が作る空の store の振る舞いを確かめる。サブテストごとに新しい store を作る。
// store は互いに何も共有しない（状態の実装は store.New の引数）ので、サブテストは並列に走らせる。
func Run(t *testing.T, newStore func(t *testing.T) Store) {
	ctx := context.Background()
	// start は、alice と bob のいる store を作る
	start := func(t *testing.T) Store {
		t.Helper()
		s := newStore(t)
		for _, u := range []pkgtype.User{alice, bob} {
			if err := s.AddUser(ctx, u); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		// 作ったばかりの store は、どの状態も空（IsEmpty は true、Get は空）
		s := newStore(t)
		wantState(t, ctx, s.States.Users, "users", userKey)
		wantState(t, ctx, s.States.Sessions, "sessions", sessionKey)
		wantState(t, ctx, s.States.Orders, "orders", orderKey)
		wantState(t, ctx, s.States.Notifications, "notifications", notificationKey)
	})

	t.Run("users", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		// findUser は email で、findUserById は id で引く。なければ nil
		wantUser(t, "findUser", &alice)(s.FindUser(ctx, "alice@example.com"))
		wantUser(t, "findUser", nil)(s.FindUser(ctx, "carol@example.com"))
		wantUser(t, "findUserById", &bob)(s.FindUserById(ctx, "u2"))
		wantUser(t, "findUserById", nil)(s.FindUserById(ctx, "u9"))
		// findUser（read userByEmail）が比べるのは、引数の名前の列（email）だけ。id の値を渡しても引けない
		wantUser(t, "findUser", nil)(s.FindUser(ctx, "u1"))
		// @db.key と @db.unique が同じ行は足せない: 効果 createUser は、表の一意の制約に当たったら、例外 type.EmailTaken を投げる
		// （@db.conflict。addUser の throws）。例外は、値で返る
		var taken pkgtype.EmailTaken
		if err := s.AddUser(ctx, pkgtype.User{Id: "u1", Email: "other@example.com", Name: "X", Hash: "h"}); !errors.As(err, &taken) {
			t.Errorf("addUser: id（@db.key）が同じ行 = %v（期待は type.EmailTaken）", err)
		}
		if err := s.AddUser(ctx, pkgtype.User{Id: "u3", Email: "alice@example.com", Name: "X", Hash: "h"}); !errors.As(err, &taken) {
			t.Errorf("addUser: email（@db.unique）が同じ行 = %v（期待は type.EmailTaken）", err)
		}
		// 状態の値は、足せた二人だけ（足した順）
		wantState(t, ctx, s.States.Users, "users", userKey, "u1", "u2")
	})

	t.Run("sessions", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		// Time は、時刻として同じものが返る（ゾーンと、ナノ秒までの小数部を持つ値で確かめる）
		expires := time.Date(2026, 1, 2, 12, 4, 5, 123456789, time.FixedZone("JST", 9*60*60))
		session := pkgtype.Session{Token: "t1", UserId: "u1", ExpiresAt: expires}
		if err := s.AddSession(ctx, session); err != nil {
			t.Fatal(err)
		}
		got, err := s.FindSession(ctx, "t1")
		if err != nil || got == nil {
			t.Fatalf("findSession = %v, %v", got, err)
		}
		if got.Token != "t1" || got.UserId != "u1" || !got.ExpiresAt.Equal(expires) {
			t.Errorf("findSession = %+v（期待は %+v）", *got, session)
		}
		wantState(t, ctx, s.States.Sessions, "sessions", sessionKey, "t1")
		// 知らないトークンは、ない（トークンのない呼び出しは、書けない: 引数の型は、列と同じ type.Token）
		if got, err := s.FindSession(ctx, "t9"); err != nil || got != nil {
			t.Errorf("findSession(t9) = %v, %v（期待は nil, nil）", got, err)
		}
		// removeSession の後は引けない。ない行を消しても、何も起きない
		if err := s.RemoveSession(ctx, "t1"); err != nil {
			t.Fatal(err)
		}
		if got, err := s.FindSession(ctx, "t1"); err != nil || got != nil {
			t.Errorf("removeSession の後の findSession = %v, %v（期待は nil, nil）", got, err)
		}
		if err := s.RemoveSession(ctx, "t1"); err != nil {
			t.Errorf("ない行の removeSession = %v（期待は nil）", err)
		}
		wantState(t, ctx, s.States.Sessions, "sessions", sessionKey)
	})

	t.Run("orders", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		orders := []pkgtype.Order{
			{Id: "o2", UserId: "u1", Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}, {Item: "pear", Quantity: 1}}, PlacedAt: at.Add(time.Hour)},
			{Id: "o3", UserId: "u2", Lines: []pkgtype.OrderLine{{Item: "plum", Quantity: 3}}, PlacedAt: at},
			{Id: "o1", UserId: "u1", Lines: []pkgtype.OrderLine{}, PlacedAt: at},
		}
		// 注文は placeOrder で足す（注文と、その通知の記録が、一緒に足される）
		for _, o := range orders {
			if err := s.PlaceOrder(ctx, o, pkgtype.Notification{UserId: o.UserId, Message: "注文 " + string(o.Id)}); err != nil {
				t.Fatal(err)
			}
		}
		// listOrders は、そのユーザーの注文だけを、足した順に返す（識別子の順でも、時刻の順でもない）
		got, err := s.ListOrders(ctx, "u1")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Id != "o2" || got[1].Id != "o1" {
			t.Fatalf("listOrders(u1) = %+v（期待は o2、o1 の順）", got)
		}
		// 配列のフィールド（lines）も、そのまま返る
		if !reflect.DeepEqual(got[0].Lines, orders[0].Lines) || len(got[1].Lines) != 0 || !got[0].PlacedAt.Equal(orders[0].PlacedAt) {
			t.Errorf("listOrders(u1) = %+v", got)
		}
		// 注文のないユーザーは、空
		if got, err := s.ListOrders(ctx, "u9"); err != nil || len(got) != 0 {
			t.Errorf("listOrders(u9) = %v, %v（期待は空）", got, err)
		}
		// latestOrder（read latestOrderOf）は、そのユーザーの注文のうち、placedAt がいちばん新しいもの。
		// 最後に足したもの（o1）ではない: u1 の o2 は、o1 より一時間新しい
		latest, err := s.LatestOrder(ctx, "u1")
		if err != nil || latest == nil || latest.Id != "o2" || !reflect.DeepEqual(latest.Lines, orders[0].Lines) {
			t.Errorf("latestOrder(u1) = %+v, %v（期待は o2）", latest, err)
		}
		// 注文のないユーザーは、ない
		if latest, err := s.LatestOrder(ctx, "u9"); err != nil || latest != nil {
			t.Errorf("latestOrder(u9) = %+v, %v（期待は nil, nil）", latest, err)
		}
		// placedAt が同じ注文が二つあれば、後から足した方（u2 に、o3 と同じ時刻の o4 を足す）
		if err := s.PlaceOrder(ctx, pkgtype.Order{Id: "o4", UserId: "u2", PlacedAt: at}, pkgtype.Notification{UserId: "u2", Message: "注文 o4"}); err != nil {
			t.Fatal(err)
		}
		if latest, err := s.LatestOrder(ctx, "u2"); err != nil || latest == nil || latest.Id != "o4" {
			t.Errorf("latestOrder(u2) = %+v, %v（期待は、後から足した o4）", latest, err)
		}
		// 状態の値は、どのユーザーの注文も、足した順。通知も、注文と同じ順に一つずつ。
		// 通知の read は仕様にないので、効果 notifyUser の結果は、状態の値（Get）で確かめる
		wantState(t, ctx, s.States.Orders, "orders", orderKey, "o2", "o3", "o1", "o4")
		wantState(t, ctx, s.States.Notifications, "notifications", notificationKey, "u1: 注文 o2", "u2: 注文 o3", "u1: 注文 o1", "u2: 注文 o4")
	})

	t.Run("placeOrder", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		// placeOrder は二つの効果（createOrder、notifyUser）を、この順に呼ぶ配線: 注文と通知を、一緒に足す。
		// 通知の model にはキーがないので、同じ文面の行も足せる
		at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		notice := pkgtype.Notification{UserId: "u1", Message: "こんにちは"}
		for _, id := range []pkgtype.OrderId{"o1", "o2"} {
			if err := s.PlaceOrder(ctx, pkgtype.Order{Id: id, UserId: "u1", PlacedAt: at}, notice); err != nil {
				t.Fatal(err)
			}
		}
		wantState(t, ctx, s.States.Orders, "orders", orderKey, "o1", "o2")
		wantState(t, ctx, s.States.Notifications, "notifications", notificationKey, "u1: こんにちは", "u1: こんにちは")
		// 一つ目の効果（createOrder）が失敗すれば、二つ目（notifyUser）は呼ばれない: id（@db.key）が同じ注文は足せず、
		// そのときは通知も足されない。どのエラーかは決まっていない。二つ目だけが失敗するときにどうなるかは、継ぎ目で決まる
		// （SQLite は、@db.transaction の継ぎ目が取り消す。onsqlite の TestPlaceOrderIsOneTransaction、TestTransactionIsTheSeam）
		if err := s.PlaceOrder(ctx, pkgtype.Order{Id: "o1", UserId: "u2", PlacedAt: at}, pkgtype.Notification{UserId: "u2", Message: "残らない"}); err == nil {
			t.Error("placeOrder: id（@db.key）が同じ注文を足せた")
		}
		wantState(t, ctx, s.States.Orders, "orders", orderKey, "o1", "o2")
		wantState(t, ctx, s.States.Notifications, "notifications", notificationKey, "u1: こんにちは", "u1: こんにちは")
		// 失敗の後も、続けて足せる
		if err := s.PlaceOrder(ctx, pkgtype.Order{Id: "o3", UserId: "u2", PlacedAt: at}, pkgtype.Notification{UserId: "u2", Message: "残る"}); err != nil {
			t.Errorf("失敗の後の placeOrder: %v", err)
		}
		wantState(t, ctx, s.States.Orders, "orders", orderKey, "o1", "o2", "o3")
		wantState(t, ctx, s.States.Notifications, "notifications", notificationKey, "u1: こんにちは", "u1: こんにちは", "u2: 残る")
	})

	t.Run("cancelled", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		// 中断された ctx で呼ばれた op は、ctx のエラーを返し、状態を変えない
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		calls := map[string]error{
			"findUser":      second(s.FindUser(cancelled, "alice@example.com")),
			"findUserById":  second(s.FindUserById(cancelled, "u1")),
			"findSession":   second(s.FindSession(cancelled, "t1")),
			"listOrders":    second(s.ListOrders(cancelled, "u1")),
			"latestOrder":   second(s.LatestOrder(cancelled, "u1")),
			"addUser":       s.AddUser(cancelled, pkgtype.User{Id: "u3", Email: "carol@example.com", Name: "Carol", Hash: "h3"}),
			"addSession":    s.AddSession(cancelled, pkgtype.Session{Token: "t1", UserId: "u1", ExpiresAt: time.Now()}),
			"removeSession": s.RemoveSession(cancelled, "t1"),
			"placeOrder": s.PlaceOrder(cancelled, pkgtype.Order{Id: "o1", UserId: "u1", PlacedAt: time.Now()},
				pkgtype.Notification{UserId: "u1", Message: "x"}),
			// 形で決まる問い合わせも ctx を取る（状態を読む）
			"users.IsEmpty":     second(s.States.Users.IsEmpty(cancelled)),
			"users.Get":         second(s.States.Users.Get(cancelled)),
			"notifications.Get": second(s.States.Notifications.Get(cancelled)),
		}
		for op, err := range calls {
			if !errors.Is(err, context.Canceled) {
				t.Errorf("中断された ctx での %s のエラーが %v（期待は context.Canceled）", op, err)
			}
		}
		// 四つの状態の値は、start の後のまま
		wantState(t, ctx, s.States.Users, "users", userKey, "u1", "u2")
		wantState(t, ctx, s.States.Sessions, "sessions", sessionKey)
		wantState(t, ctx, s.States.Orders, "orders", orderKey)
		wantState(t, ctx, s.States.Notifications, "notifications", notificationKey)
	})
}

// 状態の要素を、テストが見分ける文字列にする。
var (
	userKey         = func(u pkgtype.User) string { return string(u.Id) }
	sessionKey      = func(s pkgtype.Session) string { return string(s.Token) }
	orderKey        = func(o pkgtype.Order) string { return string(o.Id) }
	notificationKey = func(n pkgtype.Notification) string { return string(n.UserId) + ": " + n.Message }
)

// wantState は、状態 state の値（l の Get）が、key で見分けて want の並び（足した順）であることと、IsEmpty がそれに合うことを確かめる。
func wantState[T any](t *testing.T, ctx context.Context, l List[T], state string, key func(T) string, want ...string) {
	t.Helper()
	rows, err := l.Get(ctx)
	if err != nil {
		t.Errorf("%s の Get: %v", state, err)
		return
	}
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, key(row))
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s の Get = %v（期待は %v）", state, got, want)
	}
	if empty, err := l.IsEmpty(ctx); err != nil || empty != (len(want) == 0) {
		t.Errorf("%s の IsEmpty = %v, %v（期待は %v, nil）", state, empty, err, len(want) == 0)
	}
}

// wantUser は、ユーザーを引く op の結果が want（nil なら「ない」）であることを確かめる関数を返す。
func wantUser(t *testing.T, op string, want *pkgtype.User) func(*pkgtype.User, error) {
	t.Helper()
	return func(got *pkgtype.User, err error) {
		t.Helper()
		switch {
		case err != nil:
			t.Errorf("%s: %v", op, err)
		case want == nil && got != nil:
			t.Errorf("%s = %+v（期待は、ない）", op, *got)
		case want != nil && (got == nil || *got != *want):
			t.Errorf("%s = %+v（期待は %+v）", op, got, *want)
		}
	}
}

// second は、値とエラーを返す op の、エラーだけを取る。
func second[T any](_ T, err error) error { return err }
