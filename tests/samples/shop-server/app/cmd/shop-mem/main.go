// Command shop-mem は、サンプル shop-server をメモリの上で動かす（../../../README.md「人が書いたもの」）。
// shop-sqlite と違うのは、組み立て（onmem.New）だけ。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"log"
	"net/http"
	"time"

	"example.com/shop-server/app"
	"example.com/shop-server/app/onmem"
	pkgtype "example.com/shop-server/gen/type"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "待ち受けるアドレス")
	flag.Parse()

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
	handler, store, _ := onmem.New(leaves.Server(), mailer.Send)

	// 試すためのユーザーを一人入れる（store の op で）
	demo := pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: app.HashPassword("password123")}
	if err := store.AddUser(context.Background(), demo); err != nil {
		log.Fatal(err)
	}
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
