package gocli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// goldenFixtures は、出力をゴールデンと比べる入力（testdata/<名前>/）と、それが確かめる docs/plugin-go-cli.md の見出し。
// behavior は、生成したコードを動かすテスト testdata/<名前>/behavior_test.go を持つもの。
var goldenFixtures = []struct {
	name     string
	covers   string
	behavior bool
}{
	{name: "commands", covers: "「出力」「引数の解釈」「コマンドごとの関数」「実行と表示」「使い方の表示」の各行", behavior: true},
	{name: "shapes", covers: "「コマンドごとの関数」: フィールドの型と、例外が引数を持つかどうかによらず、同じ形（docs/testing.md「単体テスト」の「型に依らない形の確かめ」）", behavior: true},
	{name: "locals", covers: "名前空間のパッケージの名前が、生成したコードの局所の名前や、標準ライブラリのパッケージの名前と当たるとき（docs/plugin-go.md「パッケージと import パス」の import の名前）", behavior: true},
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
// 依存するトランスパイラの出力と go-cli の出力を書いたモジュールのルートに置いて、go test を走らせる。確かめることは、
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

// TestPlacement は「出力」の置き場と、公開の名前: cli.go は、Dir(U, N) の下のディレクトリ cli に一つ（パッケージ cli）。
// N の入口のパッケージを import し、内側のパッケージは import しない。公開の名前は Main、Presenter、Failure、JSONPresenter。
// 非公開の名前は、パッケージ名になりうる形（小文字と数字だけ）にしない（名前空間のパッケージの import の名前と当たらないように）。
func TestPlacement(t *testing.T) {
	_, files := emitFixture(t, "commands")
	if got := strings.Join(sortedPaths(files), " "); got != "gen/bare/cli/cli.go gen/demo/cli/cli.go gen/tool/cli/cli.go" {
		t.Fatalf("ファイル = %s", got)
	}
	tool := files["gen/tool/cli/cli.go"]
	for _, want := range []string{
		"\npackage cli\n",
		"\t\"example.com/app/gen/tool\"\n",
		"\t\"example.com/app/gen/fail\"\n",
		"\nfunc Main(ctx context.Context, ops tool.Ops, p Presenter, args []string, env func(string) string, stdout, stderr io.Writer) int {\n",
		"\ntype Presenter interface {\n",
		"\tPresent(w io.Writer, command string, input any, result any) error\n",
		"\tPresentError(w io.Writer, command string, input any, f Failure) error\n",
		"\ntype Failure struct {\n\tName  string\n\tValue any\n\tErr   error\n}\n",
		"\ntype JSONPresenter struct{}\n",
	} {
		if !strings.Contains(tool, want) {
			t.Errorf("gen/tool/cli/cli.go に %q がない", want)
		}
	}
	if strings.Contains(tool, "/inner\"") {
		t.Error("gen/tool/cli/cli.go が、内側のパッケージを import している")
	}
	public := map[string]bool{"Main": true, "Presenter": true, "Failure": true, "JSONPresenter": true}
	lower := regexp.MustCompile(`^[a-z0-9]+$`)
	for p, content := range files {
		for _, line := range strings.Split(content, "\n") {
			var name string
			switch {
			case strings.HasPrefix(line, "func ") && !strings.HasPrefix(line, "func ("):
				name = strings.TrimPrefix(line, "func ")
			case strings.HasPrefix(line, "type "):
				name = strings.TrimPrefix(line, "type ")
			case strings.HasPrefix(line, "var "), strings.HasPrefix(line, "const "):
				t.Errorf("%s: パッケージの変数か定数を持つ: %s", p, line)
				continue
			default:
				continue
			}
			name = name[:strings.IndexAny(name, "([ ")]
			switch {
			case name[0] >= 'A' && name[0] <= 'Z':
				if !public[name] {
					t.Errorf("%s: 文書にない公開の名前 %s", p, name)
				}
			case lower.MatchString(name):
				t.Errorf("%s: 非公開の名前 %s が、パッケージ名になりうる形（小文字と数字だけ）", p, name)
			}
		}
	}
}

// TestLines は「コマンドごとの関数」: run<Op> は `var in <Model>` と `line := cliLine{…}` で始まり、フィールドに付いた cli の
// デコレータごとに一行を、フィールドの順、その中はデコレータを書いた順に並べる。行に書くのは、デコレータの引数（名前を省いて
// いれば、フィールドの名前）と、読む先だけ。fail<Op> は、Signature.exceptions の順に、例外ごとに一行。
func TestLines(t *testing.T) {
	_, files := emitFixture(t, "commands")
	tool, demo := files["gen/tool/cli/cli.go"], files["gen/demo/cli/cli.go"]
	for _, c := range []struct{ file, name, want string }{
		{tool, "tool の emit", `	var in tool.EmitInput
	line := cliLine{args: args, getenv: env}
	line.flag("config", &in.Config)
	line.env("TOOL_CONFIG", &in.Config)
	line.arg(1, &in.Names)
	line.flag("dry-run", &in.DryRun)
	if code, done := line.read(prog, usageEmit(), stdout, stderr); done {
		return code
	}
	if !cliValidate(stderr, prog, in) {
		return 64
	}
	result, err := ops.Emit(ctx, in)
`},
		{demo, "demo の add（@cli.required、@cli.default、名前を省いた @cli.flag / @cli.env）", `	var in demo.AddInput
	line := cliLine{args: args, getenv: env}
	line.required(&in.A)
	line.arg(1, &in.A)
	line.arg(2, &in.B)
	line.flag("count", &in.Count)
	line.orDefault(&in.Count, 1)
	line.flag("verbose", &in.Verbose)
	line.flag("label", &in.Label)
	line.env("DEMO_LABEL", &in.Label)
	line.env("DEMO_SCALE", &in.Scale)
	line.flag("mode", &in.Mode)
	line.required(&in.Mode)
	line.env("tag", &in.Tag)
	line.orDefault(&in.Tag, "none")
	if code, done := line.read(prog, usageAdd(), stdout, stderr); done {
`},
		{demo, "demo の copy（補う値のリテラル）", `	line.flag("ratio", &in.Ratio)
	line.orDefault(&in.Ratio, 0.5)
	line.flag("force", &in.Force)
	line.env("DEMO_FORCE", &in.Force)
	line.flag("offset", &in.Offset)
	line.orDefault(&in.Offset, -1)
	line.arg(2, &in.To)
`},
		{demo, "demo の move（フィールドの順。位置の順ではない）", `	line.arg(3, &in.Rest)
	line.env("DEMO_REST", &in.Rest)
	line.required(&in.Rest)
	line.arg(2, &in.To)
	line.arg(1, &in.From)
	line.flag("keep", &in.Keep)
	line.orDefault(&in.Keep, true)
	line.flag("suffix", &in.Suffix)
	line.orDefault(&in.Suffix, "~")
	line.flag("weight", &in.Weight)
	line.orDefault(&in.Weight, 2)
`},
		{demo, "引数のない op（入力も、制約の検査もない）", `func runPing(ctx context.Context, ops demo.Ops, p Presenter, prog string, args []string, env func(string) string, stdout, stderr io.Writer) int {
	line := cliLine{args: args, getenv: env}
	if code, done := line.read(prog, usagePing(), stdout, stderr); done {
		return code
	}
	if err := ops.Ping(ctx); err != nil {
`},
		{tool, "tool の check の例外（引数を持つもの、持たないもの、別の名前空間のもの）", `	f, code := Failure{Err: err}, 70
	switch {
	case cliThrown[tool.CheckFailed](err, 1, &f, &code):
	case cliThrown[tool.ConfigInvalid](err, 2, &f, &code):
	case cliThrown[tool.Busy](err, 70, &f, &code):
	case cliThrown[fail.Crashed](err, 70, &f, &code):
	}
	if err := p.PresentError(stderr, "check", input, f); err != nil {
`},
		{tool, "例外を持たない op", `	f, code := Failure{Err: err}, 70
	if err := p.PresentError(stderr, "serve", input, f); err != nil {
`},
	} {
		if !strings.Contains(c.file, c.want) {
			t.Errorf("%s: 次のテキストがない:\n%s", c.name, c.want)
		}
	}
}

// TestHelpers: 補助と import は、使うものだけを出す（使わない import はコンパイルできない）。何を使うかは、付いたデコレータと、
// 入力と例外の有無で決まり、型では決まらない: 入力を持つコマンドは、入力が Validate を持つかどうかによらず、同じ補助 cliValidate を呼ぶ。
// 基底は、初めの値を入れる関数を出さない（New<Model> を呼ばない）。
func TestHelpers(t *testing.T) {
	_, files := emitFixture(t, "commands")
	tool, demo, bare := files["gen/tool/cli/cli.go"], files["gen/demo/cli/cli.go"], files["gen/bare/cli/cli.go"]
	methods := map[string]string{"flag": "func (l *cliLine) flag(", "env": "func (l *cliLine) env(", "arg": "func (l *cliLine) arg(",
		"required": "func (l *cliLine) required(", "orDefault": "func (l *cliLine) orDefault("}
	newModel := regexp.MustCompile(`\.New[A-Z]\w*\(\)`)
	for _, c := range []struct {
		name, file string
		methods    string // 出す行のメソッド（空白区切り）
		validate   int    // cliValidate の呼び出しの数（入力を持つコマンドの数）
		thrown     bool   // cliThrown を出すか（例外を持つコマンドがあるか）
	}{
		{"tool", tool, "flag env arg", 3, true},                    // @cli.required も @cli.default もない
		{"demo", demo, "flag env arg required orDefault", 3, true}, // add、copy、move（ping と quit は入力を持たない）
		{"bare", bare, "", 0, false},                               // 入力も例外もないコマンドだけ
	} {
		for name, decl := range methods {
			if want := strings.Contains(" "+c.methods+" ", " "+name+" "); strings.Contains(c.file, decl) != want {
				t.Errorf("%s: cliLine の %s を出すか = %v, want %v", c.name, name, !want, want)
			}
		}
		if n := strings.Count(c.file, "\tif !cliValidate(stderr, prog, in) {\n"); n != c.validate {
			t.Errorf("%s: cliValidate の呼び出しが %d 回, want %d（入力を持つコマンドごとに一つ）", c.name, n, c.validate)
		}
		if got := strings.Contains(c.file, "\nfunc cliValidate("); got != (c.validate > 0) {
			t.Errorf("%s: cliValidate を出すか = %v", c.name, got)
		}
		if got := strings.Contains(c.file, "\nfunc cliThrown["); got != c.thrown {
			t.Errorf("%s: cliThrown を出すか = %v, want %v", c.name, got, c.thrown)
		}
		if got := strings.Contains(c.file, "\t\"github.com/mew-ton/hozo/runtime\"\n"); got != (c.validate > 0 || c.thrown) {
			t.Errorf("%s: runtime を import するか = %v", c.name, got)
		}
		if strings.Contains(c.file, "in.Validate()") {
			t.Errorf("%s: 入力の Validate を、補助を通さずに呼んでいる", c.name)
		}
		if newModel.MatchString(c.file) {
			t.Errorf("%s: 初めの値を入れる関数（New<Model>）を呼んでいる", c.name)
		}
	}
}

// TestSameShape は、型に依らない形の確かめ（docs/testing.md「単体テスト」、docs/requirements.md「出力を推論しない」）: 入力 shapes の
// 二つの名前空間は、宣言の名前と cli のデコレータが同じで、型だけが違う（核の型と scalar、省略可能とそうでないもの、文字列と真偽値、
// 一つの値と array、引数を持つ例外と持たない例外、戻り値と省略可能な戻り値、制約を持つ入力と持たない入力）。出る cli.go は、
// 名前空間の名前のほかは、同じテキスト。違いが実行時に出ることは、testdata/shapes/behavior_test.go が確かめる。
func TestSameShape(t *testing.T) {
	_, files := emitFixture(t, "shapes")
	a, b := files["gen/shapea/cli/cli.go"], files["gen/shapeb/cli/cli.go"]
	if a == "" || b == "" {
		t.Fatalf("ファイル = %v", sortedPaths(files))
	}
	if !strings.Contains(a, "shapea.RunInput") || !strings.Contains(b, "shapeb.RunInput") {
		t.Fatal("入力の model の名前がない")
	}
	if got := strings.ReplaceAll(b, "shapeb", "shapea"); got != a {
		t.Errorf("型だけが違う二つの入力から、違うテキストが出た:\n%s", firstDiff(a, got))
	}
}

// TestNoCommands は「出力」: @cli.command の op が一つもなければ、ファイルを出さず warning を一つ返す。
func TestNoCommands(t *testing.T) {
	root := newRoot(t)
	files, diags, warnings := emitViews(t, root, loadViews(t, "nocommands"))
	if len(files) != 0 || len(diags) != 0 || len(warnings) != 1 {
		t.Fatalf("files=%v 診断=%+v warnings=%+v, want warning が一つだけ", sortedPaths(files), diags, warnings)
	}
	if warnings[0].At != "app" || !strings.Contains(warnings[0].Message, "@cli.command") {
		t.Errorf("warning = %+v", warnings[0])
	}
}

// TestUnitTargets は「対象」: サブコマンドにするのは、単位の対象の op（View.files にあるファイルの op。docs/config.md「in」）だけ。
// 単位の対象でない op は、@cli.command が付いていても、サブコマンドにならない。
func TestUnitTargets(t *testing.T) {
	var demo *ir.View
	for _, v := range loadViews(t, "commands") {
		if v.Viewer == "demo" {
			demo = v
		}
	}
	root := newRoot(t)
	files, diags, warnings := emitViews(t, root, []*ir.View{demo})
	if len(files) != 1 || len(diags) != 0 || len(warnings) != 0 {
		t.Fatalf("files=%v 診断=%+v warnings=%+v, want ファイルが一つ", sortedPaths(files), diags, warnings)
	}
	outside := *demo
	outside.Files = []string{"elsewhere.hozo"}
	files, diags, warnings = emitViews(t, root, []*ir.View{&outside})
	if len(files) != 0 || len(diags) != 0 || len(warnings) != 1 {
		t.Errorf("単位の対象でない op だけ: files=%v 診断=%+v warnings=%+v, want warning が一つだけ", sortedPaths(files), diags, warnings)
	}
}

// TestDir は「出力」: 置き場と import パスは View.config.dir（Dir(U, N)）から求める。単位の namespace の子孫では、dir が out の
// サブディレクトリになっている。
func TestDir(t *testing.T) {
	var demo *ir.View
	for _, v := range loadViews(t, "commands") {
		if v.Viewer == "demo" {
			demo = v
		}
	}
	moved := *demo
	moved.Config.Out, moved.Config.Dir = "out", "out/sub/demo"
	moved.Namespaces = map[string][]ir.NamespaceConfig{"demo": {moved.Config}}
	files, diags, _ := emitViews(t, newRoot(t), []*ir.View{&moved})
	if len(diags) != 0 {
		t.Fatalf("診断 %+v", diags)
	}
	content, ok := files["out/sub/demo/cli/cli.go"]
	if !ok || !strings.Contains(content, "\t\""+modulePath+"/out/sub/demo\"\n") {
		t.Errorf("ファイル = %v（import パスは dir から）", sortedPaths(files))
	}
}

// diagnosticFixtures は、診断を返す入力（testdata/<名前>/）。期待する診断は testdata/<名前>/expected/errors.txt
// （一行に、コード、位置、メッセージをタブで区切って。View の順、その中は返した順）。
var diagnosticFixtures = []struct{ name, code string }{
	{"cli1", codeCLIParams},       // @cli.command の op の引数が、0 個でも model 一つでもない
	{"cli2", codeCLISource},       // 入力の model のフィールドに値の出どころがない、または @cli.arg と @cli.flag の両方がある
	{"cli3", codeCLIFieldType},    // フィールドの型が受け付けられない
	{"cli4", codeCLIArrayArg},     // array の @cli.arg が、いちばん後ろの位置でない
	{"cli5", codeCLIDuplicate},    // フラグ名かコマンド名の重複
	{"cli6", codeCLIExport},       // @cli.command の付いた op に export がない
	{"cli7", codeCLIDir},          // ディレクトリ cli が、同じ単位が出力する子の名前空間 N.cli の出力先と重なる
	{"cli8", codeCLIOption},       // [emit.options.go-cli] に、知らないキーがある
	{"cli9", codeCLIValidate},     // Validate を持つ入力の model の置き場の単位が、go-valid を並べていない
	{"cli10", codeCLIPosition},    // @cli.arg の位置が、1 から、重なりも抜けもなく続いていない
	{"cli11", codeCLIOptional},    // @cli.required か @cli.default の付いたフィールドの型が、省略可能
	{"cli12", codeCLIBoth},        // 一つのフィールドに、@cli.required と @cli.default の両方がある
	{"cli13", codeCLIDefaultType}, // @cli.default のリテラルが、フィールドの型に渡せない
	{"cli14", codeCLIDefault},     // @cli.default の値が、フィールドの制約（@valid.*）に合わない
}

// TestDiagnostics は docs/plugin-go-cli.md「診断」の CLI-1〜CLI-14:診断のコード・位置・メッセージがゴールデンと同じで、
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
				if d.Message == "" {
					t.Errorf("%s（%s）にメッセージがない", d.Code, d.At)
				}
				b.WriteString(d.Code + "\t" + d.At + "\t" + d.Message + "\n")
			}
			compareTree(t, fx.name, map[string]string{"errors.txt": b.String()})
		})
	}
}

