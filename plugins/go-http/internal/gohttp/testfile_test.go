package gohttp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// このファイルは docs/plugin-go-http.md「インテグレーションテストの口」のテスト: テストのパッケージの http.go の中身（必要なケース、
// 要求の組み立て、補助）と、生成した口を、一時のモジュールで動かしたときの振る舞い。ゴールデンとの比較と、コンパイルできることは、
// TestGolden（入口のパッケージの http.go と、同じ入力）。

// caseMethods は、テストのパッケージの http.go の content の、インターフェース name のメソッドの名前（書かれた順）。
func caseMethods(t *testing.T, content, name string) []string {
	t.Helper()
	_, rest, ok := strings.Cut(content, "\ntype "+name+" interface {\n")
	if !ok {
		t.Fatalf("インターフェース %s がない", name)
	}
	body, _, _ := strings.Cut(rest, "\n}\n")
	var names []string
	for _, line := range strings.Split(body, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "//") {
			names = append(names, line[:strings.Index(line, "(")])
		}
	}
	return names
}

// TestHTTPCaseResults は「必要なケース」: 入口ごとの結果は、正常、op を呼ぶ前の失敗（入力が、本文のデコレータか、@http.required の
// 付いたフィールドを持つ op だけ）、op が投げうる例外ごと（Op.signature.exceptions の順）。結果の名前は、その op の中で当たれば、
// 名前空間を前に付ける。まとめの HTTPCases は、入口ごとに、メソッド一つ（宣言順。経路を二つ持つ op も、一つ）。
func TestHTTPCaseResults(t *testing.T) {
	_, routes := emitFixture(t, "routes")
	_, cases := emitFixture(t, "cases")
	for _, tt := range []struct {
		content, name string
		want          string
	}{
		{routes["gen/api/apitest/http.go"], "HTTPCases", "Health Create Search First Replace ListAll Put Reset Ping Count"},
		// 入力がない: 正常だけ
		{routes["gen/api/apitest/http.go"], "HealthHTTPCases", "OK"},
		// 本文の項目と、@http.required。@http.status を持つ例外
		{routes["gen/api/apitest/http.go"], "CreateHTTPCases", "OK BadRequest Conflict Rejected"},
		// @http.required のクエリ。@http.status を持たない例外も、結果
		{routes["gen/api/apitest/http.go"], "SearchHTTPCases", "OK BadRequest Broken Leaky"},
		{routes["gen/api/apitest/http.go"], "FirstHTTPCases", "OK BadRequest NotFound Limited Gone"},
		// 値を返さない op。本文の項目だけ（@http.required はない）でも、読めない本文を送れる
		{routes["gen/api/apitest/http.go"], "ReplaceHTTPCases", "OK BadRequest NotFound"},
		// ヘッダとクエリの全体の形だけ: op を呼ぶ前の失敗は、数えない（全体の形の中の項目は、@http.required を持てない）
		{routes["gen/api/apitest/http.go"], "ListAllHTTPCases", "OK"},
		// 本文の全体の形
		{routes["gen/api/apitest/http.go"], "PutHTTPCases", "OK BadRequest"},
		{routes["gen/api/apitest/http.go"], "PingHTTPCases", "OK"},
		// validate = true でも、@http.required のないクエリだけの入力は、数えない（型と制約は、見ない）
		{routes["gen/strict/stricttest/http.go"], "PlainHTTPCases", "OK"},
		{routes["gen/strict/stricttest/http.go"], "EditHTTPCases", "OK BadRequest"},
		{routes["gen/bare/baretest/http.go"], "TouchHTTPCases", "OK Unknown"},
		// 決まった名前 OK、BadRequest と当たる例外と、二つの名前空間の、同じ名前の例外は、名前空間を前に付ける
		{cases["gen/app/apptest/http.go"], "SubmitHTTPCases", "OK BadRequest AppOK AppBadRequest AppBusy OtherBusy"},
		// never を返す op には、正常のケースがない。当たらない例外は、宣言の名前のまま
		{cases["gen/app/apptest/http.go"], "FailHTTPCases", "Busy"},
	} {
		if got := strings.Join(caseMethods(t, tt.content, tt.name), " "); got != tt.want {
			t.Errorf("%s のメソッド = %s, want %s", tt.name, got, tt.want)
		}
	}
	// 入力のフィールドの名前: op の引数の名前。Given、Request、Then と当たれば、末尾に _
	for _, line := range []string{"\tGiven_ app.In\n", "\tRequest_ app.Q\n", "\tThen_ app.Q\n", "call.body(\"note\", k.Given_.Note)\n"} {
		if !strings.Contains(cases["gen/app/apptest/http.go"], line) {
			t.Errorf("cases の apptest/http.go に、%q がない", line)
		}
	}
}

