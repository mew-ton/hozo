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
	"example.com/shop-server/app/memstore"
	"example.com/shop-server/app/onmem"
	pkgtype "example.com/shop-server/gen/type"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "待ち受けるアドレス")
	flag.Parse()

	leaves := &app.Leaves{
		Now:        time.Now,
		NewToken:   func() (pkgtype.Token, error) { s, err := randomHex(); return pkgtype.Token(s), err },
		NewOrderID: func() (pkgtype.OrderId, error) { s, err := randomHex(); return pkgtype.OrderId(s), err },
		SessionTTL: 24 * time.Hour,
		PageSize:   20,
	}
	mailer := &app.Mailer{Deliver: func(_ context.Context, notice pkgtype.Notification) error {
		log.Printf("通知 → %s: %s", notice.UserId, notice.Message)
		return nil
	}}
	// ログイン（@rate.limited の付いた op）は、1 分に 5 回まで
	limiter := &app.Limiter{Now: time.Now, Max: 5, Window: time.Minute}
	handler, store := onmem.New(memstore.New(), leaves, mailer, limiter)

	// 試すためのユーザーを一人入れる（store の op で）
	demo := pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: app.HashPassword("password123")}
	if err := store.AddUser(context.Background(), demo); err != nil {
		log.Fatal(err)
	}
	log.Printf("ユーザー %s（パスワード password123）を入れた", demo.Email)

	log.Printf("http://%s で待ち受ける", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}

// randomHex は推測できない 32 桁の 16 進の文字列。乱数を読めなければ、その error（葉の op が、そのまま返す）。
func randomHex() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