// TestDiagnosticPositions は、診断の位置と数（docs/plugin-go-cli.md「診断」の表）: 表に書いた宣言の FQN。CLI-7 と CLI-8 は位置なし。
func TestDiagnosticPositions(t *testing.T) {
	for name, want := range map[string][]string{
		"cli1":  {"app.two", "app.core", "app.named", "app.optional", "app.many"},
		"cli2":  {"app.In", "app.In", "app.In", "app.In"},
		"cli3":  {"app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In"},
		"cli4":  {"app.Early", "app.Twice", "app.Order"},
		"cli5":  {"app.In", "app.again", "app.third", "app.Implied"},
		"cli6":  {"app.hidden", "app.also"},
		"cli7":  {""},
		"cli8":  {"", ""},
		"cli9":  {"app.a", "app.b"},
		"cli10": {"app.Zero", "app.Twice", "app.Gap", "app.Late"},
		"cli11": {"app.In", "app.In", "app.In", "app.In"},
		"cli12": {"app.In", "app.In"},
		"cli13": {"app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In"},
		"cli14": {"app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In", "app.In"},
	} {
		_, diags, _ := emitViews(t, newRoot(t), loadViews(t, name))
		var got []string
		for _, d := range diags {
			got = append(got, d.At)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: 位置 = %q, want %q", name, got, want)
		}
	}
}

