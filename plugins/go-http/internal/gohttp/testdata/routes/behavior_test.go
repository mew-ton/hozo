// 入力 routes の出力を動かす（plugins/go-http の TestBehavior が、基底 go・go-valid・go-http の出力を書いたモジュールのルートに
// 置いて走らせる）。確かめること（docs/plugin-go-http.md）:
//
//   - 「入力の読み方」と「通信の形」の「要求」: 三つの場所（ヘッダ、クエリ、本文）の、項目の形（名前を書いたものと、省いたもの）と、
//     全体の形。項目がないときの三つの扱い（@http.required は 400、@http.default はその値、どちらもなければ零値）を、三つの場所で。
//     ヘッダの空の文字列は、ないもの。クエリの同じ名前が二度あれば最初の値。本文の項目の値が null なら、ないもの（要求 NullIsAbsent）。
//     読み方（文字列、整数、小数、真偽値。省略可能と scalar）は、補助が、実行時に、読む先の型で決める。
//   - 「op を呼ぶ前の失敗」: 読めない要求は 400 `http.BadRequest` と決まった文言で、op を呼ばない。読めない要求が先で、次に制約の検査。
//   - 「制約の検査」: validate = true のときだけ。違反の path は、通信の上の場所。validate = false なら、確かめずに op を呼ぶ。
//     Validate を持たない入力は、同じ補助を通って、そのまま op に届く。
//   - 「成功の応答」: 返す値があれば 200 と JSON（値のない配列は []、値のない、インデックスの model は {}、零値も値）、なければ 204 で本文なし
//     （戻り値のない op、省略可能な戻り値で値がないとき）。一つの op が、呼び出しによって 200 も 204 も返す。
//   - 「失敗の応答」: @http.status を持つ例外は、そのステータスと FQN。引数を持つ例外は、引数の名前をキーにしたオブジェクトを value に
//     （引数が一つでも包む。省略可能な引数で値がなければ、キーを省く）。引数のない例外に、value はない。それ以外（ステータスを
//     持たない例外は、引数を持っていても）は 500 `internal`。ポインタで返した例外も 500。
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
	"net/url"
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
	list    api.ListInput
	put     api.PutInput
	items   []pkgtype.Item
	first   *pkgtype.Item
	counts  api.Counts
	token   *pkgtype.Token
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
func (l *apiLeaves) ListAll(ctx context.Context, in api.ListInput) (api.Counts, error) {
	l.called(ctx, "listAll")
	l.list = in
	return l.counts, l.err
}
func (l *apiLeaves) Put(ctx context.Context, in api.PutInput) (*pkgtype.Token, error) {
	l.called(ctx, "put")
	l.put = in
	return l.token, l.err
}
func (l *apiLeaves) Reset(ctx context.Context) error {
	l.called(ctx, "reset")
	return l.err
}
func (l *apiLeaves) Ping(ctx context.Context) (bool, error) {
	l.called(ctx, "ping")
	return true, l.err
}
func (l *apiLeaves) Count(ctx context.Context) (int64, error) {
	l.called(ctx, "count")
	return 0, l.err
}
func (l *apiLeaves) Version(context.Context) (string, error)          { return "1", nil }
func (l *apiLeaves) Flaky(ctx context.Context) (string, error)        { return "", nil }
func (l *apiLeaves) Helper(_ context.Context, n int64) (int64, error) { return n, nil }

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

