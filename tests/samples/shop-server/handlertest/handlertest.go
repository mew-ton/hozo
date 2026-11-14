// Package handlertest は、サンプル shop-server のハンドラ（go-http の出力 server.NewHandler）の振る舞いのテスト。
// 人が書いたもので、「両方」の組（both/app。状態は SQLite）と「go-http だけ」の組（httponly/app。状態はメモリ）が
// 同じものを走らせる: 状態の持ち方が違っても、HTTP から見た振る舞いは同じ。
//
// 応答の本文は文字列のまま比べる。ここに書いた JSON が、このサーバーを呼ぶクライアント（TS のトランスパイラなど）が
// 合わせる形になる（../README.md「分かったこと」の JSON の形）。
package handlertest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 作るサーバーの決まり。Run に渡す関数は、この通りのサーバーを返す。
const (
	// はじめからいる、ただ一人のユーザー。
	AliceID       = "u1"
	AliceEmail    = "alice@example.com"
	AliceName     = "Alice"
	AlicePassword = "password123"
	// 注文一覧の 1 ページの件数。
	PageSize = 2
	// セッションの有効期間。
	SessionTTL = time.Hour
)

// Now は止まった時計のはじめの時刻。
var Now = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// Server は試す相手。トークンは "token-1"、"token-2"…、注文の識別子は "order-1"、"order-2"… の順に作る。
type Server struct {
	Handler http.Handler
	// Advance は時計を d だけ進める。
	Advance func(d time.Duration)
	// FailDelivery は、以降の通知を届けられなくする（true）か、元に戻す（false）。
	FailDelivery func(fail bool)
	// Orders と Notifications は、その状態の行の数。
	Orders        func() int
	Notifications func() int
}

// Run は newServer が作るサーバーの振る舞いを確かめる。サブテストごとに新しいサーバーを作る。
// 状態はパッケージの変数（server.States）にあるので、サブテストは並列にしない。
func Run(t *testing.T, newServer func(t *testing.T) Server) {
	t.Run("login", func(t *testing.T) {
		s := newServer(t)
		// パスワードが違う、ユーザーがいない: どちらも 401 server.InvalidCredentials
		s.post(t, "/login", "", `{"email":"alice@example.com","password":"wrong-password"}`).
			want(t, 401, `{"error":"server.InvalidCredentials"}`)
		s.post(t, "/login", "", `{"email":"bob@example.com","password":"password123"}`).
			want(t, 401, `{"error":"server.InvalidCredentials"}`)
		// 入力が制約に合わない: 400 server.InvalidInput。違反は例外の値で運ぶ
		s.post(t, "/login", "", `{"email":"not-an-email","password":"short"}`).
			want(t, 400, `{"error":"server.InvalidInput","value":{"items":[`+
				`{"field":"body.email","constraint":"format"},{"field":"body.password","constraint":"length"}]}}`)
		// 本文が JSON でない、型が違う: op を呼ぶ前の 400 http.BadRequest
		s.post(t, "/login", "", `{"email":`).
			want(t, 400, `{"error":"http.BadRequest","message":"body: invalid JSON"}`)
		s.post(t, "/login", "", `{"email":1}`).
			want(t, 400, `{"error":"http.BadRequest","message":"body: invalid JSON"}`)
		// 成功: ユーザー（hash を除く）とトークン
		s.post(t, "/login", "", `{"email":"alice@example.com","password":"password123"}`).
			want(t, 200, `{"user":{"id":"u1","email":"alice@example.com","name":"Alice"},"token":"token-1"}`)
	})

	t.Run("me", func(t *testing.T) {
		s := newServer(t)
		token := s.login(t)
		s.get(t, "/me", token).want(t, 200, `{"id":"u1","email":"alice@example.com","name":"Alice"}`)
		// トークンがない、知らないトークン、期限切れ: 401 server.Unauthorized
		s.get(t, "/me", "").want(t, 401, `{"error":"server.Unauthorized"}`)
		s.get(t, "/me", "no-such-token").want(t, 401, `{"error":"server.Unauthorized"}`)
		s.Advance(SessionTTL)
		s.get(t, "/me", token).want(t, 401, `{"error":"server.Unauthorized"}`)
	})

	t.Run("placeOrder", func(t *testing.T) {
		s := newServer(t)
		token := s.login(t)
		s.post(t, "/orders", "", `[{"item":"apple","quantity":2}]`).want(t, 401, `{"error":"server.Unauthorized"}`)
		s.post(t, "/orders", token, `[{"item":"apple","quantity":0}]`).
			want(t, 400, `{"error":"server.InvalidInput","value":{"items":[{"field":"lines[0].quantity","constraint":"min"}]}}`)
		s.post(t, "/orders", token, `{"item":"apple"}`).
			want(t, 400, `{"error":"http.BadRequest","message":"body: invalid JSON"}`)
		s.count(t, 0, 0)
		// 成功: 注文が一つでき、通知が一つ残る
		s.post(t, "/orders", token, `[{"item":"apple","quantity":2},{"item":"pear","quantity":1}]`).
			want(t, 200, `{"id":"order-1","userId":"u1","lines":[{"item":"apple","quantity":2},{"item":"pear","quantity":1}],`+
				`"placedAt":"2026-01-02T03:04:05Z"}`)
		s.count(t, 1, 1)
	})

	t.Run("placeOrder/NotificationFailed", func(t *testing.T) {
		s := newServer(t)
		token := s.login(t)
		s.FailDelivery(true)
		// NotificationFailed は @http.status を持たないので 500 で、中身は出ない。
		// 仕様では createOrder は起きたまま（kept）なので、注文は残り、通知は残らない
		s.post(t, "/orders", token, `[{"item":"apple","quantity":2}]`).want(t, 500, `{"error":"internal"}`)
		s.count(t, 1, 0)
		s.get(t, "/orders", token).
			want(t, 200, `[{"id":"order-1","userId":"u1","lines":[{"item":"apple","quantity":2}],"placedAt":"2026-01-02T03:04:05Z"}]`)
	})

	t.Run("getOrders", func(t *testing.T) {
		s := newServer(t)
		token := s.login(t)
		s.get(t, "/orders", "").want(t, 401, `{"error":"server.Unauthorized"}`)
		// 注文がなければ空の配列（null ではない）
		s.get(t, "/orders", token).want(t, 200, `[]`)
		for _, item := range []string{"apple", "pear", "plum"} {
			if r := s.post(t, "/orders", token, `[{"item":"`+item+`","quantity":1}]`); r.status != 200 {
				t.Fatalf("POST /orders: %d %s", r.status, r.body)
			}
		}
		order := func(n, item string) string {
			return `{"id":"order-` + n + `","userId":"u1","lines":[{"item":"` + item + `","quantity":1}],"placedAt":"2026-01-02T03:04:05Z"}`
		}
		// page を書かなければ既定値の 1
		s.get(t, "/orders", token).want(t, 200, `[`+order("1", "apple")+`,`+order("2", "pear")+`]`)
		s.get(t, "/orders?page=1", token).want(t, 200, `[`+order("1", "apple")+`,`+order("2", "pear")+`]`)
		s.get(t, "/orders?page=2", token).want(t, 200, `[`+order("3", "plum")+`]`)
		s.get(t, "/orders?page=3", token).want(t, 200, `[]`)
		// page が int として読めない: op を呼ぶ前の 400 http.BadRequest
		s.get(t, "/orders?page=two", token).want(t, 400, `{"error":"http.BadRequest","message":"query page: invalid int"}`)
		s.get(t, "/orders?page=", token).want(t, 400, `{"error":"http.BadRequest","message":"query page: invalid int"}`)
	})

	t.Run("logout", func(t *testing.T) {
		s := newServer(t)
		token := s.login(t)
		s.get(t, "/me", token).want(t, 200, `{"id":"u1","email":"alice@example.com","name":"Alice"}`)
		// 戻り値のない op は 204 で、本文はない
		s.post(t, "/logout", token, "").want(t, 204, "")
		s.get(t, "/me", token).want(t, 401, `{"error":"server.Unauthorized"}`)
		// トークンがなくても、無効でも 204（仕様: 401 にしない）
		s.post(t, "/logout", "", "").want(t, 204, "")
		s.post(t, "/logout", token, "").want(t, 204, "")
	})
}

