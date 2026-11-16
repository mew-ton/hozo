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
	"example.com/shop-server/gen/rate"
	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/inner"
	pkgtype "example.com/shop-server/gen/type"
)

// 生成されたメソッドの形。インターフェースのメソッド式（最初の引数がそのインターフェース）を、関数の型に代入できることで確かめる。
//
//   - ctx: 純粋な op（仕様から割り出したもの: デコレータ、reads、effects がなく、状態に依らない。配線なら、呼び出し先も
//     すべて純粋）は取らず、そうでない op は最初の引数に取る。別の名前空間の op（MailerOps、StoreOps）も相手の op の判定に従う:
//     mailer.send はデコレータ @smtp.sendMail を、store.findSession は reads を持つので、純粋でなく、ctx を取る。
//   - error: 純粋で、throws のない op は返さない。それ以外は返す（純粋でも、throws があれば返す）。
//
// 入口のパッケージ（gen/server）から形を書けるのは、export を付けた op（server.Ops）、人が実装する葉（server.Leaves）、
// 別の名前空間の op（server.MailerOps、server.StoreOps）、継ぎ目（server.Seams）。内部の配線（issueSession、authenticate、
// notify）は、内側のパッケージ（gen/server/inner）の Ops の公開のメソッドで、テストはそこから名指しできる。
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
	// 内部の配線も同じ判定: 純粋でない op を呼ぶので、ctx も error もある
	_ func(inner.Ops, context.Context, pkgtype.User, pkgtype.Password) (server.LoginOk, error) = inner.Ops.IssueSession
	_ func(inner.Ops, context.Context, *pkgtype.Token) (pkgtype.Session, error)                = inner.Ops.Authenticate
	_ func(inner.Ops, context.Context, pkgtype.Order) error                                    = inner.Ops.Notify
	// 継ぎ目: op の引数（ctx と in）の後ろに、op と同じ形の body を取り、op と同じものを返す。
	// デコレータの付いた op は純粋でないので、継ぎ目も body も、いつも ctx を取り、error を返す
	_ func(server.Seams, context.Context, server.LoginInput, func(context.Context, server.LoginInput) (server.LoginOk, error)) (server.LoginOk, error) = server.Seams.LoginRateLimited
)

