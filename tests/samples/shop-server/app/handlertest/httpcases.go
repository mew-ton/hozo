package handlertest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/shop-server/app"
	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/servertest"
	pkgtype "example.com/shop-server/gen/type"
)

// このファイルは、HTTP の入口の、結果ごとのインテグレーションテスト（docs/plugin-go-http.md「インテグレーションテストの口」）。
// go-http が出した口（servertest.HTTPCases）を満たす型を書き、servertest.RunHTTP に渡す。要求の組み立て、送ること、結果の見分け
// （ステータスと、本文の形）は、生成したコードが持つ。ここに書くのは、入力の値と、今の状態の用意（Given）と、後の状態の確かめ（Then）だけ。
// 入口に、例外を一つ足せば、足りないメソッドで、この型はコンパイルできなくなる。
//
// RunHTTP は、サーバー一つを、すべてのケースで使う。ケースが、互いの状態に依らないように、状態を使うケースは、自分のユーザーを
// 足して使う（newUser）。

// RunCases は、newServer が作るサーバー一つに、八つの入口の、すべての結果のケースを走らせる。
// テストの名前は、<op の FQN>/<結果>（server.login/LimitExceeded）。
func RunCases(t *testing.T, newServer func(t *testing.T, f *Fixture) Server) {
	s := startServer(t, newServer)
	store, ok := s.Store.(CaseStore)
	if !ok {
		t.Fatalf("Server.Store（%T）が、CaseStore を満たさない", s.Store)
	}
	servertest.RunHTTP(t, s.Handler, HTTPCases{caseServer{s, store}})
}

// CaseStore は、ケースが、状態を用意し、確かめるのに使う store の op（Store に、三つ足したもの）。gen/store と gen/storemem の Ops の
// どちらも満たす。
type CaseStore interface {
	Store
	AddSession(ctx context.Context, session pkgtype.Session) error
	FindSession(ctx context.Context, token pkgtype.Token) (*pkgtype.Session, error)
	PlaceOrder(ctx context.Context, order pkgtype.Order, notification pkgtype.Notification) error
}

// caseServer は、ケースが使うサーバー: 試す相手と、その store。
type caseServer struct {
	Server
	Store CaseStore
}

// HTTPCases は、servertest.HTTPCases を満たす: 入口ごとに、結果ごとのケースを返す。
type HTTPCases struct{ s caseServer }

var _ servertest.HTTPCases = HTTPCases{}

// newUser は、ケース一つのためのユーザー name（識別子 "u-<name>"）と、そのセッションを足し、トークンを返す。配送は、届く側に戻す。
func (s caseServer) newUser(ctx context.Context, name string) (pkgtype.UserId, *pkgtype.Token, error) {
	s.fixture.FailDelivery(false)
	id, token := pkgtype.UserId("u-"+name), pkgtype.Token("session-"+name)
	user := pkgtype.User{Id: id, Email: pkgtype.Email(name + "@example.com"), Name: name, Hash: app.HashPassword(AlicePassword)}
	if err := s.Store.AddUser(ctx, user); err != nil {
		return "", nil, err
	}
	err := s.Store.AddSession(ctx, pkgtype.Session{Token: token, UserId: id, ExpiresAt: Now.Add(SessionTTL)})
	return id, &token, err
}

// newOrders は、ユーザー id の注文を n 件（識別子 "<id>-1"、"<id>-2"…。一分ずつ後の時刻）と、その通知の記録を足す。
func (s caseServer) newOrders(ctx context.Context, id pkgtype.UserId, n int) error {
	for i := 1; i <= n; i++ {
		order := pkgtype.Order{Id: pkgtype.OrderId(fmt.Sprintf("%s-%d", id, i)), UserId: id,
			Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: int64(i)}}, PlacedAt: Now.Add(time.Duration(i) * time.Minute)}
		if err := s.Store.PlaceOrder(ctx, order, pkgtype.Notification{UserId: id, Message: "placed"}); err != nil {
			return err
		}
	}
	return nil
}

