// 入力 routes の、インテグレーションテストの口が、応答が、そのケースのものでないときに、落ちること（plugins/go-http の TestHTTPMismatch が、
// httpcases_test.go と同じモジュールのルートに置いて走らせ、落ちたテストと、その文面を確かめる）。このファイルのテストは、落ちるのが、期待。
package behavior

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/app/gen/api/apitest"
	pkgtype "example.com/app/gen/type"
)

// mismatchCases は、apiCases のうち、いくつかのケースを、応答が、そのケースのものにならないように、差し替えたもの。
// 差し替えたケースの Then は、呼ばれない（呼ばれれば、"Then was called" で落ちる）。
// 例外のケースは、Hozo が、正常のケースから作る（口に、メソッドがない）ので、正常のケースを差し替えると、その入口の、例外のテストも、
// 同じ要求を送る。
type mismatchCases struct{ apiCases }

func thenString(t *testing.T, _ string, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
	t.Error("Then was called")
}
func thenItem(t *testing.T, _ *pkgtype.Item, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
	t.Error("Then was called")
}
func thenItems(t *testing.T, _ []pkgtype.Item, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
	t.Error("Then was called")
}
func thenNothing(t *testing.T, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
	t.Error("Then was called")
}

// noQuery は、要求から、クエリを取り除く Request（@http.required の項目がなくなり、ハンドラは、op を呼ぶ前に、400 で断る）。
func noQuery(r *http.Request) { r.URL.RawQuery = "" }

type healthMismatch struct{ apiCases }

// 正常のケースなのに、例外が返る
func (c mismatchCases) Health() apitest.HealthHTTPCases { return healthMismatch{c.apiCases} }
func (c healthMismatch) OK() apitest.HealthHTTPCase {
	return apitest.HealthHTTPCase{Given: c.returns(pkgtype.NotFound{}), Then: thenString}
}

type firstMismatch struct{ firstCases }

func (c mismatchCases) First() apitest.FirstHTTPCases { return firstMismatch{firstCases{c.apiCases}} }

// 正常のケースなのに、要求が断られる（400）。この入口の、例外のテスト（Hozo が、この正常のケースから作る: NotFound は 404、
// Limited は 429、Gone は 410 を待つ）も、同じ要求を送るので、どれも、400 を受けて、落ちる
func (c firstMismatch) OK() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(nil), Request: noQuery, Then: thenItem}
}

// op を呼ぶ前の失敗のケースなのに、要求が読めて、op が呼ばれる（省略可能な結果で、値がない: 204）
func (c firstMismatch) BadRequest() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(nil), Then: thenItem}
}

type replaceMismatch struct{ replaceCases }

// 値を返さない op の、正常のケースなのに、例外が返る
func (c mismatchCases) Replace() apitest.ReplaceHTTPCases {
	return replaceMismatch{replaceCases{c.apiCases}}
}
func (c replaceMismatch) OK() apitest.ReplaceHTTPCase {
	return apitest.ReplaceHTTPCase{In: replaceIn, Given: c.returns(pkgtype.NotFound{}), Then: thenNothing}
}

type searchMismatch struct{ searchCases }

// 正常のケースなのに、要求が断られる（400）。@http.status を持たない例外のテスト（500 を待つ）も、400 を受けて、落ちる
func (c mismatchCases) Search() apitest.SearchHTTPCases {
	return searchMismatch{searchCases{c.apiCases}}
}
func (c searchMismatch) OK() apitest.SearchHTTPCase {
	return apitest.SearchHTTPCase{In: searchIn, Given: c.returns(nil), Request: noQuery, Then: thenItems}
}

func TestHTTPMismatch(t *testing.T) {
	leaves, _ := newAPI()
	apitest.RunHTTP(t, leaves, mismatchCases{apiCases{leaves}})
}
