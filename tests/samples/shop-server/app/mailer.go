package app

import (
	"context"

	"example.com/shop-server/gen/server"
	pkgtype "example.com/shop-server/gen/type"
)

// Mailer は、op mailer.send の実装。mailer は単位を持たない名前空間（外部の配送の仕組み）なので、生成されるのは
// 呼ぶ側が受け取るインターフェース（server.MailerOps）だけで、中身は人が書く。
// *Mailer を server.New の引数 mailer にそのまま渡す。
type Mailer struct {
	// Deliver は通知を外へ届ける（メールなど）。届けられなければエラー。テストが差し替える。
	Deliver func(ctx context.Context, notice pkgtype.Notification) error
}

// *Mailer は、server が呼ぶ mailer の op をすべて持つ。
var _ server.MailerOps = (*Mailer)(nil)

// Send は mailer.send の本体。届けられなければ、仕様の例外 type.NotificationFailed。
// ctx を取るのは、mailer.send が純粋でない（送る仕組みの語彙のデコレータ @smtp.sendMail が付いている）から。
func (m *Mailer) Send(ctx context.Context, notice pkgtype.Notification) error {
	if err := m.Deliver(ctx, notice); err != nil {
		return pkgtype.NotificationFailed{}
	}
	return nil
}
