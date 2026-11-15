// Package onmem は、server を gen/storemem（op の中身は人が書いたメモリの上の実装 memstore）の上に組み立てる。
// onsqlite（../onsqlite/compose.go）と違うのは、store の import と、その準備（memstore.Setup）だけ（../../README.md「人が書いたもの」）。
package onmem

import (
	"net/http"

	"example.com/shop-server/app/memstore"
	"example.com/shop-server/gen/server"
	store "example.com/shop-server/gen/storemem"
)

// New はメモリの上の store と、その上の server のハンドラを返す。
//
// store.States はパッケージの変数なので、一つのプロセスで有効なのは、最後に New した分だけ。
func New(leaves server.Leaves, send server.MailerSendFunc) (http.Handler, store.Ops) {
	// 人が書いた実装: store の op と、形で決まる問い合わせの中身を store.States に入れる
	memstore.Setup()
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
	return server.NewHandler(ops), s
}
