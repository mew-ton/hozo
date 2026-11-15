package app_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/gen/server"
	pkgtype "example.com/shop-server/gen/type"
	"github.com/mew-ton/hozo/runtime"
)

// 純粋な op（仕様から割り出したもの: デコレータ、reads、effects がなく、状態にも別の名前空間にも触れない）の関数は
// ctx を取らず、そうでない op の関数は最初の引数に取る。生成された関数の型が、この代入をコンパイルできることで確かめる。
var (
	_ func(pkgtype.User, pkgtype.Password) error                                    = server.CheckPasswordFunc(nil)
	_ func(pkgtype.User) (pkgtype.Session, error)                                   = server.NewSessionFunc(nil)
	_ func(pkgtype.User) (pkgtype.PublicUser, error)                                = server.ToPublicUserFunc(nil)
	_ func([]pkgtype.Order, int64) ([]pkgtype.Order, error)                         = server.PageOfFunc(nil)
	_ func(pkgtype.Session, []pkgtype.OrderLine) (pkgtype.Order, error)             = server.NewOrderFunc(nil)
	_ func(pkgtype.Order) (pkgtype.Notification, error)                             = server.ComposeNoticeFunc(nil)
	_ func(context.Context, server.LoginInput) (server.LoginOk, error)              = server.LoginFunc(nil)
	_ func(context.Context, *pkgtype.Token) (pkgtype.Session, error)                = server.AuthenticateFunc(nil)
	_ func(context.Context, pkgtype.Order) error                                    = server.NotifyFunc(nil)
	_ func(context.Context, pkgtype.Notification) error                             = server.MailerSendFunc(nil)
	_ func(context.Context, *pkgtype.Token) (*pkgtype.Session, error)               = server.StoreFindSessionFunc(nil)
	_ func(context.Context, pkgtype.User, pkgtype.Password) (server.LoginOk, error) = server.IssueSessionFunc(nil)
)

// ctxKey は、要求の context を見分けるために、テストが値を載せる鍵（生成されたコードは context に値を載せない）。
type ctxKey struct{}

// TestRequestContextReachesDeps は、ハンドラが受けた要求の context が、配線を通って、別の名前空間の op の口
// （server.Deps）にそのまま届くこと。server は store のどの出力もなしに、口を埋めるだけで動く。
func TestRequestContextReachesDeps(t *testing.T) {
	f := &handlertest.Fixture{}
	var seen []context.Context
	ops := server.New(f.Leaves(), server.Deps{Store: server.StoreOps{
		FindSession: func(ctx context.Context, token *pkgtype.Token) (*pkgtype.Session, error) {
			seen = append(seen, ctx)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return &pkgtype.Session{Token: *token, UserId: "u1"}, nil
		},
		FindUserById: func(ctx context.Context, id pkgtype.UserId) (*pkgtype.User, error) {
			seen = append(seen, ctx)
			return &pkgtype.User{Id: id, Email: "alice@example.com", Name: "Alice", Hash: "h"}, nil
		},
	}})
	handler := server.NewHandler(ops)

	get := func(ctx context.Context) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
		req.Header.Set("Authorization", "token-1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// 生きている要求: 二つの口（findSession、findUserById）に、要求の context が届く
	rec := get(context.WithValue(context.Background(), ctxKey{}, "request-1"))
	if rec.Code != 200 || rec.Body.String() != `{"id":"u1","email":"alice@example.com","name":"Alice"}` {
		t.Fatalf("GET /me: %d %s", rec.Code, rec.Body)
	}
	if len(seen) != 2 || seen[0].Value(ctxKey{}) != "request-1" || seen[1].Value(ctxKey{}) != "request-1" {
		t.Errorf("口に届いた context が、要求のものではない: %v", seen)
	}

	// 中断された要求: 口は中断を見て、そのエラーを返す。後ろの口は呼ばれない
	seen = nil
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "request-2"))
	cancel()
	rec = get(ctx)
	if rec.Code != 500 || rec.Body.String() != `{"error":"internal"}` {
		t.Errorf("中断された GET /me: %d %s", rec.Code, rec.Body)
	}
	if len(seen) != 1 || seen[0].Value(ctxKey{}) != "request-2" || !errors.Is(seen[0].Err(), context.Canceled) {
		t.Errorf("中断された要求で口に届いた context: %v", seen)
	}
}

// spyOps は、呼ばれた回数を数えるだけの server.Ops。数えない op は、埋め込んだ nil のインターフェースのまま（呼べば panic）。
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

// TestMissing は、埋めていない口: 入れ忘れはコンパイルでは分からず、その op が呼ばれたときに *runtime.MissingError になり、
// ハンドラは 500 を返す。
func TestMissing(t *testing.T) {
	f := &handlertest.Fixture{}
	ctx := context.Background()
	var missing *runtime.MissingError

	// 別の名前空間の op の口（Deps）
	ops := server.New(f.Leaves(), server.Deps{})
	_, err := ops.Login(ctx, server.LoginInput{Body: server.Credentials{Email: "alice@example.com", Password: "password123"}})
	if !errors.As(err, &missing) || missing.Kind != "dep" || missing.Name != "store.findUser" {
		t.Errorf("Login のエラーが %v", err)
	}
	if err := ops.Notify(ctx, pkgtype.Order{Id: "o1", UserId: "u1"}); !errors.As(err, &missing) || missing.Name != "mailer.send" {
		t.Errorf("Notify のエラーが %v", err)
	}
	rec := httptest.NewRecorder()
	server.NewHandler(ops).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))
	if rec.Code != 500 || rec.Body.String() != `{"error":"internal"}` {
		t.Errorf("GET /me: %d %s", rec.Code, rec.Body)
	}

	// 葉の op の口（Leaves）
	ops = server.New(server.Leaves{}, server.Deps{})
	if _, err := ops.ToPublicUser(pkgtype.User{}); !errors.As(err, &missing) || missing.Kind != "leaf" || missing.Name != "server.toPublicUser" {
		t.Errorf("ToPublicUser のエラーが %v", err)
	}
}
