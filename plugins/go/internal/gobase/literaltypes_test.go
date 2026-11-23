package gobase

import (
	"strings"
	"testing"
)

// TestLiteralTypeDecls は docs/plugin-go.md「model」のリテラルの型のフィールド（構造体に項目を持たず、コメントの行を置く）と、
// 「union」のリテラルの型のアーム（値を持たない、包む型）、印で見分ける union（ふつうの、model のアームの union）:
// testdata/literals の型の宣言が、文書から、手で書いた、下のコードを含むこと。
func TestLiteralTypeDecls(t *testing.T) {
	_, files := emitFixture(t, "literals")
	src, ok := files["gen/app/types/types.go"]
	if !ok {
		t.Fatalf("gen/app/types/types.go がない: %v", fileNames(files))
	}
	for _, want := range []string{
		// リテラルの型のアームだけの union: 閉じた型
		`// Severity は診断の重さ（値の集まり）
type Severity interface{ isSeverity() }

func (SeverityError) isSeverity()   {}
func (SeverityWarning) isSeverity() {}
func (SeverityInfo) isSeverity()    {}

// SeverityError は union app.Severity のアーム Error（"error"）を包む型。
type SeverityError struct{}

// SeverityWarning は union app.Severity のアーム Warning（"warning"）を包む型。
type SeverityWarning struct{}

// SeverityInfo は union app.Severity のアーム Info（"info"）を包む型。
type SeverityInfo struct{}
`,
		// リテラルの型のフィールド: 項目の代わりの、コメントの行（フィールドの説明は、その前の行）
		`// CoreType は核の型
type CoreType struct {
	// 見分ける項目
	// kind は、いつも "core"（Go の項目は、ない）
	Name string ` + "`json:\"name\"`" + `
}
`,
		`// ListType は並びの型
type ListType struct {
	// kind は、いつも "list"（Go の項目は、ない）
	Of string ` + "`json:\"of\"`" + `
}
`,
		// 印で見分ける union: 構造体にマーカーを生やすだけ
		`// Type は型（kind の値で見分ける union）
type Type interface{ isType() }

func (CoreType) isType() {}
func (ListType) isType() {}
`,
		// リテラルの型のフィールドだけの model
		`// Marker はリテラルの型のフィールドだけの model
// tag は、いつも "marker"（Go の項目は、ない）
// version は、いつも 2（Go の項目は、ない）
type Marker struct{}
`,
		// model のアームと、文字列、整数、小数、真偽のリテラルの型のアーム
		`type Shape interface{ isShape() }

func (CoreType) isShape()   {}
func (ShapeFixed) isShape() {}
func (ShapeCount) isShape() {}
func (ShapeRatio) isShape() {}
func (ShapeYes) isShape()   {}

// ShapeFixed は union app.Shape のアーム Fixed（"fixed"）を包む型。
type ShapeFixed struct{}

// ShapeCount は union app.Shape のアーム Count（0）を包む型。
type ShapeCount struct{}

// ShapeRatio は union app.Shape のアーム Ratio（1.5）を包む型。
type ShapeRatio struct{}

// ShapeYes は union app.Shape のアーム Yes（true）を包む型。
type ShapeYes struct{}
`,
		// union と boolean? のフィールド、union の、例外の引数
		`type Report struct {
	Severity Severity ` + "`json:\"severity\"`" + `
	Flag     *bool    ` + "`json:\"flag,omitempty\"`" + `
	Shape    Shape    ` + "`json:\"shape,omitempty\"`" + `
}
`,
		`type Bad struct {
	Level Severity ` + "`json:\"level\"`" + `
}
`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("型の宣言が、文書から書いた期待を含まない\n--- want\n%s\n--- got\n%s", want, src)
		}
	}
}

