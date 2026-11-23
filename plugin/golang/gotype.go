package golang

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mew-ton/hozo/ir"
)

// RuntimePath は、生成したコードが import する runtime パッケージ（docs/plugin-go.md「runtime パッケージ」）。
const RuntimePath = "github.com/mew-ton/hozo/runtime"

// GoType は IR の型を Go の型の文字列にする（docs/plugin-go.md「型」の表）。必要な import を imports に足す。
//
//	string / boolean / int / float   → string / bool / int64 / float64
//	datetime / duration              → time.Time / time.Duration（"time" を import）
//	date / time                      → runtime.Date / runtime.TimeOfDay（RuntimePath を import）
//	optional(T)                      → T が array、インデックスの model への ref、union（arms）への ref ならそのまま（nil が「なし」）。それ以外は *T
//	array(T)                         → []T
//	ref                              → 型の名前（model は構造体、インデックスの model は名前付きのマップ、scalar は名前付きの型、union はインターフェース）
//	case(C, cases, of)               → of の型（絞り込みは Go の型にしない）
//	literal                          → 対応する核の型の Go の型（配線の本体の中の値）。model のフィールドの型と、union のアームの型は、基底が、別の形にする（同「model」「union」）。ほかの位置は、基底が GO-9 で断る（同「診断」）。
//	                                   断られる位置の型にも、形を返すのは、相乗りのプラグインが、同じ View で、止まらないため
//
// ref の修飾は imports が決める: 見る側の名前空間 N の型は修飾なし（定義のパッケージでも、入口のパッケージでも。入口は別名を持つ）、
// 別の名前空間 M の型は、M の型の置き場（docs/config.md「型の置き場」）の入口のパッケージを import して、その名前で修飾する。
// 置き場がなければ、修飾なしの名前を返し、imports.Err がそれを報告する（CFG-22 が通したはずのもの。基底は GO-INTERNAL にする）。
// imports が nil なら import は記録せず、すべての ref を修飾なしの名前にする。nil の型（void）と never は "" を返す。
func GoType(t ir.Type, imports *Imports) string {
	switch x := t.(type) {
	case nil, ir.NeverType:
		return ""
	case ir.CoreType:
		switch x.Name {
		case ir.CoreString:
			return "string"
		case ir.CoreBoolean:
			return "bool"
		case ir.CoreInt:
			return "int64"
		case ir.CoreFloat:
			return "float64"
		case ir.CoreDatetime:
			return qualify(imports, "time", "time") + ".Time"
		case ir.CoreDate:
			return qualify(imports, RuntimePath, "runtime") + ".Date"
		case ir.CoreTime:
			return qualify(imports, RuntimePath, "runtime") + ".TimeOfDay"
		case ir.CoreDuration:
			return qualify(imports, "time", "time") + ".Duration"
		}
		panic(fmt.Sprintf("golang: unknown core type %q", x.Name))
	case ir.RefType:
		return imports.ref(x.Target)
	case ir.OptionalType:
		inner := GoType(x.Of, imports)
		if IsNillable(x.Of, imports.program()) {
			return inner
		}
		return "*" + inner
	case ir.ArrayType:
		return "[]" + GoType(x.Of, imports)
	case ir.CaseType:
		return GoType(x.Of, imports)
	case ir.LiteralType:
		return GoType(ir.LiteralCore(x.Literal), imports)
	}
	panic(fmt.Sprintf("golang: unknown ir.Type %T", t))
}

// IsNillable は、その型の Go の零値 nil が「なし」を表せるか（docs/plugin-go.md「型」の optional の行）: array、
// インデックスの model への ref、union（arms）への ref。case は of を見る。optional(T) が *T にならず T のままになるのは、
// T がこれに当たるとき。p は ref が何の宣言かを引くための Program（nil なら、ref はどれも当たらないとする）。
func IsNillable(t ir.Type, p *ir.Program) bool {
	switch x := t.(type) {
	case ir.ArrayType:
		return true
	case ir.RefType:
		if p == nil {
			return false
		}
		switch d := p.Lookup(x.Target).(type) {
		case *ir.Union:
			return d.Variant == ir.UnionArms
		case *ir.Model:
			return d.Index != nil
		}
		return false
	case ir.CaseType:
		return IsNillable(x.Of, p)
	}
	return false
}

