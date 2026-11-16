// Command shop-sqlite は、サンプル shop-server を SQLite の上で動かす（../../../README.md「人が書いたもの」）。
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"log"
	"net/http"
	"time"

	_ "modernc.org/sqlite" // database/sql のドライバ "sqlite"。生成された gen/store は import しない

	"example.com/shop-server/app"
	"example.com/shop-server/app/onsqlite"
	pkgtype "example.com/shop-server/gen/type"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "待ち受けるアドレス")
	// 外部キー（@db.ref の REFERENCES）は、接続ごとに有効にしないと SQLite は確かめない。ドライバの DSN で指定する
	dsn := flag.String("db", "file:shop.db?_pragma=foreign_keys(1)", "SQLite の DSN")
	flag.Parse()

	db, err := sql.Open("sqlite", *dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	leaves := &app.Leaves{
		Now:        time.Now,
		NewToken:   func() pkgtype.Token { return pkgtype.Token(randomHex()) },
		NewOrderID: func() pkgtype.OrderId { return pkgtype.OrderId(randomHex()) },
		SessionTTL: 24 * time.Hour,
		PageSize:   20,
	}
	mailer := &app.Mailer{Deliver: func(_ context.Context, notice pkgtype.Notification) error {
		log.Printf("通知 → %s: %s", notice.UserId, notice.Message)
		return nil
	}}
	// ログイン（@rate.limited の付いた op）は、1 分に 5 回まで
	limiter := &app.Limiter{Now: time.Now, Max: 5, Window: time.Minute}
	ctx := context.Background()
	handler, store, err := onsqlite.New(ctx, db, leaves, mailer, limiter)
	if err != nil {
		log.Fatal(err)
	}

	// 試すためのユーザーを、いなければ一人入れる（store の op で。保存先の表には直に触れない）
	demo := pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: app.HashPassword("password123")}
	found, err := store.FindUser(ctx, demo.Email)
	if err != nil {
		log.Fatal(err)
	}
	if found == nil {
		if err := store.AddUser(ctx, demo); err != nil {
			log.Fatal(err)
		}
		log.Printf("ユーザー %s（パスワード password123）を入れた", demo.Email)
	}

	log.Printf("http://%s で待ち受ける", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}

// randomHex は推測できない 32 桁の 16 進の文字列。
func randomHex() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
