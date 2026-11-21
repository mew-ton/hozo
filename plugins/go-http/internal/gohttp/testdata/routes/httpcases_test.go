// 入力 routes の、インテグレーションテストの口（テストのパッケージの http.go）を動かす（plugins/go-http の TestHTTPCases が、behavior_test.go と
// 同じモジュールのルートに置いて走らせる）。確かめること（docs/plugin-go-http.md「インテグレーションテストの口」）:
//
//   - 「口の形」: すべての入口の、すべての結果のケースを満たす型を RunHTTP に渡せば、通る。テストの名前は <op の FQN>/<結果>。
//     経路を二つ持つ op は、その下に、経路ごとの段。Given、Request、Then は、nil なら呼ばれない。
//   - 「必要なケース」: 正常（値を返す op、返さない op、省略可能な結果の 204、零値の結果、空の、インデックスの model）、op を呼ぶ前の失敗、
//     @http.status を持つ例外（引数を持たないもの、持つもの、どの引数にも値がないもの）、持たない例外（500 と internal）。
//   - 「要求の組み立て」: 六つのデコレータの一行ずつ。組み立てた要求を、ハンドラが、もとの入力と同じ値に読む（「入力の読み方」の裏返し）。
//     値がない（nil のポインタ）項目は、送らない。
package behavior

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/app/gen/api"
	"example.com/app/gen/api/apitest"
	"example.com/app/gen/bare"
	"example.com/app/gen/bare/baretest"
	"example.com/app/gen/strict"
	"example.com/app/gen/strict/stricttest"
	pkgtype "example.com/app/gen/type"
)

// apiCases は apitest.HTTPCases を満たす。葉（apiLeaves）が返す値と失敗を、Given が決める。
type apiCases struct{ l *apiLeaves }

var _ apitest.HTTPCases = apiCases{}

// returns は、葉が返す失敗を err にし、呼ばれた記録を消す Given。
func (c apiCases) returns(err error) func(context.Context) error {
	return func(context.Context) error {
		c.l.err, c.l.calls = err, nil
		return nil
	}
}

// notCalled は、op が呼ばれていないことを確かめる。
func (c apiCases) notCalled(t *testing.T) {
	t.Helper()
	if len(c.l.calls) != 0 {
		t.Errorf("op が呼ばれた: %v", c.l.calls)
	}
}

// body は、要求の本文を text に置き換える Request。
func body(text string) func(*http.Request) {
	return func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader(text)) }
}

var pen = pkgtype.Item{Name: "pen", Quantity: 2, Tags: []string{"a", "b"}}

// ---- api.health: 引数がなく、戻り値がある ----

type healthCases struct{ apiCases }

func (c apiCases) Health() apitest.HealthHTTPCases { return healthCases{c} }
func (c healthCases) OK() apitest.HealthHTTPCase {
	return apitest.HealthHTTPCase{Given: c.returns(nil), Then: func(t *testing.T, got string, res *httptest.ResponseRecorder) {
		if got != "ok" || res.Code != 200 {
			t.Errorf("health = %q（%d）", got, res.Code)
		}
	}}
}

// ---- api.create: ヘッダ、クエリ、本文の項目 ----

type createCases struct{ apiCases }

var createIn = api.CreateInput{Tenant: "acme", Token: "t", RequestId: ptr("r1"), Lang: "ja", Trace: ptr("tr"), Item: pen, DryRun: true,
	Note: ptr("memo"), Copies: 3, Labels: api.Labels{"k": "v"}, Weight: 2.5, Scale: 1.25, Active: true,
	Timeout: time.Second, Wait: pkgtype.Wait(time.Minute), Owner: "me"}

