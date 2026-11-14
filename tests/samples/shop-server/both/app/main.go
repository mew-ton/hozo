package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"log"
	"net/http"
	"time"

	_ "modernc.org/sqlite" // database/sql のドライバ "sqlite"。生成された server パッケージは import しない

	"example.com/shop-server/both/server"
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

	app := &App{
		Now:        time.Now,
		NewToken:   func() server.Token { return server.Token(randomHex()) },
		NewOrderID: func() server.OrderId { return server.OrderId(randomHex()) },
		Deliver: func(n server.Notification) error {
			log.Printf("通知 → %s: %s", n.UserId, n.Message)
			return nil
		},
		SessionTTL: 24 * time.Hour,
		PageSize:   20,
	}
	handler, err := app.Setup(db)
	if err != nil {
		log.Fatal(err)
	}

	// 仕様にユーザーを足す op がないので、初めての起動なら、試すためのユーザーを一人入れる
	empty, err := server.States.Users.IsEmpty()
	if err != nil {
		log.Fatal(err)
	}
	if empty {
		demo := server.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: HashPassword("password123")}
		if err := server.Tables.Users.Insert(demo); err != nil {
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