// TestAllErrorsAreReported: 一つの出力に複数の誤りがあれば、止まらずにすべて報告する（別の規則のものも）。
func TestAllErrorsAreReported(t *testing.T) {
	view := *loadViews(t, "cli6")[0]
	view.Config.Options = map[string]map[string]any{TranspilerName: {"color": true}}
	_, diags, _ := emitViews(t, newRoot(t), []*ir.View{&view})
	var codes []string
	for _, d := range diags {
		codes = append(codes, d.Code)
	}
	if got := strings.Join(codes, " "); got != "CLI-8 CLI-6 CLI-6" {
		t.Errorf("診断 = %s", got)
	}
}

// TestNoModule: 基底のオプション module がないとき（基底が GO-8 を返す）、go-cli は、ファイルも診断も返さない。
// 仕様だけで決まる診断は、module がなくても返す。
func TestNoModule(t *testing.T) {
	files, diags, warnings := emitViews(t, "", noModule(loadViews(t, "commands")))
	if len(files) != 0 || len(diags) != 0 || len(warnings) != 0 {
		t.Errorf("files=%v 診断=%+v warnings=%+v, want どれも空（GO-8 は基底が返す）", sortedPaths(files), diags, warnings)
	}
	_, diags, _ = emitViews(t, "", noModule(loadViews(t, "cli6")))
	if len(diags) == 0 || diags[0].Code != codeCLIExport {
		t.Errorf("診断 = %+v, want CLI-6", diags)
	}
}