func (c apiCases) Create() apitest.CreateHTTPCases { return createCases{c} }
func (c createCases) OK() apitest.CreateHTTPCase {
	return apitest.CreateHTTPCase{In: createIn, Given: c.returns(nil), Then: func(t *testing.T, got pkgtype.Item, _ *httptest.ResponseRecorder) {
		// 組み立てた要求を、ハンドラが、もとの入力と同じ値に読んでいる
		if !reflect.DeepEqual(c.l.create, createIn) {
			t.Errorf("op に届いた入力 = %+v, want %+v", c.l.create, createIn)
		}
		if !reflect.DeepEqual(got, pen) {
			t.Errorf("create = %+v, want %+v", got, pen)
		}
	}}
}

// 省略可能でない項目（@http.required のヘッダ）を送らない要求は、Request で作る
func (c createCases) BadRequest() apitest.CreateHTTPCase {
	return apitest.CreateHTTPCase{In: createIn, Given: c.returns(nil),
		Request: func(r *http.Request) { r.Header.Del("X-Tenant") },
		Then: func(t *testing.T, _ pkgtype.Item, res *httptest.ResponseRecorder) {
			c.notCalled(t)
			if !strings.Contains(res.Body.String(), "header X-Tenant: missing") {
				t.Errorf("本文 = %s", res.Body)
			}
		}}
}
func (c createCases) Conflict() apitest.CreateHTTPCase {
	return apitest.CreateHTTPCase{In: createIn, Given: c.returns(pkgtype.Conflict{Existing: "pen", Count: 2}),
		Then: func(t *testing.T, got pkgtype.Item, res *httptest.ResponseRecorder) {
			if !reflect.DeepEqual(got, pkgtype.Item{}) {
				t.Errorf("例外のケースの got = %+v（零値のはず）", got)
			}
			if want := `{"error":"type.Conflict","value":{"existing":"pen","count":2}}`; res.Body.String() != want {
				t.Errorf("本文 = %s, want %s", res.Body, want)
			}
		}}
}

// Then が nil のケース
func (c createCases) Rejected() apitest.CreateHTTPCase {
	return apitest.CreateHTTPCase{In: createIn, Given: c.returns(pkgtype.Rejected{Value: pen})}
}

// ---- api.search: クエリの各型 ----

type searchCases struct{ apiCases }

var searchIn = api.SearchInput{Q: "pen & ink", Name: ptr(pkgtype.Name("n")), Owner: "me", Page: 2, Limit: ptr(int64(5)), Offset: 7,
	Min: 0.25, Ratio: 0.75, Max: ptr(1e21), Exact: true, Deep: ptr(false), Flag: ptr(pkgtype.Flag(true)), Factor: 3}

func (c apiCases) Search() apitest.SearchHTTPCases { return searchCases{c} }
func (c searchCases) OK() apitest.SearchHTTPCase {
	return apitest.SearchHTTPCase{In: searchIn, Given: c.returns(nil), Then: func(t *testing.T, got []pkgtype.Item, res *httptest.ResponseRecorder) {
		// 値がない項目（tag、size、scale）は、送られず、nil のまま届く
		if !reflect.DeepEqual(c.l.search, searchIn) {
			t.Errorf("op に届いた入力 = %+v, want %+v", c.l.search, searchIn)
		}
		// 値のない配列は、[]
		if got == nil || len(got) != 0 || res.Body.String() != "[]" {
			t.Errorf("search = %#v（本文 %s）", got, res.Body)
		}
	}}
}
func (c searchCases) BadRequest() apitest.SearchHTTPCase {
	return apitest.SearchHTTPCase{In: searchIn, Given: c.returns(nil),
		Request: func(r *http.Request) {
			q := r.URL.Query()
			q.Del("q")
			r.URL.RawQuery = q.Encode()
		},
		Then: func(t *testing.T, _ []pkgtype.Item, _ *httptest.ResponseRecorder) { c.notCalled(t) }}
}

