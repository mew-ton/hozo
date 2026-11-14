// Command app は、サンプル shop-server の「両方」の組（go-http と go-sqlite）で人が書く部分（../../README.md）。
// 生成された server パッケージに、葉の op の本体を入れてサーバーにする。
//
//   - app.go: 葉の op の本体（server.Tables の上に書く）と、それを server に入れる Setup
//   - main.go: SQLite を開いて Setup し、待ち受ける
package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"example.com/shop-server/both/server"
	"github.com/mew-ton/hozo/runtime"
)

// App は葉の op の本体が外から受け取るもの。テストが差し替えられるように、時計と識別子の作り方を持つ。
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
}

// Setup は db に表を作り、葉の op の本体を server に入れて、ハンドラを返す。
//
// server.States と server.Tables はパッケージの変数なので、一つのプロセスで有効なのは最後に Setup した一組だけ。
func (a *App) Setup(db *sql.DB) (http.Handler, error) {
	// go-sqlite の出力: 表、server.Tables、States の IsEmpty
	if err := server.SetupDB(db); err != nil {
		return nil, err
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
	return server.NewHandler(ops), nil
}

// login は server.login の本体。
func (a *App) login(in server.LoginInput) (server.LoginOk, error) {
	if err := in.Validate(); err != nil {
		return server.LoginOk{}, invalidInput(err)
	}
	user, err := server.Tables.Users.GetByEmail(in.Body.Email)
	if err != nil {
		return server.LoginOk{}, err
	}
	if user == nil || user.Hash != HashPassword(in.Body.Password) {
		return server.LoginOk{}, server.InvalidCredentials{}
	}
	session := server.Session{
		Token:     a.NewToken(),
		UserId:    user.Id,
		ExpiresAt: a.Now().UTC().Add(a.SessionTTL),
	}
	if err := server.Tables.Sessions.Insert(session); err != nil {
		return server.LoginOk{}, err
	}
	return server.LoginOk{User: server.UserToPublicUser(*user), Token: session.Token}, nil
}

// logout は server.logout の本体。トークンがない・無効なら何もしない。
func (a *App) logout(in server.AuthInput) error {
	if in.Token == nil {
		return nil
	}
	return server.Tables.Sessions.Delete(*in.Token)
}

// getMe は server.getMe の本体。
func (a *App) getMe(in server.AuthInput) (server.PublicUser, error) {
	user, err := a.caller(in.Token)
	if err != nil {
		return server.PublicUser{}, err
	}
	return server.UserToPublicUser(*user), nil
}

// getOrders は server.getOrders の本体。呼んだユーザーの注文を、注文した順に 1 ページ分返す。
func (a *App) getOrders(in server.GetOrdersInput) ([]server.Order, error) {
	user, err := a.caller(in.Token)
	if err != nil {
		return nil, err
	}
	all, err := server.Tables.Orders.List()
	if err != nil {
		return nil, err
	}
	var mine []server.Order
	for _, o := range all {
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
	if err := server.Tables.Orders.Insert(order); err != nil {
		return server.Order{}, err
	}
	return order, nil
}

// sendOrderNotice は server.sendOrderNotice の本体。届けられなければ NotificationFailed で、通知の行は足さない。
func (a *App) sendOrderNotice(order server.Order) error {
	n := server.Notification{UserId: order.UserId, Message: "注文 " + string(order.Id) + " を受け付けました"}
	if err := a.Deliver(n); err != nil {
		return server.NotificationFailed{}
	}
	return server.Tables.Notifications.Insert(n)
}

// caller はトークンから、呼んだユーザーを引く。トークンがない、セッションがない、期限が切れていれば Unauthorized。
func (a *App) caller(token *server.Token) (*server.User, error) {
	if token == nil {
		return nil, server.Unauthorized{}
	}
	session, err := server.Tables.Sessions.Get(*token)
	if err != nil {
		return nil, err
	}
	if session == nil || !a.Now().Before(session.ExpiresAt) {
		return nil, server.Unauthorized{}
	}
	user, err := server.Tables.Users.Get(session.UserId)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, server.Unauthorized{}
	}
	return user, nil
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
