package gohttp

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// goldenFixtures は、出力をゴールデンと比べる入力（testdata/<名前>/）と、それが確かめる docs/plugin-go-http.md の見出し。
// behavior は、生成したコードを動かすテスト testdata/<名前>/behavior_test.go を持つもの。
var goldenFixtures = []struct {
	name     string
	covers   string
	behavior bool
}{
	{name: "routes", covers: "「出力」のハンドラの中の 1〜5、「入力の読み方」の二つの表の各行（三つの場所の二つの形、名前を書いた項目と省いた項目、項目がないときの三つの扱い）、「補助」（使うものだけ）、「通信の形」、「制約の検査」", behavior: true},
	{name: "locals", covers: "名前空間のパッケージの名前が、NewHandler とハンドラの中の局所の名前や、標準ライブラリのパッケージの名前と当たるとき（「出力」。docs/plugin-go.md「パッケージと import パス」の import の名前）"},
	{name: "uniforma", covers: "型に依らない形の確かめ（docs/testing.md「単体テスト」）の一つ目: 核の型、省略可能でないフィールド、引数のない例外、戻り値のある op"},
	{name: "uniformb", covers: "同、二つ目: scalar、省略可能なフィールド、引数を持つ例外（一つと、二つ以上。省略可能な引数）、省略可能な戻り値の op。uniforma と同じテキストになる（TestUniform）"},
	{name: "unions", covers: "「通信の形」の「union」（印を書かない。決め手のキーで、アームを決める。包む型は、その Value）と、「補助」の「union の読み書き」（表 httpUnions と、読み書きの補助を、二つの http.go に）", behavior: true},
	{name: "cases", covers: "「インテグレーションテストの口」のうち、routes が使わない場合: 結果の名前が当たるとき、never を返す op、Given・Request・Then と当たる引数の名前、基底が、テストのパッケージを作らない出力"},
}

// TestGolden は、入力ごとに、出力がゴールデン testdata/<名前>/expected/ とバイト単位で同じこと、go/format を通した形で
// あること、同じ View から同じバイト列を返すこと、依存するトランスパイラ（基底 go と go-valid）の出力と合わせてコンパイルできる
// （一時のモジュールで go vet を通る）ことを確かめる。
func TestGolden(t *testing.T) {
	for _, fx := range goldenFixtures {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			views := loadViews(t, fx.name)
			root, files := emitFixture(t, fx.name)
			checkFormatted(t, files)
			compareTree(t, fx.name, files)
			// 決定性: もう一度、別のルート（同じモジュールのパス）で出力しても、同じバイト列
			again, _, _ := emitViews(t, newRoot(t), views)
			if len(again) != len(files) {
				t.Errorf("二度目の出力のファイルの数が違う: %d と %d", len(files), len(again))
			}
			for p, content := range files {
				if again[p] != content {
					t.Errorf("%s が、二度目の出力で変わった", p)
				}
			}
			vet(t, root, views, files)
		})
	}
}

// TestBehavior は、生成したコードを一時のモジュールで実際に動かす（docs/testing.md「単体テスト」）: 入力の behavior_test.go を、
// 依存するトランスパイラの出力と go-http の出力を書いたモジュールのルートに置いて、go test を走らせる。確かめることは、
// それぞれのファイルの頭。
func TestBehavior(t *testing.T) {
	for _, fx := range goldenFixtures {
		if !fx.behavior {
			continue
		}
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			root, files := emitFixture(t, fx.name)
			compose(t, root, loadViews(t, fx.name), files)
			test, err := os.ReadFile(filepath.Join("testdata", fx.name, "behavior_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, "behavior_test.go"), string(test))
			out, err := runGo(t, root, "test", "-count=1", "-v", ".")
			if err != nil {
				t.Fatalf("生成したコードのテストが落ちた: %v\n%s", err, out)
			}
			if !strings.Contains(out, "--- PASS: Test") {
				t.Fatalf("生成したコードのテストが一つも走らなかった:\n%s", out)
			}
		})
	}
}

