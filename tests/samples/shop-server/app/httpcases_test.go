package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/servertest"
	pkgtype "example.com/shop-server/gen/type"
)

// このファイルは、HTTP の入口の、インテグレーションテスト（docs/plugin-go-http.md「インテグレーションテストの口」）。
// go-http が出した口（servertest.HTTPCases）を満たす型を書き、servertest.RunHTTP に渡す。流れの組み立て（にせものの上の、
// server と store の配線）、要求の組み立て、送ること、結果の見分け（ステータスと、本文の形）は、生成したコードが持つ。
// ここに書くのは、入力の値と、流れの端が返す値（Given）と、足りない確かめ（Then）だけ。DB は、要らない。
//
// 書くケースは、正常と、op を呼ぶ前の失敗と、Hozo が作れない例外のケース（入口の入力で決まるもの、繰り返しの中のもの）。
// ほかの例外のケース（葉が投げる、端が投げる、端が「ない」を返す、継ぎ目が投げる）は、RunHTTP が、正常のケースの用意から、作る。

// httpCases は、servertest.HTTPCases を満たす: 入口ごとに、ケースを返す。
type httpCases struct{}

var _ servertest.HTTPCases = httpCases{}

// TestHTTPCases は、八つの入口の、すべてのテストを走らせる。テストの名前は、<op の FQN>/<結果>。例外は、出どころごとに、
// <op の FQN>/<例外>/<出どころ>（TestHTTPCases/server.placeOrder/Unauthorized/server.sessionOf.LoggedOut など）。
func TestHTTPCases(t *testing.T) {
	servertest.RunHTTP(t, fakeLeaves(), httpCases{})
}

// loggedIn は、leafSession と、そのユーザー leafAlice が、いることを、にせものに言う。
func loggedIn(f *servertest.Fakes) {
	session, alice := leafSession, leafAlice
	f.Store.Sessions.SessionByToken.Returns(&session)
	f.Store.Users.UserById.Returns(&alice)
}

// sessionToken は、leafSession のトークン。
func sessionToken() *pkgtype.Token {
	token := leafSession.Token
	return &token
}

// twoOrders は、leafAlice の注文、二件。
func twoOrders() []pkgtype.Order {
	return []pkgtype.Order{
		{Id: "o1", UserId: "u1", Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 1}}},
		{Id: "o2", UserId: "u1", Lines: []pkgtype.OrderLine{{Item: "pear", Quantity: 2}, {Item: "fig", Quantity: 1}}},
	}
}