// @http.status を持たない例外は、どれも、500 と internal
func (c searchCases) Broken() apitest.SearchHTTPCase {
	return apitest.SearchHTTPCase{In: searchIn, Given: c.returns(pkgtype.Broken{})}
}
func (c searchCases) Leaky() apitest.SearchHTTPCase {
	return apitest.SearchHTTPCase{In: searchIn, Given: c.returns(pkgtype.Leaky{Secret: "secret detail"})}
}

// ---- api.first: 別の名前空間の入力の model。省略可能な結果 ----

type firstCases struct{ apiCases }

var firstIn = pkgtype.Lookup{Id: "1", Trace: ptr("x")}

func (c apiCases) First() apitest.FirstHTTPCases { return firstCases{c} }
func (c firstCases) OK() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn,
		Given: func(ctx context.Context) error {
			c.l.first = &pen
			return c.returns(nil)(ctx)
		},
		Then: func(t *testing.T, got *pkgtype.Item, _ *httptest.ResponseRecorder) {
			c.l.first = nil
			if got == nil || !reflect.DeepEqual(*got, pen) || !reflect.DeepEqual(c.l.lookup, firstIn) {
				t.Errorf("first = %+v（入力 %+v）", got, c.l.lookup)
			}
		}}
}
func (c firstCases) BadRequest() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(nil), Request: func(r *http.Request) { r.URL.RawQuery = "" }}
}
func (c firstCases) NotFound() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(pkgtype.NotFound{})}
}
func (c firstCases) Limited() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(pkgtype.Limited{RetryAfter: pkgtype.Wait(30 * time.Second), Hint: ptr("slow down")})}
}

// 引数を持つ例外は、どの引数にも値がなくても、value を持つ（空のオブジェクト）
func (c firstCases) Gone() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(pkgtype.Gone{})}
}

// ---- api.replace: 本文の項目が一つ。戻り値がない ----

type replaceCases struct{ apiCases }

var replaceIn = api.ReplaceInput{Items: []pkgtype.Item{pen}}

func (c apiCases) Replace() apitest.ReplaceHTTPCases { return replaceCases{c} }
func (c replaceCases) OK() apitest.ReplaceHTTPCase {
	return apitest.ReplaceHTTPCase{In: replaceIn, Given: c.returns(nil), Then: func(t *testing.T, res *httptest.ResponseRecorder) {
		if !reflect.DeepEqual(c.l.replace, replaceIn) || res.Code != 204 {
			t.Errorf("op に届いた入力 = %+v（%d）", c.l.replace, res.Code)
		}
	}}
}
func (c replaceCases) BadRequest() apitest.ReplaceHTTPCase {
	return apitest.ReplaceHTTPCase{In: replaceIn, Given: c.returns(nil), Request: body(`[`),
		Then: func(t *testing.T, _ *httptest.ResponseRecorder) { c.notCalled(t) }}
}
func (c replaceCases) NotFound() apitest.ReplaceHTTPCase {
	return apitest.ReplaceHTTPCase{In: replaceIn, Given: c.returns(pkgtype.NotFound{})}
}

// ---- api.listAll: ヘッダとクエリの全体の形。結果は、インデックスの model ----

type listAllCases struct{ apiCases }

var listIn = api.ListInput{Meta: pkgtype.Meta{Tenant: "acme", Trace: ptr(pkgtype.Token("tr"))},
	Paging: pkgtype.Paging{Page: ptr(int64(2)), Size: 10, Q: "x y&z", Exact: ptr(pkgtype.Flag(true)), Ratio: 0.5}}

func (c apiCases) ListAll() apitest.ListAllHTTPCases { return listAllCases{c} }
func (c listAllCases) OK() apitest.ListAllHTTPCase {
	return apitest.ListAllHTTPCase{In: listIn, Given: c.returns(nil), Then: func(t *testing.T, got api.Counts, res *httptest.ResponseRecorder) {
		if !reflect.DeepEqual(c.l.list, listIn) {
			t.Errorf("op に届いた入力 = %+v, want %+v", c.l.list, listIn)
		}
		// 値のない、インデックスの model は、{}
		if got == nil || len(got) != 0 || res.Body.String() != "{}" {
			t.Errorf("listAll = %#v（本文 %s）", got, res.Body)
		}
	}}
}

