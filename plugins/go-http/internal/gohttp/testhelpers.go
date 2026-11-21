package gohttp

// このファイルは、テストのパッケージの http.go の補助のテキスト（docs/plugin-go-http.md「インテグレーションテストの口」の「テストの補助」）。
// 中身は、サンプル tests/samples/shop-server/ の gen/server/servertest/http.go の通りで、型にも、仕様にも依らない（どの出力でも、同じ
// テキスト）。出力ごとに変わるのは、どれを出すか（使うものだけ。testfile.go の helpers）だけ。値で変わる振る舞い（値がなければ送らない、
// 文字列の形、204 でもよい結果）は、この補助が、実行時に決める（docs/requirements.md「出力を推論しない」）。
//
// 補助の中のパッケージの名前（bytes、context、json、http、httptest、url、reflect、strconv、strings、testing、runtime）は、そのまま書く
// （helpers.go の先頭のコメントと同じ）。

var (
	// 送る要求一つと、本文の項目一つ
	testHelperCall = helper{imports: []string{"net/http", "net/url"}, text: `// httpCall は、送る要求一つ。入口の入力から、通信の上の形（docs/plugin-go-http.md「通信の形」の「要求」）で組み立てる。
type httpCall struct {
	method, path string
	headers      http.Header
	values       url.Values
	items        []httpItem // 本文の項目（@http.body）。書いた順
	whole        any        // 本文の全体（@http.bodyObj）
	hasBody      bool       // 本文を送るか（本文のデコレータのフィールドが、一つでもある）
}

// httpItem は、本文の項目一つ: キーと、その値。
type httpItem struct {
	name  string
	value any
}

`}
	// httpCall のメソッド: 「要求の組み立て」の表の一行
	testHelperHeader = helper{imports: []string{"net/http"}, text: `// header は、ヘッダの項目一つ（@http.header）。値がない（省略可能で、ない）なら、送らない。
func (c *httpCall) header(name string, v any) {
	if text, ok := httpText(v); ok {
		if c.headers == nil {
			c.headers = http.Header{}
		}
		c.headers.Set(name, text)
	}
}

`}
	testHelperQuery = helper{imports: []string{"net/url"}, text: `// query は、クエリの項目一つ（@http.query）。値がない（省略可能で、ない）なら、送らない。
func (c *httpCall) query(name string, v any) {
	if text, ok := httpText(v); ok {
		if c.values == nil {
			c.values = url.Values{}
		}
		c.values.Set(name, text)
	}
}

`}
	testHelperBody = helper{imports: []string{"reflect"}, text: `// body は、本文の項目一つ（@http.body）: 本文の、JSON のオブジェクトの、キー name の値。値がない（nil のポインタ）なら、キーを省く。
func (c *httpCall) body(name string, v any) {
	c.hasBody = true
	if x := reflect.ValueOf(v); x.Kind() == reflect.Pointer && x.IsNil() {
		return
	}
	c.items = append(c.items, httpItem{name, v})
}

`}
	testHelperHeaderObj = helper{text: `// headerObj は、ヘッダの全体（@http.headerObj）: model のフィールドの一つずつを、その json タグの名前（仕様のフィールドの名前）のヘッダにする。
func (c *httpCall) headerObj(v any) { httpFields(v, c.header) }

`}
	testHelperQueryObj = helper{text: `// queryObj は、クエリの全体（@http.queryObj）: model のフィールドの一つずつを、その json タグの名前（仕様のフィールドの名前）のクエリの項目にする。
func (c *httpCall) queryObj(v any) { httpFields(v, c.query) }

`}
	testHelperBodyObj = helper{text: `// bodyObj は、本文の全体（@http.bodyObj）: 本文が、その model の JSON。
func (c *httpCall) bodyObj(v any) {
	c.hasBody = true
	c.whole = v
}

`}
	// 要求を組み立て、given、edit の順に呼んでから、handler に送る
	testHelperSend = helper{imports: []string{"bytes", "encoding/json", "net/http", "net/http/httptest", "testing"}, text: `// send は、要求を組み立てて handler に送り、応答を返す。送る前に、edit（要求の手直し）を呼ぶ（nil なら、呼ばない）。
func (c *httpCall) send(t *testing.T, handler http.Handler, edit func(r *http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	target := c.path
	if len(c.values) > 0 {
		target += "?" + c.values.Encode()
	}
	var body bytes.Buffer
	switch {
	case c.whole != nil:
		data, err := json.Marshal(c.whole)
		if err != nil {
			t.Fatalf("encode the request body: %v", err)
		}
		body.Write(data)
	case c.hasBody:
		body.WriteByte('{')
		for i, item := range c.items {
			key, _ := json.Marshal(item.name)
			value, err := json.Marshal(item.value)
			if err != nil {
				t.Fatalf("encode the request body item %s: %v", item.name, err)
			}
			if i > 0 {
				body.WriteByte(',')
			}
			body.Write(key)
			body.WriteByte(':')
			body.Write(value)
		}
		body.WriteByte('}')
	}
	r := httptest.NewRequest(c.method, target, &body)
	for name, values := range c.headers {
		r.Header[name] = values
	}
	if c.hasBody {
		r.Header.Set("Content-Type", "application/json")
	}
	if edit != nil {
		edit(r)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, r)
	return res
}

`}
	// ヘッダとクエリの値の文字列
	testHelperText = helper{imports: []string{"reflect", "strconv"}, text: `// httpText は、ヘッダとクエリの値の文字列。形は、実行時に、v の型で決める: 文字列はそのまま、整数は 10 進、小数は strconv の 'g'、
// 真偽値は true / false。値がない（nil のポインタ）なら false。
func httpText(v any) (string, bool) {
	x := reflect.ValueOf(v)
	if x.Kind() == reflect.Pointer {
		if x.IsNil() {
			return "", false
		}
		x = x.Elem()
	}
	switch x.Kind() {
	case reflect.Int64:
		return strconv.FormatInt(x.Int(), 10), true
	case reflect.Float64:
		return strconv.FormatFloat(x.Float(), 'g', -1, 64), true
	case reflect.Bool:
		return strconv.FormatBool(x.Bool()), true
	}
	return x.String(), true
}

`}
	// 全体の形の model のフィールドの一つずつを、項目にする
	testHelperFields = helper{imports: []string{"reflect", "strings"}, text: `// httpFields は、全体の形の model（構造体）v のフィールドの一つずつを、その json タグの名前の項目として、item に渡す。
func httpFields(v any, item func(name string, v any)) {
	x := reflect.ValueOf(v)
	for i := 0; i < x.NumField(); i++ {
		name, _, _ := strings.Cut(x.Type().Field(i).Tag.Get("json"), ",")
		item(name, x.Field(i).Interface())
	}
}

`}
	// 応答が、結果のものであることの確かめ
	testHelperWantResult = helper{imports: []string{"bytes", "encoding/json", "net/http", "net/http/httptest", "reflect", "testing"}, text: `// httpWantResult は、応答 res が、成功の応答（docs/plugin-go-http.md「通信の形」の「成功の応答」）であることを確かめ、返った値を dst に読む。
// 返す値がない（dst が nil: 値を返さない op。dst の指す先がポインタ: 省略可能な結果）なら、204 で本文なし。
// あれば、200 と、dst に読める JSON（null は、値ではない）。
func httpWantResult(t *testing.T, res *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if res.Code == http.StatusNoContent && (dst == nil || reflect.TypeOf(dst).Elem().Kind() == reflect.Pointer) {
		if res.Body.Len() > 0 {
			t.Fatalf("want no body with status 204, got %s", res.Body.Bytes())
		}
		return
	}
	if dst == nil {
		t.Fatalf("want status 204, got %d: %s", res.Code, res.Body.Bytes())
	}
	data := httpWantJSON(t, res, http.StatusOK)
	if string(bytes.TrimSpace(data)) == "null" {
		t.Fatalf("want a value, got null")
	}
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("want a body readable as the result, got %s: %v", data, err)
	}
}

`}
	testHelperWantBadRequest = helper{imports: []string{"encoding/json", "net/http", "net/http/httptest", "testing"}, text: `// httpWantBadRequest は、応答 res が、op を呼ぶ前の失敗（docs/plugin-go-http.md「通信の形」の「op を呼ぶ前の失敗」）であることを確かめる:
// 400、error が http.BadRequest、value に、message（空でない）と、violations（配列）。
func httpWantBadRequest(t *testing.T, res *httptest.ResponseRecorder) {
	t.Helper()
	var value struct {
		Message    string ` + "`" + `json:"message"` + "`" + `
		Violations []struct {
			Path       string ` + "`" + `json:"path"` + "`" + `
			Constraint string ` + "`" + `json:"constraint"` + "`" + `
		} ` + "`" + `json:"violations"` + "`" + `
	}
	raw := httpWantFailure(t, res, http.StatusBadRequest, "http.BadRequest")
	if err := json.Unmarshal(raw, &value); err != nil || value.Message == "" || value.Violations == nil {
		t.Fatalf("want a value with message and violations, got %s", raw)
	}
}

`}
	testHelperWantThrown = helper{imports: []string{"encoding/json", "net/http/httptest", "reflect", runtimePath, "testing"}, text: `// httpWantThrown は、応答 res が、例外 E の失敗の応答であることを確かめる: ステータス status、error が E の FQN。
// E が値を運ぶ（引数ごとのフィールドを、一つでも持つ）なら、value は、E に読めるオブジェクト。運ばないなら、value はない。
func httpWantThrown[E runtime.Exception](t *testing.T, res *httptest.ResponseRecorder, status int) {
	t.Helper()
	var e E
	raw := httpWantFailure(t, res, status, e.HozoException())
	if x := reflect.TypeOf(e); x.Kind() != reflect.Struct || x.NumField() == 0 {
		if raw != nil {
			t.Fatalf("want no value for %s, got %s", e.HozoException(), raw)
		}
		return
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("want a value readable as %s, got %s: %v", e.HozoException(), raw, err)
	}
}

`}
	testHelperWantInternal = helper{imports: []string{"net/http", "net/http/httptest", "testing"}, text: `// httpWantInternal は、応答 res が、中身を出さない失敗であることを確かめる: 500、error が internal、value はない。
func httpWantInternal(t *testing.T, res *httptest.ResponseRecorder) {
	t.Helper()
	if raw := httpWantFailure(t, res, http.StatusInternalServerError, "internal"); raw != nil {
		t.Fatalf("want no value, got %s", raw)
	}
}

`}
	testHelperWantFailure = helper{imports: []string{"encoding/json", "net/http/httptest", "testing"}, text: `// httpWantFailure は、応答 res が、ステータス status の、失敗の応答（docs/plugin-go-http.md「通信の形」の「失敗の応答」）で、error が name で
// あることを確かめ、value を返す（なければ nil）。
func httpWantFailure(t *testing.T, res *httptest.ResponseRecorder, status int, name string) json.RawMessage {
	t.Helper()
	var body struct {
		Error string          ` + "`" + `json:"error"` + "`" + `
		Value json.RawMessage ` + "`" + `json:"value"` + "`" + `
	}
	data := httpWantJSON(t, res, status)
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("want a failure body, got %s: %v", data, err)
	}
	if body.Error != name {
		t.Fatalf("want error %s, got %s", name, data)
	}
	return body.Value
}

`}
	testHelperWantJSON = helper{imports: []string{"net/http/httptest", "testing"}, text: `// httpWantJSON は、応答 res が、ステータス status の、JSON の本文を持つことを確かめ、本文を返す。
func httpWantJSON(t *testing.T, res *httptest.ResponseRecorder, status int) []byte {
	t.Helper()
	if res.Code != status {
		t.Fatalf("want status %d, got %d: %s", status, res.Code, res.Body.Bytes())
	}
	if got := res.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("want Content-Type application/json, got %q", got)
	}
	return res.Body.Bytes()
}

`}
)
