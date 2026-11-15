// Package onsqlite は、server を gen/store（SQLite。op の中身は go-sqlite の出力）の上に組み立てる。
// onmem（../onmem/compose.go）と違うのは、store の import と、その準備（SetupDB）だけ（../../README.md「人が書いたもの」）。
package onsqlite

import (
	"database/sql"
	"net/http"

	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/store"
)

// New は db に表を作り、db の上の store と、その上の server のハンドラを返す。
//
// store.States はパッケージの変数なので、一つのプロセスで有効なのは、最後に New した db だけ。
func New(db *sql.DB, leaves server.Leaves, send server.MailerSendFunc) (http.Handler, store.Ops, error) {
	// go-sqlite の出力: 表を作り、store の op の中身を store.States に入れる
	if err := store.SetupDB(db); err != nil {
		return nil, nil, err
	}
	s := store.New()
	ops := server.New(leaves, server.Deps{
		Mailer: server.MailerOps{Send: send},
		// store の op を、server が呼ぶ口にそのまま入れる（型は同じ gen/type のもの）
		Store: server.StoreOps{
			FindUser:        s.FindUser,
			FindUserById:    s.FindUserById,
			FindSession:     s.FindSession,
			ListOrders:      s.ListOrders,
			AddSession:      s.AddSession,
			RemoveSession:   s.RemoveSession,
			AddOrder:        s.AddOrder,
			AddNotification: s.AddNotification,
		},
	})
	// go-http の出力
	return server.NewHandler(ops), s, nil
}
