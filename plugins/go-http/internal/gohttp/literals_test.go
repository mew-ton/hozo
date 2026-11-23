package gohttp

import (
	"strings"
	"testing"
)

// TestLiteralTables は docs/plugin-go-http.md「補助」の「union の読み書き」の、二つの表: httpUnions の、リテラルの型のアーム
// （value だけ）と、印で見分ける union の model のアーム（tag と value。包む型は wrap）、決め手で見分けるアーム（keys）。
// httpConsts の、リテラルの型のフィールドを持つ model（at は、その前にある、Go の項目の数）。入力は testdata/literals で、
// 期待は、文書から、手で書いた。二つの http.go（定義のパッケージと、テストのパッケージ）が、同じ表を持つ。
func TestLiteralTables(t *testing.T) {
	_, files := emitFixture(t, "literals")
	unions := "// httpUnions は、通信に出る union ごとの、アームの並び（union の宣言の順）。\n" +
		"var httpUnions = map[reflect.Type][]httpArm{\n" +
		"\treflect.TypeFor[Q.Severity](): {\n" +
		"\t\t{typ: reflect.TypeFor[Q.SeverityError](), value: `\"error\"`},\n" +
		"\t\t{typ: reflect.TypeFor[Q.SeverityWarning](), value: `\"warning\"`},\n" +
		"\t},\n" +
		"\treflect.TypeFor[Q.Type](): {\n" +
		"\t\t{typ: reflect.TypeFor[Q.CoreType](), tag: \"kind\", value: `\"core\"`},\n" +
		"\t\t{typ: reflect.TypeFor[Q.ListType](), tag: \"kind\", value: `\"list\"`},\n" +
		"\t\t{typ: reflect.TypeFor[Q.TypeMap](), tag: \"kind\", value: `\"map\"`, wrap: true},\n" +
		"\t},\n" +
		"\treflect.TypeFor[Q.Amount](): {\n" +
		"\t\t{typ: reflect.TypeFor[Q.AmountNone](), value: `0`},\n" +
		"\t\t{typ: reflect.TypeFor[Q.AmountHalf](), value: `0.5`},\n" +
		"\t\t{typ: reflect.TypeFor[Q.AmountAll](), value: `true`},\n" +
		"\t\t{typ: reflect.TypeFor[Q.Count](), keys: []string{\"n\"}},\n" +
		"\t},\n" +
		"}\n"
	consts := "// httpConsts は、リテラルの型のフィールドを持つ model ごとの、そのフィールドの並び（フィールドの順）。\n" +
		"var httpConsts = map[reflect.Type][]httpConst{\n" +
		"\treflect.TypeFor[Q.Report](): {\n" +
		"\t\t{at: 3, key: \"version\", value: `2`},\n" +
		"\t},\n" +
		"\treflect.TypeFor[Q.CoreType](): {\n" +
		"\t\t{at: 1, key: \"kind\", value: `\"core\"`},\n" +
		"\t},\n" +
		"\treflect.TypeFor[Q.ListType](): {\n" +
		"\t\t{at: 0, key: \"kind\", value: `\"list\"`},\n" +
		"\t},\n" +
		"\treflect.TypeFor[kinds.MapType](): {\n" +
		"\t\t{at: 0, key: \"kind\", value: `\"map\"`},\n" +
		"\t},\n" +
		"}\n"
	for path, qualifier := range map[string]string{"gen/api/http.go": "", "gen/api/apitest/http.go": "api."} {
		src, ok := files[path]
		if !ok {
			t.Fatalf("%s がない", path)
		}
		for _, want := range []string{strings.ReplaceAll(unions, "Q.", qualifier), strings.ReplaceAll(consts, "Q.", qualifier)} {
			if !strings.Contains(src, want) {
				t.Errorf("%s が、文書から書いた表を含まない\n--- want\n%s", path, want)
			}
		}
	}
}