// expectEmpty は、応答が、204 で、本文も Content-Type もないことを確かめる。
func expectEmpty(t *testing.T, got response) {
	t.Helper()
	if got.status != 204 || got.body != "" || got.contentType != "" {
		t.Errorf("応答 = %d %q（Content-Type %q）, want 204 で本文なし", got.status, got.body, got.contentType)
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

// createBody は、POST /items の、@http.required の項目（labels）だけを持つ本文。
const createBody = `{"labels":{"k":"v"}}`

// createHeaders は、POST /items の、@http.required のヘッダ（X-Tenant）だけ。
var createHeaders = []string{"X-Tenant", "acme"}

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
	// 零値の結果も、値: 200
	expectJSON(t, call(t, h, "GET", "/count", ""), 200, `0`)
	// 引数も戻り値もない: 204 で、本文はない
	expectEmpty(t, call(t, h, "POST", "/reset/", ""))
	if want := []string{"health", "ping", "ping", "count", "reset"}; !reflect.DeepEqual(leaves.calls, want) {
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
		got := call(t, h, method, path, "")
		if got.status != want || got.contentType == "application/json" {
			t.Errorf("%s = %d（Content-Type %q）, want %d（JSON ではない）", target, got.status, got.contentType, want)
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

// ヘッダと本文の項目: 名前を書いたものと省いたもの、項目がないときの三つの扱い。
// TestNullIsAbsent は、要求 hozo.gohttp.Requirement.NullIsAbsent（plugins/go-http/go-http.hozo）: 本文の項目の値が null なら、
// その項目を送らなかったものと読む。@http.required の項目は 400 の missing、@http.default の項目は、その値、どちらもなければ零値。
func TestNullIsAbsent(t *testing.T) {
	leaves, h := newAPI()
	// @http.required の項目（labels）の null は、ないのと同じ: 400 で、op を呼ばない
	expectJSON(t, call(t, h, "POST", "/items?dryRun=true", `{"labels":null}`, createHeaders...),
		400, `{"error":"http.BadRequest","value":{"message":"body labels: missing","violations":[]}}`)
	if len(leaves.calls) != 0 {
		t.Fatalf("op が呼ばれた: %v", leaves.calls)
	}
	// @http.default の項目（copies、active）の null は、その値。どちらもない項目（memo、item）の null は、零値
	expectJSON(t, call(t, h, "POST", "/items?dryRun=true", `{"labels":{},"copies":null,"active":null,"memo":null,"item":null}`, createHeaders...),
		200, `{"name":"","quantity":0,"tags":null}`)
	if c := leaves.create; c.Copies != 1 || !c.Active || c.Note != nil || !reflect.DeepEqual(c.Item, pkgtype.Item{}) || c.Weight != 2 {
		t.Errorf("null の項目: 入力 = %+v, want copies は 1、active は true（null は、ないもの）、memo と item は零値、weight は 2（キーがない）", c)
	}
}

func TestHeadersAndBodies(t *testing.T) {
	leaves, h := newAPI()
	// @http.required の項目だけを送る
	got := call(t, h, "POST", "/items?dryRun=true", createBody, createHeaders...)
	// 戻り値の JSON: キーはフィールド名、省略可能で値がないフィールドは省く
	expectJSON(t, got, 200, `{"name":"","quantity":0,"tags":null}`)
	want := api.CreateInput{
		Tenant: "acme",
		// どちらもない項目は、零値のまま（省略可能なフィールドは、値がない）
		Token: "", RequestId: nil, Trace: nil, Item: pkgtype.Item{}, Note: nil,
		// @http.default の項目は、その値（補助が、読む先の型に変える: 整数のリテラルを float に、文字列を scalar に、期間を Duration に）
		Lang: "en", Copies: 1, Weight: 2, Scale: 1.5, Active: true, Timeout: 30 * time.Second, Wait: pkgtype.Wait(5 * time.Minute), Owner: "anon",
		DryRun: true,
		Labels: map[string]pkgtype.Name{"k": "v"},
	}
	if !reflect.DeepEqual(leaves.create, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.create, want)
	}

	// すべてを送る。名前を省いた項目は、フィールドの名前（ヘッダ trace、本文 item）。名前を書いた項目は、その名前（本文 memo）。
	// フィールドの名前 note は、通信に現れない。知らないキーは無視する。ヘッダの値は、そのまま読む（前置きを外さない）
	body := `{"item":{"name":"x","quantity":1,"tags":["a"],"at":"2024-01-02T03:04:05Z"},"memo":"m","note":"ignored","copies":3,"labels":{},
		"weight":0.5,"scale":2,"active":false,"timeout":1000,"wait":2000,"owner":"me","unknown":true}`
	call(t, h, "POST", "/items?dryRun=0", body,
		"X-Tenant", "t", "Authorization", "Bearer abc", "X-Request-Id", "r1", "Accept-Language", "ja", "Trace", "t1")
	at := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	want = api.CreateInput{Tenant: "t", Token: "Bearer abc", RequestId: ptr("r1"), Lang: "ja", Trace: ptr("t1"),
		Item: pkgtype.Item{Name: "x", Quantity: 1, Tags: []string{"a"}, At: &at}, Note: ptr("m"), Copies: 3,
		Labels: map[string]pkgtype.Name{}, Weight: 0.5, Scale: 2, Active: false, Timeout: 1000, Wait: 2000, Owner: "me"}
	if !reflect.DeepEqual(leaves.create, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.create, want)
	}

	// validate = false: 制約に合わない入力でも、確かめずに op を呼ぶ
	leaves.calls = nil
	expectJSON(t, call(t, h, "POST", "/items?dryRun=true", `{"labels":{"k":""},"item":{"name":"","quantity":0,"tags":[]}}`, createHeaders...), 200, `{"name":"","quantity":0,"tags":[]}`)
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
		// 項目がないとき: @http.required の項目だけが、誤り（三つの場所で）
		"required のヘッダがない":   {"/items?dryRun=true", createBody, []string{"Authorization", "a"}, "header X-Tenant: missing"},
		"required のヘッダが空":    {"/items?dryRun=true", createBody, []string{"X-Tenant", ""}, "header X-Tenant: missing"},
		"required のクエリがない":   {"/items", createBody, createHeaders, "query dryRun: missing"},
		"required の本文の項目がない": {"/items?dryRun=true", `{"item":{"name":"x","quantity":1,"tags":[]}}`, createHeaders, "body labels: missing"},
		"本文が空のオブジェクト":        {"/items?dryRun=true", `{}`, createHeaders, "body labels: missing"},
		// 本文が JSON のオブジェクトとして読めない、キーの値の型が合わない
		"本文が JSON でない":      {"/items?dryRun=true", `{"item":`, createHeaders, "body: invalid JSON"},
		"本文が空":              {"/items?dryRun=true", ``, createHeaders, "body: invalid JSON"},
		"本文がオブジェクトでない":      {"/items?dryRun=true", `[1]`, createHeaders, "body: invalid JSON"},
		"本文が null":          {"/items?dryRun=true", `null`, createHeaders, "body: invalid JSON"},
		"本文の項目の型が違う":        {"/items?dryRun=true", `{"labels":{},"copies":"three"}`, createHeaders, "body: invalid JSON"},
		"本文の項目の中の型が違う":      {"/items?dryRun=true", `{"labels":{},"item":{"quantity":"x"}}`, createHeaders, "body: invalid JSON"},
		"boolean のクエリが読めない": {"/items?dryRun=maybe", createBody, createHeaders, "query dryRun: invalid boolean"},
		// フィールドの順に読み、読めなかった最初の理由で終わる
		"ヘッダも本文も読めない": {"/items", `{`, nil, "header X-Tenant: missing"},
		"本文もクエリも読めない": {"/items", `{`, createHeaders, "body: invalid JSON"},
	} {
		expectJSON(t, call(t, h, "POST", tc.target, tc.body, tc.headers...), 400, badRequest(tc.message))
		if len(leaves.calls) != 0 {
			t.Fatalf("%s: 読めない要求で、op が呼ばれた", name)
		}
	}
	for target, message := range map[string]string{
		"/items?min=1":                 "query q: missing",
		"/items?q=x":                   "query min: missing",
		"/items?q=x&min=1&offset=zero": "query offset: invalid int",
		"/items?q=x&min=1&offset=1.5":  "query offset: invalid int",
		"/items?q=x&min=1&page=":       "query page: invalid int",
		"/items?q=x&min=1&limit=a":     "query limit: invalid int",
		"/items?q=x&min=1&size=a":      "query size: invalid int",
		"/items?q=x&min=one":           "query min: invalid float",
		"/items?q=x&min=1&ratio=r":     "query ratio: invalid float",
		"/items?q=x&min=1&max=m":       "query max: invalid float",
		"/items?q=x&min=1&scale=s":     "query scale: invalid float",
		"/items?q=x&min=1&factor=f":    "query factor: invalid float",
		"/items?q=x&min=1&exact=yes":   "query exact: invalid boolean",
		"/items?q=x&min=1&deep=2":      "query deep: invalid boolean",
		"/items?q=x&min=1&flag=":       "query flag: invalid boolean",
		"/items/first":                 "query id: missing",
		// 全体の形の中の項目も、項目の形と同じに読む: 文言は、その model のフィールドの名前
		"/list?page=p":                  "query page: invalid int",
		"/list?size=big":                "query size: invalid int",
		"/list?exact=perhaps":           "query exact: invalid boolean",
		"/list?ratio=half":              "query ratio: invalid float",
		"/items?q=x&min=1&n=ok&name=no": "", // 名前を書いた項目 n だけを読む（フィールドの名前 name は、通信に現れない）
	} {
		got := call(t, h, "GET", target, "")
		if message == "" {
			if got.status != 200 {
				t.Errorf("%s = %+v, want 200", target, got)
			}
			leaves.calls = nil
			continue
		}
		expectJSON(t, got, 400, badRequest(message))
		if len(leaves.calls) != 0 {
			t.Fatalf("%s: 読めない要求で、op が呼ばれた", target)
		}
	}
	// 本文の項目が一つでも、本文は、キーを持つオブジェクト（配列そのものは、読めない）。本文の全体が null なら、オブジェクトではない
	for body, name := range map[string]string{`[{"name":"a","quantity":1}]`: "配列そのもの", `{"items":[{"quantity":"x"}]}`: "要素の型が違う",
		`{"items":{}}`: "項目が配列でない", `[`: "JSON でない", `null`: "null"} {
		expectJSON(t, call(t, h, "POST", "/items/replace", body), 400, badRequest("body: invalid JSON"))
		if len(leaves.calls) != 0 {
			t.Fatalf("本文が%s: 読めない要求で、op が呼ばれた", name)
		}
	}
	// 本文の全体（@http.bodyObj）: encoding/json で読めなければ、同じ文言
	for _, body := range []string{``, `{"name":`, `[1]`, `{"quantity":"x"}`, `"text"`} {
		expectJSON(t, call(t, h, "POST", "/put", body), 400, badRequest("body: invalid JSON"))
	}
	if len(leaves.calls) != 0 {
		t.Fatalf("読めない要求で、op が呼ばれた: %v", leaves.calls)
	}
}

// クエリの項目: 型ごとの読み方と、項目がないときの三つの扱い。
func TestQuery(t *testing.T) {
	leaves, h := newAPI()
	// @http.required のものだけ: @http.default は、その値。どちらもなければ、零値（省略可能は nil）。結果の値のない配列は、[] で返す
	expectJSON(t, call(t, h, "GET", "/items?q=&min=1.5", ""), 200, `[]`)
	want := api.SearchInput{Q: "", Owner: "nobody", Page: 1, Offset: 0, Min: 1.5, Ratio: 0.5, Exact: true, Factor: 3}
	if !reflect.DeepEqual(leaves.search, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.search, want)
	}
	// すべて。同じ名前が二度あれば、最初の値。float は指数も読む。名前を書いた項目は、その名前（n）
	leaves.items = []pkgtype.Item{{Name: "a", Quantity: 1}}
	got := call(t, h, "GET", "/items?q=pen%20case&n=nn&tag=first&tag=second&owner=me&page=3&limit=10&size=5&offset=-7&min=0.25&ratio=2&max=1e3&scale=0.1&exact=false&deep=false&flag=1&factor=0", "")
	expectJSON(t, got, 200, `[{"name":"a","quantity":1,"tags":null}]`)
	want = api.SearchInput{Q: "pen case", Name: ptr(pkgtype.Name("nn")), Tag: ptr("first"), Owner: "me", Page: 3, Limit: ptr(int64(10)),
		Size: ptr(pkgtype.Page(5)), Offset: -7, Min: 0.25, Ratio: 2, Max: ptr(1000.0), Scale: ptr(pkgtype.Ratio(0.1)),
		Exact: false, Deep: ptr(false), Flag: ptr(pkgtype.Flag(true)), Factor: 0}
	if !reflect.DeepEqual(leaves.search, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.search, want)
	}
	// クエリは、空の値でも「ある」: @http.default の値を入れない。省略可能な string は、空の文字列へのポインタ
	call(t, h, "GET", "/items?q=x&min=1&tag=&owner=", "")
	if leaves.search.Tag == nil || *leaves.search.Tag != "" || leaves.search.Owner != "" {
		t.Errorf("tag = %v、owner = %q, want 空の文字列へのポインタと、空の文字列", leaves.search.Tag, leaves.search.Owner)
	}
	// 空の配列（nil でない）も、[]
	leaves.items = []pkgtype.Item{}
	expectJSON(t, call(t, h, "GET", "/items?q=x&min=1", ""), 200, `[]`)
}

