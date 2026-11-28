package gocli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/plugins/go-cli/internal/spec/goclitest"
)

// TestRequirements は、go-cli 自身の要求（plugins/go-cli/spec/go-cli.hozo の hozo.gocli.Requirement）を、仕様から作った口
// （goclitest.RunRequirements）で走らせる。要求を足せば、requirements のメソッドを書くまで、ここがコンパイルできない。
func TestRequirements(t *testing.T) {
	goclitest.RunRequirements(t, requirements{})
}

// requirements は、要求ごとの、テストの入れ物。出力の形は golden_test.go の関数で、生成したコードの振る舞いは、入力の
// behavior_test.go のテスト（behaviorTests）で確かめる。
type requirements struct{}

func (requirements) Output(t *testing.T) {
	t.Run("placement", outputPlacement)
	t.Run("noCommands", outputNoCommands)
	t.Run("dir", outputDir)
	behave(t, "locals", "TestEnv", "TestIn")
}

func (requirements) Arguments(t *testing.T) {
	behave(t, "commands", "TestAddInput", "TestCopyInput", "TestMoveInput", "TestBare")
	behave(t, "shapes", "TestRequired", "TestValidate")
}

func (requirements) CommandFunctions(t *testing.T) {
	t.Run("lines", functionLines)
	t.Run("helpers", functionHelpers)
	t.Run("sameShape", functionSameShape)
	behave(t, "shapes", "TestSameArguments", "TestThrown")
}

func (requirements) Run(t *testing.T) {
	behave(t, "commands", "TestTool", "TestDemo", "TestPresenter", "TestJSONPresenter")
}

func (requirements) Usage(t *testing.T) {
	behave(t, "shapes", "TestUsage")
}

// behaviorTests は、要求が走らせる、入力ごとの behavior_test.go のテスト。TestEveryBehaviorIsRequired が、入力の
// behavior_test.go のテストと、ちょうど同じであることを確かめる。
var behaviorTests = map[string][]string{
	"commands": {"TestAddInput", "TestBare", "TestCopyInput", "TestDemo", "TestJSONPresenter", "TestMoveInput", "TestPresenter", "TestTool"},
	"locals":   {"TestEnv", "TestIn"},
	"shapes":   {"TestRequired", "TestSameArguments", "TestThrown", "TestUsage", "TestValidate"},
}

// TestEveryBehaviorIsRequired は、入力の behavior_test.go のテストが、どれも、どれかの要求から走ること（behaviorTests）:
// 要求に結ばないテストを足せば、ここで落ちる。
func TestEveryBehaviorIsRequired(t *testing.T) {
	re := regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`)
	for _, fx := range goldenFixtures {
		if !fx.behavior {
			continue
		}
		src, err := os.ReadFile(filepath.Join("testdata", fx.name, "behavior_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			got = append(got, m[1])
		}
		sort.Strings(got)
		if want := behaviorTests[fx.name]; strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("testdata/%s/behavior_test.go のテスト = %v、要求が走らせるもの = %v", fx.name, got, want)
		}
	}
}

// behave は、入力 name の出力（基底 go と go-valid の出力と合わせたもの）を、一時のモジュールに置き、testdata/<name>/behavior_test.go の
// テスト tests だけを走らせる。
func behave(t *testing.T, name string, tests ...string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		root, files := emitFixture(t, name)
		compose(t, root, loadViews(t, name), files)
		src, err := os.ReadFile(filepath.Join("testdata", name, "behavior_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, "behavior_test.go"), string(src))
		out, err := runGo(t, root, "test", "-count=1", "-v", "-run", "^("+strings.Join(tests, "|")+")$", ".")
		if err != nil {
			t.Fatalf("%s の %v が落ちた: %v\n%s", name, tests, err, out)
		}
		for _, test := range tests {
			if !strings.Contains(out, "--- PASS: "+test+" ") {
				t.Errorf("%s の %s が走らなかった:\n%s", name, test, out)
			}
		}
	})
}
