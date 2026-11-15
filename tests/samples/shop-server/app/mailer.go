package app

import (
	"context"

	pkgtype "example.com/shop-server/gen/type"
)

// Mailer は、op mailer.send の実装。mailer は単位を持たない名前空間（外部の配送の仕組み）なので、生成されるのは
// 呼ぶ側の口（server.Deps.Mailer.Send）だけで、中身は人が書く。
type Mailer struct {
	// Deliver は通知を外へ届ける（メールなど）。届けられなければエラー。テストが差し替える。
	Deliver func(ctx context.Context, notice pkgtype.Notification) error
}

// Send は mailer.send の本体。届けられなければ、仕様の例外 type.NotificationFailed。
// 型は server.MailerSendFunc に合っていて、server.Deps.Mailer.Send にそのまま入れられる。
// ctx を取るのは、mailer.send が純粋でない（外にあることを示すデコレータ @remote が付いている）から。
func (m *Mailer) Send(ctx context.Context, notice pkgtype.Notification) error {
	if err := m.Deliver(ctx, notice); err != nil {
		return pkgtype.NotificationFailed{}
	}
	return nil
}
