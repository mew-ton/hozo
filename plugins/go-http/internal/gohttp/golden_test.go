package gohttp

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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
	{name: "routes", covers: "「出力」のハンドラの中の 1〜5、「入力の読み方」の表の各行、「補助」（使うものだけ）、「通信の形」、「制約の検査」", behavior: true},
	{name: "locals", covers: "名前空間のパッケージの名前が、生成したコードの局所の名前や、標準ライブラリのパッケージの名前と当たるとき（docs/plugin-go.md「パッケージと import パス」の import の名前）"},
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
			case strings.HasPrefix(line, "func "):
				name = strings.TrimPrefix(line, "func ")
			case strings.HasPrefix(line, "type "):
				name = strings.TrimPrefix(line, "type ")
			default:
				continue
			}
			name = name[:strings.IndexAny(name, "( ")]
			if name != handlerName && !strings.HasPrefix(name, "http") {
				t.Errorf("%s: パッケージに足す名前 %s が、NewHandler でも、http で始まる名前でもない", p, name)
			}
		}
	}
}

// TestHelpers は「補助」: 使うものだけを、決まった順で出す。
func TestHelpers(t *testing.T) {
	_, files := emitFixture(t, "routes")
	order := []string{"httpErrorBody", "httpBadRequest", "httpViolation", "httpPlace", "httpReadBody", "httpWriteJSON", "httpWriteBadRequest", "httpWriteInvalid", "httpWriteError"}
	declared := func(content string) []string {
		var names []string
		for _, line := range strings.Split(content, "\n") {
			for _, prefix := range []string{"func http", "type http"} {
				if strings.HasPrefix(line, prefix) {
					rest := strings.TrimPrefix(line, prefix[:5])
					names = append(names, rest[:strings.IndexAny(rest, "( ")])
				}
			}
		}
		return names
	}
	for p, want := range map[string][]string{
		// validate = false: 制約の検査の補助（httpPlace、httpWriteInvalid）はない
		"gen/api/http.go": {"httpErrorBody", "httpBadRequest", "httpViolation", "httpReadBody", "httpWriteJSON", "httpWriteBadRequest", "httpWriteError"},
		// validate = true: すべて
		"gen/strict/http.go": order,
		// 入力がなく、@http.status を持つ例外もない: 失敗の本文と、JSON を書く補助と、いつも 500 の httpWriteError だけ
		"gen/bare/http.go": {"httpErrorBody", "httpWriteJSON", "httpWriteError"},
	} {
		got := declared(files[p])
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s の補助 = %v, want %v", p, got, want)
		}
	}
	if strings.Contains(files["gen/bare/http.go"], "switch") || strings.Contains(files["gen/bare/http.go"], `"errors"`) {
		t.Error("gen/bare/http.go: @http.status を持つ例外がないのに、switch を書いた")
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

// diagnosticFixtures は、診断を返す入力（testdata/<名前>/）。期待する診断は testdata/<名前>/expected/errors.txt
// （一行に、コード、位置、メッセージをタブで区切って。View の順、その中は返した順）。
var diagnosticFixtures = []struct{ name, code string }{
	{"gohttp1", codeHTTPExport},    // @http.get / @http.post の付いた op に export がない
	{"gohttp2", codeHTTPParams},    // 経路の op の引数が、0 個でも model 一つでもない
	{"gohttp3", codeHTTPPlace},     // 入力の model のフィールドが、どれも持たない、または二つ以上持つ
	{"gohttp4", codeHTTPFieldType}, // @http.header か @http.query のフィールドの型が受け付けられない
	{"gohttp5", codeHTTPJSON},      // 本文、結果、@http.status を持つ例外の値に、JSON にできない型がある
	{"gohttp6", codeHTTPGetBody},   // @http.get の op の入力が @http.body のフィールドを持つ
	{"gohttp7", codeHTTPPath},      // パスが / で始まらない、または { か } を含む
	{"gohttp8", codeHTTPDuplicate}, // 同じメソッドとパスの経路が二つある
	{"gohttp9", codeHTTPOption},    // [emit.options.go-http] に、知らないキーか、型の違う値がある
	{"gohttp10", codeHTTPName},     // NewHandler が、N の定義のパッケージの名前と当たる
}

// TestDiagnostics は docs/plugin-go-http.md「診断」の GOHTTP-1〜GOHTTP-10: 診断のコード・位置・メッセージがゴールデンと同じで、
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
		"gohttp1":  {"app.hidden", "app.also"},
		"gohttp2":  {"app.two", "app.core", "app.byId", "app.optional", "app.many", "app.either"},
		"gohttp3":  {"app.In"},
		"gohttp4":  {"app.HeaderIn", "app.QueryIn"},
		"gohttp5":  {"app.putShape", "app.putAlbum", "app.getDrawing", "app.getShapes", "app.clash"},
		"gohttp6":  {"app.read", "app.both"},
		"gohttp7":  {"app.relative", "app.empty", "app.absolute", "app.byId", "app.brace"},
		"gohttp8":  {"app.second", "app.third", "app.again"},
		"gohttp9":  {""},
		"gohttp10": {"app.Handler", "typed.NewHandler", "wrapped.New"},
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

// TestOptionValidate は「オプション」: validate は真偽値で、既定は false。true のときだけ、Validate を持つ入力の検査を出す。
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
	if n := strings.Count(on, "in.Validate()"); n != 2 {
		t.Errorf("validate = true: in.Validate() が %d 回（Validate を持つ入力は二つ）", n)
	}
	if !strings.Contains(on, "validate = true") {
		t.Error("validate = true: NewHandler のコメントの二行目がない")
	}
	for name, off := range map[string]string{"書かない": emit(nil), "validate = false": emit(map[string]any{"validate": false}), "空のテーブル": emit(map[string]any{})} {
		if strings.Contains(off, "Validate()") || strings.Contains(off, "httpWriteInvalid") || strings.Contains(off, "httpPlace") || strings.Contains(off, "runtime") {
			t.Errorf("%s: 制約の検査を出した", name)
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
