package app_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/gen/server"
	pkgtype "example.com/shop-server/gen/type"
)

// 生成されたメソッドの形。インターフェースのメソッド式（最初の引数がそのインターフェース）を、関数の型に代入できることで確かめる。
//
//   - ctx: 純粋な op（仕様から割り出したもの: デコレータ、reads、effects がなく、状態に依らない。配線なら、呼び出し先も
//     すべて純粋）は取らず、そうでない op は最初の引数に取る。別の名前空間の op（MailerOps、StoreOps）も相手の op の判定に従う:
//     mailer.send はデコレータ @smtp.sendMail を、store.findSession は reads を持つので、純粋でなく、ctx を取る。
//   - error: 純粋で、throws のない op は返さない。それ以外は返す（純粋でも、throws があれば返す）。
//
// 外から形を書けるのは、公開のメソッドだけ: export を付けた op（server.Ops）、人が実装する葉（server.Leaves）、別の名前空間の
// op（server.MailerOps、server.StoreOps）。内部の配線（issueSession、authenticate、notify）は、非公開の構造体の非公開の
// メソッドなので、このパッケージからは名指しできない。その形を確かめるのは、生成されたコードがコンパイルできることだけ。
var (
	// 純粋で、throws がない: ctx も error もない
	_ func(server.Leaves, pkgtype.User) pkgtype.Session                       = server.Leaves.NewSession
	_ func(server.Leaves, pkgtype.User) pkgtype.PublicUser                    = server.Leaves.ToPublicUser
	_ func(server.Leaves, []pkgtype.Order, int64) []pkgtype.Order             = server.Leaves.PageOf
	_ func(server.Leaves, pkgtype.Session, []pkgtype.OrderLine) pkgtype.Order = server.Leaves.NewOrder
	_ func(server.Leaves, pkgtype.Order) pkgtype.Notification                 = server.Leaves.ComposeNotice
	// 純粋で、throws がある: ctx はなく、error はある
	_ func(server.Leaves, pkgtype.User, pkgtype.Password) error     = server.Leaves.CheckPassword
	_ func(server.Leaves, pkgtype.Session) (pkgtype.Session, error) = server.Leaves.RequireFresh
	// 純粋でない: ctx も error もある
	_ func(server.Ops, context.Context, server.LoginInput) (server.LoginOk, error)      = server.Ops.Login
	_ func(server.Ops, context.Context, server.AuthInput) error                         = server.Ops.Logout
	_ func(server.Ops, context.Context, server.AuthInput) (pkgtype.PublicUser, error)   = server.Ops.GetMe
	_ func(server.Ops, context.Context, server.GetOrdersInput) ([]pkgtype.Order, error) = server.Ops.GetOrders
	_ func(server.Ops, context.Context, server.PlaceOrderInput) (pkgtype.Order, error)  = server.Ops.PlaceOrder
	_ func(server.MailerOps, context.Context, pkgtype.Notification) error               = server.MailerOps.Send
	_ func(server.StoreOps, context.Context, *pkgtype.Token) (*pkgtype.Session, error)  = server.StoreOps.FindSession
)

// ctxKey は、要求の context を見分けるために、テストが値を載せる鍵（生成されたコードは context に値を載せない）。
type ctxKey struct{}

// seenStore は、server が呼ぶ store の op（server.StoreOps）のうち、二つだけを書いたもの。届いた context を覚える。
// 書いていない op は、埋め込んだ nil のインターフェースのまま（呼べば panic）。構造体のフィールドに関数を一つずつ入れる形は
// もうないので、op を一つだけ差し替えるにも、型を一つ書く。
type seenStore struct {
	server.StoreOps
	seen []context.Context
}

