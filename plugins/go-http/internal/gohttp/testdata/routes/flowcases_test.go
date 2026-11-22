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
//   - flow.announce/caught/Undelivered/flow.notify: 受ける道。try の中の葉 notify のにせものが、投げ、catch が受けて、成功の応答になる
//     （本物の葉 notify は、投げない）。
//   - flow.reannounce/Undelivered/flow.notify: try の外の notify が投げる道。葉のにせものが、投げる（どの呼び出しでも）。
//   - flow.reannounce/caught/Undelivered/flow.notify: 人が書く、受ける道（口のメソッド CaughtUndeliveredNotify）。同じ葉を、try の外でも
//     呼ぶので、Hozo は、作れない。本物の葉 notify が、品物 "second" の、二度目の呼び出しでだけ、投げる。
//   - flow.relay/Unknown/bare.touch: 相手の入口 bare.touch（流れの端）のにせものが、相手の op の例外を投げる。
//   - flow.relay/ServerError/bare.touch@http.post、flow.relay/NetworkError/bare.touch@http.post: 相手の境界で起きる例外
//     （@http.post の outer throws）。同じにせものが、投げる。
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

// notifyCalls は、葉 notify が、品物 "second" で呼ばれた回数（受ける道のケースが、Given で、0 に戻す）。
var notifyCalls int

// Notify は、届く。品物 "second" の、二度目の呼び出しでだけ、届かない（人が書く、受ける道のケースが、使う）。
// NoteFailure は、何もしない（呼ばれたことは、にせものが覚える）。
func (flowLeaves) Notify(_ context.Context, item flow.Item) error {
	if item.Id == "second" {
		if notifyCalls++; notifyCalls == 2 {
			return flow.Undelivered{}
		}
	}
	return nil
}
func (flowLeaves) NoteFailure(context.Context, flow.Item) error { return nil }

type (
	flowCases       struct{}
	addCases        struct{}
	shelveCases     struct{}
	announceCases   struct{}
	reannounceCases struct{}
	relayCases      struct{}
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

func (flowCases) Announce() flowtest.AnnounceHTTPCases { return announceCases{} }

// OK: 葉 notify が届くので、catch には、入らない（noteFailure は、呼ばれない）。
func (announceCases) OK() flowtest.AnnounceHTTPCase {
	return flowtest.AnnounceHTTPCase{
		In: flow.AddInput{Item: flow.Item{Id: "pen"}},
		Then: func(t *testing.T, _ *httptest.ResponseRecorder, f *flowtest.Fakes) {
			if n := len(f.Leaves.Notify.Calls); n != 1 {
				t.Errorf("notify の呼び出し = %d 回（期待は、一度）", n)
			}
			if n := len(f.Leaves.NoteFailure.Calls); n != 0 {
				t.Errorf("noteFailure の呼び出し = %d 回（期待は、なし）", n)
			}
		},
	}
}

func (announceCases) BadRequest() flowtest.AnnounceHTTPCase {
	return flowtest.AnnounceHTTPCase{Request: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader("{")) }}
}

func (flowCases) Reannounce() flowtest.ReannounceHTTPCases { return reannounceCases{} }

func (reannounceCases) OK() flowtest.ReannounceHTTPCase {
	return flowtest.ReannounceHTTPCase{In: flow.AddInput{Item: flow.Item{Id: "pen"}}}
}

func (reannounceCases) BadRequest() flowtest.ReannounceHTTPCase {
	return flowtest.ReannounceHTTPCase{Request: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader("{")) }}
}

// CaughtUndeliveredNotify: try の中の notify（二度目の呼び出し）だけが、投げる。catch が受けて、noteFailure を呼び、成功の応答になる。
func (reannounceCases) CaughtUndeliveredNotify() flowtest.ReannounceHTTPCase {
	return flowtest.ReannounceHTTPCase{
		In:    flow.AddInput{Item: flow.Item{Id: "second"}},
		Given: func(*flowtest.Fakes) { notifyCalls = 0 },
		Then: func(t *testing.T, _ *httptest.ResponseRecorder, f *flowtest.Fakes) {
			if n := len(f.Leaves.Notify.Calls); n != 2 {
				t.Errorf("notify の呼び出し = %d 回（期待は、二度）", n)
			}
			if n := len(f.Leaves.NoteFailure.Calls); n != 1 {
				t.Errorf("noteFailure の呼び出し = %d 回（期待は、一度: catch が、受けた）", n)
			}
		},
	}
}

func (flowCases) Relay() flowtest.RelayHTTPCases { return relayCases{} }

// OK: 相手の入口 bare.touch は、にせもの（言われていなければ、零値を返す）。一度、呼ばれる。
func (relayCases) OK() flowtest.RelayHTTPCase {
	return flowtest.RelayHTTPCase{
		Then: func(t *testing.T, _ *httptest.ResponseRecorder, f *flowtest.Fakes) {
			if n := len(f.Bare.Touch.Calls); n != 1 {
				t.Errorf("bare.touch の呼び出し = %d 回（期待は、一度）", n)
			}
		},
	}
}

func TestFlowHTTPCases(t *testing.T) {
	flowtest.RunHTTP(t, flowLeaves{}, hoursCases{}, sizeCases{}, flowCases{})
}
