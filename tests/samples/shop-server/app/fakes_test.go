package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/shop-server/gen/flows/flowstest"
	pkghttp "example.com/shop-server/gen/http"
	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/servertest"
	pkgtype "example.com/shop-server/gen/type"
)

// このファイルは、基底が出した、にせものと、テストの中の組み立て（docs/plugin-go.md「テストのにせもの」）を使うテスト。
// servertest.NewOps が、server と store の配線（本物）を、流れの端（store の read と効果、mailer.send）と、継ぎ目の、にせものの
// 上に組み立てる。DB は、要らない。葉は、人が書いた本物（Leaves）を渡す。

// fakeLeaves は、このテストの葉: 止まった時計と、決まった識別子。
func fakeLeaves() *Leaves {
	return &Leaves{
		Now:        func() time.Time { return leafNow },
		NewToken:   func() (pkgtype.Token, error) { return "token-1", nil },
		NewOrderID: func() (pkgtype.OrderId, error) { return "order-1", nil },
		SessionTTL: time.Hour,
		PageSize:   2,
	}
}

// placeOrderIn は、leafSession のトークンで、apple を二つ、注文する入力。
func placeOrderIn() server.PlaceOrderInput {
	token := leafSession.Token
	return server.PlaceOrderInput{Token: &token, Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}}
}

// TestFakesPlaceOrder は、にせものの上の、server.placeOrder の流れ: 端が返す値（セッション）を言うと、配線と葉が、本物で通り、
// 効果が、言ったとおりの値で、一度ずつ呼ばれる。store.placeOrder の継ぎ目（@db.transaction）は、中身を、そのまま呼ぶ。
func TestFakesPlaceOrder(t *testing.T) {
	f := new(servertest.Fakes)
	session := leafSession
	f.Store.Sessions.SessionByToken.Returns(&session)
	ops := servertest.NewOps(f, fakeLeaves())

	got, err := ops.PlaceOrder(context.Background(), placeOrderIn())
	if err != nil {
		t.Fatalf("placeOrder: %v", err)
	}
	if got.Id != "order-1" || got.UserId != "u1" {
		t.Errorf("placeOrder = %+v（期待は、ユーザー u1 の、注文 order-1）", got)
	}
	if calls := f.Store.Sessions.SessionByToken.Calls; len(calls) != 1 || calls[0].Token != leafSession.Token {
		t.Errorf("sessionByToken の呼び出し = %+v（期待は、トークン %s で、一度）", calls, leafSession.Token)
	}
	if calls := f.Store.Seams.PlaceOrderDbTransaction.Calls; len(calls) != 1 {
		t.Errorf("継ぎ目 placeOrder@db.transaction の呼び出し = %d 回（期待は、一度）", len(calls))
	}
	if calls := f.Store.Orders.CreateOrder.Calls; len(calls) != 1 || calls[0].Order.Id != "order-1" {
		t.Errorf("createOrder の呼び出し = %+v（期待は、注文 order-1 で、一度）", calls)
	}
	if calls := f.Store.Notifications.NotifyUser.Calls; len(calls) != 1 || calls[0].Notification.UserId != "u1" {
		t.Errorf("notifyUser の呼び出し = %+v（期待は、ユーザー u1 への通知で、一度）", calls)
	}
	if calls := f.Mailer.Send.Calls; len(calls) != 1 || calls[0].Notice.UserId != "u1" {
		t.Errorf("mailer.send の呼び出し = %+v（期待は、ユーザー u1 への通知で、一度）", calls)
	}
	if calls := f.Leaves.NewOrder.Calls; len(calls) != 1 {
		t.Errorf("葉 newOrder の呼び出し = %d 回（期待は、一度。言われていない葉は、覚えたうえで、本物を呼ぶ）", len(calls))
	}
}

