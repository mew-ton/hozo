package onsqlite

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite" // database/sql のドライバ "sqlite"。生成された gen/store は import しない

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/gen/store"
)

// openDB は、メモリの上の SQLite を開く。
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	// 外部キー（@db.ref の REFERENCES）は、接続ごとに有効にしないと SQLite は確かめない。ドライバの DSN で指定する
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	// ":memory:" は接続ごとに別のデータベースになるので、接続を一つに限る
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// TestHandler は、go-http のハンドラ → 配線 → go-sqlite が出した store の op、の全体の振る舞い。
// onmem の TestHandler と同じ handlertest.Run を走らせる。
func TestHandler(t *testing.T) {
	handlertest.Run(t, func(t *testing.T, f *handlertest.Fixture) handlertest.Server {
		handler, s, err := New(openDB(t), f.Leaves(), f.Send())
		if err != nil {
			t.Fatal(err)
		}
		return handlertest.Server{
			Handler: handler,
			Store:   s,
			// 通知を読む op は仕様にないので、状態の単位の、形で決まる問い合わせで読む（go-sqlite が埋めた口）
			Notifications: store.States.Notifications.Get,
		}
	})
}