// TestHTTPCaseLines は「口の形」と「要求の組み立て」: フィールドごとの一行が、デコレータと、その引数（か、フィールドの名前）だけで
// 決まること（@http.required と @http.default は、行を変えない）。結果ごとの確かめの一行。
func TestHTTPCaseLines(t *testing.T) {
	_, files := emitFixture(t, "routes")
	api := files["gen/api/apitest/http.go"]
	for _, line := range []string{
		// 項目の形: 名前を書いたものと、省いたもの。@http.required、@http.default が付いていても、同じ形
		`call.header("X-Tenant", k.In.Tenant)`, `call.header("trace", k.In.Trace)`, `call.header("Accept-Language", k.In.Lang)`,
		`call.query("n", k.In.Name)`, `call.query("tag", k.In.Tag)`, `call.query("dryRun", k.In.DryRun)`,
		`call.body("memo", k.In.Note)`, `call.body("item", k.In.Item)`, `call.body("copies", k.In.Copies)`,
		// 全体の形
		`call.headerObj(k.In.Meta)`, `call.queryObj(k.In.Paging)`, `call.bodyObj(k.In.Item)`,
		// 入力の型は、口の宣言にだけ（別の名前空間の model は、その置き場の入口のパッケージから）
		"\tIn api.CreateInput\n", "\tIn pkgtype.Lookup\n",
		// 経路と、送る行
		`call := httpCall{method: "POST", path: "/items"}`, `call := httpCall{method: "POST", path: "/reset/"}`,
		"return call.send(t, handler, k.Given, k.Request)\n",
		// 経路を二つ持つ op: 結果の下に、経路ごとの段
		"send := func(t *testing.T, method, path string, k PingHTTPCase) *httptest.ResponseRecorder {\n",
		"t.Run(\"GET /ping\", func(t *testing.T) {\n", `res := send(t, "POST", "/ping", k)`,
		// 結果ごとの確かめ: 値を返す op と、返さない op。例外は、型引数と、ステータスだけ
		"var got *pkgtype.Item\n", "httpWantResult(t, res, &got)\n", "httpWantResult(t, res, nil)\n", "httpWantBadRequest(t, res)\n",
		"httpWantThrown[pkgtype.Conflict](t, res, 409)\n", "httpWantThrown[pkgtype.Gone](t, res, 410)\n", "httpWantInternal(t, res)\n",
		"k.Then(t, got, res)\n", "k.Then(t, res)\n",
		// Then の形
		"\tThen func(t *testing.T, got api.Counts, res *httptest.ResponseRecorder)\n", "\tThen func(t *testing.T, res *httptest.ResponseRecorder)\n",
	} {
		if !strings.Contains(api, line) {
			t.Errorf("gen/api/apitest/http.go に、%q がない", line)
		}
	}
	// 走らせる関数の中は、補助の呼び出しだけ: ステータスの名前も、JSON も、項目がないときの扱いも、書かない
	run := api[strings.Index(api, "func RunHTTP("):strings.Index(api, "// httpCall は")]
	for _, banned := range []string{"Status", "json.", "strconv", "httpRequired", "httpDefault", "== nil"} {
		if strings.Contains(run, banned) {
			t.Errorf("gen/api/apitest/http.go の RunHTTP が、%q を書いている（結果ごとの行は、補助の呼び出しだけ）", banned)
		}
	}
	// N の入口のパッケージは、いつも、名前を書いて import する
	if !strings.Contains(api, "\tapi \"example.com/app/gen/api\"\n") {
		t.Error("gen/api/apitest/http.go が、N の入口のパッケージを、名前を書いて import していない")
	}
}

