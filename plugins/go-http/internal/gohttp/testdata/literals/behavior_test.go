// 入力 literals の出力を動かす（plugins/go-http の TestBehavior が、基底 go・go-valid・go-http の出力を書いたモジュールのルートに置いて
// 走らせる）。確かめること（docs/plugin-http.md「通信の形」の、リテラルの型の行と「union」）:
//
//   - リテラルの型のフィールドは、そのリテラルの JSON を、仕様のフィールドの順の位置に書く。読む側は、キーがないか、値が違えば、
//     400 `body: invalid JSON` で、op を呼ばない（数は、数として比べる）。
//   - リテラルの型のアームは、値そのもの。読む側は、値が同じアームを選ぶ。どのアームの値でもなければ、400。
//   - 印を持つ union の model のアームは、印の値で選ぶ（ほかのフィールドの名前が、同じでも）。包む型は、その中身。印がない、
//     どのアームの印でもない、なら 400。
//   - 結果の、値の集まりの union。例外の引数の、リテラルの型。
//   - テストのパッケージの口（RunHTTP）も、同じ形で、要求を組み立て、応答を読む。
package behavior

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"example.com/app/gen/api"
	"example.com/app/gen/api/apitest"
	"example.com/app/gen/kinds"
)

// leaves は api の葉の op。受けた入力を覚え、そのまま（か、決めた値を）返す。
type leaves struct {
	calls    int
	report   api.Report
	parts    api.PartsIn
	severity api.Severity
	err      error
}

func (l *leaves) PutReport(_ context.Context, in api.ReportIn) (api.Report, error) {
	l.calls++
	l.report = in.Report
	return in.Report, l.err
}

func (l *leaves) PutParts(_ context.Context, in api.PartsIn) ([]api.Type, error) {
	l.calls++
	l.parts = in
	return in.Types, l.err
}

func (l *leaves) Severity(context.Context) (api.Severity, error) { return l.severity, l.err }
func (l *leaves) Clash(context.Context) (string, error)          { return "", l.err }

func do(t *testing.T, l *leaves, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	api.NewHandler(api.New(l)).ServeHTTP(rec, req)
	data, _ := io.ReadAll(rec.Body)
	return rec.Code, strings.TrimSpace(string(data))
}

func ptr[T any](v T) *T { return &v }

// TestRoundTrip は、読んだ Go の値と、書いた JSON（往復）: リテラルの型のフィールド、リテラルの型のアーム、印で見分ける union。
func TestRoundTrip(t *testing.T) {
	for _, c := range []struct {
		name, json string
		want       api.Report
	}{
		{"印が core。印は、フィールドの順の位置（name の後ろ）", `{"severity":"error","of":{"name":"int","kind":"core"},"amount":0.5,"version":2}`,
			api.Report{Severity: api.SeverityError{}, Of: api.CoreType{Name: "int"}, Amount: api.AmountHalf{}}},
		{"印が list（ほかのフィールドの名前は、core と同じ）。省略可能な union は、キーを省く", `{"severity":"warning","of":{"kind":"list","name":"xs"},"version":2}`,
			api.Report{Severity: api.SeverityWarning{}, Of: api.ListType{Name: "xs"}}},
		{"印が map（別の名前空間の model。包む型）", `{"severity":"error","of":{"kind":"map","key":"string"},"amount":true,"version":2}`,
			api.Report{Severity: api.SeverityError{}, Of: api.TypeMap{Value: kinds.MapType{Key: "string"}}, Amount: api.AmountAll{}}},
		{"整数のリテラルの型のアーム", `{"severity":"error","of":{"name":"s","kind":"core","nullable":true},"amount":0,"version":2}`,
			api.Report{Severity: api.SeverityError{}, Of: api.CoreType{Name: "s", Nullable: ptr(true)}, Amount: api.AmountNone{}}},
		{"リテラルの型のアームと混ざる、model のアーム", `{"severity":"error","of":{"kind":"list","name":"x"},"amount":{"n":3},"version":2}`,
			api.Report{Severity: api.SeverityError{}, Of: api.ListType{Name: "x"}, Amount: api.Count{N: 3}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			l := &leaves{}
			code, body := do(t, l, "POST", "/report", c.json)
			if code != http.StatusOK || body != c.json {
				t.Fatalf("応答 = %d %s, want 200 %s", code, body, c.json)
			}
			if !reflect.DeepEqual(l.report, c.want) {
				t.Fatalf("葉が受けた値 = %#v, want %#v", l.report, c.want)
			}
		})
	}
	// 数は、数として比べる: 2.0 は 2、0.0 は 0。キーの順は、問わない。書くときは、リテラルの JSON
	l := &leaves{}
	code, body := do(t, l, "POST", "/report", `{"version":2.0,"amount":0.0,"of":{"kind":"core","name":"int"},"severity":"error"}`)
	if want := `{"severity":"error","of":{"name":"int","kind":"core"},"amount":0,"version":2}`; code != http.StatusOK || body != want {
		t.Fatalf("応答 = %d %s, want 200 %s", code, body, want)
	}
}

