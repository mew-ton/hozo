package onmem

import (
	"testing"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/app/memstore"
	"example.com/shop-server/app/storetest"
	store "example.com/shop-server/gen/storemem"
)

// TestHandler は、go-http のハンドラ → 配線 → 人がメモリの上に書いた store の op、の全体の振る舞い。
// onsqlite の TestHandler と同じ handlertest.Run を走らせる。
func TestHandler(t *testing.T) {
	handlertest.Run(t, func(t *testing.T, f *handlertest.Fixture) handlertest.Server {
		handler, s, mem := New(f.Leaves(), f.Send())
		return handlertest.Server{
			Handler: handler,
			Store:   s,
			// 通知を読む op は仕様にないので、メモリの上の状態を直に数える
			Notifications: func() int { return len(mem.Notifications()) },
		}
	})
}

// TestStore は、人がメモリの上に書いた store の op の振る舞い。onsqlite の TestStore と同じ storetest.Run を走らせる
// （server は組み立てない。gen/storemem と memstore だけで動く）。
func TestStore(t *testing.T) {
	storetest.Run(t, func(t *testing.T) storetest.Ops {
		memstore.Setup()
		return store.New()
	})
}