// response は応答のうち、確かめるところ。
type response struct {
	method, path string
	status       int
	contentType  string
	body         string
}

// want は、ステータスと本文が期待と同じであることを確かめる。本文があれば Content-Type は application/json。
func (r response) want(t *testing.T, status int, body string) {
	t.Helper()
	if r.status != status || r.body != body {
		t.Errorf("%s %s:\n  got:  %d %s\n  want: %d %s", r.method, r.path, r.status, r.body, status, body)
	}
	if body != "" && r.contentType != "application/json" {
		t.Errorf("%s %s: Content-Type が %q", r.method, r.path, r.contentType)
	}
}

func (s Server) get(t *testing.T, path, token string) response {
	t.Helper()
	return s.do(t, http.MethodGet, path, token, "")
}

func (s Server) post(t *testing.T, path, token, body string) response {
	t.Helper()
	return s.do(t, http.MethodPost, path, token, body)
}

// do は要求を一つ送る。token が空でなければ、そのまま Authorization ヘッダの値にする。
func (s Server) do(t *testing.T, method, path, token, body string) response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	s.Handler.ServeHTTP(rec, req)
	res := rec.Result()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{method: method, path: path, status: res.StatusCode, contentType: res.Header.Get("Content-Type"), body: string(data)}
}

// login は alice でログインし、トークンを返す。新しいサーバーではじめに呼ぶので、トークンは "token-1"。
func (s Server) login(t *testing.T) string {
	t.Helper()
	s.post(t, "/login", "", `{"email":"`+AliceEmail+`","password":"`+AlicePassword+`"}`).
		want(t, 200, `{"user":{"id":"u1","email":"alice@example.com","name":"Alice"},"token":"token-1"}`)
	return "token-1"
}

// count は、注文と通知の行の数を確かめる。
func (s Server) count(t *testing.T, orders, notifications int) {
	t.Helper()
	if got := s.Orders(); got != orders {
		t.Errorf("注文が %d 行（期待は %d）", got, orders)
	}
	if got := s.Notifications(); got != notifications {
		t.Errorf("通知が %d 行（期待は %d）", got, notifications)
	}
}