// TestLiteralTypeBodies は docs/plugin-go.md「値」の、リテラルの型のアームに入れる式（<Union><アーム名>{}）、リテラルの型の
// フィールドを読む式（リテラルの式）と、レコードリテラル（そのフィールドを書かない）、「match」の variant（リテラルの型の
// アーム）と、三つに分ける形（対象の型が boolean?）: testdata/literals の op の本体が、文書から、手で書いた、下のコードと同じこと。
func TestLiteralTypeBodies(t *testing.T) {
	_, files := emitFixture(t, "literals")
	wiring, ok := files["gen/app/wiring.go"]
	if !ok {
		t.Fatalf("gen/app/wiring.go がない: %v", fileNames(files))
	}
	call := func(expr string) string {
		return "\tif err := " + expr + "; err != nil {\n\t\treturn err\n\t}\n"
	}
	inCase := func(expr string) string {
		return "\t\tif err := " + expr + "; err != nil {\n\t\t\treturn err\n\t\t}\n"
	}
	for _, tt := range []struct{ method, want string }{
		// union に入れる: 引数、return、レコードリテラルのフィールド、型を書いた val
		{"Raise", call(`o.Report(ctx, SeverityError{})`) + "\treturn nil"},
		{"Level", "\treturn SeverityWarning{}, nil"},
		{"File", call(`o.Keep(ctx, Report{Severity: SeverityInfo{}, Shape: ShapeFixed{}})`) + "\treturn nil"},
		{"Held", "\tvar s Severity = SeverityError{}\n\treturn s, nil"},
		// 整数、小数、真偽のリテラルの型のアームと、リテラルの型のフィールドを持つ model のアーム（kind を書かない）
		{"Shapes", call(`o.Draw(ctx, ShapeCount{})`) + call(`o.Draw(ctx, ShapeRatio{})`) + call(`o.Draw(ctx, ShapeYes{})`) +
			call(`o.Draw(ctx, CoreType{Name: "int"})`) + "\treturn nil"},
		// レコードリテラル: リテラルの型のフィールドの値は、書かない
		{"Core", "\treturn CoreType{Name: name}, nil"},
		{"Mark", "\treturn Marker{}, nil"},
		// リテラルの型のフィールドを読む式は、そのリテラルの式（当てはめる型に合わせる）
		{"KindOf", "\treturn \"core\", nil"},
		{"Version", call(`o.Count(ctx, 2)`) + "\treturn nil"},
		// variant: リテラルの型のアームの case は、値を持たない、包む型。アームの中の対象は、リテラルの式
		{"Speak", "\t// match (r.severity by app.Level)\n\tswitch r.Severity.(type) {\n" +
			"\tcase SeverityError:\n" + inCase(`o.Say(ctx, "error")`) +
			"\tcase SeverityWarning:\n" + inCase(`o.Say(ctx, "loud")`) +
			"\tcase SeverityInfo:\n" + inCase(`o.Say(ctx, "info")`) +
			"\tdefault:\n\t\treturn &runtime.ClassificationError{Classify: \"app.Level\"}\n\t}\n\treturn nil"},
		// 印で見分ける union: 型の switch。アームの中で、印の項目を読めば、リテラルの式
		{"Name", "\t// match (t by app.Kind)\n\tswitch t.(type) {\n" +
			"\tcase CoreType:\n" + inCase(`o.Say(ctx, t.(CoreType).Name)`) +
			"\tcase ListType:\n" + inCase(`o.Say(ctx, "list")`) +
			"\tdefault:\n\t\treturn &runtime.ClassificationError{Classify: \"app.Kind\"}\n\t}\n\treturn nil"},
		// variant: 残りのケースは、残りのアームの Go の型を、union の宣言の順に
		{"Form", `	// match (shape by app.Form)
	switch shape.(type) {
	case CoreType:
		return "page", nil
	case ShapeFixed:
		return "fixed", nil
	case ShapeCount, ShapeRatio, ShapeYes:
		return "number", nil
	default:
		return "", &runtime.ClassificationError{Classify: "app.Form"}
	}`},
		// 三つに分ける形: アームを書いた順（Unset、Off、On）によらず、真、偽、値がない、の順。最後の節は default:
		{"Flag", `	// match (r.flag by app.Flag)
	switch {
	case r.Flag != nil && *r.Flag:
		// On: r.flag は true
		return "on", nil
	case r.Flag != nil && !*r.Flag:
		// Off: r.flag は false
		return "off", nil
	default:
		// Unset
		return "unset", nil
	}`},
		// 偽と、値がない、を、一つのアームが受ける: 一つの節（最後なので default:）
		{"Merged", "\t// match (r.flag by app.Flag)\n\tswitch {\n" +
			"\tcase r.Flag != nil && *r.Flag:\n\t\t// On: r.flag は true\n" + inCase(`o.Say(ctx, "on")`) +
			"\tdefault:\n\t\t// Off, Unset\n" + inCase(`o.Say(ctx, "rest")`) +
			"\t}\n\treturn nil"},
		// 真と、値がない、を、一つのアームが受ける: 条件は <対象> == nil || *<対象>
		{"Apart", `	// match (r.flag by app.Flag)
	var text string
	switch {
	case r.Flag == nil || *r.Flag:
		// On, Unset
		text = "kept"
	default:
		// Off: r.flag は false
		text = "off"
	}
	return text, nil`},
		// 値がない側にケースがない: 最後の節は、分類の誤りの枝
		{"Strict", `	// match (r.flag by app.Strict)
	switch {
	case r.Flag != nil && *r.Flag:
		// Yes: r.flag は true
		return "yes", nil
	case r.Flag != nil && !*r.Flag:
		// No: r.flag は false
		return "no", nil
	default:
		return "", &runtime.ClassificationError{Classify: "app.Strict"}
	}`},
		// 残りのケースが、偽と、値がない、の両方。as true のアームの中の対象は *<対象>
		{"Lax", "\t// match (lit by app.Lax)\n\tswitch {\n" +
			"\tcase lit != nil && *lit:\n\t\t// Yes: lit は true\n" + inCase(`o.Show(ctx, *lit)`) +
			"\tdefault:\n\t\t// Other\n" + inCase(`o.Say(ctx, "other")`) +
			"\t}\n\treturn nil"},
		// 三つの側を、一つのアームが受けていれば、分けない
		{"All", "\t// match (r.flag by app.Flag)\n" + call(`o.Say(ctx, "any")`) + "\treturn nil"},
	} {
		if got := methodBody(t, wiring, tt.method); got != tt.want {
			t.Errorf("%s の本体が、文書から書いた期待と違う\n--- got\n%s\n--- want\n%s", tt.method, got, tt.want)
		}
	}
}

