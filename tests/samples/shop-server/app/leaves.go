// Package app は、サンプル shop-server で人が書く部分のうち、保存先によらないもの（../README.md「人が書いたもの」）:
// server の葉の op の本体（leaves.go）と、mailer.send の実装（mailer.go）。
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

// Leaves は、server の葉の op 七つの本体（メソッド）と、それが外から受け取るもの（フィールド）。
// *Leaves を server.New の引数 leaves にそのまま渡す。テストが差し替えられるように、時計と識別子の作り方を持つ。
//
// 七つとも仕様の上で純粋な op で、メソッドは ctx を取らない。throws のない五つは error も返さない
// （生成された server.Leaves のメソッドの形がそう決めている）。
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

// *Leaves は server の葉の op をすべて持つ。メソッドが一つでも足りなければ、ここでコンパイルできない。
var _ server.Leaves = (*Leaves)(nil)

// CheckPassword は server.checkPassword の本体。パスワードが合わなければ InvalidCredentials。
func (l *Leaves) CheckPassword(user pkgtype.User, password pkgtype.Password) error {
	if user.Hash != HashPassword(password) {
		return pkgtype.InvalidCredentials{}
	}
	return nil
}

// NewSession は server.newSession の本体。トークンを作り、期限を今から SessionTTL の後にする。
func (l *Leaves) NewSession(user pkgtype.User) pkgtype.Session {
	return pkgtype.Session{
		Token:     l.NewToken(),
		UserId:    user.Id,
		ExpiresAt: l.Now().UTC().Add(l.SessionTTL),
	}
}

// ToPublicUser は server.toPublicUser の本体。type.User を type.PublicUser に写す（hash を落とす）。
func (l *Leaves) ToPublicUser(user pkgtype.User) pkgtype.PublicUser {
	return pkgtype.PublicUser{Id: user.Id, Email: user.Email, Name: user.Name}
}

// RequireFresh は server.requireFresh の本体。今が期限（ExpiresAt）より前なら、セッションをそのまま返す。
// 期限ちょうどか、それより後なら Unauthorized。
func (l *Leaves) RequireFresh(session pkgtype.Session) (pkgtype.Session, error) {
	if !l.Now().Before(session.ExpiresAt) {
		return pkgtype.Session{}, pkgtype.Unauthorized{}
	}
	return session, nil
}

// PageOf は server.pageOf の本体。1 から数える page の分（PageSize 件）を取り出す。範囲の外なら空。
func (l *Leaves) PageOf(orders []pkgtype.Order, page int64) []pkgtype.Order {
	n, size := int64(len(orders)), int64(l.PageSize)
	start := min((max(page, 1)-1)*size, n)
	end := min(start+size, n)
	return orders[start:end]
}

// NewOrder は server.newOrder の本体。識別子を作り、注文した時刻を今にする。
func (l *Leaves) NewOrder(session pkgtype.Session, lines []pkgtype.OrderLine) pkgtype.Order {
	return pkgtype.Order{
		Id:       l.NewOrderID(),
		UserId:   session.UserId,
		Lines:    append([]pkgtype.OrderLine{}, lines...),
		PlacedAt: l.Now().UTC(),
	}
}

// ComposeNotice は server.composeNotice の本体。
func (l *Leaves) ComposeNotice(order pkgtype.Order) pkgtype.Notification {
	return pkgtype.Notification{UserId: order.UserId, Message: "注文 " + string(order.Id) + " を受け付けました"}
}

// HashPassword はパスワードのハッシュ。サンプルなので SHA-256 一回（本物は bcrypt などにする）。
func HashPassword(p pkgtype.Password) string {
	sum := sha256.Sum256([]byte(p))
	return hex.EncodeToString(sum[:])
}
