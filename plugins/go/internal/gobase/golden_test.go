package gobase

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
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
	{name: "matchliteral", covers: "「match」の、絞り込みが literal の分け方（if <対象>）と、ケースのない側の、分類の誤りの枝", behavior: true},
	{name: "literals", covers: "「型」の、リテラルの型の行: 「model」のリテラルの型のフィールド、「union」のリテラルの型のアーム、「値」の、リテラルの型のアームに入れる式とフィールドを読む式、「match」の variant（リテラルの型のアーム）と三つに分ける形（対象が boolean?）、「分類」の、対象が boolean? の literal の確かめ", behavior: true},
	{name: "types", covers: "「型」（model、scalar、union、exception、effect）と「コメント」、予約語の名前空間のパッケージ名、op のない名前空間の状態の単位、「分類」の、対象のない分類（何も出さない）"},
	{name: "names", covers: "「名前」の局所の名前、「パッケージと import パス」の import の名前、「New」の引数の名前、「別の名前空間の op」"},
	{name: "pkgnames", covers: "「パッケージと import パス」の import の名前のうち、名前空間のパッケージ名が、基底が決まった名前で使う識別子（ops、ptr、o、ctx、err）や、途中の値（v0、k0）と同じとき。「New」の引数の名前 ops"},
	{name: "statements", covers: "「呼び出し」「文」の表の各行（空の val、値を入れる文、集める文、for、disjoint を含む）、「match」の x = match と xs collect match、失敗の扱いの零値"},
	{name: "values", covers: "「値」の表の各行、「型をそろえる」、「省略可能を通るパス」"},
	{name: "unionarm", covers: "「値」の「union に入れる」（引数、レコードリテラルのフィールド、return、val、値を入れる文、集める文、throw の引数。構造体のアームと、包む型のアーム）", behavior: true},
	{name: "states", covers: "「op の置き場」（デコレータが reads と effects を運ぶ葉も Leaves）、「コメント」の Reads / Effects（配線に運ばれたものを含む）、「状態の単位」（read、効果、形で決まる命令）、「呼び出し」の read・効果・直に呼ぶ組み込みの効果、「状態の参照」", behavior: true},
	{name: "bind", covers: "「bind で埋まる位置」の 1〜4、「別の名前空間の op」"},
	{name: "matchif", covers: "「match」の二つに分ける形（nullable、list、presence）と、アームの本体の組"},
	{name: "matchswitch", covers: "「match」の switch の形（variant、述語で決まる分類）と、その「分類の誤りの枝」（型の switch の default:）、else のアーム、「絞り込んだ値」、「分類」", behavior: true},
	{name: "matchelse", covers: "「match」の、残りのケース（対象 as else）を持つ型で決まる分類: 二つに分ける形のもう一方の枝（presence）、残りのアームを並べた case と絞り込んだ値（variant）。「分類」（何も出さない）", behavior: true},
	{name: "matchclosed", covers: "「match」の、二つに分ける形の「分類の誤りの枝」（残りのケースを書かない presence の分類）と、else のアーム（受けるケースを名前で並べたアームと同じコード）", behavior: true},
	{name: "rowstate", covers: "「effect」「状態の単位」「コメント」の、行の遷移を持つ効果（メソッドの形は変わらず、コメントに行の遷移の表示。行の引数を絞り込んだ効果は「効果の入口の確かめ」）と、「分類」の、ない側のケース（述語を持たない。判定関数が、値がないことで決める）、「match」の、ない側のケースを as none で書いた presence の分類", behavior: true},
	{name: "throws", covers: "「exception」（引数ごとにフィールド一つ）と、「match」のアームの本体の throw E(<値>, …)、例外を投げる効果（「状態の単位」と、単位のメソッドのコメントの throws）", behavior: true},
	{name: "try", covers: "「例外を受ける文」の各行（try のブロックの関数、catch (e)、throw e、throw 例外(値)、空の catch、catch (e by 分類) の型の switch と default、try と catch が入れる val、for の中の try、入れ子、本体のあるデコレータの try { next(); }）", behavior: true},
	{name: "seams", covers: "「継ぎ目」の各行、「内側のフォルダと入口」", behavior: true},
	{name: "wraps", covers: "「本体のあるデコレータ」の各行（外側のメソッド、包んだ関数の呼び出しの式、値を返す op の result、値の引数、別の名前空間の op と分類の述語、継ぎ目との入れ子、包んだ関数の呼び出しのアームを持つ match の後ろの文、never を返す op）と、「デコレータのテストの口」の、関係ごとの確かめと、「受けるケース」（名前空間 desk: ignores、recovers、halts、引数で渡した例外、残りの引数、ユニオン、同じデコレータを二度、当たる名前、受けた例外が throws にもあるもの、op の例外にない例外）", behavior: true},
	{name: "typespkg", covers: "「型のパッケージ」（関数を持つ名前空間の型を、types/ に置く。定義のパッケージの再公開。別の名前空間は、型のパッケージを import する。型だけの名前空間は、分けない）と、「パッケージと import パス」（互いの型を使う二つの名前空間が、輪にならない）", behavior: true},
	{name: "unittest", covers: "「単体テストの口」（葉の op の結果ごとのケース、当たる結果の名前、Then_、絞り込んだ戻り値の確かめ、分類の判定の例、口を持たないもの）と、「効果の入口の確かめ」（包む型、New が包むこと、別の名前空間の分類の述語）", behavior: true},
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
			got := strings.ReplaceAll(second[moved], modulePath+"/elsewhere/out", modulePath+"/"+dir)
			if isTestMap(p) { // 対応表は、file のパスの頭の、出力先のディレクトリが違う
				got = strings.ReplaceAll(second[moved], `"file": "elsewhere/out/`, `"file": "`+dir+`/`)
			}
			if got != content {
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
	{"go7", codeSeamOnBody},  // 単位のトランスパイラが、本体のあるデコレータに、継ぎ目を宣言している
	{"go9", codeLiteralType}, // 宣言が、リテラルの型を、Go の形のない位置に使う
}

