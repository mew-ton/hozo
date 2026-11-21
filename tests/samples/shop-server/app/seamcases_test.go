package app

import (
	"testing"
	"time"

	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/servertest"
)

// このファイルは、デコレータのテスト（docs/ide-chain.md「テストの規約」の「デコレータのテスト」。Go の口は docs/plugin-go.md
// 「デコレータのテストの口」）: 継ぎ目 @rate.limited の実装（go-ratelimit が出した server.RateSeams と、人が書いた数え方 Limiter）
// を、結果ごとに確かめる。書いてあるのは、その結果になる状態の、継ぎ目の実装だけ: 継ぎ目を呼ぶ所と、結果の見分け（正常か、
// どの例外か。op の中身が、呼ばれたか）は、生成したコード（servertest.RunSeams）が持つ。ケースは、デコレータの宣言ごとに一組で、
// @rate.limited を、ほかの op に付けても、ここに足すものは、ない。

type seamCases struct{}

func (seamCases) RateLimited() servertest.RateLimitedCases { return rateLimitedCases{} }

type rateLimitedCases struct{}

// limited は、Window の間に max 回まで許す数え方の、継ぎ目。
func limited(max int) func() server.Seams {
	return func() server.Seams {
		return server.NewRateSeams(&Limiter{Now: func() time.Time { return leafNow }, Max: max, Window: time.Minute})
	}
}

// OK: まだ、一回、呼べる。
func (rateLimitedCases) OK() servertest.RateLimitedCase {
	return servertest.RateLimitedCase{Seams: limited(1)}
}

// LimitExceeded: もう、一回も、呼べない。
func (rateLimitedCases) LimitExceeded() servertest.RateLimitedCase {
	return servertest.RateLimitedCase{Seams: limited(0)}
}

// TestSeamCases は、継ぎ目ごと、結果ごとのケースを、すべて走らせる。テストの名前は、<op の FQN>@<デコレータの FQN>/<結果>
// （TestSeamCases/server.login@rate.limited/LimitExceeded など）。
func TestSeamCases(t *testing.T) {
	servertest.RunSeams(t, seamCases{})
}
