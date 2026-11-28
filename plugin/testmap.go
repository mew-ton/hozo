package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// TestMap はテストの対応表（docs/plugin-protocol.md「テストの対応表」）: テストを受け取る口を出すトランスパイラが、
// emit の応答の files に足す JSON の中身。
type TestMap struct {
	Tests []TestEntry `json:"tests"`
}

// TestEntry は対応表のテスト一つ。Name はテストの名前（ラベル）、File はそのテストの行を持つファイル（プロジェクトの
// ルートからの相対パス）、Lines は始まりの行と終わりの行（1 始まりで両端を含む）。
type TestEntry struct {
	Name  string `json:"name"`
	File  string `json:"file"`
	Lines [2]int `json:"lines"`
}

// testMapPrefix と testMapSuffix は、対応表のファイルの名前の、トランスパイラの名前の前と後ろ。
const (
	testMapPrefix = "hozo-tests."
	testMapSuffix = ".json"
)

// TestMapName は、トランスパイラ transpiler の対応表のファイルの名前（hozo-tests.<transpiler>.json）。
func TestMapName(transpiler string) string { return testMapPrefix + transpiler + testMapSuffix }

// TestMapTranspiler は、パス p のファイルの名前が対応表のもの（hozo-tests.<名前>.json）なら、その <名前> を返す。
func TestMapTranspiler(p string) (string, bool) {
	base := path.Base(p)
	if !strings.HasPrefix(base, testMapPrefix) || !strings.HasSuffix(base, testMapSuffix) || len(base) < len(testMapPrefix)+len(testMapSuffix) {
		return "", false
	}
	return base[len(testMapPrefix) : len(base)-len(testMapSuffix)], true
}

// TestMapFile は、dir の下に置く、トランスパイラ transpiler の対応表のファイル。テスト一つを一行に書く
// （並びと字下げは plugins/go/spec/go.hozo の Requirement.UnitTestPort の「テストの対応表」）。tests は空でないこと（口を出さない応答は、対応表を出さない）。
func TestMapFile(dir, transpiler string, tests []TestEntry) File {
	var b bytes.Buffer
	b.WriteString("{\n  \"tests\": [\n")
	for i, t := range tests {
		fmt.Fprintf(&b, "    {\"name\": %s, \"file\": %s, \"lines\": [%d, %d]}", jsonString(t.Name), jsonString(t.File), t.Lines[0], t.Lines[1])
		if i < len(tests)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n}\n")
	return File{Path: path.Join(dir, TestMapName(transpiler)), Content: b.String()}
}

// jsonString は s を JSON の文字列にする（HTML のための置き換えはしない）。
func jsonString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // 文字列の符号化は失敗しない
	return strings.TrimSuffix(b.String(), "\n")
}