// badRequestBody は、応答の本文の、http.BadRequest が運ぶ値。
func badRequestBody(t *testing.T, res *httptest.ResponseRecorder) (message string, paths []string) {
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

type loginCases struct{}

func (httpCases) Login() servertest.LoginHTTPCases { return loginCases{} }

func (loginCases) OK() servertest.LoginHTTPCase {
	return servertest.LoginHTTPCase{
		In: server.LoginInput{Body: server.Credentials{Email: "alice@example.com", Password: "password123"}},
		Given: func(f *servertest.Fakes) {
			alice := leafAlice
			f.Store.Users.UserByEmail.Returns(&alice)
		},
		Then: func(t *testing.T, got server.LoginOk, _ *httptest.ResponseRecorder, f *servertest.Fakes) {
			if got.Token != "token-1" || got.User.Id != "u1" {
				t.Errorf("login = %+v（期待は、ユーザー u1 と、トークン token-1）", got)
			}
			if calls := f.Store.Sessions.CreateSession.Calls; len(calls) != 1 || calls[0].Session.Token != "token-1" {
				t.Errorf("createSession の呼び出し = %+v（期待は、トークン token-1 のセッションで、一度）", calls)
			}
		},
	}
}

func (loginCases) BadRequest() servertest.LoginHTTPCase {
	return servertest.LoginHTTPCase{
		In: server.LoginInput{Body: server.Credentials{Email: "alice@example.com", Password: "short"}},
		Then: func(t *testing.T, _ server.LoginOk, res *httptest.ResponseRecorder, f *servertest.Fakes) {
			if message, paths := badRequestBody(t, res); message != "invalid input" || !reflect.DeepEqual(paths, []string{"body.password"}) {
				t.Errorf("message = %q, 違反の場所 = %v（期待は invalid input と body.password）", message, paths)
			}
			if n := len(f.Store.Users.UserByEmail.Calls); n != 0 {
				t.Errorf("userByEmail の呼び出し = %d 回（期待は、なし: op は、呼ばれていない）", n)
			}
		},
	}
}

// ---- server.logout ----

type logoutCases struct{}

func (httpCases) Logout() servertest.LogoutHTTPCases { return logoutCases{} }

func (logoutCases) OK() servertest.LogoutHTTPCase {
	return servertest.LogoutHTTPCase{
		In:    server.AuthInput{Token: sessionToken()},
		Given: loggedIn,
		Then: func(t *testing.T, _ *httptest.ResponseRecorder, f *servertest.Fakes) {
			if calls := f.Store.Sessions.DeleteSession.Calls; len(calls) != 1 || calls[0].Token != leafSession.Token {
				t.Errorf("deleteSession の呼び出し = %+v（期待は、トークン %s で、一度）", calls, leafSession.Token)
			}
		},
	}
}

// UnauthorizedAuthenticateAbsent: トークンを送らない（入口の入力で決まるので、人が書く）。
func (logoutCases) UnauthorizedAuthenticateAbsent() servertest.LogoutHTTPCase {
	return servertest.LogoutHTTPCase{
		Then: func(t *testing.T, _ *httptest.ResponseRecorder, f *servertest.Fakes) {
			if n := len(f.Store.Sessions.SessionByToken.Calls); n != 0 {
				t.Errorf("sessionByToken の呼び出し = %d 回（期待は、なし: トークンが、ない）", n)
			}
		},
	}
}

// ---- server.getMe ----

type getMeCases struct{}

func (httpCases) GetMe() servertest.GetMeHTTPCases { return getMeCases{} }

func (getMeCases) OK() servertest.GetMeHTTPCase {
	return servertest.GetMeHTTPCase{
		In:    server.AuthInput{Token: sessionToken()},
		Given: loggedIn,
		Then: func(t *testing.T, got pkgtype.PublicUser, _ *httptest.ResponseRecorder, _ *servertest.Fakes) {
			if got.Id != "u1" || got.Name != "Alice" {
				t.Errorf("getMe = %+v（期待は、u1 の Alice）", got)
			}
		},
	}
}

func (getMeCases) UnauthorizedAuthenticateAbsent() servertest.GetMeHTTPCase {
	return servertest.GetMeHTTPCase{}
}

// ---- server.getOrders ----

type getOrdersCases struct{}

func (httpCases) GetOrders() servertest.GetOrdersHTTPCases { return getOrdersCases{} }

func (getOrdersCases) OK() servertest.GetOrdersHTTPCase {
	return servertest.GetOrdersHTTPCase{
		In: server.GetOrdersInput{Token: sessionToken(), Page: 1},
		Given: func(f *servertest.Fakes) {
			loggedIn(f)
			f.Store.Orders.OrdersOfUser.Returns(twoOrders())
		},
		Then: func(t *testing.T, got []pkgtype.Order, _ *httptest.ResponseRecorder, f *servertest.Fakes) {
			if len(got) != 2 || got[0].Id != "o1" {
				t.Errorf("getOrders = %+v（期待は、一ページ目の二件）", got)
			}
			if calls := f.Store.Orders.OrdersOfUser.Calls; len(calls) != 1 || calls[0].UserId != "u1" {
				t.Errorf("ordersOfUser の呼び出し = %+v（期待は、ユーザー u1 で、一度）", calls)
			}
		},
	}
}

func (getOrdersCases) UnauthorizedAuthenticateAbsent() servertest.GetOrdersHTTPCase {
	return servertest.GetOrdersHTTPCase{In: server.GetOrdersInput{Page: 1}}
}

// ---- server.getLatestOrder ----

type getLatestOrderCases struct{}

func (httpCases) GetLatestOrder() servertest.GetLatestOrderHTTPCases { return getLatestOrderCases{} }

func (getLatestOrderCases) OK() servertest.GetLatestOrderHTTPCase {
	return servertest.GetLatestOrderHTTPCase{
		In: server.AuthInput{Token: sessionToken()},
		Given: func(f *servertest.Fakes) {
			loggedIn(f)
			latest := twoOrders()[1]
			f.Store.Orders.LatestOrderOf.Returns(&latest)
		},
		Then: func(t *testing.T, got *pkgtype.Order, _ *httptest.ResponseRecorder, _ *servertest.Fakes) {
			if got == nil || got.Id != "o2" {
				t.Errorf("getLatestOrder = %+v（期待は、注文 o2）", got)
			}
		},
	}
}

func (getLatestOrderCases) UnauthorizedAuthenticateAbsent() servertest.GetLatestOrderHTTPCase {
	return servertest.GetLatestOrderHTTPCase{}
}

// ---- server.getMyPage ----

type getMyPageCases struct{}

func (httpCases) GetMyPage() servertest.GetMyPageHTTPCases { return getMyPageCases{} }

func (getMyPageCases) OK() servertest.GetMyPageHTTPCase {
	return servertest.GetMyPageHTTPCase{
		In: server.AuthInput{Token: sessionToken()},
		Given: func(f *servertest.Fakes) {
			loggedIn(f)
			orders := twoOrders()
			f.Store.Orders.LatestOrderOf.Returns(&orders[1])
			f.Store.Orders.OrdersOfUser.Returns(orders)
		},
		Then: func(t *testing.T, got server.MyPage, _ *httptest.ResponseRecorder, f *servertest.Fakes) {
			if got.User.Id != "u1" || got.Latest == nil || got.Latest.Id != "o2" || len(got.Summaries) != 2 {
				t.Errorf("getMyPage = %+v（期待は、u1 と、いちばん新しい注文 o2 と、二件の要約）", got)
			}
			if n := len(f.Leaves.Summarize.Calls); n != 2 {
				t.Errorf("葉 summarize の呼び出し = %d 回（期待は、注文ごとに、二度）", n)
			}
		},
	}
}

func (getMyPageCases) UnauthorizedAuthenticateAbsent() servertest.GetMyPageHTTPCase {
	return servertest.GetMyPageHTTPCase{}
}

// ---- server.placeOrder ----

type placeOrderCases struct{}

func (httpCases) PlaceOrder() servertest.PlaceOrderHTTPCases { return placeOrderCases{} }

func (placeOrderCases) OK() servertest.PlaceOrderHTTPCase {
	return servertest.PlaceOrderHTTPCase{
		In:    server.PlaceOrderInput{Token: sessionToken(), Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}},
		Given: loggedIn,
		Then: func(t *testing.T, got pkgtype.Order, _ *httptest.ResponseRecorder, f *servertest.Fakes) {
			if got.Id != "order-1" || got.UserId != "u1" || len(got.Lines) != 1 || got.Lines[0].Item != "apple" {
				t.Errorf("placeOrder = %+v（期待は、ユーザー u1 の、apple の一行の、注文 order-1）", got)
			}
			// どの効果が、何の値で呼ばれたか: 注文と、その通知の記録が、一つずつ。配送も、一度
			if calls := f.Store.Orders.CreateOrder.Calls; len(calls) != 1 || calls[0].Order.Id != "order-1" {
				t.Errorf("createOrder の呼び出し = %+v（期待は、注文 order-1 で、一度）", calls)
			}
			if calls := f.Store.Notifications.NotifyUser.Calls; len(calls) != 1 || calls[0].Notification.UserId != "u1" {
				t.Errorf("notifyUser の呼び出し = %+v（期待は、ユーザー u1 への通知で、一度）", calls)
			}
			if n := len(f.Mailer.Send.Calls); n != 1 {
				t.Errorf("mailer.send の呼び出し = %d 回（期待は、一度）", n)
			}
		},
	}
}

