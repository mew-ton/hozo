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
	{name: "uniforma", covers: "型に依らない形の確かめ（docs/testing.md「単体テスト」）の一つ目: 核の型、省略可能でないフィールド、値を運ばない例外、戻り値のある op"},
	{name: "uniformb", covers: "同、二つ目: scalar、省略可能なフィールド、値を運ぶ例外、省略可能な戻り値の op。uniforma と同じテキストになる（TestUniform）"},
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

// TestPlacement は「出力」の置き場と、公開の名前: http.go は、N の入口のパッケージに一つ（内部の op を持つ名前空間でも、
// 内側のパッケージではない）。公開の名前は NewHandler 一つで、非公開の名前は http で始まる。
func TestPlacement(t *testing.T) {
	_, files := emitFixture(t, "routes")
	want := map[string]string{
		"gen/api/http.go":    "package api\n",
		"gen/strict/http.go": "package strict\n",
		"gen/bare/http.go":   "package bare\n",
	}
	if len(files) != len(want) {
		t.Errorf("ファイル = %v, want %d 個", sortedPaths(files), len(want))
	}
	for p, pkg := range want {
		if !strings.Contains(files[p], "\n"+pkg) {
			t.Errorf("%s が %q を持たない", p, strings.TrimSpace(pkg))
		}
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
			if name != handlerName && !strings.HasPrefix(name, "http") {
				t.Errorf("%s: パッケージに足す名前 %s が、NewHandler でも、http で始まる名前でもない", p, name)
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
// 値を運ぶ例外と運ばない例外、戻り値のある op と省略可能な戻り値の op、配列と record の結果）。二つの http.go は、フィールドごと・
// op ごと・例外ごとの行だけでなく、ファイルの全体が、同じテキストになる。違いが実行時に出ることは、behavior_test.go が確かめる。
func TestUniform(t *testing.T) {
	_, a := emitFixture(t, "uniforma")
	_, b := emitFixture(t, "uniformb")
	if len(a) != 1 || a["gen/app/http.go"] == "" {
		t.Fatalf("uniforma のファイル = %v", sortedPaths(a))
	}
	if a["gen/app/http.go"] != b["gen/app/http.go"] {
		t.Errorf("型だけが違う二つの入力の http.go が、違うテキストになった:\n%s", firstDiff(a["gen/app/http.go"], b["gen/app/http.go"]))
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
	resp, err := Handler{Getenv: func(string) string { return root }}.Emit(request(loadViews(t, "noroutes")[0], TranspilerName))
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
	if len(files) != 1 || len(diags) != 0 || len(warnings) != 0 {
		t.Fatalf("files=%v 診断=%+v warnings=%+v, want ファイルが一つ", sortedPaths(files), diags, warnings)
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
	{"gohttp1", codeHTTPExport},      // @http.get / @http.post の付いた op に export がない
	{"gohttp2", codeHTTPParams},      // 経路の op の引数が、0 個でも model 一つでもない
	{"gohttp3", codeHTTPPlace},       // 入力の model のフィールドが、場所の六つのデコレータのどれも持たない、または二つ以上持つ
	{"gohttp4", codeHTTPFieldType},   // ヘッダかクエリから読むフィールド（全体の形なら、その model のフィールド）の型が受け付けられない
	{"gohttp5", codeHTTPJSON},        // 本文、結果、@http.status を持つ例外の値に、JSON にできない型がある
	{"gohttp6", codeHTTPGetBody},     // @http.get の op の入力が、本文のデコレータのフィールドを持つ
	{"gohttp7", codeHTTPPath},        // パスが / で始まらない、または { か } を含む
	{"gohttp8", codeHTTPDuplicate},   // 同じメソッドとパスの経路が二つある
	{"gohttp9", codeHTTPOption},      // [emit.options.go-http] に、知らないキーか、型の違う値がある
	{"gohttp10", codeHTTPName},       // NewHandler が、N の定義のパッケージの名前と当たる
	{"gohttp11", codeHTTPValidate},   // validate = true で、Validate を持つ入力の model の置き場の単位が、go-valid を並べていない
	{"gohttp12", codeHTTPWholeType},  // 全体の形を付けたフィールドの型が、model でない
	{"gohttp13", codeHTTPWholeMix},   // 一つの場所に、全体の形が二つ以上ある、または、全体の形と項目の形の両方がある
	{"gohttp14", codeHTTPNoItem},     // @http.required か @http.default の付いたフィールドが、項目の形を持たない
	{"gohttp15", codeHTTPOptional},   // @http.required か @http.default の付いたフィールドの型が、省略可能
	{"gohttp16", codeHTTPResult},     // 経路の op の結果の型が、省略可能な配列か、省略可能な record
	{"gohttp17", codeHTTPBoth},       // 一つのフィールドに、@http.required と @http.default の両方がある
	{"gohttp18", codeHTTPDefault},    // @http.default のリテラルが、フィールドの型に渡せない
	{"gohttp19", codeHTTPConstraint}, // @http.default の値が、フィールドの制約（@valid.*）に合わない
}

// TestDiagnosticCodes は、「診断」の表の ID（GOHTTP-1〜GOHTTP-19。欠番はない）のどれもが、入力を一つ持つこと。
func TestDiagnosticCodes(t *testing.T) {
	seen := map[string]bool{}
	for _, fx := range diagnosticFixtures {
		seen[fx.code] = true
	}
	for n := 1; n <= 19; n++ {
		if code := "GOHTTP-" + strconv.Itoa(n); !seen[code] {
			t.Errorf("%s の入力がない", code)
		}
	}
	if len(seen) != 19 {
		t.Errorf("診断の入力のコードが %d 種類（19 のはず）", len(seen))
	}
}

// TestDiagnostics は docs/plugin-go-http.md「診断」の GOHTTP-1〜GOHTTP-19: 診断のコード・位置・メッセージがゴールデンと同じで、
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
		"gohttp1": {"app.hidden", "app.also"},
		"gohttp2": {"app.two", "app.core", "app.byId", "app.optional", "app.many", "app.either"},
		"gohttp3": {"app.In"},
		// 全体の形で受け取る model のフィールドの型は、その model の位置
		"gohttp4":  {"app.HeaderIn", "app.QueryIn", "app.Meta", "app.Paging"},
		"gohttp5":  {"app.putShape", "app.putAlbum", "app.putDrawing", "app.getDrawing", "app.getShapes", "app.clash"},
		"gohttp6":  {"app.read", "app.both", "app.whole"},
		"gohttp7":  {"app.relative", "app.empty", "app.absolute", "app.byId", "app.brace"},
		"gohttp8":  {"app.second", "app.third", "app.again"},
		"gohttp9":  {""},
		"gohttp10": {"app.NewHandler", "typed.NewHandler", "wrapped.New"},
		"gohttp11": {"app.a", "app.b"},
		"gohttp12": {"app.In", "app.More"},
		"gohttp13": {"app.Twice", "app.Mixed"},
		// 全体の形のフィールドは入力の model、その中のフィールドは、全体の形で受け取る model の位置
		"gohttp14": {"app.In", "app.Paging"},
		"gohttp15": {"app.In"},
		"gohttp16": {"app.items", "app.byName", "app.names"},
		"gohttp17": {"app.In"},
		"gohttp18": {"app.In"},
		"gohttp19": {"app.In"},
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
// 単位は、制約を確かめないので、診断なしにファイルを返す。置き場は View だけから求まるので、go.mod がなくても返す。
func TestValidateHome(t *testing.T) {
	views := loadViews(t, "gohttp11")
	files, diags, _ := emitViews(t, newRoot(t), views)
	if _, ok := files["gen/loose/http.go"]; !ok || len(files) != 1 {
		t.Errorf("ファイル = %v, want gen/loose/http.go だけ", sortedPaths(files))
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
	if _, diags, _ = emitViews(t, t.TempDir(), views); len(diags) != 2 || diags[0].Code != codeHTTPValidate {
		t.Errorf("go.mod がないとき: 診断 = %+v, want GOHTTP-11 が二つ", diags)
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
		v.Config.Options = map[string]map[string]any{}
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

// TestNoGoMod: Dir(U, N) の上に go.mod がないとき（基底が GO-1 を返す）、go-http は、ファイルも診断も返さない。
// 仕様だけで決まる診断は、go.mod がなくても返す。
func TestNoGoMod(t *testing.T) {
	files, diags, warnings := emitViews(t, t.TempDir(), loadViews(t, "routes"))
	if len(files) != 0 || len(diags) != 0 || len(warnings) != 0 {
		t.Errorf("files=%v 診断=%+v warnings=%+v, want どれも空（GO-1 は基底が返す）", sortedPaths(files), diags, warnings)
	}
	_, diags, _ = emitViews(t, t.TempDir(), loadViews(t, "gohttp1"))
	if len(diags) == 0 || diags[0].Code != codeHTTPExport {
		t.Errorf("診断 = %+v, want GOHTTP-1", diags)
	}
}

// TestBrokenView: View が欠けている（Program がない、viewer の名前空間が Program にない）のは、利用者の仕様の誤りではないので、
// 診断ではなく error で終わる（hozoc からは proc.Crashed）。
func TestBrokenView(t *testing.T) {
	view := loadViews(t, "routes")[1]
	for name, v := range map[string]*ir.View{
		"Program がない":          {Version: ir.Version, Viewer: "api", Config: view.Config},
		"viewer が Program にない": {Version: ir.Version, Viewer: "nowhere", Config: view.Config, Program: view.Program},
	} {
		out, err := Emit(request(v, TranspilerName), newRoot(t))
		if err == nil || out != nil {
			t.Errorf("%s: out=%+v err=%v, want error だけ", name, out, err)
		}
	}
}

// TestHandler は、プラグインの入口（Handler）: HOZO_PROJECT_ROOT をルートにして出力し、診断は応答の errors で返す
// （docs/plugin-protocol.md「emit」）。check と import は持たない。
func TestHandler(t *testing.T) {
	view := loadViews(t, "routes")[1]
	root := newRoot(t)
	h := Handler{Getenv: func(key string) string {
		if key == "HOZO_PROJECT_ROOT" {
			return root
		}
		return ""
	}}
	resp, err := h.Emit(request(view, TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 1 || len(resp.Errors) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v", len(resp.Files), resp.Errors, resp.Warnings)
	}
	want, _, _ := emitViews(t, root, []*ir.View{view})
	if f := resp.Files[0]; f.Path != "gen/api/http.go" || want[f.Path] != f.Content {
		t.Errorf("%s が Emit の結果と違う", f.Path)
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
	if _, err := h.Check(&plugin.CheckRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Check の err = %v, want ErrUnsupported", err)
	}
	if _, err := h.Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import の err = %v, want ErrUnsupported", err)
	}
}
