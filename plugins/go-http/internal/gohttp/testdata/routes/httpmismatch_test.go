// 入力 routes の、インテグレーションテストの口が、応答が、そのケースのものでないときに、落ちること（plugins/go-http の TestHTTPMismatch が、
// httpcases_test.go と同じモジュールのルートに置いて走らせ、落ちたテストと、その文面を確かめる）。このファイルのテストは、落ちるのが、期待。
package behavior

import (
	"context"
	"net/http/httptest"
	"testing"

	"example.com/app/gen/api/apitest"
	pkgtype "example.com/app/gen/type"
)

// mismatchCases は、apiCases のうち、いくつかのケースを、応答が、そのケースのものにならないように、差し替えたもの。
// 差し替えたケースの Then は、呼ばれない（呼ばれれば、"Then was called" で落ちる）。
type mismatchCases struct{ apiCases }

func thenString(t *testing.T, _ string, _ *httptest.ResponseRecorder) { t.Error("Then was called") }
func thenItem(t *testing.T, _ *pkgtype.Item, _ *httptest.ResponseRecorder) {
	t.Error("Then was called")
}
func thenItems(t *testing.T, _ []pkgtype.Item, _ *httptest.ResponseRecorder) {
	t.Error("Then was called")
}
func thenNothing(t *testing.T, _ *httptest.ResponseRecorder) { t.Error("Then was called") }

type healthMismatch struct{ apiCases }

// 正常のケースなのに、例外が返る
func (c mismatchCases) Health() apitest.HealthHTTPCases { return healthMismatch{c.apiCases} }
func (c healthMismatch) OK() apitest.HealthHTTPCase {
	return apitest.HealthHTTPCase{Given: c.returns(pkgtype.NotFound{}), Then: thenString}
}

type firstMismatch struct{ firstCases }

func (c mismatchCases) First() apitest.FirstHTTPCases { return firstMismatch{firstCases{c.apiCases}} }

// op を呼ぶ前の失敗のケースなのに、要求が読めて、op が呼ばれる（省略可能な結果で、値がない: 204）
func (c firstMismatch) BadRequest() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(nil), Then: thenItem}
}

// 別の例外が返る
func (c firstMismatch) NotFound() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(pkgtype.Gone{}), Then: thenItem}
}

// 例外のケースなのに、正常に終わる
func (c firstMismatch) Limited() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(nil), Then: thenItem}
}

// @http.status を持つ例外のケースなのに、持たない例外が返る
func (c firstMismatch) Gone() apitest.FirstHTTPCase {
	return apitest.FirstHTTPCase{In: firstIn, Given: c.returns(pkgtype.Leaky{Secret: "s"}), Then: thenItem}
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

// @http.status を持たない例外のケースなのに、正常に終わる
func (c mismatchCases) Search() apitest.SearchHTTPCases {
	return searchMismatch{searchCases{c.apiCases}}
}
func (c searchMismatch) Broken() apitest.SearchHTTPCase {
	return apitest.SearchHTTPCase{In: searchIn, Given: c.returns(nil), Then: thenItems}
}

// Given が失敗を返せば、そのテストは、そこで落ちる（要求は、送られない）
func (c searchMismatch) Leaky() apitest.SearchHTTPCase {
	return apitest.SearchHTTPCase{In: searchIn, Given: func(context.Context) error { return context.Canceled }, Then: thenItems}
}

func TestHTTPMismatch(t *testing.T) {
	leaves, h := newAPI()
	apitest.RunHTTP(t, h, mismatchCases{apiCases{leaves}})
}
