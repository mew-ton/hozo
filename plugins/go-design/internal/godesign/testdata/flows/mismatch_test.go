// 入力 flows の、E2E テストの口が、Given かフローが失敗を返したときに、落ちること（plugins/go-design の TestMismatch が、behavior_test.go と
// 同じモジュールのルートに置いて走らせ、落ちたテストと、その文面を確かめる）。このファイルのテストは、落ちるのが、期待。
package behavior

import (
	"context"
	"testing"

	"example.com/app/gen/act"
	"example.com/app/gen/flows"
	"example.com/app/gen/flows/flowstest"
)

// mismatchCases は、flowCases のうち、二つのケースを、正常に終わらないように、差し替えたもの。差し替えたケースの Then は、呼ばれない
// （呼ばれれば、"Then was called" で落ちる）。
type mismatchCases struct{ flowCases }

// Given が、失敗を返す: フローは、呼ばれない
func (c mismatchCases) Checkout() flowstest.CheckoutFlowCase {
	return flowstest.CheckoutFlowCase{
		Given: func(context.Context) error { return errBoom },
		Then:  func(t *testing.T, _ act.Receipt) { t.Error("Then was called") },
	}
}

// フローが、失敗を返す（操作の op が、失敗を返す）
func (c mismatchCases) Ping() flowstest.PingFlowCase {
	return flowstest.PingFlowCase{
		Given: func(context.Context) error { c.r.failTouch = errBoom; return nil },
		Then:  func(t *testing.T) { t.Error("Then was called") },
	}
}

// 差し替えていないケースの前に、失敗を止める
func (c mismatchCases) Rename() flowstest.RenameFlowCase {
	k := c.flowCases.Rename()
	k.Given = func(context.Context) error { c.r.failTouch = nil; return nil }
	return k
}

func TestFlowMismatch(t *testing.T) {
	r := &recorder{}
	flowstest.RunFlows(t, flows.New(act.New(actLeaves{r})), mismatchCases{flowCases{r}})
	for _, name := range r.log {
		if name == "buy" {
			t.Error("Given が失敗を返したのに、フローが呼ばれた")
		}
	}
}
