// Command app は、サンプル shop-server の「go-http だけ」の組で人が書く部分（../../README.md）。
// 状態の保存先のトランスパイラがいないので、状態（メモリの上）と、状態の単位の問い合わせも人が書く。
//
//   - app.go: 状態、葉の op の本体、それを server に入れる Setup
//   - main.go: Setup し、待ち受ける
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"

	"example.com/shop-server/httponly/server"
	"github.com/mew-ton/hozo/runtime"
)

// App は状態と、葉の op の本体が外から受け取るもの。テストが差し替えられるように、時計と識別子の作り方を持つ。
type App struct {
	// Now は今の時刻。
	Now func() time.Time
	// NewToken は新しいセッションのトークンを作る。
	NewToken func() server.Token
	// NewOrderID は新しい注文の識別子を作る。
	NewOrderID func() server.OrderId
	// Deliver は通知をユーザーに届ける（メールなど）。届けられなければエラー。
	Deliver func(n server.Notification) error
	// SessionTTL はセッションの有効期間。
	SessionTTL time.Duration
	// PageSize は注文一覧の 1 ページの件数。
	PageSize int

	// 状態。ハンドラは並行に呼ばれるので、mu で守る。
	mu            sync.Mutex
	users         []server.User
	sessions      map[server.Token]server.Session
	orders        []server.Order
	notifications []server.Notification
}

// Setup は葉の op の本体と状態の問い合わせを server に入れて、ハンドラを返す。
//
// server.States はパッケージの変数なので、一つのプロセスで有効なのは最後に Setup した一組だけ。
func (a *App) Setup() http.Handler {
	a.sessions = map[server.Token]server.Session{}
	// 状態の単位の問い合わせ。この組の共有する形に users の単位はない（どの op の効果の宛先でもなく、状態を選ぶ者もいない）
	server.States.Sessions.IsEmpty = func() (bool, error) { return a.locked(func() bool { return len(a.sessions) == 0 }), nil }
	server.States.Orders.IsEmpty = func() (bool, error) { return a.locked(func() bool { return len(a.orders) == 0 }), nil }
	server.States.Notifications.IsEmpty = func() (bool, error) {
		return a.locked(func() bool { return len(a.notifications) == 0 }), nil
	}
	// 効果を持つ葉の op は、その効果の宛先の状態の単位に入れる
	server.States.Sessions.Login = a.login
	server.States.Sessions.Logout = a.logout
	server.States.Orders.SaveOrder = a.saveOrder
	server.States.Notifications.SendOrderNotice = a.sendOrderNotice
	// 効果を持たない葉の op は Leaves に入れる
	ops := server.New(server.Leaves{
		GetMe:     a.getMe,
		GetOrders: a.getOrders,
	})
	// go-http の出力
	return server.NewHandler(ops)
}

// AddUser はユーザーを一人足す（仕様にユーザーを足す op がないので、起動のときとテストが使う）。
func (a *App) AddUser(u server.User) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.users = append(a.users, u)
}

// Counts は注文と通知の数。
func (a *App) Counts() (orders, notifications int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.orders), len(a.notifications)
}

// locked は mu を持って f を呼ぶ。
func (a *App) locked(f func() bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return f()
}

// login は server.login の本体。
func (a *App) login(in server.LoginInput) (server.LoginOk, error) {
	if err := in.Validate(); err != nil {
		return server.LoginOk{}, invalidInput(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, user := range a.users {
		if user.Email != in.Body.Email || user.Hash != HashPassword(in.Body.Password) {
			continue
		}
		session := server.Session{
			Token:     a.NewToken(),
			UserId:    user.Id,
			ExpiresAt: a.Now().UTC().Add(a.SessionTTL),
		}
		a.sessions[session.Token] = session
		return server.LoginOk{User: server.UserToPublicUser(user), Token: session.Token}, nil
	}
	return server.LoginOk{}, server.InvalidCredentials{}
}

// logout は server.logout の本体。トークンがない・無効なら何もしない。
func (a *App) logout(in server.AuthInput) error {
	if in.Token == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, *in.Token)
	return nil
}

// getMe は server.getMe の本体。
func (a *App) getMe(in server.AuthInput) (server.PublicUser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	user, err := a.caller(in.Token)
	if err != nil {
		return server.PublicUser{}, err
	}
	return server.UserToPublicUser(user), nil
}

// getOrders は server.getOrders の本体。呼んだユーザーの注文を、注文した順に 1 ページ分返す。
func (a *App) getOrders(in server.GetOrdersInput) ([]server.Order, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	user, err := a.caller(in.Token)
	if err != nil {
		return nil, err
	}
	var mine []server.Order
	for _, o := range a.orders {
		if o.UserId == user.Id {
			mine = append(mine, o)
		}
	}
	page := max(in.Page, 1)
	start := min((page-1)*int64(a.PageSize), int64(len(mine)))
	end := min(start+int64(a.PageSize), int64(len(mine)))
	return mine[start:end], nil
}

// saveOrder は server.saveOrder の本体。
func (a *App) saveOrder(in server.PlaceOrderInput) (server.Order, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	user, err := a.caller(in.Token)
	if err != nil {
		return server.Order{}, err
	}
	if err := in.Validate(); err != nil {
		return server.Order{}, invalidInput(err)
	}
	order := server.Order{
		Id:       a.NewOrderID(),
		UserId:   user.Id,
		Lines:    append([]server.OrderLine{}, in.Lines...),
		PlacedAt: a.Now().UTC(),
	}
	a.orders = append(a.orders, order)
	return order, nil
}

// sendOrderNotice は server.sendOrderNotice の本体。届けられなければ NotificationFailed で、通知は足さない。
func (a *App) sendOrderNotice(order server.Order) error {
	n := server.Notification{UserId: order.UserId, Message: "注文 " + string(order.Id) + " を受け付けました"}
	if err := a.Deliver(n); err != nil {
		return server.NotificationFailed{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notifications = append(a.notifications, n)
	return nil
}

// caller はトークンから、呼んだユーザーを引く。トークンがない、セッションがない、期限が切れていれば Unauthorized。
// mu を持って呼ぶ。
func (a *App) caller(token *server.Token) (server.User, error) {
	if token == nil {
		return server.User{}, server.Unauthorized{}
	}
	session, ok := a.sessions[*token]
	if !ok || !a.Now().Before(session.ExpiresAt) {
		return server.User{}, server.Unauthorized{}
	}
	for _, user := range a.users {
		if user.Id == session.UserId {
			return user, nil
		}
	}
	return server.User{}, server.Unauthorized{}
}

// invalidInput は Validate の違反を、仕様の例外 server.InvalidInput にする。
func invalidInput(err error) error {
	var ve *runtime.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	items := make([]server.Violation, 0, len(ve.Violations))
	for _, v := range ve.Violations {
		items = append(items, server.Violation{Field: v.Path, Constraint: v.Constraint})
	}
	return server.InvalidInput{Value: server.Violations{Items: items}}
}

// HashPassword はパスワードのハッシュ。サンプルなので SHA-256 一回（本物は bcrypt などにする）。
func HashPassword(p server.Password) string {
	sum := sha256.Sum256([]byte(p))
	return hex.EncodeToString(sum[:])
}
