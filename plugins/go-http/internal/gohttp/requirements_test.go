package gohttp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/plugins/go-http/internal/spec/gohttptest"
)

// TestRequirements は、go-http 自身の要求（docs/plugin-go-http.md「go-http 自身の要求」）を、仕様 plugins/go-http/go-http.hozo
// から作った口（gohttptest.RunRequirements）で走らせる。要求を足せば、requirements のメソッドを書くまで、ここがコンパイルできない。
func TestRequirements(t *testing.T) {
	gohttptest.RunRequirements(t, requirements{})
}

// requirements は、要求ごとの、テストの入れ物。どれも、入力の出力を一時のモジュールに置き、その入力の behavior_test.go の、
// 要求のテストだけを走らせる（docs/plugin-go-http.md「go-http 自身の要求」の「要求」の表）。
type requirements struct{}

func (requirements) BodyLimit(t *testing.T)         { behave(t, "limits", "TestBodyLimit") }
func (requirements) NullIsAbsent(t *testing.T)      { behave(t, "routes", "TestNullIsAbsent") }
func (requirements) FiniteFloat(t *testing.T)       { behave(t, "routes", "TestFiniteFloat") }
func (requirements) InternalErrorHook(t *testing.T) { behave(t, "routes", "TestInternalErrorHook") }

// behave は、入力 name の出力（基底 go と go-valid の出力と合わせたもの）を、一時のモジュールに置き、testdata/<name>/behavior_test.go の
// テスト test だけを走らせる（TestBehavior と同じ組み立て）。
func behave(t *testing.T, name, test string) {
	t.Helper()
	t.Parallel()
	root, files := emitFixture(t, name)
	compose(t, root, loadViews(t, name), files)
	src, err := os.ReadFile(filepath.Join("testdata", name, "behavior_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "behavior_test.go"), string(src))
	out, err := runGo(t, root, "test", "-count=1", "-v", "-run", "^"+test+"$", ".")
	if err != nil {
		t.Fatalf("%s の %s が落ちた: %v\n%s", name, test, err, out)
	}
	if !strings.Contains(out, "--- PASS: "+test) {
		t.Fatalf("%s の %s が走らなかった:\n%s", name, test, out)
	}
}