// TestPlacement は「出力」と「インテグレーションテストの口」の置き場と、公開の名前: http.go は、N の入口のパッケージに一つ（内部の op を
// 持つ名前空間でも、内側のパッケージではない）と、N のテストのパッケージに一つ。入口のパッケージの公開の名前は NewHandler 一つ。
// テストのパッケージの公開の名前は、<Op>HTTPCases、<Op>HTTPCase、HTTPCases、RunHTTP。非公開の名前は、どちらも http で始まる。
func TestPlacement(t *testing.T) {
	_, files := emitFixture(t, "routes")
	want := map[string]string{
		"gen/api/http.go":               "package api\n",
		"gen/strict/http.go":            "package strict\n",
		"gen/bare/http.go":              "package bare\n",
		"gen/flow/http.go":              "package flow\n",
		"gen/flow/flowtest/http.go":     "package flowtest\n",
		"gen/api/apitest/http.go":       "package apitest\n",
		"gen/strict/stricttest/http.go": "package stricttest\n",
		"gen/bare/baretest/http.go":     "package baretest\n",
	}
	// テストのパッケージの http.go ごとに、同じディレクトリに、テストの対応表が一つ
	maps := []string{"gen/api/apitest/hozo-tests.go-http.json", "gen/strict/stricttest/hozo-tests.go-http.json", "gen/bare/baretest/hozo-tests.go-http.json",
		"gen/flow/flowtest/hozo-tests.go-http.json"}
	for _, p := range maps {
		if files[p] == "" {
			t.Errorf("テストの対応表 %s を出していない", p)
		}
	}
	if len(files) != len(want)+len(maps) {
		t.Errorf("ファイル = %v, want %d 個", sortedPaths(files), len(want)+len(maps))
	}
	for p, pkg := range want {
		if !strings.Contains(files[p], "\n"+pkg) {
			t.Errorf("%s が %q を持たない", p, strings.TrimSpace(pkg))
		}
		inTest := strings.HasSuffix(pkg, "test\n")
		for _, line := range strings.Split(files[p], "\n") {
			var name string
			switch {
			case strings.HasPrefix(line, "func ("):
				continue // メソッド（パッケージに名前を足さない）
			case strings.HasPrefix(line, "func "):
				name = strings.TrimPrefix(line, "func ")
			case strings.HasPrefix(line, "type "):
				name = strings.TrimPrefix(line, "type ")
			case strings.HasPrefix(line, "var "):
				name = strings.TrimPrefix(line, "var ")
			case strings.HasPrefix(line, "const "):
				name = strings.TrimPrefix(line, "const ")
			default:
				continue
			}
			name = name[:strings.IndexAny(name, "([ ")]
			public := name == handlerName
			if inTest {
				public = name == casesName || name == runName || strings.HasSuffix(name, "HTTPCases") || strings.HasSuffix(name, "HTTPCase")
			}
			if !public && !strings.HasPrefix(name, "http") {
				t.Errorf("%s: パッケージに足す名前 %s が、決まった公開の名前でも、http で始まる名前でもない", p, name)
			}
		}
	}
}

// declaredHelpers は、http.go の content がパッケージに宣言する、非公開の名前（http で始まる型、変数、関数）と、httpRequest の
// メソッド（`.` を前に付けて）を、書かれた順に返す。
func declaredHelpers(content string) []string {
	var names []string
	for _, line := range strings.Split(content, "\n") {
		if rest, ok := strings.CutPrefix(line, "func (q *httpRequest) "); ok {
			names = append(names, "."+rest[:strings.Index(rest, "(")])
			continue
		}
		for _, prefix := range []string{"func http", "type http", "var http"} {
			if strings.HasPrefix(line, prefix) {
				rest := line[strings.Index(line, "http"):]
				names = append(names, rest[:strings.IndexAny(rest, "([ ")])
			}
		}
	}
	return names
}

