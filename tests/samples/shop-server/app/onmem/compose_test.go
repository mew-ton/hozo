package onmem

import (
	"testing"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/app/memstore"
	"example.com/shop-server/app/storetest"
	store "example.com/shop-server/gen/storemem"
	pkgtype "example.com/shop-server/gen/type"
)

// TestHandler は、go-http のハンドラ → 配線 → 人がメモリの上に書いた store の op、の全体の振る舞い。
// onsqlite の TestHandler と同じ handlertest.Run を走らせる。
func TestHandler(t *testing.T) {
	handlertest.Run(t, func(t *testing.T, f *handlertest.Fixture) handlertest.Server {
		handler, s := New(f.Leaves(), f.Send())
		return handlertest.Server{
			Handler: handler,
			Store:   s,
			// 通知を読む op は仕様にないので、状態の単位の、形で決まる問い合わせで読む（memstore が埋めた口）
			Notifications: store.States.Notifications.Get,
		}
	})
}

// TestStore は、人がメモリの上に書いた store の op の振る舞い。onsqlite の TestStore と同じ storetest.Run を走らせる
// （server は組み立てない。gen/storemem と memstore だけで動く）。
func TestStore(t *testing.T) {
	storetest.Run(t, func(t *testing.T) storetest.Store {
		memstore.Setup()
		return storetest.Store{Ops: store.New(), States: states()}
	})
}

// states は、store.States の形で決まる問い合わせを、storetest に渡す形に並べる（memstore.Setup の後で呼ぶ）。
// gen/store と gen/storemem の StateSet は別の型なので、共通のテストへは口を一つずつ渡す。
func states() storetest.States {
	u, s, o, n := store.States.Users, store.States.Sessions, store.States.Orders, store.States.Notifications
	return storetest.States{
		Users:         storetest.List[pkgtype.User]{IsEmpty: u.IsEmpty, Get: u.Get},
		Sessions:      storetest.List[pkgtype.Session]{IsEmpty: s.IsEmpty, Get: s.Get},
		Orders:        storetest.List[pkgtype.Order]{IsEmpty: o.IsEmpty, Get: o.Get},
		Notifications: storetest.List[pkgtype.Notification]{IsEmpty: n.IsEmpty, Get: n.Get},
	}
}