// testHelperNames は、テストのパッケージの http.go の content がパッケージに宣言する、非公開の名前（http で始まる型と関数）と、httpCall の
// メソッド（`.` を前に付けて）を、書かれた順に返す。
func testHelperNames(content string) []string {
	var names []string
	for _, line := range strings.Split(content, "\n") {
		if rest, ok := strings.CutPrefix(line, "func (c *httpCall) "); ok {
			names = append(names, "."+rest[:strings.Index(rest, "(")])
			continue
		}
		for _, prefix := range []string{"func http", "type http"} {
			if strings.HasPrefix(line, prefix) {
				rest := line[strings.Index(line, "http"):]
				names = append(names, rest[:strings.IndexAny(rest, "([ ")])
			}
		}
	}
	return names
}

// TestHTTPCaseHelpers は「テストの補助」: 決まった順で、使うものだけを出す。どれを出すかは、入口の入力のデコレータと、結果の種類だけで決まる。
// 補助のテキストは、どの出力でも同じ。
func TestHTTPCaseHelpers(t *testing.T) {
	_, routes := emitFixture(t, "routes")
	_, cases := emitFixture(t, "cases")
	for _, tt := range []struct {
		where, content string
		want           []string
	}{
		// 六つのデコレータと、四つの結果の種類を、すべて使う
		{"routes の apitest", routes["gen/api/apitest/http.go"], []string{"httpCall", "httpItem", ".header", ".query", ".body", ".headerObj", ".queryObj", ".bodyObj", ".send",
			"httpText", "httpFields", "httpWantResult", "httpWantBadRequest", "httpWantThrown", "httpWantInternal", "httpWantFailure", "httpWantJSON"}},
		// @http.headerObj を使わない。例外がない。全体の形（@http.queryObj）は、項目の形のメソッド（query）で、項目を一つずつ足す
		{"routes の stricttest", routes["gen/strict/stricttest/http.go"], []string{"httpCall", "httpItem", ".header", ".query", ".body", ".queryObj", ".bodyObj", ".send",
			"httpText", "httpFields", "httpWantResult", "httpWantBadRequest", "httpWantFailure", "httpWantJSON"}},
		// 入力がなく、@http.status を持つ例外もない
		{"routes の baretest", routes["gen/bare/baretest/http.go"], []string{"httpCall", "httpItem", ".send", "httpWantResult", "httpWantInternal", "httpWantFailure", "httpWantJSON"}},
		// 入力も、例外もない
		{"cases の wiredtest", cases["gen/wired/wiredtest/http.go"], []string{"httpCall", "httpItem", ".send", "httpWantResult", "httpWantJSON"}},
	} {
		if got := testHelperNames(tt.content); strings.Join(got, " ") != strings.Join(tt.want, " ") {
			t.Errorf("%s の補助 = %v, want %v", tt.where, got, tt.want)
		}
	}
	// 使わない補助のパッケージは、import しない
	for _, banned := range []string{`"strconv"`, `"strings"`, `/runtime"`} {
		if strings.Contains(cases["gen/wired/wiredtest/http.go"], banned) {
			t.Errorf("cases の wiredtest/http.go が、%s を import している", banned)
		}
	}
	// 補助のテキストは、型にも、仕様にも依らない: 同じ補助は、どの出力でも、同じテキスト
	block := func(content, from string) string {
		i := strings.Index(content, from)
		if i < 0 {
			t.Fatalf("%q がない", from)
		}
		end := strings.Index(content[i:], "\n}\n")
		return content[i : i+end]
	}
	_, a := emitFixture(t, "uniforma")
	_, b := emitFixture(t, "uniformb")
	for _, from := range []string{"// send は、", "// httpText は、", "// httpWantResult は、", "// httpWantThrown は、", "// httpWantFailure は、"} {
		want := block(routes["gen/api/apitest/http.go"], from)
		for name, content := range map[string]string{"uniforma": a["gen/app/apptest/http.go"], "uniformb": b["gen/app/apptest/http.go"]} {
			if block(content, from) != want {
				t.Errorf("補助（%s）のテキストが、%s と routes で違う", from, name)
			}
		}
	}
	// 型だけが違う二つの入力のテストのパッケージの http.go は、口の宣言の型（入力の model、結果の型）のほかは、同じ行
	if la, lb := strings.Split(a["gen/app/apptest/http.go"], "\n"), strings.Split(b["gen/app/apptest/http.go"], "\n"); len(la) != len(lb) {
		t.Errorf("uniforma と uniformb の apptest/http.go の行の数が違う: %d と %d", len(la), len(lb))
	} else {
		for i := range la {
			if la[i] != lb[i] && !strings.Contains(la[i], "Then func(t *testing.T, got ") && !strings.Contains(la[i], "var got ") {
				t.Errorf("uniforma と uniformb の apptest/http.go の %d 行目が違う:\n%s\n%s", i+1, la[i], lb[i])
			}
		}
	}
}

