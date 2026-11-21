// Package app は、サンプル shop-server で人が書く部分のうち、保存先によらないもの（../README.md「人が書いたもの」）:
// server の葉の op の本体（leaves.go）、mailer.send の実装（mailer.go）、@rate.limited の数え方（limiter.go）。
//
// このパッケージは保存先に触れない。import するのは gen/server と gen/type だけで、gen/store は
// import しない（leaves_test.go が確かめる）。保存先との組み立ては onsqlite にある。
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"example.com/shop-server/gen/server"
	pkgtype "example.com/shop-server/gen/type"
)

// Leaves は、server の葉の op 十の本体（メソッド）と、それが外から受け取るもの（フィールド）。
// *Leaves を server.New の引数 leaves にそのまま渡す。テストが差し替えられるように、時計と識別子の作り方を持つ。
//
// メソッドは、十とも、最初の引数に ctx を取り、最後の戻り値に error を返す（生成された server.Leaves のメソッドの形。
// docs/plugin-go.md「op の形」: 基底は、どの op にそれが要るかを判定しない）。ここの本体は、どれも待たないので ctx を見ない。
// error を返すのは、仕様の例外（checkPassword、requireFresh）と、トークンと識別子を作れなかったとき（newSession、newOrder）。
type Leaves struct {
	// Now は今の時刻。
	Now func() time.Time
	// NewToken は新しいセッションのトークンを作る。作れなければ、その error（仕様の例外ではない）。
	NewToken func() (pkgtype.Token, error)
	// NewOrderID は新しい注文の識別子を作る。作れなければ、その error（仕様の例外ではない）。
	NewOrderID func() (pkgtype.OrderId, error)
	// SessionTTL はセッションの有効期間。
	SessionTTL time.Duration
	// PageSize は注文一覧の 1 ページの件数。
	PageSize int
	// Audit は、呼ばれたことの記録の先（server.recordAudit）。nil なら、何もしない。
	Audit func()
}

// *Leaves は server の葉の op をすべて持つ。メソッドが一つでも足りなければ、ここでコンパイルできない。
var _ server.Leaves = (*Leaves)(nil)

// CheckPassword は server.checkPassword の本体。パスワードが合わなければ InvalidCredentials。
func (l *Leaves) CheckPassword(_ context.Context, user pkgtype.User, password pkgtype.Password) error {
	if user.Hash != HashPassword(password) {
		return pkgtype.InvalidCredentials{}
	}
	return nil
}

// NewSession は server.newSession の本体。トークンを作り、期限を今から SessionTTL の後にする。
// トークンを作れなければ、その error を、そのまま返す。
func (l *Leaves) NewSession(_ context.Context, user pkgtype.User) (pkgtype.Session, error) {
	token, err := l.NewToken()
	if err != nil {
		return pkgtype.Session{}, err
	}
	return pkgtype.Session{
		Token:     token,
		UserId:    user.Id,
		ExpiresAt: l.Now().UTC().Add(l.SessionTTL),
	}, nil
}

// ToPublicUser は server.toPublicUser の本体。type.User を type.PublicUser に写す（hash を落とす）。
func (l *Leaves) ToPublicUser(_ context.Context, user pkgtype.User) (pkgtype.PublicUser, error) {
	return pkgtype.PublicUser{Id: user.Id, Email: user.Email, Name: user.Name}, nil
}

// RequireFresh は server.requireFresh の本体。今が期限（ExpiresAt）より前なら、セッションをそのまま返す。
// 期限ちょうどか、それより後なら Unauthorized。
func (l *Leaves) RequireFresh(_ context.Context, session pkgtype.Session) (pkgtype.Session, error) {
	if !l.Now().Before(session.ExpiresAt) {
		return pkgtype.Session{}, pkgtype.Unauthorized{}
	}
	return session, nil
}

// PageOf は server.pageOf の本体。1 から数える page の分（PageSize 件）を取り出す。範囲の外なら空。
func (l *Leaves) PageOf(_ context.Context, orders []pkgtype.Order, page int64) ([]pkgtype.Order, error) {
	n, size := int64(len(orders)), int64(l.PageSize)
	start := min((max(page, 1)-1)*size, n)
	end := min(start+size, n)
	return orders[start:end], nil
}

// Summarize は server.summarize の本体。注文の識別子と、行の数。
func (l *Leaves) Summarize(_ context.Context, order pkgtype.Order) (server.OrderSummary, error) {
	return server.OrderSummary{Id: order.Id, Lines: int64(len(order.Lines))}, nil
}

// MyPage は server.myPage の本体。受け取ったものを、そのまま並べる。要約は、受け取った順のまま。
func (l *Leaves) MyPage(_ context.Context, user pkgtype.PublicUser, latest *pkgtype.Order, summaries []server.OrderSummary) (server.MyPage, error) {
	return server.MyPage{User: user, Latest: latest, Summaries: summaries}, nil
}

// NewOrder は server.newOrder の本体。識別子を作り、注文した時刻を今にする。
// 識別子を作れなければ、その error を、そのまま返す。
func (l *Leaves) NewOrder(_ context.Context, session pkgtype.Session, lines []pkgtype.OrderLine) (pkgtype.Order, error) {
	id, err := l.NewOrderID()
	if err != nil {
		return pkgtype.Order{}, err
	}
	return pkgtype.Order{
		Id:       id,
		UserId:   session.UserId,
		Lines:    append([]pkgtype.OrderLine{}, lines...),
		PlacedAt: l.Now().UTC(),
	}, nil
}

// ComposeNotice は server.composeNotice の本体。
func (l *Leaves) ComposeNotice(_ context.Context, order pkgtype.Order) (pkgtype.Notification, error) {
	return pkgtype.Notification{UserId: order.UserId, Message: "注文 " + string(order.Id) + " を受け付けました"}, nil
}

// RecordAudit は server.recordAudit の本体: 記録の先（Audit）を、一度呼ぶ。デコレータ @server.audited の本体が、
// 包んだ op の中身が正常に終わった後に、呼ぶ（生成された logoutServerAudited）。
func (l *Leaves) RecordAudit(_ context.Context) error {
	if l.Audit != nil {
		l.Audit()
	}
	return nil
}

// HashPassword はパスワードのハッシュ。サンプルなので SHA-256 一回（本物は bcrypt などにする）。
func HashPassword(p pkgtype.Password) string {
	sum := sha256.Sum256([]byte(p))
	return hex.EncodeToString(sum[:])
}