// TestBrokenView: View が欠けている（Program がない、viewer の名前空間が Program にない）のは、利用者の仕様の誤りではないので、
// 診断ではなく error で終わる（hozoc からは proc.Crashed）。
func TestBrokenView(t *testing.T) {
	view := loadViews(t, "commands")[0]
	for name, v := range map[string]*ir.View{
		"Program がない":          {Version: ir.Version, Viewer: "tool", Config: view.Config},
		"viewer が Program にない": {Version: ir.Version, Viewer: "nowhere", Config: view.Config, Program: view.Program},
	} {
		out, err := Emit(request(v, TranspilerName))
		if err == nil || out != nil {
			t.Errorf("%s: out=%+v err=%v, want error だけ", name, out, err)
		}
	}
}

// TestHandler は、プラグインの入口（Handler）: 要求の View から出力し、診断は応答の errors で返す
// （docs/plugin-protocol.md「emit」）。提供するトランスパイラは go-cli の一つ。import は持たない。
func TestHandler(t *testing.T) {
	view := loadViews(t, "commands")[0]
	root := newRoot(t)
	h := Handler{}
	resp, err := h.Emit(request(view, TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 1 || len(resp.Errors) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v", len(resp.Files), resp.Errors, resp.Warnings)
	}
	want, _, _ := emitViews(t, root, []*ir.View{view})
	if f := resp.Files[0]; f.Path != "gen/tool/cli/cli.go" || want[f.Path] != f.Content {
		t.Errorf("%s が Emit の結果と違う", f.Path)
	}
	// 診断は errors で返し、ファイルは返さない
	resp, err = h.Emit(request(loadViews(t, "cli8")[0], TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Errors) == 0 || resp.Errors[0].Code != codeCLIOption {
		t.Errorf("files=%d errors=%+v, want CLI-8", len(resp.Files), resp.Errors)
	}
	// ほかのトランスパイラの名前は unsupported
	for _, name := range []string{"", "go", "go-http"} {
		if _, err := h.Emit(request(view, name)); !errors.Is(err, plugin.ErrUnsupported) {
			t.Errorf("transpiler %q の err = %v, want ErrUnsupported", name, err)
		}
	}
	if _, err := h.Emit(&plugin.EmitRequest{Transpiler: TranspilerName}); err == nil {
		t.Error("View のない要求で、エラーにならない")
	}
	if _, err := h.Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import の err = %v, want ErrUnsupported", err)
	}
}

// TestProtocol は、JSON の要求を plugin.Run に通したときの応答の形（docs/plugin-protocol.md「emit」「要求と応答」）。
func TestProtocol(t *testing.T) {
	h := Handler{}
	run := func(view *ir.View) string {
		t.Helper()
		req, err := json.Marshal(request(view, TranspilerName).Batch())
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		if code := plugin.Run(h, bytes.NewReader(req), &out, &errOut); code != 0 || errOut.Len() != 0 {
			t.Fatalf("code = %d, stderr = %q", code, errOut.String())
		}
		return out.String()
	}
	// 成功: warnings は空でも [] で出し、errors は省く。select は書かない（版 0.4 でなくした）
	out := run(loadViews(t, "commands")[0])
	resp, err := plugin.SingleResult([]byte(out))
	if err != nil {
		t.Fatalf("応答が JSON でない: %v\n%s", err, out)
	}
	if len(resp.Files) != 1 || resp.Files[0].Path != "gen/tool/cli/cli.go" {
		t.Fatalf("files = %+v", resp.Files)
	}
	if !strings.Contains(out, `"warnings":[]`) || strings.Contains(out, `"errors"`) || strings.Contains(out, `"select"`) {
		t.Errorf("warnings は []、errors と select は書かない: %.200s", out)
	}
	// 診断: files は []
	out = run(loadViews(t, "cli6")[0])
	if want := `{"outputs":[{"results":[{"files":[],"warnings":[],"errors":[{"code":"CLI-6","at":"app.hidden","message":`; !strings.HasPrefix(out, want) {
		t.Errorf("応答 = %s\nwant 先頭が %s", out, want)
	}
	// 実装しない direction。check は、なくした direction（docs/plugin-protocol.md「検査のための頼みごとを、なくしたこと」）
	for _, dir := range []string{"import", "check"} {
		var o, e bytes.Buffer
		code := plugin.Run(Handler{}, strings.NewReader(`{"protocol":"`+ir.Version+`","direction":"`+dir+`"}`), &o, &e)
		if code != 2 || !strings.Contains(o.String(), `"code":"unsupported"`) {
			t.Errorf("%s: code = %d, stdout = %s", dir, code, o.String())
		}
	}
}

// TestPluginBinary は、プラグインの main を go run で起動し、標準入出力で一往復する（docs/plugin-protocol.md「起動」）。
func TestPluginBinary(t *testing.T) {
	goBin := goTool(t)
	root := newRoot(t)
	view := loadViews(t, "commands")[0]
	req, err := json.Marshal(request(view, TranspilerName).Batch())
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goBin, "run", "github.com/mew-ton/hozo/plugins/go-cli")
	cmd.Dir = repoRoot(t)
	cmd.Env = os.Environ()
	cmd.Stdin = bytes.NewReader(req)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run: %v\n%s", err, errOut.String())
	}
	resp, err := plugin.SingleResult(out.Bytes())
	if err != nil {
		t.Fatalf("応答が JSON でない: %v\n%s", err, out.String())
	}
	want, _, _ := emitViews(t, root, []*ir.View{view})
	if len(resp.Files) != 1 || resp.Files[0].Path != "gen/tool/cli/cli.go" || resp.Files[0].Content != want["gen/tool/cli/cli.go"] {
		t.Errorf("応答が Emit の結果と違う: %d 個のファイル", len(resp.Files))
	}
}