// TestHTTPCasesWithoutBaseTests は「置き場と名前」: 基底が、テストのパッケージを作らない出力（葉の op も、判定の口を出す分類もない）でも、
// テストのパッケージに http.go を出す。経路の op が一つもなければ、どちらの http.go も出さない（TestNoRoutes）。
func TestHTTPCasesWithoutBaseTests(t *testing.T) {
	_, files := emitFixture(t, "cases")
	content := files["gen/wired/wiredtest/http.go"]
	if !strings.Contains(content, "\npackage wiredtest\n") || !strings.Contains(content, "func RunHTTP(t *testing.T, handler http.Handler, c HTTPCases) {") {
		t.Errorf("gen/wired/wiredtest/http.go =\n%s", content)
	}
}

// writeRoutes は、入力 routes の出力（依存するトランスパイラの出力と合わせたもの）と、テストのファイル tests を、一時のモジュールに
// 置いて、ルートを返す。
func writeRoutes(t *testing.T, tests ...string) string {
	t.Helper()
	root, files := emitFixture(t, "routes")
	compose(t, root, loadViews(t, "routes"), files)
	for _, name := range tests {
		test, err := os.ReadFile(filepath.Join("testdata", "routes", name))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, name), string(test))
	}
	return root
}

// TestHTTPCases は、生成した口を、一時のモジュールで、同じ入力のハンドラに当てて走らせる（testdata/routes/httpcases_test.go。確かめる
// ことは、そのファイルの頭）: すべてのケースを満たす型を RunHTTP に渡せば、通る。テストの名前（ラベル）は、<op の FQN>/<結果>で、経路を
// 二つ持つ op は、その下に、経路ごとの段（docs/ide-chain.md「テストの規約」）。
func TestHTTPCases(t *testing.T) {
	t.Parallel()
	root := writeRoutes(t, "behavior_test.go", "httpcases_test.go")
	out, err := runGo(t, root, "test", "-count=1", "-v", "-run", "^(TestHTTPCases|TestStrictHTTPCases|TestBareHTTPCases)$", ".")
	if err != nil {
		t.Fatalf("口を満たす型のテストが落ちた: %v\n%s", err, out)
	}
	for _, name := range []string{
		"TestHTTPCases/api.health/OK", "TestHTTPCases/api.create/OK", "TestHTTPCases/api.create/BadRequest", "TestHTTPCases/api.create/Conflict",
		"TestHTTPCases/api.create/Rejected", "TestHTTPCases/api.search/OK", "TestHTTPCases/api.search/BadRequest", "TestHTTPCases/api.search/Broken",
		"TestHTTPCases/api.search/Leaky", "TestHTTPCases/api.first/OK", "TestHTTPCases/api.first/BadRequest", "TestHTTPCases/api.first/NotFound",
		"TestHTTPCases/api.first/Limited", "TestHTTPCases/api.first/Gone", "TestHTTPCases/api.replace/OK", "TestHTTPCases/api.replace/BadRequest",
		"TestHTTPCases/api.replace/NotFound", "TestHTTPCases/api.listAll/OK", "TestHTTPCases/api.put/OK", "TestHTTPCases/api.put/BadRequest",
		"TestHTTPCases/api.reset/OK", "TestHTTPCases/api.count/OK",
		// 経路を二つ持つ op（go test は、名前の空白を _ にする）
		"TestHTTPCases/api.ping/OK/GET_/ping", "TestHTTPCases/api.ping/OK/POST_/ping",
		"TestStrictHTTPCases/strict.order/OK", "TestStrictHTTPCases/strict.order/BadRequest", "TestStrictHTTPCases/strict.edit/BadRequest",
		"TestStrictHTTPCases/strict.bulk/BadRequest", "TestStrictHTTPCases/strict.plain/OK", "TestStrictHTTPCases/strict.now/OK",
		"TestBareHTTPCases/bare.root/OK", "TestBareHTTPCases/bare.touch/OK", "TestBareHTTPCases/bare.touch/Unknown", "TestBareHTTPCases/bare.viaAlias/OK",
	} {
		if !strings.Contains(out, "--- PASS: "+name+" ") {
			t.Errorf("テスト %s が、通っていない:\n%s", name, out)
		}
	}
}