// TestFiniteFloat は、要求 hozo.gohttp.Requirement.FiniteFloat（plugins/go-http/go-http.hozo）: クエリの float の値は、有限の値だけを
// 読む。strconv.ParseFloat が読む NaN と無限大は、読めない値で、op を呼ばずに 400 の "query <名前>: invalid float"。核の型、省略可能、
// scalar の、どれも。有限の値は、指数の形でも、読む。
func TestFiniteFloat(t *testing.T) {
	leaves, h := newAPI()
	for _, name := range []string{"min", "max", "scale"} {
		for _, text := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "inf", "Infinity", "-infinity"} {
			target := "/items?q=x&min=1&" + name + "=" + url.QueryEscape(text)
			if name == "min" {
				target = "/items?q=x&min=" + url.QueryEscape(text)
			}
			want := `{"error":"http.BadRequest","value":{"message":"query ` + name + `: invalid float","violations":[]}}`
			expectJSON(t, call(t, h, "GET", target, ""), 400, want)
		}
	}
	if len(leaves.calls) != 0 {
		t.Fatalf("op が呼ばれた: %v", leaves.calls)
	}
	// 有限の値は、読む（大きな指数も、有限なら）
	expectJSON(t, call(t, h, "GET", "/items?q=x&min=-1.5e308&max=1e308", ""), 200, `[]`)
	if leaves.search.Min != -1.5e308 || leaves.search.Max == nil || *leaves.search.Max != 1e308 {
		t.Errorf("入力 = %+v, want min は -1.5e308、max は 1e308", leaves.search)
	}
}

