// Package onsqlite は、server を gen/store（SQLite。状態の単位の実装は go-sqlite の出力）の上に組み立てる。
// onmem（../onmem/compose.go）と違うのは、store の import と、状態の単位の実装（go-sqlite の型）だけ
// （../../README.md「人が書いたもの」）。
package onsqlite

import (
	"context"
	"database/sql"
	"net/http"

	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/store"
)

// New は db に表を作り、db の上の store と、その上の server のハンドラを返す。limiter は、@rate.limited の数え方。
// 何度呼んでもよい: 返す値は互いに独立で、同じ db なら同じ表を、別の db なら別の表を読み書きする。
func New(ctx context.Context, db *sql.DB, leaves server.Leaves, mailer server.MailerOps, limiter server.RateLimiter) (http.Handler, store.Ops, error) {
	if err := store.CreateTables(ctx, db); err != nil {
		return nil, nil, err
	}
	// 状態の単位の実装は、go-sqlite が状態ごとに出した型。一つでも渡し忘れれば、コンパイルできない
	s := store.New(store.NewUsersTable(db), store.NewSessionsTable(db), store.NewOrdersTable(db), store.NewNotificationsTable(db))
	// store.Ops は、server が呼ぶ store の op（server.StoreOps）をそのまま満たす。
	// 継ぎ目（server.Seams）の実装は、go-ratelimit が出した型。この単位で継ぎ目を宣言したトランスパイラは一つなので、そのまま渡せる
	return server.NewHandler(server.New(leaves, mailer, s, server.NewRateSeams(limiter))), s, nil
}
