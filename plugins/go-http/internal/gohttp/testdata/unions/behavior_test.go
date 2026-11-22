// 入力 unions の出力を動かす（plugins/go-http の TestBehavior が、基底 go・go-valid・go-http の出力を書いたモジュールのルートに置いて
// 走らせる）。確かめること（docs/plugin-go-http.md「通信の形」の「union」）:
//
//   - union は、アームの値の JSON、そのまま（どのアームかを言うキーは、ない）。包む型（別の名前空間の model のアーム）は、その中身。
//   - 読む側は、決め手のキーで、アームを決める。読んだ値を書けば、同じ JSON に戻る（往復）。
//   - どのアームの決め手もない JSON、二つのアームの決め手がある JSON、オブジェクトでない JSON は、400 `body: invalid JSON` で、
//     op を呼ばない。
//   - union を、フィールド、省略可能なフィールド、配列、インデックスの model の値に持つ model も、同じに読み書きする。
//   - 結果の、union の配列。例外の引数の union。
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
	"example.com/app/gen/shape"
)

// leaves は api の葉の op。受けた入力を覚え、そのまま（か、決めた値を）返す。
type leaves struct {
	calls   int
	figure  api.Figure
	drawing api.Drawing
	figures []api.Figure
	err     error
}

func (l *leaves) PutFigure(_ context.Context, in api.PutInput) (api.Figure, error) {
	l.calls++
	l.figure = in.Figure
	return in.Figure, l.err
}

func (l *leaves) PutDrawing(_ context.Context, in api.DrawInput) (api.Drawing, error) {
	l.calls++
	l.drawing = in.Drawing
	return in.Drawing, l.err
}

func (l *leaves) Figures(context.Context) ([]api.Figure, error) { return l.figures, l.err }
func (l *leaves) Clash(context.Context) (string, error)         { return "", l.err }

func do(t *testing.T, l *leaves, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	api.NewHandler(api.New(l)).ServeHTTP(rec, req)
	data, _ := io.ReadAll(rec.Body)
	return rec.Code, strings.TrimSpace(string(data))
}

func ptr[T any](v T) *T { return &v }

// TestArms は、アームごとに、読んだ Go の値と、書いた JSON（往復）。
func TestArms(t *testing.T) {
	for _, c := range []struct {
		name, json string
		want       api.Figure
	}{
		{"この名前空間の model", `{"x":1,"y":2}`, api.Dot{X: 1, Y: 2}},
		{"別の名前空間の model（包む型）", `{"radius":2.5,"label":"c"}`, api.FigureCircle{Value: shape.Circle{Radius: 2.5, Label: ptr("c")}}},
		{"省略可能なフィールドのない値", `{"width":3,"height":4}`, api.FigureBox{Value: shape.Box{Width: 3, Height: 4}}},
		{"その場で書いた model", `{"text":"hi"}`, api.FigureText{Text: "hi"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			l := &leaves{}
			code, body := do(t, l, "POST", "/figure", `{"figure":`+c.json+`}`)
			if code != http.StatusOK || body != c.json {
				t.Fatalf("応答 = %d %s, want 200 %s", code, body, c.json)
			}
			if !reflect.DeepEqual(l.figure, c.want) {
				t.Fatalf("葉が受けた値 = %#v, want %#v", l.figure, c.want)
			}
		})
	}
}

// TestUnreadable は、アームを決められない JSON: 400 `body: invalid JSON` で、op を呼ばない。
func TestUnreadable(t *testing.T) {
	for name, figure := range map[string]string{
		"どのアームの決め手もない":  `{"label":"x"}`,
		"空のオブジェクト":      `{}`,
		"二つのアームの決め手がある": `{"x":1,"y":2,"radius":3}`,
		"オブジェクトでない":     `"circle"`,
		"配列":            `[{"x":1,"y":2}]`,
		"決め手の値の型が合わない":  `{"x":"one","y":2}`,
	} {
		t.Run(name, func(t *testing.T) {
			l := &leaves{}
			code, body := do(t, l, "POST", "/figure", `{"figure":`+figure+`}`)
			want := `{"error":"http.BadRequest","value":{"message":"body: invalid JSON","violations":[]}}`
			if code != http.StatusBadRequest || body != want {
				t.Fatalf("応答 = %d %s, want 400 %s", code, body, want)
			}
			if l.calls != 0 {
				t.Fatalf("op が呼ばれた")
			}
		})
	}
}