// 全体の形: ヘッダ、クエリ、本文を、一つずつの model で受け取る。
func TestWholeForms(t *testing.T) {
	leaves, h := newAPI()
	// 何も送らない: 全体の形の中の項目は、なければ、いつも零値のまま。値のない record の結果は、{} で返す
	expectJSON(t, call(t, h, "GET", "/list", ""), 200, `{}`)
	if want := (api.ListInput{}); !reflect.DeepEqual(leaves.list, want) {
		t.Errorf("入力 = %+v, want 零値", leaves.list)
	}
	// 項目の名前は、その model のフィールドの名前。読み方は、項目の形と同じ（省略可能、scalar、空のヘッダはないもの）
	leaves.counts = map[string]int64{"a": 1}
	got := call(t, h, "GET", "/list?page=2&size=10&q=pen&exact=true&ratio=0.5&page=9&paging=x", "", "Tenant", "acme", "Trace", "t1", "Meta", "x")
	expectJSON(t, got, 200, `{"a":1}`)
	want := api.ListInput{
		Meta:   pkgtype.Meta{Tenant: "acme", Trace: ptr(pkgtype.Token("t1"))},
		Paging: pkgtype.Paging{Page: ptr(int64(2)), Size: 10, Q: "pen", Exact: ptr(pkgtype.Flag(true)), Ratio: 0.5},
	}
	if !reflect.DeepEqual(leaves.list, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.list, want)
	}
	call(t, h, "GET", "/list", "", "Tenant", "", "Trace", "")
	if leaves.list.Meta.Tenant != "" || leaves.list.Meta.Trace != nil {
		t.Errorf("空のヘッダ: 入力 = %+v, want ないもの", leaves.list.Meta)
	}
	// 空の、インデックスの model（nil でないマップ）も、{}
	leaves.counts = map[string]int64{}
	expectJSON(t, call(t, h, "GET", "/list", ""), 200, `{}`)

	// 本文の全体: 本文が、その model の JSON（フィールドの名前 item では包まない）。知らないキーは無視し、ないキーは零値のまま。
	// ほかの場所の項目の形とは、一緒に読める
	expectEmpty(t, call(t, h, "POST", "/put", `{"name":"pen","tags":["a"],"item":{"name":"no"},"unknown":1}`, "X-Tenant", "acme"))
	if want := (api.PutInput{Item: pkgtype.Item{Name: "pen", Tags: []string{"a"}}, Tenant: ptr("acme")}); !reflect.DeepEqual(leaves.put, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.put, want)
	}
	// 本文の全体が null なら、model は零値のまま（encoding/json の読み方）
	expectEmpty(t, call(t, h, "POST", "/put", `null`))
	if want := (api.PutInput{}); !reflect.DeepEqual(leaves.put, want) {
		t.Errorf("本文が null: 入力 = %+v, want 零値", leaves.put)
	}
}