func (placeOrderCases) BadRequest() servertest.PlaceOrderHTTPCase {
	return servertest.PlaceOrderHTTPCase{
		In:      server.PlaceOrderInput{Token: sessionToken()},
		Given:   loggedIn,
		Request: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader(`{}`)) },
		Then: func(t *testing.T, _ pkgtype.Order, res *httptest.ResponseRecorder, f *servertest.Fakes) {
			if message, _ := badRequestBody(t, res); message != "body lines: missing" {
				t.Errorf("message = %q（期待は body lines: missing）", message)
			}
			if n := len(f.Store.Orders.CreateOrder.Calls); n != 0 {
				t.Errorf("createOrder の呼び出し = %d 回（期待は、なし: op は、呼ばれていない）", n)
			}
		},
	}
}

func (placeOrderCases) UnauthorizedAuthenticateAbsent() servertest.PlaceOrderHTTPCase {
	return servertest.PlaceOrderHTTPCase{
		In: server.PlaceOrderInput{Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}},
		Then: func(t *testing.T, _ pkgtype.Order, _ *httptest.ResponseRecorder, f *servertest.Fakes) {
			if n := len(f.Store.Orders.CreateOrder.Calls); n != 0 {
				t.Errorf("createOrder の呼び出し = %d 回（期待は、なし）", n)
			}
		},
	}
}

// ---- server.resendNotices ----

type resendNoticesCases struct{}

func (httpCases) ResendNotices() servertest.ResendNoticesHTTPCases { return resendNoticesCases{} }