// TestUnreadable は、リテラルに合わない値と、アームを決められない JSON: 400 `body: invalid JSON` で、op を呼ばない。
func TestUnreadable(t *testing.T) {
	report := func(severity, of, amount, version string) string {
		return `{"severity":` + severity + `,"of":` + of + amount + version + `}`
	}
	core := `{"name":"int","kind":"core"}`
	for name, body := range map[string]string{
		"リテラルの型のアームの、どの値でもない文字列":       report(`"fatal"`, core, "", `,"version":2`),
		"リテラルの型のアームの位置に、数":             report(`1`, core, "", `,"version":2`),
		"リテラルの型のアームだけの union に、オブジェクト": report(`{"error":true}`, core, "", `,"version":2`),
		"印が、どのアームの値でもない":               report(`"error"`, `{"name":"int","kind":"set"}`, "", `,"version":2`),
		"印のキーがない":                      report(`"error"`, `{"name":"int"}`, "", `,"version":2`),
		"印の値の型が違う":                     report(`"error"`, `{"name":"int","kind":1}`, "", `,"version":2`),
		"印で見分ける union に、文字列":           report(`"error"`, `"core"`, "", `,"version":2`),
		"リテラルの型のフィールドの値が違う":            report(`"error"`, core, "", `,"version":3`),
		"リテラルの型のフィールドのキーがない":           report(`"error"`, core, "", ``),
		"リテラルの型のフィールドの値の型が違う":          report(`"error"`, core, "", `,"version":"2"`),
		"数のリテラルの型のアームの、どの値でもない数":       report(`"error"`, core, `,"amount":1`, `,"version":2`),
		"真偽のリテラルの型のアームの、ない方の値":         report(`"error"`, core, `,"amount":false`, `,"version":2`),
		"決め手のない、オブジェクト（model のアーム）":    report(`"error"`, core, `,"amount":{"m":1}`, `,"version":2`),
	} {
		t.Run(name, func(t *testing.T) {
			l := &leaves{}
			code, got := do(t, l, "POST", "/report", body)
			want := `{"error":"http.BadRequest","value":{"message":"body: invalid JSON","violations":[]}}`
			if code != http.StatusBadRequest || got != want {
				t.Fatalf("応答 = %d %s, want 400 %s", code, got, want)
			}
			if l.calls != 0 {
				t.Fatalf("op が呼ばれた")
			}
		})
	}
}

// TestItems は、本文の項目と、結果の、union と、その配列。
func TestItems(t *testing.T) {
	l := &leaves{}
	types := `[{"name":"int","kind":"core"},{"kind":"list","name":"xs"},{"kind":"map","key":"k"}]`
	code, body := do(t, l, "POST", "/parts", `{"severity":"warning","types":`+types+`}`)
	if code != http.StatusOK || body != types {
		t.Fatalf("応答 = %d %s, want 200 %s", code, body, types)
	}
	want := api.PartsIn{Severity: api.SeverityWarning{}, Types: []api.Type{api.CoreType{Name: "int"}, api.ListType{Name: "xs"}, api.TypeMap{Value: kinds.MapType{Key: "k"}}}}
	if !reflect.DeepEqual(l.parts, want) {
		t.Fatalf("葉が受けた値 = %#v, want %#v", l.parts, want)
	}
	l = &leaves{}
	if code, _ := do(t, l, "POST", "/parts", `{"severity":"warning","types":[{"kind":"core","name":"a"},{"kind":"tuple"}]}`); code != http.StatusBadRequest || l.calls != 0 {
		t.Fatalf("決められない要素: 応答 = %d, 呼び出し = %d", code, l.calls)
	}
}

