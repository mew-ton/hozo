package golang

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mew-ton/hozo/ir"
)

// GoType は IR の型を Go の型の文字列にする（docs/plugin-go.md「型」の表）。必要な import を imports に足す。
//
//	string / boolean / int / float   → string / bool / int64 / float64
//	Time / Duration                  → time.Time / time.Duration（"time" を import）
//	optional(T)                      → T が array、record、union（arms）への ref ならそのまま（nil が「なし」）。それ以外は *T
//	array(T)                         → []T
//	record(K, V)                     → map[K]V
//	ref                              → 型の名前（model は構造体、scalar は名前付きの型、union はインターフェース）
//	case(C, cases, of)               → of の型（絞り込みは Go の型にしない）
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
		case ir.CoreTime:
			return qualify(imports, "time", "time") + ".Time"
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
	case ir.RecordType:
		return "map[" + GoType(x.Key, imports) + "]" + GoType(x.Value, imports)
	case ir.CaseType:
		return GoType(x.Of, imports)
	}
	panic(fmt.Sprintf("golang: unknown ir.Type %T", t))
}

// IsNillable は、その型の Go の零値 nil が「なし」を表せるか（docs/plugin-go.md「型」の optional の行）: array、record、
// union（arms）への ref。case は of を見る。optional(T) が *T にならず T のままになるのは、T がこれに当たるとき。
// p は ref が union かどうかを引くための Program（nil なら、ref はどれも当たらないとする）。
func IsNillable(t ir.Type, p *ir.Program) bool {
	switch x := t.(type) {
	case ir.ArrayType, ir.RecordType:
		return true
	case ir.RefType:
		if p == nil {
			return false
		}
		u, ok := p.Lookup(x.Target).(*ir.Union)
		return ok && u.Variant == ir.UnionArms
	case ir.CaseType:
		return IsNillable(x.Of, p)
	}
	return false
}

// Zero は型の零値の Go の式（docs/plugin-go.md「呼び出し」の「零値」）: ポインタ・インターフェース・スライス・マップは nil、
// 構造体は `<型>{}`、time.Time は `time.Time{}`、文字列が元の型は `""`、数が元の型は `0`、真偽値は `false`。
// 必要な import を imports に足す。nil の型（void）と never は ""。
func Zero(t ir.Type, imports *Imports) string {
	switch x := t.(type) {
	case nil, ir.NeverType:
		return ""
	case ir.OptionalType, ir.ArrayType, ir.RecordType:
		return "nil"
	case ir.CaseType:
		return Zero(x.Of, imports)
	case ir.CoreType:
		return coreZero(x, GoType(x, imports))
	case ir.RefType:
		switch d := imports.program().Lookup(x.Target).(type) {
		case *ir.Union:
			return "nil"
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

// coreZero は核の型 c の零値。goType は、その値の Go の型（time.Time か、Time が元の scalar の名前）。
func coreZero(c ir.CoreType, goType string) string {
	switch c.Name {
	case ir.CoreString:
		return `""`
	case ir.CoreBoolean:
		return "false"
	case ir.CoreTime:
		return goType + "{}"
	}
	return "0"
}

// RowDisplay は、コメントの中の行の遷移の書き方（docs/plugin-go.md「コメント」の「行の遷移の表示」）: 仕様の書き方で、
// IR（Effect.row）から組み立てる。ケースは `<分類の FQN>.<ケース名>`、前のケースの並びの区切りは `, `。
// post は `as <後>`、put は `as <前> -> <後>`（後が決まっていなければ else）、delete は、前が any なら `as <後>`、
// そうでなければ `as <前> -> <後>`。行の遷移を持たない効果（と、宛先が組み込みの効果でない効果）は ""。
func RowDisplay(e *ir.Effect) string {
	target, ok := e.Target.(ir.BuiltinTarget)
	if !ok || e.Row == nil {
		return ""
	}
	row := e.Row
	name := func(c string) string { return row.Classify + "." + c }
	after := "else"
	if row.To != "" {
		after = name(row.To)
	}
	before := "any"
	if !row.From.Any {
		names := make([]string, len(row.From.Cases))
		for i, c := range row.From.Cases {
			names[i] = name(c)
		}
		before = strings.Join(names, ", ")
	}
	switch target.Operation {
	case ir.OpPost:
		return "as " + after
	case ir.OpDelete:
		if row.From.Any {
			return "as " + after
		}
	}
	return "as " + before + " -> " + after
}

// TypeDisplay は、コメントの中の型の書き方（docs/plugin-go.md「コメント」の「型の表示」）: 核の型は名前、ref は FQN、
// optional(T) は `T?`、array(T) は `T[]`、record(K, V) は `Record<K, V>`、case はその of。
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
	case ir.RecordType:
		return "Record<" + TypeDisplay(x.Key) + ", " + TypeDisplay(x.Value) + ">"
	case ir.CaseType:
		return TypeDisplay(x.Of)
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
