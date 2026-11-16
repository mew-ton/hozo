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

// statementsLeaves は、入力 statements の Leaves のメソッドの、すべての実装（メソッド名 → ソース）。make と note は、仕様の上で
// 純粋で throws のない葉、parse は、純粋で throws のある葉。
var statementsLeaves = map[string]string{
	"Load":       "func (leaves) Load(context.Context, app.Id) (app.Item, error) { return app.Item{}, nil }",
	"Save":       "func (leaves) Save(context.Context, app.Item) error { return nil }",
	"Touch":      "func (leaves) Touch(context.Context) error { return nil }",
	"Find":       "func (leaves) Find(context.Context, app.Id, *int64, *string) (*app.Item, error) { return nil, nil }",
	"Make":       "func (leaves) Make(context.Context, app.Id) (app.Item, error) { return app.Item{}, nil }",
	"Note":       "func (leaves) Note(context.Context, app.Item) error { return nil }",
	"Parse":      "func (leaves) Parse(context.Context, string) (app.Id, error) { return \"\", nil }",
	"AsTime":     "func (leaves) AsTime(context.Context) (time.Time, error) { return time.Time{}, nil }",
	"AsDuration": "func (leaves) AsDuration(context.Context) (time.Duration, error) { return 0, nil }",
	"AsId":       "func (leaves) AsId(context.Context) (app.Id, error) { return \"\", nil }",
	"AsCount":    "func (leaves) AsCount(context.Context) (app.Count, error) { return 0, nil }",
	"AsFloat":    "func (leaves) AsFloat(context.Context) (float64, error) { return 0, nil }",
	"AsBool":     "func (leaves) AsBool(context.Context) (bool, error) { return false, nil }",
	"AsItems":    "func (leaves) AsItems(context.Context) ([]app.Item, error) { return nil, nil }",
	"AsTable":    "func (leaves) AsTable(context.Context) (map[string]int64, error) { return nil, nil }",
	"AsOptional": "func (leaves) AsOptional(context.Context) (*app.Item, error) { return nil, nil }",
	"AsShape":    "func (leaves) AsShape(context.Context) (app.Shape, error) { return nil, nil }",
}

// leavesImpl は、Leaves を満たすはずの型のパッケージのソース。replace にあるメソッドは、そのソースに置き換える（"" なら、持たない）。
func leavesImpl(replace map[string]string) string {
	var names []string
	for name := range statementsLeaves {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("package impl\n\nimport (\n\t\"context\"\n\t\"time\"\n\n\t\"" + modulePath + "/gen/app\"\n)\n\n")
	b.WriteString("type leaves struct{}\n\nvar _ context.Context\nvar _ time.Time\n\n// 葉は、New の位置引数で渡す。ここでは、渡すところを、代入で確かめる\nvar _ app.Leaves = leaves{}\n\n")
	for _, name := range names {
		src := statementsLeaves[name]
		if r, ok := replace[name]; ok {
			src = r
		}
		b.WriteString(src + "\n")
	}
	return b.String()
}

// TestLeafOfAnotherShapeDoesNotCompile は、葉の実装の漏れと、形の違いが、渡すところでコンパイルできないこと（docs/plugin-go.md
// 「実装を受け取る口」「op の形」）。仕様の上で純粋な葉（make、note）と、純粋で throws のある葉（parse）も、ctx を取り、error を
// 返す形でなければ、Leaves を満たさない: 前の形（ctx を取らない、error を返さない）の実装は、通らない。
func TestLeafOfAnotherShapeDoesNotCompile(t *testing.T) {
	root, files := emitFixture(t, "statements")
	writeFiles(t, root, files)
	impl := filepath.Join(root, "impl", "impl.go")

	// すべての葉を、ctx を取り error を返す形で持つ型は、コンパイルできる（下の失敗が、ほかの理由でないこと）
	writeFile(t, impl, leavesImpl(nil))
	if out, err := runGo(t, root, "build", "./impl"); err != nil {
		t.Fatalf("すべての葉を持つ実装が、コンパイルできない: %v\n%s", err, out)
	}
	for _, tt := range []struct{ method, what, src, want string }{
		{"Make", "純粋な葉を持たない", "", "missing method Make"},
		{"Note", "戻り値のない純粋な葉を持たない", "", "missing method Note"},
		{"Make", "純粋な葉が、ctx を取らず、error を返さない", "func (leaves) Make(app.Id) app.Item { return app.Item{} }", "wrong type for method Make"},
		{"Make", "純粋な葉が、ctx を取るが、error を返さない", "func (leaves) Make(context.Context, app.Id) app.Item { return app.Item{} }", "wrong type for method Make"},
		{"Note", "戻り値のない純粋な葉が、何も返さない", "func (leaves) Note(app.Item) {}", "wrong type for method Note"},
		{"Parse", "純粋で throws のある葉が、ctx を取らない", "func (leaves) Parse(string) (app.Id, error) { return \"\", nil }", "wrong type for method Parse"},
	} {
		writeFile(t, impl, leavesImpl(map[string]string{tt.method: tt.src}))
		out, err := runGo(t, root, "build", "./impl")
		if err == nil {
			t.Errorf("%s実装が、コンパイルできた", tt.what)
			continue
		}
		if !strings.Contains(out, tt.want) {
			t.Errorf("%s実装のコンパイルエラーが、%q を示さない:\n%s", tt.what, tt.want, out)
		}
	}
}
