// 入力 routes の出力を動かす（plugins/go-http の TestBehavior が、基底 go・go-valid・go-http の出力を書いたモジュールのルートに
// 置いて走らせる）。確かめること（docs/plugin-go-http.md）:
//
//   - 「入力の読み方」: ヘッダ（必須、省略可能、既定値持ち。空の文字列は、ないもの）、クエリ（string / int / float / boolean と scalar。
//     必須、省略可能、既定値持ち。同じ名前が二度あれば最初の値）、本文（一つ、二つ以上。知らないキーは無視、ないキーは初めの値）。
//   - 「op を呼ぶ前の失敗」: 読めない要求は 400 `http.BadRequest` と決まった文言で、op を呼ばない。読めない要求が先で、次に制約の検査。
//   - 「制約の検査」: validate = true のときだけ。違反の path は、通信の上の場所。validate = false なら、確かめずに op を呼ぶ。
//   - 「成功の応答」: 戻り値があれば 200 と JSON（空の配列は []、省略可能でなければ null）、なければ 204。
//   - 「失敗の応答」: @http.status を持つ例外は、そのステータスと FQN（と値）。それ以外は 500 `internal`。ポインタで返した例外も 500。
//   - 経路にない要求は 404、メソッドの合わない要求は 405（net/http の既定）。/ で終わるパスは、そのパスだけに合う。
//   - op には、要求の context をそのまま渡す。
package behavior

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/app/gen/api"
	"example.com/app/gen/bare"
	"example.com/app/gen/strict"
	pkgtype "example.com/app/gen/type"
)

type ctxKey struct{}

// apiLeaves は api の葉の op。呼ばれた op と入力を覚え、決めた値を返す。
type apiLeaves struct {
	calls   []string
	ctx     context.Context
	create  api.CreateInput
	search  api.SearchInput
	lookup  pkgtype.Lookup
	replace api.ReplaceInput
	items   []pkgtype.Item
	first   *pkgtype.Item
	err     error
}

func (l *apiLeaves) called(ctx context.Context, name string) {
	l.calls, l.ctx = append(l.calls, name), ctx
}

func (l *apiLeaves) Health(ctx context.Context) (string, error) {
	l.called(ctx, "health")
	return "ok", l.err
}
func (l *apiLeaves) Create(ctx context.Context, in api.CreateInput) (pkgtype.Item, error) {
	l.called(ctx, "create")
	l.create = in
	return in.Item, l.err
}
func (l *apiLeaves) Search(ctx context.Context, in api.SearchInput) ([]pkgtype.Item, error) {
	l.called(ctx, "search")
	l.search = in
	return l.items, l.err
}
func (l *apiLeaves) First(ctx context.Context, in pkgtype.Lookup) (*pkgtype.Item, error) {
	l.called(ctx, "first")
	l.lookup = in
	return l.first, l.err
}
func (l *apiLeaves) Replace(ctx context.Context, in api.ReplaceInput) error {
	l.called(ctx, "replace")
	l.replace = in
	return l.err
}
func (l *apiLeaves) Reset(ctx context.Context) error {
	l.called(ctx, "reset")
	return l.err
}
func (l *apiLeaves) Ping(ctx context.Context) (bool, error) {
	l.called(ctx, "ping")
	return true, l.err
}
func (l *apiLeaves) Version() string                           { return "1" }
func (l *apiLeaves) Flaky(ctx context.Context) (string, error) { return "", nil }
func (l *apiLeaves) Helper(n int64) int64                      { return n }

// response は、応答の、比べるところ。
type response struct {
	status      int
	contentType string
	body        string
}

// call は、ハンドラ h に要求を一つ送る。target はパスとクエリ、headers は、名前と値を交互に。
func call(t *testing.T, h http.Handler, method, target, body string, headers ...string) response {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxKey{}, "marker"))
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{res.StatusCode, res.Header.Get("Content-Type"), string(data)}
}

