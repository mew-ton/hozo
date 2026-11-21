// 入力 routes の、名前空間 flow の、インテグレーションテストの口を動かす（plugins/go-http の TestHTTPMadeCases が、モジュールのルートに
// 置いて走らせる）。確かめること（docs/plugin-go-http.md「インテグレーションテストの口」の「必要なケース」）: 人が書くのは、正常と、
// op を呼ぶ前の失敗だけ。例外のケースは、Hozo が、正常のケースから作り、走って、通る:
//
//   - flow.add/TooBig/flow.add.Big: 述語で決まる分類の match の、throw のアーム。判定のにせものが、値によらず、Big だと言う
//     （本物の述語 sizeCases は、"pen" を Small と言う）。
//   - flow.add/Taken/flow.addItem: effect fn が投げる例外。状態の単位のにせものが、投げる。
package behavior

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/app/gen/flow"
	"example.com/app/gen/flow/flowtest"
)

// sizeCases は、分類 flow.Size の述語の、本物: 識別子が、三文字より長ければ Big。
type sizeCases struct{}

func (sizeCases) Big(_ context.Context, v flow.Item) (bool, error)   { return len(v.Id) > 3, nil }
func (sizeCases) Small(_ context.Context, v flow.Item) (bool, error) { return len(v.Id) <= 3, nil }

type (
	flowCases struct{}
	addCases  struct{}
)

var _ flowtest.HTTPCases = flowCases{}

func (flowCases) Add() flowtest.AddHTTPCases { return addCases{} }

func (addCases) OK() flowtest.AddHTTPCase {
	return flowtest.AddHTTPCase{
		In: flow.AddInput{Item: flow.Item{Id: "pen"}},
		Then: func(t *testing.T, _ *httptest.ResponseRecorder, f *flowtest.Fakes) {
			if calls := f.Items.AddItem.Calls; len(calls) != 1 || calls[0].Item.Id != "pen" {
				t.Errorf("addItem の呼び出し = %+v（期待は、品物 pen で、一度）", calls)
			}
		},
	}
}

func (addCases) BadRequest() flowtest.AddHTTPCase {
	return flowtest.AddHTTPCase{
		Request: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader("{")) },
		Then: func(t *testing.T, _ *httptest.ResponseRecorder, f *flowtest.Fakes) {
			if n := len(f.Items.AddItem.Calls); n != 0 {
				t.Errorf("addItem の呼び出し = %d 回（期待は、なし）", n)
			}
		},
	}
}

func TestFlowHTTPCases(t *testing.T) {
	flowtest.RunHTTP(t, sizeCases{}, flowCases{})
}
