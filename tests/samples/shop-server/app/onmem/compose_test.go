package onmem

import (
	"testing"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/app/memstore"
	"example.com/shop-server/app/storetest"
	store "example.com/shop-server/gen/storemem"
)

// newServer は、新しい memstore.Store の上に組み立てたサーバー。
func newServer(t *testing.T, f *handlertest.Fixture) handlertest.Server {
	m := memstore.New()
	handler, s := New(m, f.Leaves(), f.Mailer())
	return handlertest.Server{
		Handler: handler,
		Store:   s,
		// 通知を読む op は仕様にないので、状態の単位の、形で決まる問い合わせで読む（memstore の型のメソッド）
		Notifications: m.Notifications().Get,
	}
}

// TestHandler は、go-http のハンドラ → 配線 → 人がメモリの上に書いた store の op、の全体の振る舞い。
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

// TestStore は、人がメモリの上に書いた store の op の振る舞い。onsqlite の TestStore と同じ storetest.Run を走らせる
// （server は組み立てない。gen/storemem と memstore だけで動く）。
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