func TestResults(t *testing.T) {
	leaves, h := newAPI()
	// 別の名前空間の入力の model。省略可能な戻り値で、値がなければ 204 で本文なし（null は返さない）
	expectEmpty(t, call(t, h, "GET", "/items/first?id=7", "", "X-Trace", "t1"))
	if want := (pkgtype.Lookup{Id: "7", Trace: ptr("t1")}); !reflect.DeepEqual(leaves.lookup, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.lookup, want)
	}
	// 同じ op が、値があれば 200 と、その JSON
	leaves.first = &pkgtype.Item{Name: "a", Quantity: 1, Tags: []string{}}
	expectJSON(t, call(t, h, "GET", "/items/first?id=7", ""), 200, `{"name":"a","quantity":1,"tags":[]}`)
	// 省略可能な scalar の戻り値も同じ
	expectEmpty(t, call(t, h, "POST", "/put", `{}`))
	leaves.token = ptr(pkgtype.Token(""))
	expectJSON(t, call(t, h, "POST", "/put", `{}`), 200, `""`)
	// 本文の項目が一つ: キー items の値が配列。戻り値がなければ 204
	expectEmpty(t, call(t, h, "POST", "/items/replace", `{"items":[{"name":"a","quantity":1,"tags":["x"],"extra":1}]}`))
	if want := (api.ReplaceInput{Items: []pkgtype.Item{{Name: "a", Quantity: 1, Tags: []string{"x"}}}}); !reflect.DeepEqual(leaves.replace, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.replace, want)
	}
	// どちらもない項目は、キーがなくても誤りにしない（零値のまま）。配列の null は、空の配列と読む
	for _, body := range []string{`{}`, `{"items":null}`} {
		expectEmpty(t, call(t, h, "POST", "/items/replace", body))
		if len(leaves.replace.Items) != 0 {
			t.Errorf("POST /items/replace（%s）の入力 = %+v", body, leaves.replace)
		}
	}
}

