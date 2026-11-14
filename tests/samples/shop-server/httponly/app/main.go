package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"log"
	"net/http"
	"time"

	"example.com/shop-server/httponly/server"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "待ち受けるアドレス")
	flag.Parse()

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
	handler := app.Setup()

	// 仕様にユーザーを足す op がないので、試すためのユーザーを一人入れる。状態はメモリの上なので、止めれば消える
	demo := server.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: HashPassword("password123")}
	app.AddUser(demo)
	log.Printf("ユーザー %s（パスワード password123）を入れた", demo.Email)

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
