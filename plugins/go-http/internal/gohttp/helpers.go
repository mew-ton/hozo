package gohttp

// このファイルは、http.go の補助のテキスト（docs/plugin-go-http.md「補助」）。中身は、サンプル tests/samples/shop-server/ の
// gen/server/http.go の通りで、型にも、仕様にも依らない（どの出力でも、同じテキスト）。出力ごとに変わるのは、どれを出すか
// （使うものだけ）と、httpWriteError の例外の並びだけ（render.go の helpers）。値で変わる振る舞い（項目がないときの扱い、文字列の
// 読み方、値がなければ 204）は、この補助が、実行時に決める（docs/requirements.md「出力を推論しない」）。
//
// 補助の中のパッケージの名前（http、json、io、errors、reflect、strconv、strings、runtime）は、そのまま書く: 標準ライブラリと
// runtime の import の名前は変わらない（docs/plugin-go.md「パッケージと import パス」の「import の名前」）。import に足すのは、
// 補助を出す側（helpers）。

// helper は、補助一つ（か、いつも一緒に出す一組）: テキストと、それが使うパッケージの import パス。
type helper struct {
	text    string
	imports []string
}

// 補助が使うパッケージ（import パス → package 節の名前）。
var helperPackages = map[string]string{
	"encoding/json": "json",
	"errors":        "errors",
	"io":            "io",
	"net/http":      "http",
	"reflect":       "reflect",
	"strconv":       "strconv",
	"strings":       "strings",
	runtimePath:     "runtime",
}

