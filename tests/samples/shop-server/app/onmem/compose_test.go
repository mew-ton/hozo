package onmem

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/app/memstore"
	"example.com/shop-server/app/storetest"
	store "example.com/shop-server/gen/storemem"
	pkgtype "example.com/shop-server/gen/type"
)

// newServer は、新しい memstore.Store の上に組み立てたサーバー。
func newServer(t *testing.T, f *handlertest.Fixture) handlertest.Server {
	m := memstore.New()
	handler, s := New(m, f.Leaves(), f.Mailer(), f.Limiter())
	return handlertest.Server{
		Handler: handler,
		Store:   s,
		// 通知の read は仕様にないので、状態の単位の、形で決まる問い合わせで読む（memstore の型のメソッド）
		Notifications: m.Notifications().Get,
	}
}

// TestHandler は、go-http のハンドラ → 配線 → 人がメモリの上に書いた read と効果、の全体の振る舞い。
// onsqlite の TestHandler と同じ handlertest.Run を走らせる。
func TestHandler(t *testing.T) {
	t.Parallel()
	handlertest.Run(t, newServer)
}

// TestTwoServers は、同じ出力（gen/storemem と gen/server）の上の二つのサーバーが、一つのプロセスで同時に動き、
// 互いに混ざらないこと: memstore.Store が二つ、store.New と server.New が二度。
func TestTwoServers(t *testing.T) {
	t.Parallel()
	handlertest.RunTwo(t, newServer)
}

// TestStore は、人がメモリの上に書いた read と効果の上の、store の op の振る舞い。onsqlite の TestStore と同じ
// storetest.Run を走らせる（server は組み立てない。gen/storemem と memstore だけで動く）。
func TestStore(t *testing.T) {
	t.Parallel()
	storetest.Run(t, func(t *testing.T) storetest.Store {
		m := memstore.New()
		users, sessions, orders, notifications := m.Users(), m.Sessions(), m.Orders(), m.Notifications()
		return storetest.Store{
			Ops: store.New(users, sessions, orders, notifications),
			// 形で決まる問い合わせは、store.New に渡したのと同じ値で読む
			States: storetest.States{Users: users, Sessions: sessions, Orders: orders, Notifications: notifications},
		}
	})
}

// failingNotifications は、効果 notifyUser がいつも失敗する、状態 notifications の単位。ほかは memstore のまま。
type failingNotifications struct{ memstore.Notifications }

var errNotify = errors.New("notifyUser が失敗した")

func (failingNotifications) NotifyUser(context.Context, pkgtype.Notification) error { return errNotify }

// TestPlaceOrderIsNotOneTransaction は、メモリの上の store.placeOrder が、一つのトランザクション**ではない**こと。
// 単位 gen/storemem には、@db.transaction を読むトランスパイラがいない（読む者がなければ、何も出ない）ので、生成された
// 配線は、二つの効果を順に呼ぶだけ: 二つ目の効果（notifyUser）が失敗しても、一つ目の効果（createOrder）の注文は残る。
// SQLite の上では残らない（onsqlite の TestPlaceOrderIsOneTransaction）。memstore の notifyUser が失敗するのは、
// 中断された ctx のときだけなので、ここでは、いつも失敗する単位に差し替えて確かめる。
func TestPlaceOrderIsNotOneTransaction(t *testing.T) {
	t.Parallel()
	m, ctx := memstore.New(), context.Background()
	s := store.New(m.Users(), m.Sessions(), m.Orders(), failingNotifications{m.Notifications()})
	order := pkgtype.Order{Id: "o1", UserId: "u1", PlacedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	if err := s.PlaceOrder(ctx, order, pkgtype.Notification{UserId: "u1", Message: "x"}); !errors.Is(err, errNotify) {
		t.Fatalf("placeOrder のエラーが %v（期待は、notifyUser の失敗）", err)
	}
	if got, err := m.Orders().Get(ctx); err != nil || len(got) != 1 || got[0].Id != "o1" {
		t.Errorf("二つ目の効果が失敗した後の orders = %+v, %v（期待は、注文 o1 が残る）", got, err)
	}
	if empty, err := m.Notifications().IsEmpty(ctx); err != nil || !empty {
		t.Errorf("二つ目の効果が失敗した後の notifications の IsEmpty = %v, %v（期待は true）", empty, err)
	}
}