// TestResults は、結果の、値の集まりの union と、例外の引数の、値の集まりの union と、リテラルの型のフィールドを持つ model。
func TestResults(t *testing.T) {
	for _, c := range []struct {
		severity api.Severity
		want     string
	}{{api.SeverityError{}, `"error"`}, {api.SeverityWarning{}, `"warning"`}} {
		if code, body := do(t, &leaves{severity: c.severity}, "GET", "/severity", ""); code != http.StatusOK || body != c.want {
			t.Fatalf("応答 = %d %s, want 200 %s", code, body, c.want)
		}
	}
	l := &leaves{err: api.Clash{Level: api.SeverityWarning{}, Other: api.CoreType{Name: "int"}}}
	if code, body := do(t, l, "GET", "/clash", ""); code != http.StatusConflict || body != `{"error":"api.Clash","value":{"level":"warning","other":{"name":"int","kind":"core"}}}` {
		t.Fatalf("例外: 応答 = %d %s", code, body)
	}
}

// cases は、テストのパッケージの口を満たす: 入力と、結果に、リテラルの型を持つ。
type cases struct{}

func (cases) PutReport() apitest.PutReportHTTPCases { return putReport{} }
func (cases) PutParts() apitest.PutPartsHTTPCases   { return putParts{} }
func (cases) Severity() apitest.SeverityHTTPCases   { return severity{} }
func (cases) Clash() apitest.ClashHTTPCases         { return clash{} }

type putReport struct{}

func (putReport) OK() apitest.PutReportHTTPCase {
	sent := api.Report{Severity: api.SeverityWarning{}, Of: api.TypeMap{Value: kinds.MapType{Key: "k"}}, Amount: api.AmountHalf{}}
	return apitest.PutReportHTTPCase{
		In: api.ReportIn{Report: sent},
		Then: func(t *testing.T, got api.Report, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
			if !reflect.DeepEqual(got, sent) {
				t.Errorf("got = %#v, want %#v", got, sent)
			}
		},
	}
}

func (putReport) BadRequest() apitest.PutReportHTTPCase {
	return apitest.PutReportHTTPCase{Request: func(r *http.Request) {
		r.Body = io.NopCloser(strings.NewReader(`{"severity":"error","of":{"name":"int","kind":"core"},"version":1}`))
	}}
}

type putParts struct{}

func (putParts) OK() apitest.PutPartsHTTPCase {
	sent := []api.Type{api.CoreType{Name: "int"}, api.ListType{Name: "xs"}}
	return apitest.PutPartsHTTPCase{
		In: api.PartsIn{Severity: api.SeverityError{}, Types: sent},
		Then: func(t *testing.T, got []api.Type, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
			if !reflect.DeepEqual(got, sent) {
				t.Errorf("got = %#v, want %#v", got, sent)
			}
		},
	}
}

func (putParts) BadRequest() apitest.PutPartsHTTPCase {
	return apitest.PutPartsHTTPCase{Request: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader(`{"severity":"fatal","types":[]}`)) }}
}

type severity struct{}

func (severity) OK() apitest.SeverityHTTPCase {
	return apitest.SeverityHTTPCase{Then: func(t *testing.T, got api.Severity, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
		if got != (api.SeverityWarning{}) {
			t.Errorf("got = %#v, want SeverityWarning{}", got)
		}
	}}
}

type clash struct{}

func (clash) OK() apitest.ClashHTTPCase { return apitest.ClashHTTPCase{} }

// TestRunHTTP は、口が、リテラルの型の値を、要求に組み立て、応答から読むこと。
func TestRunHTTP(t *testing.T) {
	apitest.RunHTTP(t, &leaves{severity: api.SeverityWarning{}}, cases{})
}