func TestErrors(t *testing.T) {
	leaves, h := newAPI()
	until := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, tc := range map[string]struct {
		err    error
		status int
		body   string
	}{
		// 引数のない例外: value はない
		"引数のない例外": {pkgtype.NotFound{}, 404, `{"error":"type.NotFound"}`},
		// 引数を持つ例外: value は、引数の名前をキーにしたオブジェクト。キーの並びは、宣言の引数の順
		"引数が二つの例外": {pkgtype.Conflict{Existing: "pen", Count: 2}, 409, `{"error":"type.Conflict","value":{"existing":"pen","count":2}}`},
		"引数の零値も、値": {pkgtype.Conflict{}, 409, `{"error":"type.Conflict","value":{"existing":"","count":0}}`},
		// 引数が一つでも、オブジェクトで包む（引数の名前 value が、キーになる）。model の引数は、その JSON
		"引数が一つの例外": {pkgtype.Rejected{Value: pkgtype.Item{Name: "x", Quantity: 1}}, 422, `{"error":"type.Rejected","value":{"value":{"name":"x","quantity":1,"tags":null}}}`},
		// 省略可能な引数で値がなければ、キーを省く。scalar の引数は元の型の JSON（Duration は、整数のナノ秒）、Time は RFC 3339
		"省略可能な引数に値がない": {pkgtype.Limited{RetryAfter: pkgtype.Wait(30 * time.Second)}, 429, `{"error":"type.Limited","value":{"retryAfter":30000000000}}`},
		"省略可能な引数に値がある": {pkgtype.Limited{RetryAfter: pkgtype.Wait(time.Second), Hint: ptr("slow down"), Until: &until}, 429,
			`{"error":"type.Limited","value":{"retryAfter":1000000000,"hint":"slow down","until":"2026-01-02T03:04:05Z"}}`},
		// 引数を持つ例外は、どの引数にも値がなくても、value を持つ（空のオブジェクト）
		"どの引数にも値がない":  {pkgtype.Gone{}, 410, `{"error":"type.Gone","value":{}}`},
		"包んだ例外":       {fmt.Errorf("wrapped: %w", pkgtype.NotFound{}), 404, `{"error":"type.NotFound"}`},
		"包んだ、引数を持つ例外": {fmt.Errorf("wrapped: %w", pkgtype.Conflict{Existing: "ink", Count: 1}), 409, `{"error":"type.Conflict","value":{"existing":"ink","count":1}}`},
		// それ以外は 500 で、中身を出さない。ステータスを持たない例外は、引数を持っていても、その値を出さない
		"ステータスを持たない例外":       {pkgtype.Broken{}, 500, `{"error":"internal"}`},
		"ステータスを持たない、引数を持つ例外": {pkgtype.Leaky{Secret: "secret detail"}, 500, `{"error":"internal"}`},
		"ほかのエラー":             {errors.New("secret detail"), 500, `{"error":"internal"}`},
		"中断":                 {context.Canceled, 500, `{"error":"internal"}`},
		// 例外は値の型で見分ける: ポインタで返せば、見分けられずに 500
		"ポインタで返した例外":       {&pkgtype.NotFound{}, 500, `{"error":"internal"}`},
		"ポインタで返した、引数を持つ例外": {&pkgtype.Conflict{Existing: "pen", Count: 2}, 500, `{"error":"internal"}`},
	} {
		leaves.err = tc.err
		// 経路の op のどれかが投げうる例外は、どの経路でも同じに見分ける（補助は一つ）。戻り値のある op も、ない op も、同じ
		for _, req := range [][2]string{{"GET", "/items/first?id=1"}, {"GET", "/health"}, {"POST", "/reset/"}, {"GET", "/list"}} {
			got := call(t, h, req[0], req[1], "")
			if got.status != tc.status || got.body != tc.body || got.contentType != "application/json" {
				t.Errorf("%s（%s %s）: 応答 = %+v, want %d %s", name, req[0], req[1], got, tc.status, tc.body)
			}
		}
	}
}

// 例外の構造体は、引数ごとにフィールド一つ（docs/plugin-go.md「exception」）。値を一つに包むフィールドは、ない。
var (
	_ = pkgtype.Conflict{Existing: "", Count: int64(0)}
	_ = pkgtype.Limited{RetryAfter: pkgtype.Wait(0), Hint: (*string)(nil), Until: (*time.Time)(nil)}
	_ = pkgtype.NotFound(struct{}{})
)

