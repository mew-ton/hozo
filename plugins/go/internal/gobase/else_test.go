package gobase

import (
	"strings"
	"testing"
)

// methodBody は、src の中の、構造体 ops のメソッド name の本体（`{` の次の行から、行頭の `}` の前まで）。
func methodBody(t *testing.T, src, name string) string {
	t.Helper()
	head := "func (o *ops) " + name + "("
	i := strings.Index(src, head)
	if i < 0 {
		t.Fatalf("メソッド %s がない", name)
	}
	open := strings.Index(src[i:], "{\n")
	end := strings.Index(src[i:], "\n}\n")
	if open < 0 || end < 0 || end < open {
		t.Fatalf("メソッド %s の本体を切り出せない", name)
	}
	return src[i+open+2 : i+end]
}

// TestElseArmSameCode は docs/plugin-go.md「match」の「else のアームは、受けるケース（MatchArm.cases）を名前で並べたアームと、
// 同じコードになる」: testdata/matchswitch の describe（すべてのケースを名前で書く）と describeElse（最後のアームを else と
// 書く）は、仕様に書いたままのコメント以外、本体が同じ。基底は MatchArm.else を読まない。
func TestElseArmSameCode(t *testing.T) {
	_, files := emitFixture(t, "matchswitch")
	wiring, ok := files["gen/app/wiring.go"]
	if !ok {
		t.Fatalf("gen/app/wiring.go がない: %v", fileNames(files))
	}
	named, withElse := methodBody(t, wiring, "Describe"), methodBody(t, wiring, "DescribeElse")
	if named != withElse {
		t.Errorf("else のアームのコードが、ケースを名前で並べたアームと違う\n--- describe\n%s\n--- describeElse\n%s", named, withElse)
	}
	// else のアームに、Go の default: の節を使わない: default: の節は、分類の誤りの枝だけ
	for _, body := range []string{named, withElse} {
		if n := strings.Count(body, "default:"); n != 1 || !strings.Contains(body, "default:\n\t\treturn \"\", &runtime.ClassificationError{Classify: \"app.PageKind\"}") {
			t.Errorf("型の switch の default: の節は、分類の誤りの枝の一つだけ（%d 個）:\n%s", n, body)
		}
	}
	// 述語で決まる分類の switch は、else のアームも、受けるケースの定数を並べた case で、default: の節を持たない
	pure := methodBody(t, wiring, "PureLabel")
	if strings.Contains(pure, "default:") || !strings.Contains(pure, "case SizeBig, SizeSmall:") {
		t.Errorf("pureLabel の else のアームが、case SizeBig, SizeSmall: でない:\n%s", pure)
	}
}

func fileNames(files map[string]string) []string {
	var names []string
	for name := range files {
		names = append(names, name)
	}
	return names
}
