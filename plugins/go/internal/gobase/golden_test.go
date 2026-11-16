package gobase

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// goldenFixtures は、出力をゴールデンと比べる入力（testdata/<名前>/）と、それが確かめる docs/plugin-go.md の見出し。
// behavior は、生成したコードを動かすテスト testdata/<名前>/behavior_test.go を持つもの。
var goldenFixtures = []struct {
	name     string
	covers   string
	behavior bool
}{
	{name: "types", covers: "「型」（model、scalar、union、exception、effect）と「コメント」、予約語の名前空間のパッケージ名、op のない名前空間の状態の単位"},
	{name: "names", covers: "「名前」の局所の名前、「パッケージと import パス」の import の名前、「New」の引数の名前、「別の名前空間の op」"},
	{name: "pkgnames", covers: "「パッケージと import パス」の import の名前のうち、名前空間のパッケージ名が、基底が決まった名前で使う識別子（ops、ptr、o、ctx、err）や、途中の値（v0、k0）と同じとき。「New」の引数の名前 ops"},
	{name: "statements", covers: "「呼び出し」「文」の表の各行、失敗の扱いの零値"},
	{name: "values", covers: "「値」の表の各行、「型をそろえる」、「省略可能を通るパス」"},
	{name: "states", covers: "「op の置き場」（デコレータが reads と effects を運ぶ葉も Leaves）、「コメント」の Reads / Effects（配線に運ばれたものを含む）、「状態の単位」（read、効果、形で決まる命令）、「呼び出し」の read・効果・直に呼ぶ組み込みの効果、「状態の参照」", behavior: true},
	{name: "bind", covers: "「bind で埋まる位置」の 1〜4、「別の名前空間の op」"},
	{name: "matchif", covers: "「match」の二つに分ける形（nullable、list、presence）と、アームの本体の組"},
	{name: "matchswitch", covers: "「match」の switch の形（variant、述語で決まる分類）、「絞り込んだ値」、「分類」", behavior: true},
	{name: "matchelse", covers: "「match」の、残りのケース（対象 as else）を持つ型で決まる分類: 二つに分ける形のもう一方の枝（presence）、残りのアームを並べた case と絞り込んだ値（variant）。「分類」（何も出さない）", behavior: true},
	{name: "throws", covers: "「exception」（引数ごとにフィールド一つ）と、「match」のアームの本体の throw E(<値>, …)", behavior: true},
	{name: "seams", covers: "「継ぎ目」の各行、「内側のフォルダと入口」", behavior: true},
}

// TestGolden は、入力ごとに、出力がゴールデン testdata/<名前>/expected/ とバイト単位で同じこと、go/format を通した形で
// あること、同じ View から同じバイト列を返すこと（docs/plugin-go.md「決定性」）、生成したコードがコンパイルできる
// （一時のモジュールで go vet を通る）ことを確かめる。
func TestGolden(t *testing.T) {
	for _, fx := range goldenFixtures {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			root, files := emitFixture(t, fx.name)
			checkFormatted(t, files)
			compareTree(t, fx.name, files)
			// 決定性: もう一度、別のルート（同じモジュールのパス）で出力しても、同じバイト列
			again, _ := emitViews(t, newRoot(t), loadViews(t, fx.name))
			if len(again) != len(files) {
				t.Errorf("二度目の出力のファイルの数が違う: %d と %d", len(files), len(again))
			}
			for p, content := range files {
				if again[p] != content {
					t.Errorf("%s が、二度目の出力で変わった", p)
				}
			}
			vet(t, root, files)
		})
	}
}