// expectJSON は、応答が、ステータス status と、JSON の本文 body であることを確かめる。
func expectJSON(t *testing.T, got response, status int, body string) {
	t.Helper()
	if got.status != status || got.body != body || got.contentType != "application/json" {
		t.Errorf("応答 = %d %q（Content-Type %q）, want %d %q（application/json）", got.status, got.body, got.contentType, status, body)
	}
}

// badRequest は、読めない要求の本文（docs/plugin-go-http.md「op を呼ぶ前の失敗」）。
func badRequest(message string) string {
	return `{"error":"http.BadRequest","value":{"message":"` + message + `","violations":[]}}`
}

func newAPI() (*apiLeaves, http.Handler) {
	leaves := &apiLeaves{}
	return leaves, api.NewHandler(api.New(leaves))
}

func ptr[T any](v T) *T { return &v }

const createBody = `{"item":{"name":"pen","quantity":2,"tags":["a"]},"note":"memo","labels":{"k":"v"},"unknown":true}`

var createHeaders = []string{"X-Tenant", "acme", "Authorization", "Bearer abc"}

func TestNoInput(t *testing.T) {
	leaves, h := newAPI()
	expectJSON(t, call(t, h, "GET", "/health", ""), 200, `"ok"`)
	// op には、要求の context をそのまま渡す
	if leaves.ctx == nil || leaves.ctx.Value(ctxKey{}) != "marker" {
		t.Error("op に、要求の context が届いていない")
	}
	// 一つの op の二つの経路
	expectJSON(t, call(t, h, "GET", "/ping", ""), 200, `true`)
	expectJSON(t, call(t, h, "POST", "/ping", ""), 200, `true`)
	// 引数も戻り値もない: 204 で、本文はない
	if got := call(t, h, "POST", "/reset/", ""); got.status != 204 || got.body != "" {
		t.Errorf("POST /reset/ = %+v, want 204 で本文なし", got)
	}
	if want := []string{"health", "ping", "ping", "reset"}; !reflect.DeepEqual(leaves.calls, want) {
		t.Errorf("呼ばれた op = %v, want %v", leaves.calls, want)
	}
}

func TestRouting(t *testing.T) {
	leaves, h := newAPI()
	// 経路にない要求は 404、メソッドの合わない要求は 405（net/http の既定の応答）
	for target, want := range map[string]int{"GET /nowhere": 404, "DELETE /health": 405, "POST /health": 405,
		// @http.* のない op と、@http.retry だけの op は、経路にならない
		"GET /version": 404, "GET /flaky": 404,
		// / で終わるパスは、その下のパスには合わない
		"POST /reset/extra": 404} {
		method, path, _ := strings.Cut(target, " ")
		if got := call(t, h, method, path, ""); got.status != want {
			t.Errorf("%s = %d, want %d", target, got.status, want)
		}
	}
	if len(leaves.calls) != 0 {
		t.Errorf("経路にない要求で、op が呼ばれた: %v", leaves.calls)
	}
	// ルートのパス / は、ルートだけに合う
	root := bare.NewHandler(bare.New(bareLeaves{}))
	expectJSON(t, call(t, root, "GET", "/", ""), 200, `"root"`)
	if got := call(t, root, "GET", "/anything", ""); got.status != 404 {
		t.Errorf("GET /anything = %d, want 404", got.status)
	}
}

