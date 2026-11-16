package app

import (
	"context"
	"testing"
	"time"

	"example.com/shop-server/gen/server"
)

// TestLimiter は、人が書いた数え方: op ごとに、Window の間に Max 回まで。Window が過ぎれば、数え直す。
func TestLimiter(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	clock := now
	// 生成されたインターフェースを通して呼ぶ
	var limiter server.RateLimiter = &Limiter{Now: func() time.Time { return clock }, Max: 2, Window: time.Minute}
	ctx := context.Background()
	allow := func(op string, want bool) {
		t.Helper()
		if got := limiter.Allow(ctx, op); got != want {
			t.Errorf("%v 後の Allow(%s) = %v（期待は %v）", clock.Sub(now), op, got, want)
		}
	}
	allow("server.login", true)
	allow("server.login", true)
	allow("server.login", false)
	// op ごとに数える
	allow("server.other", true)
	// 幅の終わりの直前までは、超えたまま。断った呼び出しは数えない
	clock = now.Add(time.Minute - time.Nanosecond)
	allow("server.login", false)
	// 幅が過ぎれば、そこから数え直す
	clock = now.Add(time.Minute)
	allow("server.login", true)
	allow("server.login", true)
	allow("server.login", false)
	clock = now.Add(2*time.Minute - time.Nanosecond)
	allow("server.login", false)
	clock = now.Add(2 * time.Minute)
	allow("server.login", true)
}