// op を呼ぶ前の失敗 http.BadRequest の形は、例外の形そのもの（value は、二つの引数 message と violations をキーにしたオブジェクト）で、
// どの例外の引数の形にも依らない: 引数を持つ例外を投げうる経路でも、同じ本文。violations は、空でも []。
func TestBadRequestShape(t *testing.T) {
	leaves, h := newAPI()
	leaves.err = pkgtype.Conflict{Existing: "pen", Count: 2}
	expectJSON(t, call(t, h, "GET", "/items/first", ""), 400, `{"error":"http.BadRequest","value":{"message":"query id: missing","violations":[]}}`)
	expectJSON(t, call(t, h, "POST", "/items?dryRun=true", `[`, createHeaders...), 400, `{"error":"http.BadRequest","value":{"message":"body: invalid JSON","violations":[]}}`)
	if len(leaves.calls) != 0 {
		t.Errorf("読めない要求で、op が呼ばれた: %v", leaves.calls)
	}
	strictHandler := strict.NewHandler(strict.New(&strictLeaves{}))
	expectJSON(t, call(t, strictHandler, "POST", "/orders", `{"lines":[{"name":"","quantity":1}]}`), 400,
		`{"error":"http.BadRequest","value":{"message":"invalid input","violations":[{"path":"body.lines[0].name","constraint":"length"}]}}`)
}

// strictLeaves は strict の葉の op。
type strictLeaves struct {
	calls []string
	order strict.OrderInput
	edit  strict.EditInput
	bulk  strict.BulkInput
}