// TestFakesThrow は、にせもので、例外を起こす: 流れの端（mailer.send）、葉（requireFresh）、継ぎ目（@db.transaction）、
// 端が返す値（セッションが、ない）。どれも、ほかは、TestFakesPlaceOrder と同じ用意のまま。
func TestFakesThrow(t *testing.T) {
	setup := func() *servertest.Fakes {
		f := new(servertest.Fakes)
		session := leafSession
		f.Store.Sessions.SessionByToken.Returns(&session)
		return f
	}
	place := func(f *servertest.Fakes) error {
		_, err := servertest.NewOps(f, fakeLeaves()).PlaceOrder(context.Background(), placeOrderIn())
		return err
	}

	t.Run("流れの端が投げる", func(t *testing.T) {
		f := setup()
		f.Mailer.Send.Throws(pkgtype.NotificationFailed{})
		var want pkgtype.NotificationFailed
		if err := place(f); !errors.As(err, &want) {
			t.Fatalf("placeOrder = %v（期待は type.NotificationFailed）", err)
		}
		if n := len(f.Store.Orders.CreateOrder.Calls); n != 1 {
			t.Errorf("createOrder の呼び出し = %d 回（期待は、一度: 配送の失敗の前に、注文は残っている）", n)
		}
	})
	t.Run("葉が投げる", func(t *testing.T) {
		f := setup()
		f.Leaves.RequireFresh.Throws(pkgtype.Unauthorized{})
		var want pkgtype.Unauthorized
		if err := place(f); !errors.As(err, &want) {
			t.Fatalf("placeOrder = %v（期待は type.Unauthorized）", err)
		}
		if n := len(f.Store.Orders.CreateOrder.Calls); n != 0 {
			t.Errorf("createOrder の呼び出し = %d 回（期待は、なし）", n)
		}
	})
	t.Run("端が、ないを返す", func(t *testing.T) {
		f := setup()
		f.Store.Sessions.SessionByToken.Returns(nil)
		var want pkgtype.Unauthorized
		if err := place(f); !errors.As(err, &want) {
			t.Fatalf("placeOrder = %v（期待は type.Unauthorized）", err)
		}
		if n := len(f.Leaves.RequireFresh.Calls); n != 0 {
			t.Errorf("葉 requireFresh の呼び出し = %d 回（期待は、なし: セッションが、ない）", n)
		}
	})
	t.Run("継ぎ目が投げる", func(t *testing.T) {
		f := setup()
		failed := errors.New("トランザクションを始められない")
		f.Store.Seams.PlaceOrderDbTransaction.Throws(failed)
		if err := place(f); !errors.Is(err, failed) {
			t.Fatalf("placeOrder = %v（期待は、継ぎ目の失敗）", err)
		}
		if n := len(f.Store.Orders.CreateOrder.Calls); n != 0 {
			t.Errorf("createOrder の呼び出し = %d 回（期待は、なし: 継ぎ目が、中身を呼んでいない）", n)
		}
	})
}

// TestFakesFlow は、にせものの上の、フロー: flowstest.NewOps が、flows と client の配線（本物）を、重ねる。client が呼ぶ server の op は、
// 相手の入口で、流れの端（docs/plugin-go.md「テストのにせもの」の「別の名前空間」）: にせものが、言った値を返し、server の中は、
// 通らない。ログインして、注文し、一覧を見るまでが、サーバーなしで通る。
func TestFakesFlow(t *testing.T) {
	f := new(flowstest.Fakes)
	s := &f.Client.Server
	s.Login.Returns(server.LoginOk{Token: "token-1"})
	s.PlaceOrder.Returns(pkgtype.Order{Id: "order-1", UserId: "u1"})
	s.GetOrders.Returns([]pkgtype.Order{{Id: "order-1", UserId: "u1"}})

	got, err := flowstest.NewOps(f).SignInAndOrder(context.Background(), "alice@example.com", "password123",
		[]pkgtype.OrderLine{{Item: "apple", Quantity: 2}})
	if err != nil {
		t.Fatalf("signInAndOrder: %v", err)
	}
	if got.Placed.Id != "order-1" || len(got.Listed) != 1 {
		t.Errorf("signInAndOrder = %+v（期待は、注文 order-1 と、一件の一覧）", got)
	}
	if calls := s.Login.Calls; len(calls) != 1 || calls[0].In.Body.Email != "alice@example.com" {
		t.Errorf("server.login の呼び出し = %+v（期待は、alice@example.com で、一度）", calls)
	}
	if calls := s.PlaceOrder.Calls; len(calls) != 1 || calls[0].In.Token == nil || *calls[0].In.Token != "token-1" {
		t.Errorf("server.placeOrder の呼び出し = %+v（期待は、ログインが返したトークン token-1 で、一度）", calls)
	}
	if calls := s.GetOrders.Calls; len(calls) != 1 || calls[0].In.Token == nil || *calls[0].In.Token != "token-1" {
		t.Errorf("server.getOrders の呼び出し = %+v（期待は、トークン token-1 で、一度）", calls)
	}
}

// TestFakesFlowThrow は、相手の入口の境界で起きる例外（@http.post の outer throws: http.NetworkError）を、相手の入口のにせもので
// 起こす: フローは、その例外で終わり、その先の op は、呼ばれない。
func TestFakesFlowThrow(t *testing.T) {
	f := new(flowstest.Fakes)
	s := &f.Client.Server
	s.Login.Returns(server.LoginOk{Token: "token-1"})
	s.PlaceOrder.Throws(pkghttp.NetworkError{})

	_, err := flowstest.NewOps(f).SignInAndOrder(context.Background(), "alice@example.com", "password123",
		[]pkgtype.OrderLine{{Item: "apple", Quantity: 2}})
	var want pkghttp.NetworkError
	if !errors.As(err, &want) {
		t.Fatalf("signInAndOrder = %v（期待は http.NetworkError）", err)
	}
	if n := len(s.GetOrders.Calls); n != 0 {
		t.Errorf("server.getOrders の呼び出し = %d 回（期待は、なし: 注文で、止まっている）", n)
	}
}
