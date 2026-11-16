package gobase

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// このファイルは、実装の漏れがコンパイルエラーになること（docs/plugin-go.md「実装を受け取る口」: メソッドが一つでも足りない値は、
// 渡すところでコンパイルできない）を、生成したコードを実際にコンパイルして確かめる。漏れを、実行のときに見つける道はない。

// linesMethods は、入力 states の状態 app.lines の単位 LinesState のメソッドの、すべての実装（メソッド名 → ソース）。
var linesMethods = map[string]string{
	"IsEmpty":    "func (lines) IsEmpty(context.Context) (bool, error) { return true, nil }",
	"Get":        "func (lines) Get(context.Context) ([]app.Line, error) { return nil, nil }",
	"Post":       "func (lines) Post(context.Context, app.Line) error { return nil }",
	"LineFor":    "func (lines) LineFor(context.Context, string) (*app.Line, error) { return nil, nil }",
	"CountLines": "func (lines) CountLines(context.Context) (int64, error) { return 0, nil }",
	"LinesSince": "func (lines) LinesSince(context.Context, time.Time, *int64) ([]app.Line, error) { return nil, nil }",
	"AddLine":    "func (lines) AddLine(context.Context, app.Line) error { return nil }",
	"ClearLines": "func (lines) ClearLines(context.Context) error { return nil }",
	"RemoveLine": "func (lines) RemoveLine(context.Context, string) error { return nil }",
}

// linesImpl は、LinesState を満たすはずの型のパッケージのソース。omit のメソッドだけを持たない。
func linesImpl(omit string) string {
	var names []string
	for name := range linesMethods {
		if name != omit {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("package impl\n\nimport (\n\t\"context\"\n\t\"time\"\n\n\t\"" + modulePath + "/gen/app\"\n)\n\n")
	b.WriteString("type lines struct{}\n\nvar _ time.Time\n\n// 状態の単位は、New の位置引数で渡す。ここでは、渡すところを、代入で確かめる\nvar _ app.LinesState = lines{}\n\n")
	for _, name := range names {
		b.WriteString(linesMethods[name] + "\n")
	}
	return b.String()
}

// TestIncompleteUnitDoesNotCompile は、状態の単位のメソッドを一つでも持たない型が、渡すところでコンパイルできないことを確かめる:
// 形で決まる問い合わせ、配線が直に呼ぶ形で決まる命令、read（配線が呼ぶものも、どの配線も呼ばないものも）、効果（同じ）。
func TestIncompleteUnitDoesNotCompile(t *testing.T) {
	root, files := emitFixture(t, "states")
	writeFiles(t, root, files)
	impl := filepath.Join(root, "impl", "impl.go")

	// すべてのメソッドを持つ型は、コンパイルできる（下の失敗が、ほかの理由でないこと）
	writeFile(t, impl, linesImpl(""))
	if out, err := runGo(t, root, "build", "./impl"); err != nil {
		t.Fatalf("すべてのメソッドを持つ実装が、コンパイルできない: %v\n%s", err, out)
	}
	for _, tt := range []struct{ method, what string }{
		{"Get", "形で決まる問い合わせ"},
		{"Post", "形で決まる命令（配線が直に呼ぶ lines.post）"},
		{"LineFor", "配線が呼ぶ read"},
		{"LinesSince", "どの配線も呼ばない read"},
		{"AddLine", "配線が呼ぶ効果"},
		{"ClearLines", "遷移を宛先にする効果"},
		{"RemoveLine", "どの配線も呼ばない効果"},
	} {
		writeFile(t, impl, linesImpl(tt.method))
		out, err := runGo(t, root, "build", "./impl")
		if err == nil {
			t.Errorf("%s %s を持たない実装が、コンパイルできた", tt.what, tt.method)
			continue
		}
		if !strings.Contains(out, "missing method "+tt.method) {
			t.Errorf("%s %s を持たない実装のコンパイルエラーが、足りないメソッドを示さない:\n%s", tt.what, tt.method, out)
		}
	}
}
