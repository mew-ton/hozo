package gosqlite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// goldenFixtures は、出力をゴールデンと比べる入力（testdata/<名前>/）と、それが確かめる docs/plugin-go-sqlite.md の見出し。
// behavior は、生成したコードを本物の SQLite の上で動かすテスト testdata/<名前>/behavior_test.go を持つもの。
var goldenFixtures = []struct {
	name     string
	covers   string
	behavior bool
}{
	{name: "columns", covers: "「表」の列の型の表の各行（核の型と scalar）、省略可能な列、制約（複数の @db.key、@db.unique、@db.ref）", behavior: true},
	{name: "ops", covers: "「op の中身」の規則の各行と、省略可能な引数、「メソッドの形とコメント」", behavior: true},
	{name: "partial", covers: "「状態ごとの型」の、規則に当たらない op があるとき（warning、var _ を出さない、出していない op の句）", behavior: true},
	{name: "names", covers: "「補助」の、当たる名前（引数、途中の変数、同じ名前の model）と、内側のフォルダを持つ出力"},
}

// TestGolden は、入力ごとに、出力（db.go と warning）がゴールデン testdata/<名前>/expected/ とバイト単位で同じこと、go/format を
// 通した形であること、同じ View から同じバイト列を返すこと（docs/plugin-protocol.md「emit」の決定性）、生成したコードが、基底の
// 出力と合わせてコンパイルできる（一時のモジュールで go vet を通る）ことを確かめる。
func TestGolden(t *testing.T) {
	for _, fx := range goldenFixtures {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			root, res := emitFixture(t, fx.name)
			if len(res.files) == 0 {
				t.Fatal("ファイルを一つも出力しなかった")
			}
			checkFormatted(t, res.files)
			got := map[string]string{}
			for p, content := range res.files {
				got[p] = content
			}
			if len(res.warnings) > 0 {
				got["warnings.txt"] = warningsText(res.warnings)
			}
			compareTree(t, fx.name, got)
			// 決定性: もう一度、別のルート（同じモジュールのパス）で出力しても、同じバイト列
			again := emitViews(t, newRoot(t), ours(t, loadViews(t, fx.name)))
			if len(again.files) != len(res.files) {
				t.Errorf("二度目の出力のファイルの数が違う: %d と %d", len(res.files), len(again.files))
			}
			for p, content := range res.files {
				if again.files[p] != content {
					t.Errorf("%s が、二度目の出力で変わった", p)
				}
			}
			if warningsText(again.warnings) != warningsText(res.warnings) {
				t.Error("warning が、二度目の出力で変わった")
			}
			vet(t, root, fx.name, res.files)
		})
	}
}