// TestManifest は、マニフェスト（docs/plugin-go-cli.md「マニフェスト」）。外部の TOML は使えないので、行で見る。
func TestManifest(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "hozo-plugin.toml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	checks := map[string]*regexp.Regexp{
		"name":     regexp.MustCompile(`(?m)^name\s*=\s*"go-cli"$`),
		"command":  regexp.MustCompile(`(?m)^command\s*=\s*\["go", "run", "github.com/mew-ton/hozo/plugins/go-cli"\]$`),
		"protocol": regexp.MustCompile(`(?m)^protocol\s*=\s*"` + regexp.QuoteMeta(ir.Version) + `"$`),
		// 提供するトランスパイラは一つ: 名前は TranspilerName、言語は go
		"transpiler": regexp.MustCompile(`(?m)^\[\[transpiler\]\]\nname\s*=\s*"` + regexp.QuoteMeta(TranspilerName) + `"\nlanguage\s*=\s*"go"$`),
		// Go の基底 go と、検証の go-valid に相乗りする
		"requires": regexp.MustCompile(`(?m)^requires\s*=\s*\["go", "go-valid"\]$`),
		// cli の語彙は、このプラグインが提供する
		"vocabulary": regexp.MustCompile(`(?m)^vocabulary\s*=\s*\["cli\.hozo"\]`),
	}
	for name, re := range checks {
		if !re.MatchString(s) {
			t.Errorf("マニフェストに %s がない:\n%s", name, s)
		}
	}
	if strings.Count(s, "[[transpiler]]") != 1 {
		t.Errorf("提供するトランスパイラは一つ:\n%s", s)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "cli.hozo")); err != nil {
		t.Errorf("plugins/go-cli/cli.hozo がない: %v", err)
	}
}