// TestInside は、union を中に持つ model: フィールド、省略可能なフィールド（ない、null）、配列、インデックスの model の値。
func TestInside(t *testing.T) {
	l := &leaves{}
	in := `{"title":"t","main":{"text":"m","size":3},"all":[{"x":1,"y":2},{"radius":1}],"named":{"a":{"width":1,"height":2},"b":{"x":0,"y":0}}}`
	code, body := do(t, l, "POST", "/drawing", in)
	if code != http.StatusOK || body != in {
		t.Fatalf("応答 = %d %s, want 200 %s", code, body, in)
	}
	want := api.Drawing{
		Title: "t",
		Main:  api.FigureText{Text: "m", Size: ptr(int64(3))},
		All:   []api.Figure{api.Dot{X: 1, Y: 2}, api.FigureCircle{Value: shape.Circle{Radius: 1}}},
		Named: api.FiguresByName{"a": api.FigureBox{Value: shape.Box{Width: 1, Height: 2}}, "b": api.Dot{}},
	}
	if !reflect.DeepEqual(l.drawing, want) {
		t.Fatalf("葉が受けた値 = %#v, want %#v", l.drawing, want)
	}
	// 省略可能な union: 値があるとき。null は、ない、と読む
	l = &leaves{}
	if code, body := do(t, l, "POST", "/drawing", `{"title":"t","main":{"x":1,"y":1},"extra":{"radius":9},"all":[],"named":{}}`); code != http.StatusOK ||
		body != `{"title":"t","main":{"x":1,"y":1},"extra":{"radius":9},"all":[],"named":{}}` {
		t.Fatalf("応答 = %d %s", code, body)
	}
	if !reflect.DeepEqual(l.drawing.Extra, api.Figure(api.FigureCircle{Value: shape.Circle{Radius: 9}})) {
		t.Fatalf("extra = %#v", l.drawing.Extra)
	}
	l = &leaves{}
	if code, _ := do(t, l, "POST", "/drawing", `{"title":"t","main":{"x":1,"y":1},"extra":null,"all":null,"named":null}`); code != http.StatusOK || l.drawing.Extra != nil {
		t.Fatalf("null の extra: 応答 = %d, extra = %#v", code, l.drawing.Extra)
	}
	// 中の union を、決められない
	l = &leaves{}
	if code, _ := do(t, l, "POST", "/drawing", `{"title":"t","main":{"x":1,"y":1},"all":[{"label":"?"}],"named":{}}`); code != http.StatusBadRequest || l.calls != 0 {
		t.Fatalf("決められない要素: 応答 = %d, 呼び出し = %d", code, l.calls)
	}
}

// TestResults は、結果の、union の配列と、例外の引数の union。
func TestResults(t *testing.T) {
	l := &leaves{figures: []api.Figure{api.Dot{X: 1, Y: 2}, api.FigureBox{Value: shape.Box{Width: 1, Height: 2, Label: ptr("b")}}}}
	if code, body := do(t, l, "GET", "/figures", ""); code != http.StatusOK || body != `[{"x":1,"y":2},{"width":1,"height":2,"label":"b"}]` {
		t.Fatalf("応答 = %d %s", code, body)
	}
	if code, body := do(t, &leaves{}, "GET", "/figures", ""); code != http.StatusOK || body != `[]` {
		t.Fatalf("値のない配列: 応答 = %d %s", code, body)
	}
	l = &leaves{err: api.Clash{Other: api.FigureText{Text: "t"}}}
	if code, body := do(t, l, "GET", "/clash", ""); code != http.StatusConflict || body != `{"error":"api.Clash","value":{"other":{"text":"t"}}}` {
		t.Fatalf("例外: 応答 = %d %s", code, body)
	}
}

// cases は、テストのパッケージの口を満たす: 入力に、union の値を持つ。
type cases struct{}

func (cases) PutFigure() apitest.PutFigureHTTPCases   { return putFigure{} }
func (cases) PutDrawing() apitest.PutDrawingHTTPCases { return putDrawing{} }
func (cases) Figures() apitest.FiguresHTTPCases       { return figures{} }
func (cases) Clash() apitest.ClashHTTPCases           { return clash{} }

type putFigure struct{}

func (putFigure) OK() apitest.PutFigureHTTPCase {
	sent := api.FigureCircle{Value: shape.Circle{Radius: 2}}
	return apitest.PutFigureHTTPCase{
		In: api.PutInput{Figure: sent},
		Then: func(t *testing.T, got api.Figure, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
			if !reflect.DeepEqual(got, api.Figure(sent)) {
				t.Errorf("got = %#v, want %#v", got, sent)
			}
		},
	}
}

func (putFigure) BadRequest() apitest.PutFigureHTTPCase {
	return apitest.PutFigureHTTPCase{Request: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader(`{"figure":{}}`)) }}
}

type putDrawing struct{}

func (putDrawing) OK() apitest.PutDrawingHTTPCase {
	sent := api.Drawing{Title: "t", Main: api.Dot{X: 1}, All: []api.Figure{api.FigureText{Text: "a"}}, Named: api.FiguresByName{"k": api.FigureBox{Value: shape.Box{Width: 1, Height: 1}}}}
	return apitest.PutDrawingHTTPCase{
		In: api.DrawInput{Drawing: sent},
		Then: func(t *testing.T, got api.Drawing, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
			if !reflect.DeepEqual(got, sent) {
				t.Errorf("got = %#v, want %#v", got, sent)
			}
		},
	}
}

func (putDrawing) BadRequest() apitest.PutDrawingHTTPCase {
	return apitest.PutDrawingHTTPCase{Request: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader(`[]`)) }}
}

type figures struct{}

func (figures) OK() apitest.FiguresHTTPCase {
	return apitest.FiguresHTTPCase{Then: func(t *testing.T, got []api.Figure, _ *httptest.ResponseRecorder, _ *apitest.Fakes) {
		if !reflect.DeepEqual(got, []api.Figure{api.Dot{X: 7, Y: 8}}) {
			t.Errorf("got = %#v", got)
		}
	}}
}

type clash struct{}

func (clash) OK() apitest.ClashHTTPCase { return apitest.ClashHTTPCase{} }

// TestRunHTTP は、口が、union の値を、要求に組み立て、応答から読むこと（作られたケース api.clash/Clash/api.clash を含む）。
func TestRunHTTP(t *testing.T) {
	apitest.RunHTTP(t, &leaves{figures: []api.Figure{api.Dot{X: 7, Y: 8}}}, cases{})
}
