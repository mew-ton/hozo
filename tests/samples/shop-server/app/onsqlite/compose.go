// Package onsqlite は、server を gen/store（SQLite。状態の単位の実装は go-sqlite の出力）の上に組み立てる
// （../../README.md「人が書いたもの」）。このサンプルの組み立ては、ここの一つ: メモリの上の組み立て（onmem）は、
// docs/decisions.md「決めたこと」388 で、なくした。
package onsqlite

import (
	"context"
	"database/sql"
	"net/http"

	"example.com/shop-server/gen/client"
	"example.com/shop-server/gen/flows"
	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/store"
)

// New は db に表を作り、db の上の store と、その上の server のハンドラを返す。limiter は、@rate.limited の数え方。
// 何度呼んでもよい: 返す値は互いに独立で、同じ db なら同じ表を、別の db なら別の表を読み書きする。
func New(ctx context.Context, db *sql.DB, leaves server.Leaves, mailer server.MailerOps, limiter server.RateLimiter) (http.Handler, store.Ops, error) {
	if err := store.CreateTables(ctx, db); err != nil {
		return nil, nil, err
	}
	// 状態の単位の実装は、go-sqlite が状態ごとに出した型（その状態の read と効果の中身）。最後は、@db.transaction の
	// 継ぎ目（store.placeOrder を、一つのトランザクションにする）の実装で、これも go-sqlite が出した型。表の型と同じ db を渡す。
	// 一つでも渡し忘れれば、コンパイルできない
	s := store.New(store.NewUsersTable(db), store.NewSessionsTable(db), store.NewOrdersTable(db), store.NewNotificationsTable(db),
		store.NewDbSeams(db))
	// store.Ops は、server が呼ぶ store の op（server.StoreOps）をそのまま満たす。
	// 継ぎ目（server.Seams）の実装は、go-ratelimit が出した型。この単位で継ぎ目を宣言したトランスパイラは一つなので、そのまま渡せる
	return server.NewHandler(server.New(leaves, mailer, s, server.NewRateSeams(limiter))), s, nil
}

// NewFlows は、New と同じ store と server の上に、操作の op（client）と、フロー（flows）を重ねて返す（フローの E2E テストと、要求の
// テストが使う。../flowtest）。client は、server の入口の Ops を、Go の上で、直に呼ぶ: server.Ops は、client が呼ぶ server の op
// （client.ServerOps）を、client.Ops は、flows が呼ぶ client の op（flows.ClientOps）を、そのまま満たす。HTTP の層は、通らない。
func NewFlows(ctx context.Context, db *sql.DB, leaves server.Leaves, mailer server.MailerOps, limiter server.RateLimiter) (flows.Ops, client.Ops, store.Ops, error) {
	if err := store.CreateTables(ctx, db); err != nil {
		return nil, nil, nil, err
	}
	s := store.New(store.NewUsersTable(db), store.NewSessionsTable(db), store.NewOrdersTable(db), store.NewNotificationsTable(db),
		store.NewDbSeams(db))
	c := client.New(server.New(leaves, mailer, s, server.NewRateSeams(limiter)))
	return flows.New(c), c, s, nil
}