// TestOptionalLiteralPredicateGuard は docs/plugin-go.md「分類」の、literal の確かめの、対象の型が boolean?（v は *bool）のもの:
// 判定関数の中で、as true のケースの述語は `if v != nil && *v`、as false のケースの述語は `if v != nil && !*v` で包む。
// 期待は、文書から、手で書いた。
func TestOptionalLiteralPredicateGuard(t *testing.T) {
	_, files := emitFixture(t, "literals")
	src, ok := files["gen/app/types/classify.go"]
	if !ok {
		t.Fatalf("gen/app/types/classify.go がない: %v", fileNames(files))
	}
	guarded := func(cond, name string) string {
		return "\tif " + cond + " {\n\t\tif ok, err := c." + name + "(ctx, v); err != nil {\n\t\t\tif first == nil {\n\t\t\t\tfirst = err\n\t\t\t}\n" +
			"\t\t} else if ok {\n\t\t\thits = append(hits, \"" + name + "\")\n\t\t\tkind = Mood" + name + "\n\t\t}\n\t}\n"
	}
	want := "func ClassifyMood(ctx context.Context, c MoodCases, v *bool) (Mood, error) {\n\tvar hits []string\n\tvar kind Mood\n\tvar first error\n" +
		guarded("v != nil && *v", "Glad") + guarded("v != nil && *v", "Calm") + guarded("v != nil && !*v", "Sad")
	if !strings.Contains(src, want) {
		t.Errorf("ClassifyMood の頭が、文書から書いた期待と違う\n--- want\n%s\n--- got\n%s", want, src)
	}
}