func (l *strictLeaves) Order(ctx context.Context, in strict.OrderInput) (int64, error) {
	l.calls, l.order = append(l.calls, "order"), in
	return int64(len(in.Lines)), nil
}
func (l *strictLeaves) Edit(ctx context.Context, in strict.EditInput) error {
	l.calls, l.edit = append(l.calls, "edit"), in
	return nil
}
func (l *strictLeaves) Bulk(ctx context.Context, in strict.BulkInput) error {
	l.calls, l.bulk = append(l.calls, "bulk"), in
	return nil
}
func (l *strictLeaves) Plain(ctx context.Context, in strict.PlainInput) (string, error) {
	l.calls = append(l.calls, "plain")
	return "plain", nil
}
func (l *strictLeaves) Now(ctx context.Context) (string, error) {
	l.calls = append(l.calls, "now")
	return "now", nil
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
	// 項目の形: 違反の path は、通信の上の場所（body.lines）と、その中のパス。確かめた順
	got := call(t, h, "POST", "/orders", `{"lines":[{"name":"a","quantity":1},{"name":"","quantity":0}]}`, "Authorization", "t")
	expectJSON(t, got, 400, invalid("body.lines[1].name", "length", "body.lines[1].quantity", "min"))
	// 名前を書いた項目は、その名前で（フィールドの名前 extra は出さない）。クエリとヘッダの値の制約も、通信の上の場所で
	got = call(t, h, "POST", "/edit?page=0", `{"item":{"name":"","quantity":1},"more":{"name":"x","quantity":0}}`, "X-Name", "")
	expectJSON(t, got, 400, invalid("query.page", "min", "body.item.name", "length", "body.more.quantity", "min"))
	// 全体の形: 場所は body、query（フィールドの名前 payload、paging は出さない）
	got = call(t, h, "POST", "/bulk?size=0", `{"name":"","quantity":1}`)
	expectJSON(t, got, 400, invalid("body.name", "length", "query.size", "min"))
	// 読めない要求が先で、次に制約の検査
	expectJSON(t, call(t, h, "POST", "/edit?page=0", `{`), 400, badRequest("body: invalid JSON"))
	expectJSON(t, call(t, h, "POST", "/edit?page=x", `{"item":{"name":"","quantity":0}}`), 400, badRequest("query page: invalid int"))
	expectJSON(t, call(t, h, "POST", "/orders", `{}`), 400, badRequest("body lines: missing"))
	// どちらもない項目（item）がなければ、零値のまま確かめる: 零値が制約に合わなければ、違反
	expectJSON(t, call(t, h, "POST", "/edit", `{}`), 400, invalid("body.item.name", "length", "body.item.quantity", "min"))
	if len(leaves.calls) != 0 {
		t.Fatalf("制約に合わない入力か、読めない要求で、op が呼ばれた: %v", leaves.calls)
	}
	// 制約に合う入力は、op に届く
	expectJSON(t, call(t, h, "POST", "/orders", `{"lines":[{"name":"a","quantity":1},{"name":"b","quantity":2}]}`), 200, `2`)
	expectEmpty(t, call(t, h, "POST", "/edit", `{"item":{"name":"a","quantity":1}}`))
	// @http.default の値（page は 1）は、制約に合う（語彙のプラグインが、確かめてある。HTTP-15）
	if leaves.edit.Page != 1 || leaves.edit.Name != nil || leaves.edit.Extra != nil || leaves.edit.Item.Name != "a" {
		t.Errorf("入力 = %+v", leaves.edit)
	}
	expectEmpty(t, call(t, h, "POST", "/bulk?size=3&q=x", `{"name":"a","quantity":2}`))
	if want := (strict.BulkInput{Payload: pkgtype.Item{Name: "a", Quantity: 2}, Paging: pkgtype.Paging{Size: 3, Q: "x"}}); !reflect.DeepEqual(leaves.bulk, want) {
		t.Errorf("入力 = %+v, want %+v", leaves.bulk, want)
	}
	// Validate を持たない入力は、同じ補助を通って、確かめるものがないまま op に届く。入力のない経路も、そのまま
	expectJSON(t, call(t, h, "GET", "/plain", ""), 200, `"plain"`)
	expectJSON(t, call(t, h, "GET", "/now", ""), 200, `"now"`)
	if want := []string{"order", "edit", "bulk", "plain", "now"}; !reflect.DeepEqual(leaves.calls, want) {
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

// NewHandler が受け取るのは、入口の Ops（New の戻り値を、そのまま渡す）と、設定。http.Handler を返す。設定は、省ける
var (
	_ func(api.Ops, ...api.HandlerOption) http.Handler = api.NewHandler
	_                                                  = api.NewHandler(api.New(&apiLeaves{}))
)

// TestInternalErrorHook は、要求 hozo.gohttp.Requirement.InternalErrorHook（plugins/go-http/go-http.hozo）: WithInternalError の関数は、
// ハンドラが 500 を返すときだけ、その要求と、op が返したエラーで、一度呼ばれる。応答は、渡さないときと同じ。@http.status を持つ例外と、
// op を呼ぶ前の失敗では、呼ばれない。@http.status を持つ例外のない出力（bare）でも、同じ。
func TestInternalErrorHook(t *testing.T) {
	type seen struct {
		route string
		err   error
	}
	var got []seen
	hook := api.WithInternalError(func(r *http.Request, err error) {
		got = append(got, seen{r.Method + " " + r.URL.Path, err})
	})
	leaves := &apiLeaves{}
	h := api.NewHandler(api.New(leaves), hook)

	// 500: 一度、要求とエラーで呼ぶ。応答は、渡さないときと同じ。要求の context は、ハンドラが受けたもの
	secret := errors.New("secret detail")
	leaves.err = secret
	expectJSON(t, call(t, h, "GET", "/items/first?id=1", ""), 500, `{"error":"internal"}`)
	if len(got) != 1 || got[0].route != "GET /items/first" || got[0].err != secret {
		t.Fatalf("500 のとき、渡した関数が受け取ったもの = %+v, want GET /items/first と、op が返したエラー、一度", got)
	}
	// ステータスを持たない例外も、500 なので、呼ぶ
	got = nil
	leaves.err = pkgtype.Broken{}
	expectJSON(t, call(t, h, "GET", "/health", ""), 500, `{"error":"internal"}`)
	if len(got) != 1 || !errors.Is(got[0].err, pkgtype.Broken{}) {
		t.Errorf("ステータスを持たない例外で、渡した関数が受け取ったもの = %+v", got)
	}
	// @http.status を持つ例外と、op を呼ぶ前の失敗と、成功では、呼ばない
	got = nil
	leaves.err = pkgtype.NotFound{}
	expectJSON(t, call(t, h, "GET", "/items/first?id=1", ""), 404, `{"error":"type.NotFound"}`)
	leaves.err = nil
	call(t, h, "GET", "/items?min=1", "") // q がない: 400
	expectJSON(t, call(t, h, "GET", "/items?q=x&min=1", ""), 200, `[]`)
	if len(got) != 0 {
		t.Errorf("500 でないのに、渡した関数が呼ばれた: %+v", got)
	}
	// 渡さなければ、応答は同じ
	leaves.err = secret
	expectJSON(t, call(t, api.NewHandler(api.New(leaves)), "GET", "/items/first?id=1", ""), 500, `{"error":"internal"}`)

	// @http.status を持つ例外のない出力でも、同じに呼ぶ
	got = nil
	expectJSON(t, call(t, bare.NewHandler(bare.New(bareLeaves{}), bare.WithInternalError(func(r *http.Request, err error) {
		got = append(got, seen{r.Method + " " + r.URL.Path, err})
	})), "POST", "/touch", ""), 500, `{"error":"internal"}`)
	if len(got) != 1 || got[0].route != "POST /touch" || got[0].err == nil {
		t.Errorf("bare の 500 で、渡した関数が受け取ったもの = %+v", got)
	}
}
