package validvocab

import (
	"reflect"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは CheckLiteral（docs/plugin-valid.md「値一つを確かめる」）: 値一つを、フィールドの制約で確かめる。
// 値が制約を通るかは、同「制約」の表の「通す値」。

// app は、制約の適用を組み立てる。args は、引数名とリテラルを交互に。
func app(decorator string, args ...any) ir.Application {
	a := ir.Application{Decorator: decorator}
	for i := 0; i+1 < len(args); i += 2 {
		a.Args = append(a.Args, ir.Arg{Param: args[i].(string), Value: ir.LiteralValue{Literal: args[i+1].(ir.Literal)}})
	}
	return a
}

func str(v string) ir.Literal  { return ir.StringLiteral{Value: v} }
func num(v int64) ir.Literal   { return ir.IntLiteral{Value: v} }
func dec(v float64) ir.Literal { return ir.FloatLiteral{Value: v} }

// constrainedScalar は、制約の付いた scalar。
func constrainedScalar(fqn string, base ir.Type, apps ...ir.Application) *ir.Scalar {
	return &ir.Scalar{DeclBase: ir.DeclBase{Name: fqn, Decorators: apps}, Base: base}
}

// constrainedField は、制約の付いたフィールド。
func constrainedField(name string, t ir.Type, apps ...ir.Application) *ir.Field {
	return &ir.Field{Name: name, Type: t, Decorators: apps}
}

func TestCheckLiteral(t *testing.T) {
	p := program(map[string][]ir.Decl{
		"type": {
			constrainedScalar("type.Page", ir.Int(), app("valid.min", "value", num(1))),
			constrainedScalar("type.SmallPage", ir.Ref("type.Page"), app("valid.max", "value", num(10))),
			constrainedScalar("type.Ratio", ir.Float(), app("valid.min", "value", dec(0)), app("valid.max", "value", dec(1))),
			constrainedScalar("type.Name", ir.String(), app("valid.length", "min", num(1), "max", num(3))),
			constrainedScalar("type.Email", ir.String(), app("valid.format", "name", str("email"))),
			constrainedScalar("type.Plain", ir.String()),
			// 標準でない valid の制約は、中身を人が書く: ここでは確かめない
			constrainedScalar("type.Code", ir.String(), app("valid.checkDigit")),
		},
	}, "type")
	fieldMin := app("valid.min", "value", num(5))
	for _, tc := range []struct {
		name  string
		field *ir.Field
		lit   ir.Literal
		want  []Mismatch
	}{
		{"制約がない", constrainedField("f", ir.Int()), num(-1), nil},
		{"フィールドの min に合う", constrainedField("f", ir.Int(), fieldMin), num(5), nil},
		{"フィールドの min に合わない", constrainedField("f", ir.Int(), fieldMin), num(4),
			[]Mismatch{{Decorator: "valid.min", Text: "@valid.min(5)"}}},
		{"フィールドの max に合わない", constrainedField("f", ir.Int(), app("valid.max", "value", num(5))), num(6),
			[]Mismatch{{Decorator: "valid.max", Text: "@valid.max(5)"}}},
		{"64 ビットの端の整数も、正確に比べる", constrainedField("f", ir.Int(), app("valid.max", "value", num(9007199254740992))), num(9007199254740993),
			[]Mismatch{{Decorator: "valid.max", Text: "@valid.max(9007199254740992)"}}},
		{"scalar の制約に合う", constrainedField("f", ir.Ref("type.Page")), num(1), nil},
		{"scalar の制約に合わない", constrainedField("f", ir.Ref("type.Page")), num(0),
			[]Mismatch{{Decorator: "valid.min", Text: "@valid.min(1)", Scalar: "type.Page"}}},
		// 確かめる順は Validate と同じ: 元の scalar、自分の scalar、フィールド
		{"元の scalar、scalar、フィールドの順", constrainedField("f", ir.Ref("type.SmallPage"), app("valid.min", "value", num(-5)), app("valid.max", "value", num(-1))), num(0), []Mismatch{
			{Decorator: "valid.min", Text: "@valid.min(1)", Scalar: "type.Page"},
			{Decorator: "valid.max", Text: "@valid.max(-1)"},
		}},
		{"元の scalar には合い、自分の scalar に合わない", constrainedField("f", ir.Ref("type.SmallPage")), num(11),
			[]Mismatch{{Decorator: "valid.max", Text: "@valid.max(10)", Scalar: "type.SmallPage"}}},
		// 整数のリテラルは float にも渡せる（docs/types.md「互換」の 10）
		{"float に渡した整数", constrainedField("f", ir.Ref("type.Ratio")), num(2),
			[]Mismatch{{Decorator: "valid.max", Text: "@valid.max(1.0)", Scalar: "type.Ratio"}}},
		{"float の小数", constrainedField("f", ir.Ref("type.Ratio")), dec(0.5), nil},
		{"float の min に合わない", constrainedField("f", ir.Ref("type.Ratio")), dec(-0.5),
			[]Mismatch{{Decorator: "valid.min", Text: "@valid.min(0.0)", Scalar: "type.Ratio"}}},
		// length は、コードポイントで数える
		{"length に合う", constrainedField("f", ir.Ref("type.Name")), str("日本語"), nil},
		{"length の max に合わない", constrainedField("f", ir.Ref("type.Name")), str("abcd"),
			[]Mismatch{{Decorator: "valid.length", Text: "@valid.length(min: 1, max: 3)", Scalar: "type.Name"}}},
		{"length の min に合わない", constrainedField("f", ir.String(), app("valid.length", "min", num(2))), str("a"),
			[]Mismatch{{Decorator: "valid.length", Text: "@valid.length(min: 2)"}}},
		{"pattern に合う", constrainedField("f", ir.String(), app("valid.pattern", "regex", str("^[a-z]+$"))), str("abc"), nil},
		{"pattern に合わない", constrainedField("f", ir.String(), app("valid.pattern", "regex", str("^[a-z]+$"))), str("ABC"),
			[]Mismatch{{Decorator: "valid.pattern", Text: `@valid.pattern("^[a-z]+$")`}}},
		{"format に合う", constrainedField("f", ir.Ref("type.Email")), str("a@example.com"), nil},
		{"format に合わない", constrainedField("f", ir.Ref("type.Email")), str("nobody"),
			[]Mismatch{{Decorator: "valid.format", Text: `@valid.format("email")`, Scalar: "type.Email"}}},
		// 省略可能なフィールドは、中の型で見る
		{"省略可能", constrainedField("f", ir.Optional(ir.Ref("type.Page"))), num(0),
			[]Mismatch{{Decorator: "valid.min", Text: "@valid.min(1)", Scalar: "type.Page"}}},
		// 確かめられないものは、合わないとは言わない: リテラルの型に当たらない制約と、解釈できない正規表現（VALID-3）、
		// 利用者が足した制約、valid の語彙でないデコレータ
		{"型の合わない制約", constrainedField("f", ir.String(), fieldMin), str("x"), nil},
		{"真偽値と期間には、当たる制約がない", constrainedField("f", ir.Boolean(), fieldMin, app("valid.length", "min", num(9))), ir.BooleanLiteral{Value: false}, nil},
		{"解釈できない正規表現", constrainedField("f", ir.String(), app("valid.pattern", "regex", str("("))), str("x"), nil},
		{"利用者が足した制約", constrainedField("f", ir.Ref("type.Code"), app("valid.even")), str("x"), nil},
		{"valid でないデコレータ", constrainedField("f", ir.Ref("type.Plain"), app("http.query"), app("db.int")), str(""), nil},
		{"配列のフィールド", constrainedField("f", ir.Array(ir.Ref("type.Page"))), num(0), nil},
	} {
		if got := CheckLiteral(p, tc.field, tc.lit); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: CheckLiteral = %+v, want %+v", tc.name, got, tc.want)
		}
	}
	if got := CheckLiteral(nil, constrainedField("f", ir.Int(), fieldMin), num(0)); len(got) != 1 {
		t.Errorf("Program がなくても、フィールドの制約は確かめる: %+v", got)
	}
	if got := CheckLiteral(p, nil, num(0)); got != nil {
		t.Errorf("フィールドがなければ、何も返さない: %+v", got)
	}
	// 自分を元に持つ scalar（hozoc の検査が通さないもの）でも、止まる
	loop := program(map[string][]ir.Decl{"x": {constrainedScalar("x.A", ir.Ref("x.A"), fieldMin)}}, "x")
	if got := CheckLiteral(loop, constrainedField("f", ir.Ref("x.A")), num(0)); len(got) != 1 {
		t.Errorf("自分を元に持つ scalar: %+v", got)
	}
}