// ---- api.put: 本文の全体の形。省略可能な結果で、値がない（204） ----

type putCases struct{ apiCases }

var putIn = api.PutInput{Item: pen, Tenant: ptr("acme")}

func (c apiCases) Put() apitest.PutHTTPCases { return putCases{c} }
func (c putCases) OK() apitest.PutHTTPCase {
	return apitest.PutHTTPCase{In: putIn, Given: c.returns(nil), Then: func(t *testing.T, got *pkgtype.Token, res *httptest.ResponseRecorder) {
		if got != nil || res.Code != 204 || !reflect.DeepEqual(c.l.put, putIn) {
			t.Errorf("put = %v（%d。入力 %+v）", got, res.Code, c.l.put)
		}
	}}
}
func (c putCases) BadRequest() apitest.PutHTTPCase {
	return apitest.PutHTTPCase{In: putIn, Given: c.returns(nil), Request: body(`{"name":1}`)}
}

// ---- api.reset、api.ping、api.count: 入力がない ----

type resetCases struct{ apiCases }

func (c apiCases) Reset() apitest.ResetHTTPCases { return resetCases{c} }

// Request も Then も nil のケース
func (c resetCases) OK() apitest.ResetHTTPCase { return apitest.ResetHTTPCase{Given: c.returns(nil)} }

type pingCases struct{ apiCases }

func (c apiCases) Ping() apitest.PingHTTPCases { return pingCases{c} }

// 経路を二つ持つ op: ケースは、経路ごとに、取り直される
func (c pingCases) OK() apitest.PingHTTPCase {
	return apitest.PingHTTPCase{Given: c.returns(nil), Then: func(t *testing.T, got bool, _ *httptest.ResponseRecorder) {
		if !got || len(c.l.calls) != 1 {
			t.Errorf("ping = %v（呼ばれた op %v）", got, c.l.calls)
		}
	}}
}

type countCases struct{ apiCases }

func (c apiCases) Count() apitest.CountHTTPCases { return countCases{c} }

// 零値の結果も、値（200）
func (c countCases) OK() apitest.CountHTTPCase {
	return apitest.CountHTTPCase{Given: c.returns(nil), Then: func(t *testing.T, got int64, res *httptest.ResponseRecorder) {
		if got != 0 || res.Code != 200 {
			t.Errorf("count = %d（%d）", got, res.Code)
		}
	}}
}

func TestHTTPCases(t *testing.T) {
	leaves, h := newAPI()
	apitest.RunHTTP(t, h, apiCases{leaves})
}

// ---- strict: validate = true。op を呼ぶ前の失敗のケースを、制約に合わない入力の値で書く（Request は要らない） ----

type strictCases struct{ l *strictLeaves }

var _ stricttest.HTTPCases = strictCases{}

type orderCases struct{ strictCases }

func (c strictCases) Order() stricttest.OrderHTTPCases { return orderCases{c} }
func (c orderCases) OK() stricttest.OrderHTTPCase {
	return stricttest.OrderHTTPCase{In: strict.OrderInput{Token: ptr(pkgtype.Token("t")), Lines: []pkgtype.Item{pen, pen}},
		Then: func(t *testing.T, got int64, _ *httptest.ResponseRecorder) {
			if got != 2 {
				t.Errorf("order = %d, want 2", got)
			}
		}}
}
func (c orderCases) BadRequest() stricttest.OrderHTTPCase {
	return stricttest.OrderHTTPCase{In: strict.OrderInput{Lines: []pkgtype.Item{{Name: "", Quantity: 0}}},
		Then: func(t *testing.T, _ int64, res *httptest.ResponseRecorder) {
			if want := invalid("body.lines[0].name", "length", "body.lines[0].quantity", "min"); res.Body.String() != want {
				t.Errorf("本文 = %s, want %s", res.Body, want)
			}
		}}
}