var (
	// 失敗の応答の本文の形（「通信の形」）
	helperErrorBody = helper{text: `// httpErrorBody は失敗の応答の本文。
type httpErrorBody struct {
	// 例外の FQN。op を呼ぶ前の失敗は "http.BadRequest"、中身を出さない失敗は "internal"。
	Error string ` + "`" + `json:"error"` + "`" + `
	// 例外が運ぶ値。
	Value any ` + "`" + `json:"value,omitempty"` + "`" + `
}

`}
	helperBadRequest = helper{text: `// httpBadRequest は "http.BadRequest" が運ぶ値。
type httpBadRequest struct {
	// 理由。要求を読めなかったなら "<場所>: <理由>"、制約に合わなかったなら "invalid input"。
	Message string ` + "`" + `json:"message"` + "`" + `
	// 制約の違反。要求を読めなかったなら空。
	Violations []httpViolation ` + "`" + `json:"violations"` + "`" + `
}

// httpViolation は制約の違反一つ。
type httpViolation struct {
	// 通信の上の場所: body、query、header と、その中のパス（body.email、body.lines[0].quantity、query.page）。
	Path string ` + "`" + `json:"path"` + "`" + `
	// 違反した制約の名前（valid の語彙のデコレータの名前）。
	Constraint string ` + "`" + `json:"constraint"` + "`" + `
}

`}
	// 入力の model のフィールドと、通信の上の場所の組
	helperPlace = helper{text: `// httpPlace は、入力の model のフィールドと、それを読む通信の上の場所の組。
type httpPlace struct{ field, place string }

`}
	// 項目がないときの扱い（httpOption）と、それを作る二つ。missing を出すときに、三つとも出す
	helperOptions = helper{text: `// httpOption は、項目がないときの扱い。フィールドのデコレータ（@http.required、@http.default）が言う。
type httpOption struct {
	required bool // @http.required
	value    any  // @http.default の値。なければ nil
}

// httpRequired は @http.required: その項目がなければ、読めない要求（"<場所> <名前>: missing"）。
var httpRequired = httpOption{required: true}

// httpDefault は @http.default(v): その項目がなければ、フィールドを v にする。
func httpDefault(v any) httpOption { return httpOption{value: v} }

`}
	// httpRequest の型。フィールド items（本文のオブジェクト）は、メソッド body を出すときだけ
	helperRequestHead = helper{imports: []string{"net/http"}, text: `// httpRequest は、要求一つを、入力のフィールドに読む。読めなかった最初の理由を fail に持ち、その後は何も読まない。
type httpRequest struct {
	r     *http.Request
	fail  string                     // 読めなかった理由（http.BadRequest の message）。読めていれば空
`}
	helperRequestItems = helper{imports: []string{"encoding/json"}, text: `	items map[string]json.RawMessage // 本文のオブジェクト。body が、初めて呼ばれたときに読む
`}
	helperRequestTail = helper{text: "}\n\n"}
	// httpRequest のメソッド
	helperHeader = helper{text: `// header は、ヘッダ name の値を dst（フィールドへのポインタ）に読む（@http.header）。空の値は、ないものと読む。
func (q *httpRequest) header(name string, dst any, opts ...httpOption) {
	text := q.r.Header.Get(name)
	q.text("header", name, text, text != "", dst, opts)
}

`}
	helperQuery = helper{text: `// query は、クエリ name の値を dst（フィールドへのポインタ）に読む（@http.query）。同じ名前が二度あれば、最初の値。
func (q *httpRequest) query(name string, dst any, opts ...httpOption) {
	values := q.r.URL.Query()
	q.text("query", name, values.Get(name), values.Has(name), dst, opts)
}

`}
	helperBody = helper{imports: []string{"encoding/json", "io"}, text: `// body は、本文（JSON のオブジェクト）のキー name の値を dst（フィールドへのポインタ）に読む（@http.body）。
// キーがあれば、値が null でも、あるものと読む。
func (q *httpRequest) body(name string, dst any, opts ...httpOption) {
	if q.fail != "" {
		return
	}
	if q.items == nil {
		data, err := io.ReadAll(q.r.Body)
		if err == nil {
			err = json.Unmarshal(data, &q.items)
		}
		if err != nil || q.items == nil {
			q.fail = "body: invalid JSON"
			return
		}
	}
	raw, present := q.items[name]
	if !present {
		q.missing("body", name, dst, opts)
		return
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		q.fail = "body: invalid JSON"
	}
}

`}
	helperHeaderObj = helper{imports: []string{"reflect", "strings"}, text: `// headerObj は、ヘッダの全体を dst（model のフィールドへのポインタ）に読む（@http.headerObj）: model のフィールドごとに、
// その json タグの名前（仕様のフィールドの名前）のヘッダを、header と同じに読む。ないヘッダのフィールドは、零値のまま。
func (q *httpRequest) headerObj(dst any) {
	v := reflect.ValueOf(dst).Elem()
	for i := 0; i < v.NumField(); i++ {
		name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		q.header(name, v.Field(i).Addr().Interface())
	}
}

`}
	helperQueryObj = helper{imports: []string{"reflect", "strings"}, text: `// queryObj は、クエリの全体を dst（model のフィールドへのポインタ）に読む（@http.queryObj）: model のフィールドごとに、
// その json タグの名前（仕様のフィールドの名前）のクエリを、query と同じに読む。ないクエリのフィールドは、零値のまま。
func (q *httpRequest) queryObj(dst any) {
	v := reflect.ValueOf(dst).Elem()
	for i := 0; i < v.NumField(); i++ {
		name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		q.query(name, v.Field(i).Addr().Interface())
	}
}

`}
	helperBodyObj = helper{imports: []string{"encoding/json", "io"}, text: `// bodyObj は、本文の全体を、JSON の値一つとして dst（model のフィールドへのポインタ）に読む（@http.bodyObj）。
func (q *httpRequest) bodyObj(dst any) {
	if q.fail != "" {
		return
	}
	data, err := io.ReadAll(q.r.Body)
	if err == nil {
		err = json.Unmarshal(data, dst)
	}
	if err != nil {
		q.fail = "body: invalid JSON"
	}
}

`}
	helperMissing = helper{imports: []string{"reflect"}, text: `// missing は、項目がなかったときの扱い。決めるのは、行に渡された opts だけで、dst の型は見ない:
// httpRequired なら、読めない要求。httpDefault(v) なら、dst（フィールドへのポインタ）の指す先を v にする。
// どちらもなければ、何も書かない（フィールドは零値のまま）。
func (q *httpRequest) missing(place, name string, dst any, opts []httpOption) {
	for _, o := range opts {
		switch {
		case o.required:
			q.fail = place + " " + name + ": missing"
		case o.value != nil:
			v := reflect.ValueOf(dst).Elem()
			v.Set(reflect.ValueOf(o.value).Convert(v.Type()))
		}
	}
}

`}
	helperText = helper{imports: []string{"reflect", "strconv"}, text: `// text は、通信の上の文字列（ヘッダとクエリの値）一つを dst（フィールドへのポインタ）に読む。読み方は、実行時に、dst の指す型で決める:
// ポインタ（省略可能）なら、値を置く先を作る。文字列はそのまま、整数、小数、真偽値は strconv で読む。
// 値がない（present が false）ときの扱いは、missing。
func (q *httpRequest) text(place, name, text string, present bool, dst any, opts []httpOption) {
	if q.fail != "" {
		return
	}
	if !present {
		q.missing(place, name, dst, opts)
		return
	}
	v := reflect.ValueOf(dst).Elem()
	if v.Kind() == reflect.Pointer {
		v.Set(reflect.New(v.Type().Elem()))
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(text)
	case reflect.Int64:
		x, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			q.fail = place + " " + name + ": invalid int"
		}
		v.SetInt(x)
	case reflect.Float64:
		x, err := strconv.ParseFloat(text, 64)
		if err != nil {
			q.fail = place + " " + name + ": invalid float"
		}
		v.SetFloat(x)
	case reflect.Bool:
		x, err := strconv.ParseBool(text)
		if err != nil {
			q.fail = place + " " + name + ": invalid boolean"
		}
		v.SetBool(x)
	}
}

`}
	helperOK = helper{imports: []string{"net/http"}, text: `// ok は、要求を読めたかどうか。読めなかったなら、400（http.BadRequest）を返して false。op は呼ばない。
func (q *httpRequest) ok(w http.ResponseWriter) bool {
	if q.fail == "" {
		return true
	}
	httpWriteJSON(w, http.StatusBadRequest, httpErrorBody{
		Error: "http.BadRequest",
		Value: httpBadRequest{Message: q.fail, Violations: []httpViolation{}},
	})
	return false
}

`}
	// 入力の制約を確かめる（validate = true のときだけ）
	helperValidate = helper{imports: []string{"errors", "net/http", runtimePath, "strings"}, text: `// httpValidate は、読んだ入力 in の制約を確かめる。in が Validate を持つかどうかは、実行時に見る（持たなければ、確かめるものがない）。
// 制約に合わなければ、400（http.BadRequest）と違反の一覧を返して false。op は呼ばない。
// 違反の Path の先頭（入力の model のフィールド名）を、places の通りに通信の上の場所に置き換える。
func httpValidate(w http.ResponseWriter, in any, places ...httpPlace) bool {
	v, has := in.(interface{ Validate() error })
	if !has {
		return true
	}
	err := v.Validate()
	if err == nil {
		return true
	}
	var invalid *runtime.ValidationError
	if !errors.As(err, &invalid) {
		httpWriteError(w, err)
		return false
	}
	violations := make([]httpViolation, 0, len(invalid.Violations))
	for _, x := range invalid.Violations {
		path := x.Path
		for _, p := range places {
			rest, ok := strings.CutPrefix(path, p.field)
			if ok && (rest == "" || rest[0] == '.' || rest[0] == '[') {
				path = p.place + rest
				break
			}
		}
		violations = append(violations, httpViolation{Path: path, Constraint: x.Constraint})
	}
	httpWriteJSON(w, http.StatusBadRequest, httpErrorBody{
		Error: "http.BadRequest",
		Value: httpBadRequest{Message: "invalid input", Violations: violations},
	})
	return false
}

`}
	// 応答を書く
	helperWriteJSON = helper{imports: []string{"encoding/json", "net/http"}, text: `// httpWriteJSON は v を JSON にして status で返す。
func httpWriteJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		status, data = http.StatusInternalServerError, []byte(` + "`" + `{"error":"internal"}` + "`" + `)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

`}
	helperWriteResult = helper{imports: []string{"net/http", "reflect"}, text: `// httpWriteResult は、op が成功したときの応答。何を返すかは、実行時に、out で決める:
// 返す値がない（戻り値のない op は nil を渡す。省略可能な戻り値で、値がない）なら、204 で本文なし。
// あれば、200 と、その JSON。値のない配列（nil）は空の配列 [] に、値のない、インデックスの model（nil のマップ）は空のオブジェクト {} にする。
func httpWriteResult(w http.ResponseWriter, out any) {
	v := reflect.ValueOf(out)
	switch {
	case !v.IsValid(), v.Kind() == reflect.Pointer && v.IsNil():
		w.WriteHeader(http.StatusNoContent)
		return
	case v.Kind() == reflect.Slice && v.IsNil():
		out = reflect.MakeSlice(v.Type(), 0, 0).Interface()
	case v.Kind() == reflect.Map && v.IsNil():
		out = reflect.MakeMap(v.Type()).Interface()
	}
	httpWriteJSON(w, http.StatusOK, out)
}

`}
	// httpWriteError は、コメントだけがここにある（中身は、例外の並びで決まる。render.go の writeError）
	helperWriteErrorComment = `// httpWriteError は op が返したエラーを応答にする。@http.status を持つ例外は、そのステータスと例外の FQN（と運ぶ値）。
// それ以外（ステータスを持たない例外、ctx のエラー、ほかのエラー）は 500 で、中身を出さない。
`
	// 例外の値を応答にする。例外の構造体は、引数ごとにフィールド一つで、json タグは引数の名前（基底が付ける。docs/plugin-go.md
	// 「exception」）なので、運ぶ値は、例外の値そのものの JSON（引数の名前をキーにしたオブジェクト。「通信の形」の「失敗の応答」）。
	// 引数の数も、名前も、型も、ここには書かない: 引数を持つかどうかは、実行時に、構造体のフィールドの数で見る
	helperThrown = helper{imports: []string{"errors", "net/http", "reflect", runtimePath}, text: `// httpThrown は、err が例外 E（値）なら、status と、例外の FQN（と運ぶ値）を返して true。
// 値を運ぶ例外かどうかは、実行時に見る（引数ごとのフィールドを、一つでも持つ）。運ぶ値は、引数の名前をキーにしたオブジェクト。
func httpThrown[E runtime.Exception](w http.ResponseWriter, err error, status int) bool {
	var e E
	if !errors.As(err, &e) {
		return false
	}
	body := httpErrorBody{Error: e.HozoException()}
	if v := reflect.ValueOf(e); v.Kind() == reflect.Struct && v.NumField() > 0 {
		body.Value = e
	}
	httpWriteJSON(w, status, body)
	return true
}

`}
)
