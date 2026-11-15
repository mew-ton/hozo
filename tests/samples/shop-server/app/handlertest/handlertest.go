// Package handlertest は、サンプル shop-server のハンドラ（go-http の出力 server.NewHandler）の振る舞いのテスト。
// 人が書いたもので、二つの組み立て（onsqlite: store は SQLite、onmem: store はメモリ）が同じものを走らせる:
// 保存先が違っても、HTTP から見た振る舞いは同じ。
//
// 応答の本文は文字列のまま比べる。ここに書いた JSON が、このサーバーを呼ぶクライアント（TS のトランスパイラなど）が
// 合わせる形になる（../../README.md「go-http が従う規則」）。
package handlertest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/shop-server/app"
	"example.com/shop-server/gen/server"
	pkgtype "example.com/shop-server/gen/type"
)

// 作るサーバーの決まり。
const (
	// はじめからいる、ただ一人のユーザー。Run が Store.AddUser で足す。
	AliceID       = "u1"
	AliceEmail    = "alice@example.com"
	AliceName     = "Alice"
	AlicePassword = "password123"
	// 注文一覧の 1 ページの件数。
	PageSize = 2
	// セッションの有効期間。
	SessionTTL = time.Hour
)

// Now は止まった時計の時刻。
var Now = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// Alice は、はじめからいるユーザー。
func Alice() pkgtype.User {
	return pkgtype.User{Id: AliceID, Email: AliceEmail, Name: AliceName, Hash: app.HashPassword(AlicePassword)}
}

// Fixture は、試すサーバーの材料のうち保存先によらないもの: テストが進める時計と連番の識別子で動く server の葉と、
// 失敗させられる配送。トークンは "token-1"、"token-2"…、注文の識別子は "order-1"、"order-2"… の順に作る。
// サーバーごとに一つ作る。並行には使えない（一つのサーバーへの要求は、一つの goroutine から順に送る）。
type Fixture struct {
	tokens, orders int
	failDelivery   bool
	elapsed        time.Duration
}

// Leaves は、人が書いた葉の op（app.Leaves）を、決まった時計と識別子で動かしたもの。server.New の引数 leaves に渡す。
// 時計は Now で止まっていて、Advance で進む。
func (f *Fixture) Leaves() server.Leaves {
	return &app.Leaves{
		Now:        func() time.Time { return Now.Add(f.elapsed) },
		NewToken:   func() pkgtype.Token { f.tokens++; return pkgtype.Token(fmt.Sprintf("token-%d", f.tokens)) },
		NewOrderID: func() pkgtype.OrderId { f.orders++; return pkgtype.OrderId(fmt.Sprintf("order-%d", f.orders)) },
		SessionTTL: SessionTTL,
		PageSize:   PageSize,
	}
}

// Mailer は mailer の op の実装（app.Mailer）。server.New の引数 mailer に渡す。FailDelivery(true) の後は、届けられない。
func (f *Fixture) Mailer() server.MailerOps {
	return &app.Mailer{Deliver: func(context.Context, pkgtype.Notification) error {
		if f.failDelivery {
			return errors.New("届けられない")
		}
		return nil
	}}
}

// FailDelivery は、以降の通知を届けられなくする（true）か、元に戻す（false）。
func (f *Fixture) FailDelivery(fail bool) { f.failDelivery = fail }

// Advance は、葉の op が見る時計を d だけ進める。
func (f *Fixture) Advance(d time.Duration) { f.elapsed += d }

// Store は、Run が状態を用意し、確かめるのに使う store の op。gen/store と gen/storemem の Ops のどちらも満たす。
type Store interface {
	AddUser(ctx context.Context, user pkgtype.User) error
	ListOrders(ctx context.Context, userId pkgtype.UserId) ([]pkgtype.Order, error)
}

// Server は試す相手。
type Server struct {
	// Handler は、Fixture の Leaves と Mailer で組み立てたサーバー。
	Handler http.Handler
	// Store は、そのサーバーの store。
	Store Store
	// Notifications は、状態 store.notifications の値を読む、形で決まる問い合わせ（その store に渡した、状態の単位の実装の Get）。
	// 通知を読む op は仕様にないが、状態の単位は、形で決まる問い合わせをいつも持つ。
	Notifications func(ctx context.Context) ([]pkgtype.Notification, error)

	fixture *Fixture
}

