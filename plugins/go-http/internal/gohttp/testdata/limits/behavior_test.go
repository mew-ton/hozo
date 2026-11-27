// 入力 limits の出力を動かす（plugins/go-http の TestBehavior と、要求のテスト TestRequirements の BodyLimit が、基底 go・
// go-valid・go-http の出力を書いたモジュールのルートに置いて走らせる）。確かめること（要求 hozo.gohttp.Requirement.BodyLimit。
// 持ち主は docs/plugin-go-http.md「オプション」の maxBodyBytes と「入力の読み方」、docs/plugin-http.md「op を呼ぶ前の失敗」）:
//
//   - request body が上限を超えた要求は、op を呼ばずに、413 と http.BadRequest の "body: too large"。本文の項目の形と、全体の形の、どちらも。
//   - 上限ちょうどの request body は、読む（op を呼ぶ）。
//   - 上限は、オプション maxBodyBytes（small は 64）。オプションがなければ 1 MiB（plain）。
package behavior

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/app/gen/plain"
	"example.com/app/gen/small"
)

// smallLeaves と plainLeaves は、葉の op。呼ばれた回数を覚える。
type smallLeaves struct{ calls int }

func (l *smallLeaves) Put(context.Context, small.PutInput) error     { l.calls++; return nil }
func (l *smallLeaves) Whole(context.Context, small.WholeInput) error { l.calls++; return nil }

type plainLeaves struct{ calls int }

func (l *plainLeaves) Put(context.Context, plain.PutInput) error     { l.calls++; return nil }
func (l *plainLeaves) Whole(context.Context, plain.WholeInput) error { l.calls++; return nil }

// sized は、ちょうど size バイトの request body: 読める JSON の後ろを、空白で埋める。
func sized(t *testing.T, size int) string {
	t.Helper()
	body := `{"name":"x","item":{"name":"x"}}`
	if size < len(body) {
		t.Fatalf("%d バイトの本文は作れない", size)
	}
	return body + strings.Repeat(" ", size-len(body))
}

// post は、h に path への POST を送り、応答を返す。
func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

// TestBodyLimit は、要求 hozo.gohttp.Requirement.BodyLimit（plugins/go-http/spec/go-http.hozo）。
func TestBodyLimit(t *testing.T) {
	const tooLarge = `{"error":"http.BadRequest","value":{"message":"body: too large","violations":[]}}`
	sl, pl := &smallLeaves{}, &plainLeaves{}
	for _, c := range []struct {
		name  string
		h     http.Handler
		limit int
		calls func() int
	}{
		{"small（maxBodyBytes = 64）", small.NewHandler(small.New(sl)), 64, func() int { return sl.calls }},
		{"plain（既定の 1 MiB）", plain.NewHandler(plain.New(pl)), 1 << 20, func() int { return pl.calls }},
	} {
		for _, path := range []string{"/put", "/whole"} {
			before := c.calls()
			rec := post(c.h, path, sized(t, c.limit+1))
			if rec.Code != http.StatusRequestEntityTooLarge || strings.TrimSpace(rec.Body.String()) != tooLarge {
				t.Errorf("%s POST %s（上限 + 1 バイト）: %d %s, want 413 %s", c.name, path, rec.Code, rec.Body, tooLarge)
			}
			if c.calls() != before {
				t.Errorf("%s POST %s（上限 + 1 バイト）: op が呼ばれた", c.name, path)
			}
			if rec := post(c.h, path, sized(t, c.limit)); rec.Code != http.StatusNoContent {
				t.Errorf("%s POST %s（上限ちょうど）: %d %s, want 204", c.name, path, rec.Code, rec.Body)
			}
			if c.calls() != before+1 {
				t.Errorf("%s POST %s（上限ちょうど）: op が呼ばれなかった", c.name, path)
			}
		}
	}
}
