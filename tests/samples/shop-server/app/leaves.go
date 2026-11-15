// Package app は、サンプル shop-server で人が書く部分のうち、保存先によらないもの（../README.md「人が書いたもの」）:
// server の葉の op の本体（leaves.go）と、mailer.send の差し込み口の実装（mailer.go）。
//
// このパッケージは保存先に触れない。import するのは gen/server と gen/type だけで、gen/store も gen/storemem も
// import しない（leaves_test.go が確かめる）。保存先との組み立ては onsqlite と onmem にある。
package app

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"example.com/shop-server/gen/server"
	pkgtype "example.com/shop-server/gen/type"
)

// Leaves は、server の葉の op の本体が外から受け取るもの。テストが差し替えられるように、時計と識別子の作り方を持つ。
type Leaves struct {
	// Now は今の時刻。
	Now func() time.Time
	// NewToken は新しいセッションのトークンを作る。
	NewToken func() pkgtype.Token
	// NewOrderID は新しい注文の識別子を作る。
	NewOrderID func() pkgtype.OrderId
	// SessionTTL はセッションの有効期間。
	SessionTTL time.Duration
	// PageSize は注文一覧の 1 ページの件数。
	PageSize int
}

// Server は、葉の op の本体を、生成された server.Leaves に入れて返す。六つとも仕様の上で純粋な op で、
// 関数は ctx を取らない（server.Leaves のフィールドの型がそう決めている）。
func (l *Leaves) Server() server.Leaves {
	return server.Leaves{
		CheckPassword: checkPassword,
		NewSession:    l.newSession,
		ToPublicUser:  toPublicUser,
		PageOf:        l.pageOf,
		NewOrder:      l.newOrder,
		ComposeNotice: composeNotice,
	}
}

// checkPassword は server.checkPassword の本体。パスワードが合わなければ InvalidCredentials。
func checkPassword(user pkgtype.User, password pkgtype.Password) error {
	if user.Hash != HashPassword(password) {
		return pkgtype.InvalidCredentials{}
	}
	return nil
}

// newSession は server.newSession の本体。トークンを作り、期限を今から SessionTTL の後にする。
func (l *Leaves) newSession(user pkgtype.User) (pkgtype.Session, error) {
	return pkgtype.Session{
		Token:     l.NewToken(),
		UserId:    user.Id,
		ExpiresAt: l.Now().UTC().Add(l.SessionTTL),
	}, nil
}

// toPublicUser は server.toPublicUser の本体。type.User を type.PublicUser に写す（hash を落とす）。
func toPublicUser(user pkgtype.User) (pkgtype.PublicUser, error) {
	return pkgtype.PublicUser{Id: user.Id, Email: user.Email, Name: user.Name}, nil
}

// pageOf は server.pageOf の本体。1 から数える page の分（PageSize 件）を取り出す。範囲の外なら空。
func (l *Leaves) pageOf(orders []pkgtype.Order, page int64) ([]pkgtype.Order, error) {
	n, size := int64(len(orders)), int64(l.PageSize)
	start := min((max(page, 1)-1)*size, n)
	end := min(start+size, n)
	return orders[start:end], nil
}

// newOrder は server.newOrder の本体。識別子を作り、注文した時刻を今にする。
func (l *Leaves) newOrder(session pkgtype.Session, lines []pkgtype.OrderLine) (pkgtype.Order, error) {
	return pkgtype.Order{
		Id:       l.NewOrderID(),
		UserId:   session.UserId,
		Lines:    append([]pkgtype.OrderLine{}, lines...),
		PlacedAt: l.Now().UTC(),
	}, nil
}

// composeNotice は server.composeNotice の本体。
func composeNotice(order pkgtype.Order) (pkgtype.Notification, error) {
	return pkgtype.Notification{UserId: order.UserId, Message: "注文 " + string(order.Id) + " を受け付けました"}, nil
}

// HashPassword はパスワードのハッシュ。サンプルなので SHA-256 一回（本物は bcrypt などにする）。
func HashPassword(p pkgtype.Password) string {
	sum := sha256.Sum256([]byte(p))
	return hex.EncodeToString(sum[:])
}