// TestBehavior は、生成したコードを、一時のモジュールで、本物の SQLite の上で動かす（docs/testing.md「単体テスト」）: 入力の
// behavior_test.go を、基底の出力と go-sqlite の出力を置いたモジュールのルートに置いて、go test を走らせる。ドライバは、
// サンプル shop-server と同じもの（addDriver）。確かめることは、それぞれのファイルの頭。
func TestBehavior(t *testing.T) {
	for _, fx := range goldenFixtures {
		if !fx.behavior {
			continue
		}
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			root, res := emitFixture(t, fx.name)
			addDriver(t, root)
			writeProject(t, root, fx.name, res.files)
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

// TestNoTables は、表の状態が一つもない名前空間（docs/plugin-go-sqlite.md「出力」）: ファイルを出さず、warning を一つ返す。
func TestNoTables(t *testing.T) {
	res := emitViews(t, newRoot(t), ours(t, loadViews(t, "none")))
	if len(res.files) != 0 || len(res.errors) != 0 {
		t.Fatalf("files=%v errors=%v, want どちらも空", sortedPaths(res.files), res.errors)
	}
	if len(res.warnings) != 1 {
		t.Fatalf("warning が %d 件（期待は 1）: %v", len(res.warnings), res.warnings)
	}
	compareTree(t, "none", map[string]string{"warnings.txt": warningsText(res.warnings)})
}

// TestOtherDir は、同じ名前空間を別の out に出力しても、ファイルの中身が、パスを除いて同じこと（db.go は、自分のパッケージの
// import パスを書かない）。
func TestOtherDir(t *testing.T) {
	view := ours(t, loadViews(t, "ops"))[0]
	root := newRoot(t)
	first := emitViews(t, root, []*ir.View{view})
	other := *view
	other.Config.Out, other.Config.Dir = "elsewhere/out", "elsewhere/out"
	second := emitViews(t, root, []*ir.View{&other})
	if len(first.files) != 1 || len(second.files) != 1 {
		t.Fatalf("ファイルの数が %d と %d（期待は、どちらも 1）", len(first.files), len(second.files))
	}
	if a, b := first.files[view.Config.Dir+"/"+FileName], second.files["elsewhere/out/"+FileName]; a == "" || a != b {
		t.Errorf("別の out の db.go が違う:\n%s", firstDiff(a, b))
	}
}

// diagnosticFixtures は、診断を返す入力（testdata/<名前>/）。期待する診断は testdata/<名前>/expected/errors.txt
// （一行に、コード、位置、メッセージをタブで区切って。View の順、その中は返した順）。
var diagnosticFixtures = []struct{ name, code string }{
	{"gosql1", codeSQLElement},   // @db の状態の要素が model でない
	{"gosql2", codeSQLShape},     // @db の状態の形が list でない
	{"gosql3", codeSQLColumn},    // 列にできない型のフィールドがある
	{"gosql4", codeSQLCollision}, // 公開の名前が当たる（条件ごとに名前空間が一つ。当たらない名前空間には出ない）
	{"gosql6", codeSQLOption},    // [emit.options.go-sqlite] に、知らないキーがある
}

// TestDiagnostics は docs/plugin-go-sqlite.md「診断」の GOSQL-1〜GOSQL-4、GOSQL-6: 診断のコード・位置・メッセージがゴールデンと
// 同じで、診断を返した出力がファイルを返さないこと（emitViews が確かめる）。
func TestDiagnostics(t *testing.T) {
	for _, fx := range diagnosticFixtures {
		t.Run(fx.name, func(t *testing.T) {
			res := emitViews(t, newRoot(t), ours(t, loadViews(t, fx.name)))
			if len(res.errors) == 0 {
				t.Fatal("診断がない")
			}
			for _, d := range res.errors {
				if d.Code != fx.code {
					t.Errorf("コード %s の診断がある（%s だけのはず）: %s", d.Code, fx.code, d.Message)
				}
			}
			compareTree(t, fx.name, map[string]string{"errors.txt": errorsText(res.errors)})
		})
	}
}

// TestDiagnosedOutputsOnly は、診断が、条件に当たった出力にだけ出ること: GOSQL-4 の入力の、当たらない名前空間 clean は、
// ファイルを返す。
func TestDiagnosedOutputsOnly(t *testing.T) {
	for _, v := range ours(t, loadViews(t, "gosql4")) {
		res := emitViews(t, newRoot(t), []*ir.View{v})
		if clean := v.Viewer == "clean"; clean != (len(res.errors) == 0) || clean != (len(res.files) == 1) {
			t.Errorf("%s: files=%d errors=%d", v.Viewer, len(res.files), len(res.errors))
		}
	}
}

// TestNoGoMod は、Dir(U, N) の上に go.mod がないとき: 報告するのは基底（GO-1。docs/plugin-go.md「診断」）で、go-sqlite は、
// ファイルも診断も返さない（型のパッケージの import パスを求められないので、何も出せない）。
func TestNoGoMod(t *testing.T) {
	res := emitViews(t, t.TempDir(), ours(t, loadViews(t, "ops")))
	if len(res.files) != 0 || len(res.errors) != 0 || len(res.warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v, want どれも空", len(res.files), res.errors, res.warnings)
	}
}

// TestMalformedView は、View が欠けている（Program がない、viewer の名前空間が Program にない）とき: 利用者の仕様の誤りではないので、
// 診断ではなく誤りを返す（プラグインは終了コード 1 で終わる。docs/plugin-protocol.md「起動」）。
func TestMalformedView(t *testing.T) {
	view := ours(t, loadViews(t, "ops"))[0]
	for name, v := range map[string]*ir.View{
		"Program がない":          {Version: ir.Version, Viewer: "shop", Config: view.Config},
		"viewer が Program にない": {Version: ir.Version, Viewer: "nowhere", Config: view.Config, Program: view.Program},
	} {
		if resp, err := Emit(request(v), newRoot(t)); err == nil {
			t.Errorf("%s: 誤りを返さない（応答 %+v）", name, resp)
		}
	}
}

// TestHandler は、プラグインの入口（Handler）: HOZO_PROJECT_ROOT をルートにして出力し、診断は応答の errors で、warning は
// warnings で返す（docs/plugin-protocol.md「emit」）。check と import は持たない。
func TestHandler(t *testing.T) {
	root := newRoot(t)
	h := Handler{Getenv: func(key string) string {
		if key == "HOZO_PROJECT_ROOT" {
			return root
		}
		return ""
	}}
	view := ours(t, loadViews(t, "ops"))[0]
	resp, err := h.Emit(request(view))
	if err != nil {
		t.Fatal(err)
	}
	want := emitViews(t, root, []*ir.View{view})
	if len(resp.Files) != 1 || len(resp.Errors) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v", len(resp.Files), resp.Errors, resp.Warnings)
	}
	if f := resp.Files[0]; f.Path != "gen/shop/db.go" || want.files[f.Path] != f.Content {
		t.Errorf("%s が Emit の結果と違う", f.Path)
	}
	// 診断は errors で返し、ファイルは返さない
	resp, err = h.Emit(request(ours(t, loadViews(t, "gosql6"))[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Errors) != 2 || resp.Errors[0].Code != codeSQLOption {
		t.Errorf("files=%d errors=%+v, want GOSQL-6 が二つ", len(resp.Files), resp.Errors)
	}
	// warning は warnings で返す
	resp, err = h.Emit(request(ours(t, loadViews(t, "none"))[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Errors) != 0 || len(resp.Warnings) != 1 {
		t.Errorf("files=%d errors=%v warnings=%v, want warning が一つ", len(resp.Files), resp.Errors, resp.Warnings)
	}
	// ほかのトランスパイラの名前は unsupported
	other := request(view)
	other.Transpiler = "go"
	if _, err := h.Emit(other); err == nil {
		t.Error("知らない transpiler で、エラーにならない")
	}
	if _, err := h.Check(&plugin.CheckRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Check の err = %v, want ErrUnsupported", err)
	}
	if _, err := h.Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import の err = %v, want ErrUnsupported", err)
	}
}