// TestBehavior は、生成したコードを一時のモジュールで実際に動かす（docs/testing.md「単体テスト」）: 入力の
// behavior_test.go を、生成したコードのモジュールのルートに置いて、go test を走らせる。確かめることは、それぞれのファイルの頭。
func TestBehavior(t *testing.T) {
	for _, fx := range goldenFixtures {
		if !fx.behavior {
			continue
		}
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			root, files := emitFixture(t, fx.name)
			writeFiles(t, root, files)
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

// TestSameNamespaceOtherDir は、同じ名前空間の二つの単位の基底のファイルが、in と単位の継ぎ目が同じなら、import パスに
// 現れる自分のディレクトリを除いて、バイト単位で同じこと（docs/plugin-go.md「決定性」）。
func TestSameNamespaceOtherDir(t *testing.T) {
	for _, name := range []string{"statements", "seams"} {
		view := loadViews(t, name)[0]
		root := newRoot(t)
		first, diags := emitViews(t, root, []*ir.View{view})
		if len(diags) > 0 {
			t.Fatalf("%s: 診断 %v", name, diags)
		}
		dir := view.Config.Dir
		other := *view
		other.Config.Out, other.Config.Dir = "elsewhere/out", "elsewhere/out"
		second, diags := emitViews(t, root, []*ir.View{&other})
		if len(diags) > 0 {
			t.Fatalf("%s: 診断 %v", name, diags)
		}
		if len(first) != len(second) {
			t.Fatalf("%s: ファイルの数が違う: %d と %d", name, len(first), len(second))
		}
		for p, content := range first {
			moved := "elsewhere/out" + strings.TrimPrefix(p, dir)
			if got := strings.ReplaceAll(second[moved], modulePath+"/elsewhere/out", modulePath+"/"+dir); got != content {
				t.Errorf("%s: %s が、別の out の出力 %s と違う:\n%s", name, p, moved, firstDiff(content, got))
			}
		}
	}
}

// diagnosticFixtures は、診断を返す入力（testdata/<名前>/）。期待する診断は testdata/<名前>/expected/errors.txt
// （一行に、コード、位置、メッセージをタブで区切って。View の順、その中は返した順）。
var diagnosticFixtures = []struct{ name, code string }{
	{"go2", codeCollision},   // 生成する名前が当たる（条件ごとに名前空間が一つ）
	{"go3", codeImportCycle}, // 出力どうしの import が循環する（輪に入る出力ごとに出る。輪に入らない出力には出ない）
	{"go4", codeInnerDir},    // 内側のフォルダ inner が、子の名前空間 N.inner の出力先と重なる
	{"go5", codeOption},      // [emit.options.go] に、知らないキーがある
	{"go6", codeVariant},     // variant の絞り込みの型と等しい型のアームが、union に二つ以上ある
}

// TestDiagnostics は docs/plugin-go.md「診断」の GO-2〜GO-6: 診断のコード・位置・メッセージがゴールデンと同じで、
// 診断を返した出力がファイルを返さないこと（emitViews が確かめる）。
func TestDiagnostics(t *testing.T) {
	for _, fx := range diagnosticFixtures {
		t.Run(fx.name, func(t *testing.T) {
			_, diags := emitViews(t, newRoot(t), loadViews(t, fx.name))
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

// TestNoGoMod は GO-1: Dir(U, N) の上に go.mod がない。位置なし。ファイルは返さない。
func TestNoGoMod(t *testing.T) {
	_, diags := emitViews(t, t.TempDir(), loadViews(t, "statements"))
	if len(diags) != 1 || diags[0].Code != codeNoGoMod || diags[0].At != "" {
		t.Fatalf("診断 = %+v, want GO-1 が一つ（位置なし）", diags)
	}
	if !strings.Contains(diags[0].Message, "gen/app") || !strings.Contains(diags[0].Message, "go.mod") {
		t.Errorf("メッセージが出力先と go.mod を示さない: %s", diags[0].Message)
	}
}

// TestInternal は GO-INTERNAL: View が欠けている（Program がない、viewer の名前空間が Program にない）。
func TestInternal(t *testing.T) {
	view := loadViews(t, "statements")[0]
	for name, v := range map[string]*ir.View{
		"Program がない":          {Version: ir.Version, Viewer: "app", Config: view.Config},
		"viewer が Program にない": {Version: ir.Version, Viewer: "nowhere", Config: view.Config, Program: view.Program},
	} {
		files, diags := Emit(request(v), newRoot(t), nil)
		if len(files) != 0 || len(diags) != 1 || diags[0].Code != codeInternal {
			t.Errorf("%s: files=%d 診断=%+v, want GO-INTERNAL が一つ", name, len(files), diags)
		}
	}
}

// TestHandler は、プラグインの入口（Handler）: HOZO_PROJECT_ROOT をルートにして出力し、診断は応答の errors で返す
// （docs/plugin-protocol.md「emit」）。check と import は持たない。
func TestHandler(t *testing.T) {
	view := loadViews(t, "statements")[0]
	root := newRoot(t)
	h := Handler{Getenv: func(key string) string {
		if key == "HOZO_PROJECT_ROOT" {
			return root
		}
		return ""
	}}
	resp, err := h.Emit(request(view))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) == 0 || len(resp.Errors) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v", len(resp.Files), resp.Errors, resp.Warnings)
	}
	want, _ := emitViews(t, root, []*ir.View{view})
	for _, f := range resp.Files {
		if want[f.Path] != f.Content {
			t.Errorf("%s が Emit の結果と違う", f.Path)
		}
	}
	// go.mod のないルート: GO-1 を errors で返し、ファイルは返さない
	empty := t.TempDir()
	h.Getenv = func(string) string { return empty }
	resp, err = h.Emit(request(view))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Errors) != 1 || resp.Errors[0].Code != codeNoGoMod {
		t.Errorf("files=%d errors=%+v, want GO-1", len(resp.Files), resp.Errors)
	}
	// ほかのトランスパイラの名前は unsupported
	other := request(view)
	other.Transpiler = "go-other"
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
