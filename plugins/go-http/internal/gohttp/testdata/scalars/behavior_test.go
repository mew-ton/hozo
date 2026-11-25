// 入力 scalars の出力を動かす（plugins/go-http の TestBehavior と、要求のテスト TestRequirements の ScalarJSON が、基底 go・
// go-valid・go-http の出力を書いたモジュールのルートに置いて走らせる）。確かめること（要求 hozo.gohttp.Requirement.ScalarJSON。
// 持ち主は docs/plugin-go-http.md「補助」の「union の読み書き」、docs/plugin-http.md「値の JSON」）:
//
//   - 元が日時の核の型の scalar（一段の Stamp、二段の Moment、date の Day、time の Clock）の値は、通信の上では、元の型の JSON。
//     request body を読むとき、結果を書くとき、例外の値を書くとき、省略可能と配列の中でも。
//   - 数の scalar（Count）は、元の型の JSON のまま（表に載らない）。
package behavior

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/app/gen/app"
	"github.com/mew-ton/hozo/runtime"
)

// leaves は、葉の op。受け取った入力を覚え、決めた値を返す。
type leaves struct {
	put   app.PutInput
	whole app.WholeInput
	err   error
}

func (l *leaves) Put(_ context.Context, in app.PutInput) (app.Event, error) {
	l.put = in
	return in.Event, l.err
}

func (l *leaves) Whole(_ context.Context, in app.WholeInput) (app.Stamp, error) {
	l.whole = in
	return in.Event.At, l.err
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

const event = `{"at":"2026-01-02T03:04:05Z","moment":"2026-01-02T03:04:05.5+09:00","day":"2026-11-21","clock":"09:30:00.25","count":3,"later":"2027-01-01T00:00:00Z","days":["2026-01-01","2026-01-02"]}`

// TestScalarJSON は、要求 hozo.gohttp.Requirement.ScalarJSON（plugins/go-http/go-http.hozo）。
func TestScalarJSON(t *testing.T) {
	l := &leaves{}
	h := app.NewHandler(app.New(l))

	// request body を読む: 元の型の JSON（文字列）から、scalar の値に
	rec := post(h, "/events", `{"event":`+event+`,"at":"2026-05-06T07:08:09Z"}`)
	if rec.Code != 200 {
		t.Fatalf("POST /events: %d %s", rec.Code, rec.Body)
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := l.put
	if !time.Time(got.Event.At).Equal(at) || !time.Time(got.At).Equal(time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)) {
		t.Errorf("読んだ Stamp と Moment = %v、%v", time.Time(got.Event.At), time.Time(got.At))
	}
	if !time.Time(got.Event.Moment).Equal(time.Date(2026, 1, 2, 3, 4, 5, 5e8, time.FixedZone("", 9*3600))) {
		t.Errorf("読んだ Moment（二段の scalar）= %v", time.Time(got.Event.Moment))
	}
	if runtime.Date(got.Event.Day) != runtime.NewDate(2026, 11, 21) || runtime.TimeOfDay(got.Event.Clock) != runtime.NewTimeOfDay(9, 30, 0, 25e7) {
		t.Errorf("読んだ Day と Clock = %v、%v", runtime.Date(got.Event.Day), runtime.TimeOfDay(got.Event.Clock))
	}
	if got.Event.Count != 3 || got.Event.Later == nil || len(got.Event.Days) != 2 || runtime.Date(got.Event.Days[1]) != runtime.NewDate(2026, 1, 2) {
		t.Errorf("読んだ Count、Later、Days = %v、%v、%v", got.Event.Count, got.Event.Later, got.Event.Days)
	}

	// 結果を書く: scalar の値を、元の型の JSON（文字列）に。読んだものと同じ値（時刻帯と、小数部の書き方は、元の型の JSON のとおり）
	want := `{"at":"2026-01-02T03:04:05Z","moment":"2026-01-02T03:04:05.5+09:00","day":"2026-11-21","clock":"09:30:00.25","count":3,"later":"2027-01-01T00:00:00Z","days":["2026-01-01","2026-01-02"]}`
	if body := strings.TrimSpace(rec.Body.String()); body != want {
		t.Errorf("結果の JSON = %s, want %s", body, want)
	}

	// 本文の全体（bodyObj）と、scalar そのものの結果
	rec = post(h, "/whole", event)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `"2026-01-02T03:04:05Z"` {
		t.Errorf("POST /whole: %d %s, want 200 \"2026-01-02T03:04:05Z\"", rec.Code, rec.Body)
	}

	// 例外の値を書く: 引数の scalar も、元の型の JSON
	l.err = app.Late{At: app.Stamp(at), Day: ptr(app.Day(runtime.NewDate(2026, 11, 21)))}
	rec = post(h, "/events", `{"event":`+event+`,"at":"2026-05-06T07:08:09Z"}`)
	if want := `{"error":"app.Late","value":{"at":"2026-01-02T03:04:05Z","day":"2026-11-21"}}`; rec.Code != 409 || strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("例外の応答 = %d %s, want 409 %s", rec.Code, rec.Body, want)
	}
	l.err = nil

	// 元の型の JSON でない値は、読めない値
	if rec := post(h, "/whole", `{"at":1,"moment":"2026-01-02T03:04:05Z","day":"2026-11-21","clock":"09:30:00","count":1,"days":[]}`); rec.Code != 400 {
		t.Errorf("数の Stamp: %d %s, want 400", rec.Code, rec.Body)
	}
}

func ptr[T any](v T) *T { return &v }
