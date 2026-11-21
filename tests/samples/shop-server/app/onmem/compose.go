// Package onmem は、server を gen/storemem（状態の単位の実装は、人が書いたメモリの上の memstore）の上に組み立てる。
// onsqlite（../onsqlite/compose.go）と違うのは、store の import と、状態の単位の実装（memstore の型）だけ
// （../../README.md「人が書いたもの」）。
package onmem

import (
	"net/http"

	"example.com/shop-server/app/memstore"
	"example.com/shop-server/gen/client"
	"example.com/shop-server/gen/flows"
	"example.com/shop-server/gen/server"
	store "example.com/shop-server/gen/storemem"
)

// New は m の上の store と、その上の server のハンドラを返す。limiter は、@rate.limited の数え方。
// 何度呼んでもよい: 返す値は互いに独立で、同じ m なら同じ状態を、別の m なら別の状態を読み書きする。
func New(m *memstore.Store, leaves server.Leaves, mailer server.MailerOps, limiter server.RateLimiter) (http.Handler, store.Ops) {
	// 状態の単位の実装は、人が状態ごとに書いた型（その状態の read と効果の本体）。一つでも渡し忘れれば、コンパイルできない。
	// この単位（gen/storemem）には @db.transaction を読むトランスパイラがいないので、継ぎ目の引数（seams）はなく、
	// store.placeOrder は、二つの効果を順に呼ぶだけ（../../README.md「人が書いたもの」）
	s := store.New(m.Users(), m.Sessions(), m.Orders(), m.Notifications())
	// store.Ops は、server が呼ぶ store の op（server.StoreOps）をそのまま満たす。
	// 継ぎ目（server.Seams）の実装は、go-ratelimit が出した型。この単位で継ぎ目を宣言したトランスパイラは一つなので、そのまま渡せる
	return server.NewHandler(server.New(leaves, mailer, s, server.NewRateSeams(limiter))), s
}

// NewFlows は、New と同じ store と server の上に、操作の op（client）と、フロー（flows）を重ねて返す（フローの E2E テストと、要求の
// テストが使う。../flowtest）。client は、server の入口の Ops を、Go の上で、直に呼ぶ: server.Ops は、client が呼ぶ server の op
// （client.ServerOps）を、client.Ops は、flows が呼ぶ client の op（flows.ClientOps）を、そのまま満たす。HTTP の層は、通らない。
func NewFlows(m *memstore.Store, leaves server.Leaves, mailer server.MailerOps, limiter server.RateLimiter) (flows.Ops, client.Ops, store.Ops) {
	s := store.New(m.Users(), m.Sessions(), m.Orders(), m.Notifications())
	c := client.New(server.New(leaves, mailer, s, server.NewRateSeams(limiter)))
	return flows.New(c), c, s
}