// TestDiagnostics は docs/plugin-go.md「診断」の GO-2〜GO-7、GO-9: 診断のコード・位置・メッセージがゴールデンと同じで、
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

// withoutModule は、view の写しで、基底へのオプション module を持たないもの（Namespaces の、どの出力の設定も）。
func withoutModule(view *ir.View) *ir.View {
	strip := func(c ir.NamespaceConfig) ir.NamespaceConfig {
		options := map[string]map[string]any{}
		for name, table := range c.Options {
			if name != c.Base {
				options[name] = table
			}
		}
		c.Options = options
		return c
	}
	v := *view
	v.Config = strip(view.Config)
	v.Namespaces = map[string][]ir.NamespaceConfig{}
	for ns, list := range view.Namespaces {
		for _, c := range list {
			v.Namespaces[ns] = append(v.Namespaces[ns], strip(c))
		}
	}
	return &v
}

// TestNoModule は GO-8: オプション module が、書かれていない、文字列でない、空。位置なし。ファイルは返さない。
// メッセージは、hozo.toml の [options.go] に書くことを示す（docs/plugin-go.md「診断」）。
func TestNoModule(t *testing.T) {
	view := loadViews(t, "statements")[0]
	for name, value := range map[string]any{"書かれていない": nil, "文字列でない": 1.0, "空": ""} {
		v := withoutModule(view)
		if value != nil {
			v.Config.Options[TranspilerName] = map[string]any{"module": value}
		}
		files, diags := emitViews(t, "", []*ir.View{v})
		if len(files) != 0 || len(diags) != 1 || diags[0].Code != "GO-8" || diags[0].At != "" {
			t.Fatalf("%s: files=%d 診断 = %+v, want GO-8 が一つ（位置なし）", name, len(files), diags)
		}
		if !strings.Contains(diags[0].Message, "[options.go]") || !strings.Contains(diags[0].Message, "module") {
			t.Errorf("%s: メッセージが、[options.go] の module を示さない: %s", name, diags[0].Message)
		}
	}
	// 知らないキーは GO-5（module は、知っているキー）
	v := withoutModule(view)
	v.Config.Options[TranspilerName] = map[string]any{"module": "example.com/app", "modul": "x"}
	if _, diags := emitViews(t, "", []*ir.View{v}); len(diags) != 1 || diags[0].Code != codeOption || !strings.Contains(diags[0].Message, "modul") {
		t.Errorf("知らないキー: 診断 = %+v, want GO-5 が一つ", diags)
	}
}

// TestReadsNothing は、プラグインが、ファイルも、環境変数も、読まないこと（決定 D425、
// docs/plugin-go.md「パッケージと import パス」）: go.mod のない、空の作業ディレクトリで、環境変数を空にしても、出力が同じ。
func TestReadsNothing(t *testing.T) {
	views := loadViews(t, "statements")
	want, diags := emitViews(t, "", views)
	if len(diags) != 0 {
		t.Fatalf("診断: %+v", diags)
	}
	t.Chdir(t.TempDir())
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	got, diags := emitViews(t, "", views)
	if len(diags) != 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("作業ディレクトリと環境を変えたら、出力が変わった（診断 %+v）", diags)
	}
}