func (resendNoticesCases) OK() servertest.ResendNoticesHTTPCase {
	return servertest.ResendNoticesHTTPCase{
		In: server.AuthInput{Token: sessionToken()},
		Given: func(f *servertest.Fakes) {
			loggedIn(f)
			f.Store.Orders.OrdersOfUser.Returns(twoOrders())
		},
		Then: func(t *testing.T, got []pkgtype.Notification, _ *httptest.ResponseRecorder, f *servertest.Fakes) {
			if len(got) != 2 {
				t.Errorf("resendNotices = %+v（期待は、注文ごとに、二件）", got)
			}
			if n := len(f.Store.Notifications.NotifyUser.Calls); n != 2 {
				t.Errorf("notifyUser の呼び出し = %d 回（期待は、送れた分の記録が、二度）", n)
			}
		},
	}
}

func (resendNoticesCases) UnauthorizedAuthenticateAbsent() servertest.ResendNoticesHTTPCase {
	return servertest.ResendNoticesHTTPCase{}
}

// NotificationFailedSend: 配送（mailer.send）は、繰り返しの中にあるので、人が書く: 注文が、一件はあって、配送が、失敗する。
func (resendNoticesCases) NotificationFailedSend() servertest.ResendNoticesHTTPCase {
	return servertest.ResendNoticesHTTPCase{
		In: server.AuthInput{Token: sessionToken()},
		Given: func(f *servertest.Fakes) {
			loggedIn(f)
			f.Store.Orders.OrdersOfUser.Returns(twoOrders())
			f.Mailer.Send.Throws(pkgtype.NotificationFailed{})
		},
		Then: func(t *testing.T, _ []pkgtype.Notification, _ *httptest.ResponseRecorder, f *servertest.Fakes) {
			if n := len(f.Mailer.Send.Calls); n != 1 {
				t.Errorf("mailer.send の呼び出し = %d 回（期待は、一度: 一つ失敗したら、そこで止まる）", n)
			}
			if n := len(f.Store.Notifications.NotifyUser.Calls); n != 0 {
				t.Errorf("notifyUser の呼び出し = %d 回（期待は、なし: 送れた分は、ない）", n)
			}
		},
	}
}

// ---- 口の形 ----

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

// withoutBadRequest は、入口 login のケースのうち、op を呼ぶ前の失敗を欠いた型。
type withoutBadRequest struct{}

func (withoutBadRequest) OK() servertest.LoginHTTPCase { return servertest.LoginHTTPCase{} }

// TestIncompleteHTTPCasesAreRejected は、ケースを欠いた型が、口を満たさないこと。欠いた型を渡す行（servertest.RunHTTP(t, leaves,
// <入口を一つ欠いた型>)）は、コンパイルできないので、ここには書けない。代わりに、口が求めるメソッドを、リフレクションで確かめる:
// HTTPCases は、@http.* の付いた op ごとに一つ。<Op>HTTPCases は、人か AI が書くケースごとに一つ（正常、op を呼ぶ前の失敗、
// Hozo が作れない例外の出どころ）。Hozo が作れる出どころ（login の InvalidCredentials と LimitExceeded など）は、口にない。
// コンパイルできないことそのものは、リポジトリの plugins/go-http の TestIncompleteHTTPCasesDoNotCompile が、確かめる。
func TestIncompleteHTTPCasesAreRejected(t *testing.T) {
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
	if reflect.TypeOf((*withoutResendNotices)(nil)).Elem().Implements(cases) {
		t.Error("resendNotices のケースを欠いた口の型が、HTTPCases を満たす")
	}
	for _, c := range []struct {
		entry reflect.Type
		want  []string
	}{
		{reflect.TypeOf((*servertest.LoginHTTPCases)(nil)).Elem(), []string{"BadRequest", "OK"}},
		{reflect.TypeOf((*servertest.LogoutHTTPCases)(nil)).Elem(), []string{"OK", "UnauthorizedAuthenticateAbsent"}},
		{reflect.TypeOf((*servertest.GetMeHTTPCases)(nil)).Elem(), []string{"OK", "UnauthorizedAuthenticateAbsent"}},
		{reflect.TypeOf((*servertest.PlaceOrderHTTPCases)(nil)).Elem(), []string{"BadRequest", "OK", "UnauthorizedAuthenticateAbsent"}},
		{reflect.TypeOf((*servertest.ResendNoticesHTTPCases)(nil)).Elem(), []string{"NotificationFailedSend", "OK", "UnauthorizedAuthenticateAbsent"}},
	} {
		if got := methods(c.entry); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s のメソッド = %v, want %v", c.entry.Name(), got, c.want)
		}
	}
	results := reflect.TypeOf((*servertest.LoginHTTPCases)(nil)).Elem()
	if reflect.TypeOf(withoutBadRequest{}).Implements(results) {
		t.Error("BadRequest のケースを欠いた型が、LoginHTTPCases を満たす")
	}
	if !reflect.TypeOf(loginCases{}).Implements(results) {
		t.Error("二つのケースを持つ型が、LoginHTTPCases を満たさない")
	}
}