func (s *seenStore) FindSession(ctx context.Context, token *pkgtype.Token) (*pkgtype.Session, error) {
	s.seen = append(s.seen, ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &pkgtype.Session{Token: *token, UserId: "u1", ExpiresAt: handlertest.Now.Add(time.Hour)}, nil
}

func (s *seenStore) FindUserById(ctx context.Context, id pkgtype.UserId) (*pkgtype.User, error) {
	s.seen = append(s.seen, ctx)
	return &pkgtype.User{Id: id, Email: "alice@example.com", Name: "Alice", Hash: "h"}, nil
}

// TestRequestContextReachesStore は、ハンドラが受けた要求の context が、配線を通って、別の名前空間の op
// （server.StoreOps）にそのまま届くこと。server は store のどの出力もなしに、インターフェースを満たす値を渡すだけで動く。
func TestRequestContextReachesStore(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	st := &seenStore{}
	handler := server.NewHandler(server.New(f.Leaves(), f.Mailer(), st))

	get := func(ctx context.Context) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
		req.Header.Set("Authorization", "token-1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// 生きている要求: 二つの op（findSession、findUserById）に、要求の context が届く
	rec := get(context.WithValue(context.Background(), ctxKey{}, "request-1"))
	if rec.Code != 200 || rec.Body.String() != `{"id":"u1","email":"alice@example.com","name":"Alice"}` {
		t.Fatalf("GET /me: %d %s", rec.Code, rec.Body)
	}
	if len(st.seen) != 2 || st.seen[0].Value(ctxKey{}) != "request-1" || st.seen[1].Value(ctxKey{}) != "request-1" {
		t.Errorf("store の op に届いた context が、要求のものではない: %v", st.seen)
	}

	// 中断された要求: op は中断を見て、そのエラーを返す。後ろの op は呼ばれない
	st.seen = nil
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "request-2"))
	cancel()
	rec = get(ctx)
	if rec.Code != 500 || rec.Body.String() != `{"error":"internal"}` {
		t.Errorf("中断された GET /me: %d %s", rec.Code, rec.Body)
	}
	if len(st.seen) != 1 || st.seen[0].Value(ctxKey{}) != "request-2" || !errors.Is(st.seen[0].Err(), context.Canceled) {
		t.Errorf("中断された要求で store の op に届いた context: %v", st.seen)
	}
}

// fixedNotice は、葉の op を一つだけ差し替えた server.Leaves: 埋め込んだ実装のうち、ComposeNotice を置き換える。
type fixedNotice struct{ server.Leaves }

func (fixedNotice) ComposeNotice(order pkgtype.Order) pkgtype.Notification {
	return pkgtype.Notification{UserId: order.UserId, Message: "差し替えた文面"}
}

// orderLog は、入口 placeOrder が通る別の名前空間の op を書いたもの: store.findSession（いつもログインしている）、
// store.addOrder、mailer.send、store.addNotification。server.MailerOps と、server.StoreOps（ほかの op は、埋め込んだ nil の
// インターフェースのまま）の両方として渡す。
type orderLog struct {
	server.StoreOps
	orders      []pkgtype.Order
	sent, added []pkgtype.Notification
}

func (l *orderLog) FindSession(_ context.Context, token *pkgtype.Token) (*pkgtype.Session, error) {
	return &pkgtype.Session{Token: *token, UserId: "u1", ExpiresAt: handlertest.Now.Add(time.Hour)}, nil
}

func (l *orderLog) AddOrder(_ context.Context, order pkgtype.Order) error {
	l.orders = append(l.orders, order)
	return nil
}

func (l *orderLog) Send(_ context.Context, notice pkgtype.Notification) error {
	l.sent = append(l.sent, notice)
	return nil
}

func (l *orderLog) AddNotification(_ context.Context, notification pkgtype.Notification) error {
	l.added = append(l.added, notification)
	return nil
}

// TestReplaceOneLeaf は、テストが葉の op を一つだけ差し替える形: 実装を埋め込んだ型に、そのメソッドだけを書く。
// 内部の配線 notify は、差し替えた composeNotice の結果を、mailer.send と store.addNotification に渡す。
//
// notify は export のない op なので、server.Ops にはなく、テストは直に呼べない。notify に届く、export を付けた op
// （placeOrder）を呼び、そこまでの道が通る op（store.findSession、葉 requireFresh と newOrder、store.addOrder）も用意する。
func TestReplaceOneLeaf(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	log := &orderLog{}
	ops := server.New(fixedNotice{f.Leaves()}, log, log)
	token := pkgtype.Token("token-1")
	order, err := ops.PlaceOrder(context.Background(), server.PlaceOrderInput{Token: &token})
	if err != nil {
		t.Fatal(err)
	}
	want := pkgtype.Notification{UserId: "u1", Message: "差し替えた文面"}
	if len(log.sent) != 1 || log.sent[0] != want || len(log.added) != 1 || log.added[0] != want {
		t.Errorf("mailer.send に %v、store.addNotification に %v（期待は、どちらも %v が一つ）", log.sent, log.added, want)
	}
	// 差し替えていない葉は、埋め込んだ実装のまま（newOrder が、識別子と、セッションのユーザーを入れる）
	if order.Id != "order-1" || order.UserId != "u1" || len(log.orders) != 1 || log.orders[0].Id != order.Id {
		t.Errorf("PlaceOrder = %+v、store.addOrder に %v", order, log.orders)
	}
}