// startServer は newServer で新しいサーバーを作り、alice を足す。
func startServer(t *testing.T, newServer func(t *testing.T, f *Fixture) Server) Server {
	t.Helper()
	f := &Fixture{}
	s := newServer(t, f)
	s.fixture = f
	if err := s.Store.AddUser(context.Background(), Alice()); err != nil {
		t.Fatal(err)
	}
	return s
}

// Run は newServer が作るサーバーの振る舞いを確かめる。サブテストごとに新しいサーバーを作る。
// サーバーは互いに何も共有しない（状態も、その実装も、server.New と store.New の引数）ので、サブテストは並列に走らせる。
func Run(t *testing.T, newServer func(t *testing.T, f *Fixture) Server) {
	start := func(t *testing.T) Server {
		t.Helper()
		return startServer(t, newServer)
	}

	t.Run("login", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		// パスワードが違う、ユーザーがいない: どちらも 401 type.InvalidCredentials
		s.post(t, "/login", "", `{"email":"alice@example.com","password":"wrong-password"}`).
			want(t, 401, `{"error":"type.InvalidCredentials"}`)
		s.post(t, "/login", "", `{"email":"bob@example.com","password":"password123"}`).
			want(t, 401, `{"error":"type.InvalidCredentials"}`)
		// 入力が制約に合わない: go-http が op を呼ぶ前に確かめて 400 http.BadRequest。違反の場所は、通信の上の場所。
		// op は呼ばれていない: 呼ばれていれば、この email のユーザーはいないので 401 になる
		s.post(t, "/login", "", `{"email":"not-an-email","password":"short"}`).
			want(t, 400, `{"error":"http.BadRequest","value":{"message":"invalid input","violations":[`+
				`{"path":"body.email","constraint":"format"},{"path":"body.password","constraint":"length"}]}}`)
		s.post(t, "/login", "", `{"email":"alice@example.com","password":"short"}`).
			want(t, 400, `{"error":"http.BadRequest","value":{"message":"invalid input","violations":[`+
				`{"path":"body.password","constraint":"length"}]}}`)
		// 本文が JSON でない、型が違う: 読めない要求の 400 http.BadRequest
		s.post(t, "/login", "", `{"email":`).
			want(t, 400, `{"error":"http.BadRequest","value":{"message":"body: invalid JSON","violations":[]}}`)
		s.post(t, "/login", "", `{"email":1}`).
			want(t, 400, `{"error":"http.BadRequest","value":{"message":"body: invalid JSON","violations":[]}}`)
		// 成功: ユーザー（hash を持たない type.PublicUser）とトークン
		s.post(t, "/login", "", `{"email":"alice@example.com","password":"password123"}`).
			want(t, 200, `{"user":{"id":"u1","email":"alice@example.com","name":"Alice"},"token":"token-1"}`)
	})

	t.Run("me", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		token := s.login(t)
		s.get(t, "/me", token).want(t, 200, `{"id":"u1","email":"alice@example.com","name":"Alice"}`)
		// トークンがない、知らないトークン: 401 type.Unauthorized
		s.get(t, "/me", "").want(t, 401, `{"error":"type.Unauthorized"}`)
		s.get(t, "/me", "no-such-token").want(t, 401, `{"error":"type.Unauthorized"}`)
	})

	t.Run("placeOrder", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		token := s.login(t)
		s.post(t, "/orders", "", `[{"item":"apple","quantity":2}]`).want(t, 401, `{"error":"type.Unauthorized"}`)
		s.post(t, "/orders", token, `[{"item":"apple","quantity":2},{"item":"pear","quantity":0}]`).
			want(t, 400, `{"error":"http.BadRequest","value":{"message":"invalid input","violations":[`+
				`{"path":"body[1].quantity","constraint":"min"}]}}`)
		// 制約を確かめるのは op を呼ぶ前なので、トークンがなくても 401 より先に 400 になる
		s.post(t, "/orders", "", `[{"item":"apple","quantity":0}]`).
			want(t, 400, `{"error":"http.BadRequest","value":{"message":"invalid input","violations":[`+
				`{"path":"body[0].quantity","constraint":"min"}]}}`)
		s.post(t, "/orders", token, `{"item":"apple"}`).
			want(t, 400, `{"error":"http.BadRequest","value":{"message":"body: invalid JSON","violations":[]}}`)
		s.count(t, 0, 0)
		// 成功: 注文が一つでき、通知が一つ残る
		s.post(t, "/orders", token, `[{"item":"apple","quantity":2},{"item":"pear","quantity":1}]`).
			want(t, 200, `{"id":"order-1","userId":"u1","lines":[{"item":"apple","quantity":2},{"item":"pear","quantity":1}],`+
				`"placedAt":"2026-01-02T03:04:05Z"}`)
		s.count(t, 1, 1)
	})

	t.Run("placeOrder/NotificationFailed", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		token := s.login(t)
		s.fixture.FailDelivery(true)
		// mailer.send が失敗すると type.NotificationFailed。@http.status を持たないので 500 で、中身は出ない。
		// 仕様では createOrder は起きたままなので、注文は残り、通知は残らない
		s.post(t, "/orders", token, `[{"item":"apple","quantity":2}]`).want(t, 500, `{"error":"internal"}`)
		s.count(t, 1, 0)
		s.get(t, "/orders", token).
			want(t, 200, `[{"id":"order-1","userId":"u1","lines":[{"item":"apple","quantity":2}],"placedAt":"2026-01-02T03:04:05Z"}]`)
	})

	t.Run("getOrders", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		token := s.login(t)
		s.get(t, "/orders", "").want(t, 401, `{"error":"type.Unauthorized"}`)
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
		// page が int として読めない: 読めない要求の 400 http.BadRequest
		s.get(t, "/orders?page=two", token).
			want(t, 400, `{"error":"http.BadRequest","value":{"message":"query page: invalid int","violations":[]}}`)
		s.get(t, "/orders?page=", token).
			want(t, 400, `{"error":"http.BadRequest","value":{"message":"query page: invalid int","violations":[]}}`)
	})

	t.Run("logout", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		token := s.login(t)
		s.get(t, "/me", token).want(t, 200, `{"id":"u1","email":"alice@example.com","name":"Alice"}`)
		// 戻り値のない op は 204 で、本文はない
		s.post(t, "/logout", token, "").want(t, 204, "")
		s.get(t, "/me", token).want(t, 401, `{"error":"type.Unauthorized"}`)
		// トークンがない、もう無効: 401（仕様: logout は authenticate を通る）
		s.post(t, "/logout", "", "").want(t, 401, `{"error":"type.Unauthorized"}`)
		s.post(t, "/logout", token, "").want(t, 401, `{"error":"type.Unauthorized"}`)
	})

	t.Run("cancelled", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		token := s.login(t)
		// 中断された要求の ctx は、ハンドラ → 配線 → store の op まで届く: store の op は ctx のエラーを返し
		// （SQLite は database/sql が、メモリは memstore が ctx を見る）、状態は変わらない。応答は 500
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s.do(t, ctx, http.MethodPost, "/orders", token, `[{"item":"apple","quantity":2}]`).want(t, 500, `{"error":"internal"}`)
		s.do(t, ctx, http.MethodGet, "/me", token, "").want(t, 500, `{"error":"internal"}`)
		s.count(t, 0, 0)
		// 同じ要求は、生きている ctx なら通る
		s.post(t, "/orders", token, `[{"item":"apple","quantity":2}]`).
			want(t, 200, `{"id":"order-1","userId":"u1","lines":[{"item":"apple","quantity":2}],"placedAt":"2026-01-02T03:04:05Z"}`)
		s.count(t, 1, 1)
	})

	t.Run("expired", func(t *testing.T) {
		t.Parallel()
		s := start(t)
		// ログインしたセッションの期限は、今（Now）から SessionTTL の後
		token := s.login(t)
		// 期限の直前までは通る
		s.fixture.Advance(SessionTTL - time.Nanosecond)
		s.get(t, "/me", token).want(t, 200, `{"id":"u1","email":"alice@example.com","name":"Alice"}`)
		// 期限ちょうどから先は、どの入口も 401 type.Unauthorized（配線 authenticate が、葉 requireFresh を通す）
		s.fixture.Advance(time.Nanosecond)
		s.get(t, "/me", token).want(t, 401, `{"error":"type.Unauthorized"}`)
		s.get(t, "/orders", token).want(t, 401, `{"error":"type.Unauthorized"}`)
		s.post(t, "/orders", token, `[{"item":"apple","quantity":2}]`).want(t, 401, `{"error":"type.Unauthorized"}`)
		s.post(t, "/logout", token, "").want(t, 401, `{"error":"type.Unauthorized"}`)
		s.count(t, 0, 0)
		// 入り直せば、新しいセッションで通る
		s.post(t, "/login", "", `{"email":"alice@example.com","password":"password123"}`).
			want(t, 200, `{"user":{"id":"u1","email":"alice@example.com","name":"Alice"},"token":"token-2"}`)
		s.get(t, "/me", "token-2").want(t, 200, `{"id":"u1","email":"alice@example.com","name":"Alice"}`)
		s.get(t, "/me", token).want(t, 401, `{"error":"type.Unauthorized"}`)
	})
}