// TestHelpers は「補助」: 使うものだけを、決まった順で出す。どれを出すかは、経路の入力のデコレータと、単位のオプションだけで決まる。
func TestHelpers(t *testing.T) {
	_, files := emitFixture(t, "routes")
	for p, want := range map[string][]string{
		// validate = false: 制約の検査の補助（httpPlace、httpValidate）はない。六つのデコレータを、すべて使う
		"gen/api/http.go": {"httpErrorBody", "httpBadRequest", "httpViolation", "httpOption", "httpRequired", "httpDefault", "httpRequest",
			".header", ".query", ".body", ".headerObj", ".queryObj", ".bodyObj", ".missing", ".text", ".ok",
			"httpWriteJSON", "httpWriteResult", "httpWriteError", "httpThrown"},
		// validate = true。@http.headerObj を使わない。@http.status を持つ例外がないので、httpThrown はない。
		// 全体の形（@http.queryObj）は、項目の形のメソッド（query）で、項目を一つずつ読む
		"gen/strict/http.go": {"httpErrorBody", "httpBadRequest", "httpViolation", "httpPlace", "httpOption", "httpRequired", "httpDefault", "httpRequest",
			".header", ".query", ".body", ".queryObj", ".bodyObj", ".missing", ".text", ".ok",
			"httpValidate", "httpWriteJSON", "httpWriteResult", "httpWriteError"},
		// 入力がなく、@http.status を持つ例外もない: 失敗の本文と、応答を書く補助だけ（validate = true でも、確かめる入力がない）
		"gen/bare/http.go": {"httpErrorBody", "httpWriteJSON", "httpWriteResult", "httpWriteError"},
	} {
		got := declaredHelpers(files[p])
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s の補助 = %v, want %v", p, got, want)
		}
	}
	if strings.Contains(files["gen/bare/http.go"], "switch {\n\tcase httpThrown") || strings.Contains(files["gen/bare/http.go"], `"errors"`) {
		t.Error("gen/bare/http.go: @http.status を持つ例外がないのに、例外を見分ける switch を書いた")
	}
	// 本文の項目（@http.body）だけを使う出力: ヘッダとクエリの値を読む text はなく、missing と、三つの httpOption はある。
	// 本文の全体（@http.bodyObj）だけなら、項目がないときの扱いは、どれも出さない
	_, files = emitFixture(t, "locals")
	if got := declaredHelpers(files["gen/api/http.go"]); !contains(got, ".missing") || !contains(got, "httpOption") {
		t.Errorf("locals の補助 = %v", got)
	}
	var view *ir.View
	for _, v := range loadViews(t, "locals") {
		if v.Viewer == "api" {
			view = v
		}
	}
	// only は、view の op のうち、keep だけを残した View の出力の補助。
	only := func(keep string) []string {
		v, p := *view, *view.Program
		p.Namespaces = append([]ir.Namespace(nil), view.Program.Namespaces...)
		for i := range p.Namespaces {
			if p.Namespaces[i].Name != view.Viewer {
				continue
			}
			var decls []ir.Decl
			for _, d := range p.Namespaces[i].Decls {
				if op, ok := d.(*ir.Op); !ok || op.Name == keep {
					decls = append(decls, d)
				}
			}
			p.Namespaces[i].Decls = decls
		}
		v.Program = &p
		files, diags, _ := emitViews(t, newRoot(t), []*ir.View{&v})
		if len(diags) > 0 {
			t.Fatalf("%s だけの出力に、診断 %+v", keep, diags)
		}
		return declaredHelpers(files["gen/api/http.go"])
	}
	for keep, want := range map[string][]string{
		"api.viaIn": {"httpErrorBody", "httpBadRequest", "httpViolation", "httpPlace", "httpOption", "httpRequired", "httpDefault", "httpRequest",
			".body", ".missing", ".ok", "httpValidate", "httpWriteJSON", "httpWriteResult", "httpWriteError", "httpThrown"},
		"api.viaReflect": {"httpErrorBody", "httpBadRequest", "httpViolation", "httpPlace", "httpRequest",
			".bodyObj", ".ok", "httpValidate", "httpWriteJSON", "httpWriteResult", "httpWriteError"},
		"api.viaOps": {"httpErrorBody", "httpBadRequest", "httpViolation", "httpPlace", "httpOption", "httpRequired", "httpDefault", "httpRequest",
			".header", ".headerObj", ".missing", ".text", ".ok", "httpValidate", "httpWriteJSON", "httpWriteResult", "httpWriteError"},
	} {
		if got := only(keep); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s だけの出力の補助 = %v, want %v", keep, got, want)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TestUniform は、型に依らない形の確かめ（docs/testing.md「単体テスト」、docs/requirements.md「出力を推論しない」）: 入力 uniforma と
// uniformb は、同じ名前のフィールドと op に、同じデコレータを書き、型だけを変えてある（核の型と scalar、省略可能とそうでないもの、
// 引数を持つ例外（一つと、二つ以上。省略可能な引数）と持たない例外、戻り値のある op と省略可能な戻り値の op、配列と record の結果）。
// 二つの http.go は、フィールドごと・op ごと・例外ごとの行だけでなく、ファイルの全体が、同じテキストになる。違いが実行時に出ることは、
// behavior_test.go が確かめる。
func TestUniform(t *testing.T) {
	_, a := emitFixture(t, "uniforma")
	_, b := emitFixture(t, "uniformb")
	const testMap = "gen/app/apptest/hozo-tests.go-http.json"
	if len(a) != 3 || a["gen/app/http.go"] == "" || a["gen/app/apptest/http.go"] == "" || a[testMap] == "" {
		t.Fatalf("uniforma のファイル = %v", sortedPaths(a))
	}
	if a[testMap] != b[testMap] {
		t.Errorf("型だけが違う二つの入力の、テストの対応表が、違うテキストになった:\n%s", firstDiff(a[testMap], b[testMap]))
	}
	if a["gen/app/http.go"] != b["gen/app/http.go"] {
		t.Errorf("型だけが違う二つの入力の http.go が、違うテキストになった:\n%s", firstDiff(a["gen/app/http.go"], b["gen/app/http.go"]))
	}
	// 二つの入力が、確かめたい違いを持っていること: 同じ例外が、片方では引数を持たず、もう片方では持つ（一つと、二つ以上。省略可能な引数）
	params := func(name, fqn string) []ir.OpParam {
		x, ok := loadViews(t, name)[0].Program.Lookup(fqn).(*ir.Exception)
		if !ok {
			t.Fatalf("%s に、例外 %s がない", name, fqn)
		}
		return x.Params
	}
	for _, fqn := range []string{"app.Missing", "app.Clash"} {
		if n := len(params("uniforma", fqn)); n != 0 {
			t.Errorf("uniforma の例外 %s が、引数を %d 個持つ（持たないはず）", fqn, n)
		}
	}
	if n := len(params("uniformb", "app.Missing")); n != 1 {
		t.Errorf("uniformb の例外 app.Missing の引数が %d 個（一つのはず）", n)
	}
	clash := params("uniformb", "app.Clash")
	if len(clash) < 2 {
		t.Fatalf("uniformb の例外 app.Clash の引数が %d 個（二つ以上のはず）", len(clash))
	}
	if _, optional := clash[len(clash)-1].Type.(ir.OptionalType); !optional {
		t.Errorf("uniformb の例外 app.Clash の最後の引数 %s が、省略可能でない", clash[len(clash)-1].Name)
	}
	// 比べたものが、確かめたい行を持っていること
	for _, line := range []string{
		`req.header("Authorization", &in.Token)`, `req.header("trace", &in.Trace)`, `req.query("page", &in.Page)`,
		`req.query("q", &in.Keyword)`, `req.query("size", &in.Size, httpDefault(10))`, `req.query("owner", &in.Owner, httpRequired)`,
		`req.header("lang", &in.Lang, httpDefault("en"))`, `req.body("item", &in.Item)`, `req.body("memo", &in.Note)`,
		`req.body("count", &in.Count, httpRequired)`, `req.headerObj(&in.Meta)`, `req.queryObj(&in.Paging)`, `req.bodyObj(&in.Item)`,
		`httpPlace{"keyword", "query.q"}`, `httpPlace{"meta", "header"}`, `httpPlace{"item", "body"}`,
		`out, err := ops.Find(r.Context(), in)`, `httpWriteResult(w, out)`,
		`case httpThrown[Missing](w, err, 404):`, `case httpThrown[Clash](w, err, 409):`,
	} {
		if !strings.Contains(a["gen/app/http.go"], line) {
			t.Errorf("uniforma の http.go に、行 %s がない", line)
		}
	}
}

// TestThrown は「補助」の httpWriteError と httpThrown、「通信の形」の「失敗の応答」: 例外ごとの行は、@http.status を持つ例外を、最初に
// 現れた順（op の宣言順、その中は exceptions の順）に、例外の型とステータスだけで書く。引数の数も、名前も、型も、行に現れない。
// 例外の値を応答にするところ（補助 httpThrown）は、どの出力でも同じテキストで、例外の値そのものを JSON にする（引数の名前を
// キーにしたオブジェクト。フィールドを名指ししない）。@http.status を持たない例外は、引数を持っていても、行を持たない。
func TestThrown(t *testing.T) {
	_, files := emitFixture(t, "routes")
	api := files["gen/api/http.go"]
	start, end := strings.Index(api, "func httpWriteError("), strings.Index(api, "// httpThrown は")
	if start < 0 || end < start {
		t.Fatal("gen/api/http.go に、httpWriteError と httpThrown がない")
	}
	want := `func httpWriteError(w http.ResponseWriter, err error) {
	switch {
	case httpThrown[pkgtype.Conflict](w, err, 409):
	case httpThrown[pkgtype.Rejected](w, err, 422):
	case httpThrown[pkgtype.NotFound](w, err, 404):
	case httpThrown[pkgtype.Limited](w, err, 429):
	case httpThrown[pkgtype.Gone](w, err, 410):
	default:
		httpWriteJSON(w, http.StatusInternalServerError, httpErrorBody{Error: "internal"})
	}
}

`
	if got := api[start:end]; got != want {
		t.Errorf("httpWriteError =\n%s\nwant\n%s", got, want)
	}
	// 補助 httpThrown は、型にも、仕様にも依らない: 引数を持つ例外だけの出力でも、持たない例外だけの出力でも、同じテキスト
	thrown := func(content string) string {
		i := strings.Index(content, "// httpThrown は")
		if i < 0 {
			t.Fatal("httpThrown がない")
		}
		return content[i:]
	}
	_, a := emitFixture(t, "uniforma")
	_, b := emitFixture(t, "uniformb")
	if thrown(api) != thrown(a["gen/app/http.go"]) || thrown(api) != thrown(b["gen/app/http.go"]) {
		t.Error("補助 httpThrown のテキストが、出力によって違う")
	}
	for _, line := range []string{"func httpThrown[E runtime.Exception](w http.ResponseWriter, err error, status int) bool {", "body.Value = e\n"} {
		if !strings.Contains(thrown(api), line) {
			t.Errorf("httpThrown に、%q がない", line)
		}
	}
	// 例外の構造体のフィールドを、名前で引かない（値を一つに包むフィールド Value は、もうない。docs/plugin-go.md「exception」）
	for _, banned := range []string{"FieldByName", `"Value"`, ".Field("} {
		if strings.Contains(thrown(api), banned) {
			t.Errorf("httpThrown が、%q を書いている（例外の値は、そのまま JSON にする）", banned)
		}
	}
}

// TestLines は「入力の読み方」の二つの表: フィールドごとの一行が、デコレータと、その引数（か、フィールドの名前）だけで決まること。
func TestLines(t *testing.T) {
	_, files := emitFixture(t, "routes")
	api := files["gen/api/http.go"]
	for _, line := range []string{
		// 項目の形: 名前を書いたものと、省いたもの（フィールドの名前）
		`req.header("X-Tenant", &in.Tenant, httpRequired)`, `req.header("Authorization", &in.Token)`, `req.header("trace", &in.Trace)`,
		`req.query("n", &in.Name)`, `req.query("tag", &in.Tag)`, `req.body("memo", &in.Note)`, `req.body("item", &in.Item)`,
		// 全体の形
		`req.headerObj(&in.Meta)`, `req.queryObj(&in.Paging)`, `req.bodyObj(&in.Item)`,
		// 項目がないときの扱い: 最後の引数。リテラルは、基底と同じ書き方（型への変換は書かない）
		`req.query("dryRun", &in.DryRun, httpRequired)`, `req.body("labels", &in.Labels, httpRequired)`,
		`req.header("Accept-Language", &in.Lang, httpDefault("en"))`, `req.body("copies", &in.Copies, httpDefault(1))`,
		`req.body("weight", &in.Weight, httpDefault(2))`, `req.body("scale", &in.Scale, httpDefault(1.5))`,
		`req.body("active", &in.Active, httpDefault(true))`, `req.body("timeout", &in.Timeout, httpDefault(time.Duration(30000000000)))`,
		`req.body("wait", &in.Wait, httpDefault(time.Duration(300000000000)))`, `req.query("ratio", &in.Ratio, httpDefault(0.5))`,
		`req.query("factor", &in.Factor, httpDefault(3))`, `req.query("owner", &in.Owner, httpDefault("nobody"))`,
		// 入力を置く。別の名前空間の model は、その置き場の入口のパッケージから
		"var in CreateInput\n", "var in pkgtype.Lookup\n",
		// op を呼ぶ行の二つの形と、成功を返す行
		"out, err := ops.Health(r.Context())\n", "out, err := ops.Create(r.Context(), in)\n",
		"if err := ops.Replace(r.Context(), in); err != nil {\n", "if err := ops.Reset(r.Context()); err != nil {\n",
		"httpWriteResult(w, out)\n", "httpWriteResult(w, nil)\n",
		// / で終わるパスは、そのパスだけに合わせる
		`mux.HandleFunc("POST /reset/{$}", `,
	} {
		if !strings.Contains(api, line) {
			t.Errorf("gen/api/http.go に、%q がない", line)
		}
	}
	// ハンドラは、ステータスも、型も書かない（補助の中だけ）
	handlers := api[strings.Index(api, "func NewHandler("):strings.Index(api, "// httpErrorBody は")]
	for _, banned := range []string{"Status", "WriteHeader", "strconv", "json.", "int64", "Validate", "== nil"} {
		if strings.Contains(handlers, banned) {
			t.Errorf("gen/api/http.go の NewHandler が、%q を書いている（経路ごとの行は、補助の呼び出しだけ）", banned)
		}
	}
	// 制約の検査: 入力を持つすべての経路に、同じ形で。場所の一覧は、すべてのフィールドを、フィールドの順に
	strict := files["gen/strict/http.go"]
	for _, line := range []string{
		`if !httpValidate(w, in, httpPlace{"token", "header.Authorization"}, httpPlace{"lines", "body.lines"}) {`,
		`if !httpValidate(w, in, httpPlace{"page", "query.page"}, httpPlace{"name", "header.X-Name"}, httpPlace{"item", "body.item"}, httpPlace{"extra", "body.more"}) {`,
		`if !httpValidate(w, in, httpPlace{"payload", "body"}, httpPlace{"paging", "query"}) {`,
		// Validate を持たない入力にも、同じ行
		`if !httpValidate(w, in, httpPlace{"id", "query.id"}) {`,
	} {
		if !strings.Contains(strict, line) {
			t.Errorf("gen/strict/http.go に、%q がない", line)
		}
	}
	if n := strings.Count(strict[:strings.Index(strict, "// httpErrorBody は")], "httpValidate("); n != 4 {
		t.Errorf("gen/strict/http.go の httpValidate の呼び出しが %d 個（入力を持つ経路は四つ。入力のない経路には出さない）", n)
	}
	if strings.Contains(api, "httpValidate") || strings.Contains(api, "httpPlace") {
		t.Error("gen/api/http.go（validate = false）が、制約の検査を出した")
	}
}

// TestNoRoutes は「対象」: 経路の op が一つもなければ、ファイルを出さず warning を一つ返す。
func TestNoRoutes(t *testing.T) {
	root := newRoot(t)
	files, diags, warnings := emitViews(t, root, loadViews(t, "noroutes"))
	if len(files) != 0 || len(diags) != 0 || len(warnings) != 1 {
		t.Fatalf("files=%v 診断=%+v warnings=%+v, want warning が一つだけ", sortedPaths(files), diags, warnings)
	}
	if warnings[0].At != "app" || !strings.Contains(warnings[0].Message, "@http.get") {
		t.Errorf("warning = %+v", warnings[0])
	}
	resp, err := Handler{}.Emit(request(loadViews(t, "noroutes")[0], TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Warnings) != 1 || len(resp.Errors) != 0 {
		t.Errorf("応答 = %+v, want warning が一つだけ", resp)
	}
}

// TestUnitTargets は「対象」: 経路の op は、単位の対象の op（View.files にあるファイルの op。docs/config.md「in」）だけ。
// 単位の対象でない op は、@http.get が付いていても、経路にならない。
func TestUnitTargets(t *testing.T) {
	var bare *ir.View
	for _, v := range loadViews(t, "routes") {
		if v.Viewer == "bare" {
			bare = v
		}
	}
	root := newRoot(t)
	files, diags, warnings := emitViews(t, root, []*ir.View{bare})
	if len(files) != 3 || files["gen/bare/http.go"] == "" || files["gen/bare/baretest/http.go"] == "" || files["gen/bare/baretest/hozo-tests.go-http.json"] == "" || len(diags) != 0 || len(warnings) != 0 {
		t.Fatalf("files=%v 診断=%+v warnings=%+v, want 二つの http.go と、テストの対応表", sortedPaths(files), diags, warnings)
	}
	outside := *bare
	outside.Files = []string{"elsewhere.hozo"}
	files, diags, warnings = emitViews(t, root, []*ir.View{&outside})
	if len(files) != 0 || len(diags) != 0 || len(warnings) != 1 {
		t.Errorf("単位の対象でない op だけ: files=%v 診断=%+v warnings=%+v, want warning が一つだけ", sortedPaths(files), diags, warnings)
	}
}

// diagnosticFixtures は、診断を返す入力（testdata/<名前>/）。期待する診断は testdata/<名前>/expected/errors.txt
// （一行に、コード、位置、メッセージをタブで区切って。View の順、その中は返した順）。
var diagnosticFixtures = []struct{ name, code string }{
	{"gohttp9", codeHTTPOption},    // [emit.options.go-http] に、知らないキーか、型の違う値がある
	{"gohttp10", codeHTTPName},     // NewHandler が、N の定義のパッケージの名前と当たる
	{"gohttp11", codeHTTPValidate}, // validate = true で、Validate を持つ入力の model の置き場の単位が、go-valid を並べていない
	{"gohttp16", codeHTTPResult},   // 経路の op の結果の型が、省略可能な配列か、省略可能な record
	{"gohttp20", codeHTTPTestName}, // テストのパッケージに足す公開の名前が、基底の公開の名前か、自分のほかの名前と当たる
	{"gohttp21", codeHTTPTestDir},  // テストのパッケージのディレクトリが、同じ単位が出力する子の名前空間の出力先と重なる
}

// TestDiagnosticCodes は、「診断」の表の ID（GOHTTP-9〜GOHTTP-11、GOHTTP-16、GOHTTP-20、GOHTTP-21）のどれもが、入力を一つ持つこと。
// ほかの番号は欠番（語彙のプラグイン http の HTTP-* に移した。docs/plugin-go-http.md「診断」の「欠番」。その入力は plugin/httpvocab）。
func TestDiagnosticCodes(t *testing.T) {
	seen := map[string]bool{}
	for _, fx := range diagnosticFixtures {
		seen[fx.code] = true
	}
	live := []int{9, 10, 11, 16, 20, 21}
	for _, n := range live {
		if code := "GOHTTP-" + strconv.Itoa(n); !seen[code] {
			t.Errorf("%s の入力がない", code)
		}
	}
	if len(seen) != len(live) {
		t.Errorf("診断の入力のコードが %d 種類（%d のはず: 欠番の入力が、残っている）", len(seen), len(live))
	}
}

// TestDiagnostics は docs/plugin-go-http.md「診断」の表の GOHTTP-*: 診断のコード・位置・メッセージがゴールデンと同じで、
// 診断を返した出力がファイルも warning も返さないこと（emitViews が確かめる）。
func TestDiagnostics(t *testing.T) {
	for _, fx := range diagnosticFixtures {
		t.Run(fx.name, func(t *testing.T) {
			_, diags, _ := emitViews(t, newRoot(t), loadViews(t, fx.name))
			if len(diags) == 0 {
				t.Fatal("診断がない")
			}
			var b bytes.Buffer
			for _, d := range diags {
				if d.Code != fx.code {
					t.Errorf("コード %s の診断がある（%s だけのはず）: %s", d.Code, fx.code, d.Message)
				}
				b.WriteString(d.Code + "\t" + d.At + "\t" + d.Message + "\n")
			}
			compareTree(t, fx.name, map[string]string{"errors.txt": b.String()})
		})
	}
}

// TestDiagnosticPositions は、診断の位置（docs/plugin-go-http.md「診断」の表）: 表に書いた宣言の FQN。GOHTTP-9 は位置なし。
func TestDiagnosticPositions(t *testing.T) {
	for name, want := range map[string][]string{
		"gohttp9":  {""},
		"gohttp10": {"app.NewHandler", "typed.NewHandler", "wrapped.New"},
		"gohttp11": {"app.a", "app.b"},
		"gohttp16": {"app.items", "app.byName", "app.names"},
		// 当たった自分の名前を生んだ経路の op。決まった名前（RunHTTP、HTTPCases）が当たったなら、相手の名前を生んだ宣言（分類、葉の op）
		"gohttp20": {"app.foo", "typed.HTTP", "leafy.HTTP", "multi.both"},
		// 最初の経路の op
		"gohttp21": {"n.first"},
	} {
		_, diags, _ := emitViews(t, newRoot(t), loadViews(t, name))
		got := map[string]bool{}
		for _, d := range diags {
			got[d.At] = true
		}
		if len(got) != len(want) {
			t.Errorf("%s: 位置 = %v, want %v", name, got, want)
		}
		for _, w := range want {
			if !got[w] {
				t.Errorf("%s: 位置 %q の診断がない（%v）", name, w, got)
			}
		}
	}
}

// TestValidateHome は GOHTTP-11 の条件: 診断を返すのは、validate = true の単位の出力だけ。同じ名前空間を validate = false で出力する
// 単位は、制約を確かめないので、診断なしにファイルを返す。置き場は View だけから求まるので、module がなくても返す。
func TestValidateHome(t *testing.T) {
	views := loadViews(t, "gohttp11")
	files, diags, _ := emitViews(t, newRoot(t), views)
	if files["gen/loose/http.go"] == "" || files["gen/loose/apptest/http.go"] == "" || files["gen/loose/apptest/hozo-tests.go-http.json"] == "" || len(files) != 3 {
		t.Errorf("ファイル = %v, want gen/loose の、二つの http.go と、テストの対応表だけ", sortedPaths(files))
	}
	if strings.Contains(files["gen/loose/http.go"], "Validate") {
		t.Error("validate = false の出力が、制約の検査を持つ")
	}
	if len(diags) != 2 {
		t.Errorf("診断 = %+v, want GOHTTP-11 が二つ", diags)
	}
	for _, d := range diags {
		if !strings.Contains(d.Message, "1 番目の [[emit]]") || !strings.Contains(d.Message, "plain.In") {
			t.Errorf("メッセージが、置き場の単位と入力の model を示さない: %s", d.Message)
		}
	}
	if _, diags, _ = emitViews(t, "", noModule(views)); len(diags) != 2 || diags[0].Code != codeHTTPValidate {
		t.Errorf("module がないとき: 診断 = %+v, want GOHTTP-11 が二つ", diags)
	}
}

// TestOptionValidate は「オプション」: validate は真偽値で、既定は false。true のときだけ、入力を持つすべての経路に、制約の検査を出す
// （入力が Validate を持つかどうかでは、変えない）。
func TestOptionValidate(t *testing.T) {
	var strict *ir.View
	for _, v := range loadViews(t, "routes") {
		if v.Viewer == "strict" {
			strict = v
		}
	}
	root := newRoot(t)
	emit := func(opts map[string]any) string {
		v := *strict
		v.Config.Options = map[string]map[string]any{"go": strict.Config.Options["go"]} // 基底の module は、そのまま
		if opts != nil {
			v.Config.Options[TranspilerName] = opts
		}
		files, diags, _ := emitViews(t, root, []*ir.View{&v})
		if len(diags) > 0 {
			t.Fatalf("診断 %+v", diags)
		}
		return files["gen/strict/http.go"]
	}
	on := emit(map[string]any{"validate": true})
	if n := strings.Count(on, "if !httpValidate(w, in, "); n != 4 {
		t.Errorf("validate = true: httpValidate の呼び出しが %d 回（入力を持つ経路は四つ）", n)
	}
	if !strings.Contains(on, "validate = true") {
		t.Error("validate = true: NewHandler のコメントの二行目がない")
	}
	for name, off := range map[string]string{"書かない": emit(nil), "validate = false": emit(map[string]any{"validate": false}), "空のテーブル": emit(map[string]any{})} {
		if strings.Contains(off, "Validate") || strings.Contains(off, "httpPlace") || strings.Contains(off, "runtime") {
			t.Errorf("%s: 制約の検査を出した", name)
		}
		// ほかは、変わらない: 制約の検査の行と補助を除けば、同じテキスト
		if !strings.Contains(off, `req.query("page", &in.Page, httpDefault(1))`) || !strings.Contains(off, "func (q *httpRequest) ok(") {
			t.Errorf("%s: 入力を読む行か、補助がない", name)
		}
		if strings.Contains(off, "validate = true") {
			t.Errorf("%s: NewHandler のコメントに、制約の検査の行がある", name)
		}
	}
}

// TestNoModule: 基底のオプション module がないとき（基底が GO-8 を返す）、go-http は、ファイルも診断も返さない。
// 仕様だけで決まる診断は、module がなくても返す。
func TestNoModule(t *testing.T) {
	files, diags, warnings := emitViews(t, "", noModule(loadViews(t, "routes")))
	if len(files) != 0 || len(diags) != 0 || len(warnings) != 0 {
		t.Errorf("files=%v 診断=%+v warnings=%+v, want どれも空（GO-8 は基底が返す）", sortedPaths(files), diags, warnings)
	}
	_, diags, _ = emitViews(t, "", noModule(loadViews(t, "gohttp16")))
	if len(diags) == 0 || diags[0].Code != codeHTTPResult {
		t.Errorf("診断 = %+v, want GOHTTP-16", diags)
	}
}

// TestUncheckedSpec は「診断」: http の語彙の使い方の誤り（HTTP-*。docs/plugin-http.md「診断」）は、語彙のプラグインが、先に断る。
// その検査を通っていない仕様（ここでは、経路の op から export を外した View: HTTP-1）を受け取ったら、go-http は、同じ診断を
// 重ねて返さず、error で終わる（hozoc からは hozoc.Crashed）。
func TestUncheckedSpec(t *testing.T) {
	var view *ir.View
	for _, v := range loadViews(t, "routes") {
		if v.Viewer == "bare" {
			view = v
		}
	}
	broken := false
	for _, d := range view.Program.Namespace("bare").Decls {
		if op, ok := d.(*ir.Op); ok && len(op.Decorators) > 0 && op.Exported {
			op.Exported, broken = false, true
		}
	}
	if !broken {
		t.Fatal("入力 routes の名前空間 bare に、経路の op がない")
	}
	resp, err := Handler{}.Emit(request(view, TranspilerName))
	if err == nil || !strings.Contains(err.Error(), "HTTP-1") {
		t.Fatalf("resp = %+v, err = %v, want HTTP-1 を名指しした error", resp, err)
	}
}

// TestBrokenView: View が欠けている（Program がない、viewer の名前空間が Program にない）のは、利用者の仕様の誤りではないので、
// 診断ではなく error で終わる（hozoc からは hozoc.Crashed）。
func TestBrokenView(t *testing.T) {
	view := loadViews(t, "routes")[1]
	for name, v := range map[string]*ir.View{
		"Program がない":          {Version: ir.Version, Viewer: "api", Config: view.Config},
		"viewer が Program にない": {Version: ir.Version, Viewer: "nowhere", Config: view.Config, Program: view.Program},
	} {
		out, err := Emit(request(v, TranspilerName))
		if err == nil || out != nil {
			t.Errorf("%s: out=%+v err=%v, want error だけ", name, out, err)
		}
	}
}

// TestHandler は、プラグインの入口（Handler）: 要求の View から出力し、診断は応答の errors で返す
// （docs/plugin-protocol.md「emit」）。import は持たない。
func TestHandler(t *testing.T) {
	view := loadViews(t, "routes")[1]
	root := newRoot(t)
	h := Handler{}
	resp, err := h.Emit(request(view, TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 3 || len(resp.Errors) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v", len(resp.Files), resp.Errors, resp.Warnings)
	}
	want, _, _ := emitViews(t, root, []*ir.View{view})
	// 並びは、入口のパッケージの http.go、テストのパッケージの http.go、その、テストの対応表
	for i, p := range []string{"gen/api/http.go", "gen/api/apitest/http.go", "gen/api/apitest/hozo-tests.go-http.json"} {
		if f := resp.Files[i]; f.Path != p || want[f.Path] != f.Content {
			t.Errorf("%d 番目のファイル %s が、Emit の結果（%s）と違う", i+1, f.Path, p)
		}
	}
	// 診断は errors で返し、ファイルは返さない
	resp, err = h.Emit(request(loadViews(t, "gohttp9")[0], TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Errors) == 0 || resp.Errors[0].Code != codeHTTPOption {
		t.Errorf("files=%d errors=%+v, want GOHTTP-9", len(resp.Files), resp.Errors)
	}
	// ほかのトランスパイラの名前は unsupported
	if _, err := h.Emit(request(view, "go-valid")); !errors.Is(err, plugin.ErrUnsupported) {
		t.Errorf("知らない transpiler の err = %v, want ErrUnsupported", err)
	}
	if _, err := h.Emit(&plugin.EmitRequest{Transpiler: TranspilerName}); err == nil {
		t.Error("View のない要求で、エラーにならない")
	}
	if _, err := h.Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import の err = %v, want ErrUnsupported", err)
	}
}