// TestInternal は GO-INTERNAL: View が欠けている（Program がない、viewer の名前空間が Program にない）。
func TestInternal(t *testing.T) {
	view := loadViews(t, "statements")[0]
	for name, v := range map[string]*ir.View{
		"Program がない":          {Version: ir.Version, Viewer: "app", Config: view.Config},
		"viewer が Program にない": {Version: ir.Version, Viewer: "nowhere", Config: view.Config, Program: view.Program},
	} {
		files, diags := Emit(request(v), nil)
		if len(files) != 0 || len(diags) != 1 || diags[0].Code != codeInternal {
			t.Errorf("%s: files=%d 診断=%+v, want GO-INTERNAL が一つ", name, len(files), diags)
		}
	}
}

// TestHandler は、プラグインの入口（Handler）: 要求の View から出力し、診断は応答の errors で返す
// （docs/plugin-protocol.md「emit」）。import は持たない。
func TestHandler(t *testing.T) {
	view := loadViews(t, "statements")[0]
	h := Handler{}
	resp, err := h.Emit(request(view))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) == 0 || len(resp.Errors) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v", len(resp.Files), resp.Errors, resp.Warnings)
	}
	want, _ := emitViews(t, "", []*ir.View{view})
	for _, f := range resp.Files {
		if want[f.Path] != f.Content {
			t.Errorf("%s が Emit の結果と違う", f.Path)
		}
	}
	// module のない設定: GO-8 を errors で返し、ファイルは返さない
	resp, err = h.Emit(request(withoutModule(view)))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Errors) != 1 || resp.Errors[0].Code != codeNoModule {
		t.Errorf("files=%d errors=%+v, want GO-8", len(resp.Files), resp.Errors)
	}
	// ほかのトランスパイラの名前は unsupported
	other := request(view)
	other.Transpiler = "go-other"
	if _, err := h.Emit(other); err == nil {
		t.Error("知らない transpiler で、エラーにならない")
	}
	if _, err := h.Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import の err = %v, want ErrUnsupported", err)
	}
}

// TestTypesPackageText は「型のパッケージ」の、出力のテキストを、文書から手で書いたものと比べる: 型のパッケージの types.go、
// 定義のパッケージ（内側）の再公開、別の名前空間の型のパッケージを import する行、型だけの名前空間（分けない）。
func TestTypesPackageText(t *testing.T) {
	_, files := emitFixture(t, "typespkg")
	want := map[string]string{
		"gen/carry/types/types.go": `// Code generated by hozoc go 0.0.0-dev; DO NOT EDIT.

package types

// Code は運送会社のコード
type Code string

// Quote は見積もり
type Quote struct {
	Code  Code  ` + "`json:\"code\"`" + `
	Price int64 ` + "`json:\"price\"`" + `
}
`,
		"gen/carry/inner/types.go": `// Code generated by hozoc go 0.0.0-dev; DO NOT EDIT.

package inner

import "example.com/app/gen/carry/types"

// Code は運送会社のコード
type Code = types.Code

// Quote は見積もり
type Quote = types.Quote
`,
		"gen/ship/types/types.go": `// Code generated by hozoc go 0.0.0-dev; DO NOT EDIT.

package types

import carry "example.com/app/gen/carry/types"

// Parcel は荷物
type Parcel struct {
	Weight int64 ` + "`json:\"weight\"`" + `
}

// Label は送り状。運送会社のコードは、carry の scalar
type Label struct {
	Carrier carry.Code ` + "`json:\"carrier\"`" + `
	Id      string     ` + "`json:\"id\"`" + `
}
`,
		"gen/plain/types.go": `// Code generated by hozoc go 0.0.0-dev; DO NOT EDIT.

package plain

import carry "example.com/app/gen/carry/types"

// Note は model plain.Note。
type Note struct {
	Text string     ` + "`json:\"text\"`" + `
	By   carry.Code ` + "`json:\"by\"`" + `
}
`,
	}
	for p, w := range want {
		if files[p] != w {
			t.Errorf("%s が、手で書いたテキストと違う:\n---- got ----\n%s\n---- want ----\n%s", p, files[p], w)
		}
	}
	// 型のパッケージは、関数のパッケージを import しない。関数のパッケージは、相手の、型のパッケージだけを import する
	for p, content := range files {
		inTypes := strings.Contains(p, "/types/")
		for _, line := range strings.Split(content, "\n") {
			if !strings.Contains(line, "\"example.com/app/gen/") {
				continue
			}
			own := strings.HasPrefix(p, "gen/ship/") && strings.Contains(line, "gen/ship") || strings.HasPrefix(p, "gen/carry/") && strings.Contains(line, "gen/carry")
			if inTypes && !strings.HasSuffix(strings.TrimSpace(line), "/types\"") {
				t.Errorf("%s: 型のパッケージが、型のパッケージでないものを import している: %s", p, line)
			}
			if !own && !strings.Contains(p, "test/") && !strings.HasSuffix(strings.TrimSpace(line), "/types\"") {
				t.Errorf("%s: 別の名前空間の、型のパッケージでないものを import している: %s", p, line)
			}
		}
	}
}