func TestHeadersAndBodies(t *testing.T) {
	leaves, h := newAPI()
	got := call(t, h, "POST", "/items?dryRun=true", createBody, createHeaders...)
	// 戻り値の JSON: キーはフィールド名、省略可能で値がないフィールドは省く
	expectJSON(t, got, 200, `{"name":"pen","quantity":2,"tags":["a"]}`)
	want := api.CreateInput{
		Tenant: "acme", Token: "Bearer abc", // ヘッダの値は、そのまま型にする（前置きを外さない）
		RequestId: nil,  // 省略可能なヘッダで、ない
		Lang:      "en", // 既定値持ちのヘッダで、ない: 初めの値のまま
		Item:      pkgtype.Item{Name: "pen", Quantity: 2, Tags: []string{"a"}},
		DryRun:    true,
		Note:      ptr("memo"),
		Copies:    1, // 本文にないキーは、初めの値（既定値）のまま
		Labels:    map[string]pkgtype.Name{"k": "v"},
	}
	if !reflect.DeepEqual(leaves.create, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.create, want)
	}

	// すべてを送る。省略可能な本文のフィールドの null は「ない」
	call(t, h, "POST", "/items?dryRun=0", `{"item":{"name":"x","quantity":1,"tags":null,"at":"2024-01-02T03:04:05Z"},"note":null,"copies":3,"labels":null}`,
		"X-Tenant", "t", "Authorization", "a", "X-Request-Id", "r1", "Accept-Language", "ja")
	at := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	want = api.CreateInput{Tenant: "t", Token: "a", RequestId: ptr("r1"), Lang: "ja",
		Item: pkgtype.Item{Name: "x", Quantity: 1, At: &at}, Copies: 3}
	if !reflect.DeepEqual(leaves.create, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.create, want)
	}

	// validate = false: 制約に合わない入力でも、確かめずに op を呼ぶ
	leaves.calls = nil
	expectJSON(t, call(t, h, "POST", "/items?dryRun=true", `{"item":{"name":"","quantity":0,"tags":[]}}`, createHeaders...), 200, `{"name":"","quantity":0,"tags":[]}`)
	if len(leaves.calls) != 1 {
		t.Errorf("validate = false なのに、op が呼ばれなかった")
	}
}

func TestUnreadableRequests(t *testing.T) {
	leaves, h := newAPI()
	for name, tc := range map[string]struct {
		target, body string
		headers      []string
		message      string
	}{
		"必須のヘッダがない":          {"/items?dryRun=true", createBody, []string{"Authorization", "a"}, "header X-Tenant: missing"},
		"必須のヘッダが空":           {"/items?dryRun=true", createBody, []string{"X-Tenant", "", "Authorization", "a"}, "header X-Tenant: missing"},
		"必須の scalar のヘッダがない": {"/items?dryRun=true", createBody, []string{"X-Tenant", "t"}, "header Authorization: missing"},
		"本文が JSON でない":       {"/items?dryRun=true", `{"item":`, createHeaders, "body: invalid JSON"},
		"本文が空":               {"/items?dryRun=true", ``, createHeaders, "body: invalid JSON"},
		"本文の型が違う":            {"/items?dryRun=true", `{"copies":"three"}`, createHeaders, "body: invalid JSON"},
		"本文がオブジェクトでない":       {"/items?dryRun=true", `[1]`, createHeaders, "body: invalid JSON"},
		"必須のクエリがない":          {"/items", createBody, createHeaders, "query dryRun: missing"},
		"boolean のクエリが読めない":  {"/items?dryRun=maybe", createBody, createHeaders, "query dryRun: invalid boolean"},
		// フィールドの順に読む: 先のフィールド（ヘッダ）の失敗が先
		"ヘッダも本文も読めない": {"/items", `{`, nil, "header X-Tenant: missing"},
		"本文もクエリも読めない": {"/items", `{`, createHeaders, "body: invalid JSON"},
	} {
		expectJSON(t, call(t, h, "POST", tc.target, tc.body, tc.headers...), 400, badRequest(tc.message))
		if len(leaves.calls) != 0 {
			t.Fatalf("%s: 読めない要求で、op が呼ばれた", name)
		}
	}
	for target, message := range map[string]string{
		"/items?offset=0&min=1":               "query q: missing",
		"/items?q=x&min=1":                    "query offset: missing",
		"/items?q=x&offset=0":                 "query min: missing",
		"/items?q=x&offset=zero&min=1":        "query offset: invalid int",
		"/items?q=x&offset=1.5&min=1":         "query offset: invalid int",
		"/items?q=x&offset=0&min=1&page=":     "query page: invalid int",
		"/items?q=x&offset=0&min=1&limit=a":   "query limit: invalid int",
		"/items?q=x&offset=0&min=1&size=a":    "query size: invalid int",
		"/items?q=x&offset=0&min=one":         "query min: invalid float",
		"/items?q=x&offset=0&min=1&ratio=r":   "query ratio: invalid float",
		"/items?q=x&offset=0&min=1&max=m":     "query max: invalid float",
		"/items?q=x&offset=0&min=1&scale=s":   "query scale: invalid float",
		"/items?q=x&offset=0&min=1&exact=yes": "query exact: invalid boolean",
		"/items?q=x&offset=0&min=1&deep=2":    "query deep: invalid boolean",
		"/items?q=x&offset=0&min=1&flag=":     "query flag: invalid boolean",
		"/items/first":                        "query id: missing",
	} {
		expectJSON(t, call(t, h, "GET", target, ""), 400, badRequest(message))
		if len(leaves.calls) != 0 {
			t.Fatalf("%s: 読めない要求で、op が呼ばれた", target)
		}
	}
	for body, name := range map[string]string{`{}`: "配列でない", `[{"quantity":"x"}]`: "要素の型が違う", `[`: "JSON でない"} {
		expectJSON(t, call(t, h, "POST", "/items/replace", body), 400, badRequest("body: invalid JSON"))
		if len(leaves.calls) != 0 {
			t.Fatalf("本文が%s: 読めない要求で、op が呼ばれた", name)
		}
	}
}