// Zero は型の零値の Go の式（docs/plugin-go.md「呼び出し」の「零値」）: ポインタ・インターフェース・スライス・マップ（インデックスの model）は nil、
// 構造体は `<型>{}`、time.Time、runtime.Date、runtime.TimeOfDay は `<型>{}`、文字列が元の型は `""`、数が元の型は `0`、真偽値は `false`。
// 必要な import を imports に足す。nil の型（void）と never は ""。
func Zero(t ir.Type, imports *Imports) string {
	switch x := t.(type) {
	case nil, ir.NeverType:
		return ""
	case ir.OptionalType, ir.ArrayType:
		return "nil"
	case ir.CaseType:
		return Zero(x.Of, imports)
	case ir.CoreType:
		return coreZero(x, GoType(x, imports))
	case ir.LiteralType:
		core := ir.LiteralCore(x.Literal)
		return coreZero(core, GoType(core, imports))
	case ir.RefType:
		switch d := imports.program().Lookup(x.Target).(type) {
		case *ir.Union:
			return "nil"
		case *ir.Model:
			if d.Index != nil {
				return "nil"
			}
		case *ir.Scalar:
			// 元の型を核の型まで辿る（scalar の元は、核の型か、別の scalar）
			base := d.Base
			for depth := 0; depth < 32; depth++ {
				r, isRef := base.(ir.RefType)
				if !isRef {
					break
				}
				next, isScalar := imports.program().Lookup(r.Target).(*ir.Scalar)
				if !isScalar {
					break
				}
				base = next.Base
			}
			if core, ok := base.(ir.CoreType); ok {
				return coreZero(core, GoType(x, imports))
			}
		}
		return GoType(x, imports) + "{}"
	}
	return "nil"
}

// coreZero は核の型 c の零値。goType は、その値の Go の型（time.Time、runtime.Date、runtime.TimeOfDay か、それが元の scalar の名前）。
func coreZero(c ir.CoreType, goType string) string {
	switch c.Name {
	case ir.CoreString:
		return `""`
	case ir.CoreBoolean:
		return "false"
	case ir.CoreDatetime, ir.CoreDate, ir.CoreTime:
		return goType + "{}"
	}
	return "0"
}

// RowDisplay は、コメントの中の行の遷移の書き方（docs/plugin-go.md「コメント」の「行の遷移の表示」）: 仕様の書き方で、
// IR（Effect.row）から組み立てる。後のケースがあれば `as <分類の FQN>.<ケース名>`、なければ `as else`。
// 行の遷移を持たない効果（と、宛先が組み込みの効果でない効果）は ""。
func RowDisplay(e *ir.Effect) string {
	if _, ok := e.Target.(ir.BuiltinTarget); !ok || e.Row == nil {
		return ""
	}
	if e.Row.To == "" {
		return "as else"
	}
	return "as " + e.Row.Classify + "." + e.Row.To
}

// NarrowedParamsDisplay は、コメントの中の、絞り込んだ引数の書き方（docs/plugin-go.md「コメント」の「絞り込んだ引数の表示」）:
// 効果の引数のうち、型が case のものを、宣言の順に、「、」でつなぐ。一つは `<引数名> on <分類の FQN>.<ケース名>`
// （いくつかのケースは " | " でつなぐ）。なければ ""。行の引数なら、これが、効果の前のケース。
func NarrowedParamsDisplay(e *ir.Effect) string {
	var parts []string
	for _, p := range e.Params {
		c, ok := p.Type.(ir.CaseType)
		if !ok {
			continue
		}
		cases := make([]string, len(c.Cases))
		for i, name := range c.Cases {
			cases[i] = c.Classify + "." + name
		}
		parts = append(parts, p.Name+" on "+strings.Join(cases, " | "))
	}
	return strings.Join(parts, "、")
}

// TypeDisplay は、コメントの中の型の書き方（docs/plugin-go.md「コメント」の「型の表示」）: 核の型は名前、ref は FQN、
// optional(T) は `T?`、array(T) は `T[]`、case はその of、リテラルの型は、仕様の綴り（`"error"`、`3`、`true`）。
func TypeDisplay(t ir.Type) string {
	switch x := t.(type) {
	case nil:
		return ""
	case ir.CoreType:
		return string(x.Name)
	case ir.RefType:
		return x.Target
	case ir.OptionalType:
		return TypeDisplay(x.Of) + "?"
	case ir.ArrayType:
		return TypeDisplay(x.Of) + "[]"
	case ir.CaseType:
		return TypeDisplay(x.Of)
	case ir.LiteralType:
		return ir.LiteralString(x.Literal)
	case ir.NeverType:
		return "never"
	}
	return ""
}

// qualify は imports に pkgPath を足し、修飾に使う名前を返す。imports が nil ならパッケージ名そのまま。
func qualify(imports *Imports, pkgPath, pkgName string) string {
	if imports == nil {
		return pkgName
	}
	return imports.Add(pkgPath, pkgName)
}

// guessNamespace は FQN の名前空間を、先頭が大文字でないセグメントの先頭からの並びとみなす
// （Program がないときの補い。宣言名は大文字で始まる慣習 NAME-1 に頼る）。
// `server.OrderPage.Ready` → `server`、`server.admin.Report` → `server.admin`、`Db` → ""（空）。
func guessNamespace(fqn string) string {
	segs := strings.Split(fqn, ".")
	n := 0
	for n < len(segs)-1 {
		r, _ := utf8.DecodeRuneInString(segs[n])
		if unicode.IsUpper(r) {
			break
		}
		n++
	}
	return strings.Join(segs[:n], ".")
}
