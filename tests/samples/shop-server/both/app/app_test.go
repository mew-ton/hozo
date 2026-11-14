package main

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"example.com/shop-server/both/server"
	"example.com/shop-server/handlertest"
)

// newServer は handlertest の決まりの通りのサーバーを、メモリの上の SQLite で作る。
func newServer(t *testing.T) handlertest.Server {
	t.Helper()
	// ":memory:" は接続ごとに別のデータベースになるので、接続を一つに限る
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	now := handlertest.Now
	failDelivery := false
	tokens, orders := 0, 0
	app := &App{
		Now:        func() time.Time { return now },
		NewToken:   func() server.Token { tokens++; return server.Token(fmt.Sprintf("token-%d", tokens)) },
		NewOrderID: func() server.OrderId { orders++; return server.OrderId(fmt.Sprintf("order-%d", orders)) },
		Deliver: func(server.Notification) error {
			if failDelivery {
				return errors.New("届けられない")
			}
			return nil
		},
		SessionTTL: handlertest.SessionTTL,
		PageSize:   handlertest.PageSize,
	}
	handler, err := app.Setup(db)
	if err != nil {
		t.Fatal(err)
	}
	alice := server.User{
		Id:    handlertest.AliceID,
		Email: handlertest.AliceEmail,
		Name:  handlertest.AliceName,
		Hash:  HashPassword(handlertest.AlicePassword),
	}
	if err := server.Tables.Users.Insert(alice); err != nil {
		t.Fatal(err)
	}
	count := func(count func() (int64, error)) func() int {
		return func() int {
			n, err := count()
			if err != nil {
				t.Fatal(err)
			}
			return int(n)
		}
	}
	return handlertest.Server{
		Handler:       handler,
		Advance:       func(d time.Duration) { now = now.Add(d) },
		FailDelivery:  func(fail bool) { failDelivery = fail },
		Orders:        count(server.Tables.Orders.Count),
		Notifications: count(server.Tables.Notifications.Count),
	}
}

// TestHandler は、go-http のハンドラ → 配線 → go-sqlite の表の上に書いた葉の op、の全体の振る舞い。
func TestHandler(t *testing.T) {
	handlertest.Run(t, newServer)
}

// TestOrderSurvivesNotificationFailure は、通知に失敗したときの状態を表から直に確かめる: 応答は 500 でも、
// 注文の行は残っている（仕様: NotificationFailed のとき createOrder は起きたまま）。
func TestOrderSurvivesNotificationFailure(t *testing.T) {
	s := newServer(t)
	s.FailDelivery(true)
	ops := server.New(server.Leaves{})
	login, err := ops.Login(server.LoginInput{Body: server.Credentials{Email: handlertest.AliceEmail, Password: handlertest.AlicePassword}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ops.PlaceOrder(server.PlaceOrderInput{Token: &login.Token, Lines: []server.OrderLine{{Item: "apple", Quantity: 2}}})
	var failed server.NotificationFailed
	if !errors.As(err, &failed) {
		t.Fatalf("PlaceOrder のエラーが %v（期待は server.NotificationFailed）", err)
	}

	order, err := server.Tables.Orders.Get("order-1")
	if err != nil {
		t.Fatal(err)
	}
	if order == nil {
		t.Fatal("注文の行が残っていない")
	}
	if order.UserId != handlertest.AliceID || len(order.Lines) != 1 || order.Lines[0] != (server.OrderLine{Item: "apple", Quantity: 2}) ||
		!order.PlacedAt.Equal(handlertest.Now) {
		t.Errorf("残った注文が違う: %+v", *order)
	}
	// 効果の結果は、状態の単位の問い合わせでも分かる
	if empty, err := server.States.Orders.IsEmpty(); err != nil || empty {
		t.Errorf("States.Orders.IsEmpty() = %v, %v（期待は false）", empty, err)
	}
	if empty, err := server.States.Notifications.IsEmpty(); err != nil || !empty {
		t.Errorf("States.Notifications.IsEmpty() = %v, %v（期待は true）", empty, err)
	}
}