// TestHTTPMismatch は、走らせる側の、結果の見分け（「必要なケース」の表の「走らせる側が確かめること」）: 応答が、そのケースのものでなければ、
// そのテストは落ち、Then は呼ばれない（testdata/routes/httpmismatch_test.go）。差し替えていないケースは、通る。
func TestHTTPMismatch(t *testing.T) {
	t.Parallel()
	root := writeRoutes(t, "behavior_test.go", "httpcases_test.go", "httpmismatch_test.go")
	out, err := runGo(t, root, "test", "-count=1", "-v", "-run", "^TestHTTPMismatch$", ".")
	if err == nil {
		t.Fatalf("応答が、ケースのものでないのに、テストが通った:\n%s", out)
	}
	for name, message := range map[string]string{
		"api.health/OK":        "want status 200, got 404",
		"api.first/BadRequest": "want status 400, got 204",
		"api.first/NotFound":   "want status 404, got 410",
		"api.first/Limited":    "want status 429, got 204",
		"api.first/Gone":       "want status 410, got 500",
		"api.replace/OK":       "want status 204, got 404",
		"api.search/Broken":    "want status 500, got 200",
		"api.search/Leaky":     "given: context canceled",
	} {
		if !strings.Contains(out, "--- FAIL: TestHTTPMismatch/"+name+" ") {
			t.Errorf("テスト %s が、落ちていない:\n%s", name, out)
		}
		if !strings.Contains(out, message) {
			t.Errorf("テスト %s の文面 %q がない:\n%s", name, message, out)
		}
	}
	if strings.Contains(out, "Then was called") {
		t.Errorf("結果の見分けに落ちたケースの Then が、呼ばれた:\n%s", out)
	}
	for _, name := range []string{"api.create/OK", "api.first/OK", "api.search/OK", "api.replace/NotFound", "api.ping/OK/GET_/ping"} {
		if !strings.Contains(out, "--- PASS: TestHTTPMismatch/"+name+" ") {
			t.Errorf("差し替えていないテスト %s が、通っていない:\n%s", name, out)
		}
	}
	if n := strings.Count(out, "\n        --- FAIL: "); n != 8 {
		t.Errorf("落ちた、結果のテストが %d 個（8 のはず）:\n%s", n, out)
	}
}

// httpCaseMethods は、口を満たす型のメソッド（<型>.<メソッド> → 宣言）。TestIncompleteHTTPCasesDoNotCompile が、一つずつ欠く。
var httpCaseMethods = map[string]string{
	// baretest.HTTPCases: 入口ごと
	"bareCases.Root":     "func (bareCases) Root() baretest.RootHTTPCases { return nil }",
	"bareCases.Touch":    "func (bareCases) Touch() baretest.TouchHTTPCases { return touchCases{} }",
	"bareCases.ViaAlias": "func (bareCases) ViaAlias() baretest.ViaAliasHTTPCases { return nil }",
	// baretest.TouchHTTPCases: 正常と、@http.status を持たない例外
	"touchCases.OK":      "func (touchCases) OK() baretest.TouchHTTPCase { return baretest.TouchHTTPCase{} }",
	"touchCases.Unknown": "func (touchCases) Unknown() baretest.TouchHTTPCase { return baretest.TouchHTTPCase{} }",
	// apitest.FirstHTTPCases: 正常、op を呼ぶ前の失敗、@http.status を持つ例外
	"firstCases.OK":         "func (firstCases) OK() apitest.FirstHTTPCase { return apitest.FirstHTTPCase{} }",
	"firstCases.BadRequest": "func (firstCases) BadRequest() apitest.FirstHTTPCase { return apitest.FirstHTTPCase{} }",
	"firstCases.NotFound":   "func (firstCases) NotFound() apitest.FirstHTTPCase { return apitest.FirstHTTPCase{} }",
	"firstCases.Limited":    "func (firstCases) Limited() apitest.FirstHTTPCase { return apitest.FirstHTTPCase{} }",
	"firstCases.Gone":       "func (firstCases) Gone() apitest.FirstHTTPCase { return apitest.FirstHTTPCase{} }",
}

