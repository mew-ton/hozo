package app

import (
	"context"
	"sync"
	"time"

	"example.com/shop-server/gen/server"
)

// Limiter は、@rate.limited の付いた op を呼べる頻度を決める: op ごとに、Window の間に Max 回まで。
// go-ratelimit の出力が受け取る server.RateLimiter の実装で、*Limiter を server.NewRateSeams にそのまま渡す。
// どう数えるか（何回まで、どの間隔で、何ごとに）は仕様にもプラグインにもなく、人が決める。ここでは、いちばん単純な、
// 区切った時間の中の回数。一つの *Limiter を、並行に呼んでよい。
type Limiter struct {
	// Now は今の時刻。テストが差し替える。
	Now func() time.Time
	// Max は、一つの op を Window の間に呼べる回数。
	Max int
	// Window は、回数を数える時間の幅。最初の呼び出しから始まり、過ぎた後の最初の呼び出しで、数え直す。
	Window time.Duration

	mu      sync.Mutex
	windows map[string]limiterWindow
}

// limiterWindow は、op 一つの、今の時間の幅の始まりと、その中で許した回数。
type limiterWindow struct {
	start time.Time
	count int
}

// *Limiter は、go-ratelimit の継ぎ目が呼ぶものをすべて持つ。
var _ server.RateLimiter = (*Limiter)(nil)

// Allow は、op（FQN）を今呼んでよければ true を返し、一回と数える。Window の間に Max 回を超える呼び出しは false で、数えない。
// op の中身が成功したかどうかは見ない（呼ぶ前に聞かれる）。
func (l *Limiter) Allow(_ context.Context, op string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	w, ok := l.windows[op]
	if !ok || !now.Before(w.start.Add(l.Window)) {
		w = limiterWindow{start: now}
	}
	if w.count >= l.Max {
		return false
	}
	w.count++
	if l.windows == nil {
		l.windows = map[string]limiterWindow{}
	}
	l.windows[op] = w
	return true
}
