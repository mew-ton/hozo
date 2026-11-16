// Package onmem は、server を gen/storemem（状態の単位の実装は、人が書いたメモリの上の memstore）の上に組み立てる。
// onsqlite（../onsqlite/compose.go）と違うのは、store の import と、状態の単位の実装（memstore の型）だけ
// （../../README.md「人が書いたもの」）。
package onmem

import (
	"net/http"

	"example.com/shop-server/app/memstore"
	"example.com/shop-server/gen/server"
	store "example.com/shop-server/gen/storemem"
)

// New は m の上の store と、その上の server のハンドラを返す。limiter は、@rate.limited の数え方。
// 何度呼んでもよい: 返す値は互いに独立で、同じ m なら同じ状態を、別の m なら別の状態を読み書きする。
func New(m *memstore.Store, leaves server.Leaves, mailer server.MailerOps, limiter server.RateLimiter) (http.Handler, store.Ops) {
	// 状態の単位の実装は、人が状態ごとに書いた型。最後は、二つの状態にまたがる op（placeOrder）の実装。
	// 一つでも渡し忘れれば、コンパイルできない
	s := store.New(m.Users(), m.Sessions(), m.Orders(), m.Notifications(), m.Shared())
	// store.Ops は、server が呼ぶ store の op（server.StoreOps）をそのまま満たす。
	// 継ぎ目（server.Seams）の実装は、go-ratelimit が出した型。この単位で継ぎ目を宣言したトランスパイラは一つなので、そのまま渡せる
	return server.NewHandler(server.New(leaves, mailer, s, server.NewRateSeams(limiter))), s
}