type editCases struct{ strictCases }

func (c strictCases) Edit() stricttest.EditHTTPCases { return editCases{c} }
func (c editCases) OK() stricttest.EditHTTPCase {
	return stricttest.EditHTTPCase{In: strict.EditInput{Page: 1, Item: pen}}
}
func (c editCases) BadRequest() stricttest.EditHTTPCase {
	return stricttest.EditHTTPCase{In: strict.EditInput{Page: 0, Item: pen}}
}

type bulkCases struct{ strictCases }

func (c strictCases) Bulk() stricttest.BulkHTTPCases { return bulkCases{c} }
func (c bulkCases) OK() stricttest.BulkHTTPCase {
	return stricttest.BulkHTTPCase{In: strict.BulkInput{Payload: pen, Paging: pkgtype.Paging{Size: 1}}}
}
func (c bulkCases) BadRequest() stricttest.BulkHTTPCase {
	return stricttest.BulkHTTPCase{In: strict.BulkInput{Payload: pen, Paging: pkgtype.Paging{Size: 0}}}
}

type plainCases struct{ strictCases }

// 入力が、@http.required のないクエリだけ: op を呼ぶ前の失敗のケースは、ない
func (c strictCases) Plain() stricttest.PlainHTTPCases { return plainCases{c} }
func (c plainCases) OK() stricttest.PlainHTTPCase      { return stricttest.PlainHTTPCase{} }

type nowCases struct{ strictCases }

func (c strictCases) Now() stricttest.NowHTTPCases { return nowCases{c} }
func (c nowCases) OK() stricttest.NowHTTPCase      { return stricttest.NowHTTPCase{} }

func TestStrictHTTPCases(t *testing.T) {
	leaves := &strictLeaves{}
	stricttest.RunHTTP(t, strict.NewHandler(strict.New(leaves)), strictCases{leaves})
}

// ---- bare: 入力も、@http.status を持つ例外もない。/ で終わるパスと、名前付きデコレータを通した経路 ----

// touchLeaves は bare の葉の op。Touch が返す失敗を、ケースが決める。
type touchLeaves struct {
	bareLeaves
	err error
}

func (l *touchLeaves) Touch(context.Context) error { return l.err }

type bareCases struct{ l *touchLeaves }

var _ baretest.HTTPCases = bareCases{}

type rootCases struct{ bareCases }

// / で終わるパス
func (c bareCases) Root() baretest.RootHTTPCases { return rootCases{c} }
func (c rootCases) OK() baretest.RootHTTPCase {
	return baretest.RootHTTPCase{Then: func(t *testing.T, got string, _ *httptest.ResponseRecorder) {
		if got != "root" {
			t.Errorf("root = %q", got)
		}
	}}
}

type touchCases struct{ bareCases }

func (c bareCases) Touch() baretest.TouchHTTPCases { return touchCases{c} }
func (c touchCases) OK() baretest.TouchHTTPCase {
	return baretest.TouchHTTPCase{Given: func(context.Context) error { c.l.err = nil; return nil }}
}
func (c touchCases) Unknown() baretest.TouchHTTPCase {
	return baretest.TouchHTTPCase{Given: func(context.Context) error { c.l.err = bare.Unknown{}; return nil }}
}

type viaAliasCases struct{ bareCases }

func (c bareCases) ViaAlias() baretest.ViaAliasHTTPCases { return viaAliasCases{c} }
func (c viaAliasCases) OK() baretest.ViaAliasHTTPCase    { return baretest.ViaAliasHTTPCase{} }

func TestBareHTTPCases(t *testing.T) {
	leaves := &touchLeaves{}
	baretest.RunHTTP(t, bare.NewHandler(bare.New(leaves)), bareCases{leaves})
}