// 入口の型は、内側の型の別名（同じ型）。入口の New が受け取る口は、内側の New にも、そのまま渡せる。
var (
	_ inner.Leaves    = server.Leaves(nil)
	_ inner.MailerOps = server.MailerOps(nil)
	_ inner.StoreOps  = server.StoreOps(nil)
	_ inner.Seams     = server.Seams(nil)
	_ inner.Seams     = server.RateSeams{} // go-ratelimit が入口のパッケージに出した型は、内側の Seams も満たす
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
	handler := server.NewHandler(server.New(f.Leaves(), f.Mailer(), st, server.NewRateSeams(f.Limiter())))

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

// noticeLog は、内部の配線 notify が呼ぶ別の名前空間の op を書いたもの: mailer.send と store.addNotification。
// server.MailerOps と、server.StoreOps（ほかの op は、埋め込んだ nil のインターフェースのまま）の両方として渡す。
type noticeLog struct {
	server.StoreOps
	sent, added []pkgtype.Notification
}

func (l *noticeLog) Send(_ context.Context, notice pkgtype.Notification) error {
	l.sent = append(l.sent, notice)
	return nil
}

func (l *noticeLog) AddNotification(_ context.Context, notification pkgtype.Notification) error {
	l.added = append(l.added, notification)
	return nil
}

// TestReplaceOneLeaf は、テストが葉の op を一つだけ差し替える形: 実装を埋め込んだ型に、そのメソッドだけを書く。
// 内部の配線 notify は、差し替えた composeNotice の結果を、mailer.send と store.addNotification に渡す。
//
// notify は export のない op なので、入口の server.Ops にはない。内側のパッケージの New が返す Ops（すべての op）から、
// 直に呼ぶ。notify が通らない op（store.findSession、葉 newOrder など）は、用意しなくてよい。
func TestReplaceOneLeaf(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	log := &noticeLog{}
	// 継ぎ目を持つ op（login）を通らないので、seams は渡さない（nil。使われないものは、確かめられない）
	ops := inner.New(fixedNotice{f.Leaves()}, log, log, nil)
	if err := ops.Notify(context.Background(), pkgtype.Order{Id: "order-1", UserId: "u1"}); err != nil {
		t.Fatal(err)
	}
	want := pkgtype.Notification{UserId: "u1", Message: "差し替えた文面"}
	if len(log.sent) != 1 || log.sent[0] != want || len(log.added) != 1 || log.added[0] != want {
		t.Errorf("mailer.send に %v、store.addNotification に %v（期待は、どちらも %v が一つ）", log.sent, log.added, want)
	}
	// 差し替えていない葉は、埋め込んだ実装のまま。葉も、内側の Ops から直に呼べる
	if order := ops.NewOrder(pkgtype.Session{UserId: "u1"}, nil); order.Id != "order-1" || order.UserId != "u1" {
		t.Errorf("NewOrder = %+v", order)
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
// nil を書いて渡すことはできる。そのときは、その実装を使う op を呼んだところで panic になる。
// 使わない op は呼べる（上の TestReplaceOneLeaf は、seams が nil のまま notify を呼んでいる）。
func TestNilIsNotChecked(t *testing.T) {
	t.Parallel()
	panics := func(name string, call func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: nil の実装に届く op を呼んでも panic にならない", name)
			}
		}()
		call()
	}
	f := &handlertest.Fixture{}
	// 葉: nil の leaves（内側の Ops から、葉を直に呼ぶ）
	panics("leaves", func() { inner.New(nil, nil, nil, nil).ToPublicUser(pkgtype.User{}) })
	// 継ぎ目: nil の seams。継ぎ目を持つ op（login）は、中身に進む前に、継ぎ目を呼ぶ
	panics("seams", func() {
		_, _ = server.New(f.Leaves(), f.Mailer(), registered{}, nil).Login(context.Background(), server.LoginInput{})
	})
}

// registered は、どの email にも、パスワードが handlertest.AlicePassword のユーザーがいる server.StoreOps。
// login が通る store の op（findUser と addSession）だけを書き、呼ばれた回数を数える。
type registered struct {
	server.StoreOps
	found, added *int
}

func (r registered) FindUser(_ context.Context, email pkgtype.Email) (*pkgtype.User, error) {
	if r.found != nil {
		*r.found++
	}
	user := handlertest.Alice()
	user.Email = email
	return &user, nil
}

func (r registered) AddSession(context.Context, pkgtype.Session) error {
	if r.added != nil {
		*r.added++
	}
	return nil
}

// TestLoginIsLimitedThroughOps は、@rate.limited が、HTTP のハンドラではなく op そのものに掛かること:
// server.New が返す Ops の Login を直に呼んでも、制限を超えれば rate.LimitExceeded が返り、中身（配線の本体）は呼ばれない。
// 継ぎ目は、基底が op の外側に出した口（server.Seams）で、go-http の出力とは関係がない。
func TestLoginIsLimitedThroughOps(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	var found, added int
	ops := server.New(f.Leaves(), f.Mailer(), registered{found: &found, added: &added}, server.NewRateSeams(f.Limiter()))
	ctx := context.Background()
	in := server.LoginInput{Body: server.Credentials{Email: handlertest.AliceEmail, Password: handlertest.AlicePassword}}

	// 制限まで: 中身が動き、セッションができる
	for i := range handlertest.LoginLimit {
		if _, err := ops.Login(ctx, in); err != nil {
			t.Fatalf("%d 回目の Login: %v", i+1, err)
		}
	}
	if found != handlertest.LoginLimit || added != handlertest.LoginLimit {
		t.Fatalf("store.findUser が %d 回、store.addSession が %d 回（期待は、どちらも %d 回）", found, added, handlertest.LoginLimit)
	}
	// 制限を超えた呼び出し: 仕様の例外 rate.LimitExceeded が値で返る。中身は呼ばれない（store のどの op も呼ばれない。
	// 仕様の `throws LimitExceeded prevents effects` を、継ぎ目の中身が守っている）
	out, err := ops.Login(ctx, in)
	var exceeded rate.LimitExceeded
	if !errors.As(err, &exceeded) {
		t.Fatalf("制限を超えた Login = %+v, %v（期待は rate.LimitExceeded）", out, err)
	}
	if out != (server.LoginOk{}) || found != handlertest.LoginLimit || added != handlertest.LoginLimit {
		t.Errorf("制限を超えた Login が中身を呼んだ: 戻り値 %+v、store.findUser %d 回、store.addSession %d 回", out, found, added)
	}
	// 時間の幅が過ぎれば、また呼べる
	f.Advance(handlertest.LoginWindow)
	if _, err := ops.Login(ctx, in); err != nil {
		t.Errorf("幅が過ぎた後の Login: %v", err)
	}
}