// orders と notices は、ユーザー id の、注文の数と、通知の記録の数。
func (s caseServer) orders(t *testing.T, id pkgtype.UserId) int {
	t.Helper()
	orders, err := s.Store.ListOrders(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return len(orders)
}

func (s caseServer) notices(t *testing.T, id pkgtype.UserId) int {
	t.Helper()
	all, err := s.Notifications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, notice := range all {
		if notice.UserId == id {
			n++
		}
	}
	return n
}

// badRequest は、応答の本文の、http.BadRequest が運ぶ値。
func badRequest(t *testing.T, res *httptest.ResponseRecorder) (message string, paths []string) {
	t.Helper()
	var body struct {
		Value struct {
			Message    string `json:"message"`
			Violations []struct {
				Path string `json:"path"`
			} `json:"violations"`
		} `json:"value"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, v := range body.Value.Violations {
		paths = append(paths, v.Path)
	}
	return body.Value.Message, paths
}

// ---- server.login ----

type loginCases struct{ s caseServer }

func (h HTTPCases) Login() servertest.LoginHTTPCases { return loginCases{h.s} }

func (c loginCases) OK() servertest.LoginHTTPCase {
	return servertest.LoginHTTPCase{
		In: server.LoginInput{Body: server.Credentials{Email: AliceEmail, Password: AlicePassword}},
		Then: func(t *testing.T, got server.LoginOk, _ *httptest.ResponseRecorder) {
			if got.User.Id != AliceID || got.Token == "" {
				t.Errorf("login = %+v（期待は、ユーザー %s と、空でないトークン）", got, AliceID)
			}
			// 後の状態: そのトークンのセッションが、できている
			if session, err := c.s.Store.FindSession(context.Background(), got.Token); err != nil || session == nil || session.UserId != AliceID {
				t.Errorf("login の後のセッション = %+v, %v（期待は、ユーザー %s のもの）", session, err, AliceID)
			}
		},
	}
}

// 入力の値が、制約に合わない（validate = true）: 要求は、手直ししなくても、400 になる
func (c loginCases) BadRequest() servertest.LoginHTTPCase {
	return servertest.LoginHTTPCase{
		In: server.LoginInput{Body: server.Credentials{Email: AliceEmail, Password: "short"}},
		Then: func(t *testing.T, _ server.LoginOk, res *httptest.ResponseRecorder) {
			if message, paths := badRequest(t, res); message != "invalid input" || !reflect.DeepEqual(paths, []string{"body.password"}) {
				t.Errorf("message = %q, 違反の場所 = %v（期待は invalid input と body.password）", message, paths)
			}
		},
	}
}

func (c loginCases) InvalidCredentials() servertest.LoginHTTPCase {
	return servertest.LoginHTTPCase{In: server.LoginInput{Body: server.Credentials{Email: AliceEmail, Password: "wrong-password"}}}
}

// 今の状態: 幅の中で、もう LoginLimit 回、呼ばれている。正しいパスワードでも、op の中身は呼ばれない
func (c loginCases) LimitExceeded() servertest.LoginHTTPCase {
	return servertest.LoginHTTPCase{
		In: server.LoginInput{Body: server.Credentials{Email: AliceEmail, Password: AlicePassword}},
		Given: func(ctx context.Context) error {
			for range LoginLimit {
				body := strings.NewReader(`{"email":"alice@example.com","password":"wrong-password"}`)
				c.s.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/login", body).WithContext(ctx))
			}
			return nil
		},
		Then: func(t *testing.T, _ server.LoginOk, res *httptest.ResponseRecorder) {
			if got := res.Body.String(); got != `{"error":"rate.LimitExceeded"}` {
				t.Errorf("本文 = %s", got)
			}
		},
	}
}

// ---- server.logout ----

type logoutCases struct{ s caseServer }

func (h HTTPCases) Logout() servertest.LogoutHTTPCases { return logoutCases{h.s} }

func (c logoutCases) OK() servertest.LogoutHTTPCase {
	token := pkgtype.Token("session-logout")
	return servertest.LogoutHTTPCase{
		In: server.AuthInput{Token: &token},
		Given: func(ctx context.Context) error {
			_, _, err := c.s.newUser(ctx, "logout")
			return err
		},
		Then: func(t *testing.T, _ *httptest.ResponseRecorder) {
			// 後の状態: セッションが、消えている
			if session, err := c.s.Store.FindSession(context.Background(), token); err != nil || session != nil {
				t.Errorf("logout の後のセッション = %+v, %v（期待は、ない）", session, err)
			}
		},
	}
}

// トークンのない要求
func (c logoutCases) Unauthorized() servertest.LogoutHTTPCase { return servertest.LogoutHTTPCase{} }

// ---- server.getMe ----

type getMeCases struct{ s caseServer }

func (h HTTPCases) GetMe() servertest.GetMeHTTPCases { return getMeCases{h.s} }

func (c getMeCases) OK() servertest.GetMeHTTPCase {
	token := pkgtype.Token("session-me")
	return servertest.GetMeHTTPCase{
		In: server.AuthInput{Token: &token},
		Given: func(ctx context.Context) error {
			_, _, err := c.s.newUser(ctx, "me")
			return err
		},
		Then: func(t *testing.T, got pkgtype.PublicUser, _ *httptest.ResponseRecorder) {
			if want := (pkgtype.PublicUser{Id: "u-me", Email: "me@example.com", Name: "me"}); got != want {
				t.Errorf("getMe = %+v（期待は %+v）", got, want)
			}
		},
	}
}

// 知らないトークン
func (c getMeCases) Unauthorized() servertest.GetMeHTTPCase {
	token := pkgtype.Token("no-such-token")
	return servertest.GetMeHTTPCase{In: server.AuthInput{Token: &token}}
}

// ---- server.getOrders ----

type getOrdersCases struct{ s caseServer }

func (h HTTPCases) GetOrders() servertest.GetOrdersHTTPCases { return getOrdersCases{h.s} }

// 三件の注文の、二ページ目（1 ページは PageSize 件）
func (c getOrdersCases) OK() servertest.GetOrdersHTTPCase {
	token := pkgtype.Token("session-orders")
	return servertest.GetOrdersHTTPCase{
		In: server.GetOrdersInput{Token: &token, Page: 2},
		Given: func(ctx context.Context) error {
			id, _, err := c.s.newUser(ctx, "orders")
			if err != nil {
				return err
			}
			return c.s.newOrders(ctx, id, 3)
		},
		Then: func(t *testing.T, got []pkgtype.Order, _ *httptest.ResponseRecorder) {
			if len(got) != 1 || got[0].Id != "u-orders-3" {
				t.Errorf("getOrders の 2 ページ目 = %+v（期待は、注文 u-orders-3 だけ）", got)
			}
		},
	}
}

func (c getOrdersCases) Unauthorized() servertest.GetOrdersHTTPCase {
	return servertest.GetOrdersHTTPCase{In: server.GetOrdersInput{Page: 1}}
}

// ---- server.getLatestOrder ----

type getLatestOrderCases struct{ s caseServer }

func (h HTTPCases) GetLatestOrder() servertest.GetLatestOrderHTTPCases {
	return getLatestOrderCases{h.s}
}

func (c getLatestOrderCases) OK() servertest.GetLatestOrderHTTPCase {
	token := pkgtype.Token("session-latest")
	return servertest.GetLatestOrderHTTPCase{
		In: server.AuthInput{Token: &token},
		Given: func(ctx context.Context) error {
			id, _, err := c.s.newUser(ctx, "latest")
			if err != nil {
				return err
			}
			return c.s.newOrders(ctx, id, 2)
		},
		Then: func(t *testing.T, got *pkgtype.Order, _ *httptest.ResponseRecorder) {
			if got == nil || got.Id != "u-latest-2" {
				t.Errorf("getLatestOrder = %+v（期待は、注文 u-latest-2）", got)
			}
		},
	}
}

func (c getLatestOrderCases) Unauthorized() servertest.GetLatestOrderHTTPCase {
	return servertest.GetLatestOrderHTTPCase{}
}

// ---- server.getMyPage ----

type getMyPageCases struct{ s caseServer }

func (h HTTPCases) GetMyPage() servertest.GetMyPageHTTPCases { return getMyPageCases{h.s} }

func (c getMyPageCases) OK() servertest.GetMyPageHTTPCase {
	token := pkgtype.Token("session-page")
	return servertest.GetMyPageHTTPCase{
		In: server.AuthInput{Token: &token},
		Given: func(ctx context.Context) error {
			id, _, err := c.s.newUser(ctx, "page")
			if err != nil {
				return err
			}
			return c.s.newOrders(ctx, id, 2)
		},
		Then: func(t *testing.T, got server.MyPage, _ *httptest.ResponseRecorder) {
			if got.User.Id != "u-page" || got.Latest == nil || got.Latest.Id != "u-page-2" || len(got.Summaries) != 2 {
				t.Errorf("getMyPage = %+v（期待は、ユーザー u-page、いちばん新しい注文 u-page-2、要約が二つ）", got)
			}
		},
	}
}

func (c getMyPageCases) Unauthorized() servertest.GetMyPageHTTPCase {
	return servertest.GetMyPageHTTPCase{}
}

// ---- server.placeOrder ----

type placeOrderCases struct{ s caseServer }

func (h HTTPCases) PlaceOrder() servertest.PlaceOrderHTTPCases { return placeOrderCases{h.s} }

func (c placeOrderCases) OK() servertest.PlaceOrderHTTPCase {
	token := pkgtype.Token("session-place")
	return servertest.PlaceOrderHTTPCase{
		In: server.PlaceOrderInput{Token: &token, Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}},
		Given: func(ctx context.Context) error {
			_, _, err := c.s.newUser(ctx, "place")
			return err
		},
		Then: func(t *testing.T, got pkgtype.Order, _ *httptest.ResponseRecorder) {
			if got.UserId != "u-place" || len(got.Lines) != 1 || got.Lines[0].Item != "apple" {
				t.Errorf("placeOrder = %+v（期待は、ユーザー u-place の、apple の一行）", got)
			}
			// 後の状態: 注文と、その通知の記録が、一つずつ
			if orders, notices := c.s.orders(t, "u-place"), c.s.notices(t, "u-place"); orders != 1 || notices != 1 {
				t.Errorf("placeOrder の後の注文は %d、通知は %d（期待は 1 と 1）", orders, notices)
			}
		},
	}
}

// 入力の値では作れない要求: @http.required の項目 lines のない本文。Request で、組み立てた本文を、置き換える
func (c placeOrderCases) BadRequest() servertest.PlaceOrderHTTPCase {
	token := pkgtype.Token("session-refused")
	return servertest.PlaceOrderHTTPCase{
		In: server.PlaceOrderInput{Token: &token},
		Given: func(ctx context.Context) error {
			_, _, err := c.s.newUser(ctx, "refused")
			return err
		},
		Request: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader(`{}`)) },
		Then: func(t *testing.T, _ pkgtype.Order, res *httptest.ResponseRecorder) {
			if message, _ := badRequest(t, res); message != "body lines: missing" {
				t.Errorf("message = %q（期待は body lines: missing）", message)
			}
			// 後の状態: op は、呼ばれていない
			if orders := c.s.orders(t, "u-refused"); orders != 0 {
				t.Errorf("断られた placeOrder の後の注文は %d（期待は 0）", orders)
			}
		},
	}
}

func (c placeOrderCases) Unauthorized() servertest.PlaceOrderHTTPCase {
	return servertest.PlaceOrderHTTPCase{In: server.PlaceOrderInput{Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}}}
}

// 今の状態: 通知を、届けられない。type.NotificationFailed は @http.status を持たないので、応答は 500 と internal。
// 仕様では、注文と通知の記録は、配送の前に、一緒に残る
func (c placeOrderCases) NotificationFailed() servertest.PlaceOrderHTTPCase {
	token := pkgtype.Token("session-undelivered")
	return servertest.PlaceOrderHTTPCase{
		In: server.PlaceOrderInput{Token: &token, Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}},
		Given: func(ctx context.Context) error {
			_, _, err := c.s.newUser(ctx, "undelivered")
			c.s.fixture.FailDelivery(true)
			return err
		},
		Then: func(t *testing.T, _ pkgtype.Order, _ *httptest.ResponseRecorder) {
			c.s.fixture.FailDelivery(false)
			if orders, notices := c.s.orders(t, "u-undelivered"), c.s.notices(t, "u-undelivered"); orders != 1 || notices != 1 {
				t.Errorf("配送に失敗した placeOrder の後の注文は %d、通知は %d（期待は 1 と 1）", orders, notices)
			}
		},
	}
}

// ---- server.resendNotices ----

type resendNoticesCases struct{ s caseServer }

func (h HTTPCases) ResendNotices() servertest.ResendNoticesHTTPCases { return resendNoticesCases{h.s} }

func (c resendNoticesCases) OK() servertest.ResendNoticesHTTPCase {
	token := pkgtype.Token("session-resend")
	return servertest.ResendNoticesHTTPCase{
		In: server.AuthInput{Token: &token},
		Given: func(ctx context.Context) error {
			id, _, err := c.s.newUser(ctx, "resend")
			if err != nil {
				return err
			}
			return c.s.newOrders(ctx, id, 2)
		},
		Then: func(t *testing.T, got []pkgtype.Notification, _ *httptest.ResponseRecorder) {
			if len(got) != 2 {
				t.Errorf("resendNotices = %+v（期待は、通知が二つ）", got)
			}
			// 後の状態: 注文のときの記録二つに、送り直した記録が二つ、足されている
			if notices := c.s.notices(t, "u-resend"); notices != 4 {
				t.Errorf("resendNotices の後の通知は %d（期待は 4）", notices)
			}
		},
	}
}

func (c resendNoticesCases) Unauthorized() servertest.ResendNoticesHTTPCase {
	return servertest.ResendNoticesHTTPCase{}
}

// 今の状態: 通知を、届けられない。一つ目で止まるので、送り直した記録は、足されない
func (c resendNoticesCases) NotificationFailed() servertest.ResendNoticesHTTPCase {
	token := pkgtype.Token("session-unsent")
	return servertest.ResendNoticesHTTPCase{
		In: server.AuthInput{Token: &token},
		Given: func(ctx context.Context) error {
			id, _, err := c.s.newUser(ctx, "unsent")
			if err != nil {
				return err
			}
			c.s.fixture.FailDelivery(true)
			return c.s.newOrders(ctx, id, 1)
		},
		Then: func(t *testing.T, _ []pkgtype.Notification, _ *httptest.ResponseRecorder) {
			c.s.fixture.FailDelivery(false)
			if notices := c.s.notices(t, "u-unsent"); notices != 1 {
				t.Errorf("配送に失敗した resendNotices の後の通知は %d（期待は、注文のときの 1）", notices)
			}
		},
	}
}

// RunIncomplete は、ケースを欠いた型が、口を満たさないこと。欠いた型を渡す行（servertest.RunHTTP(t, handler, <入口を一つ欠いた型>)）は、
// コンパイルできないので、ここには書けない。代わりに、口が求めるメソッドを、リフレクションで確かめる: HTTPCases は、@http.* の付いた
// op ごとに一つ。<Op>HTTPCases は、結果ごとに一つ。コンパイルできないことそのものは、リポジトリの plugins/go-http の
// TestIncompleteHTTPCasesDoNotCompile が、生成したコードをコンパイルして確かめる。
func RunIncomplete(t *testing.T) {
	cases := reflect.TypeOf((*servertest.HTTPCases)(nil)).Elem()
	methods := func(typ reflect.Type) []string {
		var names []string
		for i := 0; i < typ.NumMethod(); i++ {
			names = append(names, typ.Method(i).Name)
		}
		return names
	}
	// 入口ごと（reflect は、名前の順に並べる）
	entries := []string{"GetLatestOrder", "GetMe", "GetMyPage", "GetOrders", "Login", "Logout", "PlaceOrder", "ResendNotices"}
	if got := methods(cases); !reflect.DeepEqual(got, entries) {
		t.Errorf("HTTPCases のメソッド = %v, want %v", got, entries)
	}
	if !reflect.TypeOf(HTTPCases{}).Implements(cases) {
		t.Error("すべての入口のケースを持つ型が、HTTPCases を満たさない")
	}
	if reflect.TypeOf((*withoutResendNotices)(nil)).Elem().Implements(cases) {
		t.Error("resendNotices のケースを欠いた口の型が、HTTPCases を満たす")
	}
	// 結果ごと: 正常、op を呼ぶ前の失敗（本文か、@http.required を持つ入力）、op が投げうる例外（デコレータが運ぶものを含む）
	for _, c := range []struct {
		entry reflect.Type
		want  []string
	}{
		{reflect.TypeOf((*servertest.LoginHTTPCases)(nil)).Elem(), []string{"BadRequest", "InvalidCredentials", "LimitExceeded", "OK"}},
		{reflect.TypeOf((*servertest.LogoutHTTPCases)(nil)).Elem(), []string{"OK", "Unauthorized"}},
		{reflect.TypeOf((*servertest.GetOrdersHTTPCases)(nil)).Elem(), []string{"OK", "Unauthorized"}},
		{reflect.TypeOf((*servertest.PlaceOrderHTTPCases)(nil)).Elem(), []string{"BadRequest", "NotificationFailed", "OK", "Unauthorized"}},
		{reflect.TypeOf((*servertest.ResendNoticesHTTPCases)(nil)).Elem(), []string{"NotificationFailed", "OK", "Unauthorized"}},
	} {
		if got := methods(c.entry); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s のメソッド = %v, want %v", c.entry.Name(), got, c.want)
		}
	}
	results := reflect.TypeOf((*servertest.LoginHTTPCases)(nil)).Elem()
	if reflect.TypeOf(withoutLimitExceeded{}).Implements(results) {
		t.Error("LimitExceeded のケースを欠いた型が、LoginHTTPCases を満たす")
	}
	if !reflect.TypeOf(loginCases{}).Implements(results) {
		t.Error("四つの結果のケースを持つ型が、LoginHTTPCases を満たさない")
	}
}

// withoutResendNotices は、HTTPCases から、入口 resendNotices のケースを欠いた口。
type withoutResendNotices interface {
	Login() servertest.LoginHTTPCases
	Logout() servertest.LogoutHTTPCases
	GetMe() servertest.GetMeHTTPCases
	GetOrders() servertest.GetOrdersHTTPCases
	GetLatestOrder() servertest.GetLatestOrderHTTPCases
	GetMyPage() servertest.GetMyPageHTTPCases
	PlaceOrder() servertest.PlaceOrderHTTPCases
}

// withoutLimitExceeded は、入口 login の結果のうち、デコレータ @rate.limited が運ぶ例外のケースを欠いた型。
type withoutLimitExceeded struct{}

func (withoutLimitExceeded) OK() servertest.LoginHTTPCase         { return servertest.LoginHTTPCase{} }
func (withoutLimitExceeded) BadRequest() servertest.LoginHTTPCase { return servertest.LoginHTTPCase{} }
func (withoutLimitExceeded) InvalidCredentials() servertest.LoginHTTPCase {
	return servertest.LoginHTTPCase{}
}