// spyOps は、呼ばれた回数を数えるだけの server.Ops（go-http が受け取る、export を付けた五つの op）。
// 数えない三つは、埋め込んだ nil のインターフェースのまま（呼べば panic）。
type spyOps struct {
	server.Ops
	logins, placeOrders int
}

func (s *spyOps) Login(context.Context, server.LoginInput) (server.LoginOk, error) {
	s.logins++
	return server.LoginOk{}, nil
}

func (s *spyOps) PlaceOrder(context.Context, server.PlaceOrderInput) (pkgtype.Order, error) {
	s.placeOrders++
	return pkgtype.Order{}, nil
}

// TestValidationPrecedesOp は、go-http（validate = true）が、読めない要求と制約に合わない入力では op を呼ばないこと。
func TestValidationPrecedesOp(t *testing.T) {
	t.Parallel()
	spy := &spyOps{}
	handler := server.NewHandler(spy)
	post := func(path, body string) int {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec.Code
	}
	for _, c := range []struct {
		path, body string
		status     int
	}{
		{"/login", `{"email":"not-an-email","password":"password123"}`, 400},
		{"/login", `{"email":"alice@example.com","password":"short"}`, 400},
		{"/login", `{"email":`, 400},
		{"/orders", `[{"item":"apple","quantity":0}]`, 400},
		{"/orders", `{}`, 400},
	} {
		if got := post(c.path, c.body); got != c.status {
			t.Errorf("POST %s %s: %d（期待は %d）", c.path, c.body, got, c.status)
		}
	}
	if spy.logins != 0 || spy.placeOrders != 0 {
		t.Fatalf("op が呼ばれた: login %d 回、placeOrder %d 回", spy.logins, spy.placeOrders)
	}
	// 制約に合う入力なら、op を呼ぶ
	if got := post("/login", `{"email":"alice@example.com","password":"password123"}`); got != 200 {
		t.Errorf("POST /login: %d", got)
	}
	if got := post("/orders", `[{"item":"apple","quantity":1}]`); got != 200 {
		t.Errorf("POST /orders: %d", got)
	}
	if spy.logins != 1 || spy.placeOrders != 1 {
		t.Errorf("op が呼ばれた回数: login %d 回、placeOrder %d 回（期待は 1 回ずつ）", spy.logins, spy.placeOrders)
	}
}

// TestNilIsNotChecked は、New が引数を確かめないこと。渡し忘れ（引数が足りない、メソッドが足りない）はコンパイルできないが、
// nil を書いて渡すことはできる。そのときは *runtime.MissingError ではなく、その実装を使う op を呼んだところで panic になる。
// 葉の op は server.Ops にないので、葉に届く入口（login）を、nil の leaves で呼ぶ。
func TestNilIsNotChecked(t *testing.T) {
	t.Parallel()
	ops := server.New(nil, nil, registered{})
	defer func() {
		if recover() == nil {
			t.Error("nil の leaves に届く op を呼んでも panic にならない")
		}
	}()
	_, _ = ops.Login(context.Background(), server.LoginInput{})
}

// registered は、どの email にもユーザーがいる server.StoreOps。login が、葉 checkPassword（leaves）まで進む。
type registered struct{ server.StoreOps }

func (registered) FindUser(_ context.Context, email pkgtype.Email) (*pkgtype.User, error) {
	return &pkgtype.User{Id: "u1", Email: email}, nil
}
