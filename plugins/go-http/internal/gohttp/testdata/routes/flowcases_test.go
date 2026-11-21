// 入力 routes の、名前空間 flow の、インテグレーションテストの口を動かす（plugins/go-http の TestHTTPMadeCases が、モジュールのルートに
// 置いて走らせる）。確かめること（docs/plugin-go-http.md「インテグレーションテストの口」の「必要なケース」）: 人が書くのは、正常と、
// op を呼ぶ前の失敗だけ。例外のケースは、Hozo が、正常のケースから作り、走って、通る:
//
//   - flow.add/TooBig/flow.add.Big: 述語で決まる分類の match の、throw のアーム。判定のにせものが、値によらず、Big だと言う
//     （本物の述語 sizeCases は、"pen" を Small と言う）。
//   - flow.add/Taken/flow.addItem: effect fn が投げる例外。状態の単位のにせものが、投げる。
//   - flow.shelve/Jammed/flow.checkDoor: 本体のあるデコレータ @flow.opened の本体が呼ぶ葉が、投げる例外。葉のにせものが、投げる。
//   - flow.shelve/Shut/flow.shelve@flow.opened.Late: デコレータの本体の match の、throw のアーム。判定のにせものが、Late だと言う
//     （本物の述語 hoursCases は、番 "day" を Open と言う）。
//   - flow.shelve/Taken/flow.addItem: デコレータの本体の proceed() の先（op の中身）の、effect fn が投げる例外。
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

// hoursCases は、分類 flow.Hours の述語の、本物: 番が "day" なら Open。
type hoursCases struct{}

func (hoursCases) Open(_ context.Context, v flow.Item) (bool, error) { return v.Id == "day", nil }
func (hoursCases) Late(_ context.Context, v flow.Item) (bool, error) { return v.Id != "day", nil }

// flowLeaves は、flow の、人が書く葉の、本物: 今の番は "day"。戸は、壊れていない。
type flowLeaves struct{}

func (flowLeaves) Duty(context.Context) (flow.Item, error) { return flow.Item{Id: "day"}, nil }
func (flowLeaves) CheckDoor(context.Context) error         { return nil }

type (
	flowCases   struct{}
	addCases    struct{}
	shelveCases struct{}
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

func (flowCases) Shelve() flowtest.ShelveHTTPCases { return shelveCases{} }

// OK: デコレータ @flow.opened の本体（本物）が、葉 checkDoor と duty を呼んでから、op の中身へ進む。
func (shelveCases) OK() flowtest.ShelveHTTPCase {
	return flowtest.ShelveHTTPCase{
		In: flow.AddInput{Item: flow.Item{Id: "cup"}},
		Then: func(t *testing.T, _ *httptest.ResponseRecorder, f *flowtest.Fakes) {
			if n := len(f.Leaves.CheckDoor.Calls); n != 1 {
				t.Errorf("checkDoor の呼び出し = %d 回（期待は、一度）", n)
			}
			if calls := f.Items.AddItem.Calls; len(calls) != 1 || calls[0].Item.Id != "cup" {
				t.Errorf("addItem の呼び出し = %+v（期待は、品物 cup で、一度）", calls)
			}
		},
	}
}

func (shelveCases) BadRequest() flowtest.ShelveHTTPCase {
	return flowtest.ShelveHTTPCase{Request: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader("{")) }}
}

func TestFlowHTTPCases(t *testing.T) {
	flowtest.RunHTTP(t, flowLeaves{}, hoursCases{}, sizeCases{}, flowCases{})
}
