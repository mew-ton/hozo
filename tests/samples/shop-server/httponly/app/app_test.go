package main

import (
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/shop-server/handlertest"
	"example.com/shop-server/httponly/server"
)

// newServer は handlertest の決まりの通りのサーバーを、メモリの上の状態で作る。
func newServer(t *testing.T) handlertest.Server {
	t.Helper()
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
	handler := app.Setup()
	app.AddUser(server.User{
		Id:    handlertest.AliceID,
		Email: handlertest.AliceEmail,
		Name:  handlertest.AliceName,
		Hash:  HashPassword(handlertest.AlicePassword),
	})
	return handlertest.Server{
		Handler:       handler,
		Advance:       func(d time.Duration) { now = now.Add(d) },
		FailDelivery:  func(fail bool) { failDelivery = fail },
		Orders:        func() int { n, _ := app.Counts(); return n },
		Notifications: func() int { _, n := app.Counts(); return n },
	}
}

// TestHandler は、go-http のハンドラ → 配線 → 人が書いた葉の op（状態はメモリ）の全体の振る舞い。
// both/app と同じテストを走らせる: go-http の出力は、go-sqlite がいなくても同じに動く。
func TestHandler(t *testing.T) {
	handlertest.Run(t, newServer)
}

// TestMissingImplementation は、実装が入っていない op を呼んだときの応答: *runtime.MissingError は 500 で、中身を出さない。
func TestMissingImplementation(t *testing.T) {
	server.States = server.StateSet{}
	handler := server.NewHandler(server.New(server.Leaves{}))
	for _, c := range []struct{ method, path string }{
		{"GET", "/me"},      // Leaves.GetMe が nil
		{"POST", "/logout"}, // States.Sessions.Logout が nil
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		body, _ := io.ReadAll(rec.Result().Body)
		if rec.Code != 500 || string(body) != `{"error":"internal"}` {
			t.Errorf("%s %s: %d %s（期待は 500 {\"error\":\"internal\"}）", c.method, c.path, rec.Code, body)
		}
	}
}