func TestQuery(t *testing.T) {
	leaves, h := newAPI()
	// 必須のものだけ: 既定値持ちは初めの値のまま、省略可能は nil。結果の空の配列は、必ず [] で返す
	expectJSON(t, call(t, h, "GET", "/items?q=&offset=0&min=1.5", ""), 200, `[]`)
	want := api.SearchInput{Q: "", Owner: "nobody", Page: 1, Offset: 0, Min: 1.5, Ratio: 0.5, Exact: false}
	if !reflect.DeepEqual(leaves.search, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.search, want)
	}
	// すべて。同じ名前が二度あれば、最初の値。float は指数も読む
	leaves.items = []pkgtype.Item{{Name: "a", Quantity: 1}}
	got := call(t, h, "GET", "/items?q=pen%20case&name=n&tag=first&tag=second&owner=me&page=3&limit=10&size=5&offset=-7&min=0.25&ratio=2&max=1e3&scale=0.1&exact=true&deep=false&flag=1", "")
	expectJSON(t, got, 200, `[{"name":"a","quantity":1,"tags":null}]`)
	want = api.SearchInput{Q: "pen case", Name: ptr(pkgtype.Name("n")), Tag: ptr("first"), Owner: "me", Page: 3, Limit: ptr(int64(10)),
		Size: ptr(pkgtype.Page(5)), Offset: -7, Min: 0.25, Ratio: 2, Max: ptr(1000.0), Scale: ptr(pkgtype.Ratio(0.1)),
		Exact: true, Deep: ptr(false), Flag: ptr(pkgtype.Flag(true))}
	if !reflect.DeepEqual(leaves.search, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.search, want)
	}
	// 省略可能な string のクエリは、空の値でも「ある」
	call(t, h, "GET", "/items?q=x&offset=0&min=1&tag=", "")
	if leaves.search.Tag == nil || *leaves.search.Tag != "" {
		t.Errorf("tag = %v, want 空の文字列へのポインタ", leaves.search.Tag)
	}
}