// RunTwo は、newServer が作る二つのサーバーが、一つのプロセスで同時に動き、互いに混ざらないことを確かめる。
// 同じ出力（同じ gen/store、同じ gen/server）の上の二つでよい: 状態の実装はパッケージの変数ではなく、
// store.New と server.New の引数なので、組み立てた数だけ別々にある。
func RunTwo(t *testing.T, newServer func(t *testing.T, f *Fixture) Server) {
	a, b := startServer(t, newServer), startServer(t, newServer)
	// 二つに同時に要求を送る: a には注文を三つ、b には一つ。どちらのトークンも "token-1"、注文は "order-1" から
	var wg sync.WaitGroup
	for _, c := range []struct {
		s      Server
		orders int
	}{{a, 3}, {b, 1}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token := c.s.login(t)
			for range c.orders {
				if r := c.s.post(t, "/orders", token, `[{"item":"apple","quantity":1}]`); r.status != 200 {
					t.Errorf("POST /orders: %d %s", r.status, r.body)
				}
			}
		}()
	}
	wg.Wait()
	a.count(t, 3, 3)
	b.count(t, 1, 1)
	// a でログアウトしても、b の同じ値のトークンは生きている
	a.post(t, "/logout", "token-1", "").want(t, 204, "")
	a.get(t, "/me", "token-1").want(t, 401, `{"error":"type.Unauthorized"}`)
	b.get(t, "/me", "token-1").want(t, 200, `{"id":"u1","email":"alice@example.com","name":"Alice"}`)
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
	return s.do(t, context.Background(), http.MethodGet, path, token, "")
}

func (s Server) post(t *testing.T, path, token, body string) response {
	t.Helper()
	return s.do(t, context.Background(), http.MethodPost, path, token, body)
}

// do は要求を一つ、ctx を要求の context にして送る。token が空でなければ、そのまま Authorization ヘッダの値にする。
func (s Server) do(t *testing.T, ctx context.Context, method, path, token, body string) response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	s.Handler.ServeHTTP(rec, req)
	res := rec.Result()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("%s %s: 応答の本文を読めない: %v", method, path, err)
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

// count は、alice の注文の数（store.listOrders で数える）と、通知の数（状態 store.notifications の Get で数える）を確かめる。
func (s Server) count(t *testing.T, orders, notifications int) {
	t.Helper()
	got, err := s.Store.ListOrders(context.Background(), AliceID)
	if err != nil {
		t.Errorf("listOrders: %v", err)
	}
	if len(got) != orders {
		t.Errorf("注文が %d 件（期待は %d）", len(got), orders)
	}
	notices, err := s.Notifications(context.Background())
	if err != nil {
		t.Errorf("notifications の Get: %v", err)
	}
	if len(notices) != notifications {
		t.Errorf("通知が %d 件（期待は %d）", len(notices), notifications)
	}
}