// httpCasesImpl は、口を満たす型を持つパッケージのテキスト。omit の宣言を、書かない。
func httpCasesImpl(omit string) string {
	var b strings.Builder
	b.WriteString(`package impl

import (
	"testing"

	"example.com/app/gen/api/apitest"
	"example.com/app/gen/bare/baretest"
)

type (
	bareCases  struct{}
	touchCases struct{}
	firstCases struct{}
)

// Run は、口を満たす型を、走らせる関数に渡す。口のメソッドが、一つでも足りなければ、ここで、コンパイルできない。
func Run(t *testing.T) {
	baretest.RunHTTP(t, nil, bareCases{})
	var _ apitest.FirstHTTPCases = firstCases{}
}

`)
	for name, decl := range httpCaseMethods {
		if name != omit {
			b.WriteString(decl + "\n")
		}
	}
	return b.String()
}

// TestIncompleteHTTPCasesDoNotCompile は、インテグレーションテストのケースの漏れが、コンパイルエラーになること（「口の形」「必要なケース」。
// CLAUDE.md「実装の漏れは、コンパイルエラーにする」）: 入口を一つ欠いた HTTPCases、結果（正常、op を呼ぶ前の失敗、例外）を一つ欠いた
// <Op>HTTPCases は、走らせる関数に渡すところで、コンパイルできない。
func TestIncompleteHTTPCasesDoNotCompile(t *testing.T) {
	t.Parallel()
	root := writeRoutes(t)
	impl := filepath.Join(root, "impl", "impl.go")

	// すべてのメソッドを持つ型は、コンパイルできる（下の失敗が、ほかの理由でないこと）
	writeFile(t, impl, httpCasesImpl(""))
	if out, err := runGo(t, root, "vet", "./impl"); err != nil {
		t.Fatalf("すべてのケースを持つ実装が、コンパイルできない: %v\n%s", err, out)
	}
	for _, tt := range []struct{ omit, what string }{
		{"bareCases.Touch", "入口 touch のケース"},
		{"bareCases.ViaAlias", "入口 viaAlias のケース"},
		{"touchCases.OK", "入口 touch の、正常のケース"},
		{"touchCases.Unknown", "入口 touch の、例外 bare.Unknown のケース"},
		{"firstCases.BadRequest", "入口 first の、op を呼ぶ前の失敗のケース"},
		{"firstCases.NotFound", "入口 first の、例外 type.NotFound のケース"},
		{"firstCases.Gone", "入口 first の、例外 type.Gone のケース"},
	} {
		method := tt.omit[strings.IndexByte(tt.omit, '.')+1:]
		writeFile(t, impl, httpCasesImpl(tt.omit))
		out, err := runGo(t, root, "vet", "./impl")
		if err == nil {
			t.Errorf("%sを持たない実装が、コンパイルできた", tt.what)
			continue
		}
		if !strings.Contains(out, "missing method "+method) {
			t.Errorf("%sを持たない実装のコンパイルエラーが、足りないメソッド %s を示さない:\n%s", tt.what, method, out)
		}
	}
}

// TestHTTPCaseDir は GOHTTP-21 の条件: 基底も、テストのパッケージを作る出力（葉の op がある）では、基底が GO-4 を返すので、重ねて返さない。
// 置き場が要るので、go.mod がないときも、返さない（基底が GO-1 を返す）。
func TestHTTPCaseDir(t *testing.T) {
	views := loadViews(t, "gohttp21")
	_, diags, _ := emitViews(t, newRoot(t), views)
	if len(diags) != 1 || diags[0].Code != codeHTTPTestDir || diags[0].At != "n.first" {
		t.Errorf("診断 = %+v, want n.first の GOHTTP-21 が一つ（m は、基底が GO-4 を返す）", diags)
	}
	if _, diags, _ = emitViews(t, t.TempDir(), views); len(diags) != 0 {
		t.Errorf("go.mod がないとき: 診断 = %+v, want なし", diags)
	}
}