func TestResults(t *testing.T) {
	leaves, h := newAPI()
	// 別の名前空間の入力の model。省略可能な戻り値で、値がなければ null
	expectJSON(t, call(t, h, "GET", "/items/first?id=7", "", "X-Trace", "t1"), 200, `null`)
	if want := (pkgtype.Lookup{Id: "7", Trace: ptr("t1")}); !reflect.DeepEqual(leaves.lookup, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.lookup, want)
	}
	leaves.first = &pkgtype.Item{Name: "a", Quantity: 1, Tags: []string{}}
	expectJSON(t, call(t, h, "GET", "/items/first?id=7", ""), 200, `{"name":"a","quantity":1,"tags":[]}`)
	// 本文が一つ（配列）。戻り値がなければ 204
	if got := call(t, h, "POST", "/items/replace", `[{"name":"a","quantity":1,"tags":["x"],"extra":1}]`); got.status != 204 || got.body != "" {
		t.Errorf("POST /items/replace = %+v, want 204 で本文なし", got)
	}
	if want := (api.ReplaceInput{Items: []pkgtype.Item{{Name: "a", Quantity: 1, Tags: []string{"x"}}}}); !reflect.DeepEqual(leaves.replace, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.replace, want)
	}
	// 配列の null は、空の配列と読む
	if got := call(t, h, "POST", "/items/replace", `null`); got.status != 204 || len(leaves.replace.Items) != 0 {
		t.Errorf("POST /items/replace（null）= %+v、入力 %+v", got, leaves.replace)
	}
}

func TestErrors(t *testing.T) {
	leaves, h := newAPI()
	for name, tc := range map[string]struct {
		err    error
		status int
		body   string
	}{
		"値を運ばない例外":      {pkgtype.NotFound{}, 404, `{"error":"type.NotFound"}`},
		"その場で書いた値を運ぶ例外": {pkgtype.Conflict{Value: pkgtype.ConflictValue{Existing: "pen", Count: 2}}, 409, `{"error":"type.Conflict","value":{"existing":"pen","count":2}}`},
		"model の値を運ぶ例外": {pkgtype.Rejected{Value: pkgtype.Item{Name: "x", Quantity: 1}}, 422, `{"error":"type.Rejected","value":{"name":"x","quantity":1,"tags":null}}`},
		"包んだ例外":         {fmt.Errorf("wrapped: %w", pkgtype.NotFound{}), 404, `{"error":"type.NotFound"}`},
		// それ以外は 500 で、中身を出さない
		"ステータスを持たない例外": {pkgtype.Broken{}, 500, `{"error":"internal"}`},
		"ほかのエラー":       {errors.New("secret detail"), 500, `{"error":"internal"}`},
		"中断":           {context.Canceled, 500, `{"error":"internal"}`},
		// 例外は値の型で見分ける: ポインタで返せば、見分けられずに 500
		"ポインタで返した例外": {&pkgtype.NotFound{}, 500, `{"error":"internal"}`},
	} {
		leaves.err = tc.err
		// 経路の op のどれかが投げうる例外は、どの経路でも同じに見分ける（補助は一つ）
		for _, req := range [][2]string{{"GET", "/items/first?id=1"}, {"GET", "/health"}, {"POST", "/reset/"}} {
			got := call(t, h, req[0], req[1], "")
			if got.status != tc.status || got.body != tc.body || got.contentType != "application/json" {
				t.Errorf("%s（%s %s）: 応答 = %+v, want %d %s", name, req[0], req[1], got, tc.status, tc.body)
			}
		}
	}
}

// strictLeaves は strict の葉の op。
type strictLeaves struct {
	calls []string
	order strict.OrderInput
	edit  strict.EditInput
}

func (l *strictLeaves) Order(ctx context.Context, in strict.OrderInput) (int64, error) {
	l.calls, l.order = append(l.calls, "order"), in
	return int64(len(in.Lines)), nil
}
func (l *strictLeaves) Edit(ctx context.Context, in strict.EditInput) error {
	l.calls, l.edit = append(l.calls, "edit"), in
	return nil
}
func (l *strictLeaves) Plain(ctx context.Context, in strict.PlainInput) (string, error) {
	l.calls = append(l.calls, "plain")
	return "plain", nil
}

// invalid は、制約に合わない入力の本文（docs/plugin-go-http.md「op を呼ぶ前の失敗」）。violations は、path と constraint を交互に。
func invalid(violations ...string) string {
	var parts []string
	for i := 0; i+1 < len(violations); i += 2 {
		parts = append(parts, `{"path":"`+violations[i]+`","constraint":"`+violations[i+1]+`"}`)
	}
	return `{"error":"http.BadRequest","value":{"message":"invalid input","violations":[` + strings.Join(parts, ",") + `]}}`
}

func TestValidate(t *testing.T) {
	leaves := &strictLeaves{}
	h := strict.NewHandler(strict.New(leaves))
	// 本文が一つ: 違反の path は body と、その中のパス（入力の model のフィールド名 lines は出さない）。確かめた順
	got := call(t, h, "POST", "/orders", `[{"name":"a","quantity":1},{"name":"","quantity":0}]`, "Authorization", "t")
	expectJSON(t, got, 400, invalid("body[1].name", "length", "body[1].quantity", "min"))
	// 本文が二つ以上: body.<フィールド名>。クエリの値の制約も、通信の上の場所で
	got = call(t, h, "POST", "/edit?page=0", `{"item":{"name":"","quantity":1},"extra":{"name":"x","quantity":0}}`, "X-Name", "n")
	expectJSON(t, got, 400, invalid("query.page", "min", "body.item.name", "length", "body.extra.quantity", "min"))
	// 読めない要求が先で、次に制約の検査
	expectJSON(t, call(t, h, "POST", "/edit?page=0", `{`), 400, badRequest("body: invalid JSON"))
	expectJSON(t, call(t, h, "POST", "/edit?page=x", `{"item":{"name":"","quantity":0}}`), 400, badRequest("query page: invalid int"))
	if len(leaves.calls) != 0 {
		t.Fatalf("制約に合わない入力か、読めない要求で、op が呼ばれた: %v", leaves.calls)
	}
	// 制約に合う入力は、op に届く
	expectJSON(t, call(t, h, "POST", "/orders", `[{"name":"a","quantity":1},{"name":"b","quantity":2}]`), 200, `2`)
	if got := call(t, h, "POST", "/edit", `{"item":{"name":"a","quantity":1}}`); got.status != 204 {
		t.Errorf("POST /edit = %+v, want 204", got)
	}
	if leaves.edit.Page != 1 || leaves.edit.Name != nil || leaves.edit.Extra != nil || leaves.edit.Item.Name != "a" {
		t.Errorf("入力 = %+v", leaves.edit)
	}
	// Validate を持たない入力は、確かめずに呼ぶ
	expectJSON(t, call(t, h, "GET", "/plain", ""), 200, `"plain"`)
	if want := []string{"order", "edit", "plain"}; !reflect.DeepEqual(leaves.calls, want) {
		t.Errorf("呼ばれた op = %v, want %v", leaves.calls, want)
	}
}

// bareLeaves は bare の葉の op。
type bareLeaves struct{}

func (bareLeaves) Root(ctx context.Context) (string, error) { return "root", nil }
func (bareLeaves) Touch(ctx context.Context) error          { return bare.Unknown{} }
func (bareLeaves) ViaAlias(ctx context.Context) (string, error) {
	return "pong", nil
}

// @http.status を持つ例外が一つもない名前空間: どのエラーも 500。
func TestNoStatus(t *testing.T) {
	h := bare.NewHandler(bare.New(bareLeaves{}))
	expectJSON(t, call(t, h, "POST", "/touch", ""), 500, `{"error":"internal"}`)
	// 名前付きデコレータを通して付けた @http.get の経路
	expectJSON(t, call(t, h, "GET", "/pong", ""), 200, `"pong"`)
}

// NewHandler が受け取るのは、入口の Ops（New の戻り値を、そのまま渡す）。http.Handler を返す。
var _ func(api.Ops) http.Handler = api.NewHandler
